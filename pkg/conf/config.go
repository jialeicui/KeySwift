package conf

import (
	"os"
	"path/filepath"
)

// DefaultConfigPath returns the default configuration path
func DefaultConfigPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(configDir, "keyswift", "config.js")
}
