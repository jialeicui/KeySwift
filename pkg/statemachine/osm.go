package statemachine

import (
	"log/slog"
	"sync"
	"time"
)

// OSM (Output State Machine) manages the desired and actual output states
type OSM struct {
	mu      sync.RWMutex
	desired OutputState
	actual  OutputState
	binder  *StateBinder
	output  OutputDevice
	config  AlignmentConfig
	// Track compensated keys for alignment
	compensatedKeys map[KeyCode]time.Time
	// Track pending releases
	pendingReleases map[KeyCode]time.Time
}

// NewOSM creates a new Output State Machine
func NewOSM(binder *StateBinder, output OutputDevice, config AlignmentConfig) *OSM {
	return &OSM{
		desired: OutputState{
			PressedKeys: make(map[KeyCode]OutputKeyInfo),
		},
		actual: OutputState{
			PressedKeys: make(map[KeyCode]OutputKeyInfo),
		},
		binder:          binder,
		output:          output,
		config:          config,
		compensatedKeys: make(map[KeyCode]time.Time),
		pendingReleases: make(map[KeyCode]time.Time),
	}
}

// ApplyCommands applies output commands from the mapping engine
func (osm *OSM) ApplyCommands(commands []OutputCommand, inputKeys []KeyCode, mappingID MappingID) error {
	osm.mu.Lock()
	defer osm.mu.Unlock()

	// Group commands by action
	var toPress, toRelease []KeyCode
	for _, cmd := range commands {
		switch cmd.Action {
		case KeyPress:
			toPress = append(toPress, cmd.Key)
		case KeyRelease:
			toRelease = append(toRelease, cmd.Key)
		}
	}

	// Process releases first (for clean state transitions)
	for _, key := range toRelease {
		if _, ok := osm.desired.PressedKeys[key]; ok {
			delete(osm.desired.PressedKeys, key)
			if err := osm.executeRelease(key); err != nil {
				return err
			}
		}
	}

	// Process presses
	for _, key := range toPress {
		if _, ok := osm.desired.PressedKeys[key]; !ok {
			osm.desired.PressedKeys[key] = OutputKeyInfo{
				PressedAt:       time.Now(),
				SourceMapping:   mappingID,
				BoundTo:         append([]KeyCode{}, inputKeys...),
				ReleaseStrategy: ReleaseOnAnyBoundKeyReleased,
			}
			if err := osm.executePress(key); err != nil {
				return err
			}
		}
	}

	// Create binding between input and output keys
	if len(inputKeys) > 0 && len(toPress) > 0 {
		osm.binder.CreateBinding(inputKeys, toPress, ReleaseOnAnyBoundKeyReleased)
	}

	return nil
}

// HandleInputRelease handles an input key release event
func (osm *OSM) HandleInputRelease(key KeyCode) error {
	osm.mu.Lock()
	defer osm.mu.Unlock()

	// Check if this triggers any output releases via binding
	toRelease := osm.binder.OnInputKeyReleased(key)

	for _, outKey := range toRelease {
		if _, ok := osm.desired.PressedKeys[outKey]; ok {
			delete(osm.desired.PressedKeys, outKey)
			if err := osm.executeRelease(outKey); err != nil {
				return err
			}
		}
	}

	// Check if this was a compensated key
	if _, ok := osm.compensatedKeys[key]; ok {
		delete(osm.compensatedKeys, key)
		slog.Debug("Compensated key physically released", "key", key)
	}

	return nil
}

// Sync synchronizes desired state with actual state
func (osm *OSM) Sync() error {
	osm.mu.Lock()
	defer osm.mu.Unlock()

	diff := osm.calculateDiff()

	// Process releases first
	for _, key := range diff.ShouldRelease {
		// Check if this is a compensated key
		if _, ok := osm.compensatedKeys[key]; ok {
			// Don't release yet, wait for physical release
			osm.pendingReleases[key] = time.Now()
			continue
		}

		if err := osm.executeRelease(key); err != nil {
			return err
		}
	}

	// Process presses
	for _, key := range diff.ShouldPress {
		if err := osm.executePress(key); err != nil {
			return err
		}
	}

	return nil
}

// EmergencyRelease releases all output keys (safety mechanism)
func (osm *OSM) EmergencyRelease() []KeyCode {
	osm.mu.Lock()
	defer osm.mu.Unlock()

	var released []KeyCode
	for key := range osm.desired.PressedKeys {
		released = append(released, key)
		if err := osm.executeRelease(key); err != nil {
			slog.Error("Failed to emergency release key", "key", key, "error", err)
		}
	}

	osm.desired.PressedKeys = make(map[KeyCode]OutputKeyInfo)
	osm.binder.Clear()

	slog.Warn("Emergency release executed", "releasedKeys", released)
	return released
}

// MarkCompensated marks a key as compensated
func (osm *OSM) MarkCompensated(key KeyCode) {
	osm.mu.Lock()
	defer osm.mu.Unlock()
	osm.compensatedKeys[key] = time.Now()
}

// IsCompensated checks if a key is compensated
func (osm *OSM) IsCompensated(key KeyCode) bool {
	osm.mu.RLock()
	defer osm.mu.RUnlock()
	_, ok := osm.compensatedKeys[key]
	return ok
}

// GetDesiredState returns the desired output state
func (osm *OSM) GetDesiredState() OutputState {
	osm.mu.RLock()
	defer osm.mu.RUnlock()
	return osm.desired.Clone()
}

// GetActualState returns the actual output state
func (osm *OSM) GetActualState() OutputState {
	osm.mu.RLock()
	defer osm.mu.RUnlock()
	return osm.actual.Clone()
}

// GetDiff returns the current state difference
func (osm *OSM) GetDiff() StateDiff {
	osm.mu.RLock()
	defer osm.mu.RUnlock()
	return osm.calculateDiff()
}

// executePress executes a key press
func (osm *OSM) executePress(key KeyCode) error {
	cmd := OutputCommand{Key: key, Action: KeyPress}
	if err := osm.output.Execute(cmd); err != nil {
		return err
	}
	osm.actual.PressedKeys[key] = osm.desired.PressedKeys[key]
	slog.Debug("Executed key press", "key", key)
	return nil
}

// executeRelease executes a key release
func (osm *OSM) executeRelease(key KeyCode) error {
	cmd := OutputCommand{Key: key, Action: KeyRelease}
	if err := osm.output.Execute(cmd); err != nil {
		return err
	}
	delete(osm.actual.PressedKeys, key)
	delete(osm.pendingReleases, key)
	slog.Debug("Executed key release", "key", key)
	return nil
}

// calculateDiff calculates the difference between desired and actual states
func (osm *OSM) calculateDiff() StateDiff {
	var diff StateDiff

	// Find keys that should be pressed but aren't
	for key := range osm.desired.PressedKeys {
		if _, ok := osm.actual.PressedKeys[key]; !ok {
			diff.ShouldPress = append(diff.ShouldPress, key)
		}
	}

	// Find keys that should be released but aren't
	for key := range osm.actual.PressedKeys {
		if _, ok := osm.desired.PressedKeys[key]; !ok {
			diff.ShouldRelease = append(diff.ShouldRelease, key)
		}
	}

	return diff
}

// CheckTimeouts checks for and handles timed-out keys
func (osm *OSM) CheckTimeouts() []KeyCode {
	osm.mu.Lock()
	defer osm.mu.Unlock()

	now := time.Now()
	var timedOut []KeyCode

	for key, info := range osm.desired.PressedKeys {
		if info.ReleaseStrategy == ReleaseOnTimeout {
			if now.Sub(info.PressedAt) > osm.config.EmergencyTimeout {
				timedOut = append(timedOut, key)
			}
		}
	}

	// Release timed out keys
	for _, key := range timedOut {
		delete(osm.desired.PressedKeys, key)
		if err := osm.executeRelease(key); err != nil {
			slog.Error("Failed to release timed-out key", "key", key, "error", err)
		}
	}

	return timedOut
}

// Clear resets the output state machine
func (osm *OSM) Clear() {
	osm.mu.Lock()
	defer osm.mu.Unlock()

	// Release all keys
	for key := range osm.actual.PressedKeys {
		if err := osm.executeRelease(key); err != nil {
			slog.Error("Failed to release key during clear", "key", key, "error", err)
		}
	}

	osm.desired = OutputState{PressedKeys: make(map[KeyCode]OutputKeyInfo)}
	osm.actual = OutputState{PressedKeys: make(map[KeyCode]OutputKeyInfo)}
	osm.compensatedKeys = make(map[KeyCode]time.Time)
	osm.pendingReleases = make(map[KeyCode]time.Time)
}

// Reset clears tracked output state without writing to the device.
func (osm *OSM) Reset() {
	osm.mu.Lock()
	defer osm.mu.Unlock()

	osm.desired = OutputState{PressedKeys: make(map[KeyCode]OutputKeyInfo)}
	osm.actual = OutputState{PressedKeys: make(map[KeyCode]OutputKeyInfo)}
	osm.compensatedKeys = make(map[KeyCode]time.Time)
	osm.pendingReleases = make(map[KeyCode]time.Time)
	osm.binder.Clear()
}
