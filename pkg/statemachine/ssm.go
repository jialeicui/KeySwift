package statemachine

import (
	"log/slog"
	"sync"
	"time"
)

// SSM (Semantic State Machine) transforms physical input into semantic meaning
type SSM struct {
	mu             sync.RWMutex
	current        SemanticState
	previousActive map[KeyCode]ModifierInfo
	config         SemanticConfig
	engine         MappingEngine
	// Callback for when a pending modifier times out
	onPendingTimeout func(KeyCode)
}

// NewSSM creates a new Semantic State Machine
func NewSSM(config SemanticConfig, engine MappingEngine) *SSM {
	return &SSM{
		current: SemanticState{
			Modifiers: ModifierSemantic{
				Active:  make(map[KeyCode]ModifierInfo),
				Pending: make(map[KeyCode]*PendingModifier),
			},
			Independents: make(map[KeyCode]IndependentState),
			Combo:        ComboState{},
		},
		config: config,
		engine: engine,
	}
}

// SetPendingTimeoutCallback sets the callback for pending modifier timeout
func (ssm *SSM) SetPendingTimeoutCallback(cb func(KeyCode)) {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()
	ssm.onPendingTimeout = cb
}

// Translate converts input state to semantic state
func (ssm *SSM) Translate(input InputState) SemanticState {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	newState := SemanticState{
		Modifiers: ModifierSemantic{
			Active:  make(map[KeyCode]ModifierInfo),
			Pending: make(map[KeyCode]*PendingModifier),
		},
		Independents: make(map[KeyCode]IndependentState),
		Combo:        ComboState{},
	}

	// Process each key in input state
	for key, state := range input.KeyStates {
		if !state.Pressed {
			continue
		}

		if !IsModifier(key) {
			// Non-modifier keys go to combo
			newState.Combo.ActiveKeys = append(newState.Combo.ActiveKeys, key)
			continue
		}

		// Check if this modifier appears in any mapping rules
		potentialMatches := ssm.engine.PrefixMatch([]KeyCode{key})

		if len(potentialMatches) == 0 {
			// No mapping rules use this modifier — treat as independent (pass-through)
			newState.Independents[key] = IndependentState{
				Key:       key,
				IsPressed: true,
			}
			slog.Debug("No potential matches for modifier, treating as independent",
				"key", key)
			continue
		}

		// This modifier appears in mapping rules — classify as Active.
		// It will be absorbed (not forwarded) until a combo key is pressed.
		newState.Modifiers.Active[key] = ModifierInfo{
			Key:       key,
			PressedAt: state.PressedAt,
		}
		slog.Debug("Modifier classified as active (has mapping rules)", "key", key)
	}

	// Save current active modifiers before overwriting
	ssm.previousActive = make(map[KeyCode]ModifierInfo)
	for k, v := range ssm.current.Modifiers.Active {
		ssm.previousActive[k] = v
	}

	ssm.current = newState
	return newState
}

// WasActiveModifier returns whether the given key was an Active modifier
// in the state just before the last Translate call.
// This is used to detect modifier release without combo.
func (ssm *SSM) WasActiveModifier(key KeyCode) (ModifierInfo, bool) {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	info, ok := ssm.previousActive[key]
	return info, ok
}

// GetCurrent returns the current semantic state
func (ssm *SSM) GetCurrent() SemanticState {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	return ssm.cloneState(ssm.current)
}

// AttemptDowngrade attempts to downgrade an independent modifier back to pending
// This is used when a combination is detected after the modifier became independent
func (ssm *SSM) AttemptDowngrade(key KeyCode) bool {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	if !ssm.config.AllowDowngrade {
		return false
	}

	independent, ok := ssm.current.Independents[key]
	if !ok || !independent.IsPressed || !independent.DowngradedFrom {
		return false
	}

	// Check if within downgrade window
	// We use the current time minus when it became independent
	// Since we don't track that exactly, we use a heuristic
	now := time.Now()

	ssm.current.Modifiers.Pending[key] = &PendingModifier{
		Key:               key,
		PressedAt:         now.Add(-ssm.config.ModifierPendingThreshold), // Approximate
		DowngradeDeadline: now.Add(ssm.config.DowngradeWindow),
		PassedThrough:     true, // Already passed through, mark as such
	}
	delete(ssm.current.Independents, key)

	slog.Debug("Successfully downgraded independent to pending", "key", key)
	return true
}

// ConfirmPending confirms a pending modifier as active
func (ssm *SSM) ConfirmPending(key KeyCode) bool {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	pending, ok := ssm.current.Modifiers.Pending[key]
	if !ok {
		return false
	}

	ssm.current.Modifiers.Active[key] = ModifierInfo{
		Key:       key,
		PressedAt: pending.PressedAt,
	}
	delete(ssm.current.Modifiers.Pending, key)

	slog.Debug("Pending modifier confirmed as active", "key", key)
	return true
}

// MarkPendingPassedThrough marks a pending modifier as already passed through
func (ssm *SSM) MarkPendingPassedThrough(key KeyCode) bool {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	pending, ok := ssm.current.Modifiers.Pending[key]
	if !ok {
		return false
	}

	pending.PassedThrough = true
	slog.Debug("Pending modifier marked as passed through", "key", key)
	return true
}

// IsModifierActive checks if a modifier is in active state
func (ssm *SSM) IsModifierActive(key KeyCode) bool {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	_, ok := ssm.current.Modifiers.Active[key]
	return ok
}

// IsModifierPending checks if a modifier is in pending state
func (ssm *SSM) IsModifierPending(key KeyCode) bool {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	_, ok := ssm.current.Modifiers.Pending[key]
	return ok
}

// IsIndependent checks if a key is in independent state
func (ssm *SSM) IsIndependent(key KeyCode) bool {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	state, ok := ssm.current.Independents[key]
	return ok && state.IsPressed
}

// GetActiveModifiers returns all active modifiers
func (ssm *SSM) GetActiveModifiers() []KeyCode {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()

	keys := make([]KeyCode, 0, len(ssm.current.Modifiers.Active))
	for key := range ssm.current.Modifiers.Active {
		keys = append(keys, key)
	}
	return keys
}

// GetPendingModifiers returns all pending modifiers
func (ssm *SSM) GetPendingModifiers() []KeyCode {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()

	keys := make([]KeyCode, 0, len(ssm.current.Modifiers.Pending))
	for key := range ssm.current.Modifiers.Pending {
		keys = append(keys, key)
	}
	return keys
}

// GetComboKeys returns the current combo keys
func (ssm *SSM) GetComboKeys() []KeyCode {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	return append([]KeyCode{}, ssm.current.Combo.ActiveKeys...)
}

// Clear resets the semantic state machine
func (ssm *SSM) Clear() {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	ssm.current = SemanticState{
		Modifiers: ModifierSemantic{
			Active:  make(map[KeyCode]ModifierInfo),
			Pending: make(map[KeyCode]*PendingModifier),
		},
		Independents: make(map[KeyCode]IndependentState),
		Combo:        ComboState{},
	}
}

// cloneState creates a deep copy of a semantic state
func (ssm *SSM) cloneState(state SemanticState) SemanticState {
	cloned := SemanticState{
		Modifiers: ModifierSemantic{
			Active:  make(map[KeyCode]ModifierInfo),
			Pending: make(map[KeyCode]*PendingModifier),
		},
		Independents: make(map[KeyCode]IndependentState),
		Combo: ComboState{
			ActiveKeys: append([]KeyCode{}, state.Combo.ActiveKeys...),
		},
	}

	for k, v := range state.Modifiers.Active {
		cloned.Modifiers.Active[k] = v
	}
	for k, v := range state.Modifiers.Pending {
		cloned.Modifiers.Pending[k] = v
	}
	for k, v := range state.Independents {
		cloned.Independents[k] = v
	}

	return cloned
}
