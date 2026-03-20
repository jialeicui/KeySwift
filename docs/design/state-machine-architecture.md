# KeySwift State Machine Architecture Design

## Table of Contents

1. [Background and Problem Definition](#1-background-and-problem-definition)
2. [Core Challenges](#2-core-challenges)
3. [Special Cases](#3-special-cases)
4. [Architecture Design](#4-architecture-design)
5. [Detailed Design](#5-detailed-design)
6. [State Transitions](#6-state-transitions)
7. [Implementation Roadmap](#7-implementation-roadmap)

---

## 1. Background and Problem Definition

### 1.1 Problems in Current Architecture

KeySwift's existing event-driven architecture has fundamental flaws when handling complex input scenarios:

```
Current Architecture:
Physical Keyboard → Event Capture → JavaScript Decision → Direct Output → Virtual Device
                        ↓
                    No State Management
```

**Core Issues**:
- **State Desynchronization**: Physical key states and output device states are not modeled uniformly
- **Race Conditions**: Event ordering cannot be guaranteed during rapid key combinations
- **Sticky Keys**: Modifier keys (Ctrl/Alt/Super) easily enter a "continuously pressed" state after remapping
- **Intent Ambiguity**: Cannot distinguish between "pressing Super alone" and "Super+C combination" intents

### 1.2 Target Scenarios

#### Scenario A: Basic Mapping
```
User Action: Cmd + C
System Output: Ctrl + C
```

#### Scenario B: Delayed Combination (Critical Scenario)
```
T0:     Press Cmd
T+0.5s: Press C

Requirements:
- During T0~T+0.5s, Cmd is available for mouse operations (e.g., Cmd+Click)
- At T+0.5s, convert to Ctrl+C
- Conversion is transparent to the user
```

#### Scenario C: Rapid Switching
```
User Action: Cmd+C (quick) → Release C immediately → Press V immediately
Requirements:
- Ctrl+C triggers correctly
- Ctrl doesn't get stuck, preventing accidental Ctrl+V trigger
```

#### Scenario D: Mixed Scenario
```
User Actions:
1. Hold Cmd (preparing for mouse operation)
2. Change mind, press C (want to copy)
3. Release C
4. Continue using Cmd+mouse operation

Requirements:
- Step 2 correctly triggers Ctrl+C
- Step 4 Cmd remains effective
```

---

## 2. Core Challenges

### 2.1 Challenge 1: The Delayed Decision Paradox

```
Problem Essence:
- When Cmd is pressed, the system doesn't know user intent
- If pass-through immediately, subsequent conversion needs to "reclaim" the sent key
- If no pass-through, standalone Cmd scenario fails

Deficiencies in Traditional Solutions:
1. Timeout Mechanism: Set fixed threshold (e.g., 200ms)
   - Problem: Threshold is hard to determine, varies by user habit
   
2. Compensation Mechanism: Send release then resend
   - Problem: User is still holding the key, causing physical/system state mismatch
```

### 2.2 Challenge 2: State Consistency

```
Problem Essence:
Physical World      System State
   Cmd pressed  →    Cmd released (after compensation)
   
When user releases Cmd:
- System receives Release event
- But system thinks Cmd is already released
- Causes state tracking chaos
```

### 2.3 Challenge 3: Atomicity Guarantee

```
Problem Essence:
Key combination mapping is not atomic:
Cmd↓ C↓ → [Convert] → Ctrl↓ C↓ C↑ Ctrl↑

Any step failure or interruption in between leads to sticky keys.
```

### 2.4 Challenge 4: Cross-Device Synchronization

```
Problem Essence:
- Multiple physical keyboards input simultaneously
- Virtual output device needs to synchronize with physical device states
- System-level shortcuts (e.g., Ctrl+Alt+T) need special handling
```

---

## 3. Special Cases

### 3.1 Case 1: Repeat Trigger Protection

```
User Action:
Cmd↓ C↓ C↑ C↓ C↑ ... (rapid consecutive presses)

Risk:
- Each C↓ triggers a new mapping
- Ctrl is repeatedly pressed/released, potentially causing state chaos

Solution:
- Same combination only triggers once before Cmd is released
- Or: Support repeat triggering but maintain state consistency
```

### 3.2 Case 2: Nested Combinations

```
User Action:
Cmd↓ Shift↓ C↓

Mapping Configuration:
- Cmd+C → Ctrl+C
- Cmd+Shift+C → Ctrl+Shift+C

Challenge:
- Need to handle hierarchical relationships of combinations
- Avoid false triggers from partial matching
```

### 3.3 Case 3: Partial Release

```
User Action:
Cmd↓ C↓ → Release Cmd (while holding C) → Release C

Risk:
- After Ctrl+C triggers, Cmd release should release Ctrl
- But C is still held, potentially causing C to stick

Solution:
- Binding relationship tracking: Ctrl binds to [Cmd, C]
- Release triggered when any bound key is released
```

### 3.4 Case 4: System-level Shortcut Passthrough

```
Scenario:
User configures Cmd+Space to map to Ctrl+Space
But system needs Cmd+Space to switch input methods

Solution:
- Whitelist mechanism: Certain combinations are never mapped
- Or: Send original combination after mapping (compatibility mode)
```

### 3.5 Case 5: Multi-keyboard Input

```
Scenario:
- Keyboard A presses Cmd
- Keyboard B presses C

Challenge:
- Requires global state management, not per-device
- Avoid one keyboard's release affecting another keyboard's state
```

---

## 4. Architecture Design

### 4.1 Core Philosophy

**Layered State Machines + Explicit State Binding + Speculative Execution**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           KeySwift State Machine                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────────────┐ │
│  │   Input Layer   │───▶│  Semantic Layer │───▶│    Mapping Engine       │ │
│  │ (Physical Input)│    │ (Semantic Abstr)│    │    (Mapping Decision)   │ │
│  └─────────────────┘    └─────────────────┘    └─────────────────────────┘ │
│           │                      │                        │                 │
│           ▼                      ▼                        ▼                 │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────────────┐ │
│  │  Input State    │    │ Semantic State  │    │   Output Commands       │ │
│  │   Machine       │    │    Machine      │    │   (State Transition)    │ │
│  │   (ISM)         │    │    (SSM)        │    │                         │ │
│  └─────────────────┘    └─────────────────┘    └─────────────────────────┘ │
│                                                                             │
│  ═══════════════════════════════════════════════════════════════════════   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Output State Machine (OSM)                      │   │
│  │                                                                      │   │
│  │  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐          │   │
│  │  │   Desired    │───▶│   Aligner    │───▶│   Actual     │          │   │
│  │  │    State     │    │(State Align) │    │    State     │          │   │
│  │  └──────────────┘    └──────────────┘    └──────────────┘          │   │
│  │         │                                     │                    │   │
│  │         ▼                                     ▼                    │   │
│  │  ┌─────────────────────────────────────────────────────────────┐   │   │
│  │  │              State Binder (State Binding)                    │   │   │
│  │  │  • Output Key ↔ Input Key Binding                            │   │   │
│  │  │  • Lifecycle Management                                      │   │   │
│  │  │  • Auto-release Tracking                                     │   │   │
│  │  └─────────────────────────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ═══════════════════════════════════════════════════════════════════════   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Speculative Executor                              │   │
│  │                                                                      │   │
│  │  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐          │   │
│  │  │ Speculative  │───▶│ Confirmation │───▶│ Compensation │          │   │
│  │  │    Layer     │    │   Window     │    │   Engine     │          │   │
│  │  └──────────────┘    └──────────────┘    └──────────────┘          │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Key Components

#### 4.2.1 Input State Machine (ISM)

```go
type ISM struct {
    // Current physical state
    current InputState
    
    // Historical states (for debounce detection, double-tap recognition, etc.)
    history *RingBuffer[InputState]
    
    // Multi-device aggregation
    devices map[DeviceID]*DeviceState
}

type InputState struct {
    Timestamp   time.Time
    KeyStates   map[KeyCode]KeyState
    
    // Raw event sequence (order-preserving)
    EventSeq    []KeyEvent
}

type KeyState struct {
    Pressed     bool
    PressedAt   time.Time
    Device      DeviceID
    
    // For debounce elimination
    PressCount  int  // Number of presses in short time
}
```

#### 4.2.2 Semantic State Machine (SSM)

```go
type SSM struct {
    // Semantic state after transformation
    current SemanticState
    
    // Configuration parameters
    config SemanticConfig
}

type SemanticState struct {
    // Modifier semantics (for combination shortcuts)
    Modifiers ModifierSemantic
    
    // Independent key semantics (for mouse operations, etc.)
    Independents map[KeyCode]IndependentState
    
    // Combination state
    Combo ComboState
}

type ModifierSemantic struct {
    // Which keys currently act as modifiers
    Active map[KeyCode]ModifierInfo
    
    // Which keys are in "pending" state (can become independent)
    Pending map[KeyCode]*PendingModifier
}

type PendingModifier struct {
    Key           KeyCode
    PressedAt     time.Time
    
    // Key: Downgrade deadline
    DowngradeDeadline time.Time
    
    // Whether already passed-through
    PassedThrough bool
}

type IndependentState struct {
    Key           KeyCode
    IsPressed     bool
    
    // Flag: Whether downgraded from Pending
    DowngradedFrom bool
}
```

#### 4.2.3 Output State Machine (OSM)

```go
type OSM struct {
    // Desired state (determined by mapping engine)
    desired OutputState
    
    // Actual state (already sent to virtual device)
    actual OutputState
    
    // State binder
    binder *StateBinder
    
    // Aligner
    aligner *StateAligner
    
    // Output device
    output OutputDevice
}

type OutputState struct {
    PressedKeys map[KeyCode]OutputKeyInfo
}

type OutputKeyInfo struct {
    PressedAt       time.Time
    SourceMapping   MappingID
    
    // Core: Which input keys this binds to
    BoundTo         []KeyCode
    
    // Release strategy
    ReleaseStrategy ReleaseStrategy
}

type ReleaseStrategy int
const (
    ReleaseOnAnyBoundKeyReleased ReleaseStrategy = iota
    ReleaseOnAllBoundKeysReleased
    ReleaseOnTimeout
)
```

#### 4.2.4 State Binder (Core Innovation)

```go
type StateBinder struct {
    // Input key → Output reverse index
    // For fast lookup: when an input key is released, which outputs need release
    inputToOutput map[KeyCode]map[KeyCode]struct{}
    
    // Binding relationship graph
    bindings map[BindingID]*Binding
}

type Binding struct {
    ID          BindingID
    InputKeys   []KeyCode
    OutputKeys  []KeyCode
    
    // Creation time (for debugging)
    CreatedAt   time.Time
    
    // Binding status
    Status      BindingStatus
}

// When input key is released, automatically release bound output keys
func (sb *StateBinder) OnInputKeyReleased(key KeyCode) []KeyCode {
    affectedOutputs := sb.inputToOutput[key]
    
    var toRelease []KeyCode
    for outKey := range affectedOutputs {
        binding := sb.findBinding(outKey)
        
        // Check release conditions
        if sb.shouldRelease(binding, key) {
            toRelease = append(toRelease, outKey)
            sb.unbind(outKey)
        }
    }
    
    return toRelease
}
```

#### 4.2.5 Speculative Executor

```go
type SpeculativeExecutor struct {
    // Speculative layer
    layer *SpeculativeLayer
    
    // Compensation engine
    compensator *Compensator
    
    // Confirmation window configuration
    config SpeculativeConfig
}

type SpeculativeLayer struct {
    // Pending speculative operations
    pending map[KeyCode]*SpeculativeOp
    
    // Confirmed operations (irrevocable)
    confirmed map[KeyCode]*SpeculativeOp
}

type SpeculativeOp struct {
    ID            OpID
    Key           KeyCode
    Action        KeyAction  // Press or Release
    
    // Execution time
    ExecutedAt    time.Time
    
    // Confirmation deadline
    ConfirmDeadline time.Time
    
    // Status
    Status        SpeculativeStatus
    
    // Compensation strategy
    Compensation  CompensationStrategy
}

type CompensationStrategy struct {
    // How to revoke this operation
    RevertActions []OutputCommand
    
    // Alignment actions after revocation
    AlignmentActions []OutputCommand
}
```

---

## 5. Detailed Design

### 5.1 Data Flow

```
┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
│  Event  │────▶│   ISM   │────▶│   SSM   │────▶│ Mapping │────▶│   OSM   │
│  Source │     │         │     │         │     │ Engine  │     │         │
└─────────┘     └─────────┘     └─────────┘     └─────────┘     └─────────┘
     │               │               │               │               │
     │               │               │               │               │
     ▼               ▼               ▼               ▼               ▼
  Raw Event    Input State    Semantic State   Output Cmds    Device Output
(Physical)     (Physical)      (Semantic)       (Commands)      (Virtual)
```

### 5.2 Key Algorithms

#### 5.2.1 Semantic Translation Algorithm

```go
func (ssm *SSM) Translate(input InputState) SemanticState {
    semantic := SemanticState{
        Modifiers: ModifierSemantic{
            Active:  make(map[KeyCode]ModifierInfo),
            Pending: make(map[KeyCode]*PendingModifier),
        },
        Independents: make(map[KeyCode]IndependentState),
    }
    
    for key, state := range input.KeyStates {
        if !isModifier(key) {
            // Non-modifier keys pass through directly
            semantic.Combo.ActiveKeys = append(semantic.Combo.ActiveKeys, key)
            continue
        }
        
        // Check for potential matches
        potentialMatches := ssm.mappingEngine.PrefixMatch([]KeyCode{key})
        
        if len(potentialMatches) == 0 {
            // No potential matches, treat as independent key
            semantic.Independents[key] = IndependentState{
                Key:       key,
                IsPressed: true,
            }
            continue
        }
        
        // Calculate hold duration
        holdDuration := time.Since(state.PressedAt)
        
        if holdDuration < ssm.config.ModifierPendingThreshold {
            // Within threshold, mark as Pending
            semantic.Modifiers.Pending[key] = &PendingModifier{
                Key:               key,
                PressedAt:         state.PressedAt,
                DowngradeDeadline: state.PressedAt.Add(ssm.config.ModifierPendingThreshold),
                PassedThrough:     false,  // Don't pass-through yet
            }
        } else {
            // Exceeds threshold, convert to independent key
            semantic.Independents[key] = IndependentState{
                Key:            key,
                IsPressed:      true,
                DowngradedFrom: true,
            }
        }
    }
    
    return semantic
}
```

#### 5.2.2 Speculative Execution and Compensation

```go
func (se *SpeculativeExecutor) ExecuteSpeculative(cmd OutputCommand) error {
    // 1. Execute operation
    se.output.Execute(cmd)
    
    // 2. Create speculative operation record
    op := &SpeculativeOp{
        ID:              generateID(),
        Key:             cmd.Key,
        Action:          cmd.Action,
        ExecutedAt:      time.Now(),
        ConfirmDeadline: time.Now().Add(se.config.ConfirmationWindow),
        Status:          SpeculativePending,
        Compensation:    se.buildCompensation(cmd),
    }
    
    // 3. Add to pending queue
    se.layer.pending[cmd.Key] = op
    
    // 4. Start confirmation timer
    se.startConfirmationTimer(op)
    
    return nil
}

func (se *SpeculativeExecutor) ConfirmOrCompensate(key KeyCode, matchFound bool) error {
    op, ok := se.layer.pending[key]
    if !ok {
        return nil  // Not a speculative operation
    }
    
    if matchFound {
        // Confirm: convert to irrevocable
        op.Status = SpeculativeConfirmed
        se.layer.confirmed[key] = op
        delete(se.layer.pending, key)
        return nil
    }
    
    // Needs compensation
    return se.compensate(op)
}

func (se *SpeculativeExecutor) compensate(op *SpeculativeOp) error {
    // 1. Execute compensation operations
    for _, cmd := range op.Compensation.RevertActions {
        se.output.Execute(cmd)
    }
    
    // 2. Execute alignment operations
    for _, cmd := range op.Compensation.AlignmentActions {
        se.output.Execute(cmd)
    }
    
    // 3. Mark status
    op.Status = SpeculativeCompensated
    delete(se.layer.pending, op.Key)
    
    // 4. Record compensation history (for subsequent alignment)
    se.compensator.Record(op)
    
    return nil
}
```

#### 5.2.3 State Alignment

```go
func (sa *StateAligner) Align() error {
    diff := sa.calculateDiff()
    
    // Process keys that should be released
    for _, key := range diff.ShouldRelease {
        // Check if it's a compensated key
        if sa.compensator.IsCompensated(key) {
            // Special handling: user might still be holding this key
            sa.handleCompensatedKeyRelease(key)
        } else {
            sa.forceRelease(key)
        }
    }
    
    // Process keys that should be pressed
    for _, key := range diff.ShouldPress {
        sa.forcePress(key)
    }
    
    return nil
}

func (sa *StateAligner) handleCompensatedKeyRelease(key KeyCode) {
    // Strategy: Don't send release, wait for physical release
    // But mark as "expected release" to prevent duplicate press
    sa.pendingReleases[key] = PendingRelease{
        Key:        key,
        ExpectedAt: time.Now(),
    }
}
```

### 5.3 Configuration Design

```go
type StateMachineConfig struct {
    // ISM configuration
    Input InputConfig
    
    // SSM configuration
    Semantic SemanticConfig
    
    // OSM configuration
    Output OutputConfig
    
    // Speculative execution configuration
    Speculative SpeculativeConfig
}

type SemanticConfig struct {
    // Modifier pending threshold
    // After this time without forming a combination, convert to independent
    ModifierPendingThreshold time.Duration  // Default 200ms
    
    // Whether to allow downgrade (independent → modifier)
    AllowDowngrade bool  // Default true
    
    // Downgrade time window
    DowngradeWindow time.Duration  // Default 100ms
    
    // Debounce threshold
    DebounceThreshold time.Duration  // Default 5ms
}

type SpeculativeConfig struct {
    // Confirmation window
    ConfirmationWindow time.Duration  // Default 50ms
    
    // Whether to enable speculative execution
    Enabled bool  // Default true
    
    // Compensation strategy
    CompensationMode CompensationMode
}

type CompensationMode int
const (
    // Immediate compensation: send release
    CompensationImmediate CompensationMode = iota
    
    // Deferred compensation: wait for physical release
    CompensationDeferred
    
    // Smart compensation: decide based on context
    CompensationSmart
)
```

---

## 6. State Transitions

### 6.1 Normal Mapping Flow

```
[Idle]
  │
  │ Cmd↓
  ▼
[PendingModifier] ──(timeout 200ms)──▶ [Independent] ──(Cmd↑)──▶ [Idle]
  │                                   │
  │ C↓ (within timeout)                │
  ▼                                   │
[ComboMatched]                        │
  │                                   │
  │ Convert: Ctrl↓ C↓                  │
  ▼                                   │
[MappingActive] ◀─────────────────────┘
  │
  │ C↑
  ▼
[PartialRelease]
  │
  │ Cmd↑
  ▼
[Idle] (Send Ctrl↑)
```

### 6.2 Delayed Decision Flow

```
[Idle]
  │
  │ Cmd↓
  ▼
[SpeculativePending] ──(confirmation window 50ms)──▶ [SpeculativeConfirmed]
  │                                                   │
  │ C↓ (within window)                                │ Cmd↑
  ▼                                                   ▼
[MatchConfirmed]                                   [Idle]
  │
  │ Compensate Cmd↓, send Ctrl↓ C↓
  ▼
[MappingActive]
```

### 6.3 Downgrade Flow

```
[PendingModifier] (Cmd↓, waiting)
  │
  │ (Exceeds 200ms, no combination)
  ▼
[Downgrading]
  │
  │ Send Cmd↓ (pass-through)
  ▼
[IndependentActive]
  │
  │ C↓ (within downgrade window 100ms)
  ▼
[AttemptDowngrade]
  │
  ├─ Success ──▶ [MatchConfirmed] ──▶ [MappingActive]
  │
  └─ Failure ──▶ [IndependentActive] (C as normal key)
```

---

## 7. Implementation Roadmap

### Phase 1: Foundation (2-3 weeks)

1. **ISM Implementation**
   - Input state tracking
   - Multi-device aggregation
   - Historical state management

2. **OSM Foundation**
   - Desired/actual state separation
   - Basic output device interface
   - State alignment mechanism

3. **State Binder**
   - Binding relationship management
   - Auto-release tracking

### Phase 2: Semantic Layer (2 weeks)

1. **SSM Implementation**
   - Semantic translation logic
   - Pending/Active/Independent state management
   - Configuration integration

2. **Downgrade Mechanism**
   - Timeout detection
   - Downgrade decision
   - State recovery

### Phase 3: Speculative Execution (2 weeks)

1. **Speculative Layer**
   - Speculative operation management
   - Confirmation window implementation
   - Timer management

2. **Compensator**
   - Compensation strategies
   - Alignment mechanism
   - Compensation history

### Phase 4: Integration and Optimization (2 weeks)

1. **Integration with Existing Code**
   - JavaScript engine adaptation
   - Configuration format compatibility
   - Migration tools

2. **Performance Optimization**
   - Memory pools
   - Batching
   - Lock optimization

3. **Test Coverage**
   - Unit tests
   - Integration tests
   - Fuzzing tests

### Phase 5: Advanced Features (Optional)

1. **Machine Learning Assistance**
   - User behavior learning
   - Dynamic threshold adjustment

2. **Visual Debugging**
   - State machine visualization
   - Real-time state monitoring

---

## Appendix

### A. Glossary

| Term | Description |
|------|-------------|
| ISM | Input State Machine |
| SSM | Semantic State Machine |
| OSM | Output State Machine |
| Pending Modifier | Modifier key waiting to be determined as modifier or independent |
| Speculative Execution | Execute in advance but revocable |
| Compensation | Revoke an already executed operation |
| State Binding | Association between output keys and input keys |

### B. Reference Implementations

- [QMK](https://qmk.fm/): Keyboard firmware state machine implementation
- [Kanata](https://github.com/jtroo/kanata): Keyboard remapping tool in Rust
- [KMonad](https://github.com/kmonad/kmonad): Keyboard manager in Haskell

---

*Document Version: v1.0*
*Last Updated: 2026-02-11*
