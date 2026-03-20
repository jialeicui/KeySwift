package statemachine

import (
	"context"
	"log/slog"
	"time"

	"github.com/jialeicui/keyswift/pkg/config"
)

// ConfigStateMachine wraps the state machine with config-based mapping
type ConfigStateMachine struct {
	machine           *Machine
	engine            *ConfigMappingEngine
	output            OutputDevice
	config            Config
	windowClass       string
	windowClassGetter func() string
}

// NewConfigStateMachine creates a new state machine from configuration
func NewConfigStateMachine(
	output OutputDevice,
	cfg *config.Config,
	windowClassGetter func() string,
	machineConfig Config,
) *ConfigStateMachine {
	engine := NewConfigMappingEngine(cfg)

	machine := NewMachine(engine, output, machineConfig)

	return &ConfigStateMachine{
		machine:           machine,
		engine:            engine,
		output:            output,
		config:            machineConfig,
		windowClassGetter: windowClassGetter,
	}
}

// Start starts the state machine
func (c *ConfigStateMachine) Start(ctx context.Context) error {
	return c.machine.Start(ctx)
}

// Stop stops the state machine
func (c *ConfigStateMachine) Stop() error {
	return c.machine.Stop()
}

// ProcessEvent processes a key event
func (c *ConfigStateMachine) ProcessEvent(device DeviceID, key KeyCode, pressed bool) error {
	// Update window class before processing each event
	if c.windowClassGetter != nil {
		newClass := c.windowClassGetter()
		if newClass != c.windowClass {
			c.SetWindowClass(newClass)
		}
	}

	action := KeyRelease
	if pressed {
		action = KeyPress
	}

	event := KeyEvent{
		Key:       key,
		Action:    action,
		Timestamp: time.Now(),
		Device:    device,
	}

	return c.machine.ProcessEvent(event)
}

// SetWindowClass updates the current window class
func (c *ConfigStateMachine) SetWindowClass(class string) {
	if c.windowClass != class {
		slog.Info("State machine window class changed", "from", c.windowClass, "to", class)
	}
	c.windowClass = class
	c.engine.SetWindowClass(class)
}

// EmergencyRelease forces release of all output keys
func (c *ConfigStateMachine) EmergencyRelease() []KeyCode {
	return c.machine.EmergencyRelease()
}

// HandleDeviceLost releases any keys still associated with a device that went away.
func (c *ConfigStateMachine) HandleDeviceLost(device DeviceID) error {
	return c.machine.HandleDeviceLost(device)
}
