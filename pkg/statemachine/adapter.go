package statemachine

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jialeicui/golibevdev"
	"github.com/jialeicui/keyswift/pkg/keys"
)

// ErrOutputResetRequired signals that the output device was rebuilt and the
// state machine must discard its tracked state.
var ErrOutputResetRequired = errors.New("output device reset required")

type lowLevelOutputDevice interface {
	WriteKey(key KeyCode, value int32) error
	Sync() error
	Close() error
}

type outputDeviceFactory func(name string) (lowLevelOutputDevice, error)

type uinputLowLevelDevice struct {
	device *golibevdev.UInputDev
}

func (d *uinputLowLevelDevice) WriteKey(key KeyCode, value int32) error {
	return d.device.WriteEvent(golibevdev.EvKey, key, value)
}

func (d *uinputLowLevelDevice) Sync() error {
	return d.device.WriteEvent(golibevdev.EvSyn, golibevdev.SynReport, 0)
}

func (d *uinputLowLevelDevice) Close() error {
	d.device.Close()
	return nil
}

func newDefaultOutputDeviceFactory(name string) (lowLevelOutputDevice, error) {
	device, err := golibevdev.NewVirtualKeyboard(name)
	if err != nil {
		return nil, err
	}
	return &uinputLowLevelDevice{device: device}, nil
}

// RecoveringOutputDevice rebuilds the virtual keyboard after write failures.
type RecoveringOutputDevice struct {
	mu      sync.Mutex
	name    string
	factory outputDeviceFactory
	device  lowLevelOutputDevice
}

// NewRecoveringOutputDevice creates an output device with automatic recovery.
func NewRecoveringOutputDevice(name string) (*RecoveringOutputDevice, error) {
	return newRecoveringOutputDevice(name, newDefaultOutputDeviceFactory)
}

func newRecoveringOutputDevice(name string, factory outputDeviceFactory) (*RecoveringOutputDevice, error) {
	device, err := factory(name)
	if err != nil {
		return nil, fmt.Errorf("create virtual keyboard: %w", err)
	}

	return &RecoveringOutputDevice{
		name:    name,
		factory: factory,
		device:  device,
	}, nil
}

// Execute implements OutputDevice.
func (d *RecoveringOutputDevice) Execute(cmd OutputCommand) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.device == nil {
		return d.recoverLocked(fmt.Errorf("output device is not available"))
	}

	value := int32(0)
	if cmd.Action == KeyPress {
		value = 1
	}

	if err := d.device.WriteKey(cmd.Key, value); err != nil {
		return d.recoverLocked(fmt.Errorf("write key event: %w", err))
	}
	if err := d.device.Sync(); err != nil {
		return d.recoverLocked(fmt.Errorf("write sync event: %w", err))
	}

	return nil
}

// Sync implements OutputDevice.
func (d *RecoveringOutputDevice) Sync() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.device == nil {
		return d.recoverLocked(fmt.Errorf("output device is not available"))
	}

	if err := d.device.Sync(); err != nil {
		return d.recoverLocked(fmt.Errorf("sync output device: %w", err))
	}

	return nil
}

// Close implements OutputDevice.
func (d *RecoveringOutputDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.device == nil {
		return nil
	}

	err := d.device.Close()
	d.device = nil
	return err
}

func (d *RecoveringOutputDevice) recoverLocked(cause error) error {
	slog.Warn("Output device write failed, rebuilding virtual keyboard", "name", d.name, "error", cause)

	if d.device != nil {
		if err := d.device.Close(); err != nil {
			slog.Warn("Failed to close broken output device", "name", d.name, "error", err)
		}
		d.device = nil
	}

	device, err := d.factory(d.name)
	if err != nil {
		return fmt.Errorf("%w: recover virtual keyboard: %v (cause: %v)", ErrOutputResetRequired, err, cause)
	}

	d.device = device
	slog.Info("Recovered virtual keyboard device", "name", d.name)

	return fmt.Errorf("%w: %v", ErrOutputResetRequired, cause)
}

// SimpleMappingEngine is a simple implementation of MappingEngine for testing
type SimpleMappingEngine struct {
	mappings []MappingRule
}

// MappingRule defines a single key mapping rule
type MappingRule struct {
	// Input pattern (e.g., [Cmd, C])
	Input []keys.Key
	// Output pattern (e.g., [Ctrl, C])
	Output []keys.Key
}

// NewSimpleMappingEngine creates a new simple mapping engine
func NewSimpleMappingEngine() *SimpleMappingEngine {
	return &SimpleMappingEngine{
		mappings: make([]MappingRule, 0),
	}
}

// AddMapping adds a mapping rule
func (e *SimpleMappingEngine) AddMapping(rule MappingRule) {
	e.mappings = append(e.mappings, rule)
}

// Match implements MappingEngine
func (e *SimpleMappingEngine) Match(state SemanticState) ([]OutputCommand, bool) {
	// Build input set from semantic state
	inputKeys := make([]KeyCode, 0)

	// Add active modifiers
	for key := range state.Modifiers.Active {
		inputKeys = append(inputKeys, key)
	}

	// Add pending modifiers
	for key := range state.Modifiers.Pending {
		inputKeys = append(inputKeys, key)
	}

	// Add combo keys
	inputKeys = append(inputKeys, state.Combo.ActiveKeys...)

	// Check each mapping
	for _, mapping := range e.mappings {
		if e.matches(inputKeys, mapping.Input) {
			commands := make([]OutputCommand, len(mapping.Output))
			for i, key := range mapping.Output {
				commands[i] = OutputCommand{
					Key:    key,
					Action: KeyPress,
				}
			}
			return commands, true
		}
	}

	return nil, false
}

// PrefixMatch implements MappingEngine
func (e *SimpleMappingEngine) PrefixMatch(keys []KeyCode) []MappingID {
	var matches []MappingID

	for i, mapping := range e.mappings {
		if e.isPrefix(keys, mapping.Input) {
			matches = append(matches, MappingID(fmt.Sprintf("mapping-%d", i)))
		}
	}

	return matches
}

// matches checks if input matches the pattern (set equality)
func (e *SimpleMappingEngine) matches(input, pattern []KeyCode) bool {
	if len(input) != len(pattern) {
		return false
	}

	inputMap := make(map[KeyCode]bool)
	for _, key := range input {
		inputMap[key] = true
	}

	for _, key := range pattern {
		if !inputMap[key] {
			return false
		}
	}

	return true
}

// isPrefix checks if keys is a prefix of pattern
func (e *SimpleMappingEngine) isPrefix(keys, pattern []KeyCode) bool {
	if len(keys) > len(pattern) {
		return false
	}

	for i, key := range keys {
		if key != pattern[i] {
			return false
		}
	}

	return true
}

// LoggingOutputDevice wraps an OutputDevice with logging
type LoggingOutputDevice struct {
	wrapped OutputDevice
	logger  *slog.Logger
}

// NewLoggingOutputDevice creates a new logging output device
func NewLoggingOutputDevice(wrapped OutputDevice) *LoggingOutputDevice {
	return &LoggingOutputDevice{
		wrapped: wrapped,
		logger:  slog.Default(),
	}
}

// Execute implements OutputDevice with logging
func (d *LoggingOutputDevice) Execute(cmd OutputCommand) error {
	action := "release"
	if cmd.Action == KeyPress {
		action = "press"
	}
	d.logger.Debug("Output command", "key", cmd.Key, "action", action)
	return d.wrapped.Execute(cmd)
}

// Sync implements OutputDevice with logging
func (d *LoggingOutputDevice) Sync() error {
	d.logger.Debug("Sync output device")
	return d.wrapped.Sync()
}

// Close implements OutputDevice with logging.
func (d *LoggingOutputDevice) Close() error {
	d.logger.Debug("Close output device")
	return d.wrapped.Close()
}
