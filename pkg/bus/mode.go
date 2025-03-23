package bus

import (
	"github.com/jialeicui/golibevdev"

	"github.com/jialeicui/keyswift/pkg/wininfo"
)

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
	Keys     []golibevdev.KeyEventCode
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
