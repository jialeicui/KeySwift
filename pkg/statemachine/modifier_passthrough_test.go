package statemachine

import (
	"context"
	"testing"
	"time"

	"github.com/jialeicui/golibevdev"
	"github.com/jialeicui/keyswift/pkg/config"
	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startModifierTestMachine(t *testing.T, engine MappingEngine, optimistic ...KeyCode) (*Machine, *MockOutputDevice) {
	t.Helper()

	mock := &MockOutputDevice{}
	cfg := DefaultConfig()
	cfg.OptimisticModifiers = make(map[KeyCode]bool, len(optimistic))
	for _, key := range optimistic {
		cfg.OptimisticModifiers[key] = true
	}

	machine := NewMachine(engine, mock, cfg)
	require.NoError(t, machine.Start(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, machine.Stop())
	})
	return machine, mock
}

func sendKey(t *testing.T, machine *Machine, key KeyCode, action KeyAction) {
	t.Helper()
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key:       key,
		Action:    action,
		Timestamp: time.Now(),
		Device:    "kbd",
	}))
}

func TestMachine_OptimisticModifierPassesThroughImmediately(t *testing.T) {
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyLeftShift, golibevdev.KeyZ},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyLeftShift, golibevdev.KeyZ},
	})
	machine, mock := startModifierTestMachine(t, engine, golibevdev.KeyLeftShift)

	sendKey(t, machine, golibevdev.KeyLeftShift, KeyPress)
	assert.Equal(t, []OutputCommand{{Key: golibevdev.KeyLeftShift, Action: KeyPress}}, mock.GetCommands())

	require.NoError(t, machine.FlushAbsorbedModifiers())
	assert.Len(t, mock.GetCommands(), 1, "already-flushed modifier must not be flushed twice")

	sendKey(t, machine, golibevdev.KeyLeftShift, KeyRelease)
	assert.Equal(t, []OutputCommand{
		{Key: golibevdev.KeyLeftShift, Action: KeyPress},
		{Key: golibevdev.KeyLeftShift, Action: KeyRelease},
	}, mock.GetCommands())
}

func TestMachine_OptimisticModifierRetractedWhenAbsentFromOutput(t *testing.T) {
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyB},
		Output: []keys.Key{golibevdev.KeyLeft},
	})
	machine, mock := startModifierTestMachine(t, engine, golibevdev.KeyLeftCtrl)

	sendKey(t, machine, golibevdev.KeyLeftCtrl, KeyPress)
	sendKey(t, machine, golibevdev.KeyB, KeyPress)
	assert.Equal(t, []OutputCommand{
		{Key: golibevdev.KeyLeftCtrl, Action: KeyPress},
		{Key: golibevdev.KeyLeftCtrl, Action: KeyRelease},
		{Key: golibevdev.KeyLeft, Action: KeyPress},
	}, mock.GetCommands(), "Ctrl must be retracted before Left is pressed")

	sendKey(t, machine, golibevdev.KeyB, KeyRelease)
	sendKey(t, machine, golibevdev.KeyLeftCtrl, KeyRelease)
	assert.Equal(t, []OutputCommand{
		{Key: golibevdev.KeyLeftCtrl, Action: KeyPress},
		{Key: golibevdev.KeyLeftCtrl, Action: KeyRelease},
		{Key: golibevdev.KeyLeft, Action: KeyPress},
		{Key: golibevdev.KeyLeft, Action: KeyRelease},
		{Key: golibevdev.KeyB, Action: KeyRelease},
	}, mock.GetCommands())
}

func TestMachine_OptimisticModifierRetractedBeforeShiftMacro(t *testing.T) {
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyK},
		Output: []keys.Key{golibevdev.KeyLeftShift, golibevdev.KeyEnd, golibevdev.KeyDelete},
	})
	machine, mock := startModifierTestMachine(t, engine, golibevdev.KeyLeftCtrl)

	sendKey(t, machine, golibevdev.KeyLeftCtrl, KeyPress)
	sendKey(t, machine, golibevdev.KeyK, KeyPress)
	assert.Equal(t, []OutputCommand{
		{Key: golibevdev.KeyLeftCtrl, Action: KeyPress},
		{Key: golibevdev.KeyLeftCtrl, Action: KeyRelease},
		{Key: golibevdev.KeyLeftShift, Action: KeyPress},
		{Key: golibevdev.KeyEnd, Action: KeyPress},
		{Key: golibevdev.KeyDelete, Action: KeyPress},
	}, mock.GetCommands(), "Ctrl+K -> Shift+End+Delete must not leave Ctrl held")
}

func TestMachine_OptimisticModifierRetainedWhenKeptInOutput(t *testing.T) {
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyLeftShift, golibevdev.KeyZ},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyLeftShift, golibevdev.KeyZ},
	})
	machine, mock := startModifierTestMachine(t, engine, golibevdev.KeyLeftShift)

	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyPress)
	sendKey(t, machine, golibevdev.KeyLeftShift, KeyPress)
	sendKey(t, machine, golibevdev.KeyZ, KeyPress)

	assert.Equal(t, []OutputCommand{
		{Key: golibevdev.KeyLeftShift, Action: KeyPress},
		{Key: golibevdev.KeyLeftCtrl, Action: KeyPress},
		{Key: golibevdev.KeyZ, Action: KeyPress},
	}, mock.GetCommands(), "Shift must neither be retracted nor pressed twice")

	mock.Clear()
	sendKey(t, machine, golibevdev.KeyZ, KeyRelease)
	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyRelease)
	for _, cmd := range mock.GetCommands() {
		assert.NotEqual(t, OutputCommand{Key: golibevdev.KeyLeftShift, Action: KeyRelease}, cmd,
			"releasing Z/Cmd must not release physically-held Shift")
	}

	mock.Clear()
	sendKey(t, machine, golibevdev.KeyLeftShift, KeyRelease)
	assert.Equal(t, []OutputCommand{{Key: golibevdev.KeyLeftShift, Action: KeyRelease}}, mock.GetCommands())
}

func TestMachine_OptimisticModifierReleasedFirstDuringMapping(t *testing.T) {
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyLeftShift, golibevdev.KeyZ},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyLeftShift, golibevdev.KeyZ},
	})
	machine, mock := startModifierTestMachine(t, engine, golibevdev.KeyLeftShift)

	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyPress)
	sendKey(t, machine, golibevdev.KeyLeftShift, KeyPress)
	sendKey(t, machine, golibevdev.KeyZ, KeyPress)
	mock.Clear()

	sendKey(t, machine, golibevdev.KeyLeftShift, KeyRelease)
	assert.ElementsMatch(t, []OutputCommand{
		{Key: golibevdev.KeyLeftCtrl, Action: KeyRelease},
		{Key: golibevdev.KeyZ, Action: KeyRelease},
		{Key: golibevdev.KeyLeftShift, Action: KeyRelease},
	}, mock.GetCommands())

	mock.Clear()
	sendKey(t, machine, golibevdev.KeyZ, KeyRelease)
	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyRelease)
	assert.Equal(t, []OutputCommand{{Key: golibevdev.KeyZ, Action: KeyRelease}}, mock.GetCommands())
	assert.Empty(t, machine.GetOutputState().PressedKeys)
}

func TestMachine_AbsorbedModifierFlushedByPointerAction(t *testing.T) {
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})
	machine, mock := startModifierTestMachine(t, engine, golibevdev.KeyLeftShift, golibevdev.KeyLeftCtrl)

	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyPress)
	assert.Empty(t, mock.GetCommands(), "Super/Cmd must still be absorbed to avoid GNOME overview tap")

	require.NoError(t, machine.FlushAbsorbedModifiers())
	assert.Equal(t, []OutputCommand{{Key: golibevdev.KeyLeftMeta, Action: KeyPress}}, mock.GetCommands())

	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyRelease)
	assert.Equal(t, []OutputCommand{
		{Key: golibevdev.KeyLeftMeta, Action: KeyPress},
		{Key: golibevdev.KeyLeftMeta, Action: KeyRelease},
	}, mock.GetCommands())
}

func TestMachine_PointerFlushedModifierRetractedByLaterMapping(t *testing.T) {
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})
	machine, mock := startModifierTestMachine(t, engine)

	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyPress)
	require.NoError(t, machine.FlushAbsorbedModifiers())
	sendKey(t, machine, golibevdev.KeyC, KeyPress)

	assert.Equal(t, []OutputCommand{
		{Key: golibevdev.KeyLeftMeta, Action: KeyPress},
		{Key: golibevdev.KeyLeftMeta, Action: KeyRelease},
		{Key: golibevdev.KeyLeftCtrl, Action: KeyPress},
		{Key: golibevdev.KeyC, Action: KeyPress},
	}, mock.GetCommands())

	sendKey(t, machine, golibevdev.KeyC, KeyRelease)
	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyRelease)
	assert.Empty(t, machine.GetOutputState().PressedKeys)
}

func TestMachine_FlushAbsorbedModifiersMultipleSorted(t *testing.T) {
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyLeftAlt, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})
	machine, mock := startModifierTestMachine(t, engine)

	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyPress)
	sendKey(t, machine, golibevdev.KeyLeftAlt, KeyPress)
	assert.Empty(t, mock.GetCommands())

	require.NoError(t, machine.FlushAbsorbedModifiers())
	assert.Equal(t, []OutputCommand{
		{Key: golibevdev.KeyLeftAlt, Action: KeyPress},
		{Key: golibevdev.KeyLeftMeta, Action: KeyPress},
	}, mock.GetCommands())
}

func TestMachine_FlushAbsorbedModifiersRecoversOutputFailure(t *testing.T) {
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})
	out := &ResettingOutputDevice{}
	machine := NewMachine(engine, out, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	sendKey(t, machine, golibevdev.KeyLeftMeta, KeyPress)
	out.failOnce = true
	require.NoError(t, machine.FlushAbsorbedModifiers())
	assert.False(t, machine.GetInputState().KeyStates[golibevdev.KeyLeftMeta].Pressed,
		"output reset during pointer flush must clear tracked input state")
}

func TestMachine_FlushAbsorbedModifiersWhenStopped(t *testing.T) {
	machine := NewMachine(NewSimpleMappingEngine(), &MockOutputDevice{}, DefaultConfig())
	require.NoError(t, machine.FlushAbsorbedModifiers())
}

func TestConfigStateMachineAppliesOptimisticModifierSettings(t *testing.T) {
	mock := &MockOutputDevice{}
	cfg := &config.Config{
		Mappings: []config.MappingRule{
			{
				Input:  []keys.Key{golibevdev.KeyLeftShift, golibevdev.KeyZ},
				Output: []keys.Key{golibevdev.KeyZ},
			},
			{
				Input:  []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyB},
				Output: []keys.Key{golibevdev.KeyLeft},
			},
		},
		Settings: config.Settings{
			OptimisticModifiers: []keys.Key{golibevdev.KeyLeftShift},
		},
	}

	sm := NewConfigStateMachine(mock, cfg, nil, DefaultConfig())
	require.NoError(t, sm.Start(context.Background()))
	defer sm.Stop()

	require.NoError(t, sm.ProcessEvent("kbd", golibevdev.KeyLeftShift, true))
	require.NoError(t, sm.ProcessEvent("kbd", golibevdev.KeyLeftCtrl, true))
	assert.Equal(t, []OutputCommand{{Key: golibevdev.KeyLeftShift, Action: KeyPress}}, mock.GetCommands(),
		"only configured Shift should be optimistic; Ctrl remains absorbed")
}
