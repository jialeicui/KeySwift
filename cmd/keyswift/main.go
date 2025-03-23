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

	"github.com/jialeicui/keyswift/pkg/bus"
	"github.com/jialeicui/keyswift/pkg/evdev"
	"github.com/jialeicui/keyswift/pkg/handler"
	"github.com/jialeicui/keyswift/pkg/utils"
	"github.com/jialeicui/keyswift/pkg/wininfo/dbus"
)

var (
	flagKeyboards        = flag.String("keyboards", "HHKB", "Comma-separated list of keyboard device name substrings")
	flagConfig           = flag.String("config", "", "Configuration file path (defaults to $XDG_CONFIG_HOME/keyswift/config.js)")
	flagVerbose          = flag.Bool("verbose", false, "Enable verbose logging")
	flagOutputDeviceName = flag.String("output-device-name", "keyswift", "Name of the virtual keyboard device")
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
		configPath = utils.DefaultConfigPath()
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
	out, err := golibevdev.NewVirtualKeyboard(*flagOutputDeviceName)
	if err != nil {
		slog.Error("Failed to create virtual keyboard", "error", err)
		os.Exit(1)
	}
	defer out.Close()

	script, err := os.ReadFile(configPath)
	if err != nil {
		slog.Error("Failed to read configuration file", "error", err)
		os.Exit(1)
	}

	// Initialize bus manager
	busMgr, err := bus.New(string(script), windowMonitor, out)
	if err != nil {
		slog.Error("Failed to initialize bus manager", "error", err)
		os.Exit(1)
	}
	slog.Info("bus manager initialized")

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
			return strings.Contains(item.Name, pattern) && item.Name != *flagOutputDeviceName
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
	deviceManager := handler.New()
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
	slog.Info(fmt.Sprintf("Processing events from %d devices... Press Ctrl+C to exit", len(deviceManager.GetDevices())))
	deviceManager.ProcessEvents(out, busMgr)

	// Wait for all processing to complete (typically won't reach here except on error)
	deviceManager.Wait()
}
