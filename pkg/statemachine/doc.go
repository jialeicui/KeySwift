// Package statemachine provides a robust state machine architecture for key remapping.
//
// The architecture solves the "sticky key" problem through three key innovations:
//
// 1. Layered State Machines
//   - Input State Machine (ISM): Tracks physical input state
//   - Semantic State Machine (SSM): Transforms physical input into semantic meaning
//   - Output State Machine (OSM): Manages desired and actual output states
//
// 2. Explicit State Binding
//   - StateBinder creates explicit bindings between input and output keys
//   - When input keys are released, bound output keys are automatically released
//   - This prevents modifier keys from getting stuck
//
// 3. Speculative Execution
//   - Allows "pass-through" of modifier keys before confirming the combination
//   - If a combination is detected, compensates for the speculative operation
//   - If no combination is detected within the timeout, confirms the pass-through
//
// Basic Usage:
//
//	// Create components
//	output := &UInputDeviceAdapter{device: uinputDev}
//	engine := NewSimpleMappingEngine()
//	engine.AddMapping(MappingRule{
//		Input:  []keys.Key{golibevdev.KeyLeftMeta, golibevdev.KeyC},
//		Output: []keys.Key{golibevdev.KeyLeftCtrl, golibevdev.KeyC},
//	})
//
//	// Create and start state machine
//	machine := NewMachine(engine, output, DefaultConfig())
//	machine.Start(context.Background())
//	defer machine.Stop()
//
//	// Process events
//	machine.ProcessEvent(KeyEvent{
//		Key:       golibevdev.KeyLeftMeta,
//		Action:    KeyPress,
//		Timestamp: time.Now(),
//		Device:    "keyboard1",
//	})
//
// Architecture Overview:
//
//	┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
//	│  Event  │────▶│   ISM   │────▶│   SSM   │────▶│ Mapping │────▶│   OSM   │
//	│  Source │     │         │     │         │     │ Engine  │     │         │
//	└─────────┘     └─────────┘     └─────────┘     └─────────┘     └─────────┘
//	     │               │               │               │               │
//	     ▼               ▼               ▼               ▼               ▼
//	  Raw Event    Input State    Semantic State   Output Cmds    Device Output
//	(Physical)     (Physical)      (Semantic)       (Commands)      (Virtual)
//
// For more details, see the design document at docs/design/state-machine-architecture.md
package statemachine
