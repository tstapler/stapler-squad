# Research: Pitfalls of Adding a Fast-Recheck Timer Alongside `reconnectLoop`

Scope: what commonly goes wrong when a second, independent, backoff-decoupled
goroutine/timer is added to `TmuxServerRegistry` (`session/tmux/server_registry.go`)
to call `syncSessions()` at a bounded, fast cadence (`fastRecheckAttempts ×
(fastRecheckSyncTimeout + fastRecheckInterval) = 700ms`) alongside the existing
`reconnectLoop` (lines 309-400). All line references are to
`session/tmux/server_registry.go` on the current branch tip unless noted.

## 1. Race conditions from concurrent `syncSessions()` calls

**Locking as it exists today** (`syncSessions`, lines 210-246):

```go
r.mu.Lock()
var disappeared []string
for name := range r.sessions {
    if !sessions[name] {
        disappeared = append(disappeared, name)
    }
}
r.sessions = sessions
r.mu.Unlock()

for _, name := range disappeared {
    r.firePaneExit(name)
}
```

`syncSessions()` itself is *crash-safe* to call from two goroutines
concurrently — the map swap under `r.mu.Lock()` (line 231-239) can't corrupt
`r.sessions`, and `firePaneExit` (lines 198-207) already copies subscribers
out under `subsMu` and closes outside the lock, combined with `paneExitSub`'s
`sync.Once`-guarded `close()` (line 37). **Double-firing pane-exit for the
same session is already structurally impossible**: the first caller to reach
`firePaneExit(name)` deletes the subscriber slice from the map (line 201)
under `subsMu`, so a second concurrent caller (whether that's the
fast-recheck goroutine racing `reconnectLoop`'s post-connect sync, or racing
a `%session-closed` control-mode event via `handleEvent`, line 437-447) finds
an empty/absent slice and is a no-op. This part of the design already
generalizes safely to a second caller.

**The real race is a lost-update / "resurrected session" bug**, not a
double-fire. Each `syncSessions()` call does its own untimed sequence:
(1) shell out to `list-sessions` (10s-timeout subprocess, line 211-218),
(2) build a fresh map, (3) lock, diff against *whatever `r.sessions`
currently is*, swap, unlock. Two concurrent callers each capture the real
tmux state at a different wall-clock instant, but there is nothing that
orders their *lock-acquisition-and-swap* by *which subprocess result is
newer*. Concretely:

- Fast-recheck call **F** starts `list-sessions` at T0 (before `kill-session`
  lands) and observes the session still present.
- `reconnectLoop`'s post-connect call **R** starts `list-sessions` at T1
  (after `kill-session` lands) and observes the session gone; **R** wins the
  lock race, sets `r.sessions` without the entry, and fires pane-exit —
  subscribers are correctly notified.
- **F** then wins the *next* lock acquisition (subprocess simply took longer,
  e.g. under CI fork/exec contention) and overwrites `r.sessions` with its
  stale map that still contains the session. `SessionExists(name)` now
  transiently reports `true` again for a session that has already exited and
  already had `firePaneExit` fired for it — a "ghost" resurrection. Any
  *new* subscriber that calls `SubscribePaneExit` for that name in this
  window will wait until the *next* sync corrects `r.sessions`, adding
  latency exactly opposite to this fix's goal, and any caller polling
  `SessionExists` gets a stale `true`.

This is a genuine ordering hazard introduced specifically by adding a second,
independent caller of `syncSessions()` — it does not exist today because only
one call path runs at a time relative to any given disconnect/reconnect
cycle.

**Recommendation:** serialize `syncSessions()` invocations — e.g. a
dedicated `syncMu sync.Mutex` held for the *entire* fetch-diff-swap sequence
(not just the map swap), or a "single-flight" guard so a fast-recheck attempt
that finds a sync already in progress waits for it / skips its own redundant
call rather than launching a second overlapping subprocess+diff+swap. This
also directly mitigates point 5's "overlapping fast-recheck instances" case
below.

## 2. Goroutine leak risk on `Stop()` / teardown mid-cycle

`syncSessions()`'s subprocess timeout is **not** derived from `r.ctx`:

```go
// line 211
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
```

This is rooted in `context.Background()`, completely disconnected from the
registry's own lifecycle context. This is a *pre-existing* gap that the new
fast-recheck path will inherit and amplify: calling `r.Stop()` (which only
does `r.cancel()`, line 102) cannot abort an in-flight `list-sessions`
subprocess — it runs to completion or its own 10s cap regardless. In normal
operation `list-sessions` returns in well under 100ms, so this is low-risk
today, but a fast-recheck loop that fires several of these calls back-to-back
within a 700ms bound, under CI resource contention (many tmux subprocess
spawns across parallel test packages — already called out in
`server_registry_integration_test.go`'s own comments on `newSessionWithRetry`
and `registryPollTimeout`), raises the odds of a straggling subprocess call
outliving `t.Cleanup(cancel)` in a test, which is exactly the shape of a
goroutine-leak / "test finished but background work is still writing to
`r.mu`" failure.

**What must hold for the new goroutine specifically:**
- Every iteration boundary of the fast-recheck loop must `select` on
  `r.ctx.Done()`, not just check it once at the top — a `time.Sleep()` or a
  ticker fire between attempts must be raced against `r.ctx.Done()` so
  `Stop()` mid-cycle causes prompt exit rather than waiting out the full
  remaining attempt budget.
- Ideally, each `syncSessions()`-equivalent call the fast-recheck loop makes
  should derive its subprocess timeout from `r.ctx` (e.g.
  `context.WithTimeout(r.ctx, fastRecheckSyncTimeout)`) rather than reusing
  `syncSessions()`'s existing `context.Background()`-rooted timeout verbatim
  — otherwise "bounded to 700ms" is only true when tmux behaves; under load
  it's actually unbounded up to `fastRecheckSyncTimeout`'s own ceiling in the
  worst case, and `Stop()` still can't cut it short.
- Do not spawn attempt N+1 while attempt N's `syncSessions()` call is still
  in flight — sequence attempts (loop that waits for the previous call to
  return before starting the next timer), not a free-running ticker that
  fires regardless of whether the prior sync completed. This also avoids
  stacking concurrent `syncSessions()` callers (point 1).

## 3. Could the fix itself make the new regression test flaky?

Yes, in two distinct ways:

1. **Under `-race` / CI contention, the nominal 700ms budget is optimistic.**
   `-race` instrumentation adds significant CPU/memory overhead, and AC1/AC3
   require `go test -race -tags integration ./session/tmux/...` to pass
   reliably. If `fastRecheckAttempts × (fastRecheckSyncTimeout +
   fastRecheckInterval)` is tuned tightly around 700ms assuming an
   uncontended `list-sessions` subprocess call, real CI load (many tmux
   subprocess spawns across the package, per the existing
   `registryPollTimeout` comment already documenting this exact class of
   slowdown) can push actual detection latency past that ceiling. The new
   test's assertion window should carry real headroom over 700ms (in the
   same spirit as the existing `PaneExitChannel` test using a 3s deadline
   instead of a number close to the nominal backoff math) — asserting at a
   tight bound just relocates the flake from "detection missed the 3s test
   deadline" to "detection missed the regression test's tighter deadline."
2. **The mechanism used to "artificially elevate backoff" in the new test is
   itself timing-sensitive.** `backoff` in `reconnectLoop` (line 314) is a
   function-local variable, not a struct field — the new test can't inject a
   value directly without either (a) a refactor to expose/parameterize it
   (in scope, since `server_registry.go` is one of the two files the fix may
   touch, but changing internal loop state shape is more invasive than the
   stated goal), or (b) driving the *real* elevated-backoff state by forcing
   several real connect failures (e.g., pointing `startControlMode` at an
   unreachable socket transiently) and waiting for wall-clock backoff to
   climb past 700ms before killing the session under test. Approach (b) is
   real-clock-dependent by construction — exactly the pattern that produced
   the original flake — so the regression test needs deterministic
   synchronization points (e.g., a test-visible signal for "backoff has
   reached at least N", not a fixed sleep) to avoid becoming a second
   flake of the same species it's meant to catch.

## 4. `time.Timer` / `time.Ticker` pitfalls

- **Forgotten `Stop()` on a `time.Timer`/`time.Ticker` leaks the underlying
  runtime timer** until it fires (Timer) or forever (Ticker, which never
  self-stops). For a small, *bounded* number of attempts
  (`fastRecheckAttempts`, implied to be small — e.g. 3-5 — by the 700ms
  total), the simplest and pitfall-free construction is a plain loop using a
  fresh `time.After(fastRecheckInterval)` per attempt inside a `select`
  against `r.ctx.Done()`, rather than a persistent `time.Ticker`. A `Ticker`
  implies *indefinite* periodicity, which is the wrong shape for "N bounded
  attempts then stop," and every exit path (ctx-cancelled early-return,
  attempt-exhaustion, any future error path) would need its own
  `ticker.Stop()` call — easy to miss the ctx.Done() early-exit path
  specifically, which is exactly the path Stop()/teardown exercises (point 2).
- **Reusing an already-fired `Timer` via `Reset()` without draining `.C`
  first** is the classic pitfall (`Reset` on an undrained, already-fired
  timer can deliver a stale tick). This doesn't apply if a fresh timer (or
  `time.After`) is created per attempt instead of being reused across
  iterations — which is also the simpler choice for a fixed small attempt
  count, and sidesteps the entire Timer-reuse pitfall class.
- Net recommendation: prefer `time.After` per-attempt inside a `select` with
  `r.ctx.Done()`, not a shared `Ticker`/`Timer` field — it needs no explicit
  `Stop()` bookkeeping across multiple exit paths, at the cost of one small
  allocation per attempt, which is negligible given the attempt count is
  bounded and small.

## 5. `-race` risk from new shared fields on `TmuxServerRegistry`

The integration suite runs `-race` explicitly (AC1/AC3), so any new field
read/written across goroutines without synchronization will be caught by CI,
not shipped silently — but it's worth designing it correctly up front rather
than discovering it via a red `-race` run:

- **A plain `bool`/flag field** (e.g. `elevatedBackoff bool`, or a
  "fast-recheck in progress" flag) written by `reconnectLoop` and read by the
  new fast-recheck goroutine's start condition **must** reuse the existing
  `healthMu sync.RWMutex` pattern (already used for exactly this shape —
  `healthy bool` at lines 53-54) or get a dedicated mutex. A bare field
  access "to avoid lock overhead" is precisely what `-race` exists to catch.
- **A signal/notify channel** used to wake the fast-recheck loop early when
  `reconnectLoop` reconnects successfully (so the fast path can stand down)
  is race-safe under Go's memory model *only* if it's never written-to after
  close and never closed twice. If the design closes a single long-lived
  channel repeatedly across reconnect cycles to broadcast "reconnected," that
  panics on the second close — it needs the same `sync.Once`-guarded close
  discipline `paneExitSub` already uses (line 33-37), or the channel must be
  recreated fresh under a lock each cycle rather than reused.
- **Existing precedent worth flagging, not fixing (out of scope):** the
  debounce state at lines 403-407 (`debounceTimer`, `debounceMu`,
  `debounceDelay`) is **package-level**, not a field on
  `TmuxServerRegistry` — meaning *all* registries in the process (e.g. two
  different sockets, common in tests that create several isolated
  registries) currently share one debounce timer, so a `%sessions-changed`
  event on socket A can cancel/reschedule a debounce timer another
  registry's socket B is relying on. This is properly mutex-guarded (so it
  won't trip `-race`), but it's the wrong scope. Do **not** copy this
  pattern for new fast-recheck state — any new coalescing/tracking field
  must live on the `TmuxServerRegistry` struct itself, guarded by a lock,
  per-instance — not as a new bare package-level `var`.
- **`Start()` already writes `r.ctx`/`r.cancel` without a lock**
  (lines 84-87: `r.cancel(); r.ctx = childCtx; r.cancel = childCancel`),
  tolerated today only because `Start()` is a single-caller precondition
  executed *before* any goroutine that reads `r.ctx` is launched (`go
  r.reconnectLoop()` comes after, line 95). Any new fast-recheck bookkeeping
  field must follow the same ordering discipline: initialize the field(s),
  *then* `go r.fastRecheckLoop(...)` — never the reverse — and must not be
  written again by a second concurrent `Start()`/reconnect cycle while a
  prior fast-recheck goroutine from an earlier cycle might still be running.
  This is the same overlap risk as point 1/2: if `reconnectLoop` can trigger
  a new fast-recheck cycle before the previous cycle's (bounded, but up to
  ~700ms) goroutine has exited, you get two fast-recheck instances plus
  `reconnectLoop`'s own post-connect sync racing each other on
  `syncSessions()` simultaneously — three concurrent callers instead of two,
  compounding point 1's lost-update hazard. The fast-recheck loop should be
  scoped/joined per disconnect cycle (e.g., only launched once per
  `reconnectLoop` iteration and guaranteed to have exited — via ctx-done or
  attempt-exhaustion — before that cycle's iteration considers itself done),
  not left to potentially outlive the cycle that spawned it.

## Summary of concrete recommendations for the implementation

1. Serialize `syncSessions()` so only one fetch-diff-swap sequence is ever
   in flight at a time (dedicated mutex or single-flight guard covering the
   whole sequence, not just the map swap) — fixes the lost-update/ghost-
   session race (§1) and prevents overlapping-goroutine pileups (§2, §5).
2. Make every fast-recheck loop iteration boundary `select` on
   `r.ctx.Done()`; do not rely on `syncSessions()`'s existing
   `context.Background()`-rooted 10s timeout for cancellation — derive a
   fresh timeout from `r.ctx` for whatever fetch call the fast-recheck path
   makes.
3. Use `time.After(fastRecheckInterval)` per attempt inside a `select`
   against `r.ctx.Done()`, not a persistent `Ticker`/reused `Timer` —
   avoids the `Stop()`-on-every-exit-path discipline entirely.
4. Guard any new shared field (flag, signal channel, counter) with an
   existing or new mutex, scoped per-`TmuxServerRegistry` instance — never a
   bare field or a new package-level `var` (the existing `debounceTimer`
   package-level pattern is precedent to avoid repeating, not to follow).
5. Scope the fast-recheck goroutine's lifetime to a single disconnect/
   reconnect cycle and ensure it has fully exited before the next cycle
   could spawn another, so at most one fast-recheck goroutine (plus
   `reconnectLoop`'s own sync call, itself serialized per recommendation 1)
   is ever running.
6. Give the new regression test real headroom over the nominal 700ms bound
   (mirroring why `PaneExitChannel` uses a 3s deadline, not a number close
   to the computed minimum), and drive its "elevated backoff" precondition
   via a deterministic signal rather than a fixed sleep racing real
   wall-clock backoff growth.
