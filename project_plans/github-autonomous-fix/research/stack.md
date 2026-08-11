# Stack Research: Autonomous/LLM Control Infrastructure

## 1. ClaudeController (`session/claude_controller.go`)

### Core struct fields
```go
type ClaudeController struct {
    sessionName      string
    instance         InstanceContext        // narrow interface to avoid bidirectional dep
    ptyAccess        *PTYAccess
    responseStream   *ResponseStream
    statusDetector   *detection.StatusDetector
    idleDetector     *detection.IdleDetector
    rateLimitHandler *ratelimit.PTYConsumer
    queue            *CommandQueue
    executor         *CommandExecutor
    history          *CommandHistory
    statusChangeListener StatusChangeListener   // callback on status transitions
    lastEmittedStatus    detection.DetectedStatus
    statusCheckCh    chan struct{}               // capacity 1; non-blocking send
}
```

### Key interface: `InstanceContext`
Only the narrow set of methods ClaudeController needs from an `*Instance`:
```go
type InstanceContext interface {
    GetTitle() string
    GetPTYReader() (*os.File, error)
    Preview() (string, error)
    LastMeaningfulOutputTime() time.Time
    GetCreatedAt() time.Time
    SetLastMeaningfulOutput(t time.Time)
    GetStatus() int
    WriteToPTY(data []byte) (int, error)
}
```

### StatusChangeListener callback type
```go
type StatusChangeListener func(newStatus detection.DetectedStatus, sessionName string)
```
Wired via `SetStatusChangeListener(fn)`. Fires from a dedicated background goroutine (`runStatusChangeLoop`) on every terminal status transition. Called **outside** `cc.mu`.

### Idle state detection entry points
- `GetIdleState() (detection.IdleState, time.Time)` — returns state + last-activity time
- `IsIdle() bool` — true when `IdleStateWaiting` or `IdleStateTimeout`
- `IsActive() bool` — true when `IdleStateActive`
- `GetIdleStateInfo() detection.IdleStateInfo` — full info struct for display

Both are hash-cached: same terminal content returns cached result with zero allocs.

### Sending input
- `SendCommand(text string, priority int) (string, error)` — queued (async)
- `SendCommandImmediate(text string) (*ExecutionResult, error)` — bypasses queue, waits

### Activation/deactivation
- `Start(ctx context.Context) error` — creates all components, starts background goroutines
- `Stop() error` — cancels context, saves queue/history, stops all components

The PTY `SetOnOutput` callback chains two things: `idleDetector.RecordActivity()` and a non-blocking send to `statusCheckCh`. The `runStatusChangeLoop` goroutine drains `statusCheckCh` and fires `statusChangeListener` on transitions.

---

## 2. Instance Controller Lifecycle (`session/instance_controller.go`)

### Start/Stop wiring
`StartController()` on Instance:
1. Checks `controllerManager.statusManager != nil` and `i.started`
2. Creates `NewClaudeController(i)`
3. Wires PTY-EOF callback: on exit, fires `EventExited` and optionally `recoverFromStaleResume()`
4. Calls `wireStatusChangeCallback(controller)` **before** `Start()` to avoid race
5. Calls `controller.Start(context.Background())`
6. Stores controller via `controllerManager.RegisterController`
7. Calls `wireRateLimitCallbacks(controller)`

### Callback wiring
```go
// From instance to controller:
func (i *Instance) SetStatusChangeCallback(fn func(detection.DetectedStatus, string))
func (i *Instance) SetRateLimitCallbacks(onDetected, onRecovery func(...))

// Delegation to controller:
func (i *Instance) GetController() *ClaudeController
func (i *Instance) GetRateLimitState() int
func (i *Instance) GetRateLimitResetTime() time.Time
```

---

## 3. Detection Package (`session/detection/`)

### `DetectedStatus` values (ordered by priority in Detect())
```
StatusUnknown, StatusReady, StatusProcessing, StatusNeedsApproval,
StatusInputRequired, StatusError, StatusTestsFailing, StatusIdle,
StatusActive, StatusSuccess
```

### `IdleState` values
```go
IdleStateUnknown  // cannot determine
IdleStateActive   // shows "esc to interrupt"
IdleStateWaiting  // at ">" prompt, "? for shortcuts", "— INSERT —"
IdleStateTimeout  // waiting + idled > IdleThreshold (5s default)
```

### Key methods on `IdleDetector`
```go
func (id *IdleDetector) DetectStateFromContent(content string) IdleState
func (id *IdleDetector) GetState() IdleState          // cached, no detection
func (id *IdleDetector) GetLastActivity() time.Time
func (id *IdleDetector) GetIdleDuration() time.Duration
func (id *IdleDetector) RecordActivity()              // debounced, 500ms min interval
func (id *IdleDetector) InitializeFromTimestamp(timestamp time.Time)
```

`DetectStateFromContent` is the preferred method — takes pre-fetched content from `tmux capture-pane` path rather than re-reading the circular buffer.

`mapStatusToIdleState` mapping:
- `StatusActive` / `StatusProcessing` → `IdleStateActive`
- `StatusIdle` / `StatusReady` → `IdleStateWaiting` (or `IdleStateTimeout` if idle > threshold)
- `StatusNeedsApproval` → `IdleStateWaiting`
- `StatusError` → `IdleStateWaiting`

---

## 4. Instance Fields for Autonomous Control (`session/instance.go` lines 140-234)

```go
AutonomousMode  bool   `json:"autonomous_mode,omitempty"`  // defined but NOT wired to any driver

OneShot         bool   // run claude with -p; exit after completion
AllowedTools    string // --allowedTools "Bash,Read,Edit" or "Bash(git commit *),Read"
PermissionMode  string // --permission-mode "default"|"acceptEdits"|"bypassPermissions"|"auto"
AppendSystemPrompt string // --append-system-prompt injected text
MCPServerURL    string // --mcp-config HTTP URL for stapler-squad MCP

// GitHub integration
GitHubPRNumber  int
GitHubPRURL     string
GitHubOwner     string
GitHubRepo      string
GitHubPRState   string    // "open" / "closed" / "merged"
GitHubPRPriority string   // "blocking"/"ready"/"pending"/"draft"/"complete"/"no_pr"
GitHubCheckConclusion string // CI rollup
```

**`AutonomousMode` is currently orphaned** — stored/serialized but never read by any goroutine. No `AutonomousDriver` goroutine exists yet.

---

## 5. `RunWithResume` (`session/instance_claude.go` lines 373-410)

Spawns a **subprocess** (not PTY):
```go
cmd := safeexec.CommandContext(ctx, claudePath, "-p", "--resume", uuid, "--output-format", "json", message)
cmd.Dir = i.GetEffectiveRootDir()
out, runErr := cmd.Output()
```
Returns `result` string from JSON `"result"` field. Updates `ConversationUUID` if it changes. Used by `steer_session` for completed OneShot sessions.

---

## 6. `steer_session` MCP Tool (`server/mcp/tools_terminal.go` lines 603-683)

Two dispatch paths:
1. **OneShot + stopped + UUID present** → `inst.RunWithResume(ctx, message)` (subprocess, method: `"resume_subprocess"`)
2. **Otherwise** → `inst.SendKeys(message + "\r")` with 5s timeout (PTY send-keys, method: `"send_keys"`)

---

## 7. Approval Handler (`server/services/approval_handler.go` lines 1-330)

HTTP POST endpoint `/api/hooks/permission-request`. Flow:
1. Secret scan → auto-deny if plaintext secret found
2. Domain age check → escalate newly-registered domains
3. `AskUserQuestion` → informational notification, defer to native dialog
4. Classifier → `AutoAllow` / `AutoDeny` returns immediately; `Escalate` falls through
5. Creates `PendingApproval{ID, SessionID, ToolName, ToolInput, ExpiresAt}` with 4-min timeout
6. Broadcasts notification to web UI
7. **Blocks** on `approval.decisionCh` until: user responds, server times out (4 min), or context canceled

On timeout → returns empty HTTP 200, Claude Code falls back to native terminal dialog. On context cancel → silently allows.

For an AutonomousDriver that wants to **skip** the approval handler entirely, setting `PermissionMode: "bypassPermissions"` or `AllowedTools: "Bash,Edit,..."` on the `Instance` before launch is the clean path (these are passed as CLI flags in `instance_tmux.go`).
