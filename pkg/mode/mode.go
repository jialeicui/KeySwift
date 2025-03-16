package mode

import (
	"github.com/jialeicui/keyswift/pkg/wininfo"
)

// Mode represents a key remapping mode/layer
type Mode interface {
	// Name returns the name of the mode
	Name() string
	// ProcessEvent processes an event and returns actions to execute
	// Returns nil if no action should be taken
}

// Event represents any type of event that can trigger an action
type Event struct {
	// KeyPress represents a keyboard key press event
	KeyPress *KeyPressEvent
	// MouseClick represents a mouse click event
	MouseClick *MouseClickEvent
	// WindowFocus represents a window focus change event
	WindowFocus *WindowFocusEvent
}

// KeyPressEvent represents a keyboard key press
type KeyPressEvent struct {
	Keys     []string
	Pressed  bool // true for press, false for release
	Repeated bool // true if key repeat
}

// MouseClickEvent represents a mouse click
type MouseClickEvent struct {
	X       float64
	Y       float64
	Button  int
	Pressed bool // true for press, false for release
}

// WindowFocusEvent represents a window focus change
type WindowFocusEvent struct {
	Window *wininfo.WinInfo
}

// ModeTransition represents a mode transition
type ModeTransition struct {
	// Mode to transition to, empty string means pop current mode
	Mode string
	// Push new mode or replace current mode
	Push bool
}
