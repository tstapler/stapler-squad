# Findings: Architecture - Session Lifecycle Design Patterns

## Summary

The session package already has meaningful layering: `TmuxProcessManager` wraps `TmuxSession` (concrete), `Instance` delegates through `TmuxProcessManager`, and `ClaudeController` reads the PTY via the `InstanceContext` interface. However, three structural gaps remain:

1. **No lifecycle event surface.** Nothing fires when a session starts, exits, or is restarted. Consumers poll or read state flags at call time.
2. **No zombie detection.** `Instance.started` is set to `true` and never cleared when the underlying tmux session dies. `IsControllerActive` in `InstanceStatusInfo` relies on `ClaudeController.IsStarted()`, which tests whether a context is non-nil — not whether tmux is alive.
3. **Server layer bypasses the Instance API.** `connectrpc_websocket.go` calls `instance.GetTmuxSession()` to reach `TmuxSession` directly, re-exposing the concrete type that `TmuxProcessManager` was introduced to hide.

The codebase already contains `session/api_design_proposal.md` recognising the coupling problem; it proposes `Detach/Reattach/Destroy` but does not address events or zombie detection.

---

## Options Surveyed

### Option A: Observer / Listener pattern

Attach a `LifecycleListener` interface to `Instance`. Callers register at construction time; `Instance.start()`, `Resume()`, and an exit path call each listener.

```go
type LifecycleEvent int
const (
    EventStarted LifecycleEvent = iota
    EventExited
    EventZombieDetected
    EventRestarted
)

type LifecycleListener interface {
    OnLifecycleEvent(inst *Instance, event LifecycleEvent)
}
```

Listeners are stored as a slice on `Instance` and called synchronously (or in a goroutine, caller's choice). The `ReviewQueuePoller` and any future server-side logic register themselves at startup.

**Pros:** Explicit, strongly typed, zero external dependencies, easy to test by injecting a recording listener.
**Cons:** Synchronous dispatch blocks `Instance.start()` if a listener is slow; need a `sync.RWMutex` to allow concurrent `Register`/`Unregister`; no replay for late registrants (a listener added after `EventStarted` misses it).

---

### Option B: Channel-based event bus (Go-idiomatic)

A package-level or injected `chan LifecycleEvent` is shared across subsystems. Senders are non-blocking (buffered channel, drop on full). Consumers run their own goroutines.

```go
type LifecycleEvent struct {
    InstanceTitle string
    Kind          EventKind
    Timestamp     time.Time
}

type EventBus struct {
    subs []chan LifecycleEvent
    mu   sync.Mutex
}

func (b *EventBus) Publish(ev LifecycleEvent) { ... } // fan-out, non-blocking
func (b *EventBus) Subscribe() <-chan LifecycleEvent { ... }
```

**Pros:** Publishers and subscribers are fully decoupled; multiple consumers with independent lag; fits Go's concurrency model well.
**Cons:** Channels require explicit lifecycle management (close on shutdown, drain or drop on slow consumer). Backpressure decisions are non-trivial. Debugging event loss is harder than debugging a missed function call. The existing codebase has no event bus — adding one is a new primitive.

---

### Option C: Polling reconciliation loop (Kubernetes-style)

A background goroutine — a `LifecycleReconciler` — runs on a ticker. It queries every `Instance.started == true` and calls `TmuxProcessManager.IsAlive()`. If alive==false it transitions the instance to a new `Zombie` (or `Crashed`) status and triggers corrective action (restart, cleanup, notification). This is the current pattern used by `ReviewQueuePoller.pollLoop()`.

```go
type LifecycleReconciler struct {
    instances InstanceSource
    interval  time.Duration
    onZombie  func(*Instance)
    onRestart func(*Instance)
}
```

**Pros:** No changes to `Instance` internals; tolerates missed transitions (reconciler will catch them on the next tick); well understood by Go/k8s practitioners; already fits the codebase pattern.
**Cons:** Detection latency proportional to poll interval (typically 5–30 s); tmux `list-sessions` is a subprocess call (~5 ms each); calling it for every running instance on every tick is non-trivial cost; no instant notification to subscribers.

---

### Option D: Reactive / channel-based with PTY EOF detection

`ResponseStream.streamLoop()` already reads the PTY in a goroutine. When the tmux session dies, the PTY's `Read()` returns `io.EOF`. This is an existing reactive signal that is currently discarded. Wire `io.EOF` in `streamLoop` to an `onSessionExit` callback on the `ClaudeController`, which propagates it to the `Instance` (via `InstanceContext` extension) or to a lifecycle bus.

```go
// In ResponseStream.streamLoop(), on EOF:
if rs.onEOF != nil {
    rs.onEOF()
}
```

**Pros:** Zero polling cost; latency is milliseconds (PTY EOF fires immediately when tmux session dies); reuses infrastructure already running.
**Cons:** Only fires for controller-managed sessions (external/non-PTY sessions are not covered); PTY EOF is also fired on a normal `Detach()` call — distinguishing intentional detach from crash requires additional state; wiring the callback back from `ResponseStream` through `ClaudeController` to `Instance` without creating circular imports requires careful interface design.

---

### Option E: Finite State Machine (FSM) with transition callbacks

Model session state as an explicit FSM: `Creating → Running → Paused → Restarting → Zombie → Stopped`. Each `transitionTo()` call (already exists in `instance.go`) fires registered callbacks. Illegal transitions return errors.

```go
type StateTransition struct {
    From, To Status
    At       time.Time
}

type TransitionHook func(inst *Instance, t StateTransition)
```

**Pros:** Makes the lifecycle explicit and auditable; catches bugs where code sets the wrong status; the `transitionTo()` skeleton already exists in `instance.go`.
**Cons:** Requires defining `Zombie`/`Crashed` as a new legal status (currently omitted); existing code transitions status directly by writing `i.Status = X` in several places outside `transitionTo` — all those sites must be audited and refactored before hooks fire reliably.

---

## Trade-off Matrix

| Criterion                        | A: Observer | B: EventBus | C: Poll Reconciler | D: PTY EOF | E: FSM+Hooks |
|----------------------------------|:-----------:|:-----------:|:-----------------:|:----------:|:------------:|
| Detection latency                | instant     | instant     | poll interval     | ~ms        | instant      |
| Cost per idle session            | zero        | zero        | O(sessions) subprocs | zero  | zero         |
| Works for non-PTY sessions       | yes         | yes         | yes               | no         | yes          |
| Migration cost to existing code  | low         | medium      | low               | medium     | high         |
| Debuggability                    | high        | medium      | high              | medium     | high         |
| Handles intentional detach vs crash | manual  | manual      | yes (checks tmux) | needs flag | yes (state)  |
| Testability                      | high        | medium      | high              | high       | high         |
| New Go primitives required       | no          | chan+goroutine | goroutine only | callback  | no           |
| Replay for late registrants      | no          | no          | yes (query state) | no         | yes (query)  |

---

## Risk and Failure Modes

**Observer (A)**
- A panicking listener kills `Instance.start()` unless each call is wrapped in a `recover`. Design must decide: synchronous with panic guard, or always async in a goroutine.
- Listener leak: if code forgets to `Unregister`, listeners accumulate over restarts.

**EventBus (B)**
- Channel full: if a slow consumer's channel fills, `Publish` must either block (deadlock risk), drop (silent data loss), or force-close (crashes consumer goroutine). None is obviously correct.
- Shutdown ordering: `EventBus.Close()` must drain before subsystems that depend on it shut down.

**Poll Reconciler (C)**
- False zombie detection: if `tmux list-sessions` times out (common under load, already documented in `DoesSessionExist` timeouts), the reconciler sees `IsAlive=false` and may incorrectly kill or restart a live session. Requires at least two consecutive failures before acting.
- Cascading recovery: if reconciler triggers a restart while `ReviewQueuePoller` is mid-check, two paths write to `Instance.Status` concurrently. Must hold `stateMutex`.

**PTY EOF (D)**
- False EOF on detach: `Detach()` calls `closePTYAndAttachCmd()` which closes `ptmx`; `Read()` in `streamLoop` returns EOF. This is indistinguishable from session death unless `Instance.detaching` (which exists on `TmuxSession`) is checked at the time of EOF. The flag lives on the wrong struct.
- Only covers controller-managed sessions: external mux sessions, paused sessions, and sessions where controller startup failed (PTY attach failed after retries, logged in `instance.go:847`) will not fire EOF events.

**FSM (E)**
- `Instance.go` is already ~2400 lines; the file touches `Status` directly in many places. A mechanical audit to route all writes through `transitionTo` is high-risk for a file with no section tests covering every transition.
- `Zombie` is not a current legal status. Introducing it requires updating serialisation (JSON tags in `InstanceData`), the web API protobuf, the TUI renderer, and the frontend React components.

---

## Migration and Adoption Cost

`instance.go` is approximately 2,300 lines (confirmed by reading to offset 2,300+ without hitting EOF). It contains `start()`, `Resume()`, `Pause()`, `Kill()`, status transitions, git manager wiring, controller wiring, checkpoint logic, and GitHub PR polling. It is the highest-coupling file in the session package.

**Adding lifecycle hooks (Observer, Option A):**
- Add a `[]LifecycleListener` slice and `RegisterLifecycleListener()` to `Instance`. ~30 lines.
- Add `fireEvent(ev)` calls at the end of `start()`, `Resume()`, and a new `onExit()` path. ~5 call sites.
- No changes to `TmuxProcessManager`, `TmuxSession`, or the server layer.
- Estimated: 1–2 days including tests.

**Zombie detection (Poll Reconciler, Option C + FSM status):**
- Add a `Zombie` status to the `Status` enum and serialisation. Update status string, protobuf, and web-app label.
- Write `ZombieReconciler` struct (~100 lines) that runs a goroutine on a 15 s ticker, calls `TmuxProcessManager.IsAlive()`, and fires the `onZombie` lifecycle callback.
- Wire it at server startup alongside `ReviewQueuePoller`.
- The `IsAlive()` method already exists on `TmuxProcessManager`.
- Estimated: 2–3 days including serialisation and UI changes.

**SessionController interface (replacing server-layer direct access):**
- `GetTmuxSession()` is called in `connectrpc_websocket.go:453` to start control mode streaming. Wrap this behind a `StartControlModeStream(sessionName string)` method on `Instance` or an interface.
- Audit all `server/` files for direct `TmuxSession` access; only the one occurrence found above.
- Estimated: 0.5–1 day.

**Full FSM (Option E):** The audit of all `i.Status = X` assignments scattered in `instance.go` alone is risky and time-consuming. Recommend deferring until after a dedicated refactor session.

---

## Operational Concerns

**Subprocess budget.** `TmuxProcessManager.IsAlive()` calls `DoesSessionExist()`, which runs `tmux list-sessions` (a subprocess). With 20 sessions and a 15 s reconciler tick, this is 20 subprocesses per 15 s — roughly 1.3 subprocs/s. This is acceptable. With 100 sessions it becomes non-trivial; consider batching: one `tmux list-sessions` call returns all names, compare against all known sessions at once.

**Cache invalidation.** `DoesSessionExist()` has a 500 ms TTL cache. A zombie reconciler running at 15 s is well outside this window and always gets fresh data.

**Restart thundering herd.** If the tmux server dies (handled by `recoverFromServerFailure`), all sessions simultaneously appear zombie. The reconciler must check `serverNotRunning` before triggering individual session restarts — otherwise it fires `onZombie` for every session, spawning N concurrent restart attempts. The existing server recovery code uses a `recoveryInFlight` mutex for exactly this reason; the reconciler should check `checkServerNotRunning` before acting.

**Review queue staleness after zombie recovery.** After a session is detected zombie and restarted, the review queue may hold a stale item for the old session state. `ReviewQueuePoller.RemoveInstance()` clears the content cache; calling it as part of the restart lifecycle hook clears the stale entry cleanly.

**Controller frozen but tmux alive.** This is a distinct failure mode from zombie: the `ClaudeController.responseStream` is stuck (blocked Read, goroutine leak) but `DoesSessionExist()` returns true. Detecting this requires a `heartbeat` check on the `ResponseStream` (e.g., last successful Read time). Out of scope for zombie detection; tracked separately.

---

## Prior Art and Lessons Learned

**Kubernetes controller-manager reconciliation loop:** The k8s pattern is: `desired state minus observed state equals corrective action`. The reconciler does not trust in-memory flags; it always re-queries the external system (etcd / API server). Applied here: the zombie reconciler should call `tmux list-sessions` rather than trusting `instance.started`. This matches how `DoesSessionExist()` is already implemented.

**Docker daemon session tracking:** Docker handles container exits through event streams from the OCI runtime. The Go equivalent is the PTY EOF approach (Option D). Docker specifically guards against false exits during intentional shutdown by checking an "intentional stop" flag before firing `container.exit` events. The equivalent guard in this codebase would be checking `TmuxSession.detaching` before treating PTY EOF as a crash.

**Supervisord / systemd service watches:** Process supervisors handle the zombie problem by keeping the parent PID and waiting on it. In this codebase, the `tmux attach-session` process (`attachCmd *exec.Cmd`) could serve this role: when `attachCmd.Wait()` returns, the tmux session is gone. However, `closePTYAndAttachCmd()` kills `attachCmd` deliberately during normal operations, so this signal is not reliable without an intentional-shutdown flag.

**Go's `context.Context` cancellation:** The existing `ClaudeController` wires its internal components via a shared context. When the controller's `cancel()` is called, all goroutines exit cleanly. This is the right pattern for the controller's own cleanup. It does not address the tmux-layer zombie problem because the context is internal to the controller — tmux dying does not cancel the context; it causes an EOF on the PTY read, which is a different signal.

**The `onServerRecovered` callback in `tmux.go`:** This is the only existing lifecycle hook in the codebase. It demonstrates the pattern is acceptable: a package-level `var onServerRecovered func()` is set by the server layer via `SetServerRecoveryCallback`. This pattern works but does not scale to per-session events (a package-level var per session is not feasible). Per-instance listeners (Option A) are the right generalisation.

---

## Open Questions

1. **Should `Zombie` be a serialised status or an in-memory-only transition?** If the process crashes and restarts with a zombie session persisted as `Running`, the reconciler still catches it on startup. Serialising `Zombie` adds schema complexity with limited benefit; recommend treating it as transient and serialising only `Running`/`Crashed` (a new terminal status meaning "tmux died and auto-restart is not configured").

2. **Should the reconciler auto-restart or just notify?** Auto-restart risks infinite restart loops if the session program crashes immediately. Recommendation: notify via lifecycle hook and let the server layer decide. A simple restart backoff (exponential, capped at 5 attempts) can be implemented in the hook handler, not the reconciler itself.

3. **What is the right batch strategy for `list-sessions` in the reconciler?** The current `DoesSessionExist()` is per-session. A batch version — one `tmux list-sessions` call, parsed into a set, checked against all known sessions — is more efficient. This requires a package-level or manager-level function rather than a method on `TmuxSession`. Recommend adding `tmux.ListAllSessions(serverSocket string) (map[string]bool, error)`.

4. **How should the `SessionController` interface be scoped?** The existing `InstanceContext` interface (used by `ClaudeController`) covers PTY access. A separate `SessionController` interface for the server layer should cover only what the server layer legitimately needs: `StartControlModeStream`, `SendInput`, `GetPTY`, `GetWindowSize`. Putting this interface in the `server/services` package (as a consumer-owned interface) avoids import cycles.

5. **When the controller fails to start (PTY attach failed after retries), the session runs without zombie detection.** Should the reconciler always run regardless of controller state, or should it be conditional? Recommendation: always run — it is the fallback health check.

---

## Recommendation

### Lifecycle hooks: Observer pattern (Option A) with synchronous-then-async dispatch

Add a `LifecycleListener` interface and `RegisterLifecycleListener()`/`UnregisterLifecycleListener()` to `Instance`. Fire events from `start()`, `Resume()`, and the zombie recovery path. Use synchronous dispatch with a `go` prefix for each listener call (fire-and-forget goroutine per listener), so `Instance.start()` is not blocked. Listeners that need ordering guarantees manage their own internal queues.

This is the smallest change that gives `ReviewQueuePoller` and the server layer the signal they need, fits the existing `onServerRecovered` callback convention, and introduces no new Go primitives.

Do not use a channel-based event bus (Option B) — it is over-engineered for the number of consumers (two: review queue, server layer) and adds channel lifecycle complexity.

### Zombie detection: Poll reconciler (Option C) with batched `list-sessions`

Add a `ZombieReconciler` goroutine that:
- Runs on a 15 s ticker
- Calls a new `tmux.ListAllSessions(serverSocket)` returning `map[string]bool` (one subprocess)
- For each `Instance` with `started=true` and `Status != Paused` and `Status != Stopped`, checks if the session name is in the live set
- On two consecutive misses (to guard against transient timeouts), calls the registered lifecycle `onZombieDetected` hook
- The hook sets `instance.Status = Crashed` (or a new terminal status) and fires a UI refresh

The reconciler should skip the check entirely when `checkServerNotRunning` returns true (entire server is down — the recovery path in `recoverFromServerFailure` handles that case).

Do not rely solely on PTY EOF (Option D) for zombie detection — it misses sessions where the controller never started, and the false-positive-on-detach problem requires a `detaching` flag that currently lives in the wrong struct.

### SessionController interface: Consumer-owned interface in the server layer

Create a minimal `SessionStreamer` interface in `server/services`:

```go
// server/services/session_streamer.go
type SessionStreamer interface {
    StartControlMode() error
    StopControlMode()
    SubscribeControlMode(id string) (<-chan []byte, error)
    UnsubscribeControlMode(id string)
}
```

Have `Instance` implement `SessionStreamer` by delegating to `TmuxProcessManager`/`TmuxSession`. The server layer accepts `SessionStreamer`, not `*session.Instance`, in the WebSocket handler that currently calls `GetTmuxSession()`. This severs the server layer's direct dependency on `TmuxSession` without changing any other code.

This is a Go-idiomatic "consumer-owned interface" — defined where it is used, not where it is implemented. It keeps the interface minimal and prevents the server layer from acquiring new TmuxSession methods without an explicit interface update.

---

## Pending Web Searches

The following searches were not executed (web search may not be available) but would increase confidence in the recommendations:

1. `golang observer pattern large struct benchmarks goroutine dispatch 2024` — to confirm per-listener goroutine dispatch is acceptable versus a worker pool.
2. `kubernetes reconciler batch external process check pattern` — to validate the two-consecutive-miss guard for transient timeouts.
3. `golang consumer owned interface anti-patterns` — to surface any known issues with the consumer-owned interface pattern in large Go codebases.
4. `tmux list-sessions parse output golang performance` — to verify there are no known parse edge cases with `list-sessions -F #{session_name}` that the existing code already handles.
5. `golang io.EOF PTY close race condition` — to understand whether there are documented race conditions in `os.File.Read()` returning EOF on `ptmx.Close()` across Go versions.
