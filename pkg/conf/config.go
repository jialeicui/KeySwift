package conf

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure for keyswift
type Config struct {
	ModeActions map[string]ModeAction `yaml:"mode_actions"`
}

// ModeAction is an array of trigger bindings
type ModeAction struct {
	If       string            `yaml:"if"`
	Triggers []*TriggerBinding `yaml:"triggers"`
}

// TriggerBinding connects a source event with an action
type TriggerBinding struct {
	SourceEvent *SourceEvent `yaml:"source_event"`
	Action      *Action      `yaml:"action"`
}

// Load loads the configuration from the specified path
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

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
	return filepath.Join(configDir, "keyswift", "config.yaml")
}
