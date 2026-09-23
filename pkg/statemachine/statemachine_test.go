package statemachine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jialeicui/golibevdev"
	"github.com/jialeicui/keyswift/pkg/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockOutputDevice is a mock implementation of OutputDevice for testing
type MockOutputDevice struct {
	commands []OutputCommand
	events   []golibevdev.Event
}

func (m *MockOutputDevice) Execute(cmd OutputCommand) error {
	m.commands = append(m.commands, cmd)
	return nil
}

func (m *MockOutputDevice) ForwardEvent(ev golibevdev.Event) error {
	m.events = append(m.events, ev)
	return nil
}

func (m *MockOutputDevice) Sync() error {
	return nil
}

func (m *MockOutputDevice) Close() error {
	return nil
}

func (m *MockOutputDevice) GetCommands() []OutputCommand {
	return m.commands
}

func (m *MockOutputDevice) Clear() {
	m.commands = nil
}

type ResettingOutputDevice struct {
	commands []OutputCommand
	failOnce bool
}

func (r *ResettingOutputDevice) Execute(cmd OutputCommand) error {
	if r.failOnce {
		r.failOnce = false
		return ErrOutputResetRequired
	}
	r.commands = append(r.commands, cmd)
	return nil
}

func (r *ResettingOutputDevice) ForwardEvent(ev golibevdev.Event) error {
	return nil
}

func (r *ResettingOutputDevice) Sync() error {
	return nil
}

func (r *ResettingOutputDevice) Close() error {
	return nil
}

type fakeLowLevelOutput struct {
	writes   []OutputCommand
	events   []golibevdev.Event
	failNext error
	closed   bool
}

func (f *fakeLowLevelOutput) WriteKey(key KeyCode, value int32) error {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}

	action := KeyRelease
	if value == 1 {
		action = KeyPress
	}
	f.writes = append(f.writes, OutputCommand{Key: key, Action: action})
	return nil
}

func (f *fakeLowLevelOutput) WriteEvent(typ golibevdev.EventType, code golibevdev.EventCode, value int32) error {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}
	f.events = append(f.events, golibevdev.Event{Type: typ, Code: code, Value: value})
	return nil
}

func (f *fakeLowLevelOutput) Sync() error {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}
	return nil
}

func (f *fakeLowLevelOutput) Close() error {
	f.closed = true
	return nil
}

// Test ISM
func TestISM_ProcessEvent(t *testing.T) {
	ism := NewISM(DefaultSemanticConfig())

	// Test key press
	event := KeyEvent{
		Key:       golibevdev.KeyLeftMeta,
		Action:    KeyPress,
		Timestamp: time.Now(),
		Device:    "test-device",
	}

	state, changed := ism.ProcessEvent(event)
	require.True(t, changed)
	assert.True(t, state.KeyStates[golibevdev.KeyLeftMeta].Pressed)

	// Test key release
	event.Action = KeyRelease
	state, changed = ism.ProcessEvent(event)
	require.True(t, changed)
	assert.False(t, state.KeyStates[golibevdev.KeyLeftMeta].Pressed)
}

func TestISM_Debounce(t *testing.T) {
	config := DefaultSemanticConfig()
	config.DebounceThreshold = 50 * time.Millisecond
	ism := NewISM(config)

	now := time.Now()

	// First press
	event := KeyEvent{
		Key:       golibevdev.KeyA,
		Action:    KeyPress,
		Timestamp: now,
		Device:    "test-device",
	}
	_, changed := ism.ProcessEvent(event)
	assert.True(t, changed)

	// Immediate duplicate press (should be debounced)
	event.Timestamp = now.Add(1 * time.Millisecond)
	_, changed = ism.ProcessEvent(event)
	assert.False(t, changed)

	// Press after debounce threshold
	event.Timestamp = now.Add(100 * time.Millisecond)
	_, changed = ism.ProcessEvent(event)
	assert.True(t, changed)
}

func TestISM_GetAllPressedKeys(t *testing.T) {
	ism := NewISM(DefaultSemanticConfig())

	// Press multiple keys
	keys := []golibevdev.KeyEventCode{
		golibevdev.KeyLeftMeta,
		golibevdev.KeyC,
	}

	for _, key := range keys {
		event := KeyEvent{
			Key:       key,
			Action:    KeyPress,
			Timestamp: time.Now(),
			Device:    "test-device",
		}
		ism.ProcessEvent(event)
	}

	pressed := ism.GetAllPressedKeys()
	assert.Len(t, pressed, 2)
}

// Test SSM
func TestSSM_Translate(t *testing.T) {
	engine := NewSimpleMappingEngine()
	// Add a mapping so KeyLeftMeta is recognized as a potential modifier
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})
	ssm := NewSSM(DefaultSemanticConfig(), engine)

	// Create input state with modifier
	input := InputState{
		Timestamp: time.Now(),
		KeyStates: map[KeyCode]KeyState{
			golibevdev.KeyLeftMeta: {
				Pressed:   true,
				PressedAt: time.Now(),
			},
		},
	}

	semantic := ssm.Translate(input)

	// Should be Active (modifier has mapping rules)
	assert.Len(t, semantic.Modifiers.Active, 1)
	assert.Len(t, semantic.Independents, 0)
}

func TestSSM_Translate_LongHold(t *testing.T) {
	config := DefaultSemanticConfig()
	engine := NewSimpleMappingEngine()
	// Add a mapping so KeyLeftMeta is recognized as a potential modifier
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})
	ssm := NewSSM(config, engine)

	// Create input state with modifier pressed long ago
	input := InputState{
		Timestamp: time.Now(),
		KeyStates: map[KeyCode]KeyState{
			golibevdev.KeyLeftMeta: {
				Pressed:   true,
				PressedAt: time.Now().Add(-1 * time.Second),
			},
		},
	}

	semantic := ssm.Translate(input)

	// Should STILL be Active — mapped modifiers stay Active regardless of hold duration
	assert.Len(t, semantic.Modifiers.Active, 1)
	assert.Len(t, semantic.Independents, 0)
}

func TestSSM_UnmappedModifier(t *testing.T) {
	engine := NewSimpleMappingEngine()
	// Only add a mapping for KeyLeftMeta — KeyLeftCtrl has NO mapping
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})
	ssm := NewSSM(DefaultSemanticConfig(), engine)

	// Press KeyRightAlt which has no mapping rules
	input := InputState{
		Timestamp: time.Now(),
		KeyStates: map[KeyCode]KeyState{
			golibevdev.KeyRightAlt: {
				Pressed:   true,
				PressedAt: time.Now(),
			},
		},
	}

	semantic := ssm.Translate(input)

	// Should be Independent (no mapping rules for this modifier)
	assert.Len(t, semantic.Modifiers.Active, 0)
	assert.Len(t, semantic.Independents, 1)
}

// Test StateBinder
func TestStateBinder_CreateBinding(t *testing.T) {
	binder := NewStateBinder()

	inputKeys := []KeyCode{golibevdev.KeyLeftMeta, golibevdev.KeyC}
	outputKeys := []KeyCode{golibevdev.KeyLeftCtrl, golibevdev.KeyC}

	id := binder.CreateBinding(inputKeys, outputKeys, ReleaseOnAnyBoundKeyReleased)
	assert.NotEmpty(t, id)

	// Verify binding exists
	binding, ok := binder.GetBinding(id)
	require.True(t, ok)
	assert.Equal(t, inputKeys, binding.InputKeys)
	assert.Equal(t, outputKeys, binding.OutputKeys)
}

func TestStateBinder_OnInputKeyReleased(t *testing.T) {
	binder := NewStateBinder()

	inputKeys := []KeyCode{golibevdev.KeyLeftMeta, golibevdev.KeyC}
	outputKeys := []KeyCode{golibevdev.KeyLeftCtrl, golibevdev.KeyC}

	binder.CreateBinding(inputKeys, outputKeys, ReleaseOnAnyBoundKeyReleased)

	// Release one input key
	toRelease := binder.OnInputKeyReleased(golibevdev.KeyC)

	// Should release all output keys
	assert.Len(t, toRelease, 2)
	assert.Contains(t, toRelease, golibevdev.KeyLeftCtrl)
	assert.Contains(t, toRelease, golibevdev.KeyC)
}

// Test OSM
func TestOSM_ApplyCommands(t *testing.T) {
	mock := &MockOutputDevice{}
	binder := NewStateBinder()
	osm := NewOSM(binder, mock, DefaultAlignmentConfig())

	commands := []OutputCommand{
		{Key: golibevdev.KeyLeftCtrl, Action: KeyPress},
		{Key: golibevdev.KeyC, Action: KeyPress},
	}

	inputKeys := []KeyCode{golibevdev.KeyLeftMeta, golibevdev.KeyC}

	err := osm.ApplyCommands(commands, inputKeys, "test-mapping")
	require.NoError(t, err)

	// Should have executed press commands
	assert.Len(t, mock.GetCommands(), 2)

	// Verify desired state
	state := osm.GetDesiredState()
	assert.Len(t, state.PressedKeys, 2)
}

func TestOSM_HandleInputRelease(t *testing.T) {
	mock := &MockOutputDevice{}
	binder := NewStateBinder()
	osm := NewOSM(binder, mock, DefaultAlignmentConfig())

	// First apply some commands
	commands := []OutputCommand{
		{Key: golibevdev.KeyLeftCtrl, Action: KeyPress},
		{Key: golibevdev.KeyC, Action: KeyPress},
	}
	inputKeys := []KeyCode{golibevdev.KeyLeftMeta, golibevdev.KeyC}
	osm.ApplyCommands(commands, inputKeys, "test-mapping")

	mock.Clear()

	// Release an input key
	err := osm.HandleInputRelease(golibevdev.KeyC)
	require.NoError(t, err)

	// Should have released output keys
	cmds := mock.GetCommands()
	assert.Len(t, cmds, 2)
	assert.Equal(t, KeyRelease, cmds[0].Action)
	assert.Equal(t, KeyRelease, cmds[1].Action)
}

// Test SpeculativeExecutor
func TestSpeculativeExecutor_ExecuteAndConfirm(t *testing.T) {
	mock := &MockOutputDevice{}
	config := DefaultSpeculativeConfig()
	executor := NewSpeculativeExecutor(config, mock)

	cmd := OutputCommand{Key: golibevdev.KeyLeftMeta, Action: KeyPress}

	op, err := executor.ExecuteSpeculative(cmd)
	require.NoError(t, err)
	assert.NotNil(t, op)
	assert.Equal(t, SpeculativePending, op.Status)

	// Should have executed
	assert.Len(t, mock.GetCommands(), 1)

	// Confirm
	confirmed := executor.Confirm(golibevdev.KeyLeftMeta)
	assert.True(t, confirmed)
	assert.True(t, executor.IsConfirmed(golibevdev.KeyLeftMeta))
}

func TestSpeculativeExecutor_Compensate(t *testing.T) {
	mock := &MockOutputDevice{}
	config := DefaultSpeculativeConfig()
	executor := NewSpeculativeExecutor(config, mock)

	cmd := OutputCommand{Key: golibevdev.KeyLeftMeta, Action: KeyPress}

	_, err := executor.ExecuteSpeculative(cmd)
	require.NoError(t, err)

	mock.Clear()

	// Compensate
	err = executor.Compensate(golibevdev.KeyLeftMeta)
	require.NoError(t, err)

	// Should have sent release
	cmds := mock.GetCommands()
	require.Len(t, cmds, 1)
	assert.Equal(t, KeyRelease, cmds[0].Action)
}

// Test Integration
func TestMachine_BasicMapping(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()

	// Add a simple mapping: Cmd+C -> Ctrl+C
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	// Press Cmd
	event1 := KeyEvent{
		Key:       golibevdev.KeyLeftMeta,
		Action:    KeyPress,
		Timestamp: time.Now(),
		Device:    "test",
	}
	err := machine.ProcessEventSync(event1)
	require.NoError(t, err)

	// After pressing just the modifier, the machine may speculatively pass it through
	// This is expected behavior - clear the mock for the next phase
	mock.Clear()

	// Press C
	event2 := KeyEvent{
		Key:       golibevdev.KeyC,
		Action:    KeyPress,
		Timestamp: time.Now(),
		Device:    "test",
	}
	err = machine.ProcessEventSync(event2)
	require.NoError(t, err)

	// Should have output (may include speculative modifier pass-through)
	cmds := mock.GetCommands()
	assert.True(t, len(cmds) >= 2, "Should have at least 2 output commands, got %d", len(cmds))

	// Find the Ctrl+C press commands in the output
	var foundCtrlPress, foundCPress bool
	for _, cmd := range cmds {
		if cmd.Key == golibevdev.KeyLeftCtrl && cmd.Action == KeyPress {
			foundCtrlPress = true
		}
		if cmd.Key == golibevdev.KeyC && cmd.Action == KeyPress {
			foundCPress = true
		}
	}
	assert.True(t, foundCtrlPress, "Should have Ctrl press in output")
	assert.True(t, foundCPress, "Should have C press in output")
}

func TestMachine_StickyKeyPrevention(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()

	// Add mapping: Cmd+C -> Ctrl+C
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()

	// Press Cmd
	machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyLeftMeta,
		Action:    KeyPress,
		Timestamp: now,
		Device:    "test",
	})

	// Press C
	machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyC,
		Action:    KeyPress,
		Timestamp: now.Add(10 * time.Millisecond),
		Device:    "test",
	})

	mock.Clear()

	// Release C
	machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyC,
		Action:    KeyRelease,
		Timestamp: now.Add(20 * time.Millisecond),
		Device:    "test",
	})

	// Should release C
	cmds := mock.GetCommands()
	assert.True(t, len(cmds) >= 1)

	// Release Cmd
	machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyLeftMeta,
		Action:    KeyRelease,
		Timestamp: now.Add(30 * time.Millisecond),
		Device:    "test",
	})

	// Should release Ctrl
	cmds = mock.GetCommands()
	foundCtrlRelease := false
	for _, cmd := range cmds {
		if cmd.Key == golibevdev.KeyLeftCtrl && cmd.Action == KeyRelease {
			foundCtrlRelease = true
			break
		}
	}
	assert.True(t, foundCtrlRelease, "Ctrl should be released to prevent sticky key")
}

func TestMachine_HandleDeviceLostReleasesMappedOutput(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyLeftMeta,
		Action:    KeyPress,
		Timestamp: now,
		Device:    "kbd-1",
	}))
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyC,
		Action:    KeyPress,
		Timestamp: now.Add(10 * time.Millisecond),
		Device:    "kbd-1",
	}))

	mock.Clear()
	require.NoError(t, machine.HandleDeviceLost("kbd-1"))

	cmds := mock.GetCommands()
	assert.Contains(t, cmds, OutputCommand{Key: golibevdev.KeyLeftCtrl, Action: KeyRelease})
	assert.Contains(t, cmds, OutputCommand{Key: golibevdev.KeyC, Action: KeyRelease})
	assert.Empty(t, machine.GetInputState().KeyStates)
}

func TestMachine_HandleDeviceLostReleasesPassthroughKey(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyA,
		Action:    KeyPress,
		Timestamp: time.Now(),
		Device:    "kbd-1",
	}))

	mock.Clear()
	require.NoError(t, machine.HandleDeviceLost("kbd-1"))
	assert.Contains(t, mock.GetCommands(), OutputCommand{Key: golibevdev.KeyA, Action: KeyRelease})
}

func TestMachine_HandleDeviceLostReleasesFlushedModifier(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyLeftMeta,
		Action:    KeyPress,
		Timestamp: now,
		Device:    "kbd-1",
	}))
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyA,
		Action:    KeyPress,
		Timestamp: now.Add(10 * time.Millisecond),
		Device:    "kbd-1",
	}))

	mock.Clear()
	require.NoError(t, machine.HandleDeviceLost("kbd-1"))

	cmds := mock.GetCommands()
	assert.Contains(t, cmds, OutputCommand{Key: golibevdev.KeyA, Action: KeyRelease})
	assert.Contains(t, cmds, OutputCommand{Key: golibevdev.KeyLeftMeta, Action: KeyRelease})
}

func TestMachine_ResetsStateAfterOutputFailure(t *testing.T) {
	output := &ResettingOutputDevice{failOnce: true}
	engine := NewSimpleMappingEngine()
	machine := NewMachine(engine, output, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyA,
		Action:    KeyPress,
		Timestamp: time.Now(),
		Device:    "kbd-1",
	}))

	assert.Empty(t, machine.GetInputState().KeyStates)
	assert.Empty(t, machine.GetSemanticState().Combo.ActiveKeys)
	assert.Empty(t, machine.GetOutputState().PressedKeys)

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key:       golibevdev.KeyA,
		Action:    KeyPress,
		Timestamp: time.Now().Add(10 * time.Millisecond),
		Device:    "kbd-1",
	}))

	assert.Contains(t, output.commands, OutputCommand{Key: golibevdev.KeyA, Action: KeyPress})
}

func TestRecoveringOutputDeviceRebuildsAfterFailure(t *testing.T) {
	first := &fakeLowLevelOutput{failNext: errors.New("write failed")}
	second := &fakeLowLevelOutput{}
	callCount := 0

	device, err := newRecoveringOutputDevice("keyswift-test", func(name string) (lowLevelOutputDevice, error) {
		callCount++
		if callCount == 1 {
			return first, nil
		}
		return second, nil
	})
	require.NoError(t, err)

	err = device.Execute(OutputCommand{Key: golibevdev.KeyA, Action: KeyPress})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOutputResetRequired)
	assert.True(t, first.closed)

	require.NoError(t, device.Execute(OutputCommand{Key: golibevdev.KeyA, Action: KeyRelease}))
	assert.Contains(t, second.writes, OutputCommand{Key: golibevdev.KeyA, Action: KeyRelease})
	require.NoError(t, device.Close())
}

func TestRecoveringOutputDeviceForwardsEvents(t *testing.T) {
	low := &fakeLowLevelOutput{}
	device, err := newRecoveringOutputDevice("keyswift-test", func(name string) (lowLevelOutputDevice, error) {
		return low, nil
	})
	require.NoError(t, err)

	ev := golibevdev.Event{Type: golibevdev.EvRel, Code: golibevdev.RelX, Value: -3}
	require.NoError(t, device.ForwardEvent(ev))
	require.Len(t, low.events, 1)
	assert.Equal(t, ev.Type, low.events[0].Type)
	assert.Equal(t, ev.Code, low.events[0].Code)
	assert.Equal(t, ev.Value, low.events[0].Value)
	require.NoError(t, device.Close())
}

func TestRecoveringOutputDeviceRebuildsAfterForwardFailure(t *testing.T) {
	first := &fakeLowLevelOutput{failNext: errors.New("write failed")}
	second := &fakeLowLevelOutput{}
	callCount := 0

	device, err := newRecoveringOutputDevice("keyswift-test", func(name string) (lowLevelOutputDevice, error) {
		callCount++
		if callCount == 1 {
			return first, nil
		}
		return second, nil
	})
	require.NoError(t, err)

	err = device.ForwardEvent(golibevdev.Event{Type: golibevdev.EvRel, Code: golibevdev.RelY, Value: 1})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOutputResetRequired)
	assert.True(t, first.closed)

	require.NoError(t, device.ForwardEvent(golibevdev.Event{Type: golibevdev.EvRel, Code: golibevdev.RelY, Value: 2}))
	require.Len(t, second.events, 1)
	assert.Equal(t, int32(2), second.events[0].Value)
	require.NoError(t, device.Close())
}

func TestRingBuffer(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)

	assert.Equal(t, 3, rb.Len())

	// Push beyond capacity
	rb.Push(4)

	assert.Equal(t, 3, rb.Len())

	// Check order (oldest first)
	slice := rb.Slice()
	assert.Equal(t, []int{2, 3, 4}, slice)

	// Pop
	val, ok := rb.Pop()
	assert.True(t, ok)
	assert.Equal(t, 2, val)
	assert.Equal(t, 2, rb.Len())
}

func TestIsModifier(t *testing.T) {
	assert.True(t, IsModifier(golibevdev.KeyLeftCtrl))
	assert.True(t, IsModifier(golibevdev.KeyRightCtrl))
	assert.True(t, IsModifier(golibevdev.KeyLeftAlt))
	assert.True(t, IsModifier(golibevdev.KeyLeftMeta))
	assert.True(t, IsModifier(golibevdev.KeyLeftShift))

	assert.False(t, IsModifier(golibevdev.KeyA))
	assert.False(t, IsModifier(golibevdev.KeySpace))
	assert.False(t, IsModifier(golibevdev.KeyEnter))
}

// =============================================================================
// Stuck-key regression tests
//
// Cover the bug where a release event triggered a re-match against the
// remaining ism keys, ran handleMapping, and returned without forwarding
// the release of the just-released key. If that key had been forwarded as
// passthrough, it stayed pressed on the virtual keyboard forever — the
// "permanently held key" symptom users reported. See machine.go
// handleEvent / handleReleaseEvent for the corresponding fix.
// =============================================================================

// TestMachine_PassthroughKeyReleaseDoesNotStickOnOS reproduces the original
// bug. Pre-fix the release event for X (forwarded as passthrough) was eaten
// by the matched-on-release path and X remained pressed on the OS forever.
func TestMachine_PassthroughKeyReleaseDoesNotStickOnOS(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()

	// Press Cmd → absorbed.
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyLeftMeta, Action: KeyPress, Timestamp: now, Device: "kbd",
	}))
	// Press X → no Cmd+X rule, flushes Cmd, forwards X via passthrough.
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyX, Action: KeyPress,
		Timestamp: now.Add(10 * time.Millisecond), Device: "kbd",
	}))
	// Press C → no Cmd+X+C rule, forwards C via passthrough.
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyC, Action: KeyPress,
		Timestamp: now.Add(20 * time.Millisecond), Device: "kbd",
	}))

	mock.Clear()

	// Release X. Pre-fix: ism keys after release are {Cmd, C}; the engine
	// matches Cmd+C and the X release is dropped.
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyX, Action: KeyRelease,
		Timestamp: now.Add(30 * time.Millisecond), Device: "kbd",
	}))

	cmds := mock.GetCommands()
	foundXRelease := false
	for _, cmd := range cmds {
		if cmd.Key == golibevdev.KeyX && cmd.Action == KeyRelease {
			foundXRelease = true
			break
		}
	}
	assert.True(t, foundXRelease,
		"release of passthrough X must reach the OS even when remaining keys would re-match; got %+v", cmds)
}

// TestMachine_FlushedModifierReleaseForwardsToOS verifies that releasing a
// Flushed modifier forwards the release to OS and cleans up tracking state.
func TestMachine_FlushedModifierReleaseForwardsToOS(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyLeftMeta, Action: KeyPress, Timestamp: now, Device: "kbd",
	}))
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyX, Action: KeyPress,
		Timestamp: now.Add(10 * time.Millisecond), Device: "kbd",
	}))

	machine.mu.RLock()
	state, exists := machine.modifierStates[golibevdev.KeyLeftMeta]
	machine.mu.RUnlock()
	require.True(t, exists, "Cmd should be tracked")
	require.Equal(t, ModifierStateFlushed, state, "press of unmapped X should flush Cmd")

	mock.Clear()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyLeftMeta, Action: KeyRelease,
		Timestamp: now.Add(20 * time.Millisecond), Device: "kbd",
	}))

	cmds := mock.GetCommands()
	require.Contains(t, cmds, OutputCommand{Key: golibevdev.KeyLeftMeta, Action: KeyRelease},
		"flushed Cmd release must be forwarded to OS; got %+v", cmds)

	machine.mu.RLock()
	_, modStillSet := machine.modifierStates[golibevdev.KeyLeftMeta]
	_, ptStillSet := machine.passthroughPressed[golibevdev.KeyLeftMeta]
	machine.mu.RUnlock()
	assert.False(t, modStillSet, "modifierStates entry for Cmd should be deleted")
	assert.False(t, ptStillSet, "passthroughPressed entry for Cmd should be deleted")
}

// TestMachine_IndependentModifierReleasesOnOS covers the path where a
// modifier is not used in any rule (so SSM classifies it Independent), is
// passthrough-pressed, and must get its release forwarded.
func TestMachine_IndependentModifierReleasesOnOS(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyRightAlt, Action: KeyPress, Timestamp: now, Device: "kbd",
	}))
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyRightAlt, Action: KeyRelease,
		Timestamp: now.Add(10 * time.Millisecond), Device: "kbd",
	}))

	cmds := mock.GetCommands()
	require.Contains(t, cmds, OutputCommand{Key: golibevdev.KeyRightAlt, Action: KeyPress})
	require.Contains(t, cmds, OutputCommand{Key: golibevdev.KeyRightAlt, Action: KeyRelease})
}

// TestMachine_AbsorbedModifierReleaseStillTaps verifies that the
// "modifier-tap" semantic (pressing a mapped modifier alone and releasing
// it produces a press+release pair on OS, e.g. Wayland Meta → Activities)
// survives the release-path refactor.
func TestMachine_AbsorbedModifierReleaseStillTaps(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyLeftMeta, Action: KeyPress, Timestamp: now, Device: "kbd",
	}))
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyLeftMeta, Action: KeyRelease,
		Timestamp: now.Add(10 * time.Millisecond), Device: "kbd",
	}))

	assert.Equal(t,
		[]OutputCommand{
			{Key: golibevdev.KeyLeftMeta, Action: KeyPress},
			{Key: golibevdev.KeyLeftMeta, Action: KeyRelease},
		},
		mock.GetCommands(),
		"absorbed modifier should tap on solo release",
	)
}

// TestMachine_ReleaseDoesNotTriggerNewMapping locks in the design choice
// of disabling mapping match on release events. With both Cmd+Shift+Z and
// Cmd+Z configured, releasing Shift mid-Cmd+Shift+Z must NOT fire Cmd+Z.
func TestMachine_ReleaseDoesNotTriggerNewMapping(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input: []keys.Key{
			golibevdev.KeyLeftMeta,
			golibevdev.KeyLeftShift,
			golibevdev.KeyZ,
		},
		Output: []keys.Key{
			golibevdev.KeyLeftCtrl,
			golibevdev.KeyLeftShift,
			golibevdev.KeyZ,
		},
	})
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyZ},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyZ},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyLeftMeta, Action: KeyPress, Timestamp: now, Device: "kbd",
	}))
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyLeftShift, Action: KeyPress,
		Timestamp: now.Add(10 * time.Millisecond), Device: "kbd",
	}))
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyZ, Action: KeyPress,
		Timestamp: now.Add(20 * time.Millisecond), Device: "kbd",
	}))

	mock.Clear()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyLeftShift, Action: KeyRelease,
		Timestamp: now.Add(30 * time.Millisecond), Device: "kbd",
	}))

	cmds := mock.GetCommands()
	for _, cmd := range cmds {
		if cmd.Action == KeyPress {
			t.Errorf("no KeyPress should be emitted after release Shift, got %+v in %+v", cmd, cmds)
		}
	}
}

// TestMachine_NoSpuriousFlushAfterPassthroughRelease checks the modifier-
// state cleanup side-effect of the fix: after the stuck-key sequence runs
// and is properly cleaned up, a fresh non-modifier press must not trigger
// a phantom modifier flush from a stale Absorbed entry.
func TestMachine_NoSpuriousFlushAfterPassthroughRelease(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()

	for i, ev := range []KeyEvent{
		{Key: golibevdev.KeyLeftMeta, Action: KeyPress},
		{Key: golibevdev.KeyX, Action: KeyPress},
		{Key: golibevdev.KeyC, Action: KeyPress},
		{Key: golibevdev.KeyX, Action: KeyRelease},
		{Key: golibevdev.KeyC, Action: KeyRelease},
		{Key: golibevdev.KeyLeftMeta, Action: KeyRelease},
	} {
		ev.Timestamp = now.Add(time.Duration(10*(i+1)) * time.Millisecond)
		ev.Device = "kbd"
		require.NoError(t, machine.ProcessEventSync(ev))
	}

	machine.mu.RLock()
	leftover := len(machine.modifierStates)
	machine.mu.RUnlock()
	require.Zero(t, leftover, "modifierStates should be empty after sequence")

	mock.Clear()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyY, Action: KeyPress,
		Timestamp: now.Add(100 * time.Millisecond), Device: "kbd",
	}))

	assert.Equal(t,
		[]OutputCommand{{Key: golibevdev.KeyY, Action: KeyPress}},
		mock.GetCommands(),
		"press Y after sequence should emit only Y press, not a phantom modifier",
	)
}

// TestMachine_RepeatedSameMappingDoesNotAccumulateBindings exercises the
// HasMatchingActiveBinding fast path. Repeating the same combo (e.g. via
// autorepeat past the debounce threshold) must not accumulate active
// bindings.
func TestMachine_RepeatedSameMappingDoesNotAccumulateBindings(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()
	engine.AddMapping(MappingRule{
		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
	})

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	now := time.Now()

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyLeftMeta, Action: KeyPress, Timestamp: now, Device: "kbd",
	}))
	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyC, Action: KeyPress,
		Timestamp: now.Add(10 * time.Millisecond), Device: "kbd",
	}))

	require.Len(t, machine.binder.GetActiveBindings(), 1,
		"first Cmd+C press should create exactly one active binding")

	for i := 1; i <= 5; i++ {
		require.NoError(t, machine.ProcessEventSync(KeyEvent{
			Key: golibevdev.KeyC, Action: KeyPress,
			Timestamp: now.Add(time.Duration(20+i*10) * time.Millisecond),
			Device:    "kbd",
		}))
		assert.Len(t, machine.binder.GetActiveBindings(), 1,
			"iteration %d: active binding count should remain 1", i)
	}

	require.NoError(t, machine.ProcessEventSync(KeyEvent{
		Key: golibevdev.KeyC, Action: KeyRelease,
		Timestamp: now.Add(200 * time.Millisecond), Device: "kbd",
	}))
	assert.Empty(t, machine.binder.GetActiveBindings(),
		"release C should clean up all active bindings")
}

// TestMachine_ConcurrentDevicesNoRace exercises eventMu serialization.
// With -race enabled, this would have flagged interleaved access to
// ism/osm/modifierStates without the lock. Functionally we just check
// that emitted press/release pairs balance.
func TestMachine_ConcurrentDevicesNoRace(t *testing.T) {
	mock := &MockOutputDevice{}
	engine := NewSimpleMappingEngine()

	machine := NewMachine(engine, mock, DefaultConfig())
	require.NoError(t, machine.Start(context.Background()))
	defer machine.Stop()

	const iterations = 50
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ts := time.Now()
			_ = machine.ProcessEvent(KeyEvent{
				Key: golibevdev.KeyA, Action: KeyPress, Timestamp: ts, Device: "kbd-A",
			})
			_ = machine.ProcessEvent(KeyEvent{
				Key: golibevdev.KeyA, Action: KeyRelease,
				Timestamp: ts.Add(time.Millisecond), Device: "kbd-A",
			})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ts := time.Now()
			_ = machine.ProcessEvent(KeyEvent{
				Key: golibevdev.KeyB, Action: KeyPress, Timestamp: ts, Device: "kbd-B",
			})
			_ = machine.ProcessEvent(KeyEvent{
				Key: golibevdev.KeyB, Action: KeyRelease,
				Timestamp: ts.Add(time.Millisecond), Device: "kbd-B",
			})
		}
	}()
	wg.Wait()

	pressA, releaseA := 0, 0
	pressB, releaseB := 0, 0
	for _, cmd := range mock.GetCommands() {
		switch {
		case cmd.Key == golibevdev.KeyA && cmd.Action == KeyPress:
			pressA++
		case cmd.Key == golibevdev.KeyA && cmd.Action == KeyRelease:
			releaseA++
		case cmd.Key == golibevdev.KeyB && cmd.Action == KeyPress:
			pressB++
		case cmd.Key == golibevdev.KeyB && cmd.Action == KeyRelease:
			releaseB++
		}
	}
	assert.Equal(t, pressA, releaseA, "KeyA press/release counts: %d vs %d", pressA, releaseA)
	assert.Equal(t, pressB, releaseB, "KeyB press/release counts: %d vs %d", pressB, releaseB)
}
