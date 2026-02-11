package statemachine

import (
	"context"
	"errors"
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
}

func (m *MockOutputDevice) Execute(cmd OutputCommand) error {
	m.commands = append(m.commands, cmd)
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

func (r *ResettingOutputDevice) Sync() error {
	return nil
}

func (r *ResettingOutputDevice) Close() error {
	return nil
}

type fakeLowLevelOutput struct {
	writes   []OutputCommand
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
