package dev

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/jialeicui/golibevdev"

	"github.com/jialeicui/keyswift/pkg/mode"
)

// InputDevice represents a grabbed input device
type InputDevice struct {
	Device *golibevdev.InputDev
	Name   string
	Path   string
}

// InputDeviceManager manages multiple input devices
type InputDeviceManager struct {
	devices []*InputDevice
	wg      sync.WaitGroup
}

// NewInputDeviceManager creates a new input device manager
func NewInputDeviceManager() *InputDeviceManager {
	return &InputDeviceManager{
		devices: make([]*InputDevice, 0),
	}
}

// GetDevices returns all input devices
func (m *InputDeviceManager) GetDevices() []*InputDevice {
	return m.devices
}

// AddDevice adds and grabs a new input device
func (m *InputDeviceManager) AddDevice(name, path string) error {
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
func (m *InputDeviceManager) ProcessEvents(virtualKeyboard *golibevdev.UInputDev, modeManager *mode.Manager) {
	for _, dev := range m.devices {
		m.wg.Add(1)
		go func(d *InputDevice) {
			defer m.wg.Done()
			m.processDeviceEvents(d, virtualKeyboard, modeManager)
		}(dev)
	}
}

// processDeviceEvents processes events from a single device
func (m *InputDeviceManager) processDeviceEvents(dev *InputDevice, virtualKeyboard *golibevdev.UInputDev, modeManager *mode.Manager) {
	slog.Info("Starting event processing for device", "device", dev.Name)
	mu := sync.Mutex{}

	lastPressed := map[golibevdev.KeyEventCode]struct{}{}
	keyStack := []golibevdev.Event{}

	for {
		ev, err := dev.Device.NextEvent(golibevdev.ReadFlagNormal)
		if err != nil {
			slog.Error("Error reading from device", "device", dev.Name, "error", err)
			return
		}

		// Always add the event to the stack first
		keyStack = append(keyStack, ev)

		var handled bool
		if ev.Type == golibevdev.EvKey {
			if ev.Value != 0 && ev.Value != 1 {
				continue
			}

			mu.Lock()
			if ev.Value == 1 {
				lastPressed[ev.Code.(golibevdev.KeyEventCode)] = struct{}{}
			} else if ev.Value == 0 {
				delete(lastPressed, ev.Code.(golibevdev.KeyEventCode))
			}
			// Convert to our event type
			var ks []string
			for key := range lastPressed {
				ks = append(ks, keyCodeToString(key))
			}
			mu.Unlock()

			if len(ks) == 0 {
				continue
			}

			event := &mode.Event{
				KeyPress: &mode.KeyPressEvent{
					Keys:    ks,
					Pressed: ev.Value == 1,
				},
			}

			// Process the event through the mode manager
			handled, err = modeManager.ProcessEvent(event)
			if err != nil {
				slog.Error("Error processing event", "device", dev.Name, "error", err)
				keyStack = keyStack[:0]
				continue
			}
		}

		// If the event wasn't handled by any mode, send all collected events in order
		if !handled {
			for _, ev := range keyStack {
				// Directly send the original key event to the output device
				_ = virtualKeyboard.WriteEvent(ev.Type, ev.Code, ev.Value)
			}
			keyStack = keyStack[:0]
		}
	}
}

// Wait waits for all event processing to complete
func (m *InputDeviceManager) Wait() {
	m.wg.Wait()
}

// Close closes all input devices
func (m *InputDeviceManager) Close() {
	for _, dev := range m.devices {
		slog.Info("Closing device", "device", dev.Name)
		dev.Device.Close()
	}
}

// keyCodeToString converts a key code to a string representation
// This is a simplified version - you'd want a complete mapping
func keyCodeToString(code golibevdev.KeyEventCode) string {
	// You'll need a complete mapping from golibevdev key codes to strings
	// This is just a starting point
	keyMap := map[golibevdev.KeyEventCode]string{
		golibevdev.KeyLeftMeta: "cmd",
		golibevdev.KeyEsc:      "esc",
		golibevdev.KeyA:        "a",
		golibevdev.KeyB:        "b",
		golibevdev.KeyC:        "c",
		golibevdev.KeyD:        "d",
		golibevdev.KeyE:        "e",
		golibevdev.KeyF:        "f",
		golibevdev.KeyG:        "g",
		golibevdev.KeyH:        "h",
		golibevdev.KeyI:        "i",
		golibevdev.KeyJ:        "j",
		golibevdev.KeyK:        "k",
		golibevdev.KeyL:        "l",
		golibevdev.KeyM:        "m",
		golibevdev.KeyN:        "n",
		golibevdev.KeyO:        "o",
		golibevdev.KeyP:        "p",
		golibevdev.KeyQ:        "q",
		golibevdev.KeyR:        "r",
		golibevdev.KeyS:        "s",
		golibevdev.KeyT:        "t",
		golibevdev.KeyU:        "u",
		golibevdev.KeyV:        "v",
		golibevdev.KeyW:        "w",
		golibevdev.KeyX:        "x",
		golibevdev.KeyY:        "y",
		golibevdev.KeyZ:        "z",
		// Add more keys as needed
	}

	if name, ok := keyMap[code]; ok {
		return name
	}
	return code.String()
}
