package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/jialeicui/golibevdev"
	"github.com/samber/lo"

	"github.com/jialeicui/keyswift/pkg/conf"
	"github.com/jialeicui/keyswift/pkg/evdev"
	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/jialeicui/keyswift/pkg/mode"
	"github.com/jialeicui/keyswift/pkg/wininfo/dbus"
)

var (
	flagKeyboards = flag.String("keyboards", "keyboard", "Comma-separated list of keyboard device name substrings")
	flagConfig    = flag.String("config", "", "Configuration file path (defaults to $XDG_CONFIG_HOME/keyswift/config.yaml)")
	// TODO change this to false
	flagVerbose = flag.Bool("verbose", true, "Enable verbose logging")
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
	for {
		ev, err := dev.Device.NextEvent(golibevdev.ReadFlagNormal)
		if err != nil {
			slog.Error("Error reading from device", "device", dev.Name, "error", err)
			return
		}

		slog.Debug("Received event", "device", dev.Name, "event", ev)

		var handled bool
		if ev.Type == golibevdev.EvKey {
			// Convert to our event type
			event := &mode.Event{
				KeyPress: &mode.KeyPressEvent{
					Key:      keyCodeToString(ev.Code.(golibevdev.KeyEventCode)),
					Pressed:  ev.Value == 1,
					Repeated: ev.Value == 2,
				},
			}
			slog.Debug("Key press event", "device", dev.Name, "keyPress", event.KeyPress)

			// Process the event through the mode manager
			handled, err = modeManager.ProcessEvent(event)
			if err != nil {
				slog.Error("Error processing event", "device", dev.Name, "error", err)
				continue
			}
		}
		// If the event wasn't handled by any mode, pass it through directly
		if !handled {
			slog.Debug("Passing through unhandled event", "device", dev.Name, "event", ev)
			// Directly send the original key event to the output device
			virtualKeyboard.WriteEvent(ev.Type, ev.Code, ev.Value)
		}
		slog.Debug("--------------------")
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

func main() {
	flag.Parse()

	// Configure logging
	if *flagVerbose {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	// Load configuration
	configPath := *flagConfig
	if configPath == "" {
		configPath = conf.DefaultConfigPath()
	}

	c, err := conf.Load(configPath)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded config from %s\n", configPath)

	// Initialize window info service
	windowMonitor, err := dbus.New()
	if err != nil {
		slog.Error("Failed to initialize window monitor", "error", err)
		os.Exit(1)
	}
	defer windowMonitor.Close()
	fmt.Println("Window Monitor service is running...")

	// Initialize virtual keyboard for output
	out, err := golibevdev.NewVirtualKeyboard("keyswift")
	if err != nil {
		slog.Error("Failed to create virtual keyboard", "error", err)
		os.Exit(1)
	}
	defer out.Close()
	gnomeKeys := keys.NewGnomeKeys(out)

	// Initialize mode manager
	modeManager, err := mode.NewManager(c, gnomeKeys, windowMonitor)
	if err != nil {
		slog.Error("Failed to initialize mode manager", "error", err)
		os.Exit(1)
	}
	fmt.Println("Mode manager initialized")

	// Find input devices
	devs, err := evdev.NewOverviewImpl().ListInputDevices()
	if err != nil {
		slog.Error("Failed to list input devices", "error", err)
		os.Exit(1)
	}

	// Parse keyboard patterns and find matching devices
	keyboardPatterns := strings.Split(*flagKeyboards, ",")
	var matchedDevices []*evdev.InputDevice

	for _, pattern := range keyboardPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		matches := lo.Filter(devs, func(item *evdev.InputDevice, _ int) bool {
			return strings.Contains(item.Name, pattern)
		})

		matchedDevices = append(matchedDevices, matches...)
	}

	// Remove duplicates
	matchedDevices = lo.UniqBy(matchedDevices, func(dev *evdev.InputDevice) string {
		return dev.Path
	})

	if len(matchedDevices) == 0 {
		fmt.Println("Available keyboards:")
		for _, d := range devs {
			fmt.Printf("  - %s\n", d.Name)
		}
		slog.Error("No keyboards matching patterns", "patterns", *flagKeyboards)
		os.Exit(1)
	}

	// Initialize and set up the device manager
	deviceManager := NewInputDeviceManager()
	defer deviceManager.Close()

	// Add all matched devices to the manager
	for _, dev := range matchedDevices {
		fmt.Printf("Using keyboard: %s (%s)\n", dev.Name, dev.Path)
		if err := deviceManager.AddDevice(dev.Name, dev.Path); err != nil {
			slog.Warn("Failed to add device", "device", dev.Name, "error", err)
			continue
		}
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("Shutting down...")
		deviceManager.Close()
		out.Close()
		windowMonitor.Close()
		os.Exit(0)
	}()

	// Start processing events from all devices
	fmt.Printf("Processing events from %d devices... Press Ctrl+C to exit\n", len(deviceManager.devices))
	deviceManager.ProcessEvents(out, modeManager)

	// Wait for all processing to complete (typically won't reach here except on error)
	deviceManager.Wait()
}

// keyCodeToString converts a key code to a string representation
// This is a simplified version - you'd want a complete mapping
func keyCodeToString(code golibevdev.KeyEventCode) string {
	// You'll need a complete mapping from golibevdev key codes to strings
	// This is just a starting point
	keyMap := map[golibevdev.KeyEventCode]string{
		golibevdev.KeyEsc: "esc",
		golibevdev.KeyA:   "a",
		golibevdev.KeyB:   "b",
		golibevdev.KeyC:   "c",
		golibevdev.KeyD:   "d",
		golibevdev.KeyE:   "e",
		golibevdev.KeyF:   "f",
		golibevdev.KeyG:   "g",
		golibevdev.KeyH:   "h",
		golibevdev.KeyI:   "i",
		golibevdev.KeyJ:   "j",
		golibevdev.KeyK:   "k",
		golibevdev.KeyL:   "l",
		golibevdev.KeyM:   "m",
		golibevdev.KeyN:   "n",
		golibevdev.KeyO:   "o",
		golibevdev.KeyP:   "p",
		golibevdev.KeyQ:   "q",
		golibevdev.KeyR:   "r",
		golibevdev.KeyS:   "s",
		golibevdev.KeyT:   "t",
		golibevdev.KeyU:   "u",
		golibevdev.KeyV:   "v",
		golibevdev.KeyW:   "w",
		golibevdev.KeyX:   "x",
		golibevdev.KeyY:   "y",
		golibevdev.KeyZ:   "z",
		// Add more keys as needed
	}

	if name, ok := keyMap[code]; ok {
		return name
	}
	return code.String()
}
