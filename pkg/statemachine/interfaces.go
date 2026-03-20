package statemachine

import (
	"context"
)

// OutputDevice abstracts the virtual output device
type OutputDevice interface {
	// Execute sends a command to the output device
	Execute(cmd OutputCommand) error
	// Sync synchronizes the device state
	Sync() error
	// Close releases the underlying output device
	Close() error
}

// MappingEngine abstracts the key mapping decision engine
type MappingEngine interface {
	// Match checks if the current input matches any mapping
	// Returns the output commands and true if matched
	Match(state SemanticState) ([]OutputCommand, bool)
	// PrefixMatch checks if the current input is a prefix of any mapping
	// Returns potential matches
	PrefixMatch(keys []KeyCode) []MappingID
}

// RingBuffer is a circular buffer for storing historical states
type RingBuffer[T any] struct {
	buffer []T
	head   int
	tail   int
	size   int
	count  int
}

// NewRingBuffer creates a new ring buffer with the given capacity
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{
		buffer: make([]T, capacity),
		size:   capacity,
	}
}

// Push adds an element to the buffer
func (rb *RingBuffer[T]) Push(item T) {
	if rb.count == rb.size {
		// Buffer is full, overwrite oldest
		rb.tail = (rb.tail + 1) % rb.size
	} else {
		rb.count++
	}
	rb.buffer[rb.head] = item
	rb.head = (rb.head + 1) % rb.size
}

// Pop removes and returns the oldest element
func (rb *RingBuffer[T]) Pop() (T, bool) {
	var zero T
	if rb.count == 0 {
		return zero, false
	}
	item := rb.buffer[rb.tail]
	rb.tail = (rb.tail + 1) % rb.size
	rb.count--
	return item, true
}

// Peek returns the newest element without removing it
func (rb *RingBuffer[T]) Peek() (T, bool) {
	var zero T
	if rb.count == 0 {
		return zero, false
	}
	idx := (rb.head - 1 + rb.size) % rb.size
	return rb.buffer[idx], true
}

// Len returns the number of elements in the buffer
func (rb *RingBuffer[T]) Len() int {
	return rb.count
}

// Clear removes all elements
func (rb *RingBuffer[T]) Clear() {
	rb.head = 0
	rb.tail = 0
	rb.count = 0
}

// Slice returns a copy of all elements in order (oldest first)
func (rb *RingBuffer[T]) Slice() []T {
	result := make([]T, 0, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.tail + i) % rb.size
		result = append(result, rb.buffer[idx])
	}
	return result
}

// StateMachine is the main interface for the state machine
type StateMachine interface {
	// ProcessEvent processes a raw key event
	ProcessEvent(event KeyEvent) error
	// GetInputState returns the current input state
	GetInputState() InputState
	// GetSemanticState returns the current semantic state
	GetSemanticState() SemanticState
	// GetOutputState returns the current desired output state
	GetOutputState() OutputState
	// Start starts the state machine background tasks
	Start(ctx context.Context) error
	// Stop stops the state machine
	Stop() error
}
