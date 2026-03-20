package statemachine

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/jialeicui/keyswift/pkg/config"
	"github.com/jialeicui/keyswift/pkg/keys"
)

// ConfigMappingEngine is a mapping engine that uses static configuration.
// It performs set-based matching: key combinations are unordered sets,
// so [cmd, 1] and [1, cmd] are equivalent.
type ConfigMappingEngine struct {
	mu          sync.RWMutex
	mappings    []config.MappingRule
	windowClass string

	// inputKeySets stores sorted key sets for O(n) set comparison
	inputKeySets [][]keys.Key
	// keyToRules maps each key to the rules it appears in (for PrefixMatch)
	keyToRules map[keys.Key][]int
}

// NewConfigMappingEngine creates a new mapping engine from configuration
func NewConfigMappingEngine(cfg *config.Config) *ConfigMappingEngine {
	engine := &ConfigMappingEngine{
		mappings:   cfg.Mappings,
		keyToRules: make(map[keys.Key][]int),
	}

	// Build sorted input key sets and key-to-rule index
	engine.inputKeySets = make([][]keys.Key, len(cfg.Mappings))
	for i := range cfg.Mappings {
		sorted := make([]keys.Key, len(cfg.Mappings[i].Input))
		copy(sorted, cfg.Mappings[i].Input)
		sort.Slice(sorted, func(a, b int) bool { return sorted[a] < sorted[b] })
		engine.inputKeySets[i] = sorted

		// Index each key to the rules it appears in
		for _, key := range sorted {
			engine.keyToRules[key] = append(engine.keyToRules[key], i)
		}
	}

	slog.Info("Config mapping engine initialized", "mappings", len(cfg.Mappings))
	return engine
}

// SetWindowClass updates the current window class
func (e *ConfigMappingEngine) SetWindowClass(class string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.windowClass != class {
		slog.Debug("Config engine window class changed", "from", e.windowClass, "to", class)
	}
	e.windowClass = class
}

// Match implements MappingEngine.
// It treats key combinations as unordered sets.
func (e *ConfigMappingEngine) Match(state SemanticState) ([]OutputCommand, bool) {
	e.mu.RLock()
	windowClass := e.windowClass
	e.mu.RUnlock()

	// Build input set from semantic state
	// Include: active modifiers + pending modifiers + independents (downgraded modifiers) + combo keys
	inputKeys := make([]keys.Key, 0)

	for key := range state.Modifiers.Active {
		inputKeys = append(inputKeys, key)
	}
	for key := range state.Modifiers.Pending {
		inputKeys = append(inputKeys, key)
	}
	// Include independents that were downgraded from modifiers —
	// they may still be part of a valid combo if the non-modifier key arrives later
	for key, info := range state.Independents {
		if info.DowngradedFrom {
			inputKeys = append(inputKeys, key)
		}
	}
	inputKeys = append(inputKeys, state.Combo.ActiveKeys...)

	sort.Slice(inputKeys, func(i, j int) bool { return inputKeys[i] < inputKeys[j] })

	slog.Debug("Config engine matching", "inputKeys", inputKeys, "windowClass", windowClass)

	// Find matching rules (exact set match)
	for i, sortedRule := range e.inputKeySets {
		if keySetsEqual(inputKeys, sortedRule) {
			mapping := &e.mappings[i]
			if mapping.MatchWindowClass(windowClass) {
				commands := e.buildOutputCommands(mapping.Output)
				slog.Info("Mapping matched", "input", mapping.Input, "output", mapping.Output, "window", windowClass)
				return commands, true
			}
		}
	}

	return nil, false
}

// PrefixMatch implements MappingEngine.
// Returns all mapping IDs where the given keys are a SUBSET of the mapping's input keys.
// This is used by SSM to determine if a modifier key could potentially be part of a combo.
func (e *ConfigMappingEngine) PrefixMatch(queryKeys []keys.Key) []MappingID {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if len(queryKeys) == 0 {
		return nil
	}

	// For each query key, find rules that contain it
	// Start with rules containing the first key, then intersect
	candidates, ok := e.keyToRules[queryKeys[0]]
	if !ok || len(candidates) == 0 {
		return nil
	}

	// If single key query (common case: modifier check), just return matching rules
	if len(queryKeys) == 1 {
		matches := make([]MappingID, len(candidates))
		for i, idx := range candidates {
			matches[i] = MappingID(fmt.Sprintf("mapping-%d", idx))
		}
		return matches
	}

	// Multiple keys: intersect candidates
	candidateSet := make(map[int]bool, len(candidates))
	for _, idx := range candidates {
		candidateSet[idx] = true
	}

	for _, key := range queryKeys[1:] {
		rules, ok := e.keyToRules[key]
		if !ok {
			return nil
		}
		newSet := make(map[int]bool)
		for _, idx := range rules {
			if candidateSet[idx] {
				newSet[idx] = true
			}
		}
		candidateSet = newSet
		if len(candidateSet) == 0 {
			return nil
		}
	}

	matches := make([]MappingID, 0, len(candidateSet))
	for idx := range candidateSet {
		matches = append(matches, MappingID(fmt.Sprintf("mapping-%d", idx)))
	}
	return matches
}

func (e *ConfigMappingEngine) buildOutputCommands(output []keys.Key) []OutputCommand {
	commands := make([]OutputCommand, 0, len(output))

	// Press all output keys
	for _, key := range output {
		commands = append(commands, OutputCommand{
			Key:    key,
			Action: KeyPress,
		})
	}

	return commands
}

// keySetsEqual checks if two sorted key slices are equal
func keySetsEqual(a, b []keys.Key) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
