# Architecture Review: tmux-paneexit-reconnect-flake
**Date**: 2026-08-06 (iteration 2)
**Verdict**: CONCERNS

## Scope of this pass
Targeted re-review of the current `project_plans/tmux-paneexit-reconnect-flake/implementation/plan.md`
against the 2 Blockers and 2 Concerns raised in iteration 1. Confirmed against
`requirements.md` and the ground-truth `session/tmux/server_registry.go` /
`session/tmux/server_registry_integration_test.go` (the plan is not yet
implemented — both files are still pre-fix on disk, as expected at this stage;
Task 1.1.1c's "3 call sites" claim was checked against the file's actual 3
current call sites of `r.syncSessions()`, lines 90/342/456, and matches).

## Blockers
None remaining. Both prior Blockers are resolved by the redesign (see below).

### Former Blocker 1 — `syncMu` unbounded wait — RESOLVED
The `TryLock()`/skip-if-busy split genuinely bounds `syncSessionsFastRecheck`'s
own latency regardless of contention:
- `syncSessions(ctx, timeout)` (blocking, 3 pre-existing callers) → `Lock(); defer Unlock(); return syncSessionsLocked(...)`.
- `syncSessionsFastRecheck(ctx, timeout)` → `if !r.syncMu.TryLock() { return nil }; defer Unlock(); return syncSessionsLocked(...)`.
- `syncSessionsLocked` is the only place holding the fetch/diff/swap body and is called from exactly these two entry points, both of which hold `syncMu` for the full call — traced all 3 pre-existing call sites (Task 1.1.1c: `Start` line 90, `reconnectLoop` post-connect line 342, debounce callback line 456) plus the new fast-recheck path; no path calls `syncSessionsLocked` without the lock held, and no path was missed in the split.
- `sync.Mutex.TryLock()` (Go 1.18+) cannot itself block, so `waitBackoffWithFastRecheck`'s per-attempt cost is bounded by `fastRecheckSyncTimeout` (when the lock is won) or ~0 (when skipped) plus the `fastRecheckInterval` pacing wait — `2 × (150ms + 200ms) = 700ms` total, matching the `ponytail:` comment and AC5, **regardless of what the debounce callback or any other blocking caller is doing**. This is a genuine fix, not just a documentation change — it changes the concurrency primitive from an unbounded `Lock()` to a non-blocking `TryLock()`, which structurally cannot reproduce the priority-inversion the original design had.
- Bonus, not asked for but correct: `syncSessionsLocked`'s `fetchCtx` is now rooted on the passed-in `ctx` (`r.ctx`) instead of `context.Background()` (today's code hardcodes `context.Background()` at server_registry.go:211) — this means `Stop()`'s `r.cancel()` now actually cancels an in-flight fetch subprocess, an incidental improvement over today's behavior.

### Former Blocker 2 — regression test can't detect the bug above — downgraded, now an accepted/documented Concern
The redesigned `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff`
still elevates backoff by killing the keepalive session and keeping
control-mode down for the whole window — **unchanged from iteration 1**, it
still does not exercise the `%sessions-changed` debounce-callback contention
path, so the `syncMu` `TryLock`-vs-blocking-`Lock` race is not exercised by
any test in this plan's two-file confinement. Per requirements AC6 / plan's
Pattern Decisions table, a third file (`server_registry_test.go`, the only
place with unexported access to `syncMu`) is out of scope, so this really
cannot be closed within this fix's boundaries.

This is downgraded from Blocker to Concern (not silently closed) because:
1. The underlying bug is now fixed **by construction** — `TryLock()`'s
   non-blocking guarantee is verifiable by code reading and Go's documented
   `sync.Mutex` semantics, not something that needs an empirical, timing-based
   test to prove. Iteration 1's Blocker existed because the *code* had the bug
   and the test gave false confidence; now the code doesn't have the bug, so
   the missing coverage is a coverage gap, not a live correctness risk.
2. The plan meets iteration 1's own stated minimum remediation: "at minimum,
   note this gap explicitly in the test's doc comment... rather than assuming
   20/20 green covers the whole fix." Both the test's doc comment (plan.md
   lines ~441-451) and the plan's Unresolved Questions section state the gap
   explicitly, name why it can't be closed in-scope, and don't overclaim
   coverage.

**Recommendation** (non-blocking): if a follow-up ever touches
`server_registry_test.go` (in-package, unexported access), add the
contention-path test then. Not required for this fix to ship.

## Concerns

- [ ] **Residual: `syncMu` `TryLock`-contention path remains untested within this fix's file confinement.** See "Former Blocker 2" above — carried forward as a documented, accepted gap rather than a blocker. No action required beyond what the plan already does (explicit doc-comment + Unresolved Questions note).

## Nitpicks

- **ADR-003 (`time.Sleep`) conflict — resolved.** The regression test's setup phase now uses `waitForReconnectCycles`, a condition-driven poll (counts `IsHealthy()` false→true→false pulses) instead of `time.Sleep(2 * time.Second)`. No static sleep remains in the new test code. This mirrors the exact busy-poll-with-`runtime.Gosched()` idiom already used by `TestTmuxServerRegistry_StartsHealthy` in the same file (`server_registry_integration_test.go:122-133`) for the identical "catch a brief healthy window" problem, so it's consistent with established precedent, not a novel pattern.
- **`syncMu`-held-across-subprocess-call / `Stop()` latency — addressed via documentation, and the fast-recheck path specifically avoids adding to it.** The plan's Unresolved Questions section correctly frames this as an inherited (not introduced) tradeoff, unchanged from today's single-caller `syncSessions()`'s relationship to `Stop()`/`r.ctx` cancellation, and notes the `TryLock`-based fast-recheck path never contributes to it since it never blocks on `syncMu`. Combined with the `fetchCtx` now being rooted on `r.ctx` (see Former Blocker 1's "Bonus" note above), `Stop()`'s effective bound is, if anything, slightly improved rather than regressed. No further action needed for this fix's scope.
- **`waitForReconnectCycles`'s pulse-counting has an edge case, but it's self-correcting within budget.** If the helper's very first `IsHealthy()` sample happens to land mid-pulse (already `true` without having observed the preceding rise), that specific pulse goes uncounted by design (documented behavior, same as the "already-true-at-call-time" case) — worst case this costs one extra reconnect cycle before `minElevatedBackoffCycles` (4) is reached. Traced the cumulative timing: normal case needs ~1.5s (100+200+400+800ms) of elapsed backoff-wait time to observe 4 pulses; the degraded case needs ~3.1s (100+200+400+800+1600ms) — both comfortably inside the 5s `backoffElevationPollTimeout`, so this edge case cannot turn into a false test failure, only a (harmless, still-passing) slightly higher final backoff value than the nominal 1600ms. No action needed.
- Lock ordering (`syncMu` before `r.mu`, `subsMu` never nested with `syncMu`) is unchanged by the 3-way split — `syncSessionsLocked` is a straight extraction of the pre-existing body, and `firePaneExit`'s subsMu acquisition still happens after `r.mu` is released, same as today. No new deadlock risk introduced by the refactor.
