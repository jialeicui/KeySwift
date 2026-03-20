package statemachine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jialeicui/golibevdev"
	"github.com/jialeicui/keyswift/pkg/config"
	"github.com/jialeicui/keyswift/pkg/evdev"
	"github.com/jialeicui/keyswift/pkg/wininfo"
)

const (
	initialReconnectDelay = 500 * time.Millisecond
	maxReconnectDelay     = 5 * time.Second
)

// HandlerWithStateMachine manages input devices and processes events through the state machine.
type HandlerWithStateMachine struct {
	devices   []*handlerDevice
	devicesMu sync.RWMutex
	wg        sync.WaitGroup
	out       OutputDevice
	closeOnce sync.Once

	stateMachine *ConfigStateMachine
	overview     evdev.Overview

	ctx    context.Context
	cancel context.CancelFunc
	cfg    *config.Config
}

// handlerDevice represents a managed input device that can reconnect.
type handlerDevice struct {
	mu     sync.RWMutex
	id     DeviceID
	device *golibevdev.InputDev
	name   string
	path   string
	online bool
}

// NewHandlerWithStateMachine creates a new handler with state machine support.
func NewHandlerWithStateMachine(cfg *config.Config) *HandlerWithStateMachine {
	ctx, cancel := context.WithCancel(context.Background())
	return &HandlerWithStateMachine{
		devices:  make([]*handlerDevice, 0),
		ctx:      ctx,
		cancel:   cancel,
		cfg:      cfg,
		overview: evdev.NewOverviewImpl(),
	}
}

// AddDevice adds and grabs a new input device.
func (h *HandlerWithStateMachine) AddDevice(name, path string) error {
	dev, err := openGrabbedInputDevice(path)
	if err != nil {
		return err
	}

	h.devicesMu.Lock()
	defer h.devicesMu.Unlock()

	deviceID := DeviceID(fmt.Sprintf("%s#%d", name, len(h.devices)+1))
	h.devices = append(h.devices, &handlerDevice{
		id:     deviceID,
		device: dev,
		name:   name,
		path:   path,
		online: true,
	})
	return nil
}

// ProcessEvents starts processing events from all devices.
func (h *HandlerWithStateMachine) ProcessEvents(output OutputDevice, windowMonitor wininfo.WinGetter) {
	h.out = output

	if h.cfg == nil {
		slog.Error("State machine requires configuration but none provided")
		return
	}

	var windowClassGetter func() string
	if windowMonitor != nil {
		windowClassGetter = func() string {
			info, _ := windowMonitor.GetActiveWindow()
			if info != nil {
				return info.Class
			}
			return ""
		}
	} else {
		windowClassGetter = func() string { return "" }
	}

	h.stateMachine = NewConfigStateMachine(
		output,
		h.cfg,
		windowClassGetter,
		DefaultConfig(),
	)

	if err := h.stateMachine.Start(h.ctx); err != nil {
		slog.Error("Failed to start state machine", "error", err)
		return
	}

	slog.Info("State machine enabled for event processing", "mappings", len(h.cfg.Mappings))

	h.devicesMu.RLock()
	devices := append([]*handlerDevice(nil), h.devices...)
	h.devicesMu.RUnlock()

	for _, dev := range devices {
		h.wg.Add(1)
		go func(d *handlerDevice) {
			defer h.wg.Done()
			h.deviceSupervisor(d)
		}(dev)
	}
}

func (h *HandlerWithStateMachine) deviceSupervisor(dev *handlerDevice) {
	slog.Info("Starting event processing for device", "device", dev.name, "path", dev.currentPath())

	for {
		err := h.processDeviceEvents(dev)
		if err == nil {
			return
		}

		select {
		case <-h.ctx.Done():
			return
		default:
		}

		slog.Warn("Input device disconnected, attempting recovery",
			"device", dev.name,
			"path", dev.currentPath(),
			"error", err)

		if h.stateMachine != nil {
			if releaseErr := h.stateMachine.HandleDeviceLost(dev.id); releaseErr != nil {
				slog.Error("Failed to release lost device state", "device", dev.name, "error", releaseErr)
			}
		}

		dev.closeCurrent()

		if err := h.waitForReconnect(dev); err != nil {
			if h.ctx.Err() == nil {
				slog.Error("Stopped reconnecting device", "device", dev.name, "error", err)
			}
			return
		}

		slog.Info("Input device recovered", "device", dev.name, "path", dev.currentPath())
	}
}

// processDeviceEvents processes events using the state machine until the device fails.
func (h *HandlerWithStateMachine) processDeviceEvents(dev *handlerDevice) error {
	for {
		select {
		case <-h.ctx.Done():
			return nil
		default:
		}

		input := dev.currentDevice()
		if input == nil {
			return fmt.Errorf("device handle is not available")
		}

		ev, err := input.NextEvent(golibevdev.ReadFlagNormal)
		if err != nil {
			select {
			case <-h.ctx.Done():
				return nil
			default:
			}
			return err
		}

		if ev.Type == golibevdev.EvSyn || ev.Type != golibevdev.EvKey {
			continue
		}

		keyCode := ev.Code.(golibevdev.KeyEventCode)
		pressed := ev.Value == 1
		released := ev.Value == 0

		if !pressed && !released {
			slog.Debug("Skipping key event with value", "value", ev.Value)
			continue
		}

		slog.Debug("Received key event", "key", keyCode, "pressed", pressed, "device", dev.name)

		if err := h.stateMachine.ProcessEvent(dev.id, keyCode, pressed); err != nil {
			slog.Error("Failed to process event through state machine",
				"device", dev.name,
				"key", keyCode,
				"error", err)
		}
	}
}

func (h *HandlerWithStateMachine) waitForReconnect(dev *handlerDevice) error {
	delay := initialReconnectDelay

	for {
		if err := h.tryReconnect(dev); err == nil {
			return nil
		} else {
			slog.Warn("Reconnect attempt failed",
				"device", dev.name,
				"path", dev.currentPath(),
				"retryIn", delay,
				"error", err)
		}

		select {
		case <-h.ctx.Done():
			return h.ctx.Err()
		case <-time.After(delay):
		}

		if delay < maxReconnectDelay {
			delay *= 2
			if delay > maxReconnectDelay {
				delay = maxReconnectDelay
			}
		}
	}
}

func (h *HandlerWithStateMachine) tryReconnect(dev *handlerDevice) error {
	currentPath := dev.currentPath()
	if currentPath != "" {
		input, err := openGrabbedInputDevice(currentPath)
		if err == nil {
			dev.setConnection(currentPath, input)
			return nil
		}
	}

	devices, err := h.overview.ListInputDevices()
	if err != nil {
		return fmt.Errorf("list input devices: %w", err)
	}

	candidate := evdev.FindDeviceByName(devices, dev.name, h.activePaths(dev.id))
	if candidate == nil {
		return fmt.Errorf("device %q not found during rescan", dev.name)
	}

	input, err := openGrabbedInputDevice(candidate.Path)
	if err != nil {
		return fmt.Errorf("reopen device %s: %w", candidate.Path, err)
	}

	dev.setConnection(candidate.Path, input)
	return nil
}

func (h *HandlerWithStateMachine) activePaths(exclude DeviceID) map[string]struct{} {
	h.devicesMu.RLock()
	defer h.devicesMu.RUnlock()

	paths := make(map[string]struct{})
	for _, dev := range h.devices {
		if dev.id == exclude {
			continue
		}
		path, online := dev.currentPathState()
		if online && path != "" {
			paths[path] = struct{}{}
		}
	}
	return paths
}

// Wait waits for all event processing to complete.
func (h *HandlerWithStateMachine) Wait() {
	h.wg.Wait()
}

// Close closes all input devices and stops the state machine.
func (h *HandlerWithStateMachine) Close() {
	h.closeOnce.Do(func() {
		h.cancel()

		if h.stateMachine != nil {
			if err := h.stateMachine.Stop(); err != nil {
				slog.Error("Error stopping state machine", "error", err)
			}
		}

		h.devicesMu.RLock()
		devices := append([]*handlerDevice(nil), h.devices...)
		h.devicesMu.RUnlock()

		for _, dev := range devices {
			slog.Info("Closing device", "device", dev.name)
			dev.closeCurrent()
		}

		h.wg.Wait()

		if h.out != nil {
			if err := h.out.Close(); err != nil {
				slog.Error("Failed to close output device", "error", err)
			}
		}
	})
}

// EmergencyRelease forces release of all keys.
func (h *HandlerWithStateMachine) EmergencyRelease() {
	if h.stateMachine != nil {
		h.stateMachine.EmergencyRelease()
	}
}

func openGrabbedInputDevice(path string) (*golibevdev.InputDev, error) {
	dev, err := golibevdev.NewInputDev(path)
	if err != nil {
		return nil, err
	}

	if err := dev.Grab(); err != nil {
		dev.Close()
		return nil, err
	}

	return dev, nil
}

func (d *handlerDevice) currentDevice() *golibevdev.InputDev {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.device
}

func (d *handlerDevice) currentPath() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.path
}

func (d *handlerDevice) currentPathState() (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.path, d.online
}

func (d *handlerDevice) setConnection(path string, device *golibevdev.InputDev) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.path = path
	d.device = device
	d.online = true
}

func (d *handlerDevice) closeCurrent() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.device != nil {
		d.device.Close()
		d.device = nil
	}
	d.online = false
}
