// Package statemachine provides a robust state machine architecture for key remapping.
// It solves the sticky key problem through layered state machines, explicit state binding,
// and speculative execution.
package statemachine

import (
	"time"

	"github.com/jialeicui/golibevdev"
)

// KeyCode is an alias for golibevdev key event codes
type KeyCode = golibevdev.KeyEventCode

// DeviceID identifies an input device
type DeviceID string

// MappingID identifies a key mapping configuration
type MappingID string

// BindingID identifies a state binding
type BindingID string

// OpID identifies a speculative operation
type OpID string

// KeyAction represents a key press or release action
type KeyAction int

const (
	KeyPress KeyAction = iota
	KeyRelease
)

// KeyEvent represents a raw key event from input devices
type KeyEvent struct {
	Key       KeyCode
	Action    KeyAction
	Timestamp time.Time
	Device    DeviceID
}

// OutputCommand represents a command to be sent to the output device
type OutputCommand struct {
	Key    KeyCode
	Action KeyAction
}

// InputState represents the current state of all physical keys
type InputState struct {
	Timestamp time.Time
	KeyStates map[KeyCode]KeyState
	// EventSeq preserves the order of raw events
	EventSeq []KeyEvent
}

// KeyState tracks the state of a single key
type KeyState struct {
	Pressed    bool
	PressedAt  time.Time
	Device     DeviceID
	PressCount int // For debounce detection
}

// Clone creates a deep copy of InputState
func (is InputState) Clone() InputState {
	cloned := InputState{
		Timestamp: is.Timestamp,
		KeyStates: make(map[KeyCode]KeyState, len(is.KeyStates)),
		EventSeq:  make([]KeyEvent, len(is.EventSeq)),
	}
	for k, v := range is.KeyStates {
		cloned.KeyStates[k] = v
	}
	copy(cloned.EventSeq, is.EventSeq)
	return cloned
}

// SemanticState represents the semantic interpretation of input
type SemanticState struct {
	Modifiers    ModifierSemantic
	Independents map[KeyCode]IndependentState
	Combo        ComboState
}

// ModifierSemantic tracks modifier keys and their semantic role
type ModifierSemantic struct {
	Active  map[KeyCode]ModifierInfo
	Pending map[KeyCode]*PendingModifier
}

// ModifierInfo tracks information about an active modifier
type ModifierInfo struct {
	Key       KeyCode
	PressedAt time.Time
}

// PendingModifier represents a modifier waiting to determine its semantic role
type PendingModifier struct {
	Key               KeyCode
	PressedAt         time.Time
	DowngradeDeadline time.Time
	PassedThrough     bool
}

// IndependentState represents a key acting independently (not as a modifier)
type IndependentState struct {
	Key            KeyCode
	IsPressed      bool
	DowngradedFrom bool // Whether converted from Pending state
}

// ComboState tracks active key combinations
type ComboState struct {
	ActiveKeys []KeyCode
}

// OutputState represents the desired or actual output state
type OutputState struct {
	PressedKeys map[KeyCode]OutputKeyInfo
}

// Clone creates a deep copy of OutputState
func (os OutputState) Clone() OutputState {
	cloned := OutputState{
		PressedKeys: make(map[KeyCode]OutputKeyInfo, len(os.PressedKeys)),
	}
	for k, v := range os.PressedKeys {
		cloned.PressedKeys[k] = v
	}
	return cloned
}

// OutputKeyInfo tracks metadata about a pressed output key
type OutputKeyInfo struct {
	PressedAt       time.Time
	SourceMapping   MappingID
	BoundTo         []KeyCode
	ReleaseStrategy ReleaseStrategy
}

// ReleaseStrategy determines when an output key should be released
type ReleaseStrategy int

const (
	// ReleaseOnAnyBoundKeyReleased releases when any bound input key is released
	ReleaseOnAnyBoundKeyReleased ReleaseStrategy = iota
	// ReleaseOnAllBoundKeysReleased releases only when all bound input keys are released
	ReleaseOnAllBoundKeysReleased
	// ReleaseOnTimeout releases after a maximum hold time
	ReleaseOnTimeout
)

// Binding represents a relationship between input and output keys
type Binding struct {
	ID         BindingID
	InputKeys  []KeyCode
	OutputKeys []KeyCode
	CreatedAt  time.Time
	Status     BindingStatus
}

// BindingStatus represents the current status of a binding
type BindingStatus int

const (
	BindingActive BindingStatus = iota
	BindingReleasing
	BindingReleased
)

// SpeculativeOp represents a speculatively executed operation
type SpeculativeOp struct {
	ID              OpID
	Key             KeyCode
	Action          KeyAction
	ExecutedAt      time.Time
	ConfirmDeadline time.Time
	Status          SpeculativeStatus
	Compensation    CompensationStrategy
}

// SpeculativeStatus represents the status of a speculative operation
type SpeculativeStatus int

const (
	SpeculativePending SpeculativeStatus = iota
	SpeculativeConfirmed
	SpeculativeCompensated
)

// CompensationStrategy defines how to compensate for a speculative operation
type CompensationStrategy struct {
	RevertActions    []OutputCommand
	AlignmentActions []OutputCommand
}

// StateDiff represents the difference between desired and actual states
type StateDiff struct {
	ShouldPress   []KeyCode
	ShouldRelease []KeyCode
}

// IsModifier returns true if the key is a modifier key
func IsModifier(key KeyCode) bool {
	switch key {
	case golibevdev.KeyLeftCtrl, golibevdev.KeyRightCtrl,
		golibevdev.KeyLeftAlt, golibevdev.KeyRightAlt,
		golibevdev.KeyLeftShift, golibevdev.KeyRightShift,
		golibevdev.KeyLeftMeta, golibevdev.KeyRightMeta:
		return true
	}
	return false
}

// Config holds configuration for the state machine
type Config struct {
	Semantic    SemanticConfig
	Speculative SpeculativeConfig
	Alignment   AlignmentConfig
}

// SemanticConfig configures the semantic state machine
type SemanticConfig struct {
	ModifierPendingThreshold time.Duration
	AllowDowngrade           bool
	DowngradeWindow          time.Duration
	DebounceThreshold        time.Duration
}

// DefaultSemanticConfig returns the default semantic configuration
func DefaultSemanticConfig() SemanticConfig {
	return SemanticConfig{
		ModifierPendingThreshold: 200 * time.Millisecond,
		AllowDowngrade:           true,
		DowngradeWindow:          100 * time.Millisecond,
		DebounceThreshold:        5 * time.Millisecond,
	}
}

// SpeculativeConfig configures speculative execution
type SpeculativeConfig struct {
	Enabled            bool
	ConfirmationWindow time.Duration
	CompensationMode   CompensationMode
}

// CompensationMode determines how to compensate for speculative operations
type CompensationMode int

const (
	CompensationImmediate CompensationMode = iota
	CompensationDeferred
	CompensationSmart
)

// DefaultSpeculativeConfig returns the default speculative configuration
func DefaultSpeculativeConfig() SpeculativeConfig {
	return SpeculativeConfig{
		Enabled:            true,
		ConfirmationWindow: 50 * time.Millisecond,
		CompensationMode:   CompensationSmart,
	}
}

// AlignmentConfig configures state alignment
type AlignmentConfig struct {
	SyncInterval     time.Duration
	EmergencyTimeout time.Duration
}

// DefaultAlignmentConfig returns the default alignment configuration
func DefaultAlignmentConfig() AlignmentConfig {
	return AlignmentConfig{
		SyncInterval:     100 * time.Millisecond,
		EmergencyTimeout: 5 * time.Second,
	}
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		Semantic:    DefaultSemanticConfig(),
		Speculative: DefaultSpeculativeConfig(),
		Alignment:   DefaultAlignmentConfig(),
	}
}
