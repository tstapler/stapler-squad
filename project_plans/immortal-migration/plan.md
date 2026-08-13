# Plan: Immortal Migration — Process Manager Modularization

Status: Updated | Phase: 3 — Planning
Created: 2026-05-14
Updated: 2026-05-22
Inputs: requirements.md, research/synthesis.md, research/findings-architecture.md,
        research/findings-pitfalls.md, codebase audit (2026-05-22)

## Summary

Extract a backend-agnostic `ProcessManager` interface from the existing `TmuxManager` seam,
add OpenFeature SDK for config-driven backend selection, and wire `Instance` to depend on the
interface rather than the concrete struct. Tmux remains the default; `NativeProcessManager`
with PTY + restart loop is Phase 2.

---

## Verified Codebase Facts (2026-05-22 audit)

- `session/instance.go:248`: field is `tmuxManager TmuxProcessManager` — **concrete struct**, not interface
- `TmuxManager` interface: `session/tmux_process_manager.go:353–391`, 30 methods
- `tmuxManager` call sites: **89 total** (grep count), in `session/` package
- `DoesSessionExist()` callers in `instance.go`: lines 580, 1022, 1509, 1655, 2373, 2421, 2456
- `GetTmuxSessionName()` callers: `instance.go:908`, `review_queue_poller.go:406,550,576`,
  `session.go:184` — **NOT** pty_discovery.go (research was inaccurate on that point)
- `pty_discovery.go`: zero occurrences of `GetTmuxSessionName` — no type-assert needed there
- `comprehensive_session_creation_test.go:226`: accesses `instance.tmuxManager.session` directly
- `open-feature/go-sdk`: **NOT** in go.mod — must be added with `go get`
- `creack/pty v1.1.24`: already in go.mod (line 14)
- `TmuxManager` interface leaks: `Session() *tmux.TmuxSession` and `SetSession(*tmux.TmuxSession)`
- `DoesSessionExist()` is effectively a duplicate of `IsAlive()` — kept in TmuxManager for
  internal use but excluded from ProcessManager interface

---

## Phase 1: Interface Extraction + OpenFeature Wiring (zero behavior change)

**Goal:** `Instance` holds a `ProcessManager` interface field. OpenFeature reads
`process_manager_backend` from `config.json` and returns the appropriate backend at startup.
All existing tests pass unchanged.

### Story 1.1 — Create `session/process_manager.go`

New file. Defines the `ProcessManager` interface that `Instance` actually calls, hiding
tmux-specific types. Every method maps 1:1 to an existing `TmuxManager` method, with
three exceptions:

- `GetSessionIdentifier() string` — replaces `GetTmuxSessionName()` for backend-agnostic callers
- `Session()` / `SetSession()` — excluded (tmux type leaks); accessed via `TmuxBackend.TmuxManager()`
- `DoesSessionExist()` — excluded from ProcessManager; instance.go callers get mapped to `IsAlive()`

```go
package session

import "os"

// ProcessManager abstracts terminal process lifecycle and I/O.
// Implementations: TmuxBackend (wraps TmuxProcessManager), NativeProcessManager (Phase 2).
type ProcessManager interface {
    // Lifecycle
    Start(dir string) error
    RestoreWithWorkDir(workDir string) error
    Close() error
    IsAlive() bool

    // Identification
    GetSessionIdentifier() string

    // Existence / state
    HasSession() bool

    // Working directory
    GetCurrentWorkingDirectory() (string, error)

    // Terminal I/O
    GetPTY() (*os.File, error)
    SendKeys(keys string) (int, error)
    TapEnter() error
    SendPromptWithEnter(prompt string) error

    // Terminal state
    CapturePaneContent() (string, error)
    CapturePaneContentRaw() (string, error)
    CapturePaneContentWithOptions(startLine, endLine string) (string, error)
    CaptureViewport(lines int) (string, error)
    GetCursorPosition() (x, y int, err error)
    GetPaneDimensions() (width, height int, err error)
    SetWindowSize(cols, rows int) error
    SetDetachedSize(width, height int, instanceTitle string) error
    RefreshClient() error

    // Process metadata
    GetPanePID() (int32, error)

    // Content helpers
    HasUpdated() (updated bool, hasPrompt bool, content string)
    FilterBanners(content string) (string, int)
    HasMeaningfulContent(content string) bool

    // Streaming (control mode)
    StartControlMode() error
    StopControlMode() error
    SubscribeToControlModeUpdates() (string, chan []byte)
    UnsubscribeFromControlModeUpdates(id string)

    // Attach (interactive TUI)
    Attach() (chan struct{}, error)
    DetachSafely() error

    // Exit notifications
    SetOnExitCallback(fn func(string))
    ResetExitOnce()
}

// ProcessManagerBackend identifies the backend implementation.
type ProcessManagerBackend string

const (
    BackendTmux   ProcessManagerBackend = "tmux"
    BackendNative ProcessManagerBackend = "native"
)

// ProcessManagerOptions holds constructor parameters for NewProcessManager.
type ProcessManagerOptions struct {
    SessionName  string
    Prefix       string
    ServerSocket string
}
```

**Compile-time check added in tmux_backend.go (Story 1.2).**

### Story 1.2 — Create `session/tmux_backend.go`

New file. `TmuxBackend` implements `ProcessManager` by delegating 1:1 to `TmuxManager`.
Provides a `TmuxManager()` accessor for the small set of callers that need type-assertion
access to tmux-specific operations (review_queue_poller.go reconciliation path).

`GetSessionIdentifier()` delegates to `b.mgr.GetTmuxSessionName()` — this is the only
name-mapping method.

```go
package session

// TmuxBackend implements ProcessManager by delegating to TmuxManager.
type TmuxBackend struct {
    mgr TmuxManager
}

func NewTmuxBackend(mgr TmuxManager) *TmuxBackend {
    return &TmuxBackend{mgr: mgr}
}

// TmuxManager returns the underlying TmuxManager for type assertions in
// reconciliation paths that need tmux-specific operations.
func (b *TmuxBackend) TmuxManager() TmuxManager { return b.mgr }

func (b *TmuxBackend) GetSessionIdentifier() string { return b.mgr.GetTmuxSessionName() }
// ... all other ProcessManager methods delegate directly to b.mgr

// compile-time check
var _ ProcessManager = (*TmuxBackend)(nil)
```

### Story 1.3 — Add OpenFeature SDK + create `session/backend_factory.go`

**Prerequisite:** `go get github.com/open-feature/go-sdk@latest`

New file. Provides `RegisterBackendProvider` (called once at startup) and `NewProcessManager`
(called per-instance). Uses the built-in `InMemoryProvider` seeded from config — no external
flag service needed.

```go
package session

// RegisterBackendProvider seeds the OpenFeature SDK with the backend flag.
// Call once at startup, before any ProcessManager is created.
func RegisterBackendProvider(backend ProcessManagerBackend)

// NewProcessManager returns the ProcessManager implementation selected by the
// OpenFeature "process-manager-backend" flag. Panics on "native" until Phase 2.
func NewProcessManager(ctx context.Context, defaultBackend ProcessManagerBackend, opts ProcessManagerOptions) ProcessManager
```

Factory switch:
- `"tmux"` → `NewTmuxBackend(newTmuxProcessManager(opts))`
- `"native"` → `panic("native backend not yet implemented")` (explicit gate; Phase 2 removes panic)
- default → tmux

### Story 1.4 — Wire `config/config.go` + `main.go`

Add to `Config` struct in `config/config.go`:
```go
// ProcessManagerBackend selects the process manager implementation.
// Valid values: "tmux" (default), "native" (Phase 2).
// Empty string is backwards-compatible and defaults to "tmux".
ProcessManagerBackend string `json:"process_manager_backend,omitempty"`
```

In `main.go` startup (after config load):
```go
backend := session.ProcessManagerBackend(cfg.ProcessManagerBackend)
if backend == "" {
    backend = session.BackendTmux
}
session.RegisterBackendProvider(backend)
```

In `Instance` initialization (where `TmuxProcessManager` is currently constructed):
```go
instance.processManager = session.NewProcessManager(ctx, backend, session.ProcessManagerOptions{
    SessionName:  instance.Title,
    Prefix:       instance.TmuxPrefix,
    ServerSocket: instance.TmuxServerSocket,
})
```

### Story 1.5 — Update `session/instance.go` (field + ~89 call sites)

**This is the largest single change.**

1. Change `tmuxManager TmuxProcessManager` → `processManager ProcessManager`
2. Rename all 89 occurrences of `i.tmuxManager.X()` → `i.processManager.X()`
3. Map `DoesSessionExist()` callers to `IsAlive()` — confirmed identical semantics for
   the checked paths (lines 580, 1022, 1509, 1655, 2373, 2421, 2456)
4. Update `GetTmuxSessionName()` method on `Instance` (line 907): now calls
   `i.processManager.GetSessionIdentifier()` internally; the public `GetTmuxSessionName()`
   method on `Instance` is kept as-is because `review_queue_poller.go` calls it on the
   `*Instance` struct (not on the interface) — only the internal delegation changes
5. The three `review_queue_poller.go` callers (lines 406, 550, 576) call `inst.GetTmuxSessionName()`
   on `*Instance` — no change needed there; `Instance.GetTmuxSessionName()` becomes a wrapper
   around `GetSessionIdentifier()`

**Compile check after each file change:** `go build ./session/...`

### Story 1.6 — Reduce tmux-type leaks from `TmuxManager` interface (Phase 1 scope)

In `session/tmux_process_manager.go`, remove only `Session() *tmux.TmuxSession` and
`SetSession(*tmux.TmuxSession)` from the `TmuxManager` interface — but only after migrating
all call sites.

**Call sites for Session() in instance.go:** lines 2081, 2459 (and via `GetTmuxSession()`)
**Call sites for SetSession() in instance.go:** lines 563, 565, 574, 576, 1182, 1819, 1821, 2111

**Phase 1 strategy (pragmatic):** Keep `Session()` and `SetSession()` as exported methods on the
`TmuxProcessManager` concrete struct (not in the TmuxManager interface). The 3 call sites in
instance.go that call `Session()` will use type-assertion:
```go
// Before:
tmuxSession := i.tmuxManager.Session()
// After:
tmuxSession := i.processManager.(*TmuxBackend).TmuxManager().(*TmuxProcessManager).Session()
```
The `SetSession()` callers in `FromInstanceData()` and `start()` similarly type-assert.

**Defer to Phase 3:** Full cleanup of SetSession() calls — moving initialization logic into
`NewProcessManager()` factory so instance.go no longer calls SetSession directly.

**Keep** `DoesSessionExist()` and `HasSession()` in TmuxManager — both are used internally.
`HasSession()` is now also in ProcessManager (Story 1.1). TmuxBackend delegates to `b.mgr.HasSession()`.

### Story 1.7 — Fix `session/comprehensive_session_creation_test.go`

Line 226 accesses `instance.tmuxManager.session` directly — a concrete struct field.
After Story 1.5 renames `tmuxManager` to `processManager`, this breaks.

Fix: type-assert to `*TmuxBackend` to get to `TmuxManager()`, then set session:
```go
// Before:
instance.tmuxManager.session = mockTmuxSession
// After:
instance.processManager.(*TmuxBackend).TmuxManager().(*TmuxProcessManager).SetSession(mockTmuxSession)
// OR: expose a test helper Init method on TmuxBackend
```

Preferred fix: add `func (b *TmuxBackend) SetTmuxSession(s *tmux.TmuxSession)` that is
test-only (build tag `//go:build testing` or just package-internal acceptance).

---

## Phase 2: NativeProcessManager (deferred; unblock after Phase 1 lands)

**Goal:** A working NativeProcessManager that satisfies ProcessManager, uses `creack/pty`
for PTY launch, and restarts the process after unexpected exit with exponential backoff.
Config `process_manager_backend: "native"` routes to it.

### Story 2.1 — Create `session/native_process_manager.go`

New file. `NativeProcessManager` struct:

```go
type NativeProcessManager struct {
    opts      ProcessManagerOptions
    cmd       *exec.Cmd
    ptm       *os.File        // PTY master fd
    mu        sync.Mutex      // guards cmd + ptm reassignment on restart
    stopCh    chan struct{}    // closed by Close(); stops restart loop
    lastSize  *pty.Winsize    // tracks last-set terminal size; reapplied on restart
    onExit    func(string)    // called after unexpected exit (before restart)

    // subscriber fan-out
    subsMu sync.Mutex
    subs   map[string]chan []byte
}
```

Implements `ProcessManager` interface. Compile-time check:
```go
var _ ProcessManager = (*NativeProcessManager)(nil)
```

### Story 2.2 — Implement `Start()` / PTY launch in NativeProcessManager

```go
func (n *NativeProcessManager) Start(dir string) error {
    n.mu.Lock()
    defer n.mu.Unlock()

    cmd := exec.Command(n.opts.Program, n.opts.Args...) //nolint:norawexec long-running PTY process; WaitDelay and ManagedProcess are pipe-based only, not PTY-compatible
    cmd.Dir = dir
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

    ptm, err := pty.Start(cmd)
    if err != nil {
        return err
    }
    if n.lastSize != nil {
        _ = pty.Setsize(ptm, n.lastSize)
    }
    n.cmd = cmd
    n.ptm = ptm

    go n.supervise()
    go n.fanOut()

    return nil
}
```

The `//nolint:norawexec` comment with justification is required to pass the `norawexec`
lint pass (`tools/lint/norawexec/`). All other subprocess calls (metadata queries) must
use `executor.ShortLivedCmd`.

### Story 2.3 — Implement `supervise()` restart loop

~50 LOC exponential backoff loop. Non-negotiable design commitments (from findings-pitfalls.md):

- **NM-1**: `cmd.Wait()` is the sole owner; never skip it (zombie prevention)
- **NM-2**: `startMu` with double-checked locking on Start()
- **NM-3**: `Close()` iterates and sends SIGTERM before context cancellation
- **NM-4**: `Setpgid: true` in SysProcAttr (already in Story 2.2)
- **NM-5**: All goroutines exit when `stopCh` is closed
- **NM-7**: On crash, fire `onExit` callback (EventExited lifecycle event) then restart

```go
func (n *NativeProcessManager) supervise() {
    backoff := 500 * time.Millisecond
    for {
        err := n.cmd.Wait()
        select {
        case <-n.stopCh:
            return // intentional stop; do not restart
        default:
        }
        if err != nil {
            if n.onExit != nil {
                n.onExit("crash: " + err.Error())
            }
        }
        time.Sleep(backoff)
        backoff = min(backoff*2, 30*time.Second)
        n.mu.Lock()
        _ = n.launchPTY()
        n.mu.Unlock()
    }
}
```

### Story 2.4 — Implement key ProcessManager methods for NativeProcessManager

- `GetSessionIdentifier()`: return `n.opts.SessionName` (stable, set at construction)
- `GetPaneDimensions()`: return last-set size from `n.lastSize` (track rather than query;
  avoids TIOCGWINSZ syscall on hot path — GetPaneDimensions is called 5× per resize)
- `SetWindowSize(cols, rows int)`: set `n.lastSize`, call `pty.Setsize(n.ptm, size)`
- `GetCursorPosition()`: return `(0, 0)` — confirmed zero callers in `server/` package
- `SubscribeToControlModeUpdates()`: add to `n.subs`, return ID + `chan []byte`
- `UnsubscribeFromControlModeUpdates(id)`: delete from `n.subs`
- `StartControlMode()` / `StopControlMode()`: no-op for native backend (raw PTY fan-out
  replaces control mode — the `fanOut()` goroutine handles byte distribution)
- `Close()`: send SIGTERM to process group via `syscall.Kill(-n.cmd.Process.Pid, syscall.SIGTERM)`,
  close `stopCh`, then wait for supervise() to return
- `IsAlive()`: check if `n.cmd` is non-nil and process has not exited (non-blocking select on done channel)

Methods that require deeper implementation (deferred to Phase 2 follow-on stories):
- `CapturePaneContent()` / `CapturePaneContentRaw()` / `CaptureViewport()`: read from scrollback
- `HasUpdated()` / `HasMeaningfulContent()` / `FilterBanners()`: operate on scrollback content
- `GetPanePID()`: return `int32(n.cmd.Process.Pid)` directly — no ShortLivedCmd needed; the
  PID is already on the struct from `cmd.Start()`
- `GetCurrentWorkingDirectory()`: use `executor.ShortLivedCmd` to run `lsof -p <pid>` or
  read `/proc/<pid>/cwd` (Linux) / `lsof -p <pid> +d /` (macOS); alternatively, track the
  workDir passed to `Start()` and update it when the process CWDs change (deferred to follow-on)

Stubs returning zero values or errors are acceptable for the Phase 2 gate — the goal is
demonstrable crash-restart behavior, not full parity with the tmux backend.

### Story 2.5 — Wire NativeProcessManager in `session/backend_factory.go`

Remove `panic("native backend not yet implemented")`. Add:
```go
case BackendNative:
    return NewNativeProcessManager(opts)
```

### Story 2.6 — Test or manual verification of crash-restart

Demonstrate that `NativeProcessManager` restarts after the supervised process is killed.

Option A (unit test — preferred):
```go
func TestNativeProcessManagerRestartsAfterKill(t *testing.T) {
    mgr := NewNativeProcessManager(ProcessManagerOptions{
        SessionName: "test",
        Program:     "bash",
        Args:        []string{"-c", "echo alive; sleep 600"},
    })
    _ = mgr.Start(t.TempDir())
    pid1 := mgr.cmd.Process.Pid
    _ = mgr.cmd.Process.Kill()
    time.Sleep(2 * time.Second)
    pid2 := mgr.cmd.Process.Pid
    assert.NotEqual(t, pid1, pid2, "process should have restarted with new PID")
    _ = mgr.Close()
}
```

Option B: manual verification with `STAPLER_SQUAD_PROCESS_MANAGER=native` + kill the
supervised process + confirm session status returns to Running.

---

## Non-Negotiable Implementation Requirements

From findings-pitfalls.md (must be met before Phase 2 can ship):

| ID | Requirement | Story |
|----|-------------|-------|
| NM-1 | Every `cmd.Start()` paired with `Wait()` goroutine | 2.3 |
| NM-2 | `ProcessManager.Start()` uses double-checked locking | 2.3 |
| NM-3 | `Close()` sends SIGTERM before cancelling context | 2.4 |
| NM-4 | All child processes: `Setpgid: true` in `SysProcAttr` | 2.2 |
| NM-5 | All goroutines exit on context/stopCh cancellation | 2.3 |
| NM-6 | Constructor accepts base directory (test isolation) | 2.1 |
| NM-7 | PTY ownership on restart: fire lifecycle event, not silent fd swap | 2.3 |

---

## Implementation Order

Strict — each step must compile before proceeding:

1. `session/process_manager.go` — interface + constants + types (no dependencies)
2. `session/tmux_backend.go` — TmuxBackend adapter (depends on TmuxManager)
3. `session/tmux_process_manager.go` — remove Session()/SetSession() from TmuxManager interface (Story 1.6)
4. `go get github.com/open-feature/go-sdk`
5. `session/backend_factory.go` — factory + RegisterBackendProvider
6. `config/config.go` — add ProcessManagerBackend field
7. `session/instance.go` — change field + all 89 call sites (compile-check after each method)
8. `main.go` — RegisterBackendProvider at startup
9. `session/comprehensive_session_creation_test.go` — fix direct field access (Story 1.7)
10. `go build ./...` + `make lint` + `go test ./session/...` — Phase 1 gate
11. `session/native_process_manager.go` — NativeProcessManager struct + Start() + supervise()
12. `session/backend_factory.go` — remove native panic
13. Phase 2 test or manual verification

**Verification after every step:** `go build ./...`

---

## Known Pitfalls to Address

| Pitfall | Description | Mitigation | Story |
|---------|-------------|-----------|-------|
| ~89 call site rename misses | One unrenamed call compiles as nil-deref after type change | Compile check + `grep -rn tmuxManager session/` after rename | 1.5 |
| norawexec lint blocks PTY exec | `exec.Command` outside executor/ fails build | `//nolint:norawexec` with justification comment | 2.2 |
| open-feature/go-sdk not in go.mod | Import fails until added | `go get github.com/open-feature/go-sdk` | 1.3 |
| GetPaneDimensions hot path | Called 5× per resize in connectrpc_websocket.go | NativeProcessManager tracks last size, no syscall per-call | 2.4 |
| comprehensive_session_creation_test.go concrete field | `instance.tmuxManager.session = mockTmuxSession` breaks | Type-assert through TmuxBackend | 1.7 |
| DoesSessionExist vs IsAlive | 7 callers in instance.go use DoesSessionExist() | Map to IsAlive() in Story 1.5; explicitly verify line 580 semantics (pre-start instance check) | 1.5 |
| HasSession() missing from ProcessManager | 6 callers in instance.go; not in original interface | Added to ProcessManager in Story 1.1 | 1.1 |
| GetCurrentWorkingDirectory missing | instance.go:2459 calls Session().GetPaneCurrentPath() | Added to ProcessManager; TmuxBackend delegates via Session() | 1.1, 1.6 |
| Session()/SetSession() in TmuxManager | 10 call sites in instance.go; can't simply remove | Phase 1: type-assert at 3 Session() callers; Phase 3: full cleanup | 1.6 |
| review_queue_poller.go 3 callers | Lines 406, 550, 576 call `inst.GetTmuxSessionName()` | Instance.GetTmuxSessionName() becomes a wrapper; no change to callers | 1.5 |
| PTY ownership on restart | Restart creates new ptmx; streamLoop reads stale fd | Option C: fire EventExited, let instance layer reconnect stream | 2.3 |
| Zombie accumulation re-introduction | New supervisor code re-introduces F1 if Wait() is skipped | supervise() owns Wait() and never skips it | 2.3 |
| Double-start race | Supervisor restart callback is a third concurrent caller | startMu in Start(); double-checked locking | 2.3 |
| macOS signal behavior | Setpgid + PTY may behave differently than Linux | Test on macOS explicitly; document any Darwin-specific workarounds | 2.2 |

---

## Files Changed

### Phase 1

| File | Change | Story |
|------|--------|-------|
| `session/process_manager.go` | CREATE — interface + constants + ProcessManagerOptions | 1.1 |
| `session/tmux_backend.go` | CREATE — TmuxBackend adapter + compile check | 1.2 |
| `session/backend_factory.go` | CREATE — factory + RegisterBackendProvider | 1.3 |
| `go.mod` / `go.sum` | EDIT — add `github.com/open-feature/go-sdk` | 1.3 |
| `config/config.go` | EDIT — add `ProcessManagerBackend` field | 1.4 |
| `main.go` | EDIT — RegisterBackendProvider at startup | 1.4 |
| `session/tmux_process_manager.go` | EDIT — remove Session()/SetSession() from TmuxManager interface | 1.6 |
| `session/instance.go` | EDIT — field rename + ~89 call site renames + DoesSessionExist→IsAlive | 1.5 |
| `session/comprehensive_session_creation_test.go` | EDIT — fix direct field access at line 226 | 1.7 |

### Phase 2

| File | Change | Story |
|------|--------|-------|
| `session/native_process_manager.go` | CREATE — NativeProcessManager | 2.1–2.4 |
| `session/backend_factory.go` | EDIT — remove native panic, add native case | 2.5 |
| `session/native_process_manager_test.go` | CREATE — restart integration test | 2.6 |

---

## Test Strategy

| Test | What it proves | Phase |
|------|---------------|-------|
| All existing `session/` tests pass | Zero behavior change in Phase 1 | 1 |
| `var _ ProcessManager = (*TmuxBackend)(nil)` compiles | Adapter is complete | 1 |
| `var _ ProcessManager = (*NativeProcessManager)(nil)` compiles | New backend is complete | 2 |
| `NewProcessManager` returns `TmuxBackend` when flag = "tmux" | Factory routes correctly | 1 |
| `NewProcessManager` panics when flag = "native" (Phase 1) | Explicit gate, not silent failure | 1 |
| Config round-trip: `process_manager_backend: "tmux"` loads | Config wiring works | 1 |
| `TestNativeProcessManagerRestartsAfterKill` | NativeProcessManager actually restarts | 2 |
| Mock `ProcessManager` in Instance unit tests | tmux no longer required for unit tests | 1 |

---

## ADRs Required

Two architecture decision records should be written (via `/plan:adr`) before Phase 1 implementation:

1. **ADR: ProcessManager interface method selection** — why these ~28 methods, why not fewer,
   why `GetSessionIdentifier()` instead of `GetTmuxSessionName()`, why `DoesSessionExist()` is
   excluded from ProcessManager but kept in TmuxManager

2. **ADR: OpenFeature for backend selection** — why not build tags, why not a simple config
   string passed as a constructor parameter, why InMemoryProvider is sufficient for startup-time
   flag evaluation

---

## Open Questions — RESOLVED

- [x] **Web UI wire format?** Raw bytes — tmux octal encoding stripped in `decodeControlModeOutput()` before reaching subscriber channel. NativeProcessManager reads from ptmx and broadcasts raw bytes. No protocol changes.
- [x] **GetCursorPosition hot path?** Zero callers in `server/`. Static `(0,0)` acceptable for Phase 2.
- [x] **GetPaneDimensions hot path?** 5× per resize in `connectrpc_websocket.go`. NativeProcessManager must track last-set size instead of querying each time.
- [x] **pty_discovery.go GetTmuxSessionName?** Zero occurrences (research finding was inaccurate). No type-assert needed for pty_discovery.go.
- [x] **review_queue_poller.go callers?** Three callers (lines 406, 550, 576), not one. All call `inst.GetTmuxSessionName()` on `*Instance` struct — Instance method becomes a wrapper; no change to callers.
- [x] **OpenFeature reload?** Startup-only is fine — config hot-reload is not supported anywhere in the app.
- [x] **DoesSessionExist semantics?** Seven callers in instance.go. Most check "is the session alive/running" — same semantics as IsAlive(). Story 1.5 must explicitly verify line 580 (FromInstanceData recovery check) where DoesSessionExist() is called on a pre-start instance; semantics must be confirmed equivalent before substitution.
- [x] **HasSession() in ProcessManager?** Yes — 6 callers in instance.go require it. Added to ProcessManager interface in Story 1.1; NativeProcessManager implements as alias for IsAlive().
- [x] **GetPaneCurrentPath / GetCurrentWorkingDirectory?** instance.go:2459 calls Session().GetPaneCurrentPath() directly. Added `GetCurrentWorkingDirectory()` to ProcessManager. TmuxBackend implements via type-assert to Session().GetPaneCurrentPath(). Phase 3 defers to a proper CWD tracking approach.
- [x] **Session()/SetSession() removal scope?** Cannot simply remove from TmuxManager — 10 call sites in instance.go. Phase 1: remove from interface only; Phase 3: migrate initialization out of instance.go.
