// Package config provides configuration parsing and management for KeySwift.
// The configuration is a JSON array of rules that are loaded at startup
// to build static mappings for the state machine.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jialeicui/keyswift/pkg/keys"
)

// Config represents the parsed configuration
type Config struct {
	Vars     map[string][]string `json:"vars"`
	Mappings []MappingRule       `json:"mappings"`
}

// MappingRule represents a single key mapping
type MappingRule struct {
	Input  []keys.Key     `json:"input"`
	Output []keys.Key     `json:"output"`
	When   *WhenCondition `json:"when,omitempty"`
}

// WhenCondition represents conditions for a mapping to apply
type WhenCondition struct {
	Window    interface{} `json:"window,omitempty"`    // string or []string
	NotWindow interface{} `json:"notWindow,omitempty"` // string or []string
}

// RawRule represents a rule as parsed from JSON
type RawRule struct {
	Type   string         `json:"type"`
	Name   string         `json:"name,omitempty"`
	Value  interface{}    `json:"value,omitempty"`
	Input  []string       `json:"input,omitempty"`
	Output []string       `json:"output,omitempty"`
	When   *WhenCondition `json:"when,omitempty"`
}

// LoadConfig loads and parses the configuration file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var rawRules []RawRule
	if err := json.Unmarshal(data, &rawRules); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cfg := &Config{
		Vars:     make(map[string][]string),
		Mappings: make([]MappingRule, 0),
	}

	// First pass: collect all vars
	for _, rule := range rawRules {
		if rule.Type == "var" {
			values, err := parseStringSlice(rule.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid var %s: %w", rule.Name, err)
			}
			cfg.Vars[rule.Name] = values
		}
	}

	// Second pass: process mappings
	for i, rule := range rawRules {
		if rule.Type != "map" {
			continue
		}

		mapping, err := cfg.processMappingRule(rule)
		if err != nil {
			return nil, fmt.Errorf("failed to process mapping %d: %w", i, err)
		}
		cfg.Mappings = append(cfg.Mappings, *mapping)
	}

	return cfg, nil
}

func (cfg *Config) processMappingRule(rule RawRule) (*MappingRule, error) {
	if len(rule.Input) == 0 {
		return nil, fmt.Errorf("mapping must have input keys")
	}
	if len(rule.Output) == 0 {
		return nil, fmt.Errorf("mapping must have output keys")
	}

	// Resolve input keys
	input, err := cfg.resolveKeys(rule.Input)
	if err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// Resolve output keys
	output, err := cfg.resolveKeys(rule.Output)
	if err != nil {
		return nil, fmt.Errorf("invalid output: %w", err)
	}

	// Process when condition
	when := rule.When
	if when != nil {
		if when.Window != nil {
			windows, err := cfg.resolveVarRef(when.Window)
			if err != nil {
				return nil, fmt.Errorf("invalid window condition: %w", err)
			}
			when.Window = windows
		}
		if when.NotWindow != nil {
			windows, err := cfg.resolveVarRef(when.NotWindow)
			if err != nil {
				return nil, fmt.Errorf("invalid notWindow condition: %w", err)
			}
			when.NotWindow = windows
		}
	}

	return &MappingRule{
		Input:  input,
		Output: output,
		When:   when,
	}, nil
}

// resolveKeys converts key names to key codes
func (cfg *Config) resolveKeys(keyNames []string) ([]keys.Key, error) {
	result := make([]keys.Key, len(keyNames))
	for i, name := range keyNames {
		key, err := keys.GetKeyCode(name)
		if err != nil {
			return nil, fmt.Errorf("unknown key '%s': %w", name, err)
		}
		result[i] = key
	}
	return result, nil
}

// resolveVarRef resolves variable references like "$terminals" to their values
func (cfg *Config) resolveVarRef(v interface{}) ([]string, error) {
	switch val := v.(type) {
	case string:
		// Check if it's a variable reference
		if strings.HasPrefix(val, "$") {
			varName := strings.TrimPrefix(val, "$")
			if varValues, ok := cfg.Vars[varName]; ok {
				return varValues, nil
			}
			return nil, fmt.Errorf("undefined variable: %s", varName)
		}
		// Single string value
		return []string{val}, nil
	case []interface{}:
		// Array of strings (possibly with variable references)
		result := make([]string, 0)
		for _, item := range val {
			itemStr, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string in array, got %T", item)
			}
			if strings.HasPrefix(itemStr, "$") {
				varName := strings.TrimPrefix(itemStr, "$")
				if varValues, ok := cfg.Vars[varName]; ok {
					result = append(result, varValues...)
				} else {
					return nil, fmt.Errorf("undefined variable: %s", varName)
				}
			} else {
				result = append(result, itemStr)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected string or array, got %T", v)
	}
}

func parseStringSlice(v interface{}) ([]string, error) {
	switch val := v.(type) {
	case string:
		return []string{val}, nil
	case []interface{}:
		result := make([]string, len(val))
		for i, item := range val {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			result[i] = str
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected string or array, got %T", v)
	}
}

// String returns a unique string representation of the mapping rule
func (r *MappingRule) String() string {
	// Create a deterministic string representation
	inputStr := make([]string, len(r.Input))
	for i, k := range r.Input {
		inputStr[i] = fmt.Sprintf("%d", k)
	}
	outputStr := make([]string, len(r.Output))
	for i, k := range r.Output {
		outputStr[i] = fmt.Sprintf("%d", k)
	}
	data := fmt.Sprintf("%v->%v+%v", inputStr, outputStr, r.When)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// MatchWindowClass checks if a mapping matches the current window class
func (r *MappingRule) MatchWindowClass(windowClass string) bool {
	if r.When == nil {
		// No condition means match all windows
		return true
	}

	// Check window condition
	if r.When.Window != nil {
		windows := toStringSlice(r.When.Window)
		if !contains(windows, windowClass) {
			return false
		}
	}

	// Check notWindow condition
	if r.When.NotWindow != nil {
		windows := toStringSlice(r.When.NotWindow)
		if contains(windows, windowClass) {
			return false
		}
	}

	return true
}

func toStringSlice(v interface{}) []string {
	switch val := v.(type) {
	case string:
		return []string{val}
	case []string:
		return val
	case []interface{}:
		result := make([]string, len(val))
		for i, item := range val {
			if str, ok := item.(string); ok {
				result[i] = str
			}
		}
		return result
	default:
		return nil
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
