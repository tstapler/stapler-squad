# Adversarial Review: tmux-paneexit-reconnect-flake

**Date**: 2026-08-06 (iteration 2)
**Verdict**: CONCERNS

## Prior blockers — resolution status

All 3 iteration-1 blockers are resolved by the redesign (traced against the current
`plan.md`, `requirements.md`, `research/pitfalls.md`, and ground-truth
`session/tmux/server_registry.go` / `server_registry_integration_test.go` — both still
pre-fix on disk, as expected; `git status` confirms only `project_plans/**` files changed
in this repair pass, no source touched).

1. **`syncMu` unbounded-block risk — RESOLVED.** `syncSessionsFastRecheck` uses
   `TryLock()`, not `Lock()`; a fast-recheck attempt that can't immediately acquire
   `syncMu` returns `nil` and does no work rather than blocking. `waitBackoffWithFastRecheck`'s
   own elapsed time is therefore bounded by `fastRecheckAttempts × (fastRecheckSyncTimeout +
   fastRecheckInterval)` regardless of what the 3 blocking callers (`Start`, `reconnectLoop`
   post-connect, debounce callback) are doing — a genuine mechanism change (`TryLock` vs
   `Lock`), not a documentation fix. Walking the actual overlap window: debounce only
   schedules while control-mode is connected (fires from `handleEvent`, itself only reachable
   from `readLines` while the process is up), and fast-recheck only runs during backoff-wait
   (disconnected state) — the two are largely disjoint, with the only real overlap being a
   debounce timer scheduled ~50ms before disconnect firing shortly after `reconnectLoop`
   has already entered backoff-wait. In that narrow window `TryLock` now correctly *skips*
   (no correctness loss — the in-flight debounce caller itself will observe and fire the
   disappearance), not blocks. This is a real fix, not a relocation of the problem.
2. **Goroutine-outlives-teardown via a queued `syncMu.Lock()` — RESOLVED** by the same
   `TryLock` change: a fast-recheck attempt can never be the thing blocked on `syncMu` when
   `Stop()`/`r.ctx` cancellation fires. Bonus (correctly noted by architecture-review too):
   `syncSessionsLocked`'s `fetchCtx` is now rooted on the passed-in `ctx` (`r.ctx`) instead
   of today's `context.Background()` — `Stop()` can now actually cancel an in-flight fetch
   subprocess for all 3 blocking callers too, an improvement beyond what was asked.
3. **Static-sleep regression test that didn't verify its own precondition — RESOLVED.**
   `waitForReconnectCycles` polls `IsHealthy()` for false→true→false pulses instead of
   sleeping a fixed 2s. Traced the math: `reconnectLoop`'s `healthy` toggling is unchanged by
   this fix (same code locations, same order relative to `readLines`), so the pulse-counting
   mechanism works identically on fixed and unfixed code — it doesn't presuppose the fix,
   which is exactly what's needed for it to be a valid precondition check. With the keepalive
   session killed once on an isolated (non-auto-recreating) socket, `backoff` deterministically
   doubles every cycle (100→200→400→800→1600ms) since no connection is ever stable for
   `minStableConnection`=5s; 4 counted pulses reliably corresponds to the *next* wait being
   1600ms, confirmed structurally rather than assumed from wall-clock time.

## Blockers

None.

## Concerns

- [ ] **The detection-assertion margin (1.5s deadline vs. the unfixed code's earliest possible
  detection) is thinner than the plan's own stated "real headroom" philosophy elsewhere, and
  is squeezable by CI jitter on the pre-kill setup path.** Traced precisely: on unfixed code,
  detection of the killed target session cannot happen before `backoff` (1600ms, verified
  above) elapses — the wait is a hard `time.After`/timer duration, not probabilistic — plus
  whatever subprocess overhead (`startControlMode` + `syncSessions`) the *next* cycle takes to
  actually run its `list-sessions` diff. The test's own `1500ms` deadline starts counting
  right after its `kill-session` `exec.Command` call returns, which itself happens shortly
  *after* the backoff-wait begins (test does `SubscribePaneExit` setup + fork/exec `kill-session`
  before starting the deadline timer). Net guaranteed margin = `1600 − 1500 − δ + μ` where `δ`
  is that pre-kill setup/exec delay and `μ` is the next-cycle subprocess overhead — a
  **structural floor of only ~100ms** before accounting for `μ` (which usually helps) and `δ`
  (which always hurts). Under heavy CI/fork contention (the same class of slowdown
  `research/pitfalls.md` §3.1 and this file's own `registryPollTimeout` comment already
  document for *other* tests in this package), `δ` alone spiking past ~100ms would let unfixed
  code coincidentally "pass" the test — a false negative on the regression guard's discriminating
  power (this does **not** threaten CI stability of the shipped fixed code, which clears the
  deadline with a ~2x margin regardless of `backoff`; it only weakens confidence that the test
  would actually catch a future regression back to the old behavior). Contrast with this same
  plan's `PaneExitChannel` 3s-vs-sub-second and fixed-code 700ms-vs-1500ms margins, both far
  more generous. — **Recommendation**: bump `minElevatedBackoffCycles` to 5 (backoff → 3200ms)
  or otherwise widen the gap between assumed-elevated-backoff and the assertion deadline, so
  the "would fail on `main`" claim has real headroom instead of a ~100ms structural floor.

- [ ] **The Unresolved Questions' justification for skipping a lost-update-race test doesn't
  consider the one option that actually fits inside the two-file confinement.** The text frames
  the choice as needing either unexported `syncMu` access or controllable subprocess latency —
  "only reachable from `server_registry_test.go`, a third file" — and stops there. But a
  black-box, *probabilistic* concurrency-stress test (e.g., rapid session create/kill churn
  racing an intermittent keepalive kill/restore, then polling `SessionExists`/`ListSessions` in
  a tight loop asserting a once-killed session never transiently reappears) needs no unexported
  access at all and would live entirely in `server_registry_integration_test.go` — the *second*
  of the two already-allowed files, not a third one. It wouldn't deterministically prove the fix
  (can't force the exact interleaving `research/pitfalls.md` §1 describes), but it's a legitimate,
  in-scope regression guard that the accepted-gap reasoning doesn't rule out or even mention.
  — **Recommendation**: either add such a stress test, or replace the current "only reachable
  from a third file" framing with an accurate one (e.g., "a probabilistic black-box stress test
  is possible in-scope but was judged not worth the added CI runtime/flake surface for an
  already-fixed-by-construction bug") so the documented gap reflects what was actually
  considered.

- [ ] **Minor mischaracterization: the plan's "not new exposure" framing for `syncMu`
  contention among the 3 pre-existing blocking callers undersells a real (if narrow) new
  latency-compounding path.** Unresolved Questions states holding `syncMu` across the whole
  subprocess call "is the same shape today's single-caller `syncSessions()` already has
  relative to `Stop()`" — but pre-fix there is no mutex at all, so no blocking caller could
  ever queue behind another (each ran its own independent, fully-parallel subprocess call).
  Post-fix, `syncMu` means e.g. the debounce callback and `reconnectLoop`'s post-connect sync
  *can* now queue behind each other (bounded by `defaultSyncTimeout`=10s each, low-probability
  given they mostly fire at different loop phases, but structurally new). This doesn't affect
  `Stop()` itself (which never acquires `syncMu`) and is correctly bounded/low-impact — not a
  blocker — but "not new exposure" overstates it; "new but narrow and self-bounded" would be
  accurate.

## Minors

- The `fetchCtx := context.WithTimeout(ctx, timeout)` change (rooting the subprocess timeout on
  `r.ctx` instead of today's `context.Background()`) is a real, unasked-for improvement to
  `Stop()`'s ability to cut short an in-flight `list-sessions` call for all 3 blocking callers,
  not just the new fast-recheck path — worth keeping in the PR description as a incidental fix,
  not just an implementation detail.
- `waitForReconnectCycles`'s pure `runtime.Gosched()` busy-poll (no sleep) could in principle
  miss a very brief pulse under scheduler pressure, costing one extra reconnect cycle before
  `minElevatedBackoffCycles` is reached — self-correcting within the 5s `backoffElevationPollTimeout`
  (worst-case ~3.1s of real elapsed backoff-wait time even with one missed cycle) and consistent
  with the same busy-poll idiom `TestTmuxServerRegistry_StartsHealthy` already uses in this file.
  Not a new risk, just inherited tolerance — no action needed.
