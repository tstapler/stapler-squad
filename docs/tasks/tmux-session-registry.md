# Implementation Plan: Tmux Session Registry (Eliminate Fork Throttling)

## Context

The stapler-squad systemd cgroup has been throttled 152,623 times with
"resource temporarily unavailable" because the process repeatedly forks
`exec.Command("tmux", ...)` subprocesses to check session health. The hot
paths are:

| Location | Frequency | Forks per interval |
|---|---|---|
| `session/mux/multiplexer.go:592` `monitorTmuxSession()` | 500ms ticker | 2 (has-session + list-panes) |
| `session/tmux/tmux.go` `DoesSessionExist()` | every cache miss (500ms TTL) | 1 (list-sessions) |
| `session/pty_discovery.go` `monitorLoop()` | 5s ticker | N+3 per session (list-sessions, display-message × N, ps × N) |
| `session/tmux/tmux.go` `RestoreWithWorkDir()` | startup / 5 retries | 5 (DoesSessionExist × 5) |

**Goal**: reduce fork rate from ~40/sec to near-zero for session-status checks
by replacing polling with a push-based tmux control-mode event stream.

---

## Architecture Decision Records

### ADR-001: Server-Level Control Mode vs Per-Session

**Status**: Accepted

**Context**: The existing `TmuxSession.StartControlMode()` (in
`session/tmux/control_mode.go`) opens a `tmux -C attach-session -t
<name>` per session. With 10 sessions that is 10 control-mode processes.
A server-level `tmux -C` connection receives events for all sessions on
that server via `%sessions-changed`, `%session-created`,
`%session-closed`, and `%pane-exited` notifications.

**Decision**: Create a singleton `TmuxServerRegistry` in
`session/tmux/server_registry.go` that holds exactly one `tmux -C
new-session -d -s registry-sentinel` or `tmux -C attach -t keepalive`
connection per tmux server socket. Per-session control mode connections
(for `%output` streaming) remain unchanged — they serve a different
purpose (terminal content delivery).

**Consequences**:
- One persistent process instead of N
- `DoesSessionExist()` becomes a map lookup with no subprocess
- `monitorTmuxSession()` replaced by `PaneExitSubscriber` callback
- PTYDiscovery gets session-list updates pushed to it instead of polling
- Fallback: if the registry connection drops, callers degrade to the
  existing `exec.Command("tmux", ...)` path (already present)

**Rejected alternative**: Global shared per-session control mode — would
require joining all session event streams and adds ordering complexity.

---

### ADR-002: Interface Segregation for Registry Consumers

**Status**: Accepted

**Context**: Three distinct callers need different registry capabilities:
- `DoesSessionExist()` — existence check (boolean map lookup)
- `monitorTmuxSession()` — pane-exit notification
- `PTYDiscovery.discoverOrphanedPTYs()` — enumerate all sessions

**Decision**: Define three narrow interfaces in `session/tmux/`:

```go
// SessionExistenceChecker answers "is session X alive right now?"
type SessionExistenceChecker interface {
    SessionExists(name string) bool
}

// SessionLister returns a snapshot of all live session names.
type SessionLister interface {
    ListSessions() map[string]bool
}

// PaneExitSubscriber delivers a channel closed when the named pane exits.
// Channel-based signaling is idiomatic Go (see Go Idioms section).
type PaneExitSubscriber interface {
    SubscribePaneExit(ctx context.Context, sessionName string) <-chan struct{}
}
```

`TmuxServerRegistry` implements all three. Callers receive only the
interface they need via constructor injection, keeping coupling minimal.

**Consequences**: Callers import the interface, not the concrete type,
satisfying the Dependency Inversion Principle. Tests can provide a fake
implementation without starting tmux.

---

### ADR-003: Fallback Strategy on Registry Failure

**Status**: Accepted

**Context**: The registry's control-mode process may crash (tmux server
restart, socket error). If it does, all callers that removed their polling
loops would have no fallback.

**Decision**: Use a degraded-mode flag in `TmuxServerRegistry`. When the
registry is unhealthy (`registry.IsHealthy() == false`):
- `SessionExists()` delegates to `exec.Command("tmux", "list-sessions")`
  (same as today)
- `ListSessions()` delegates to the existing `ListAllSessions()` function
- `SubscribePaneExit()` starts a one-shot goroutine with a 500ms poll

Registry auto-reconnects with exponential backoff (cap 30s) — callers
never need to handle reconnection themselves.

**Consequences**: Zero regression risk on tmux server restarts. The
throttling reduction materialises only while the registry is healthy
(which should be the steady state).

---

## Interfaces (Hexagonal Port)

**File**: `session/tmux/registry_port.go`

```go
package tmux

// SessionExistenceChecker answers "is session X alive right now?"
// Used by TmuxSession.DoesSessionExist to avoid exec.Command forks.
type SessionExistenceChecker interface {
    SessionExists(name string) bool
    IsHealthy() bool
}

// SessionLister returns a snapshot of all live session names.
// Used by PTYDiscovery and reconciliation loops.
type SessionLister interface {
    ListSessions() map[string]bool
    IsHealthy() bool
}

// PaneExitSubscriber delivers a channel that is closed when the named pane
// exits. Used by Multiplexer.monitorTmuxSession to replace its polling loop.
// The caller selects on the returned channel alongside ctx.Done(). Cancelling
// ctx unregisters the subscription; the channel is then closed immediately
// without firing. Using a channel (not a callback) is the idiomatic Go
// approach: it composes with select and avoids holding locks during delivery.
type PaneExitSubscriber interface {
    SubscribePaneExit(ctx context.Context, sessionName string) <-chan struct{}
}

// TmuxStatePort is the full registry interface. Callers that need all
// capabilities receive this. Those that need a subset should use the
// narrower interface above.
type TmuxStatePort interface {
    SessionExistenceChecker
    SessionLister
    PaneExitSubscriber
}
```

---

## Component Design

### `TmuxServerRegistry`

**File**: `session/tmux/server_registry.go`

**Responsibilities** (Single Responsibility):
- Hold and restart a `tmux -C` control-mode process
- Parse incoming event lines and update the in-memory `map[string]bool`
  session map
- Deliver `PaneExited` callbacks to registered subscribers
- Expose `SessionExists`, `ListSessions`, `SubscribePaneExit`
- Auto-reconnect on failure

**Not responsible for**:
- Issuing tmux commands (that stays with `TmuxSession` and package-level
  functions)
- Managing per-session terminal output (`%output` events are dropped)
- Session creation/deletion logic

**Internal state**:
```go
// Receiver name: r (consistent throughout; see Go Idioms section)
type TmuxServerRegistry struct {
    serverSocket string

    mu       sync.RWMutex
    sessions map[string]bool  // live session names

    // subsMu guards subscribers. IMPORTANT: never call close(ch) while
    // holding subsMu — that would block if the receiver's select is also
    // trying to acquire subsMu, causing deadlock. Copy the channels out
    // under the lock, release the lock, then close. See Known Issues.
    subsMu      sync.Mutex
    subscribers map[string][]chan struct{}  // sessionName -> one-shot signal channels

    healthMu sync.RWMutex
    healthy  bool

    // ctx is the lifetime context passed to Start; cancel is called by Stop.
    // Every goroutine spawned by the registry selects on ctx.Done().
    ctx    context.Context
    cancel context.CancelFunc
}
```

**Control-mode process lifecycle**:
1. Start: `tmux [-L <socket>] -C attach-session -t <keepalive-session> -r`
   (read-only; `-r` prevents writes back to the session)
2. Parse each line from stdout (same logic as existing
   `processControlModeLine`)
3. On `%sessions-changed` or `%session-created <name>` / `%session-closed
   <name>`: update `sessions` map under write lock, fire pane-exit
   callbacks for closed sessions
4. On process exit: set `healthy = false`, wait backoff, restart
5. On `%pane-exited` (if received): fire matching callbacks

**Events consumed**:
| Event | Action |
|---|---|
| `%session-created $ID name` | add `name` to sessions map |
| `%session-closed $ID name` | remove `name`, fire pane-exit callbacks |
| `%sessions-changed` | trigger a `list-sessions` refresh (one exec, cached) |
| `%pane-exited` | fire pane-exit callbacks for the named session |
| `%exit` | set unhealthy, reconnect |
| `%output` | drop (per-session control mode handles this) |

**Note on `%sessions-changed`**: this event signals the list changed but
does not name the session. One `tmux list-sessions` call is needed to
sync the map. This is acceptable because it replaces the steady-state
500ms polling; the exec only happens on actual session creation/deletion,
not every 500ms.

---

### Global Registry Accessor

**File**: `session/tmux/server_registry.go` (continued)

```go
var (
    defaultRegistryMu   sync.Mutex
    defaultRegistry     *TmuxServerRegistry
)

// GetServerRegistry returns the singleton registry for the given server
// socket (empty = default socket). Creates it on first call via sync.Once
// semantics under defaultRegistryMu.
func GetServerRegistry(serverSocket string) *TmuxServerRegistry
```

**Go idiom note**: The preferred pattern is constructor injection — callers
receive a `TmuxStatePort` interface, not a concrete type fetched from a
package-level var. `GetServerRegistry` is a convenience shim for production
wiring only (e.g., `RunWithName` defaulting to the standard socket). It must
never be called from `init()`. The var is initialised lazily on first call,
protected by `defaultRegistryMu`. Tests construct a `TmuxServerRegistry`
directly via `NewTmuxServerRegistry` and inject it — they do not use the
global accessor.

---

### `TmuxSession` Changes

**File**: `session/tmux/tmux.go`

Add optional field:
```go
registry SessionExistenceChecker  // nil = use exec fallback
```

Modify `DoesSessionExist()`:
```go
func (t *TmuxSession) DoesSessionExist() bool {
    if t.registry != nil && t.registry.IsHealthy() {
        return t.registry.SessionExists(t.sanitizedName)
    }
    // existing exec.Command path (unchanged)
    ...
}
```

Modify constructors (`NewTmuxSession`, etc.) to call
`GetServerRegistry(serverSocket)` and assign to the new field.

**Invariant**: when registry is nil or unhealthy, behaviour is identical
to today's implementation. No regression is possible.

---

### `Multiplexer` Changes

**File**: `session/mux/multiplexer.go`

Add field:
```go
paneExitSub tmux.PaneExitSubscriber  // nil = use polling fallback
```

Replace `monitorTmuxSession()` polling loop using idiomatic channel-based
signaling (see Go Idioms section — channels for ownership transfer, not callbacks):

```go
// Receiver name: m (consistent throughout)
func (m *Multiplexer) startSessionMonitor() {
    if m.paneExitSub != nil {
        // SubscribePaneExit returns a channel closed on pane exit.
        // Passing m.ctx propagates cancellation; the registry cleans up
        // the subscription when the context is done.
        exitCh := m.paneExitSub.SubscribePaneExit(m.ctx, m.tmuxSession)
        go func() {
            select {
            case <-exitCh:
                m.Shutdown()
            case <-m.ctx.Done():
                // context cancelled; registry already cleaned up subscription
            }
        }()
        return
    }
    // fallback: existing ticker loop (unchanged code path)
    m.wg.Add(1)
    go m.monitorTmuxSession()
}
```

Constructor receives `PaneExitSubscriber` via options pattern or direct
parameter; defaults to `tmux.GetServerRegistry("")`.

---

### `PTYDiscovery` Changes

**File**: `session/pty_discovery.go`

Add field:
```go
sessionLister tmux.SessionLister  // nil = use exec fallback
```

Modify `discoverOrphanedPTYs()` and `discoverExternalClaude()` to call
`sessionLister.ListSessions()` instead of `exec.Command("tmux",
"list-sessions", ...)`.

Optionally, stop the `monitorLoop` ticker for the tmux-list portion and
subscribe to `%sessions-changed` events from the registry to trigger
targeted refreshes — this eliminates the 5s polling for session
additions/removals entirely. Full `ps` scan for process classification
still runs on change event (not on every tick).

---

## Known Issues

### Potential Bug: Race Between Registry Startup and First DoesSessionExist Call

**Severity**: Medium

**Description**: On process startup, `GetServerRegistry()` is called
during `NewTmuxSession`. The registry's control-mode process needs a
second or two to connect and populate its session map. During that window
`IsHealthy()` returns false and callers fall back to exec. However, if a
caller caches the `SessionExists(name) == false` result from the fallback
and the registry becomes healthy shortly after with a stale map, the
first map hit could return false for an existing session.

**Mitigation**:
- Registry populates its map from a `list-sessions` exec during startup
  before marking itself `healthy = true`
- `DoesSessionExist` checks `IsHealthy()` on every call (no caching of
  the delegation decision)
- The existing `existsCache` in `TmuxSession` is retained as the final
  TTL guard

**Files affected**: `session/tmux/server_registry.go`,
`session/tmux/tmux.go`

---

### Potential Bug: Missing `%pane-exited` in Some tmux Versions

**Severity**: Medium

**Description**: The `%pane-exited` notification is documented in tmux
3.2+ but its exact payload format varies. Older tmux versions may emit
`%session-closed` without `%pane-exited`. Multiplexer shutdown relies on
at least one of these events.

**Mitigation**:
- Parse both `%pane-exited` and `%session-closed` to fire callbacks
- In `startControlMode`, check `tmux -V` and log a warning if < 3.2;
  fall back to polling for that session
- Add integration test that exercises both event paths

**Files affected**: `session/tmux/server_registry.go`

---

### Potential Bug: Callback Goroutine Leak on Registry Shutdown

**Severity**: Low

**Description**: `SubscribePaneExit` stores a channel in
`subscribers[sessionName]`. If the registry shuts down before the session
exits, the channel is never closed and the Multiplexer's goroutine
(`select { case <-exitCh: ... case <-m.ctx.Done(): ... }`) would leak if
the registry context is not the same as the multiplexer's context.

**Mitigation**:
- `SubscribePaneExit` accepts `ctx context.Context`; when the caller's
  context is cancelled, the registry removes the subscription and closes
  the channel immediately — no goroutine leak is possible as long as the
  caller passes its own context
- On registry `Stop()` (context cancellation), all pending subscriber
  channels are closed outside `subsMu` (copy-under-lock pattern)
- Multiple `close` calls on the same channel are guarded by removing the
  entry from the map before closing

**Files affected**: `session/tmux/server_registry.go`

---

### Potential Bug: Duplicate Session Names Across Sockets

**Severity**: Low

**Description**: `TmuxServerRegistry` is keyed by `serverSocket`. If two
registry instances exist (default socket and a test socket) they are
independent, which is correct. But the global accessor
`GetServerRegistry("")` must not be shared with isolated-socket sessions.

**Mitigation**:
- `GetServerRegistry(socket)` stores registries in a `map[string]*TmuxServerRegistry`
  keyed by socket string
- `NewTmuxSessionWithServerSocket` creates a registry for its socket, not
  the default one
- Test helpers that call `NewTmuxSessionWithServerSocket` automatically
  get an isolated registry

**Files affected**: `session/tmux/server_registry.go`

---

### Potential Bug: `%sessions-changed` Flood on High-Churn Environments

**Severity**: Low

**Description**: During batch session creation (tests, bulk startup),
each session creation emits `%sessions-changed`. Each triggers one
`list-sessions` exec. With 10 sessions created in rapid succession this
is 10 execs — better than today's 500ms polling but still a burst.

**Mitigation**:
- Debounce `%sessions-changed` refreshes with a 50ms cooldown timer
- A pending refresh cancels when a new event arrives within the window;
  only one `list-sessions` runs at the end of the burst

**Files affected**: `session/tmux/server_registry.go`

---

### Potential Bug: Deadlock from Closing Subscriber Channels Under Lock [SEVERITY: High]

**Description**: This is a Go-specific concurrency hazard. If the registry
holds `subsMu` while calling `close(ch)` on a subscriber channel, and the
subscriber goroutine is simultaneously blocked waiting for `subsMu` (e.g.,
in a concurrent `SubscribePaneExit` call), the two goroutines deadlock. Even
without direct mutex contention, closing a channel while holding a lock can
produce subtle ordering bugs that `go test -race` may or may not surface.

**Mitigation**:
- The event-processing loop must copy the subscriber channels out of the map
  **under** `subsMu`, then release the lock, then close each channel outside
  the lock:

```go
r.subsMu.Lock()
chs := r.subscribers[sessionName]
delete(r.subscribers, sessionName)
r.subsMu.Unlock()
// Lock is NOT held here. close() called outside the critical section.
for _, ch := range chs {
    close(ch)
}
```

- This pattern is enforced via code review checklist and a `go test -race`
  integration test that simultaneously creates subscriptions and fires events.
- The same rule applies to registry shutdown: `Stop()` must copy all
  subscriber channels, clear the map, release `subsMu`, then close all channels.

**Files affected**: `session/tmux/server_registry.go`

---

## Go Idioms

This section maps Go-specific best practices to the concrete types and files in
this feature. Every implementation task below references these by name.

### Interface Placement

Interfaces are defined in the **consuming** package, not the producer
(`session/tmux/registry_port.go` is in the `tmux` package, which is the
package that also contains `TmuxSession` — the primary consumer). Callers in
`session/mux` and `session/` receive the narrower interface they need via
constructor injection, so they depend on the `tmux` package's interface
declaration, not on the concrete `TmuxServerRegistry` type.

Compile-time interface satisfaction checks go in `server_registry.go`:

```go
var _ SessionExistenceChecker = (*TmuxServerRegistry)(nil)
var _ SessionLister            = (*TmuxServerRegistry)(nil)
var _ PaneExitSubscriber       = (*TmuxServerRegistry)(nil)
var _ TmuxStatePort            = (*TmuxServerRegistry)(nil)
```

Single-method interfaces use the `-er` suffix (`SessionExistenceChecker`,
`SessionLister`) per Go convention.

### Channel-Based Subscriptions (not callbacks)

`PaneExitSubscriber` must be redesigned to transfer ownership via a channel
rather than registering a callback function. Channels are the idiomatic Go
mechanism for signaling, and they compose naturally with `select`:

```go
// PaneExitSubscriber delivers a channel that is closed when the named pane exits.
// The caller selects on the returned channel alongside its own ctx.Done().
// Cancelling ctx unregisters the subscription; the channel is then closed
// immediately without firing. Reading from a closed channel returns immediately.
type PaneExitSubscriber interface {
    SubscribePaneExit(ctx context.Context, sessionName string) <-chan struct{}
}
```

The registry allocates a `make(chan struct{})` per subscription and stores it in
`subscribers[sessionName]`. On the relevant event it calls `close(ch)` — a
single close signals all `range` / `<-ch` readers without needing a loop.

This replaces the `cb func()` + `cancel func()` pair from the original design.

### Context Propagation

Every method that performs I/O or spawns goroutines accepts `context.Context`
as its first parameter:

```go
func (r *TmuxServerRegistry) Start(ctx context.Context) error
func (r *TmuxServerRegistry) SubscribePaneExit(ctx context.Context, sessionName string) <-chan struct{}
```

Immediately after acquiring a derived context:

```go
ctx, cancel := context.WithTimeout(parent, 2*time.Second)
defer cancel()
```

Every goroutine inside the registry selects on `ctx.Done()` as its exit
condition. No goroutine is permitted to block indefinitely without a documented
exit path.

### Error Wrapping

All errors returned from registry methods wrap their cause with context:

```go
fmt.Errorf("start control mode for socket %q: %w", r.serverSocket, err)
```

Sentinel errors are declared at package level:

```go
var ErrRegistryUnavailable = errors.New("tmux registry unavailable")
```

Internal errors that cannot be surfaced to callers (event-parse failures,
background reconnect failures) must be **logged**, not silently dropped.
`IsHealthy()` returning false is not a substitute for logging root cause.

### Receiver Naming

Consistent short receiver names throughout:
- `TmuxServerRegistry` methods: `r`
- `Multiplexer` methods: `m`
- `TmuxSession` methods: `t`
- `PTYDiscovery` methods: `pd`

### Anti-Patterns to Avoid

**No global mutable state.** The `GetServerRegistry` accessor documented in the
Component Design section is a convenience shim only. The preferred path is
constructor injection. If the global accessor is retained, it must be
initialised lazily under a `sync.Once` — never in `init()`. Tests must not
rely on the global; they construct a `TmuxServerRegistry` directly via
`NewTmuxServerRegistry` and inject it.

**No goroutine variable capture bugs.** Any loop that spawns goroutines must
capture the loop variable explicitly:

```go
for _, name := range names {
    name := name  // capture
    go func() { ... use name ... }()
}
```

Document this explicitly wherever the event-parsing loop fires subscriber
channels.

**Documented goroutine exit conditions.** Every goroutine spawned by the
registry has exactly one exit path: cancellation of the context passed to
`Start`. The reconnect loop, the stdout-reader goroutine, and the
`%sessions-changed` debounce timer goroutine all select on `ctx.Done()`.

### Testing

Table-driven tests are required for in-memory state transitions (event →
sessions map → subscriber channel state). Use `t.Cleanup` to stop registry
goroutines so subtests do not leak:

```go
func TestRegistry_EventParsing(t *testing.T) {
    r := NewTmuxServerRegistry("")
    ctx, cancel := context.WithCancel(context.Background())
    t.Cleanup(cancel)
    r.Start(ctx)
    ...
}
```

`go test -race ./...` must pass for all tasks that touch shared state (T2 and
T6 are the highest-risk; all tasks must satisfy this requirement).

---

## Implementation Tasks

### Task dependencies

```
T1 (interfaces + stubs)
  └─> T2 (server_registry core)
        └─> T3 (TmuxSession wiring)
              └─> T4 (Multiplexer migration)
              └─> T5 (PTYDiscovery migration)
        └─> T6 (integration tests)
              └─> T4, T5
```

---

### T1: Define Registry Interfaces and Fake

**Effort**: small (1-2h)
**Files**: `session/tmux/registry_port.go` (new),
`session/tmux/fake_registry_test.go` (new)

Create the three interfaces (`SessionExistenceChecker`, `SessionLister`,
`PaneExitSubscriber`) and the combined `TmuxStatePort` in
`registry_port.go`. All interfaces are defined in the `tmux` package (the
consuming package), not in a separate `registry` package.

`PaneExitSubscriber.SubscribePaneExit` accepts `context.Context` as its
first parameter and returns `<-chan struct{}` (channel-based signaling, not
a callback — see Go Idioms section).

Create `FakeTmuxRegistry` in `fake_registry_test.go` that implements
`TmuxStatePort` with an in-memory map, for use in unit tests of T3–T5.
Add compile-time interface satisfaction checks in `fake_registry_test.go`:

```go
var _ tmux.TmuxStatePort = (*FakeTmuxRegistry)(nil)
```

Acceptance criteria:
- `go build ./session/tmux/...` passes
- `FakeTmuxRegistry` implements `TmuxStatePort` (compile-time check)
- No existing tests break
- `go test -race ./session/tmux/...` passes (no state in interfaces, but
  establishes the baseline for T2)

---

### T2: Implement `TmuxServerRegistry` Core

**Effort**: medium (3-4h)
**Files**: `session/tmux/server_registry.go` (new),
`session/tmux/server_registry_test.go` (new)

Implement `TmuxServerRegistry` with:
1. `Start(ctx context.Context) error` — launch `tmux [-L socket] -C
   attach-session -t <keepalive> -r`, parse output lines, populate
   `sessions` map from `list-sessions` before marking healthy. Use
   `errgroup` for the parallel stdout-reader and reconnect goroutines.
   Use `ctx, cancel := context.WithTimeout(...); defer cancel()` for the
   initial `list-sessions` bootstrap call.
2. `Stop()` — cancel the registry's context; all goroutines exit via
   `ctx.Done()`. Copy subscriber channels out from under `subsMu`, then
   close each outside the lock (deadlock prevention — see Known Issues).
3. `SessionExists(name string) bool`, `ListSessions() map[string]bool`,
   `SubscribePaneExit(ctx context.Context, sessionName string) <-chan struct{}`
4. Auto-reconnect goroutine — selects on `ctx.Done()` and a backoff
   timer; exponential backoff (100ms base, 30s cap). Documented exit
   condition: `ctx.Done()`.
5. `GetServerRegistry(socket string)` global accessor — lazy init under
   `defaultRegistryMu`; never called from `init()`.
6. Compile-time checks: `var _ SessionExistenceChecker = (*TmuxServerRegistry)(nil)` etc.

Go idioms in this task:
- Receiver name `r` throughout
- `fmt.Errorf("start control mode for %q: %w", r.serverSocket, err)` error wrapping
- `var ErrRegistryUnavailable = errors.New("tmux registry unavailable")` sentinel
- Close subscriber channels **outside** `subsMu` (copy-under-lock, close-outside pattern)
- Loop variable capture: `for _, ch := range chs { ch := ch; ... }`

Unit tests (no real tmux needed, use a fake stdout pipe):
- Table-driven tests for event parsing: `%session-created`, `%session-closed`,
  `%sessions-changed`, `%pane-exited`, `%exit` → expected sessions map state
- `SessionExists` returns true after `%session-created`, false after `%session-closed`
- Subscriber channel is closed on `%pane-exited` and `%session-closed`
- Cancelling the subscription context closes the channel immediately without
  waiting for the event
- Unhealthy registry causes `IsHealthy()` to return false
- Use `t.Cleanup(cancel)` to stop registry goroutines after each subtest

Acceptance criteria:
- All unit tests pass
- `go test -race ./session/tmux/...` passes (mandatory for this task)
- `go vet ./session/tmux/...` clean
- `golangci-lint run ./session/tmux/...` clean

---

### T3: Wire Registry into `TmuxSession.DoesSessionExist()`

**Effort**: small (1-2h)
**Files**: `session/tmux/tmux.go`,
`session/tmux/tmux_test.go` (extend)

Add `registry SessionExistenceChecker` field to `TmuxSession`. Receiver
name `t` throughout (consistent with existing code).

Modify `newTmuxSessionWithSocket()` to call
`GetServerRegistry(serverSocket)` and assign the result.

Add `WithRegistry(r SessionExistenceChecker)` option so tests can inject
a `FakeTmuxRegistry` — tests must not use the global accessor.

Modify `DoesSessionExist()`:
```go
// t is the receiver (TmuxSession)
if t.registry != nil && t.registry.IsHealthy() {
    return t.registry.SessionExists(t.sanitizedName)
}
// existing path unchanged
```

Go idioms in this task:
- Constructor injection via `WithRegistry` option; no package-level state
  touched in tests
- `IsHealthy()` called on every invocation (no caching the delegation
  decision)

Add unit test: `DoesSessionExist` returns fake registry result when
healthy, falls back to exec path when `IsHealthy()` is false.

Acceptance criteria:
- All existing `tmux_test.go` tests still pass
- New test demonstrates zero-exec path through `DoesSessionExist` when
  registry is healthy
- No change to `DoesSessionExistNoCache()` (critical validation, always
  execs)
- `go test -race ./session/tmux/...` passes

---

### T4: Replace `monitorTmuxSession()` Polling in Multiplexer

**Effort**: small (1-2h)
**Files**: `session/mux/multiplexer.go`,
`session/mux/multiplexer_test.go` (extend)

Add `paneExitSub tmux.PaneExitSubscriber` field to `Multiplexer`. Receiver
name `m` throughout.

Extract the fallback ticker logic from `monitorTmuxSession()` into a
private `monitorTmuxSessionPolling()` method (rename, no logic change).

Add `startSessionMonitor()` as described in the design section above.
The implementation uses channel-based signaling (not a callback):

```go
// m is the receiver (Multiplexer)
func (m *Multiplexer) startSessionMonitor() {
    if m.paneExitSub != nil {
        exitCh := m.paneExitSub.SubscribePaneExit(m.ctx, m.tmuxSession)
        go func() {
            select {
            case <-exitCh:
                m.Shutdown()
            case <-m.ctx.Done():
            }
        }()
        return
    }
    m.wg.Add(1)
    go m.monitorTmuxSessionPolling()
}
```

The goroutine's exit condition is documented: it exits on either `exitCh`
close or `m.ctx.Done()`, whichever comes first.

Call `startSessionMonitor()` from `Start()` instead of
`go m.monitorTmuxSession()`.

Wire `tmux.GetServerRegistry("")` as the default subscriber in
`RunWithName()`.

Add a constructor option `WithPaneExitSubscriber(s tmux.PaneExitSubscriber)`
for test injection — tests inject `FakeTmuxRegistry` directly.

Unit test: verify `Shutdown()` is called exactly once when the fake
registry closes the pane-exit channel. Use `t.Cleanup` to cancel the
multiplexer context after each subtest.

Acceptance criteria:
- All existing `multiplexer_test.go` tests pass
- New test demonstrates no `exec.Command` fork for session monitoring when
  subscriber is provided
- Fallback polling path still reachable (used in tests with nil subscriber)
- `go test -race ./session/mux/...` passes

---

### T5: Replace `exec.Command("tmux", "list-sessions")` in `PTYDiscovery`

**Effort**: medium (2-3h)
**Files**: `session/pty_discovery.go`, `session/pty_discovery_test.go`
(new or extend)

Add `sessionLister tmux.SessionLister` field to `PTYDiscovery`. Receiver
name `pd` throughout.

Modify `discoverOrphanedPTYs()`: replace the `exec.Command("tmux",
"list-sessions", ...)` call with `pd.sessionLister.ListSessions()` (when
non-nil and healthy). The per-session `display-message` and `ps` calls
are unchanged.

Modify `discoverExternalClaude()` similarly.

Add `WithSessionLister(l tmux.SessionLister)` option to
`NewPTYDiscoveryWithConfig`. Tests inject `FakeTmuxRegistry` — they do
not use the global accessor.

Wire `tmux.GetServerRegistry("")` as default lister in `NewPTYDiscovery`.

Go idioms in this task:
- `pd.sessionLister.ListSessions()` returns a copy of the map; no lock held
  by the caller. `PTYDiscovery` must not retain the returned map between
  calls (treat it as a snapshot).
- Internal errors from `ListSessions` when unhealthy are logged, not silently
  swallowed; the fallback exec path produces its own errors separately.

Unit test with `FakeTmuxRegistry` (table-driven): `discoverOrphanedPTYs`
returns connections only for sessions reported by the lister (no exec needed).

Acceptance criteria:
- All existing PTY discovery tests pass
- New test demonstrates zero `list-sessions` exec when lister is healthy
- `ps` and `display-message` execs are unaffected (still present)
- `go test -race ./session/...` passes

---

### T6: Integration Test — Registry with Real tmux

**Effort**: medium (2-3h)
**Files**: `session/tmux/server_registry_integration_test.go` (new)

Use the existing `NewTmuxSessionWithServerSocket` pattern (isolated socket
per test) to create a real but isolated tmux server.

Test cases:
1. `TmuxServerRegistry` starts and becomes healthy within 2 seconds
2. Creating a tmux session results in `SessionExists` returning true
   within 500ms (event-driven, not poll-based)
3. Killing a tmux session closes the `SubscribePaneExit` channel within
   500ms (channel-based, not callback-based)
4. Registry reconnects after the tmux server is killed and restarted
5. `ListSessions()` returns the correct set after multiple
   create/destroy cycles

Go idioms in this task:
- Each test uses `t.Cleanup(cancel)` to cancel the registry context on
  teardown — no explicit `Stop()` call needed in the happy path
- `ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)` for
  health-check assertions
- `go test -race -tags integration ./session/tmux/...` is the canonical
  run command; race detector must pass (this is the highest-risk task for
  the deadlock hazard documented in Known Issues)
- Concurrent subscription test: spawn 10 goroutines each calling
  `SubscribePaneExit` for the same session simultaneously while the event
  loop fires — verifies no deadlock and no double-close panic

Build tag: `//go:build integration` (skipped by default `go test`;
run via `go test -race -tags integration ./session/tmux/...`).

Acceptance criteria:
- All 5 test cases pass against a real tmux binary
- Concurrent subscription test passes under `-race`
- `go test -race ./session/tmux/...` (unit tests) also passes in this task
- Tests clean up their isolated socket on success and failure
- Test file is excluded from regular `make test` by build tag

---

## Migration Checklist (post-implementation)

- [x] T1 merged — interfaces defined, no behaviour change (5e474b0)
- [x] T2 merged — registry available, not yet used (2bc47ac)
- [x] T3 merged — `DoesSessionExist()` uses registry (2b8919f)
- [x] T4 merged — `monitorTmuxSession()` replaced (017f0b1)
- [x] T5 merged — PTYDiscovery uses registry (e987331)
- [x] T6 merged — integration tests green in CI (ca78293)
- [ ] One release cycle in production; confirm cgroup throttle counter no
      longer climbs
- [ ] Remove the polling fallback methods in a follow-up cleanup PR once
      registry stability is proven over 2 weeks

---

## Non-Goals

- Replacing per-session control mode connections used for `%output`
  streaming (those are kept as-is)
- Eliminating `display-message` calls for pane dimensions, cursor
  position, and pane path (single-shot queries, not hot loops)
- Handling multiple tmux servers other than the default and
  test-isolation sockets (exotic multi-server setups are out of scope)

---

## Success Metrics

| Metric | Before | Target |
|---|---|---|
| systemd cgroup `EAGAIN` throttle events / hour | ~144,000 (extrapolated) | < 500 |
| `exec.Command("tmux", "list-sessions")` calls / sec | ~8 | 0 (healthy registry) |
| `exec.Command("tmux", "has-session")` calls / sec | ~2/session (~20 total) | 0 |
| `exec.Command("tmux", "list-panes")` calls / sec | ~2/session (~20 total) | 0 |
| Registry reconnect time after tmux server restart | N/A | < 5 seconds |
