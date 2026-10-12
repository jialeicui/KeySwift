package statemachine

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// KeyEventProcessor is the event entry point used by input device goroutines.
type KeyEventProcessor interface {
	ProcessEvent(device DeviceID, key KeyCode, pressed bool) error
}

// ChatterFilter filters switch chatter: a worn key emitting spurious
// release+press pairs while physically held.
//
// Filtering a press that follows a release is not enough — the spurious
// release has already propagated (released outputs, emitted modifier taps),
// leaving the key logically released while the user still holds it. So for
// guarded keys this filter DEFERS releases by the configured threshold: if a
// press for the same key arrives within the window, the release+press pair is
// swallowed as chatter and the key never left the pressed state; otherwise the
// release is forwarded when the timer fires. The cost is that real releases of
// guarded keys take effect up to threshold later.
type ChatterFilter struct {
	inner     KeyEventProcessor
	threshold time.Duration
	keys      map[KeyCode]bool // empty = guard all keys
	ctx       context.Context

	mu      sync.Mutex
	pressed map[KeyCode]bool        // press forwarded, release not yet forwarded
	pending map[KeyCode]*time.Timer // deferred releases awaiting confirmation
}

// NewChatterFilter wraps inner with chatter filtering. A zero threshold makes
// it a pure pass-through.
func NewChatterFilter(inner KeyEventProcessor, threshold time.Duration, keys map[KeyCode]bool, ctx context.Context) *ChatterFilter {
	return &ChatterFilter{
		inner:     inner,
		threshold: threshold,
		keys:      keys,
		ctx:       ctx,
		pressed:   make(map[KeyCode]bool),
		pending:   make(map[KeyCode]*time.Timer),
	}
}

// ProcessEvent implements KeyEventProcessor.
func (f *ChatterFilter) ProcessEvent(device DeviceID, key KeyCode, pressed bool) error {
	if f.threshold <= 0 || (len(f.keys) > 0 && !f.keys[key]) {
		return f.inner.ProcessEvent(device, key, pressed)
	}

	f.mu.Lock()
	if pressed {
		if timer, ok := f.pending[key]; ok {
			// A release for this key is being deferred: this press completes a
			// chatter pair. Swallow both — the key never left pressed state.
			timer.Stop()
			delete(f.pending, key)
			f.mu.Unlock()
			slog.Debug("Chatter filtered: swallowed release+press pair", "key", key, "device", device)
			return nil
		}
		f.pressed[key] = true
		f.mu.Unlock()
		return f.inner.ProcessEvent(device, key, pressed)
	}

	if !f.pressed[key] {
		// Stray release for a key we never forwarded a press for.
		f.mu.Unlock()
		return f.inner.ProcessEvent(device, key, pressed)
	}

	f.pending[key] = time.AfterFunc(f.threshold, func() {
		f.mu.Lock()
		if _, ok := f.pending[key]; !ok {
			// Cancelled after the timer fired (lost the race with Stop).
			f.mu.Unlock()
			return
		}
		delete(f.pending, key)
		delete(f.pressed, key)
		f.mu.Unlock()

		if f.ctx.Err() != nil {
			return
		}
		if err := f.inner.ProcessEvent(device, key, false); err != nil {
			slog.Error("Failed to forward deferred release", "key", key, "device", device, "error", err)
		}
	})
	f.mu.Unlock()
	return nil
}

// HandleDeviceLost cancels deferred releases; the state machine synthesizes
// its own release events for lost devices, so forwarding them later would
// only produce duplicates.
func (f *ChatterFilter) HandleDeviceLost() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, timer := range f.pending {
		timer.Stop()
		delete(f.pending, key)
	}
	f.pressed = make(map[KeyCode]bool)
}
