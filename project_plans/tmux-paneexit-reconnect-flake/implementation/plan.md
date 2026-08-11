# Implementation Plan: tmux-paneexit-reconnect-flake

**Feature**: Decouple `TmuxServerRegistry` pane-exit detection latency from `reconnectLoop`'s exponential backoff via a bounded, mutex-serialized fast-recheck helper, fixing `TestTmuxServerRegistry_PaneExitChannel`'s flake at its root cause.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: None new — stdlib-only, hand-rolled helper following the in-package `ensureServerRunningWithRetry` (`session/tmux/tmux.go:478`) convention; no non-standard technology choice rises to ADR weight for a ~70-line, two-file bugfix. Complies with the existing `docs/adr/003-no-static-sleeps-in-tests.md` ("No Static Sleeps in Tests," Accepted): Task 1.2.1a's regression test uses a condition-driven polling helper (`waitForReconnectCycles`, counting `IsHealthy()` transitions), not a static `time.Sleep`.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `TmuxServerRegistry` | Struct maintaining one control-mode connection per tmux socket and an in-memory `sessions` map subscribers query instead of forking tmux. | `session/tmux/server_registry.go:42-58` |
| `reconnectLoop` | Goroutine (one per registry, started in `Start()`) that starts control-mode, reads its events until it exits, then waits out an exponential backoff before retrying. | `server_registry.go:309-400` |
| `syncSessions` | **Blocking** wrapper: `r.syncMu.Lock(); defer r.syncMu.Unlock(); return r.syncSessionsLocked(ctx, timeout)`. Used by the 3 pre-existing callers with no tight latency budget (`Start`, `reconnectLoop` post-connect, debounce callback). | Currently `server_registry.go:210-246`; this plan splits it into `syncSessions` + `syncSessionsLocked` + `syncSessionsFastRecheck` |
| `syncSessionsLocked` | **New** unexported method holding the actual fetch→diff→swap body (list-sessions, diff against `r.sessions`, swap, `firePaneExit` for disappeared sessions). Callers MUST already hold `syncMu` — this method never acquires/releases it. Shared by both `syncSessions` (blocking) and `syncSessionsFastRecheck` (non-blocking) so the diff/fire logic exists exactly once. | New, in `server_registry.go` |
| `syncSessionsFastRecheck` | **New** unexported method used only by `waitBackoffWithFastRecheck`. `TryLock`s `syncMu`; if already held, returns `nil` immediately (no-op) instead of blocking. This is what keeps the 700ms ceiling honest under contention — resolves the priority-inversion bug both architecture-review and adversarial-review independently flagged in the original plain-`Lock()` design. | New, in `server_registry.go` |
| `backoff` | `reconnectLoop`'s function-local `time.Duration`, doubling from `backoffBase` (100ms) to `backoffCap` (30s) on every connection that dies before `minStableConnection`. | `server_registry.go:311-314` |
| `minStableConnection` | 5s threshold; a connection alive at least this long resets `backoff` to `backoffBase` on its next disconnect. | `server_registry.go:366-369` |
| `waitBackoffWithFastRecheck` | **New** unexported helper replacing the two `time.After(backoff)` waits in `reconnectLoop`; blocks up to `backoff` (or `r.ctx.Done()`) like before, but also fires a bounded number of independent `syncSessionsFastRecheck()` calls while waiting. The 700ms figure it documents bounds fast-recheck's *own* polling delay, not overall detection latency in every possible contention scenario — see `syncMu` row below. | New, in `server_registry.go` |
| `fastRecheckAttempts` | **New** constant, `2` — number of independent `syncSessionsFastRecheck()` calls `waitBackoffWithFastRecheck` makes per backoff-wait. | Local to the new helper |
| `fastRecheckSyncTimeout` | **New** constant, `150 * time.Millisecond` — subprocess timeout for each fast-recheck `syncSessionsFastRecheck()` call (applies only once `syncMu` is acquired via `TryLock`; the `TryLock` itself never blocks). | Local to the new helper |
| `fastRecheckInterval` | **New** constant, `200 * time.Millisecond` — wait between fast-recheck attempts. | Local to the new helper |
| `defaultSyncTimeout` | **New** package-level constant, `10 * time.Second` — replaces the hardcoded `10*time.Second` at the 3 pre-existing `syncSessions()` call sites (`Start`, `reconnectLoop` post-connect, debounce callback). | New, package scope in `server_registry.go` |
| `syncMu` | **New** `sync.Mutex` field on `TmuxServerRegistry`, held for `syncSessionsLocked()`'s entire fetch→diff→swap sequence. The 3 pre-existing callers (debounce timer, `reconnectLoop` post-connect, `Start`) acquire it via blocking `syncSessions` (`Lock()`); the fast-recheck path acquires it via non-blocking `syncSessionsFastRecheck` (`TryLock()`, skip-if-busy) so a fast-recheck attempt never waits on it and never contributes to `Stop()`'s shutdown latency. | New field, fixes the pitfalls.md §1 lost-update race; `TryLock` split fixes the architecture-review/adversarial-review Blocker 1 unbounded-wait issue in the original design |
| `firePaneExit` | Closes every subscriber channel for a session name; copies subscribers out under `subsMu`, closes outside the lock. Already double-fire-safe (deletes the map entry before closing). | `server_registry.go:198-207`, unchanged |
| `paneExitSub` / `subsMu` | Per-subscriber struct with a `sync.Once`-guarded close; `subsMu` is the mutex guarding the subscriber map, with the "never `close(ch)` while holding `subsMu`" discipline documented at lines 48-50. | Unchanged |
| keepalive session | Sentinel tmux session (`TmuxPrefix+"keepalive"`) that control-mode's `attach-session` targets. On isolated test sockets it is NOT auto-recreated by `startControlMode()` — only the default (empty) socket gets that behavior. | `server_registry.go:256-275`; exploited by the new regression test to force real backoff growth |
| control-mode | `tmux -C attach-session -t keepalive` subprocess; its stdout event stream (`%session-created`, `%session-closed`, `%pane-exited`, `%sessions-changed`, `%exit`) is the live-detection path, live only while connected. | `startControlMode`, `readLines`, `handleEvent` |
| `waitForReconnectCycles` | **New** test helper (`server_registry_integration_test.go`) that polls `registry.IsHealthy()` and counts complete false→true→false pulses — each pulse corresponds to one full `reconnectLoop` iteration (connect attempt, brief healthy window while `syncSessions` runs, `readLines` returning). Used to know structurally, not by wall-clock guess, when `backoff` has doubled past a target value. Condition-driven, so it satisfies ADR-003 (No Static Sleeps in Tests) — replaces the earlier `time.Sleep(2 * time.Second)` design. | New, `server_registry_integration_test.go` |
| `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff` | **New** regression test (external `tmux_test` package) that forces real backoff growth by killing the keepalive session once, uses `waitForReconnectCycles` to wait until `backoff` has deterministically doubled to 3200ms, then verifies a target session's pane-exit is still detected within a bounded window well under what backoff alone would take. Does **not** exercise the `syncMu` `TryLock`-contention scenario — documented as an accepted gap (see Unresolved Questions). | New, `server_registry_integration_test.go` |

---

## Pattern Decisions

### Step 0.5 — alternatives considered for the core mechanism

| # | Approach | Strength | Weakness |
|---|----------|----------|----------|
| A (chosen) | Inline synchronous `waitBackoffWithFastRecheck` helper, called from `reconnectLoop`'s existing goroutine in place of the two bare `time.After(backoff)` waits. | No new goroutine lifecycle — inherits `r.ctx`/`Stop()` cancellation for free; fast-recheck's `syncSessions()` calls are strictly sequential with `reconnectLoop`'s own post-connect sync (never overlapping). | Fast-recheck only runs during backoff waits, not e.g. mid-`startControlMode()` — but per requirements' root-cause analysis, that's the only gap that exists, so this is a fit not a limitation. |
| B (rejected) | Free-running `time.Ticker` goroutine started once in `Start()`, calling `syncSessions()` on a fixed period for the registry's whole lifetime. | Simple mental model (matches `StartZombieReaper`/`hibernation_sweeper` ticker idiom already in the codebase). | Runs continuously even while control-mode is healthy (pure waste — the whole point of control-mode is to make polling unnecessary); needs its own child context captured fresh across every `Start()` call (which replaces `r.ctx`/`r.cancel`, `server_registry.go:84-87`) or it leaks/goes stale; adds a second free-running caller of `syncSessions()` racing `reconnectLoop`'s own sync indefinitely, not just during a bounded window. |
| C (rejected) | Refactor `backoff` from a `reconnectLoop`-local variable into a struct field / small state machine, exposing it (or a derived signal) so tests can set it directly and production code can branch on its magnitude. | Would make the regression test trivial (no need to force real reconnect failures) and open a path to richer backoff tuning later. | Violates the requirements' explicit non-goal ("Reworking the reconnect/backoff algorithm itself... stays as-is") and the frozen-exported-API constraint if any new field/method were exposed for tests; changes internal loop state shape more invasively than the stated goal, for no production benefit. |

Approach A is the strongest: it fixes exactly the root-cause window (backoff-wait), touches the fewest lines, and requires no new goroutine-lifecycle or state-shape design.

### Per-component pattern choices

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Fast-recheck timing loop | Plain bounded `for` loop + `select`/`time.After` (no GoF/PoEAA pattern needed — this is the idiom `ensureServerRunningWithRetry` and `reconnectLoop` itself already use) | `session/tmux/tmux.go:478`, `server_registry.go:307-400` | `time.Ticker`-based free-running goroutine (Approach B above) | Ticker implies indefinite periodicity; wrong shape for "N bounded attempts then stop," and needs its own `Stop()`/leak discipline on every exit path (pitfalls.md §2, §4) |
| `syncSessions()` concurrency guard | Dedicated `syncMu sync.Mutex` held for the whole fetch→diff→swap sequence — plain mutual exclusion, no pattern name needed | pitfalls.md §1 recommendation | `golang.org/x/sync/singleflight` (already a repo dependency, used in `session/git/worktree.go:15`, `session/tmux/tmux.go:27`) | Wrong shape: singleflight collapses *identical concurrent calls* into one result: it doesn't guarantee ordering-by-freshness, so a slow in-flight call could still be the one whose stale result "wins" if it's the one all callers are coalesced onto. A plain mutex around the full sequence directly prevents the interleaving that causes the lost-update/ghost-session bug. |
| `syncSessions()` concurrency guard (2nd alternative) | (same as above) | pitfalls.md §1 | Bare `atomic.Bool` "sync in progress" flag, skip if set | Signals overlap but doesn't serialize execution — a fast-recheck attempt that sees "in progress" and skips its own call still needs *something* to wait on the in-flight result, which is a mutex/channel anyway; simpler to just serialize directly. |
| Fast-recheck's `syncMu` acquisition strategy | Non-blocking `TryLock()` + skip-if-busy, via `syncSessionsFastRecheck` calling a shared `syncSessionsLocked` body | architecture-review Blocker 1 remediation + adversarial-review Blocker 1 remediation (both reviews independently converged on this same fix for the same root cause) | (a) Narrow `syncMu` to guard only the diff+swap (not the fetch) and add a monotonic generation/timestamp check so a later-started call always wins the race regardless of finish order; (b) make the `Lock()` wait itself boundable via a buffered-channel-based mutex raced against `time.After`/`r.ctx.Done()` | (a) requires threading a sequence number through `syncSessionsLocked`'s signature/return and still leaves the *fetch* itself unserialized across concurrent callers (two `list-sessions` subprocesses running concurrently), which the current single-fetch design deliberately avoids; (b) `sync.Mutex` has no native cancellable/timed `Lock()` — reimplementing one via a channel adds real complexity for a case Go 1.18+'s built-in `TryLock()` already handles directly and simply |
| `syncSessions()` timeout | Widen existing method signature (`ctx context.Context, timeout time.Duration`) — plain refactor, no new type | architecture.md §2 | New separate `fastSyncSessions()` function with its own shorter timeout | Would duplicate the diff-and-fire logic (`server_registry.go:220-243`) a second time, directly violating "don't fix the race by duplicating the mechanism" — the diff/fire logic must stay singular per requirements Goal 3; this is now embodied by `syncSessionsLocked` being the single shared body for both the blocking and `TryLock` entry points |
| Retry/backoff mechanism itself | Hand-rolled helper in-package | build-vs-buy.md | `github.com/cenkalti/backoff/v5` | Present in `go.sum` only as an *indirect* transitive dependency (via otel's retry internals) — not imported by any repo package today; making it direct violates the "must not introduce a new dependency" constraint |
| Retry/backoff mechanism itself (2nd alternative) | (same as above) | build-vs-buy.md | `testutil/wait.WaitForCondition` / `RetryOperation` | Test-only package (`testutil`); importing it from production code (`server_registry.go`) inverts the intended dependency direction and doesn't fit the ceiling-math shape needed anyway |
| Regression test's backoff-elevation mechanism | Kill the keepalive session once via the isolated tmux socket's real `exec.Command`, then poll `IsHealthy()` pulse transitions (via the new `waitForReconnectCycles` helper) until `reconnectLoop`'s own repeated failed-attach cycles have deterministically doubled `backoff` past the target value — pure black-box, condition-driven, no pattern name needed | architecture.md's "Constraint check" section; adversarial-review Blocker 3 remediation | (a) Add a test-only exported hook/field exposing `backoff`'s current value; (b) a fixed `time.Sleep` margin (the plan's original design) | (a) violates the frozen exported-API-surface constraint (`SubscribePaneExit`, `SessionExists`, `ListSessions`, `IsHealthy`, `Start`, `Stop`, plus package-level registry functions) and the file-confinement AC — `server_registry_integration_test.go` is external `package tmux_test` and has zero access to unexported state by design; (b) violates ADR-003 (No Static Sleeps in Tests) and never verifies its own precondition (that `backoff` actually reached the target value before proceeding), risking "a second flake of the same species it's meant to catch" per adversarial-review (echoing research/pitfalls.md §3.2) |

This is a small, targeted Go concurrency bugfix in one existing file plus its integration test — not a new system. Most rows above correctly land on "no pattern needed, plain function/mutex," which is the expected and appropriate answer here.

---

## Observability Plan
- **Logs**: Existing `log.Warn("[registry] control-mode start failed, retrying", ...)` and `log.Info("[registry] control-mode exited; reconnecting", ...)` already fire around both call sites the fast-recheck helper wraps. One new line: fast-recheck's `syncSessionsFastRecheck()` call logs its error at `Debug` level (`log.Debug("[registry] fast-recheck sync failed", "err", err)`) instead of silently dropping it — `Debug` rather than the debounce callback's `Warn` because fast-recheck runs every ~200-350ms during a backoff wait, so a recurring failure during an extended outage would otherwise be completely invisible, but logging it at `Warn` cadence would be noisy relative to the debounce path's much lower frequency. A skipped attempt (the `TryLock` failed — another sync is already in flight) is not an error and is not logged at all.
- **Metrics**: No new metrics needed — this is a latency-bound-correctness fix, not a new observable subsystem, and the repo has no existing metrics on `TmuxServerRegistry` to extend consistently.
- **Alerts**: No new alerts required.

## Risk Control
- **Feature flag**: Not gated — this is a pure latency/correctness fix to existing detection behavior with no externally observable behavior change beyond "detects faster." Gating it would add complexity disproportionate to a stdlib-only, two-file diff.
- **Rollback procedure**: Revert the commit touching `session/tmux/server_registry.go` and `session/tmux/server_registry_integration_test.go`; no schema, config, or persisted state is touched, so a plain `git revert` is sufficient with no follow-up cleanup.
- **Staged rollout**: Full rollout on merge — internal-only tmux session-lifecycle detection code, exercised entirely by the existing integration test suite before merge.

## Unresolved Questions
- [ ] None blocking. The prior tuning note about the regression test's fixed `time.Sleep(2 * time.Second)` margin no longer applies — Task 1.2.1a now uses the condition-driven `waitForReconnectCycles` helper (counts `IsHealthy()` pulse transitions) instead of a wall-clock sleep, which both resolves the ADR-003 (No Static Sleeps in Tests) conflict flagged in architecture-review and verifies the precondition (that `backoff` actually reached the target value) structurally instead of assuming it from elapsed time, per adversarial-review's Blocker 3 remediation.
- [ ] Accepted gap, not blocking: neither this plan's regression test nor any other test in the two allowed files exercises (a) the `syncMu` `TryLock`-contention path (a fast-recheck attempt skipped because another caller holds the lock) or (b) the lost-update/ghost-session race `syncMu` exists to prevent in the first place. Both would need either unexported access to `syncMu`/internal timing, or a reliable way to force `list-sessions` latency, from outside the package — only reachable from `server_registry_test.go`, a third file outside this fix's AC6 file-confinement (`session/tmux/server_registry.go` + `session/tmux/server_registry_integration_test.go` only). Documented here per architecture-review's own stated minimum remediation for this gap ("at minimum, note this gap explicitly in the test's doc comment... rather than assuming 20/20 green covers the whole fix") rather than expanding scope to a third file. The regression test's doc comment also calls this out (see Task 1.2.1a).
- [ ] Accepted, inherited (not introduced) tradeoff: holding `syncMu` across the whole subprocess call for the 3 blocking callers means `Stop()`'s shutdown latency is not instant if a queued `Lock()` waiter is blocked behind a slow in-flight sync (up to `defaultSyncTimeout`=10s) — `sync.Mutex.Lock()` has no `ctx`-cancellation escape. This is the same shape today's single-caller `syncSessions()` already has relative to `Stop()`/`r.ctx` cancellation (a slow in-flight `list-sessions` subprocess already bounds `Stop()`'s effective latency), not new exposure introduced by this fix — the `TryLock`-based fast-recheck path specifically avoids adding to it, since it never blocks on `syncMu` at all. Out of scope for this bugfix (would require a cancellable-mutex primitive Go's stdlib doesn't provide); noted here as a candidate follow-up if `Stop()` latency is ever observed to be a real problem.
- [ ] Accepted, pre-existing (amplified, not introduced) risk — pre-mortem.md failure #1: `NotifySessionCreated` (`server_registry.go:151-155`) writes `r.sessions[name] = true` directly under `r.mu`, bypassing `syncMu` entirely. A `syncSessionsLocked` call whose `list-sessions` fetch started *before* a session was created can still win the `r.mu` swap *after* `NotifySessionCreated` marked it present, transiently erasing it — a TOCTOU race that exists on `main` today, independent of this fix. Fast-recheck adds up to `fastRecheckAttempts`=2 extra `syncSessionsLocked` calls per backoff wait, which widens how often this window opens specifically during the connection-instability periods this fix targets (more polling, same race). A monotonic sequence/timestamp guard in `syncSessionsLocked`'s swap would close it properly, but doing so is out of scope for this bugfix (AC6 file-confinement; the race isn't specific to pane-exit detection). Documented here per pre-mortem's Prevention note rather than silently left for a future session to rediscover.
- [ ] Accepted, bounded tradeoff — pre-mortem.md failure #3: fast-recheck's extra `list-sessions` forks (up to 2 per backoff wait, 150ms-timeout each) reintroduce a small, bounded dose of the fork-rate pressure `backoff` exists to protect an unhealthy tmux server from. Not gated behind consecutive-failure counting, since doing so would add real complexity (new counter state, another threshold to tune) to guard against a small, capped exposure (2 extra short-timeout forks per cycle, not an unbounded ticker) — judged disproportionate for this fix's scope. Noted as a candidate follow-up if fork-rate pressure is ever observed to be a real problem at scale (many concurrent registries during a systemic tmux/host issue).
- [ ] Candidate follow-up, not required for this fix — pre-mortem.md failure #5: the accepted `syncMu` contention/lost-update coverage gap (row above) is concretely realized by the `NotifySessionCreated` interaction, not just theoretical. adversarial-review's suggested black-box stress test (rapid session create/kill churn racing an intermittent keepalive kill/restore, asserting no transient flicker either direction) would fit `server_registry_integration_test.go` without unexported access, but is not required to close this bugfix's 6 ACs — left as a follow-up rather than expanded scope.

## Dependency Visualization

```
1.1.1a (syncMu field + defaultSyncTimeout const)
   |
   v
1.1.1b (split syncSessions -> blocking syncSessions + syncSessionsLocked body)
   |
   v
1.1.1c (update 3 existing call sites: Start, reconnectLoop post-connect, debounce)
   |
   v
1.1.2a (syncSessionsFastRecheck [TryLock] + waitBackoffWithFastRecheck + constants + ponytail comment)
   |
   v
1.1.2b (wire helper into both reconnectLoop backoff-wait sites)
   |
   +-------------------------------------------+
   |                                           |
   v                                           v
1.2.1a (regression test: elevate backoff)   2.1.1a (AC1/AC2: -count=20 flaky-test run)
   |                                           |
   v                                           |
1.2.1b (regression test: assert fast detect)  |
   |                                           |
   +-------------------+-----------------------+
                        v
              2.1.2a (AC3: full package -race run)
                        |
                        v
              2.1.3a (AC4: make ci)
                        |
                        v
              2.1.4a (AC5: ponytail comment check)
                        |
                        v
              2.1.5a (AC6: git diff --name-only confinement check)
```

---

## Phase 1: Core Fix

### Epic 1.1: Decouple `syncSessions` from backoff timing (production code)
**Goal**: Make `syncSessions()` safely callable from a second, independent, bounded caller, then add that caller into `reconnectLoop`'s backoff waits.

#### Story 1.1.1: Serialize and parameterize `syncSessions()`, splitting a shared locked body out for reuse
**As a** `TmuxServerRegistry` maintainer, **I want** `syncSessions()`'s fetch-diff-swap sequence to be mutex-serialized, its subprocess timeout parameterized, and its body factored so a non-blocking caller can reuse it, **so that** a second (fast-recheck) caller can safely and *non-blockingly* check for updates — without waiting behind the existing three callers, without a lost-update race, and without hardcoding a 10s timeout on a path that needs a 150ms one.
**Acceptance Criteria**:
- `syncSessionsLocked()` never lets a slower call's stale snapshot overwrite a newer one already applied to `r.sessions`, for the two callers that do serialize (the 3 pre-existing blocking callers via `syncSessions`, and any one fast-recheck attempt that won the `TryLock`).
  - *Given* two goroutines each successfully acquire `syncMu` and call `r.syncSessionsLocked()` in sequence (e.g. the debounce timer callback via blocking `syncSessions`, followed by a fast-recheck attempt that later acquires the now-free lock via `syncSessionsFastRecheck`), *When* the slower call's `list-sessions` subprocess returns after the faster call has already swapped `r.sessions`, *Then* the slower call's swap happens strictly after and does not silently resurrect a session the faster call already removed and fired pane-exit for — enforced because `syncMu` fully serializes any two calls to `syncSessionsLocked` end-to-end, never letting two calls run concurrently, rather than only serializing the map swap.
- A fast-recheck attempt that cannot immediately acquire `syncMu` never blocks waiting for it.
  - *Given* `syncMu` is currently held by another caller (e.g. the debounce callback, using blocking `syncSessions` with `defaultSyncTimeout`=10s), *When* `syncSessionsFastRecheck` is called, *Then* its `TryLock()` fails immediately, it returns `nil` without touching `r.sessions` or calling `syncSessionsLocked` at all, and the caller (`waitBackoffWithFastRecheck`) proceeds to its next step without having blocked on the mutex — the in-flight caller's own eventual completion is what reflects the disappearance in `r.sessions` in this scenario, not this skipped attempt.
- All 3 pre-existing call sites (`Start`, `reconnectLoop` post-connect, debounce callback) compile against the new `syncSessions(ctx context.Context, timeout time.Duration) error` signature and behave identically to before (same 10s timeout, same `r.ctx` cancellation semantics, still always blocking).
  - *Given* `Start(ctx)` is called, *When* it invokes `r.syncSessions(r.ctx, defaultSyncTimeout)` at what is currently line 90, *Then* the bootstrap sync behaves exactly as before (still logs a warning and continues on failure, same 10s subprocess budget, still blocks until `syncMu` is acquired since `Start` has no tight latency budget).
**Files**: `session/tmux/server_registry.go`

##### Task 1.1.1a: Add `syncMu` field and `defaultSyncTimeout` constant (~3 min)
- In the `TmuxServerRegistry` struct (currently `server_registry.go:42-58`), add a new field directly below `subsMu`/`subscribers` (or below `healthy`, either is fine as long as it's grouped with its own doc comment, not bare):
  ```go
  // syncMu serializes syncSessionsLocked()'s fetch -> diff -> swap sequence so
  // two calls can never interleave and let a slower, stale call overwrite a
  // newer snapshot already applied to r.sessions. The 3 pre-existing callers
  // (debounce timer, reconnectLoop post-connect, Start) acquire it via the
  // blocking syncSessions(); the fast-recheck path acquires it via the
  // non-blocking syncSessionsFastRecheck() (TryLock, skip-if-busy) so a
  // fast-recheck attempt never waits on this mutex -- see
  // waitBackoffWithFastRecheck's ponytail: comment for why that matters.
  syncMu sync.Mutex
  ```
- Directly above the (soon-to-be-modified) `syncSessions` method — currently right after `firePaneExit` ends at line 207 — add:
  ```go
  // defaultSyncTimeout is the list-sessions subprocess budget used by every
  // syncSessions() caller except the fast-recheck path (see
  // fastRecheckSyncTimeout in waitBackoffWithFastRecheck).
  const defaultSyncTimeout = 10 * time.Second
  ```
- Files: `session/tmux/server_registry.go`

##### Task 1.1.1b: Split `syncSessions` into a blocking wrapper and a lock-assuming body (~6 min)
- Replace the current `syncSessions()` (lines 210-246) with two functions:
  ```go
  // syncSessions acquires syncMu and runs the fetch-diff-swap sequence. Used
  // by the 3 pre-existing callers with no tight latency budget (Start,
  // reconnectLoop's post-connect sync, the debounce callback). The
  // fast-recheck path uses syncSessionsFastRecheck instead (see
  // waitBackoffWithFastRecheck) so it never blocks on syncMu. ctx bounds
  // cancellation (Stop() aborts a call in flight); timeout bounds the
  // subprocess itself.
  func (r *TmuxServerRegistry) syncSessions(ctx context.Context, timeout time.Duration) error {
  	r.syncMu.Lock()
  	defer r.syncMu.Unlock()
  	return r.syncSessionsLocked(ctx, timeout)
  }

  // syncSessionsLocked runs list-sessions, diffs the result against
  // r.sessions, swaps the map, and fires pane-exit for every session that
  // disappeared. Callers MUST already hold syncMu for the whole call -- this
  // method never acquires or releases it itself, so the two lock-acquisition
  // strategies (blocking Lock in syncSessions, non-blocking TryLock in
  // syncSessionsFastRecheck) share one fetch-diff-swap implementation instead
  // of duplicating it.
  func (r *TmuxServerRegistry) syncSessionsLocked(ctx context.Context, timeout time.Duration) error {
  	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
  	defer cancel()
  	args := prependSocket(r.serverSocket, []string{"list-sessions", "-F", "#{session_name}"})
  	cmd := safeexec.CommandContext(fetchCtx, Binary(), args...)
  	out, err := cmd.Output()
  	if err != nil {
  		return fmt.Errorf("list-sessions: %w", err)
  	}

  	sessions := make(map[string]bool)
  	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
  		if name != "" {
  			sessions[name] = true
  		}
  	}

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

  	return nil
  }
  ```
- Note: the diff/fire logic in `syncSessionsLocked` is byte-for-byte unchanged from today's `syncSessions` — only the signature, the `syncMu`/`syncSessionsLocked` split, and rooting the subprocess `context.WithTimeout` off the passed-in `ctx` (was `context.Background()`) changed. `syncSessionsFastRecheck`, the third (non-blocking) entry point that also calls `syncSessionsLocked`, is added in Task 1.1.2a alongside `waitBackoffWithFastRecheck` since it exists only to serve that helper.
- Files: `session/tmux/server_registry.go`

##### Task 1.1.1c: Update the 3 existing call sites (~4 min)
- `Start()` (currently line 90): `if err := r.syncSessions(); err != nil {` → `if err := r.syncSessions(r.ctx, defaultSyncTimeout); err != nil {`. (`r.ctx` is already the freshly-assigned child context at this point — set at lines 84-87, before this call.)
- `reconnectLoop` post-connect sync (currently line 342): `if err := r.syncSessions(); err != nil {` → `if err := r.syncSessions(r.ctx, defaultSyncTimeout); err != nil {`.
- Debounce callback inside `handleEvent` (currently line 456): `if err := r.syncSessions(); err != nil {` → `if err := r.syncSessions(r.ctx, defaultSyncTimeout); err != nil {`.
- Run `go build ./session/tmux/...` to confirm no other call sites were missed (should be exactly these 3, plus the new fast-recheck call added in Task 1.1.2a).
- Files: `session/tmux/server_registry.go`

#### Story 1.1.2: Add the bounded, non-blocking fast-recheck helper and wire it into `reconnectLoop`
**As a** caller of `SubscribePaneExit`, **I want** pane-exit detection to be checked independently a few times during any backoff wait, without ever blocking on a slower in-flight sync, **so that** my exit notification isn't bound by how large backoff has grown, or by what other callers happen to be doing at the same moment.
**Acceptance Criteria**:
- The fast-recheck ceiling is a concrete, documented number tied to the actual constants, and its scope is stated accurately (fast-recheck's own polling delay, not a ceiling on detection under all contention scenarios).
  - *Given* the new `waitBackoffWithFastRecheck` helper's constant block, *When* a reviewer reads the `ponytail:`-style comment directly above it, *Then* it states `fastRecheckAttempts × (fastRecheckSyncTimeout + fastRecheckInterval) = 700ms` using the actual constant values (`2 × (150ms + 200ms) = 700ms`), explains why this must stay decoupled from `backoff`, and explains that the `TryLock`/skip-if-busy design in `syncSessionsFastRecheck` is what makes this a ceiling on fast-recheck's *own* polling delay regardless of contention, rather than a claim about total detection latency in every scenario.
- Both backoff-wait sites in `reconnectLoop` use the new helper, not a bare `time.After(backoff)`.
  - *Given* `reconnectLoop`'s `err != nil` branch (currently lines 324-338) and its post-`readLines` branch (currently lines 382-398), *When* either branch's wait executes, *Then* it calls `r.waitBackoffWithFastRecheck(backoff)` and preserves the exact same `r.ctx.Done()`-triggers-early-`return` semantics as the code being replaced.
- Detection during an uncontended backoff wait is genuinely independent of backoff's magnitude.
  - **[Amended post-implementation — see requirements.md's "Post-implementation finding"]** *Given* `reconnectLoop` is sleeping out a 1600ms+ `backoff` wait (the `fastRecheckMinBackoff` threshold added during implementation after this AC's original 800ms example was found to measurably worsen `TestTmuxServerRegistry_PaneExitChannel`'s failure rate under heavy concurrent tmux load — extra `list-sessions` forks during short, early cycles provided zero detection benefit while adding fork pressure that compounded a separate, pre-existing "server exited unexpectedly" flake) and no other caller currently holds `syncMu`, *When* a tracked session's pane exits partway through that wait, *Then* `waitBackoffWithFastRecheck`'s first or second `syncSessionsFastRecheck()` attempt acquires `syncMu` via `TryLock` immediately, observes and fires the disappearance well before the backoff itself would have elapsed. **Below `fastRecheckMinBackoff` (100/200/400/800ms cycles), detection is NOT decoupled from backoff** — this narrows Goal 1's original "genuinely independent of backoff's magnitude" claim to "independent once backoff ≥ ~1.6s," a deliberate, evidence-based tradeoff, not an oversight.
- A fast-recheck attempt degrades gracefully — by skipping, not blocking — when `syncMu` is contended.
  - *Given* `syncMu` is held by another caller (e.g. the debounce callback, via the blocking `syncSessions` with `defaultSyncTimeout`=10s) for the entire duration of a given fast-recheck attempt's window, *When* `waitBackoffWithFastRecheck` calls `syncSessionsFastRecheck`, *Then* `TryLock()` fails, the call returns `nil` immediately without blocking, and `waitBackoffWithFastRecheck` proceeds straight to its `fastRecheckInterval` wait — so this specific fast-recheck attempt does no work, but the helper's own total elapsed time for its `fastRecheckAttempts` loop is unaffected by the contention (still bounded by `deadline`/`r.ctx.Done()`), unlike the original plain-`Lock()` design both reviews flagged.
**Files**: `session/tmux/server_registry.go`

##### Task 1.1.2a: Write `syncSessionsFastRecheck`, `waitBackoffWithFastRecheck`, constants, and the `ponytail:` comment (~7 min)
- Add these two new methods, placed directly after `reconnectLoop` (i.e., after its current closing brace at line 400), before the debounce `var` block:
  ```go
  // syncSessionsFastRecheck attempts syncSessionsLocked's fetch-diff-swap
  // sequence only if syncMu is immediately available. If another caller
  // (Start, reconnectLoop's post-connect sync, or the debounce callback) is
  // already mid-sync, this returns nil without doing any work instead of
  // blocking -- that in-flight call will itself observe and fire any
  // disappearance once it completes, so no detection is permanently lost,
  // only this specific fast-recheck attempt is skipped. This is what keeps
  // waitBackoffWithFastRecheck's own worst-case latency bounded to
  // fastRecheckAttempts * (fastRecheckSyncTimeout + fastRecheckInterval)
  // regardless of what any other caller is doing -- see the ponytail: comment
  // below.
  func (r *TmuxServerRegistry) syncSessionsFastRecheck(ctx context.Context, timeout time.Duration) error {
  	if !r.syncMu.TryLock() {
  		return nil
  	}
  	defer r.syncMu.Unlock()
  	return r.syncSessionsLocked(ctx, timeout)
  }

  // waitBackoffWithFastRecheck blocks for up to backoff (or until r.ctx is
  // cancelled) — the same contract as a plain time.After(backoff) wait — but
  // also makes a small, bounded number of independent syncSessionsFastRecheck
  // calls while it waits, so a pane exit during a long backoff sleep is
  // detected quickly instead of only on the next successful reconnect.
  func (r *TmuxServerRegistry) waitBackoffWithFastRecheck(backoff time.Duration) {
  	const (
  		fastRecheckAttempts    = 2
  		fastRecheckSyncTimeout = 150 * time.Millisecond
  		fastRecheckInterval    = 200 * time.Millisecond
  	)
  	// ponytail: fastRecheckAttempts * (fastRecheckSyncTimeout + fastRecheckInterval)
  	// = 700ms is a hard ceiling on fast-recheck's OWN polling delay during a
  	// backoff sleep -- not a ceiling on total detection latency regardless of
  	// what other callers are doing. Each attempt goes through
  	// syncSessionsFastRecheck, which TryLocks syncMu and skips (does no work,
  	// returns nil) rather than blocking if another caller (Start,
  	// reconnectLoop's post-connect sync, or the debounce callback -- all of
  	// which use the blocking syncSessions and can hold syncMu for up to
  	// defaultSyncTimeout=10s) is already mid-sync. So a fast-recheck attempt
  	// itself never waits on syncMu, and this loop's own elapsed time is
  	// always bounded by this ceiling. In the specific case where an attempt
  	// is skipped due to contention, the in-flight caller that holds the lock
  	// will itself observe and fire the disappearance once it finishes --
  	// just not necessarily within this 700ms window. backoff itself climbs
  	// 100ms..30s (backoffBase..backoffCap) and is tuned to protect a
  	// possibly-unhealthy tmux server from a reconnect fork-rate explosion
  	// (see minStableConnection above) -- detection latency is a separate,
  	// caller-facing guarantee (SubscribePaneExit) that must not inherit
  	// backoff's slowness. Keep these decoupled: widen backoff freely, but
  	// any change to these three constants must keep this ceiling accurate.
  	deadline := time.NewTimer(backoff)
  	defer deadline.Stop()

  	// failedAttempts is logged once after the loop, not per-attempt: this
  	// helper runs on every backoff-wait cycle, so a per-attempt log line
  	// during a real tmux-server outage becomes a hot loop logging concern
  	// (pre-mortem.md failure #2) — batching to one summary line per
  	// waitBackoffWithFastRecheck call keeps volume proportional to
  	// reconnect cycles, not fast-recheck attempts.
  	var failedAttempts int
  	for i := 0; i < fastRecheckAttempts; i++ {
  		select {
  		case <-r.ctx.Done():
  			return
  		case <-deadline.C:
  			return
  		default:
  		}

  		if err := r.syncSessionsFastRecheck(r.ctx, fastRecheckSyncTimeout); err != nil {
  			failedAttempts++
  		}

  		select {
  		case <-r.ctx.Done():
  			return
  		case <-deadline.C:
  			return
  		case <-time.After(fastRecheckInterval):
  		}
  	}
  	if failedAttempts > 0 {
  		log.Debug("[registry] fast-recheck sync failed", "failedAttempts", failedAttempts, "of", fastRecheckAttempts)
  	}

  	select {
  	case <-r.ctx.Done():
  	case <-deadline.C:
  	}
  }
  ```
- Files: `session/tmux/server_registry.go`

##### Task 1.1.2b: Replace both `time.After(backoff)` waits in `reconnectLoop` (~4 min)
- `err != nil` branch (currently lines 324-338):
  ```go
  if err != nil {
  	log.Warn("[registry] control-mode start failed, retrying", "err", err, "backoff", backoff)
  	r.waitBackoffWithFastRecheck(backoff)
  	select {
  	case <-r.ctx.Done():
  		return
  	default:
  	}
  	if backoff < backoffCap {
  		backoff *= 2
  		if backoff > backoffCap {
  			backoff = backoffCap
  		}
  	}
  	continue
  }
  ```
- Post-`readLines` branch (currently lines 382-398):
  ```go
  select {
  case <-r.ctx.Done():
  	return
  default:
  	log.Info("[registry] control-mode exited; reconnecting", "backoff", backoff)
  	r.waitBackoffWithFastRecheck(backoff)
  	select {
  	case <-r.ctx.Done():
  		return
  	default:
  	}
  	if backoff < backoffCap {
  		backoff *= 2
  		if backoff > backoffCap {
  			backoff = backoffCap
  		}
  	}
  }
  ```
- Run `go build ./session/tmux/...` and `go vet ./session/tmux/...` to confirm the rewritten control flow compiles and no unreachable-code/vet issues were introduced.
- Files: `session/tmux/server_registry.go`

### Epic 1.2: Regression test proving the fix structurally
**Goal**: A test that fails on unmodified `main` (or would, if it existed there) and passes only because fast-recheck genuinely decouples detection from backoff — not by re-running the flaky test until it happens to pass.

#### Story 1.2.1: `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff`
**As a** future maintainer, **I want** a regression test that specifically elevates `backoff` before killing a tracked session, using a condition-driven wait rather than a static sleep, **so that** any future change reintroducing the backoff-bound detection bug is caught structurally, not just by flaky re-runs, and so the test itself complies with ADR-003 (No Static Sleeps in Tests).
**Acceptance Criteria**:
- The test elevates real `backoff` growth using only the black-box exported API plus ordinary `exec.Command` calls (this file is external `package tmux_test`), and *verifies* the elevation happened rather than assuming it from wall-clock time.
  - *Given* an isolated registry from `startIsolatedRegistry(t)` that is healthy and already tracking a target session, *When* the test kills the keepalive session once via `exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", keepaliveName)` and then calls the new `waitForReconnectCycles(t, registry, minElevatedBackoffCycles, backoffElevationPollTimeout)` helper, *Then* the helper blocks until it has observed `minElevatedBackoffCycles` (5) complete `IsHealthy()` false→true→false pulses — each pulse corresponding to one full failed `reconnectLoop` reconnect attempt (keepalive stays gone — isolated sockets don't get it auto-recreated, `startControlMode`) — which, given `reconnectLoop`'s deterministic doubling sequence (100→200→400→800→1600→3200ms, never resetting since no attempt is ever stable for `minStableConnection`=5s), structurally guarantees `backoff` has reached 3200ms for the wait the loop is now in, with no access to the unexported `backoff` variable itself. The helper `t.Fatal`s with a clear message (including the pulse count observed) if `minElevatedBackoffCycles` pulses are not seen within `backoffElevationPollTimeout`, rather than silently proceeding on an unverified precondition.
- The test proves detection is decoupled from that elevated backoff, with a large enough margin that unfixed code would very likely fail this specific assertion (not just "would take longer than the ceiling in principle").
  - *Given* backoff has been elevated to 3200ms per above (a ~1.7s structural margin over the 1.5s assertion window below — bumped from an earlier 4-cycle/1600ms design per adversarial-review's and the Engineering triad lens's recommendation, since 1600ms left only a ~100ms margin over 1.5s, too thin to reliably distinguish fixed from unfixed code under timing jitter), so unfixed code — bound by backoff alone — would very likely still be waiting when the deadline below fires, *When* the test then subscribes to the target session's pane-exit and kills that session, *Then* the returned channel closes within a bounded window with real headroom over the 700ms ceiling (1.5s, mirroring `TestTmuxServerRegistry_PaneExitChannel`'s existing 3s-over-nominal margin) — fixed code detects it well within `fastRecheckAttempts × (fastRecheckSyncTimeout + fastRecheckInterval)` = 700ms of this window opening.
- **Known, documented gap** (not required to close for this AC): this test's backoff-elevation mechanism keeps control-mode down for its entire duration, so it never exercises the `syncMu` `TryLock`-contention path (a fast-recheck attempt skipped because another caller holds the lock) — see the test's own doc comment and the plan's Unresolved Questions section.
**Files**: `session/tmux/server_registry_integration_test.go`

##### Task 1.2.1a: Add `waitForReconnectCycles` helper and write the backoff-elevation setup phase (~8 min)
- Add this new helper to `server_registry_integration_test.go`, directly after `pollUntil`'s definition (currently ending at line 116):
  ```go
  // waitForReconnectCycles blocks until it has observed minCycles complete
  // IsHealthy() pulses (false -> true -> false). Each pulse corresponds to one
  // full reconnectLoop iteration: a connect attempt, a brief healthy window
  // while syncSessions runs (see reconnectLoop's runtime.Gosched() comment,
  // which exists specifically so this window is observable), then readLines
  // returning and the loop entering its next backoff wait. prevHealthy starts
  // at the registry's current IsHealthy() value so an already-true state at
  // call time (the tail of a still-live connection dropping) is never
  // miscounted as a completed pulse -- only a rise observed *during* this
  // call, followed by a fall, counts. This is a condition-driven replacement
  // for a fixed sleep (see docs/adr/003-no-static-sleeps-in-tests.md): it
  // verifies its own precondition instead of assuming it from elapsed time.
  // t.Fatal with the observed count if minCycles is not reached in time.
  func waitForReconnectCycles(t *testing.T, registry *tmux.TmuxServerRegistry, minCycles int, timeout time.Duration) {
  	t.Helper()
  	deadline := time.Now().Add(timeout)
  	pulses := 0
  	sawRise := false
  	prevHealthy := registry.IsHealthy()
  	for time.Now().Before(deadline) {
  		h := registry.IsHealthy()
  		switch {
  		case !prevHealthy && h:
  			sawRise = true
  		case prevHealthy && !h && sawRise:
  			pulses++
  			sawRise = false
  			if pulses >= minCycles {
  				return
  			}
  		}
  		prevHealthy = h
  		runtime.Gosched()
  	}
  	t.Fatalf("only observed %d/%d reconnect cycles within %s -- backoff did not grow as expected", pulses, minCycles, timeout)
  }
  ```
- Add the new test to `server_registry_integration_test.go`, after `TestTmuxServerRegistry_PaneExitChannel` (after its closing brace at line 187):
  ```go
  // Test 6 (regression): pane-exit is still detected quickly even while
  // reconnectLoop's backoff has grown large. Exercises the fastRecheckAttempts
  // x (fastRecheckSyncTimeout + fastRecheckInterval) = 700ms ceiling
  // structurally, not by re-running the flaky test until it happens to pass.
  //
  // Backoff-elevation mechanism: killing the isolated socket's keepalive
  // session (never auto-recreated off the default socket, see
  // startControlMode) forces every subsequent attach-session attempt to fail
  // near-instantly, so reconnectLoop's backoff doubles every cycle without
  // ever resetting (100->200->400->800->1600->3200ms...). waitForReconnectCycles
  // counts the resulting IsHealthy() true->false pulses to know, structurally
  // rather than by wall-clock guess, when backoff has reached the target
  // value. This replaces an earlier design that used a fixed
  // time.Sleep(2 * time.Second), which violated ADR-003 (No Static Sleeps in
  // Tests, docs/adr/003-no-static-sleeps-in-tests.md) and didn't verify its
  // own precondition.
  //
  // Known gap: this test elevates backoff via a clean control-mode outage, so
  // no %sessions-changed event / debounce callback ever fires during its
  // fast-recheck phase, and syncSessionsFastRecheck's TryLock never contends
  // with the blocking syncSessions() path. The syncMu-contention scenario
  // (fast-recheck skipping a check because another caller holds syncMu) is
  // therefore NOT exercised here. Verifying that scenario needs either
  // unexported access to syncMu or a way to reliably slow list-sessions from
  // outside the package, both unavailable to this external tmux_test package
  // (server_registry_test.go, the only place with that access, is a third
  // file outside this fix's file-confinement -- see requirements.md AC6) --
  // accepted as a documented gap rather than expanded scope.
  func TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff(t *testing.T) {
  	registry, socket := startIsolatedRegistry(t)
  	keepaliveName := tmux.TmuxPrefix + "keepalive"

  	pollUntil(t, registryPollTimeout, "registry did not become healthy initially", registry.IsHealthy)

  	sessionName := "testpaneexit-elevated-backoff"
  	newSessionWithRetry(t, socket, "-d", "-s", sessionName)
  	pollUntil(t, registryPollTimeout, "session not visible before elevating backoff", func() bool {
  		return registry.SessionExists(sessionName)
  	})

  	// Elevate reconnectLoop's backoff: kill the keepalive session once, then
  	// wait for enough reconnect cycles to have completed that backoff has
  	// deterministically doubled to 3200ms (100->200->400->800->1600->3200,
  	// one doubling per completed pulse). 3200ms (5 cycles, not the original 4)
  	// is chosen to sit comfortably past
  	// the 1.5s detection-assertion window below, so unfixed code (which has
  	// no fast-recheck and is bound by backoff alone) would very likely still
  	// be waiting when that window's deadline fires -- not just "slower in
  	// principle."
  	if out, err := exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", keepaliveName).CombinedOutput(); err != nil {
  		t.Fatalf("kill-session keepalive: %v (%s)", err, out)
  	}
  	const minElevatedBackoffCycles = 5
  	// backoffElevationPollTimeout is 8s, not a tight ~1.5s-nominal margin:
  	// pre-mortem.md failure #4 flagged that -race (2-10x slowdown) plus
  	// shared-CI-runner tmux-fork contention can push real per-cycle time well
  	// past the clean-local-run nominal figure, same class of slowdown this
  	// file's own registryPollTimeout/newSessionWithRetry comments already
  	// document for other tests in this package -- real headroom here avoids
  	// trading the original flake for a new one in the test meant to catch it.
  	const backoffElevationPollTimeout = 8 * time.Second
  	waitForReconnectCycles(t, registry, minElevatedBackoffCycles, backoffElevationPollTimeout)
  ```
- Files: `session/tmux/server_registry_integration_test.go`

##### Task 1.2.1b: Write the detection-assertion phase and close the test (~4 min)
- Immediately following Task 1.2.1a's code (same function body), add:
  ```go
  	ctx, cancel := context.WithCancel(context.Background())
  	t.Cleanup(cancel)
  	exitCh := registry.SubscribePaneExit(ctx, sessionName)

  	if out, err := exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", sessionName).CombinedOutput(); err != nil {
  		t.Fatalf("kill-session %s: %v (%s)", sessionName, err, out)
  	}

  	select {
  	case <-exitCh:
  		// detected despite elevated backoff -- the fix is working
  	case <-time.After(1500 * time.Millisecond):
  		t.Fatal("SubscribePaneExit channel not closed within 1.5s despite elevated backoff -- fast-recheck did not decouple detection from backoff")
  	}
  }
  ```
- This 1.5s bound is unchanged from the original design and still makes sense given the redesigned setup: backoff is now a verified 3200ms (bumped from 1600ms per adversarial-review's and the Engineering triad lens's recommendation, for a real ~1.7s structural margin rather than a thin ~100ms one), which only widens the margin between the fix's expected ~700ms-or-faster detection and what unfixed code could achieve in the same window.
- Run `go build -tags integration ./session/tmux/...` to confirm the new test and helper compile (no new imports are needed — `context`, `exec`, `runtime`, `time`, `tmux` are already imported in this file, `runtime` via `TestTmuxServerRegistry_StartsHealthy`'s existing use).
- Files: `session/tmux/server_registry_integration_test.go`

---

## Phase 2: Verification

### Epic 2.1: Confirm all 6 acceptance criteria mechanically
**Goal**: Every AC in requirements.md is checked by running the exact command it specifies, not asserted narratively.

#### Story 2.1.1: Reliability of the originally-flaky test (AC1, AC2)
**Acceptance Criteria**:
- 20/20 consecutive runs of the target test pass.
  - *Given* the Phase 1 changes are complete, *When* running `go test -race -tags integration ./session/tmux -run TestTmuxServerRegistry_PaneExitChannel -count=20`, *Then* the command exits 0 with all 20 subtests reported `PASS` and none `FAIL`.
- The fix addresses the root cause, not the test's deadline.
  - *Given* `server_registry_integration_test.go`'s `TestTmuxServerRegistry_PaneExitChannel`, *When* diffed against its pre-fix version, *Then* its `3 * time.Second` deadline (line 179 pre-fix) is unchanged — the fix lives entirely in `waitBackoffWithFastRecheck`, `syncSessionsFastRecheck`, and `syncSessions`'/`syncSessionsLocked`'s new serialization/parameterization in `server_registry.go`, confirmed by `git diff` showing no numeric change to that test's timeout.
**Files**: `session/tmux/server_registry.go`, `session/tmux/server_registry_integration_test.go`

##### Task 2.1.1a: Run the flaky test 20x and confirm the deadline is untouched (~3 min)
- Run: `go test -race -tags integration ./session/tmux -run TestTmuxServerRegistry_PaneExitChannel -count=20`
- Confirm output shows `--- PASS` 20 times for this test and the command's final line is `ok`.
- Run: `git diff -- session/tmux/server_registry_integration_test.go | grep -n '3 \* time.Second'` and confirm the `TestTmuxServerRegistry_PaneExitChannel` deadline line is not present in the diff's added/removed lines (only context, or absent — i.e., untouched).
- Files: none changed — verification only

#### Story 2.1.2: No regression to the rest of the package (AC3)
**Acceptance Criteria**:
- The full package (including both named sibling flakes and the new regression test) passes under `-race`.
  - *Given* Phase 1 is complete, *When* running `go test -race -tags integration ./session/tmux/...`, *Then* the command exits 0, and its output includes `--- PASS` for `TestEnsureServerRunning_NoOp`, `TestKillOrphanedControlModeClients`, and `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff`.
**Files**: `session/tmux/server_registry.go`, `session/tmux/server_registry_integration_test.go`

##### Task 2.1.2a: Run the full `session/tmux` package under `-race` (~3 min)
- Run: `go test -race -tags integration ./session/tmux/...`
- Confirm exit code 0 and grep the output for the three named tests (`TestEnsureServerRunning_NoOp`, `TestKillOrphanedControlModeClients`, `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff`) each showing `--- PASS`.
- Files: none changed — verification only

#### Story 2.1.3: Full CI pipeline (AC4)
**Acceptance Criteria**:
- `make ci` passes end-to-end.
  - *Given* Phase 1 is complete and Stories 2.1.1-2.1.2 are green, *When* running `make ci`, *Then* build, unit tests, integration tests, and lint all complete with exit code 0.
**Files**: N/A (repo-wide command)

##### Task 2.1.3a: Run `make ci` (~5 min, mostly wait time)
- Run: `make ci`
- Confirm the command's final exit code is 0; if lint flags anything in the two touched files (e.g. `gofmt`), fix it and re-run.
- Files: `session/tmux/server_registry.go`, `session/tmux/server_registry_integration_test.go` (only if lint requires formatting fixes)

#### Story 2.1.4: Ceiling documentation present (AC5)
**Acceptance Criteria**:
- The `ponytail:`-style comment naming the ceiling exists in the shipped diff.
  - *Given* the completed change, *When* running `git diff -- session/tmux/server_registry.go | grep -n "ponytail:"`, *Then* the output includes the line containing `fastRecheckAttempts * (fastRecheckSyncTimeout + fastRecheckInterval) = 700ms`.
**Files**: `session/tmux/server_registry.go`

##### Task 2.1.4a: Grep-verify the ponytail comment (~1 min)
- Run: `git diff -- session/tmux/server_registry.go | grep -n "ponytail:"`
- Confirm the matched line's surrounding context (view a few lines via `git diff` output directly) contains the `700ms` ceiling formula with the real constant values.
- Files: none changed — verification only

#### Story 2.1.5: Diff confinement (AC6)
**Acceptance Criteria**:
- The diff touches exactly the two allowed files.
  - *Given* the completed change on its branch, *When* running `git diff --name-only main...HEAD` (or the equivalent against the base commit the work started from), *Then* the output is exactly `session/tmux/server_registry.go` and `session/tmux/server_registry_integration_test.go`, in either order, with no other paths listed.
**Files**: N/A (repo-wide verification)

##### Task 2.1.5a: Run `git diff --name-only` and confirm the file list (~1 min)
- Run: `git diff --name-only main...HEAD` (substitute the actual base ref/commit this work branched from if not `main`)
- Confirm the output is exactly the two lines `session/tmux/server_registry.go` and `session/tmux/server_registry_integration_test.go` — no other file appears.
- Files: none changed — verification only
