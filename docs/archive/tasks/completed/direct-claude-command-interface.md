# Direct Claude Command Interface - AIC Task Breakdown

## Epic Overview

**Goal**: Implement a direct command interface to Claude instances that allows sending commands without attaching to tmux sessions, with real-time status monitoring, response streaming, and intelligent approval handling.

**Value Proposition**: Dramatically improves developer workflow by eliminating the need to manually attach to tmux sessions for simple commands, while providing real-time feedback and automatic handling of routine prompts.

**Success Metrics**:
- Command execution without tmux attachment
- < 100ms status detection latency
- 95% accuracy in approval pattern detection
- Zero data loss in response streaming
- Support for 10+ concurrent command queues

**Technical Requirements**:
- Thread-safe PTY access layer
- Pattern-based status detection
- Real-time response streaming
- Command queue management
- Automatic approval handling

## Story Breakdown

### Story 1: Core Infrastructure (Week 1)
**Objective**: Establish the foundation for PTY communication and status detection

### Story 2: Command Execution System (Week 2)
**Objective**: Implement command queuing and execution with status monitoring

### Story 3: Approval Automation (Week 3)
**Objective**: Add intelligent approval handling for file permission prompts

### Story 4: UI Integration (Week 4)
**Objective**: Create user interface for command input and response viewing

## Atomic Tasks

### Story 1: Core Infrastructure

#### Task 1.1: PTY Access Layer (3h)

**Scope**: Create thread-safe PTY access wrapper for Claude sessions

**Files**:
- session/pty_access.go (create)
- session/pty_access_test.go (create)

**Context**:
- Uses existing tmux PTY integration from session/tmux/
- Requires RWMutex for concurrent access protection
- Must handle PTY session lifecycle (attach/detach)

**Implementation**:
```go
type PTYAccess struct {
    mu          sync.RWMutex
    sessionName string
    pty         *os.File
    buffer      *CircularBuffer
}

func (p *PTYAccess) Write(data []byte) (int, error)
func (p *PTYAccess) Read(buf []byte) (int, error)
func (p *PTYAccess) GetBuffer() []byte
```

**Success Criteria**:
- Thread-safe read/write operations
- No race conditions under concurrent access
- Proper error handling for disconnected PTYs

**Testing**: Unit tests with concurrent goroutines, mock PTY for isolation

**Dependencies**: None

**Status**: ⏳ Pending

---

#### Task 1.2: Circular Buffer Implementation (2h)

**Scope**: Implement circular buffer with disk fallback for PTY output

**Files**:
- session/circular_buffer.go (create)
- session/circular_buffer_test.go (create)

**Context**:
- 10MB in-memory buffer
- Automatic disk fallback when full
- Thread-safe operations required

**Implementation**:
```go
type CircularBuffer struct {
    data      []byte
    size      int
    head      int
    tail      int
    diskFile  *os.File
    mu        sync.RWMutex
}

func NewCircularBuffer(size int) *CircularBuffer
func (cb *CircularBuffer) Write(data []byte) (int, error)
func (cb *CircularBuffer) GetRecent(n int) []byte
```

**Success Criteria**:
- Maintains last 10MB of output
- Seamless disk fallback
- O(1) write operations

**Testing**: Stress tests with large data volumes, boundary conditions

**Dependencies**: None

**Status**: ⏳ Pending

---

#### Task 1.3: Status Detection Engine (4h)

**Scope**: Pattern-based status detection for Claude instances

**Files**:
- session/status_detector.go (create)
- session/status_patterns.yaml (create)
- session/status_detector_test.go (create)

**Context**:
- Regex patterns for Ready, Processing, Needs Approval, Error states
- Configurable patterns via YAML
- Must handle Claude UI variations

**Implementation**:
```go
type Status int
const (
    StatusUnknown Status = iota
    StatusReady
    StatusProcessing
    StatusNeedsApproval
    StatusError
)

type StatusDetector struct {
    patterns map[Status][]*regexp.Regexp
}

func (sd *StatusDetector) Detect(output []byte) Status
func (sd *StatusDetector) LoadPatterns(path string) error
```

**Success Criteria**:
- Accurate status detection within 100ms
- Configurable patterns without recompilation
- Handles multi-line patterns

**Testing**: Pattern matching tests with real Claude output samples

**Dependencies**: Task 1.1

**Status**: ⏳ Pending

---

#### Task 1.4: Response Stream Foundation (3h)

**Scope**: Real-time streaming of Claude responses

**Files**:
- session/response_stream.go (create)
- session/response_stream_test.go (create)

**Context**:
- Uses PTY access layer for data source
- Channel-based streaming to multiple consumers
- Buffered to prevent blocking

**Implementation**:
```go
type ResponseStream struct {
    sessionName string
    ptyAccess   *PTYAccess
    subscribers []chan []byte
    mu          sync.RWMutex
}

func (rs *ResponseStream) Subscribe() <-chan []byte
func (rs *ResponseStream) Unsubscribe(ch <-chan []byte)
func (rs *ResponseStream) Start(ctx context.Context)
```

**Success Criteria**:
- Real-time streaming with < 50ms latency
- Multiple concurrent subscribers
- Clean shutdown without goroutine leaks

**Testing**: Integration tests with mock PTY, concurrent subscriber tests

**Dependencies**: Task 1.1, Task 1.2

**Status**: ⏳ Pending

---

### Story 2: Command Execution System

#### Task 2.1: Command Queue Implementation (3h)

**Scope**: Sequential command queue with priority support

**Files**:
- session/command_queue.go (create)
- session/command_queue_test.go (create)

**Context**:
- FIFO queue with priority lanes
- Thread-safe enqueue/dequeue operations
- Persistence for recovery after crashes

**Implementation**:
```go
type Command struct {
    ID        string
    Text      string
    Priority  int
    Timestamp time.Time
    Status    CommandStatus
}

type CommandQueue struct {
    commands  []*Command
    mu        sync.Mutex
    notifyCh  chan struct{}
}

func (cq *CommandQueue) Enqueue(cmd *Command)
func (cq *CommandQueue) Dequeue() *Command
func (cq *CommandQueue) Cancel(id string) error
```

**Success Criteria**:
- FIFO ordering within priority levels
- O(log n) priority insertion
- Persistent queue state

**Testing**: Unit tests for queue operations, priority ordering tests

**Dependencies**: None

**Status**: ⏳ Pending

---

#### Task 2.2: Command Execution Engine (4h)

**Scope**: Execute commands with status monitoring and timeout handling

**Files**:
- session/command_executor.go (create)
- session/command_executor_test.go (create)

**Context**:
- Monitors status before/during/after execution
- Context-aware timeouts
- Handles command cancellation

**Implementation**:
```go
type CommandExecutor struct {
    sessionName    string
    ptyAccess      *PTYAccess
    statusDetector *StatusDetector
    queue          *CommandQueue
}

func (ce *CommandExecutor) Execute(ctx context.Context, cmd *Command) error
func (ce *CommandExecutor) WaitForStatus(status Status, timeout time.Duration) error
func (ce *CommandExecutor) Cancel(cmdID string) error
```

**Success Criteria**:
- Reliable command execution
- Proper timeout handling
- Clean cancellation without side effects

**Testing**: Integration tests with mock PTY, timeout scenarios

**Dependencies**: Task 1.1, Task 1.3, Task 2.1

**Status**: ⏳ Pending

---

#### Task 2.3: Command History Tracking (2h)

**Scope**: Persistent command history per session

**Files**:
- session/command_history.go (create)
- session/command_history_test.go (create)

**Context**:
- Store last 1000 commands per session
- Include timestamps and execution results
- Query interface for history retrieval

**Implementation**:
```go
type CommandHistory struct {
    sessionName string
    commands    []*HistoryEntry
    maxSize     int
    mu          sync.RWMutex
}

type HistoryEntry struct {
    Command   *Command
    StartTime time.Time
    EndTime   time.Time
    Result    string
    Error     error
}

func (ch *CommandHistory) Add(entry *HistoryEntry)
func (ch *CommandHistory) GetRecent(n int) []*HistoryEntry
```

**Success Criteria**:
- Persistent storage in JSON format
- Efficient retrieval of recent commands
- Automatic rotation at max size

**Testing**: Unit tests for history operations, persistence tests

**Dependencies**: Task 2.2

**Status**: ⏳ Pending

---

#### Task 2.4: High-Level Claude Control API (3h)

**Scope**: Unified API for controlling Claude instances

**Files**:
- session/claude_control.go (create)
- session/claude_control_test.go (create)

**Context**:
- Facade pattern over lower-level components
- Simple interface for UI layer
- Error handling and recovery

**Implementation**:
```go
type ClaudeControl struct {
    sessionName string
    executor    *CommandExecutor
    stream      *ResponseStream
    history     *CommandHistory
}

func (cc *ClaudeControl) SendCommand(text string) (string, error)
func (cc *ClaudeControl) GetStatus() Status
func (cc *ClaudeControl) StreamResponses() <-chan []byte
func (cc *ClaudeControl) GetHistory(n int) []*HistoryEntry
```

**Success Criteria**:
- Clean, simple API
- Proper error propagation
- Thread-safe operations

**Testing**: Integration tests covering full command lifecycle

**Dependencies**: Task 2.2, Task 1.4, Task 2.3

**Status**: ⏳ Pending

---

### Story 3: Approval Automation

#### Task 3.1: Approval Pattern Detection (3h)

**Scope**: Detect approval prompts in Claude output

**Files**:
- session/approval_detector.go (create)
- session/approval_patterns.yaml (create)
- session/approval_detector_test.go (create)

**Context**:
- Pattern matching for various approval prompts
- Context extraction (file names, operations)
- Configurable patterns

**Implementation**:
```go
type ApprovalPrompt struct {
    Type      string
    Message   string
    Options   []string
    Context   map[string]string
}

type ApprovalDetector struct {
    patterns []*ApprovalPattern
}

func (ad *ApprovalDetector) Detect(output []byte) *ApprovalPrompt
func (ad *ApprovalDetector) LoadPatterns(path string) error
```

**Success Criteria**:
- 95% accuracy in prompt detection
- Extracts relevant context
- Handles multi-line prompts

**Testing**: Pattern tests with real approval scenarios

**Dependencies**: Task 1.3

**Status**: ⏳ Pending

---

#### Task 3.2: Approval Policy Engine (3h)

**Scope**: Configurable approval policies for automatic handling

**Files**:
- session/approval_policy.go (create)
- session/approval_policy_test.go (create)
- config/approval_policies.yaml (create)

**Context**:
- Policy types: auto-approve, auto-deny, manual, conditional
- File pattern matching for conditional policies
- Safety checks for destructive operations

**Implementation**:
```go
type ApprovalPolicy struct {
    Mode       PolicyMode
    FilePatterns []string
    Operations []string
    Conditions []PolicyCondition
}

type ApprovalHandler struct {
    policies []ApprovalPolicy
    detector *ApprovalDetector
}

func (ah *ApprovalHandler) ShouldApprove(prompt *ApprovalPrompt) (bool, string)
func (ah *ApprovalHandler) LoadPolicies(path string) error
```

**Success Criteria**:
- Flexible policy configuration
- Safe defaults (deny destructive ops)
- Override mechanism for manual control

**Testing**: Policy evaluation tests, safety check verification

**Dependencies**: Task 3.1

**Status**: ⏳ Pending

---

#### Task 3.3: Automatic Approval Execution (2h)

**Scope**: Execute approval decisions automatically

**Files**:
- session/approval_executor.go (create)
- session/approval_executor_test.go (create)

**Context**:
- Sends approval response via PTY
- Handles timeout for hung prompts
- Logs all approval decisions

**Implementation**:
```go
type ApprovalExecutor struct {
    ptyAccess *PTYAccess
    handler   *ApprovalHandler
    logger    *ApprovalLogger
}

func (ae *ApprovalExecutor) HandlePrompt(prompt *ApprovalPrompt) error
func (ae *ApprovalExecutor) SendApproval(response string) error
```

**Success Criteria**:
- Reliable approval sending
- Timeout handling for hung prompts
- Audit trail of all approvals

**Testing**: Integration tests with mock prompts

**Dependencies**: Task 3.2, Task 1.1

**Status**: ⏳ Pending

---

#### Task 3.4: Approval History and Audit (2h)

**Scope**: Track all approval decisions for audit

**Files**:
- session/approval_history.go (create)
- session/approval_history_test.go (create)

**Context**:
- Persistent storage of approval decisions
- Query interface for history
- Export capability for compliance

**Implementation**:
```go
type ApprovalRecord struct {
    Timestamp time.Time
    Prompt    *ApprovalPrompt
    Decision  string
    Policy    string
    Manual    bool
}

type ApprovalHistory struct {
    records []*ApprovalRecord
    mu      sync.RWMutex
}

func (ah *ApprovalHistory) Record(record *ApprovalRecord)
func (ah *ApprovalHistory) Query(filter ApprovalFilter) []*ApprovalRecord
```

**Success Criteria**:
- Complete audit trail
- Efficient querying
- Export to JSON/CSV

**Testing**: History recording and query tests

**Dependencies**: Task 3.3

**Status**: ⏳ Pending

---

### Story 4: UI Integration

#### Task 4.1: Command Input Overlay (3h)

**Scope**: Modal overlay for entering commands

**Files**:
- ui/overlay/command_input.go (create)
- ui/overlay/command_input_test.go (create)

**Context**:
- Similar to existing text input overlays
- Command history navigation
- Auto-completion support

**Implementation**:
```go
type CommandInputOverlay struct {
    input       textinput.Model
    history     []string
    historyIdx  int
    suggestions []string
}

func (c *CommandInputOverlay) HandleKeyPress(key tea.KeyMsg) tea.Cmd
func (c *CommandInputOverlay) View() string
```

**Success Criteria**:
- Smooth text input
- History navigation with arrow keys
- Tab completion for common commands

**Testing**: UI component tests with teatest

**Dependencies**: None

**Status**: ⏳ Pending

---

#### Task 4.2: Status Indicators in List View (2h)

**Scope**: Visual status indicators in session list

**Files**:
- ui/list.go (modify)
- ui/status_icons.go (create)

**Context**:
- Add status icons to session list items
- Color coding for different states
- Real-time status updates

**Implementation**:
```go
func getStatusIcon(status Status) string {
    switch status {
    case StatusReady:
        return "●" // Green
    case StatusProcessing:
        return "◐" // Yellow
    case StatusNeedsApproval:
        return "!" // Red
    default:
        return "○" // Gray
    }
}
```

**Success Criteria**:
- Clear visual status indication
- Real-time updates without flicker
- Accessible color choices

**Testing**: Visual regression tests

**Dependencies**: Task 2.4

**Status**: ⏳ Pending

---

#### Task 4.3: Response Preview Pane (3h)

**Scope**: Real-time response display in preview pane

**Files**:
- ui/response_viewer.go (create)
- ui/response_viewer_test.go (create)
- app/app.go (modify)

**Context**:
- Streams responses in real-time
- Syntax highlighting for code blocks
- Auto-scroll to latest output

**Implementation**:
```go
type ResponseViewer struct {
    content    []string
    scrollPos  int
    autoScroll bool
    stream     <-chan []byte
}

func (rv *ResponseViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (rv *ResponseViewer) View() string
```

**Success Criteria**:
- Smooth streaming display
- Proper line wrapping
- Syntax highlighting

**Testing**: Streaming display tests

**Dependencies**: Task 1.4, Task 4.1

**Status**: ⏳ Pending

---

#### Task 4.4: Key Binding Integration (2h)

**Scope**: Add 'c' key binding for command mode

**Files**:
- keys/keys.go (modify)
- keys/help.go (modify)
- app/app.go (modify)

**Context**:
- Follow existing key binding patterns
- Update help system
- Add to menu shortcuts

**Implementation**:
```go
// In keys.go
KeyCommandMode KeyName = "command_mode"

// In GlobalKeyStringsMap
"c": KeyCommandMode

// In app.go key handler
case keys.KeyCommandMode:
    return m.enterCommandMode()
```

**Success Criteria**:
- Key binding works from main view
- Appears in help system
- Shows in bottom menu

**Testing**: Key binding integration tests

**Dependencies**: Task 4.1

**Status**: ⏳ Pending

---

#### Task 4.5: Command History View (2h)

**Scope**: Display command history in UI

**Files**:
- ui/command_history_view.go (create)
- ui/command_history_view_test.go (create)

**Context**:
- List view of recent commands
- Shows execution time and result
- Allows re-execution of commands

**Implementation**:
```go
type CommandHistoryView struct {
    history    []*HistoryEntry
    selected   int
    sessionName string
}

func (ch *CommandHistoryView) View() string
func (ch *CommandHistoryView) HandleKeyPress(key tea.KeyMsg) tea.Cmd
```

**Success Criteria**:
- Clear history display
- Command re-execution
- Filterable by status

**Testing**: History view rendering tests

**Dependencies**: Task 2.3

**Status**: ⏳ Pending

## Dependency Visualization

```
Story 1: Core Infrastructure
├─ Task 1.1: PTY Access Layer ──────────┐
├─ Task 1.2: Circular Buffer            │
├─ Task 1.3: Status Detection ──────────┼─→ Task 1.4: Response Stream
└─────────────────────────────────────────┘

Story 2: Command Execution (depends on Story 1)
├─ Task 2.1: Command Queue
├─ Task 2.2: Command Executor ──────────┐
├─ Task 2.3: Command History ───────────┼─→ Task 2.4: Claude Control API
└─────────────────────────────────────────┘

Story 3: Approval Automation (depends on Story 1)
├─ Task 3.1: Approval Detection
├─ Task 3.2: Policy Engine
├─ Task 3.3: Approval Execution
└─ Task 3.4: Approval History

Story 4: UI Integration (depends on Stories 2 & 3)
├─ Task 4.1: Command Input ──┐
├─ Task 4.2: Status Icons    ├─→ Integration Point
├─ Task 4.3: Response Viewer │
├─ Task 4.4: Key Binding ────┘
└─ Task 4.5: History View
```

## Context Preparation

### Required Understanding
- BubbleTea TUI framework patterns
- Go concurrency patterns (channels, mutexes)
- tmux PTY communication
- Regular expression pattern matching
- YAML configuration parsing

### Relevant Files to Study
- `session/instance.go` - Session lifecycle management
- `session/tmux/tmux.go` - tmux integration patterns
- `ui/overlay/textInput.go` - Overlay implementation pattern
- `app/app.go` - Main application state machine
- `keys/keys.go` - Key binding system

### Testing Strategy
1. **Unit Tests**: All business logic components
2. **Integration Tests**: PTY communication, command execution
3. **Mock Tests**: External dependencies (tmux, file system)
4. **UI Tests**: Using teatest framework
5. **Performance Tests**: Concurrent command execution, streaming

## Progress Tracking

**Overall Progress**: 0/20 tasks (0%)

**Story Completion**:
- Story 1 (Core Infrastructure): 0/4 tasks (0%)
- Story 2 (Command Execution): 0/4 tasks (0%)
- Story 3 (Approval Automation): 0/4 tasks (0%)
- Story 4 (UI Integration): 0/5 tasks (0%)

**Estimated Total Effort**: 53 hours

**Critical Path**:
1. Task 1.1 (PTY Access) → Task 1.3 (Status Detection) → Task 2.2 (Command Executor) → Task 2.4 (Control API) → Task 4.1 (Command Input)

**Parallel Execution Opportunities**:
- Task 1.2 (Circular Buffer) can run parallel with Task 1.3
- Task 2.1 (Command Queue) can run parallel with Story 1
- Story 3 can run parallel with Story 2 after Task 1.3
- Task 4.2 and 4.5 can run parallel after dependencies

## Risk Mitigation

### Technical Risks
1. **PTY Buffer Overflow**
   - Mitigation: Circular buffer with disk fallback (Task 1.2)
   - Monitoring: Buffer usage metrics

2. **Race Conditions**
   - Mitigation: RWMutex protection (Task 1.1)
   - Testing: Concurrent access tests

3. **Status Detection Errors**
   - Mitigation: Configurable patterns (Task 1.3)
   - Fallback: Manual status override

4. **Claude UI Changes**
   - Mitigation: YAML configuration files
   - Monitoring: Pattern match success rate

5. **Network Interruptions**
   - Mitigation: Retry logic with exponential backoff
   - Recovery: Command queue persistence

### Process Risks
1. **Scope Creep**
   - Mitigation: Strict INVEST criteria enforcement
   - Control: Weekly story reviews

2. **Integration Complexity**
   - Mitigation: Clear interface boundaries
   - Testing: Integration tests after each story

## Success Criteria

### Functional Requirements
- ✅ Send commands without tmux attachment
- ✅ Real-time status monitoring
- ✅ Response streaming to UI
- ✅ Command queue management
- ✅ Automatic approval handling
- ✅ Command history tracking

### Non-Functional Requirements
- ✅ < 100ms status detection latency
- ✅ 95% approval pattern accuracy
- ✅ Zero data loss in streaming
- ✅ Support 10+ concurrent queues
- ✅ < 50ms UI responsiveness

### Definition of Done
- All unit tests passing
- Integration tests complete
- Documentation updated
- Code review approved
- Performance benchmarks met
- UI manually tested