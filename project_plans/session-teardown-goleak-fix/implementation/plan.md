# Implementation Plan: session-teardown-goleak-fix

**Feature**: Join the two untracked `ClaudeController`/`PTYConsumer` goroutines so `DeleteSession` teardown is fully synchronous, eliminating the `TestServer_Shutdown_JoinsBackgroundTickers` goleak flake caused by the preceding test's leftover goroutines.
**Date**: 2026-09-07
**Status**: Ready for implementation
**ADRs**: None — this is a plain `sync.WaitGroup` addition matching an idiom already used three times in the same two files (`CommandExecutor.wg`, `ResponseStream.wg`, `Instance.driverWG`/`hibernateWG`); no novel technology or architectural choice is being made.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `ClaudeController` | Per-session orchestrator owning the PTY, response stream, command executor, and status-change loop for one Claude Code session. | `session/claude_controller.go` |
| `runStatusChangeLoop` | Goroutine that watches `statusCheckCh` and fans out status transitions to registered listeners; currently launched with a bare `go` and never joined. | `session/claude_controller.go:365`, loop body at `:1178` |
| `PTYConsumer` | Component that polls a session's PTY output buffer for rate-limit detection. | `session/detection/ratelimit/integration.go` |
| `pollLoop` | `PTYConsumer`'s background polling goroutine, launched from `Start()`; currently untracked by any WaitGroup. | `session/detection/ratelimit/integration.go:143` |
| `cc.wg` (new) | A `sync.WaitGroup` field added to `ClaudeController`, `Add(1)`'d immediately before `go cc.runStatusChangeLoop(innerCtx)` and `Done()`'d by that goroutine on exit; waited on synchronously in `Stop()`. | New field, mirrors `ResponseStream.wg` / `CommandExecutor.wg`. |
| `pc.wg` (new) | The same pattern applied to `PTYConsumer`: `Add(1)` immediately before `go pc.pollLoop(ctx)` in `Start()`, `Done()` on `pollLoop` exit, `Wait()` added to `Stop()`. | New field. |
| `waitForTmuxTeardown` | Existing test helper that polls `inst.TmuxSessionExists()` after `DeleteSession`; becomes a sufficient goroutine-teardown signal once `cc.wg`/`pc.wg` land, because `StopController()` runs synchronously before `KillSession()` in `Instance.Destroy()`'s single teardown goroutine. | `server/server_integration_test.go:628` |
| `TestSessionService_CreateThenImmediateDelete_NoDataRace` | The test whose incomplete teardown leaks goroutines into the next test in the binary; requires no source change under this plan — it becomes correct once the two component fixes land. | `server/server_integration_test.go:473` |
| `TestServer_Shutdown_JoinsBackgroundTickers` | The goleak assertion that intermittently fails; its own behavior is out of scope — the fix is entirely upstream of it. | `server/server_test.go:310`, assertion at `:257` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `ClaudeController` goroutine join | `sync.WaitGroup` field (`cc.wg`), `Add(1)` before `go`, `Done()` in loop, `Wait()` in `Stop()` | Existing `ResponseStream.wg` (`session/response_stream.go:41,403`) and `CommandExecutor.wg` (`session/command_executor.go:89,172`) in the *same package* | (A) `Stopped() <-chan struct{}` channel signal (the pattern named in requirements.md's "Existing Prior Art") | Channel-close is the right shape when a caller only needs to *observe* completion asynchronously (the three cited precedents — watchers with independent consumers). Here `Stop()` itself needs to *block* until exit, which is exactly what `ResponseStream`/`CommandExecutor` — the two siblings in the identical file/call chain — already do with `wg.Wait()`. Matching the sibling idiom in the same `Stop()` method is more locally consistent than introducing a second signaling shape into the same struct. |
| `ClaudeController` goroutine join | (chosen above) | — | (B) Aggregate join at `Instance.Destroy()` or `DeleteSession` level (e.g. a new `Instance.Stopped()` channel) | research/architecture.md confirms `StopController()` already runs synchronously before `destroyChain()`'s `KillSession()` in the same goroutine — no new cross-layer signal is needed once the two leaf components join themselves; adding one would be an unjustified generic per `interface-pollution-checklist.md` (a wrapper with no added behavior over what program order already guarantees). |
| `PTYConsumer` goroutine join | `sync.WaitGroup` field (`pc.wg`), same shape as `ClaudeController` | Same as above, applied to `session/detection/ratelimit/integration.go` | (C) Reuse `notifyCh` or `running` bool as a completion signal | Both already exist for a different purpose (avoiding poll delay, guarding double-start) and are not synchronized for "has fully exited" semantics; overloading them would conflate two concerns rather than adding the one missing primitive. |
| Timeout-on-expiry behavior (if a bounded wait is added anywhere in the join path) | **RESOLVED (post-pre-mortem, supersedes the row below's original "unbounded" reading)**: `cc.wg.Wait()`/`pc.wg.Wait()` are wrapped in `syncutil.WaitWithTimeout(&wg, stopJoinTimeout)` (the existing 10s constant, `session/pty_discovery.go:28`), with `log.Error` (not `log.Warn`) on expiry naming the specific goroutine that failed to join. | pre-mortem.md failure #1 (P1): the original "leave unbounded" decision was a stretch reading of requirements.md AC5 ("any new wait has a bounded timeout with a clear test failure message on expiry") — AC5's plain text does not carve out an exception for production-path waits, and an unbounded wait under `i.mu` (held across `ClaudeController.Stop()`) turns any future `i.mu` re-entrancy regression (Task 1.1.1b) into a silent full hang with no goleak signal at all. | Originally: leave unbounded, matching `ResponseStream.Stop()`/`CommandExecutor.Stop()`'s existing unbounded `wg.Wait()` calls. | Matching the sibling calls exactly would have been the more "minimal seam" choice, but it fails AC5 on a plain reading and converts a detectable CI flake into an undetectable production deadlock if the re-entrancy risk ever materializes — not an acceptable trade for two new call sites that already have a ready-made bounded-wait helper (`syncutil.WaitWithTimeout`) and precedent constant (`stopJoinTimeout`) to reuse. `log.Error` (louder than `JoinSessionDriver`'s existing `log.Warn`) is used specifically because unlike that helper's callers, a timeout here means the fix's own core guarantee failed. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `ClaudeController`/`PTYConsumer` shutdown join gap | Two of six goroutines named in requirements.md were never joined by a `WaitGroup`, while three siblings in the same files (`CommandExecutor`, `ResponseStream`, and `ResponseStream`'s own tracking of the `MangleCorrelator` eviction goroutine) already do it correctly | **Extend as-is** | This does not deepen or work around an existing violation — it closes a gap in an otherwise-correct, already-idiomatic pattern that the rest of `ClaudeController.Stop()` already follows (per research/architecture.md's "minimal seam, not a refactor" disposition). No restructuring of `Instance.Destroy()`, `DeleteSession`, or the controller ownership model is warranted or in scope. |

---

## Migration Plan
N/A — no schema or data changes.

## Observability Plan
None beyond the existing test failure output. No new logging is required in the production path (acceptance criterion 5 rules out behavior changes); `ClaudeController.Stop()`'s existing `log.Info("claude controller stopped", ...)` call is unaffected. If implementation reveals the `i.mu`-held-during-`wg.Wait()` risk flagged in research/architecture.md's "RISK TO VERIFY" note actually manifests as slow joins in practice, add a one-line `log.Debug` around the new `Wait()` calls — not required by any acceptance criterion, so only add it if profiling during implementation shows a need.

## Risk Control
Not gated behind a feature flag — this is test-infrastructure/internal-concurrency-correctness, not a user-facing surface. Standard revert path: revert via PR close or a follow-up revert commit if the fix introduces a regression (e.g. the deadlock risk named in Task 1.1.1a's acceptance detail). No staged rollout applicable.

## Unresolved Questions
- [ ] Does any registered `StatusChangeListener` re-enter an `Instance` method that also takes `i.mu`, which combined with `StopController()` holding `i.mu` across `cc.wg.Wait()` could turn the new join into a deadlock? — blocks Task 1.1.1a — owner: implementer, resolve via the grep specified in that task before merging (research/architecture.md's "RISK TO VERIFY" note; not blocking on this triage/planning session since the existing `exec`/`rs` joins already run under the same lock today without incident, so this is a verification step, not a redesign trigger). Now that `cc.wg.Wait()` is bounded (`syncutil.WaitWithTimeout`, see Task 1.1.1a), a re-entrancy hit degrades to a `stopJoinTimeout`-bounded, loudly-logged delay rather than a silent permanent hang — lowers but does not eliminate the need to actually run the grep.
- [ ] Does `TestSessionService_CreateThenImmediateDelete_NoDataRace` reproduce the leak under a 100-iteration stress run even with Stories 1.1.1/1.1.2 applied, due to the `StartController`/`StopController` registration TOCTOU pre-mortem.md flags as failure #2 (P1)? — blocks Story 1.1.3's completion — owner: implementer, resolve via Task 1.1.3c. If it reproduces, this item cannot close on Stories 1.1.1/1.1.2 alone; a separate bug must be filed for the registration gap per `.claude/rules/fix-flaky-tests-dont-defer.md`.
- [ ] Does a residual `-race -count=10` failure (if any) in Task 1.1.3a trace to the 6th, out-of-scope goroutine (`RestoreWithWorkDir`'s `os/exec.(*Cmd).Wait`, pre-mortem.md failure #3, P1)? — owner: implementer, resolve via the BUG-084 cross-check added to Task 1.1.3a; if it matches, this plan's scope must extend to it before the item closes.

Added post-pre-mortem (2026-09-14): the three P1 items in `implementation/pre-mortem.md` are addressed above — #1 (unbounded-wait/AC5 conflict) by the Pattern Decisions and Task 1.1.1a/1.1.2a updates, #2 (registration TOCTOU) by new Task 1.1.3c, #3 (6th goroutine misattribution risk) by the Task 1.1.3a update. None required a change to this plan's core Epic 1.1 fix (the two-goroutine join is still correct and necessary); they add verification scope and a safety margin, not a redesign.

## Dependency Visualization

```
Phase 1
  Epic 1.1: Join the two untracked goroutines
    Story 1.1.1 (ClaudeController.runStatusChangeLoop)
      Task 1.1.1a: add cc.wg field + Add/Done/Wait wiring ─┐
      Task 1.1.1b: verify no i.mu re-entrancy risk ────────┤
    Story 1.1.2 (PTYConsumer.pollLoop)                      ├──▶ Story 1.1.3
      Task 1.1.2a: add pc.wg field + Add/Done/Wait wiring ─┘     (verification run)
                                                                  Task 1.1.3a: race+count=10 run (+ BUG-084 stack cross-check)
                                                                  Task 1.1.3b: goroutine-swap ordering audit
                                                                  Task 1.1.3c: TOCTOU stress test, count=100 (P1, blocks close)
                                                                        │
                                                                        ▼
Phase 2 (implementer-only, sequenced last)
  Epic 2.1: Close superseded bug docs
    Story 2.1.1
      Task 2.1.1a: move 3 docs to docs/bugs/fixed/, cross-reference this item
```

---

## Phase 1: Fix the goroutine-join gap

### Epic 1.1: Join `ClaudeController.runStatusChangeLoop` and `PTYConsumer.pollLoop`
**Goal**: Make `ClaudeController.Stop()` and `PTYConsumer.Stop()` block until their respective background goroutines have actually exited, so `waitForTmuxTeardown` (already correct in program order per research/architecture.md) stops observing goroutines that outlive it.

#### Story 1.1.1: `ClaudeController` joins its status-change loop
**As a** test relying on `Instance.Destroy()`/`DeleteSession` teardown, **I want** `ClaudeController.Stop()` to block until `runStatusChangeLoop` has exited, **so that** no `TestServer_Shutdown_JoinsBackgroundTickers` goleak check downstream in the same binary observes a leftover status-change goroutine from a prior test's session.

**Acceptance Criteria**:
- `ClaudeController.Stop()` does not return until `runStatusChangeLoop`'s goroutine has fully exited.
  - *Given* a started `ClaudeController` with a live `runStatusChangeLoop` goroutine, *When* `Stop()` is called, *Then* `Stop()` does not return until that goroutine has observed `ctx.Done()` and returned (verified by `cc.wg.Wait()` completing before `Stop()`'s return statement).
- `Add(1)` for the new WaitGroup always happens-before any `Wait()` call can observe it, even under `TestSessionService_CreateThenImmediateDelete_NoDataRace`'s fast create-then-delete pattern.
  - *Given* `ClaudeController.Start()` is called, *When* it reaches the `go cc.runStatusChangeLoop(innerCtx)` launch site, *Then* `cc.wg.Add(1)` has already executed synchronously on the calling goroutine, in the same critical section as the other sub-component `Add(1)` calls, strictly before the `go` statement — never inside `runStatusChangeLoop` itself.
- No behavior change to `TestServer_Shutdown_JoinsBackgroundTickers` itself.
  - *Given* the test file `server/server_test.go` unmodified, *When* this story's changes land, *Then* `git diff` shows zero changes to `server/server_test.go`.

**Files**: `session/claude_controller.go`

##### Task 1.1.1a: Add `cc.wg sync.WaitGroup`, wire `Add`/`Done`/`Wait` (~5 min)
- In the `ClaudeController` struct (`session/claude_controller.go:100-140`), add a new field `wg sync.WaitGroup` near the other lifecycle-related fields (next to `lifecycle Locked[controllerLifecycle]` at line 110 — not inside the atomic-pointer block at 118-125, since this isn't a swappable sub-component).
- At `session/claude_controller.go:365`, immediately **before** the existing line `go cc.runStatusChangeLoop(innerCtx)`, insert `cc.wg.Add(1)`. **This ordering is load-bearing**: `Add(1)` must execute synchronously on the `Start()` caller's goroutine, in the same place `CommandExecutor`/`ResponseStream` already do their own `Add(1)` immediately before their own `go` statements (per pitfalls.md §3) — never move it inside `runStatusChangeLoop` itself, or a fast-following `Stop()`/`Destroy()` (as in `TestSessionService_CreateThenImmediateDelete_NoDataRace`) can call `cc.wg.Wait()` while the counter is still 0, silently masking the exact leak this fix exists to catch.
- At the top of `runStatusChangeLoop` (`session/claude_controller.go:1178`), add `defer cc.wg.Done()` as the very first statement inside the function body.
- In `Stop()` (`session/claude_controller.go:397`), after `cancelFn()` (line 409) and before Phase 2's sub-component swaps (line 411), add `internal/syncutil.WaitWithTimeout(&cc.wg, stopJoinTimeout)` and `log.Error` if it returns false (timed out), naming `runStatusChangeLoop` in the message. **Superseded by pre-mortem.md failure #1 (P1)**: do *not* leave this unbounded — the Pattern Decisions table's "Timeout-on-expiry" row was updated after pre-mortem review to require this, reconciling requirements.md AC5's bounded-timeout requirement. This does diverge from `ResponseStream.Stop()`/`CommandExecutor.Stop()`'s existing unbounded calls; that divergence is intentional and is not extended to those two existing sibling calls (out of scope — they're pre-existing, not part of this fix).
- Files: `session/claude_controller.go`

##### Task 1.1.1b: Verify no `i.mu` re-entrancy deadlock risk (~5 min)
- Run `grep -rn "AddStatusChangeListener\|SetStatusChangeListener" session/ server/` — confirmed exact names via `session/claude_controller.go:157` (`AddStatusChangeListener`, the preferred fan-out registration) and `:165` (`SetStatusChangeListener`, legacy single-listener form, kept for backward compatibility) — to enumerate every registered listener callback.
- For each listener found, confirm it does not call back into any `Instance` method that takes `i.mu` (the same lock `StopController()` holds across `ClaudeController.Stop()`, per research/architecture.md's "RISK TO VERIFY" note). `runStatusChangeLoop` already copies the listener slice before invoking callbacks (existing pattern, not new), so this check is about what the *listener bodies* do, not the loop itself.
- If a re-entrant call is found: do not restructure `Stop()`'s locking — instead confirm the specific callback already runs outside `i.mu` via the existing copy-then-call pattern, or flag it as a new finding requiring its own fix (out of scope for this plan; file as its own bug per `.claude/rules/fix-flaky-tests-dont-defer.md` if found and not fixable in this task's 5-minute budget).
- If no re-entrant call is found (expected outcome, since the existing `exec.Stop()`/`rs.Stop()` joins already run under this same lock today with no reported deadlock): record that finding in the task's completion note; no code change needed for this task.
- Files: none (verification-only; may produce a follow-up bug filing, not a code change, if the risk materializes)

#### Story 1.1.2: `PTYConsumer` joins its poll loop
**As a** test relying on `ClaudeController.Stop()` → `rlh.Stop()` teardown, **I want** `PTYConsumer.Stop()` to block until `pollLoop` has exited, **so that** the rate-limit poller from a prior test's session is never still running when a later test's goleak check runs.

**Acceptance Criteria**:
- `PTYConsumer.Stop()` does not return until `pollLoop`'s goroutine has fully exited.
  - *Given* a started `PTYConsumer` with a live `pollLoop` goroutine, *When* `Stop()` is called, *Then* `Stop()` does not return until `pollLoop` has observed `ctx.Done()` (via `cancelFn()`) and returned.
- `Add(1)` happens synchronously in `Start()`, strictly before the `go pollLoop(...)` statement.
  - *Given* `PTYConsumer.Start()` is called, *When* it reaches `go pc.pollLoop(ctx)` (`session/detection/ratelimit/integration.go:125`), *Then* `pc.wg.Add(1)` has already executed on the calling goroutine, under the same `pc.mu` lock already held for the `running`/`cancelFn` writes in `Start()` — never inside `pollLoop` itself.
- `ClaudeController.Stop()`'s existing `rlh.Stop()` call (line 441) now transitively blocks on this join with no source change needed at that call site.
  - *Given* `ClaudeController.Stop()`'s Phase 3 cleanup reaches `rlh.Stop()`, *When* `PTYConsumer.Stop()` returns, *Then* `pollLoop` has already exited — no new call site is added to `claude_controller.go` for this story.

**Files**: `session/detection/ratelimit/integration.go`

##### Task 1.1.2a: Add `pc.wg sync.WaitGroup`, wire `Add`/`Done`/`Wait` (~5 min)
- In the `PTYConsumer` struct (`session/detection/ratelimit/integration.go:85-93`), add a new field `wg sync.WaitGroup` next to the existing `mu sync.Mutex` field.
- In `Start()` (`session/detection/ratelimit/integration.go:113-125`), immediately **before** `go pc.pollLoop(ctx)` (line 125), insert `pc.wg.Add(1)`. This must stay inside the existing `pc.mu.Lock()`/`defer pc.mu.Unlock()` critical section that already guards `pc.running = true` — same ordering requirement as Task 1.1.1a: `Add(1)` before the `go` statement, never inside `pollLoop`.
- At the top of `pollLoop` (`session/detection/ratelimit/integration.go:143`), add `defer pc.wg.Done()` as the first statement.
- In `Stop()` (`session/detection/ratelimit/integration.go:128-141`), restructure away from the existing `defer pc.mu.Unlock()` idiom: call `pc.mu.Unlock()` **explicitly** (not via defer) once the locked mutations (`pc.cancelFn()`/`pc.cancelFn = nil`/`pc.running = false`) are done, add a one-line comment stating why ("wg.Wait() must run after the lock is released, not deferred, or a future pollLoop change that takes pc.mu would deadlock"), then call `internal/syncutil.WaitWithTimeout(&pc.wg, stopJoinTimeout)` outside the lock, `log.Error` naming `pollLoop` if it times out. **Two things superseded by pre-mortem.md**: (a) failure #4 (P2) — a naive trailing `pc.wg.Wait()` after a bare `defer pc.mu.Unlock()` would actually execute *before* the deferred unlock fires (defers run at return), silently holding the lock during the wait; the explicit-unlock restructure closes that trap. (b) failure #1 (P1) — bounded wait via `syncutil.WaitWithTimeout`, matching Task 1.1.1a's updated rationale, not unbounded.
- Add a unit test asserting `pc.mu` is provably unlocked before `pc.wg.Wait()` is entered (e.g. a test `pollLoop` stand-in that itself attempts `pc.mu.TryLock()` during the wait and asserts success) — this directly covers pre-mortem.md failure #4's "no test currently exercising that interleaving" gap.
- Files: `session/detection/ratelimit/integration.go`

#### Story 1.1.3: Confirm the flake is closed
**As a** maintainer of this test suite, **I want** repeated evidence that `TestServer_Shutdown_JoinsBackgroundTickers` no longer flakes under full-suite load, **so that** acceptance criterion 2 is verifiably met before this item is closed.

**Acceptance Criteria**:
- `go test ./server/... -race -count=10` completes without reproducing the goleak failure at `server/server_test.go:257`.
  - *Given* Stories 1.1.1 and 1.1.2 are complete, *When* `go test ./server/... -race -count=10` is run, *Then* `TestServer_Shutdown_JoinsBackgroundTickers` passes in all 10 iterations (no `found unexpected goroutines` failure).
- `TestSessionService_CreateThenImmediateDelete_NoDataRace` itself continues to pass with no source changes.
  - *Given* the same test run, *When* `TestSessionService_CreateThenImmediateDelete_NoDataRace` executes, *Then* it passes, and `git diff` shows no changes to `server/server_integration_test.go`.

**Files**: none (verification task; no source changes expected beyond Stories 1.1.1/1.1.2)

##### Task 1.1.3a: Run the repro command and capture the result (~3 min)
- Run `go test ./server/... -race -count=10 -run 'TestServer_Shutdown_JoinsBackgroundTickers|TestSessionService_CreateThenImmediateDelete_NoDataRace|TestServer'` (or the full `./server/...` package per acceptance criterion 2 if time budget allows — the requirements doc's own wording is "or equivalent").
- Record pass/fail for all 10 iterations. If any iteration still reproduces the goleak failure, **do not assume it's a new, unrelated flake** — per pre-mortem.md failure #3 (P1), first check the failing goroutine's stack against BUG-084's 2026-08-21 recurrence log entry (`docs/bugs/open/BUG-084-server-shutdown-goleak-false-positive-cross-test-tmux-goroutines.md`). If it matches `session/tmux.(*TmuxSession)`'s `RestoreWithWorkDir` → `os/exec.(*Cmd).Wait` diagnostic goroutine (the 6th goroutine named in requirements.md, deliberately out of scope per architecture.md), this plan's scope was incomplete — do not close the item; extend `waitForTmuxTeardown` or `KillSession` to also confirm that goroutine has been reaped, or file it as an explicit, separately-scoped follow-up bug per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than re-excusing it as "unrelated."
- Files: none

##### Task 1.1.3b: Spot-check goroutine-swap ordering under `-race` (~3 min)
- Re-read `ClaudeController.Stop()`'s Phase 2 (`session/claude_controller.go:411-419`, the `Swap(nil)` calls) to confirm the new `cc.wg.Wait()` added in Task 1.1.1a sits in Phase 3 order relative to these swaps consistently with the existing `exec`/`rs` joins (i.e., no accidental reordering that would let a swapped-out component's goroutine still be mid-flight when `Wait()` returns).
- Files: none (read-only confirmation)

##### Task 1.1.3c: Stress-test the `StartController`/`StopController` registration TOCTOU (~10 min)
**Added post-pre-mortem — addresses pre-mortem.md failure #2 (P1).** This is upstream of, and not fixed by, Stories 1.1.1/1.1.2: `StartController()` (`session/instance_controller.go:19-159`) only registers the controller into `cm.controller` (`session/controller_manager.go:80-86`) *after* the slow `controller.Start()` call returns and *after* `i.mu` is released. A concurrent `StopController()` in that window sees `HasController() == false` and returns without ever calling `Stop()` — meaning `cc.wg`/`pc.wg`'s new join logic never runs, and the goroutine leaks regardless of Stories 1.1.1/1.1.2 being correct in isolation.
- Run a dedicated tight-loop stress test isolating this window, separate from the 10-iteration full-package run: `go test -race -count=100 -run TestSessionService_CreateThenImmediateDelete_NoDataRace ./server/...` (100 iterations, not 10, to surface a narrow TOCTOU window that a full-package `-count=10` run is unlikely to hit reliably).
- If it reproduces the leak (with or without Stories 1.1.1/1.1.2's fixes applied): this is a **separate, upstream bug** — the registration gap in `StartController`/`StopController`, not the two-goroutine join gap this plan targets. Do not fold a fix for it into this plan's Stories 1.1.1/1.1.2; file it as its own bug immediately per `.claude/rules/fix-flaky-tests-dont-defer.md` (candidate fix direction: register the controller into `cm.controller` under `i.mu`, before releasing it for the slow `Start()` call, or add a "starting" state `StopController` can observe and wait on).
- If it does not reproduce in 100 iterations: record that finding; Story 1.1.3 can proceed to close on the strength of the 10-iteration full-package run (Task 1.1.3a).
- This task blocks Story 1.1.3's completion (dependency diagram below) — acceptance criterion 2 ("no longer reproduces... under an equivalent repeated full-package run") is not credibly met if this narrower, more targeted repro is skipped, since pre-mortem.md identified a plausible mechanism by which the full-package run's 10 iterations could simply not hit the window.
- Files: none (verification-only; may produce a new bug filing, not a code change to this plan's scope, if the TOCTOU reproduces)

---

## Phase 2: Close superseded bug docs (implementer-only, sequenced last)

**Not part of this triage/planning session's own scope.** This phase is for whoever implements Phase 1 to execute once Story 1.1.3's verification run confirms the flake is closed — do not move the docs before the fix is verified.

### Epic 2.1: Retire the three superseded write-ups
**Goal**: Satisfy acceptance criterion 4 — close the three prior independent bug docs this item consolidates, per `.claude/rules/fix-flaky-tests-dont-defer.md`'s convention of moving fixed flakes to `docs/bugs/fixed/`.

#### Story 2.1.1: Move and cross-reference the superseded docs
**As a** future reader of `docs/bugs/open/`, **I want** the three superseded write-ups moved out once this item's fix is verified, **so that** the backlog doesn't retain three stale open reports of an already-fixed flake.

**Acceptance Criteria**:
- All three docs are moved from `docs/bugs/open/` to `docs/bugs/fixed/` and each references this item.
  - *Given* Story 1.1.3's verification run has passed, *When* the implementer moves the three files, *Then* `docs/bugs/open/BUG-083-server-shutdown-goleak-flake-under-full-suite.md`, `docs/bugs/open/BUG-083-testservershutdownjoinsbackgroundtickers-picks-up-goroutines-from-prior-test.md`, and `docs/bugs/open/BUG-084-server-shutdown-goleak-false-positive-cross-test-tmux-goroutines.md` no longer exist in `docs/bugs/open/`, and their `docs/bugs/fixed/` counterparts each contain a line referencing backlog item `1cea70ed-3127-48db-8e68-f02eac685510` (this project, `session-teardown-goleak-fix`).

**Files**: `docs/bugs/open/BUG-083-server-shutdown-goleak-flake-under-full-suite.md`, `docs/bugs/open/BUG-083-testservershutdownjoinsbackgroundtickers-picks-up-goroutines-from-prior-test.md`, `docs/bugs/open/BUG-084-server-shutdown-goleak-false-positive-cross-test-tmux-goroutines.md`, `docs/bugs/fixed/` (destination)

##### Task 2.1.1a: Move the three docs and add the cross-reference (~4 min)
- For each of the three files listed above: `git mv docs/bugs/open/<file>.md docs/bugs/fixed/<file>.md`.
- In each moved file, add (or update an existing "Status" line) with a reference such as: "Fixed by session-teardown-goleak-fix (backlog `1cea70ed-3127-48db-8e68-f02eac685510`); root cause and fix: `session/claude_controller.go`'s `runStatusChangeLoop` and `session/detection/ratelimit/integration.go`'s `pollLoop` were never joined by a `WaitGroup` before `DeleteSession` teardown returned."
- Do **not** edit the two unrelated BUG-083/BUG-084 docs that share the same numeric prefix but a different subject (`docs/bugs/open/BUG-083-server-services-flaky-under-full-suite-parallel-load.md`, `docs/bugs/open/BUG-084-commitimportexternalsession-eventbus-goleak-flake-under-full-suite.md`, `docs/bugs/open/BUG-084-forbidden-deps-test-flaky-under-full-suite-parallel-load.md`) — confirmed distinct issues, out of scope.
- Files: the three docs named above, moved into `docs/bugs/fixed/`
