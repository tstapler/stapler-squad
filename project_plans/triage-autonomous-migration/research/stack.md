# Stack Research: triage-autonomous-migration

## Go Version

`go 1.25.0` (`go.mod` line 3)

---

## Key Dependencies

- `connectrpc.com/connect v1.19.0` — ConnectRPC handler framework
- `entgo.io/ent v0.14.5` — ORM for SQLite
- `github.com/mattn/go-sqlite3 v1.14.40` — SQLite driver
- `golang.org/x/sync v0.20.0` — provides `errgroup` (imported via `golang.org/x/sync/errgroup`)
- `github.com/linkdata/deadlock v0.5.5` — deadlock-detecting mutex wrapper used in place of `sync.Mutex`/`sync.RWMutex` throughout tmux code
- `github.com/puzpuzpuz/xsync/v4 v4.5.0` — lock-free concurrent maps

---

## Key Concurrency Patterns

The codebase uses Go's standard concurrency primitives with some distinctive patterns:

### 1. `atomic.Bool` with `CompareAndSwap` for idempotency guards
```go
// session/autonomous_driver.go
if !d.driverRunning.CompareAndSwap(false, true) {
    return nil
}
// session/session_driver.go
if !inst.driverRunning.CompareAndSwap(false, true) {
    return
}
```
Used extensively to prevent duplicate goroutine launches.

### 2. `sync.Mutex` guarding callback fields
```go
// autonomous_driver.go
d.mu.Lock()
d.cancel = cancel
d.controller = d.inst.GetController()
d.mu.Unlock()
```

### 3. Channels as signals (capacity 1 for non-blocking drops)
```go
// autonomous_driver.go
statusCh := make(chan detection.DetectedStatus, 1)
d.controller.AddStatusChangeListener(func(newStatus detection.DetectedStatus, _ string) {
    select {
    case statusCh <- newStatus:
    default:
    }
})
```

### 4. Channel-based concurrency semaphore
```go
// headless/pool.go
concurrencySem chan struct{}
// Acquiring:
select {
case p.concurrencySem <- struct{}{}:
case <-ctx.Done():
    ...
}
// Releasing:
defer func() { <-p.concurrencySem }()
```

### 5. `deadlock.Mutex` / `deadlock.RWMutex` in hot paths
Used throughout `tmux/tmux.go` (`controlModeSubMu`, `existsCacheMutex`, `detachMutex`, `cmdSendMu`) in place of standard mutexes to detect deadlocks in development.

### 6. `sync.WaitGroup` for goroutine lifetime tracking
Used inside `TmuxSession` (`wg *sync.WaitGroup`) to join goroutines during Detach.

### 7. `sync.Once` for single-fire events
`attachCmdWaitOnce *sync.Once` in `TmuxSession` — ensures `attachCmd.Wait()` is called exactly once.

### 8. `context.WithCancel` / `context.WithTimeout` for tree cancellation
Driver goroutines use `ctx.WithCancel` stored on the struct; `Stop()` calls the cancel function.

---

## ent ORM: Session Entity

### Schema location
`/home/tstapler/Programming/stapler-squad/session/ent/schema/session.go`

### Key fields relevant to autonomous/triage work

| Field | Type | Notes |
|---|---|---|
| `title` | String, unique, NotEmpty | Session identifier |
| `uuid` | String, optional | Stable UUID, survives renames |
| `path` | String, NotEmpty | Workspace repo root |
| `status` | Int | Enum: Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4 |
| `program` | String, NotEmpty | e.g. `"claude"` |
| `session_type` | String, optional | `"directory"`, `"new_worktree"`, `"existing_worktree"` |
| `one_shot` | Bool, default=false | Runs claude in `-p` mode; session exits after task |
| `initial_prompt` | String, optional | Prompt injected at Ready state via SessionDriver |
| `hidden` | Bool, default=false | Excluded from UI list and review queue |
| `workflow_id` | String, optional | UUID of spawning Workflow |
| `archived_at` | Time, nillable | Set when archived |
| `last_meaningful_output` | Time, nillable | Used for inactivity detection |
| `last_acknowledged` | Time, nillable | Last human acknowledgement |

### Edges (relationships)
- `worktree` → `Worktree` (one-to-one)
- `tags` → `Tag` (many-to-many)
- `claude_session` → `ClaudeSession` (one-to-one, stores conversation UUID)
- `project` ← `Project` (many-to-one)
- `backlog_items` ← `BacklogItem` (many-to-many)
- `shells` → `Shell` (one-to-many)

### Indexes
`title`, `status`, `category`, `last_meaningful_output`, `last_acknowledged`, `created_at`, `workflow_id`, `archived_at`

### ORM generation command (CRITICAL — must include `--feature sql/upsert`)
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

---

## ConnectRPC Handler Patterns

### Compile-time interface assertion
```go
// session_service.go line 39
var _ sessionv1connect.SessionServiceHandler = (*SessionService)(nil)
```

### Handler signature
```go
func (s *SessionService) CreateSession(
    ctx context.Context,
    req *connect.Request[sessionv1.CreateSessionRequest],
) (*connect.Response[sessionv1.CreateSessionResponse], error) {
    ...
    return connect.NewResponse(&sessionv1.CreateSessionResponse{...}), nil
    // or
    return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("..."))
}
```

### Handler marker comments (for feature registry)
```go
// +api: session:create
func (s *SessionService) CreateSession(...) { ... }
```

### Key patterns observed
- Validation at top of handler → return early with `connect.NewError(connect.Code..., err)`
- Load live instances via `s.reviewQueuePoller.GetInstances()` rather than `LoadInstances()` to avoid side-effects
- Async session start in a goroutine; handler returns `Creating` state immediately
- Proto-to-domain mapping in `resolveSessionType()` helper

---

## HeadlessPool: Interface and Struct

### Narrow interface (what AutonomousDriver needs)
```go
// session/autonomous_driver.go
type HeadlessPoolClient interface {
    CallBlockingWithOptions(ctx context.Context, key headless.FeatureKey, systemPrompt string, userPrompt string, opts headless.CallOptions) (string, error)
}
// *headless.Pool satisfies this interface directly.
```

### Concrete struct (`session/headless/pool.go`)
```go
type Pool struct {
    claudeBin string
    cfg       PoolConfig   // MaxCallsPerSession (default 25), MaxConcurrentSessions (default 5), DefaultModel
    runner    ClaudeRunner // ProcessRunner or FakeRunner in tests

    mu       sync.Mutex
    sessions map[FeatureKey]*sessionState   // per-key session ID and call counts
    keyMu    map[FeatureKey]*sync.Mutex     // per-key serialization

    concurrencySem chan struct{}   // bounded concurrency, capacity = MaxConcurrentSessions
}
```

### Session state per feature key
```go
type sessionState struct {
    sessionID         string   // claude --resume ID; empty = first call
    callCount         int      // rotate after MaxCallsPerSession
    consecutiveErrors int      // circuit-breaker: rotate after 3 consecutive errors
}
```

### Pool lifecycle in session_service.go
- Wired via `s.SetHeadlessPool(pool)` at server startup
- Stored as `headlessPool *headless.Pool` on `SessionService`
- Nil-safe: handlers check for nil and return `CodeUnimplemented` or log a warning
- Package-level default: `headless.SetDefaultPool(pool)` / `headless.DefaultPool()`

### Key call paths
```go
// Blocking (returns full response string):
pool.CallBlocking(ctx, featureKey, systemPrompt, userPrompt) (string, error)
pool.CallBlockingWithOptions(ctx, featureKey, systemPrompt, userPrompt, opts) (string, error)

// Streaming (returns channel):
pool.Call(ctx, featureKey, systemPrompt, userPrompt) (<-chan StreamChunk, error)
```

### First call vs. resume
- **First call**: uses `claude -p --output-format json --system-prompt <prompt>`, reads JSON result, captures `session_id`
- **Resumed call**: uses `claude -p --resume <session_id>`, streams line-by-line

### Feature key constants (`session/headless/features.go`)
```go
FeatureKeyAutonomousFix      FeatureKey = "autonomous_fix"
FeatureKeyAutonomousApproval FeatureKey = "autonomous_approval"
FeatureKeyReview             FeatureKey = "review"
// ... etc.
```

---

## tmux Integration

### Package: `session/tmux/`
The core struct is `TmuxSession` in `session/tmux/tmux.go`.

### Session creation flow
1. `instance_tmux.go: initTmuxSession()` — calls `buildLaunchCommand()` to assemble the full `claude` invocation string including flags, then creates a `tmux.NewTmuxSessionWithPrefix(title, command, prefix)`
2. `buildLaunchCommand()` appends flags based on instance fields:
   - `--resume <id>` if `claudeSession.ConversationUUID` is set
   - `--mcp-config` if `MCPServerURL` non-empty
   - `--append-system-prompt` if `AppendSystemPrompt` non-empty
   - `--allowedTools` if `AllowedTools` non-empty
   - `--permission-mode` if `PermissionMode` non-empty
   - `--dangerously-skip-permissions` if `AutoYes`
   - **`-p --output-format json`** if `OneShot` (key flag)
   - `<prompt>` argument if `Prompt` and (no resume or OneShot)

### TmuxSession struct fields (selected)
```go
type TmuxSession struct {
    sanitizedName string
    program       string
    serverSocket  string     // -L flag for isolated tmux servers (used in tests)
    ptmx          *os.File   // PTY for tmux attach
    attachCmd     *exec.Cmd  // tmux attach-session process
    // Control mode (replaces pipe-pane + FIFO)
    controlModeCmd         *exec.Cmd
    controlModeStdout      io.ReadCloser
    controlModeStdin       io.WriteCloser
    controlModeDone        chan struct{}
    controlModeSubscribers map[string]chan []byte
    controlModeSubMu       deadlock.RWMutex
    // Priority-queued command dispatch
    highPriSendCh  chan cmSendReq   // user send-keys
    normPriSendCh  chan cmSendReq   // background polling
}
```

### Terminal I/O
- PTY attach via `github.com/creack/pty`
- Control mode (`tmux -C attach`) for push-based event delivery (replaces polling)
- `capture-pane` for `Preview()` output snapshotting

---

## AutonomousDriver: Struct, Lifecycle, Behavior

### Definition (`session/autonomous_driver.go`)
```go
type AutonomousDriver struct {
    inst          *Instance
    controller    *ClaudeController
    headlessPool  HeadlessPoolClient   // narrow interface; *headless.Pool satisfies it
    goal          string               // task description passed to orchestrator LLM
    maxTurns      int                  // default 20
    completionCb  CompletionCallback   // called on exit
    turnCb        TurnCallback         // called after each injection
    driverRunning atomic.Bool          // idempotency guard
    cancel        context.CancelFunc
    mu            sync.Mutex           // guards cancel, controller, callbacks
}
```

### Construction
```go
driver := session.NewAutonomousDriver(inst, pool, goal, maxTurns)
// maxTurns <= 0 → default 20
driver.RegisterCompletionCallback(cb)
driver.RegisterTurnCallback(cb)
err := driver.Start(ctx)
```

### Lifecycle
1. `Start(ctx)` — atomic CAS prevents duplicate start; spawns `run(ctx)` goroutine
2. `run(ctx)`:
   a. Registers a status-change listener that sends to a capacity-1 `statusCh` channel
   b. Waits up to 60s for initial idle state
   c. Loop up to `maxTurns`:
      - Waits for rate-limit to clear (up to 4 hours)
      - Reads session tail via `inst.Preview()`
      - Calls `headlessPool.CallBlockingWithOptions()` with orchestrator prompt
      - Parses `NEXT_MESSAGE: <text>` or `DONE: <reason>` from LLM response
      - On `DONE`: fires `completionCb` with `AutonomousDriverOutcome{Done: true, PRUrl: ...}`
      - On `NEXT_MESSAGE`: calls `controller.SendCommandImmediate(msg + "\r")`, waits up to 5 min for idle
3. `Stop()` — cancels context; pool call exits via `ctx.Done` select

### Outcome struct
```go
type AutonomousDriverOutcome struct {
    Done   bool
    Reason string
    PRUrl  string   // extracted from last 200 lines of output
    Turns  int
    Stuck  bool     // true if maxTurns reached without DONE
}
```

### Registry in SessionService
```go
// session_service.go
driverMu       sync.RWMutex
driverRegistry map[string]*session.AutonomousDriver   // title → driver
```
Drivers are registered via `registerDriver()` and stopped via `stopAndDeregisterDriver()` on session delete/hibernate.

### Wiring in CreateSession
```go
// After async session start:
if instance.AutonomousMode && s.headlessPool != nil {
    driver := session.NewAutonomousDriver(instance, s.headlessPool, instance.Prompt, 0)
    driver.RegisterCompletionCallback(s.onAutonomousDriverComplete)
    driver.Start(s.driverCtx())
    s.registerDriver(instanceTitle, driver)
}
```

---

## OneShot Field

### Where it is set

**Instance struct** (`session/instance.go` line 197-198):
```go
// OneShot runs claude in -p mode; the session exits after the task completes.
OneShot bool
```

**InstanceOptions struct** (`session/instance.go` line 445-446):
```go
// OneShot runs claude in -p mode; the session exits after the task completes.
OneShot bool
```

**ent schema** (`session/ent/schema/session.go` line 84-87):
```go
field.Bool("one_shot").
    Default(false).
    Comment("When true, runs claude in -p mode; session exits after task completes."),
```

**Set in session_service.go** (line ~1055, inside `CreateSession`):
```go
OneShot: req.Msg.OneShot,
```

### What it controls

**1. Command-line flags** (`session/instance_tmux.go: buildLaunchCommand()`):
```go
if i.OneShot && strings.Contains(program, "claude") {
    program = program + " -p --output-format json"
}
if i.Prompt != "" && (claudeSessionID == "" || i.OneShot) && strings.Contains(program, "claude") {
    program = fmt.Sprintf("%s %q", program, i.Prompt)
}
```
`OneShot=true` causes claude to be launched as `claude -p --output-format json "<prompt>"` — runs non-interactively and exits after one task.

**2. SessionDriver retry suppression** (`session/session_driver.go: isOneShot()`):
```go
func isOneShot(inst *Instance) bool {
    return inst.HasTag("backlog:triage") || inst.HasTag("backlog:review")
}
```
Note: `isOneShot()` checks *tags*, not `inst.OneShot` directly. One-shot instances are NOT auto-retried on crash/exit.

**3. Claude session ID extraction** (`session_driver.go` line 173-174):
```go
if inst.OneShot {
    tryExtractClaudeSessionID(inst)
}
```
When a OneShot session stops, the driver reads `--output-format json` output to extract the `session_id` for potential future `--resume`.

**4. BacklogService `CreateDirectorySession` signature**:
```go
CreateDirectorySession(ctx, title, path, prompt string, tags []string, oneShot bool, hidden bool) (*session.Instance, error)
```
BacklogService explicitly passes `oneShot=true` for triage/review sessions to prevent auto-retry loops.

---

## Summary: Key Architectural Patterns for Triage/Autonomous Migration

1. **Triage sessions use `OneShot=true` + tags `backlog:triage`/`backlog:review`** to:
   - Run claude non-interactively (`-p --output-format json`)
   - Prevent SessionDriver from retrying on exit
   - Let `BacklogLifecycleListener` handle state transitions

2. **AutonomousDriver** is the interactive steering mechanism — it does NOT apply to OneShot sessions (which exit after one prompt). It applies to long-running sessions where an LLM orchestrator injects turn-by-turn prompts.

3. **HeadlessPool** is the shared resource for all LLM subprocess calls — both AutonomousDriver (via `CallBlockingWithOptions`) and RunOneShot RPC (via `headlessPool.CallBlockingWithOptions` with `WorkDir` option).

4. **SessionDriver** (`session_driver.go`) handles startup dialogs, initial prompt injection, and crash recovery for all session types. It defers to `isOneShot()` (tag-based) to skip retry logic.

5. **ConnectRPC** uses the `connectrpc.com/connect` package with generics: `*connect.Request[ProtoMsg]` → `*connect.Response[ProtoMsg]`. Error codes mirror gRPC status codes.
