package conf

import "strings"

// Keymap defines a single key mapping
type Keymap struct {
	Key         string `yaml:"key"`                   // Source key combination (e.g., "Ctrl+Shift+A")
	Command     string `yaml:"command"`               // Target action or key combination
	When        string `yaml:"when,omitempty"`        // Optional condition for when this keymap applies
	Description string `yaml:"description,omitempty"` // Human-readable description
	Enabled     bool   `yaml:"enabled"`               // Whether this keymap is active
}

// ParseKeyCombo parses a key combination string like "Ctrl+Shift+A" into modifiers and key
func ParseKeyCombo(combo string) (modifiers []string, key string) {
	parts := strings.Split(combo, "+")
	if len(parts) == 1 {
		return nil, parts[0]
	}
	return parts[:len(parts)-1], parts[len(parts)-1]
}
