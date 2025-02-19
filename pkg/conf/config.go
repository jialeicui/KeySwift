package conf

import "fmt"

// Config represents the root configuration for KeySwift
type Config struct {
	AppGroups    map[string]AppGroup    `yaml:"appGroups"`
	KeymapGroups map[string]KeymapGroup `yaml:"keymapGroups"`
}

// Validate ensures the configuration is valid
func (c *Config) Validate() error {
	// Validate app groups references in keymap groups
	for name, kmGroup := range c.KeymapGroups {
		for _, appGroupName := range kmGroup.AppGroups {
			if _, exists := c.AppGroups[appGroupName]; !exists {
				return fmt.Errorf("keymap group '%s' references non-existent app group '%s'", name, appGroupName)
			}
		}
	}
	return nil
}
