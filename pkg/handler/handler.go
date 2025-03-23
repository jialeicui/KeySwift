package handler

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jialeicui/golibevdev"
	"github.com/samber/lo"

	"github.com/jialeicui/keyswift/pkg/bus"
)

const (
	KeyPressed  = 1
	KeyReleased = 0
)

// InputDevice represents a grabbed input device
type InputDevice struct {
	Device *golibevdev.InputDev
	Name   string
	Path   string
}

// Handler manages multiple input devices
type Handler struct {
	devices []*InputDevice
	wg      sync.WaitGroup

	out *golibevdev.UInputDev
}

// New creates a new input device handler
func New() *Handler {
	return &Handler{
		devices: make([]*InputDevice, 0),
	}
}

// GetDevices returns all input devices
func (m *Handler) GetDevices() []*InputDevice {
	return m.devices
}

// AddDevice adds and grabs a new input device
func (m *Handler) AddDevice(name, path string) error {
	dev, err := golibevdev.NewInputDev(path)
	if err != nil {
		return fmt.Errorf("failed to open input device %s: %w", path, err)
	}

	if err = dev.Grab(); err != nil {
		dev.Close()
		return fmt.Errorf("failed to grab input device %s: %w", path, err)
	}

	m.devices = append(m.devices, &InputDevice{
		Device: dev,
		Name:   name,
		Path:   path,
	})
	return nil
}

// ProcessEvents starts processing events from all devices
func (m *Handler) ProcessEvents(virtualKeyboard *golibevdev.UInputDev, modeManager *bus.Impl) {
	m.out = virtualKeyboard
	for _, dev := range m.devices {
		m.wg.Add(1)
		go func(d *InputDevice) {
			defer m.wg.Done()
			m.processDeviceEvents(d, modeManager)
		}(dev)
	}
}

// KeyState represents the state of a key
type KeyState struct {
	Time time.Time
}

// processDeviceEvents processes events from a single device
func (m *Handler) processDeviceEvents(dev *InputDevice, modeManager *bus.Impl) {
	slog.Info("Starting event processing for device", "device", dev.Name)
	mu := sync.Mutex{}

	var (
		keyStates       = make(map[golibevdev.KeyEventCode]KeyState)
		eventStack      []golibevdev.Event
		modifier        = NewModifier()
		passThroughKeys = make(map[golibevdev.KeyEventCode]struct{})
		byPassKeys      = make(map[golibevdev.KeyEventCode]struct{})

		lastKeyIsModifier  = false
		lastEventIsRelease = false
	)

	// modifier key(here we only consider ctrl, alt) always pass through
	// when modifier key + other key hit the rules
	// we simulate the related modifier key release event to output device
	// e.g. When ctrl pressed, we send the press event of ctrl to output device
	// and then if c pressed, and ctrl+c hit the rules, we send the release event of ctrl to output device
	// this is useful for the scenario like holding ctrl and click mouse in browser to open new tab

	modeManager.SetBeforeSendKeysPerSession(func() {
		if len(passThroughKeys) == 0 {
			return
		}

		for key := range passThroughKeys {
			_ = m.out.WriteEvent(golibevdev.EvKey, key, 0)
		}
		_ = m.out.WriteEvent(golibevdev.EvSyn, golibevdev.SynReport, 0)

		passThroughKeys = make(map[golibevdev.KeyEventCode]struct{})
	})

	for {
		ev, err := dev.Device.NextEvent(golibevdev.ReadFlagNormal)
		if err != nil {
			slog.Error("Error reading from device", "device", dev.Name, "error", err)
			return
		}

		slog.Debug("event", "code", ev.Code, "value", ev.Value, "time", ev.Time.UnixMicro())

		// Handle sync events
		if ev.Type == golibevdev.EvSyn {
			if len(keyStates) == 0 && len(passThroughKeys) > 0 {
				passThroughKeys = make(map[golibevdev.KeyEventCode]struct{})
			}
			eventStack = append(eventStack, ev)
			// Process any pending events in the stack
			forceNoPassThrough := lastKeyIsModifier && !lastEventIsRelease
			handled := m.processEventStack(eventStack, keyStates, modeManager, forceNoPassThrough)
			if !forceNoPassThrough {
				eventStack = eventStack[:0]
			}
			if handled {
				byPassKeys = make(map[golibevdev.KeyEventCode]struct{})
				for key := range keyStates {
					byPassKeys[key] = struct{}{}
				}
				continue
			}
			for key := range keyStates {
				_, ok := passThroughKeys[key]
				if !ok && modifier.ShouldPassThrough(key) {
					passThroughKeys[key] = struct{}{}
					m.sendSingleKey(key, KeyPressed)
				}
			}
			continue
		}

		// Handle key events
		if ev.Type == golibevdev.EvKey {
			if ev.Value != KeyPressed && ev.Value != KeyReleased {
				continue
			}

			keyCode := ev.Code.(golibevdev.KeyEventCode)
			isModifier := modifier.IsModifier(keyCode)
			lastKeyIsModifier = isModifier
			lastEventIsRelease = ev.Value == KeyReleased

			// Update key state
			mu.Lock()
			if ev.Value == KeyPressed {
				keyStates[keyCode] = KeyState{
					Time: ev.Time,
				}
				if isModifier {
					modifier.Press(keyCode)
				}
			} else {
				delete(keyStates, keyCode)
				if isModifier {
					modifier.Release(keyCode)
				}
			}
			mu.Unlock()

			if ev.Value == KeyReleased {
				k := ev.Code.(golibevdev.KeyEventCode)
				if _, ok := byPassKeys[k]; ok {
					slog.Debug("drop key release event", "key", k.String())
					delete(byPassKeys, k)
					continue
				}
			}

			// Add event to stack
			eventStack = append(eventStack, ev)
		}
	}
}

// processEventStack processes a stack of events and determines if they should be handled
// return true if the events should be handled, false if the events should be forwarded
func (m *Handler) processEventStack(
	events []golibevdev.Event,
	keyStates map[golibevdev.KeyEventCode]KeyState,
	modeManager *bus.Impl,
	forceNoPassThrough bool,
) bool {
	// Get currently pressed keys
	pressedKeys := lo.Keys(keyStates)

	if len(pressedKeys) == 0 {
		// No keys are pressed, just forward all events
		for _, ev := range events {
			_ = m.out.WriteEvent(ev.Type, ev.Code, ev.Value)
		}
		slog.Debug("forward all events", "events", events)
		return false
	}

	// Create event for bus processing
	event := &bus.Event{
		KeyPress: &bus.KeyPressEvent{
			Keys:    pressedKeys,
			Pressed: true,
		},
	}

	// Process through bus manager
	handled, err := modeManager.ProcessEvent(event)
	if err != nil {
		slog.Error("Error processing event", "error", err)
		return false
	}

	if handled {
		// If handled, we don't forward the events
		return true
	}

	if forceNoPassThrough {
		slog.Debug("force no pass through", "events", events)
		return false
	}

	// If not handled, forward all events in order
	for _, ev := range events {
		if ev.Type == golibevdev.EvKey {
			slog.Debug("Forwarding key event", "key", ev.Code.(golibevdev.KeyEventCode).String(), "pressed", ev.Value)
		}
		_ = m.out.WriteEvent(ev.Type, ev.Code, ev.Value)
	}

	return false
}

func (m *Handler) sendSingleKey(code golibevdev.KeyEventCode, value int32) {
	_ = m.out.WriteEvent(golibevdev.EvKey, code, value)
	_ = m.out.WriteEvent(golibevdev.EvSyn, golibevdev.SynReport, 0)
	slog.Debug("send single key", "code", code, "value", value)
}

// Wait waits for all event processing to complete
func (m *Handler) Wait() {
	m.wg.Wait()
}

// Close closes all input devices
func (m *Handler) Close() {
	for _, dev := range m.devices {
		slog.Info("Closing device", "device", dev.Name)
		dev.Device.Close()
	}
}
