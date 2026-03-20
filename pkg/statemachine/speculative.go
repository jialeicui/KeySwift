package statemachine

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// SpeculativeExecutor handles speculative execution with compensation
type SpeculativeExecutor struct {
	mu           sync.RWMutex
	layer        *SpeculativeLayer
	compensator  *Compensator
	config       SpeculativeConfig
	output       OutputDevice
	onConfirm    func(KeyCode)
	onCompensate func(KeyCode)
	nextOpID     int
}

// SpeculativeLayer tracks speculative operations
type SpeculativeLayer struct {
	pending   map[KeyCode]*SpeculativeOp
	confirmed map[KeyCode]*SpeculativeOp
}

// Compensator handles compensation for speculative operations
type Compensator struct {
	history []CompensationRecord
	mu      sync.RWMutex
}

// CompensationRecord tracks a completed compensation
type CompensationRecord struct {
	Op            *SpeculativeOp
	CompensatedAt time.Time
}

// NewSpeculativeExecutor creates a new SpeculativeExecutor
func NewSpeculativeExecutor(config SpeculativeConfig, output OutputDevice) *SpeculativeExecutor {
	return &SpeculativeExecutor{
		layer: &SpeculativeLayer{
			pending:   make(map[KeyCode]*SpeculativeOp),
			confirmed: make(map[KeyCode]*SpeculativeOp),
		},
		compensator: &Compensator{
			history: make([]CompensationRecord, 0),
		},
		config:   config,
		output:   output,
		nextOpID: 1,
	}
}

// SetCallbacks sets the confirmation and compensation callbacks
func (se *SpeculativeExecutor) SetCallbacks(onConfirm, onCompensate func(KeyCode)) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.onConfirm = onConfirm
	se.onCompensate = onCompensate
}

// ExecuteSpeculative executes a command speculatively
func (se *SpeculativeExecutor) ExecuteSpeculative(cmd OutputCommand) (*SpeculativeOp, error) {
	if !se.config.Enabled {
		// Speculative execution disabled, execute directly
		return nil, se.output.Execute(cmd)
	}

	se.mu.Lock()
	defer se.mu.Unlock()

	// Execute the command
	if err := se.output.Execute(cmd); err != nil {
		return nil, err
	}

	// Create speculative operation
	op := &SpeculativeOp{
		ID:              OpID(fmt.Sprintf("%d", se.nextOpID)),
		Key:             cmd.Key,
		Action:          cmd.Action,
		ExecutedAt:      time.Now(),
		ConfirmDeadline: time.Now().Add(se.config.ConfirmationWindow),
		Status:          SpeculativePending,
		Compensation:    se.buildCompensation(cmd),
	}
	se.nextOpID++

	se.layer.pending[cmd.Key] = op

	slog.Debug("Executed speculative command",
		"key", cmd.Key,
		"action", cmd.Action,
		"opID", op.ID,
		"deadline", op.ConfirmDeadline)

	return op, nil
}

// Confirm confirms a speculative operation
func (se *SpeculativeExecutor) Confirm(key KeyCode) bool {
	se.mu.Lock()
	defer se.mu.Unlock()

	op, ok := se.layer.pending[key]
	if !ok {
		return false
	}

	op.Status = SpeculativeConfirmed
	se.layer.confirmed[key] = op
	delete(se.layer.pending, key)

	slog.Debug("Confirmed speculative operation", "key", key, "opID", op.ID)

	if se.onConfirm != nil {
		se.onConfirm(key)
	}

	return true
}

// Compensate compensates for a speculative operation
func (se *SpeculativeExecutor) Compensate(key KeyCode) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	op, ok := se.layer.pending[key]
	if !ok {
		return nil // Not a speculative operation
	}

	return se.compensateOp(op)
}

// CompensateAll compensates all pending speculative operations
func (se *SpeculativeExecutor) CompensateAll() error {
	se.mu.Lock()
	defer se.mu.Unlock()

	var lastErr error
	for key, op := range se.layer.pending {
		if err := se.compensateOp(op); err != nil {
			lastErr = err
			slog.Error("Failed to compensate operation", "key", key, "error", err)
		}
	}

	return lastErr
}

// CheckDeadlines checks and handles expired confirmation deadlines
func (se *SpeculativeExecutor) CheckDeadlines() []KeyCode {
	se.mu.Lock()
	defer se.mu.Unlock()

	now := time.Now()
	var expired []KeyCode

	for key, op := range se.layer.pending {
		if now.After(op.ConfirmDeadline) {
			expired = append(expired, key)
			// Auto-confirm expired operations (safer than compensating)
			op.Status = SpeculativeConfirmed
			se.layer.confirmed[key] = op
			delete(se.layer.pending, key)

			slog.Debug("Auto-confirmed expired speculative operation",
				"key", key,
				"opID", op.ID)

			if se.onConfirm != nil {
				se.onConfirm(key)
			}
		}
	}

	return expired
}

// IsPending checks if a key has a pending speculative operation
func (se *SpeculativeExecutor) IsPending(key KeyCode) bool {
	se.mu.RLock()
	defer se.mu.RUnlock()
	_, ok := se.layer.pending[key]
	return ok
}

// IsConfirmed checks if a key has a confirmed speculative operation
func (se *SpeculativeExecutor) IsConfirmed(key KeyCode) bool {
	se.mu.RLock()
	defer se.mu.RUnlock()
	_, ok := se.layer.confirmed[key]
	return ok
}

// GetPendingOps returns all pending operations
func (se *SpeculativeExecutor) GetPendingOps() []*SpeculativeOp {
	se.mu.RLock()
	defer se.mu.RUnlock()

	ops := make([]*SpeculativeOp, 0, len(se.layer.pending))
	for _, op := range se.layer.pending {
		ops = append(ops, se.cloneOp(op))
	}
	return ops
}

// Clear clears all speculative operations
func (se *SpeculativeExecutor) Clear() {
	se.mu.Lock()
	defer se.mu.Unlock()

	se.layer.pending = make(map[KeyCode]*SpeculativeOp)
	se.layer.confirmed = make(map[KeyCode]*SpeculativeOp)
	se.compensator.Clear()
}

// compensateOp compensates for a single operation
func (se *SpeculativeExecutor) compensateOp(op *SpeculativeOp) error {
	slog.Debug("Compensating speculative operation",
		"key", op.Key,
		"opID", op.ID,
		"mode", se.config.CompensationMode)

	switch se.config.CompensationMode {
	case CompensationImmediate:
		// Execute compensation immediately
		for _, cmd := range op.Compensation.RevertActions {
			if err := se.output.Execute(cmd); err != nil {
				return err
			}
		}
		for _, cmd := range op.Compensation.AlignmentActions {
			if err := se.output.Execute(cmd); err != nil {
				return err
			}
		}

	case CompensationDeferred:
		// Don't execute yet, just mark for later
		// The actual compensation happens when physical key is released

	case CompensationSmart:
		// Smart mode: decide based on context
		// For now, same as immediate
		for _, cmd := range op.Compensation.RevertActions {
			if err := se.output.Execute(cmd); err != nil {
				return err
			}
		}
	}

	// Update status
	op.Status = SpeculativeCompensated
	delete(se.layer.pending, op.Key)

	// Record compensation
	se.compensator.Record(CompensationRecord{
		Op:            op,
		CompensatedAt: time.Now(),
	})

	if se.onCompensate != nil {
		se.onCompensate(op.Key)
	}

	return nil
}

// buildCompensation builds the compensation strategy for a command
func (se *SpeculativeExecutor) buildCompensation(cmd OutputCommand) CompensationStrategy {
	strategy := CompensationStrategy{
		RevertActions:    make([]OutputCommand, 0),
		AlignmentActions: make([]OutputCommand, 0),
	}

	switch cmd.Action {
	case KeyPress:
		// To compensate a press, we release
		strategy.RevertActions = append(strategy.RevertActions, OutputCommand{
			Key:    cmd.Key,
			Action: KeyRelease,
		})
		// Alignment: ensure clean state
		strategy.AlignmentActions = append(strategy.AlignmentActions, OutputCommand{
			Key:    cmd.Key,
			Action: KeyRelease,
		})

	case KeyRelease:
		// To compensate a release, we press
		strategy.RevertActions = append(strategy.RevertActions, OutputCommand{
			Key:    cmd.Key,
			Action: KeyPress,
		})
	}

	return strategy
}

// cloneOp creates a copy of a speculative operation
func (se *SpeculativeExecutor) cloneOp(op *SpeculativeOp) *SpeculativeOp {
	return &SpeculativeOp{
		ID:              op.ID,
		Key:             op.Key,
		Action:          op.Action,
		ExecutedAt:      op.ExecutedAt,
		ConfirmDeadline: op.ConfirmDeadline,
		Status:          op.Status,
		Compensation:    op.Compensation,
	}
}

const maxCompensationHistory = 1000

// Record records a compensation
func (c *Compensator) Record(record CompensationRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = append(c.history, record)
	if len(c.history) > maxCompensationHistory {
		c.history = c.history[len(c.history)-maxCompensationHistory:]
	}
}

// IsCompensated checks if a key was recently compensated
func (c *Compensator) IsCompensated(key KeyCode, within time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cutoff := time.Now().Add(-within)
	for _, record := range c.history {
		if record.Op.Key == key && record.CompensatedAt.After(cutoff) {
			return true
		}
	}
	return false
}

// Clear clears compensation history
func (c *Compensator) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = make([]CompensationRecord, 0)
}
