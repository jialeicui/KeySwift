package conf

// SourceEvent represents different types of events that can trigger actions
type SourceEvent struct {
	// Logical operation to combine events
	Operation string `yaml:"operation,omitempty"`
	// Different types of events
	KeyPressEvent    *KeyPressEvent    `yaml:"key_press_event,omitempty"`
	MouseClickEvent  *MouseClickEvent  `yaml:"mouse_click_event,omitempty"`
	DBusEvent        *DBusEvent        `yaml:"dbus_event,omitempty"`
	WindowFocusEvent *WindowFocusEvent `yaml:"window_focus_event,omitempty"`
}

// KeyPressEvent represents a keyboard key press
type KeyPressEvent struct {
	Key string `yaml:"key"`
}

// MouseClickEvent represents a mouse click at specific coordinates
type MouseClickEvent struct {
	X float64 `yaml:"x"`
	Y float64 `yaml:"y"`
}

// DBusEvent represents a DBus event
type DBusEvent struct {
	Table  string `yaml:"table"`
	Action string `yaml:"action"`
}

// WindowFocusEvent represents a window focus change
type WindowFocusEvent struct {
	WindowClass []string `yaml:"window_class"`
}
