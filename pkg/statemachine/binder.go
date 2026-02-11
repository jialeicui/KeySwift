package statemachine

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// StateBinder manages the binding relationships between input and output keys
// This is the core component that prevents sticky keys
type StateBinder struct {
	mu            sync.RWMutex
	bindings      map[BindingID]*Binding
	inputToOutput map[KeyCode]map[BindingID]struct{}
	outputToInput map[KeyCode]BindingID
	nextID        int
}

// NewStateBinder creates a new StateBinder
func NewStateBinder() *StateBinder {
	return &StateBinder{
		bindings:      make(map[BindingID]*Binding),
		inputToOutput: make(map[KeyCode]map[BindingID]struct{}),
		outputToInput: make(map[KeyCode]BindingID),
		nextID:        1,
	}
}

// CreateBinding creates a new binding between input and output keys
func (sb *StateBinder) CreateBinding(inputKeys, outputKeys []KeyCode, strategy ReleaseStrategy) BindingID {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	id := BindingID(fmt.Sprintf("%d", sb.nextID))
	sb.nextID++

	binding := &Binding{
		ID:         id,
		InputKeys:  append([]KeyCode{}, inputKeys...),
		OutputKeys: append([]KeyCode{}, outputKeys...),
		CreatedAt:  time.Now(),
		Status:     BindingActive,
	}

	sb.bindings[id] = binding

	// Build reverse index
	for _, inKey := range inputKeys {
		if sb.inputToOutput[inKey] == nil {
			sb.inputToOutput[inKey] = make(map[BindingID]struct{})
		}
		sb.inputToOutput[inKey][id] = struct{}{}
	}

	for _, outKey := range outputKeys {
		sb.outputToInput[outKey] = id
	}

	slog.Debug("Created binding",
		"id", id,
		"inputKeys", inputKeys,
		"outputKeys", outputKeys,
		"strategy", strategy)

	return id
}

// ReleaseBinding releases all output keys associated with a binding
func (sb *StateBinder) ReleaseBinding(id BindingID) []KeyCode {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	binding, ok := sb.bindings[id]
	if !ok || binding.Status != BindingActive {
		return nil
	}

	binding.Status = BindingReleased

	// Clean up indices
	for _, inKey := range binding.InputKeys {
		if bindings, ok := sb.inputToOutput[inKey]; ok {
			delete(bindings, id)
			if len(bindings) == 0 {
				delete(sb.inputToOutput, inKey)
			}
		}
	}

	for _, outKey := range binding.OutputKeys {
		delete(sb.outputToInput, outKey)
	}

	slog.Debug("Released binding", "id", id, "outputKeys", binding.OutputKeys)

	return binding.OutputKeys
}

// OnInputKeyReleased handles an input key release event
// Returns the output keys that should be released
func (sb *StateBinder) OnInputKeyReleased(key KeyCode) []KeyCode {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	affectedBindings, ok := sb.inputToOutput[key]
	if !ok {
		return nil
	}

	var toRelease []KeyCode
	bindingsToRelease := make([]BindingID, 0)

	for id := range affectedBindings {
		binding, ok := sb.bindings[id]
		if !ok || binding.Status != BindingActive {
			continue
		}

		// Check release strategy
		strategy := sb.determineReleaseStrategy(binding)

		switch strategy {
		case ReleaseOnAnyBoundKeyReleased:
			// Release all output keys in this binding
			for _, outKey := range binding.OutputKeys {
				toRelease = append(toRelease, outKey)
			}
			bindingsToRelease = append(bindingsToRelease, id)

		case ReleaseOnAllBoundKeysReleased:
			// Check if all input keys are released
			allReleased := true
			for _, inKey := range binding.InputKeys {
				// We need external state to know if key is pressed
				// For now, assume we're only called when a key is released
				if inKey == key {
					continue // This is the key being released
				}
				// Check if this binding is still indexed (meaning key is still held)
				if _, ok := sb.inputToOutput[inKey]; ok {
					if bindings, ok := sb.inputToOutput[inKey]; ok {
						if _, hasBinding := bindings[id]; hasBinding {
							allReleased = false
							break
						}
					}
				}
			}
			if allReleased {
				for _, outKey := range binding.OutputKeys {
					toRelease = append(toRelease, outKey)
				}
				bindingsToRelease = append(bindingsToRelease, id)
			}
		}
	}

	// Release the bindings
	for _, id := range bindingsToRelease {
		if binding, ok := sb.bindings[id]; ok {
			binding.Status = BindingReleased
			// Clean up indices
			for _, inKey := range binding.InputKeys {
				if bindings, ok := sb.inputToOutput[inKey]; ok {
					delete(bindings, id)
					if len(bindings) == 0 {
						delete(sb.inputToOutput, inKey)
					}
				}
			}
			for _, outKey := range binding.OutputKeys {
				delete(sb.outputToInput, outKey)
			}
		}
	}

	if len(toRelease) > 0 {
		slog.Debug("Input key released, triggering output release",
			"inputKey", key,
			"outputKeys", toRelease)
	}

	return toRelease
}

// GetBindingForOutput returns the binding ID for an output key
func (sb *StateBinder) GetBindingForOutput(key KeyCode) (BindingID, bool) {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	id, ok := sb.outputToInput[key]
	return id, ok
}

// GetBinding returns a binding by ID
func (sb *StateBinder) GetBinding(id BindingID) (*Binding, bool) {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	binding, ok := sb.bindings[id]
	if !ok {
		return nil, false
	}
	// Return a copy
	return &Binding{
		ID:         binding.ID,
		InputKeys:  append([]KeyCode{}, binding.InputKeys...),
		OutputKeys: append([]KeyCode{}, binding.OutputKeys...),
		CreatedAt:  binding.CreatedAt,
		Status:     binding.Status,
	}, true
}

// GetActiveBindings returns all active bindings
func (sb *StateBinder) GetActiveBindings() []*Binding {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	var active []*Binding
	for _, binding := range sb.bindings {
		if binding.Status == BindingActive {
			active = append(active, &Binding{
				ID:         binding.ID,
				InputKeys:  append([]KeyCode{}, binding.InputKeys...),
				OutputKeys: append([]KeyCode{}, binding.OutputKeys...),
				CreatedAt:  binding.CreatedAt,
				Status:     binding.Status,
			})
		}
	}
	return active
}

// Clear releases all bindings
func (sb *StateBinder) Clear() []KeyCode {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	var allOutputs []KeyCode
	for _, binding := range sb.bindings {
		if binding.Status == BindingActive {
			allOutputs = append(allOutputs, binding.OutputKeys...)
		}
	}

	sb.bindings = make(map[BindingID]*Binding)
	sb.inputToOutput = make(map[KeyCode]map[BindingID]struct{})
	sb.outputToInput = make(map[KeyCode]BindingID)

	return allOutputs
}

// determineReleaseStrategy determines the release strategy for a binding
// This can be extended to support per-binding configuration
func (sb *StateBinder) determineReleaseStrategy(binding *Binding) ReleaseStrategy {
	// Default strategy: release on any bound key release
	// This is the safest option for preventing sticky keys
	return ReleaseOnAnyBoundKeyReleased
}

// IsOutputKeyBound checks if an output key is currently bound
func (sb *StateBinder) IsOutputKeyBound(key KeyCode) bool {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	_, ok := sb.outputToInput[key]
	return ok
}

// GetBoundInputKeys returns the input keys bound to an output key
func (sb *StateBinder) GetBoundInputKeys(outputKey KeyCode) []KeyCode {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	id, ok := sb.outputToInput[outputKey]
	if !ok {
		return nil
	}

	binding, ok := sb.bindings[id]
	if !ok {
		return nil
	}

	return append([]KeyCode{}, binding.InputKeys...)
}
