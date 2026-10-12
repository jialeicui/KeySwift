package statemachine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jialeicui/golibevdev"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedEvent struct {
	key     KeyCode
	pressed bool
}

type mockProcessor struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (m *mockProcessor) ProcessEvent(device DeviceID, key KeyCode, pressed bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, recordedEvent{key: key, pressed: pressed})
	return nil
}

func (m *mockProcessor) snapshot() []recordedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]recordedEvent{}, m.events...)
}

func TestChatterFilter_SwallowsChatterPair(t *testing.T) {
	mock := &mockProcessor{}
	filter := NewChatterFilter(mock, 80*time.Millisecond,
		map[KeyCode]bool{golibevdev.KeyLeftMeta: true}, context.Background())

	require.NoError(t, filter.ProcessEvent("kbd", golibevdev.KeyLeftMeta, true))
	require.NoError(t, filter.ProcessEvent("kbd", golibevdev.KeyLeftMeta, false)) // deferred
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, filter.ProcessEvent("kbd", golibevdev.KeyLeftMeta, true)) // chatter pair swallowed

	time.Sleep(150 * time.Millisecond) // pending release must have been cancelled

	events := mock.snapshot()
	require.Len(t, events, 1, "only the original press should be forwarded, got %v", events)
	assert.True(t, events[0].pressed)

	// The real release is forwarded after the defer window
	require.NoError(t, filter.ProcessEvent("kbd", golibevdev.KeyLeftMeta, false))
	time.Sleep(150 * time.Millisecond)

	events = mock.snapshot()
	require.Len(t, events, 2)
	assert.False(t, events[1].pressed)
}

func TestChatterFilter_ForwardsNormalReleaseAfterDelay(t *testing.T) {
	mock := &mockProcessor{}
	filter := NewChatterFilter(mock, 50*time.Millisecond, nil, context.Background())

	require.NoError(t, filter.ProcessEvent("kbd", golibevdev.KeyA, true))
	require.NoError(t, filter.ProcessEvent("kbd", golibevdev.KeyA, false))

	// Not yet forwarded
	assert.Len(t, mock.snapshot(), 1)

	time.Sleep(100 * time.Millisecond)
	events := mock.snapshot()
	require.Len(t, events, 2)
	assert.False(t, events[1].pressed)
}

func TestChatterFilter_PassThrough(t *testing.T) {
	mock := &mockProcessor{}

	// Zero threshold: filter off
	off := NewChatterFilter(mock, 0, map[KeyCode]bool{golibevdev.KeyA: true}, context.Background())
	require.NoError(t, off.ProcessEvent("kbd", golibevdev.KeyA, true))
	require.NoError(t, off.ProcessEvent("kbd", golibevdev.KeyA, false))
	require.NoError(t, off.ProcessEvent("kbd", golibevdev.KeyA, true))
	assert.Len(t, mock.snapshot(), 3)

	// Key not in the guard list: release is immediate even with filter on
	on := NewChatterFilter(mock, time.Hour, map[KeyCode]bool{golibevdev.KeyA: true}, context.Background())
	require.NoError(t, on.ProcessEvent("kbd", golibevdev.KeyB, true))
	require.NoError(t, on.ProcessEvent("kbd", golibevdev.KeyB, false))
	assert.Len(t, mock.snapshot(), 5)
}

func TestChatterFilter_StrayReleaseForwarded(t *testing.T) {
	mock := &mockProcessor{}
	filter := NewChatterFilter(mock, time.Hour, nil, context.Background())

	// Release without a prior press (e.g. key held across startup)
	require.NoError(t, filter.ProcessEvent("kbd", golibevdev.KeyA, false))
	assert.Len(t, mock.snapshot(), 1)
}
