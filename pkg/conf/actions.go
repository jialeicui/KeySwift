package conf

// Action represents possible actions that can be taken
type Action struct {
	MarkMode  *MarkMode  `yaml:"set_value,omitempty"`
	MapToKeys *MapToKeys `yaml:"map_to_keys,omitempty"`
	Trigger   *Trigger   `yaml:"trigger,omitempty"`
}

// MarkMode sets modes on or off
type MarkMode map[string]any

// MapToKeys maps to a series of keyboard keys
type MapToKeys struct {
	Keys []Key `yaml:"keys"`
}

// Key represents a keyboard key with optional modifiers
type Key struct {
	Key       string   `yaml:"key"`
	Modifiers []string `yaml:"modifiers,omitempty"`
}

// Trigger represents different types of triggers
type Trigger struct {
	// For BuiltinTrigger
	Type  string `yaml:"type,omitempty"`
	Value string `yaml:"value,omitempty"`

	// For ShellTrigger
	Command string `yaml:"command,omitempty"`
}

// IsBuiltinTrigger checks if this is a builtin trigger
func (t *Trigger) IsBuiltinTrigger() bool {
	return t.Type != "" && t.Value != ""
}

// IsShellTrigger checks if this is a shell trigger
func (t *Trigger) IsShellTrigger() bool {
	return t.Command != ""
}
