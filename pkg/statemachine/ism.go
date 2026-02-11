package statemachine

import (
	"sync"
	"time"
)

// ISM (Input State Machine) tracks the physical state of all input devices
type ISM struct {
	mu      sync.RWMutex
	current InputState
	history *RingBuffer[InputState]
	devices map[DeviceID]*DeviceState
	config  SemanticConfig
}

// DeviceState tracks state for a single input device
type DeviceState struct {
	ID        DeviceID
	KeyStates map[KeyCode]KeyState
	LastEvent time.Time
}

// NewISM creates a new Input State Machine
func NewISM(config SemanticConfig) *ISM {
	return &ISM{
		current: InputState{
			Timestamp: time.Now(),
			KeyStates: make(map[KeyCode]KeyState),
			EventSeq:  make([]KeyEvent, 0),
		},
		history: NewRingBuffer[InputState](100),
		devices: make(map[DeviceID]*DeviceState),
		config:  config,
	}
}

// ProcessEvent processes a raw key event and updates the input state
func (ism *ISM) ProcessEvent(event KeyEvent) (InputState, bool) {
	ism.mu.Lock()
	defer ism.mu.Unlock()

	// Save current state to history before modification
	if ism.current.Timestamp.After(time.Time{}) {
		ism.history.Push(ism.current.Clone())
	}

	// Update device state
	device, exists := ism.devices[event.Device]
	if !exists {
		device = &DeviceState{
			ID:        event.Device,
			KeyStates: make(map[KeyCode]KeyState),
		}
		ism.devices[event.Device] = device
	}

	// Check for debounce
	if existing, ok := ism.current.KeyStates[event.Key]; ok {
		if event.Action == KeyPress && existing.Pressed {
			// Duplicate press — check debounce threshold
			if event.Timestamp.Sub(existing.PressedAt) < ism.config.DebounceThreshold {
				// Within debounce window, ignore
				return ism.current.Clone(), false
			}
			// After debounce window, treat as key repeat
			existing.PressCount++
			existing.PressedAt = event.Timestamp
			ism.current.KeyStates[event.Key] = existing
			return ism.current.Clone(), true
		}
		if event.Action == KeyRelease && !existing.Pressed {
			// Duplicate release, ignore
			return ism.current.Clone(), false
		}
	}

	// Update key state
	switch event.Action {
	case KeyPress:
		ism.current.KeyStates[event.Key] = KeyState{
			Pressed:    true,
			PressedAt:  event.Timestamp,
			Device:     event.Device,
			PressCount: 1,
		}
		device.KeyStates[event.Key] = ism.current.KeyStates[event.Key]

	case KeyRelease:
		// Keep the PressedAt for history tracking
		if existing, ok := ism.current.KeyStates[event.Key]; ok {
			ism.current.KeyStates[event.Key] = KeyState{
				Pressed:    false,
				PressedAt:  existing.PressedAt,
				Device:     event.Device,
				PressCount: 0,
			}
		}
		delete(device.KeyStates, event.Key)
	}

	// Update event sequence (limit to max 100 events to prevent memory leak)
	const maxEventSeqLen = 100
	ism.current.EventSeq = append(ism.current.EventSeq, event)
	if len(ism.current.EventSeq) > maxEventSeqLen {
		ism.current.EventSeq = ism.current.EventSeq[len(ism.current.EventSeq)-maxEventSeqLen:]
	}
	ism.current.Timestamp = event.Timestamp
	device.LastEvent = event.Timestamp

	return ism.current.Clone(), true
}

// GetCurrent returns the current input state
func (ism *ISM) GetCurrent() InputState {
	ism.mu.RLock()
	defer ism.mu.RUnlock()
	return ism.current.Clone()
}

// GetHistory returns historical states
func (ism *ISM) GetHistory() []InputState {
	ism.mu.RLock()
	defer ism.mu.RUnlock()
	return ism.history.Slice()
}

// GetDeviceState returns the state for a specific device
func (ism *ISM) GetDeviceState(deviceID DeviceID) (DeviceState, bool) {
	ism.mu.RLock()
	defer ism.mu.RUnlock()
	device, ok := ism.devices[deviceID]
	if !ok {
		return DeviceState{}, false
	}
	// Clone the device state
	cloned := DeviceState{
		ID:        device.ID,
		KeyStates: make(map[KeyCode]KeyState),
		LastEvent: device.LastEvent,
	}
	for k, v := range device.KeyStates {
		cloned.KeyStates[k] = v
	}
	return cloned, true
}

// GetAllPressedKeys returns all currently pressed keys across all devices
func (ism *ISM) GetAllPressedKeys() []KeyCode {
	ism.mu.RLock()
	defer ism.mu.RUnlock()

	keys := make([]KeyCode, 0, len(ism.current.KeyStates))
	for key, state := range ism.current.KeyStates {
		if state.Pressed {
			keys = append(keys, key)
		}
	}
	return keys
}

// IsKeyPressed checks if a specific key is currently pressed
func (ism *ISM) IsKeyPressed(key KeyCode) bool {
	ism.mu.RLock()
	defer ism.mu.RUnlock()
	state, ok := ism.current.KeyStates[key]
	return ok && state.Pressed
}

// GetKeyHoldDuration returns how long a key has been held
func (ism *ISM) GetKeyHoldDuration(key KeyCode) time.Duration {
	ism.mu.RLock()
	defer ism.mu.RUnlock()
	state, ok := ism.current.KeyStates[key]
	if !ok || !state.Pressed {
		return 0
	}
	return time.Since(state.PressedAt)
}

// RemoveDevice removes a device and all its associated key states
func (ism *ISM) RemoveDevice(deviceID DeviceID) {
	ism.mu.Lock()
	defer ism.mu.Unlock()

	for key, state := range ism.current.KeyStates {
		if state.Device == deviceID {
			delete(ism.current.KeyStates, key)
		}
	}
	delete(ism.devices, deviceID)
}

// Clear resets the input state machine
func (ism *ISM) Clear() {
	ism.mu.Lock()
	defer ism.mu.Unlock()

	ism.current = InputState{
		Timestamp: time.Now(),
		KeyStates: make(map[KeyCode]KeyState),
		EventSeq:  make([]KeyEvent, 0),
	}
	ism.history.Clear()
	ism.devices = make(map[DeviceID]*DeviceState)
}
