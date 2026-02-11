package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jialeicui/keyswift/pkg/config"
	"github.com/jialeicui/keyswift/pkg/evdev"
	"github.com/jialeicui/keyswift/pkg/statemachine"
	"github.com/jialeicui/keyswift/pkg/utils"
	"github.com/jialeicui/keyswift/pkg/wininfo/dbus"
)

var (
	flagKeyboards        = flag.String("keyboards", "HHKB", "Comma-separated list of keyboard device name substrings")
	flagConfig           = flag.String("config", "", "Configuration file path (defaults to $XDG_CONFIG_HOME/keyswift/config.json)")
	flagVerbose          = flag.Bool("verbose", false, "Enable verbose logging")
	flagOutputDeviceName = flag.String("output-device-name", "keyswift", "Name of the virtual keyboard device")
	flagVersion          = flag.Bool("version", false, "Print version information and exit")
)

// These variables are injected at compile time
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	flag.Parse()

	if *flagVersion {
		fmt.Printf("keyswift version %s (commit: %s)\n", version, commit)
		os.Exit(0)
	}

	// Configure logging
	logLevel := slog.LevelInfo
	if *flagVerbose {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	// Load configuration
	configPath := *flagConfig
	if configPath == "" {
		configPath = utils.DefaultConfigPath()
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err, "path", configPath)
		os.Exit(1)
	}
	slog.Info("Configuration loaded", "mappings", len(cfg.Mappings), "vars", len(cfg.Vars))

	// Initialize window info service
	windowMonitor, err := dbus.New()
	if err != nil {
		slog.Error("Failed to initialize window monitor", "error", err)
		os.Exit(1)
	}
	defer windowMonitor.Close()
	slog.Info("Window Monitor service is running...")

	// Initialize virtual keyboard for output
	out, err := statemachine.NewRecoveringOutputDevice(*flagOutputDeviceName)
	if err != nil {
		slog.Error("Failed to create virtual keyboard", "error", err)
		os.Exit(1)
	}

	// Find input devices
	devs, err := evdev.NewOverviewImpl().ListInputDevices()
	if err != nil {
		slog.Error("Failed to list input devices", "error", err)
		os.Exit(1)
	}

	// Parse keyboard patterns and find matching devices
	keyboardPatterns := strings.Split(*flagKeyboards, ",")
	matchedDevices := findMatchingDevices(devs, keyboardPatterns)

	if len(matchedDevices) == 0 {
		slog.Info("Available keyboards:")
		for _, d := range devs {
			slog.Info("  - ", d.Name, " (", d.Path, ")")
		}
		slog.Error("No keyboards matching patterns", "patterns", *flagKeyboards)
		os.Exit(1)
	}

	// Initialize handler with state machine
	handler := statemachine.NewHandlerWithStateMachine(cfg)
	defer handler.Close()

	for _, d := range matchedDevices {
		slog.Info("Using keyboard", "name", d.Name, "path", d.Path)
		if err := handler.AddDevice(d.Name, d.Path); err != nil {
			slog.Warn("Failed to add device", "device", d.Name, "error", err)
			continue
		}
	}

	// Setup signal handling with context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("Shutting down...")
		cancel()
		handler.Close()
	}()

	// Start processing events
	slog.Info(fmt.Sprintf("Processing events from %d devices... Press Ctrl+C to exit", len(matchedDevices)))
	handler.ProcessEvents(out, windowMonitor)

	// Wait for context cancellation or processing to complete
	<-ctx.Done()
	slog.Info("Shutdown complete")

	handler.Wait()
}

// findMatchingDevices filters input devices by pattern, removing duplicates
func findMatchingDevices(devs []*evdev.InputDevice, patterns []string) []*evdev.InputDevice {
	seen := make(map[string]bool)
	var matched []*evdev.InputDevice

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		for _, dev := range devs {
			if strings.Contains(dev.Name, pattern) && dev.Name != *flagOutputDeviceName && !seen[dev.Path] {
				seen[dev.Path] = true
				matched = append(matched, dev)
			}
		}
	}

	return matched
}
