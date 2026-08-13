# ADR-002: Zombie Reconciliation — Per-Instance Mutex + Debounced Health Checker

Status: Accepted
Date: 2026-04-16
Deciders: Tyler Stapler

---

## Context

After Phase 1 (ADR-001) ships, the common exit case is handled event-driven. However, two
structural gaps remain in the health checker fallback path:

**Gap 1 — Double-start race condition (findings-pitfalls.md, pitfall 5)**
`Instance.start()` does not hold a mutex for the entire start sequence. If the health checker
and the server restore path both call `Start(false)` on the same instance simultaneously (a
real scenario — both run on independent goroutines), both can pass the `i.started` guard before
either sets it to `true`. Both then call `RestoreWithWorkDir()`, attaching two `ptmx` file
descriptors to the same tmux session. Two `streamLoop` goroutines race to broadcast the same
bytes to subscribers, and one calls `closeAllSubscribers()` on EOF, leaving the other goroutine
with no subscribers and no way to clean up.

**Gap 2 — False zombie detection during tmux server restart (findings-pitfalls.md, pitfall 3)**
`checkSingleSession` calls `TmuxAlive()` → `DoesSessionExist()` → `tmux has-session`. During a
tmux server restart, all `has-session` calls return "no server running" for 50–200 ms. The
health checker sees every session as dead and calls `instance.Start(false)` on each one. This
races with the server-level `recoverFromServerFailure()` path and can produce cascading
restart failures.

**Gap 3 — N subprocess calls per health check cycle**
With N active sessions, `CheckAllSessions` issues N `tmux has-session` subprocesses per cycle.
A single `tmux list-sessions` call returns all names at once, reducing N subprocess calls to 1.

## Decision

### 2a: Per-instance `startMu sync.Mutex`

Add `startMu sync.Mutex` to `Instance`. Acquire it at the top of `start()` and defer its
release. Re-check `i.started` inside the lock (double-checked locking) to make concurrent
starts sequential no-ops after the first completes.

```
start() called concurrently by health checker + restore path
  → both call startMu.Lock()
  → one wins, proceeds to create session
  → other waits, acquires lock after first completes
  → other re-checks i.started — now true — returns nil early
  → result: one tmux session, one streamLoop goroutine
```

The mutex ensures serialization. The double-check ensures the second caller bails out cleanly
once the first has already done the work.

### 2b: Server-down guard + 2-cycle debounce in health checker

Add a `failureCounts map[string]int` to `SessionHealthChecker` (in-memory, reset on restart).

Before triggering recovery for any instance:
1. Check if the tmux server itself is down (reuse existing server-down detection pattern from
   `tmux.go`). If down, skip all individual recovery — `recoverFromServerFailure()` handles bulk.
2. Require the instance to fail `TmuxAlive()` for 2 consecutive health check cycles before
   starting recovery. This absorbs the 50–200 ms server restart window.

With a 2s health check interval, the worst-case zombie detection latency is 4s — well within
the ≤10s requirement from `requirements.md`. With the server-down guard, false positives during
server restarts are eliminated.

### 2c: `ListAllSessions` batch function

Add `func ListAllSessions(serverSocket string) (map[string]bool, error)` to the `session/tmux`
package. `CheckAllSessions` calls it once per cycle and passes the result set to each
`checkSingleSession` call, replacing per-session `DoesSessionExist()` subprocess calls.

This is a performance optimisation, not a correctness fix. It can ship independently after
2a/2b are stable.

## Alternatives Considered

**Alternative for double-start: atomic CAS flag**
Use `sync/atomic.CompareAndSwap` on a `uint32` "startInProgress" field. The second goroutine
that loses the CAS returns immediately without waiting. Lighter than a mutex if high-frequency
contention is not expected.
Not chosen: `sync.Mutex` is clearer and its semantics (serialization + double-check) directly
match the intent. The contention window is long-lived (tmux start + PTY attach takes 100-500ms),
not a tight spin loop. Mutex is the right primitive.

**Alternative for debounce: separate ZombieReconciler goroutine**
A dedicated goroutine on a 15s ticker that calls `ListAllSessions` once and checks all instances.
More explicit separation of concerns than adding debounce logic to the existing health checker.
Not chosen for this iteration: the existing `SessionHealthChecker` infrastructure is already
wired at startup. Adding debounce to it is less invasive than introducing a new goroutine with
independent lifecycle management. The dedicated reconciler is the right long-term shape; deferred.

**Alternative for server-down detection: subscribe to server recovery event**
Register a callback on `recoverFromServerFailure()` to set a "server recovering" flag that the
health checker checks before running.
Not chosen: the existing `recoveryInFlight` mutex in `tmux.go` serves this role. Checking
`IsServerRunning()` (or equivalent) before each individual recovery attempt is simpler and does
not require coordination between the server recovery and health checker goroutines.

## Consequences

Positive:
- Double-start race eliminated for all concurrent callers (health checker, restore, manual Start)
- Zero false zombie recovery attempts during tmux server restart
- Zombie detection latency guaranteed ≤ 2 × health check interval (≤4s at default 2s interval)
- N subprocess calls per health check cycle reduced to 1 (2c)
- In-memory failure counter resets on process restart — no persistence complexity

Negative / Accepted risks:
- `startMu` serialises all `Start()` calls. If `start()` is called many times in quick succession
  (e.g., during bulk restore), they queue behind the mutex. This is the desired behaviour; the
  queue ensures no double-start, not a performance concern.
- Failure counter is in-memory. A process restart resets it. On restart, Phase 1 (event-driven
  exit) handles the common case before the health checker fires; the counter reset is acceptable.
- `isServerDown()` helper must be correctly implemented to avoid suppressing legitimate zombie
  recovery when the server is actually running. Use the existing error-string check from `tmux.go`
  rather than reimplementing.
