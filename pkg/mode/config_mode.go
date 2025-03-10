package mode

import (
	"log/slog"
	"slices"

	"github.com/jialeicui/keyswift/pkg/conf"
)

// ConfigMode implements Mode using configuration
type ConfigMode struct {
	name    string
	actions conf.ModeAction
}

// NewConfigMode creates a new config-based mode
func NewConfigMode(name string, actions conf.ModeAction) *ConfigMode {
	return &ConfigMode{
		name:    name,
		actions: actions,
	}
}

// Name returns the name of the mode
func (m *ConfigMode) Name() string {
	return m.name
}

// ProcessEvent processes an event and returns actions to execute
func (m *ConfigMode) ProcessEvent(event *Event) []*conf.Action {
	// Match event against bindings
	for _, binding := range m.actions.Triggers {
		if matchesEvent(binding.SourceEvent, event) {
			return []*conf.Action{binding.Action}
		}
	}
	return nil
}

// matchesEvent checks if an event matches a source event configuration
func matchesEvent(sourceEvent *conf.SourceEvent, event *Event) bool {
	if sourceEvent == nil || event == nil {
		return false
	}

	// Handle key press events
	if sourceEvent.KeyPressEvent != nil && event.KeyPress != nil {
		// Only match on key press, not release
		if !event.KeyPress.Pressed && !event.KeyPress.Repeated {
			return false
		}

		// Match by key
		if sourceEvent.KeyPressEvent.Key == event.KeyPress.Key {
			slog.Debug("Matched key press", "key", event.KeyPress.Key)
			return true
		}
	}

	// Handle window focus events
	if sourceEvent.WindowFocusEvent != nil && event.WindowFocus != nil {
		if event.WindowFocus.Window != nil &&
			slices.Contains(sourceEvent.WindowFocusEvent.WindowClass, event.WindowFocus.Window.Class) {
			slog.Debug("Matched window focus", "class", event.WindowFocus.Window.Class)
			return true
		}
	}

	// Handle mouse click events
	if sourceEvent.MouseClickEvent != nil && event.MouseClick != nil {
		// For simplicity, we're not doing exact coordinate matching
		// A more sophisticated implementation might use a proximity check
		if event.MouseClick.Pressed {
			slog.Debug("Matched mouse click", "x", event.MouseClick.X, "y", event.MouseClick.Y)
			return true
		}
	}

	return false
}
