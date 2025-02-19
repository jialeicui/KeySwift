package conf

// KeymapGroup represents a group of key mappings that apply to specific app groups
type KeymapGroup struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	AppGroups   []string `yaml:"appGroups"` // App groups this keymap applies to (empty for global)
	Keymaps     []Keymap `yaml:"keymaps"`   // The actual key mappings
	Enabled     bool     `yaml:"enabled"`   // Whether this group is active
}

// AppliesTo checks if this keymap group applies to a given app
func (kg *KeymapGroup) AppliesTo(windowClass string, appGroups map[string]AppGroup) bool {
	// If no app groups specified, this is a global keymap
	if len(kg.AppGroups) == 0 {
		return true
	}

	// Check if any of the app groups match
	for _, appGroupName := range kg.AppGroups {
		if appGroup, exists := appGroups[appGroupName]; exists {
			if appGroup.MatchesApp(windowClass) {
				return true
			}
		}
	}
	return false
}
