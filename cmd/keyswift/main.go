package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jialeicui/golibevdev"
	"github.com/samber/lo"

	"github.com/jialeicui/keyswift/pkg/conf"
	"github.com/jialeicui/keyswift/pkg/dev"
	"github.com/jialeicui/keyswift/pkg/evdev"
	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/jialeicui/keyswift/pkg/mode"
	"github.com/jialeicui/keyswift/pkg/wininfo/dbus"
)

var (
	flagKeyboards = flag.String("keyboards", "Apple", "Comma-separated list of keyboard device name substrings")
	flagConfig    = flag.String("config", "", "Configuration file path (defaults to $XDG_CONFIG_HOME/keyswift/config.js)")
	// TODO change this to false
	flagVerbose = flag.Bool("verbose", true, "Enable verbose logging")
)

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

	// Initialize window info service
	windowMonitor, err := dbus.New()
	if err != nil {
		slog.Error("Failed to initialize window monitor", "error", err)
		os.Exit(1)
	}
	defer windowMonitor.Close()
	slog.Info("Window Monitor service is running...")

	// Initialize virtual keyboard for output
	out, err := golibevdev.NewVirtualKeyboard("keyswift")
	if err != nil {
		slog.Error("Failed to create virtual keyboard", "error", err)
		os.Exit(1)
	}
	defer out.Close()
	gnomeKeys := keys.NewGnomeKeys(out)

	script, err := os.ReadFile(configPath)
	if err != nil {
		slog.Error("Failed to read configuration file", "error", err)
		os.Exit(1)
	}

	// Initialize mode manager
	modeManager, err := mode.NewManager(string(script), gnomeKeys, windowMonitor)
	if err != nil {
		slog.Error("Failed to initialize mode manager", "error", err)
		os.Exit(1)
	}
	slog.Info("Mode manager initialized")

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
		slog.Info("Available keyboards:")
		for _, d := range devs {
			slog.Info("  - ", d.Name, " (", d.Path, ")")
		}
		slog.Error("No keyboards matching patterns", "patterns", *flagKeyboards)
		os.Exit(1)
	}

	// Initialize and set up the device manager
	deviceManager := dev.NewInputDeviceManager()
	defer deviceManager.Close()

	// Add all matched devices to the manager
	for _, d := range matchedDevices {
		slog.Info("Using keyboard: ", d.Name, d.Path)
		if err := deviceManager.AddDevice(d.Name, d.Path); err != nil {
			slog.Warn("Failed to add device", "device", d.Name, "error", err)
			continue
		}
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("Shutting down...")
		deviceManager.Close()
		out.Close()
		windowMonitor.Close()
		os.Exit(0)
	}()

	// Start processing events from all devices
	slog.Info(fmt.Sprintf("Processing events from %d devices... Press Ctrl+C to exit\n", len(deviceManager.GetDevices())))
	deviceManager.ProcessEvents(out, modeManager)

	// Wait for all processing to complete (typically won't reach here except on error)
	deviceManager.Wait()
}
