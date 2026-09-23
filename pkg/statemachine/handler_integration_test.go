package statemachine

import (
	"errors"
	"testing"

	"github.com/jialeicui/golibevdev"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSameInputDevice(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{
			name:     "same device name",
			expected: "HHKB-Hybrid_1 Keyboard",
			actual:   "HHKB-Hybrid_1 Keyboard",
			want:     true,
		},
		{
			name:     "event node reused by another device",
			expected: "HHKB-Hybrid_1 Keyboard",
			actual:   "MX Anywhere 3",
			want:     false,
		},
		{
			name:     "empty actual name",
			expected: "HHKB-Hybrid_1 Keyboard",
			actual:   "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSameInputDevice(tt.expected, tt.actual))
		})
	}
}

func TestClassifyInputEvent(t *testing.T) {
	tests := []struct {
		name string
		ev   golibevdev.Event
		want inputEventDispatch
	}{
		{
			name: "key event goes to the state machine",
			ev:   golibevdev.Event{Type: golibevdev.EvKey, Code: golibevdev.KeyA, Value: 1},
			want: inputEventStateMachine,
		},
		{
			name: "mouse button goes to the state machine",
			ev:   golibevdev.Event{Type: golibevdev.EvKey, Code: golibevdev.BtnLeft, Value: 1},
			want: inputEventStateMachine,
		},
		{
			name: "syn dropped triggers sync recovery",
			ev:   golibevdev.Event{Type: golibevdev.EvSyn, Code: golibevdev.SynDropped, Value: 0},
			want: inputEventSyncRecovery,
		},
		{
			name: "syn report is forwarded",
			ev:   golibevdev.Event{Type: golibevdev.EvSyn, Code: golibevdev.SynReport, Value: 0},
			want: inputEventForward,
		},
		{
			name: "relative motion is forwarded",
			ev:   golibevdev.Event{Type: golibevdev.EvRel, Code: golibevdev.RelX, Value: -5},
			want: inputEventForward,
		},
		{
			name: "wheel scroll is forwarded",
			ev:   golibevdev.Event{Type: golibevdev.EvRel, Code: golibevdev.RelWheel, Value: 1},
			want: inputEventForward,
		},
		{
			name: "absolute motion is forwarded",
			ev:   golibevdev.Event{Type: golibevdev.EvAbs, Code: golibevdev.AbsoluteAxesEventCode(0), Value: 100},
			want: inputEventForward,
		},
		{
			name: "misc event is forwarded",
			ev:   golibevdev.Event{Type: golibevdev.EvMsc, Code: golibevdev.MiscEventCode(4), Value: 30},
			want: inputEventForward,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyInputEvent(tt.ev))
		})
	}
}

type failingOutputDevice struct {
	err error
}

func (f *failingOutputDevice) Execute(cmd OutputCommand) error        { return f.err }
func (f *failingOutputDevice) ForwardEvent(ev golibevdev.Event) error { return f.err }
func (f *failingOutputDevice) Sync() error                            { return f.err }
func (f *failingOutputDevice) Close() error                           { return nil }

func TestForwardInputEvent(t *testing.T) {
	out := &MockOutputDevice{}
	h := &HandlerWithStateMachine{out: out}
	dev := &handlerDevice{name: "test-device"}

	ev := golibevdev.Event{Type: golibevdev.EvRel, Code: golibevdev.RelWheel, Value: -1}
	h.forwardInputEvent(dev, ev)

	require.Len(t, out.events, 1)
	assert.Equal(t, golibevdev.EvRel, out.events[0].Type)
	assert.Equal(t, golibevdev.EventCode(golibevdev.RelWheel), out.events[0].Code)
	assert.Equal(t, int32(-1), out.events[0].Value)
}

func TestForwardInputEventFailureDoesNotPropagate(t *testing.T) {
	h := &HandlerWithStateMachine{out: &failingOutputDevice{err: errors.New("write failed")}}
	dev := &handlerDevice{name: "test-device"}

	// A forward failure must be logged and swallowed, never killing the
	// event loop.
	h.forwardInputEvent(dev, golibevdev.Event{Type: golibevdev.EvRel, Code: golibevdev.RelX, Value: 1})
}

func TestForwardInputEventWithoutOutputDevice(t *testing.T) {
	h := &HandlerWithStateMachine{}
	dev := &handlerDevice{name: "test-device"}

	h.forwardInputEvent(dev, golibevdev.Event{Type: golibevdev.EvRel, Code: golibevdev.RelX, Value: 1})
}
