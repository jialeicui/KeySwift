package statemachine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// Machine is the main state machine that orchestrates all components
type Machine struct {
	config   Config
	ism      *ISM
	ssm      *SSM
	osm      *OSM
	binder   *StateBinder
	executor *SpeculativeExecutor
	engine   MappingEngine
	output   OutputDevice

	// Lifecycle
	stopCh chan struct{}
	wg     sync.WaitGroup

	// State
	mu                 sync.RWMutex
	running            bool
	modifierStates     map[KeyCode]ModifierState
	passthroughPressed map[KeyCode]struct{}

	// Callbacks
	onMappingTriggered func([]OutputCommand)
}

// NewMachine creates a new state machine with all components
func NewMachine(engine MappingEngine, output OutputDevice, config Config) *Machine {
	binder := NewStateBinder()

	return &Machine{
		config:             config,
		ism:                NewISM(config.Semantic),
		ssm:                NewSSM(config.Semantic, engine),
		binder:             binder,
		osm:                NewOSM(binder, output, config.Alignment),
		executor:           NewSpeculativeExecutor(config.Speculative, output),
		engine:             engine,
		output:             output,
		modifierStates:     make(map[KeyCode]ModifierState),
		passthroughPressed: make(map[KeyCode]struct{}),
		stopCh:             make(chan struct{}),
	}
}

// ProcessEvent processes a key event synchronously through the state machine
func (m *Machine) ProcessEvent(event KeyEvent) error {
	m.mu.RLock()
	if !m.running {
		m.mu.RUnlock()
		return fmt.Errorf("state machine is not running")
	}
	m.mu.RUnlock()

	err := m.handleEvent(event)
	if errors.Is(err, ErrOutputResetRequired) {
		m.HandleOutputFailure()
		return nil
	}
	return err
}

// ProcessEventSync is an alias for ProcessEvent (kept for test compatibility)
func (m *Machine) ProcessEventSync(event KeyEvent) error {
	return m.ProcessEvent(event)
}

// Start starts the state machine background processing
func (m *Machine) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("state machine is already running")
	}

	m.running = true
	m.stopCh = make(chan struct{})

	// Start sync ticker
	m.wg.Add(1)
	go m.syncLoop(ctx)

	// Start deadline checker
	if m.config.Speculative.Enabled {
		m.wg.Add(1)
		go m.deadlineChecker(ctx)
	}

	slog.Info("State machine started")
	return nil
}

// Stop stops the state machine
func (m *Machine) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("State machine stopped gracefully")
	case <-time.After(5 * time.Second):
		slog.Warn("State machine stop timed out, forcing emergency release")
		m.osm.EmergencyRelease()
	}

	return nil
}

// GetInputState returns the current input state
func (m *Machine) GetInputState() InputState {
	return m.ism.GetCurrent()
}

// GetSemanticState returns the current semantic state
func (m *Machine) GetSemanticState() SemanticState {
	return m.ssm.GetCurrent()
}

// GetOutputState returns the current desired output state
func (m *Machine) GetOutputState() OutputState {
	return m.osm.GetDesiredState()
}

// SetMappingTriggeredCallback sets the callback for when a mapping is triggered
func (m *Machine) SetMappingTriggeredCallback(cb func([]OutputCommand)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMappingTriggered = cb
}

// EmergencyRelease forces release of all output keys
func (m *Machine) EmergencyRelease() []KeyCode {
	return m.osm.EmergencyRelease()
}

// HandleDeviceLost synthesizes release events for all pressed keys from a device.
func (m *Machine) HandleDeviceLost(deviceID DeviceID) error {
	deviceState, ok := m.ism.GetDeviceState(deviceID)
	if !ok {
		return nil
	}

	lostKeys := make([]lostDeviceKey, 0, len(deviceState.KeyStates))
	for key, state := range deviceState.KeyStates {
		if state.Pressed {
			lostKeys = append(lostKeys, lostDeviceKey{key: key, state: state})
		}
	}

	sort.Slice(lostKeys, func(i, j int) bool {
		if lostKeys[i].state.PressedAt.Equal(lostKeys[j].state.PressedAt) {
			return lostKeys[i].key < lostKeys[j].key
		}
		return lostKeys[i].state.PressedAt.After(lostKeys[j].state.PressedAt)
	})

	for _, lost := range lostKeys {
		if err := m.ProcessEvent(KeyEvent{
			Key:       lost.key,
			Action:    KeyRelease,
			Timestamp: time.Now(),
			Device:    deviceID,
		}); err != nil {
			return err
		}
	}

	m.ism.RemoveDevice(deviceID)
	m.cleanupLostDeviceState(lostKeys)
	return nil
}

// HandleOutputFailure clears tracked state after the output device is rebuilt.
func (m *Machine) HandleOutputFailure() {
	slog.Warn("Resetting state machine after output device recovery")

	m.mu.Lock()
	m.modifierStates = make(map[KeyCode]ModifierState)
	m.passthroughPressed = make(map[KeyCode]struct{})
	m.mu.Unlock()

	m.ism.Clear()
	m.ssm.Clear()
	m.osm.Reset()
	m.executor.Clear()
}

// ModifierState tracks the lifecycle of an active modifier
type ModifierState int

type lostDeviceKey struct {
	key   KeyCode
	state KeyState
}

const (
	ModifierStateAbsorbed ModifierState = iota
	ModifierStateUsedInMapping
	ModifierStateFlushed
)

func (m *Machine) cleanupLostDeviceState(lostKeys []lostDeviceKey) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, lost := range lostKeys {
		delete(m.modifierStates, lost.key)
		delete(m.passthroughPressed, lost.key)
	}
}

func (m *Machine) executePassthrough(cmd OutputCommand) error {
	if err := m.output.Execute(cmd); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if cmd.Action == KeyPress {
		m.passthroughPressed[cmd.Key] = struct{}{}
	} else {
		delete(m.passthroughPressed, cmd.Key)
	}

	return nil
}

// handleEvent handles a single key event
func (m *Machine) handleEvent(event KeyEvent) error {
	slog.Debug("handleEvent called", "key", event.Key, "action", event.Action)

	// Step 1: Update ISM
	inputState, changed := m.ism.ProcessEvent(event)
	slog.Debug("ISM ProcessEvent result", "changed", changed, "keyStates", len(inputState.KeyStates), "eventSeqLen", len(inputState.EventSeq))
	if !changed {
		return nil
	}

	// Step 2: Handle key release in OSM (for binding cleanup)
	if event.Action == KeyRelease {
		if err := m.osm.HandleInputRelease(event.Key); err != nil {
			return err
		}
	}

	// Step 3: Translate to semantic state
	semanticState := m.ssm.Translate(inputState)
	slog.Debug("SSM Translate result", "modifiersActive", len(semanticState.Modifiers.Active), "comboKeys", len(semanticState.Combo.ActiveKeys))

	// Step 4: Check for mapping match
	commands, matched := m.engine.Match(semanticState)
	slog.Debug("Engine Match result", "matched", matched, "commands", commands)
	if matched {
		m.mu.Lock()
		var toRelease []KeyCode
		for modKey := range semanticState.Modifiers.Active {
			if state, exists := m.modifierStates[modKey]; exists && state == ModifierStateFlushed {
				// Previously flushed for an unmapped combo, but now used in a mapping.
				// We must release it from the OS, otherwise it will remain stuck.
				slog.Debug("Releasing previously flushed modifier before mapping", "key", modKey)
				toRelease = append(toRelease, modKey)
			}
			m.modifierStates[modKey] = ModifierStateUsedInMapping
		}
		m.mu.Unlock()

		for _, rel := range toRelease {
			if err := m.executePassthrough(OutputCommand{Key: rel, Action: KeyRelease}); err != nil {
				return err
			}
		}

		return m.handleMapping(commands, inputState, semanticState)
	}

	// Step 5: No mapping matched — decide whether to forward

	if event.Action == KeyPress {
		if _, isActive := semanticState.Modifiers.Active[event.Key]; isActive {
			slog.Debug("Active modifier absorbed (waiting for combo)", "key", event.Key)
			m.mu.Lock()
			m.modifierStates[event.Key] = ModifierStateAbsorbed
			m.mu.Unlock()
			return nil
		}

		// Non-modifier key was pressed and didn't match any mappings.
		// Flush absorbed modifiers to OS so native shortcuts work.
		if !IsModifier(event.Key) {
			m.mu.Lock()
			var toFlush []KeyCode
			for modKey, state := range m.modifierStates {
				if state == ModifierStateAbsorbed {
					toFlush = append(toFlush, modKey)
				}
			}
			m.mu.Unlock()

			for _, modKey := range toFlush {
				slog.Debug("Flushing absorbed modifier for unmapped combo", "key", modKey)
				if err := m.executePassthrough(OutputCommand{Key: modKey, Action: KeyPress}); err != nil {
					return err
				}
				m.mu.Lock()
				m.modifierStates[modKey] = ModifierStateFlushed
				m.mu.Unlock()
			}
		}
	}

	if event.Action == KeyRelease && IsModifier(event.Key) {
		m.mu.Lock()
		state, exists := m.modifierStates[event.Key]
		if exists {
			delete(m.modifierStates, event.Key)
		}
		m.mu.Unlock()

		if exists {
			if state == ModifierStateAbsorbed {
				slog.Debug("Absorbed modifier released without combo, tapping", "key", event.Key)
				if err := m.executePassthrough(OutputCommand{Key: event.Key, Action: KeyPress}); err != nil {
					return err
				}
				return m.executePassthrough(OutputCommand{Key: event.Key, Action: KeyRelease})
			} else if state == ModifierStateFlushed {
				slog.Debug("Flushed modifier released, passing through release", "key", event.Key)
				return m.executePassthrough(OutputCommand{Key: event.Key, Action: KeyRelease})
			} else if state == ModifierStateUsedInMapping {
				slog.Debug("Used modifier released, absorbing release", "key", event.Key)
				return nil
			}
		}
	}

	// Non-modifier keys or independent modifiers: forward directly
	slog.Debug("Forwarding event", "key", event.Key, "action", event.Action)
	cmd := OutputCommand{
		Key:    event.Key,
		Action: event.Action,
	}
	return m.executePassthrough(cmd)
}

// handleMapping handles a successful mapping match
func (m *Machine) handleMapping(commands []OutputCommand, inputState InputState, semanticState SemanticState) error {
	// Get all active input keys for binding
	inputKeys := make([]KeyCode, 0)
	for key, state := range inputState.KeyStates {
		if state.Pressed {
			inputKeys = append(inputKeys, key)
		}
	}

	// Check for speculative operations that need confirmation
	for _, cmd := range commands {
		if cmd.Action == KeyPress && m.executor.IsPending(cmd.Key) {
			m.executor.Confirm(cmd.Key)
		}
	}

	// Compensate any pending modifiers that are not part of this mapping
	for key := range semanticState.Modifiers.Pending {
		// Check if this key is in the input keys
		found := false
		for _, inKey := range inputKeys {
			if inKey == key {
				found = true
				break
			}
		}
		if !found {
			// This pending modifier is not part of the mapping
			// Mark it as passed through and compensate if needed
			m.ssm.MarkPendingPassedThrough(key)
		}
	}

	// Apply the mapping commands
	if err := m.osm.ApplyCommands(commands, inputKeys, "mapping"); err != nil {
		return fmt.Errorf("failed to apply commands: %w", err)
	}

	// Trigger callback
	if m.onMappingTriggered != nil {
		m.onMappingTriggered(commands)
	}

	slog.Debug("Mapping triggered",
		"inputKeys", inputKeys,
		"commands", commands)

	return nil
}

// syncLoop periodically synchronizes output state
func (m *Machine) syncLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.Alignment.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.osm.Sync(); err != nil {
				if errors.Is(err, ErrOutputResetRequired) {
					m.HandleOutputFailure()
					continue
				}
				slog.Error("Failed to sync output state", "error", err)
			}

			// Check for timed-out keys
			if timedOut := m.osm.CheckTimeouts(); len(timedOut) > 0 {
				slog.Warn("Released timed-out keys", "keys", timedOut)
			}

		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		}
	}
}

// deadlineChecker checks for expired speculative operation deadlines
func (m *Machine) deadlineChecker(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			expired := m.executor.CheckDeadlines()
			if len(expired) > 0 {
				slog.Debug("Auto-confirmed expired speculative operations", "keys", expired)
			}

			// Also check for pending modifier timeouts
			m.checkPendingModifierTimeouts()

		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		}
	}
}

// checkPendingModifierTimeouts checks and handles pending modifier timeouts
func (m *Machine) checkPendingModifierTimeouts() {
	// This triggers the SSM to downgrade pending modifiers
	// The actual downgrade happens in the next Translate call
	// But we can force it by getting the current state
	inputState := m.ism.GetCurrent()
	_ = m.ssm.Translate(inputState)
}
