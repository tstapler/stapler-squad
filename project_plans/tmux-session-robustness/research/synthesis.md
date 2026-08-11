# Research Synthesis: TMux Session Robustness & API Controllability

Status: Complete | Phase: 2 → 3 handoff
Date: 2026-04-16
Input files: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md, requirements.md

---

## Decision Required

How should stapler-squad detect session exit, prevent zombie state, and surface a clean lifecycle
API — and which architectural option achieves the requirements.md success criteria with the least
disruption to the existing codebase?

---

## Context

The system has four compounding gaps that together produce the observed production failure
(`staplersquad_stapler-squad-testing-and-refactoring` exits, review queue stays stuck):

**Gap 1 — Exit signal received but not propagated.**
`processControlModeLine()` in `session/tmux/control_mode.go` line 249 handles `%exit` with a
log statement and comment "let the caller handle cleanup." No caller does. The signal is dead on
arrival. This is the primary driver of zombie state.

**Gap 2 — PTY EOF does not update the state machine.**
`ResponseStream.streamLoop()` in `session/response_stream.go` correctly detects EOF (lines
149–154) and logs to `ForSession()`, but then calls `closeAllSubscribers()` and returns without
calling any transition on the `Instance`. `instance.started` remains `true`; `instance.Status`
remains `Running`. The UI shows the session alive; the review queue keeps polling it.

**Gap 3 — Health checker is the only zombie fallback, but it has double-start and false-positive
risks.**
`health.go:checkSingleSession()` calls `instance.TmuxAlive()` → `DoesSessionExist()` and, on
failure, immediately calls `instance.Start(false)` — with no debounce, no server-down guard,
and no `startMu` serialisation. Concurrent calls (health checker + restore path) produce the
"double PTY connection" log pattern documented in findings-pitfalls.md.

**Gap 4 — Server layer bypasses Instance API.**
`server/services/connectrpc_websocket.go:453` calls `instance.GetTmuxSession()` and then
`tmuxSession.StartControlMode()` directly. This re-exposes the concrete `TmuxSession` type that
`TmuxProcessManager` was introduced to abstract.

The additional context provided confirms a critical detail: `tmux -C attach-session` (single `-C`,
not `-CC`) sends `%exit` when the control mode process exits — which happens when the SESSION it
is attached to dies. This IS the per-session exit signal. On tmux 3.6a (installed here), the
process exits promptly when the session closes. `%pane-exited` and `%session-closed` are thus
bonus events; `%exit` + scanner EOF together provide a reliable signal on the installed version
without version-gating.

---

## Options Considered

### Option A: Targeted control mode propagation (recommended)

Wire `%exit` from `processControlModeLine` through a callback on `TmuxSession` up to `Instance`,
add an `onEOF` callback from `streamLoop`, add a per-instance `startMu`, fix the health checker
debounce, and expose a minimal `SessionStreamer` interface for the server layer. All changes stay
within `session/tmux/`, `session/`, and `server/services/`. No new dependencies.

### Option B: Direct tmux unix socket protocol

Implement the `imsg` binary protocol over `/tmp/tmux-<UID>/default` directly. Requires
`SCM_RIGHTS` file-descriptor passing, reverse-engineering the undocumented binary format, and
handling version divergence between tmux 2.x and 3.x. No Go library implements this. Confirmed
not viable by prior `terminal-jank/research/tmux-socket-protocol.md`.

### Option C: Replace tmux with custom process supervisor

Build a Go daemon per session (like `claude-mux` in `session/mux/`) that owns the PTY and exposes
a Unix socket. Provides excellent exit notification (daemon owns the process), but loses
`capture-pane`, `send-keys`, the keepalive architecture, and all tmux introspection. Migration
cost is 3–6 months. The `claude-mux` pattern already exists for external sessions; extending it
to all sessions is conceptually possible but architecturally disruptive.

### Option D: Full FSM with transition hooks on Instance

Model all state as an explicit FSM where every `i.Status = X` assignment routes through
`transitionTo()` and fires registered `TransitionHook` callbacks. `instance.go` is ~2,400 lines
with status writes scattered in many places; a mechanical audit is high-risk. Deferred to a
follow-on refactor session.

---

## Dominant Trade-off

**Event-driven exit detection (low latency, risk of double-fire) vs. poll-based reconciliation
(safe and self-healing, but adds detection latency).**

Neither alone is sufficient:
- Pure event-driven (`%exit` wiring) fails if the control mode process is not running when the
  session dies (e.g., control mode failed to start, session died between `StopControlMode` and
  `StartControlMode`).
- Pure polling fails the ≤10s requirement only if the interval is tuned tightly, and has
  false-positive risk during server restarts.

The resolution is a three-layer model: reactive event (primary path, ~ms latency), scanner EOF
fallback (secondary, same goroutine), and periodic reconciliation (tertiary, catches anything
missed). The three layers converge on the same `onExit` callback path, protected by `sync.Once`
to prevent double-transition.

---

## Recommendation

**Choose: Option A — targeted propagation of existing signals, plus minimal structural guards.**

Because: the exit detection infrastructure (control mode process, `%exit` parsing, scanner loop,
PTY EOF detection, `DoesSessionExist`, health checker) is already present and partially working.
The problem is propagation gaps and missing guards, not missing capabilities. Every required signal
already reaches the right goroutine; no signal reaches the state machine.

Accept these costs:
- `%pane-exited` / `%session-closed` are not handled initially (we rely on `%exit` + EOF, which
  is sufficient on tmux 3.6a). Add `%session-closed` as a bonus in the same pass since it is a
  single additional `case` in the switch statement.
- Exit codes are not captured in this iteration (requires tmux ≥ 3.3 `%pane-exited` or
  `remain-on-exit` polling). Tracked as a follow-on.
- The FSM refactor (full `transitionTo` audit) is deferred; the hooks described below wire into
  the existing `transitionTo` call sites.

Reject these alternatives:
- Option B: rejected because the binary socket protocol is undocumented, version-sensitive, and
  has no Go library support. The control mode text protocol is the documented API for this use
  case.
- Option C: rejected because it loses `capture-pane`, `send-keys`, and the existing tmux
  persistence architecture. The exit notification problem is a wiring gap, not a transport gap.
- Option D (full FSM now): rejected because the mechanical audit of `instance.go`'s ~2,400 lines
  of status writes is high-risk and out of scope. The targeted callback approach achieves the
  requirements.md success criteria without it.

---

## Implementation Plan (Ordered by Impact/Effort)

### Phase 1: Exit Detection — High impact, Low effort

**1a. Add `onExit` callback to `TmuxSession` and wire `%exit`**

File: `session/tmux/control_mode.go`

Add a field to `TmuxSession`:
```go
onExit     func(reason string)
onExitOnce sync.Once
```

In `processControlModeLine`, replace the existing `case "%exit":` log-only handler:
```go
case "%exit":
    log.InfoLog.Printf("Control mode received %%exit for session '%s': %s", t.sanitizedName, reason)
    t.onExitOnce.Do(func() {
        if t.onExit != nil {
            t.onExit("control-mode-%exit")
        }
    })
```

Also handle `%session-closed` in the same switch (additive case, same cost):
```go
case "%session-closed":
    // %session-closed $SESSION_ID
    log.InfoLog.Printf("Control mode session-closed for '%s'", t.sanitizedName)
    t.onExitOnce.Do(func() {
        if t.onExit != nil {
            t.onExit("session-closed")
        }
    })
```

In `readControlModeOutput()` (same file, ~line 172), after `t.controlModeExited = true`, fire the
same callback via `onExitOnce` as a scanner-EOF fallback (covers cases where `%exit` was not
received):
```go
t.onExitOnce.Do(func() {
    if t.onExit != nil {
        t.onExit("control-mode-pipe-closed")
    }
})
```

`StopControlMode()` must NOT fire `onExit` — this is an operator-initiated stop, not a session
crash. Set a flag before calling `StopControlMode`:
```go
// Add to TmuxSession:
intentionalStop bool
```
Gate `onExitOnce.Do` behind `!t.intentionalStop`. Set `t.intentionalStop = true` at the top of
`StopControlMode()`.

**1b. Wire `onExit` from Instance into TmuxSession**

File: `session/instance.go`

In the session start path (around line 855 where `transitionTo(Running)` is called), set:
```go
i.tmuxManager.GetTmuxSession().onExit = func(reason string) {
    log.ForSession(i.Title).Warning("Session exited via control mode: %s", reason)
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    _ = i.transitionTo(Stopped)
    i.started = false
}
```

Note: `GetTmuxSession()` access here is inside the session package itself — acceptable. The server
layer bypass (Phase 3) is a separate concern.

**1c. Wire `onEOF` from `streamLoop` to Instance**

File: `session/response_stream.go`

Add `OnEOF func()` field to `ResponseStream`. In `streamLoop`, at the three exit-via-EOF paths
(lines 153, 169), call `rs.OnEOF()` before returning (nil-guarded).

File: `session/instance.go`

In the controller start path, wire:
```go
responseStream.OnEOF = func() {
    log.ForSession(i.Title).Warning("Session exited (PTY EOF)")
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    _ = i.transitionTo(Stopped)
    i.started = false
}
```

Also fix `streamLoop` to reset `rs.started = false` (under `rs.mu`) before returning on any exit
path — see pitfall 6b. This unblocks subsequent `Start()` calls after an EOF-driven exit.

---

### Phase 2: Zombie Reconciliation — High impact, Medium effort

**2a. Add per-instance `startMu` to serialise `Start()`**

File: `session/instance.go`

Add `startMu sync.Mutex` to the `Instance` struct. At the top of `start()`, call
`i.startMu.Lock()` and defer `i.startMu.Unlock()`. Re-check `i.started` inside the lock
(double-checked locking). This eliminates the double-start race in `health.go:85` where
`instance.Start(false)` is called without serialisation.

**2b. Add server-down guard and debounce to health checker**

File: `session/health.go`

Before calling `instance.Start(false)` on a zombie-detected session, check whether the tmux
server itself is down (use the existing `checkServerNotRunning()` path or a call to
`tmux.IsServerRunning()`). If the server is down, skip individual session recovery — the
server-level `recoverFromServerFailure()` handles bulk recovery.

Add a per-instance consecutive-failure counter (in-memory, on `Instance` or `SessionHealthChecker`).
Require 2 consecutive `TmuxAlive() == false` results before triggering recovery. This absorbs
the 50–200 ms server-restart window documented in findings-pitfalls.md pitfall 3.

The `ReviewQueuePoller` has a 2-second `PollInterval` (confirmed in `review_queue_poller.go:25`).
The `ScheduledHealthCheck` interval is configurable. The debounce target from requirements.md is
≤10s zombie detection. With a 2-cycle debounce at 2s poll interval, detection latency is ~4s —
well within the 10s budget.

**2c. Batch `list-sessions` for reconciliation efficiency**

File: `session/tmux/tmux.go` (new function) or `session/tmux_process_manager.go`

Add `ListAllSessions(serverSocket string) (map[string]bool, error)` that runs a single
`tmux list-sessions -F #{session_name}` and returns a set. The health checker can call this once
per cycle and check all instances against the result, replacing N per-session `DoesSessionExist()`
subprocess calls with one. The existing `DoesSessionExist()` 500 ms TTL cache means the batch
call must bypass or refresh the cache — pass a `forceRefresh bool` or add a separate
`ListAllSessions` that does not go through the cache.

---

### Phase 3: Clean API Surface — Medium impact, Medium effort

**3a. Consumer-owned `SessionStreamer` interface in the server layer**

File: `server/services/session_streamer.go` (new file, minimal — ~30 lines)

```go
// SessionStreamer is the interface the WebSocket handler needs.
// Defined here (consumer-owned) to prevent import cycles.
type SessionStreamer interface {
    StartControlMode() error
    StopControlMode() error
    SubscribeControlMode(id string) (<-chan []byte, error)
    UnsubscribeControlMode(id string)
}
```

File: `session/instance.go`

Add `StartControlModeStream() error`, `StopControlModeStream() error`, etc. as methods on
`*Instance` that delegate to `i.tmuxManager.GetTmuxSession()`. These implement `SessionStreamer`.

File: `server/services/connectrpc_websocket.go`

Replace the direct `GetTmuxSession()` access at line 453 with acceptance of a `SessionStreamer`.
The handler already receives `*session.Instance`; the implicit interface satisfaction means no
call site changes are required — just stop calling `GetTmuxSession()` from this file.

**3b. Lifecycle hooks for review queue and server layer**

File: `session/instance.go`

Add `LifecycleListener` interface and `RegisterLifecycleListener()` / `UnregisterLifecycleListener()`
to `Instance`. Events: `EventStarted`, `EventExited`, `EventRestarted`. Dispatch asynchronously
(fire-and-forget goroutine per listener) so `Instance.start()` is not blocked.

The `onExit` callbacks from Phase 1 become the authoritative `EventExited` firing points. The
`ReviewQueuePoller` and any future server-side component registers as a listener at startup.

Do NOT use a channel-based event bus — it is over-engineered for two consumers and adds shutdown
ordering complexity. The `onServerRecovered` callback pattern already in `tmux.go` is the right
precedent: a simple function field, set by the layer that needs the event.

---

## Open Questions Before Committing

- [ ] **Does `tmux -C attach-session -t <name>` exit immediately when the named session is
  destroyed on tmux 3.6a?** The additional context says yes (`%exit` fires when the session it
  is attached to dies). Verify empirically once before implementing:
  `tmux new-session -d -s test && tmux -C attach-session -t test`
  then in another pane: `tmux kill-session -t test` — observe whether the attach process exits.

- [ ] **Is `intentionalStop` flag thread-safe?** `StopControlMode()` writes it; `readControlModeOutput()`
  reads it in the `onExitOnce.Do` gate. These run in different goroutines. Protect with
  `controlModeSubMu` or make it `atomic.Bool` (Go 1.19+, already used in this codebase?).

- [ ] **Does `transitionTo(Stopped)` from `onExit` work without a `Started()` pre-check?**
  Confirm the FSM transition table in `instance.go:733` allows `Running → Stopped` and
  `Ready → Stopped`. If not, the callback needs to either bypass `transitionTo` or set
  `i.Status = Stopped` directly (acceptable in Phase 1 before the full FSM refactor).

- [ ] **Should `i.started = false` be set before or after `transitionTo(Stopped)`?**
  Setting it before means any concurrent `Started()` check returns false while the transition
  is in progress. Setting it after means the window between PTY EOF and state update is minimised.
  Recommendation: set under `stateMutex` in the same lock acquisition as `transitionTo`.

- [ ] **`STAPLER_SQUAD_USE_CONTROL_MODE=false` — does control mode start at all?**
  If control mode can be disabled, Phase 1a (`%exit` wiring) provides no coverage in that mode.
  Phase 1c (PTY `onEOF` callback) remains the only exit signal. Confirm whether
  `STAPLER_SQUAD_USE_CONTROL_MODE` is a deployment concern or a test concern.

- [ ] **Consecutive-failure counter for zombie debounce — persisted or in-memory?**
  In-memory (on `SessionHealthChecker`) is simpler and sufficient. On restart, the counter
  resets, which means the next health check cycle re-evaluates from zero — acceptable, since
  Phase 1 (event-driven exit) handles the common restart case before the health checker fires.

---

## Risk Register

| Risk | Severity | Mitigation |
|------|----------|------------|
| Double-fire of `onExit` (control mode `%exit` + scanner EOF arrive within same ms) | Medium | `sync.Once` wrapper on all `onExit` call sites |
| `onExit` fires for intentional `StopControlMode` (operator-initiated stop) | High | `intentionalStop` flag set before `StopControlMode`; gate `onExitOnce.Do` |
| Double-start race from concurrent health checker + restore path | High | `startMu sync.Mutex` per instance (Phase 2a) |
| Zombie false positive during tmux server restart | Medium | 2-cycle debounce + server-down guard (Phase 2b) |
| `transitionTo(Stopped)` called while `stateMutex` not held | Medium | Acquire `stateMutex` inside `onExit` callback before calling `transitionTo` |
| `streamLoop` goroutine leak if `attachCmd` not killed before `ptmx.Close()` | High | Audit `TmuxSession.Close()` teardown order (pitfall 2, noted as CRITICAL in tmux.go:54) |
| PTY stale-pointer race in `streamLoop` (`pty` snapshot escapes RLock before `SetReadDeadline`) | Medium | Deferred to Phase 2 or follow-on; the string-match error handler already absorbs the symptom |

---

## What is NOT in Scope (Confirmed by Research)

- **Exit code capture.** `%pane-exited` (tmux ≥ 3.3) or `remain-on-exit` polling. Sufficient for
  future work; requirements.md does not require it.
- **Full FSM audit.** The `transitionTo` call sites across all of `instance.go`. Follow-on
  refactor session.
- **`%pane-exited` parsing.** Additive and low-risk, but the `%exit` + EOF path is sufficient
  for the requirements on tmux 3.6a. Add opportunistically in the same PR if it is a single case.
- **Protobuf / web UI changes.** No new session status visible to the frontend. `Stopped` is
  already a legal serialised status.
- **Batch `list-sessions` optimisation.** Correct to build (Phase 2c) but can ship after Phase 1
  and 2a/2b are stable.

---

## Sources

**Codebase files verified:**
- `session/tmux/control_mode.go` — `processControlModeLine` switch, line 249 (`%exit` dead handler)
- `session/response_stream.go` — `streamLoop` lines 149–170 (EOF detection, no state machine callback)
- `session/health.go` — `checkSingleSession` lines 78–99 (single-shot Start without debounce or server guard)
- `session/instance.go` — `transitionTo` line 733, `startMu` absent, `started` set at line 862
- `server/services/connectrpc_websocket.go` — `GetTmuxSession()` direct access at line 453
- `session/review_queue_poller.go` — `PollInterval: 2 * time.Second` (line 25)

**Research findings files:**
- `findings-stack.md` — Option A/B/C comparison, tmux control mode protocol details, prior FIFO history
- `findings-features.md` — Three-layer detection model (reactive + stream-end + defensive), supervisord/s6/Zellij/Overmind survey
- `findings-architecture.md` — Observer vs. EventBus vs. Poll Reconciler vs. PTY EOF vs. FSM trade-offs
- `findings-pitfalls.md` — PTY close race, goroutine leaks, zombie false positives, double-start race, `streamLoop` state machine disconnect

**Additional context provided:**
- tmux version on target machine: 3.6a (confirms `%exit` fires on session death, not just server shutdown)
- `StartControlMode()` uses `tmux -C attach-session -t <session>` (single `-C`)
- `processControlModeLine` handles `%output`, `%exit`, `%session-changed` but not `%pane-exited` or `%session-closed`
- `%exit` fires when the control mode process exits — which IS when the attached session dies on this version
