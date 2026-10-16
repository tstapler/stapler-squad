# BUG-093: Headless Pool Queue-Wait Counts Against the Caller's Own Call Budget, Misreporting Pool Contention as a Genuine LLM Timeout [SEVERITY: High]

**Status**: ✅ FIXED (2026-09-08)
**Discovered**: 2026-09-08, `backlog-feature-improvement` audit (`docs/tasks/backlog-feature-improvement.md`, "Update — 2026-09-08" section), prompted by a direct user question: "why are some backlog items churning and never completing triage?"
**Fixed**: 2026-09-08 — `session/headless/caller.go`, `session/headless/pool.go`, `session/headless/runner.go`, `server/services/backlog_service_triage.go`.
**Impact**: `session/headless/Pool.call` — the single internal implementation shared by every `Pool.Call`/`Pool.CallBlocking` invocation across the codebase (triage, review, one-shot PR creation, autonomous-driver calls, approval-handler LLM classification, and every `session/headless/features.go` helper).

## Problem Description

Live state at discovery: `ListStuckBacklogItems` returned 21 stuck items, 20 of them
`STUCK_REASON_ORPHANED_TRIAGE`, all parked at the 5-attempt remediation cap
(`MaxRemediationAttempts`, `session/backlog_remediation.go`). 17 of the 20 ended with
`EndReason=timeout`. `firstDetectedAt` timestamps clustered into three tight bursts (7-8 items
created within 100ms-2min of each other, 2026-08-30 and 2026-09-04) rather than a steady trickle
— consistent with a bulk import, not organic one-at-a-time creation.

**Root cause**: every backlog item created with a `repo_path` fires `TriggerTriage`
(`server/services/backlog_service_triage.go:2564`) immediately, which opens a 30-minute call
budget — `triageCtx, cancel := context.WithTimeout(s.shutdownCtx, triageCallBudget)` — **before**
calling `s.headlessPool.CallBlocking(triageCtx, ...)`. That pool is a single **global**
concurrency semaphore, `MaxConcurrentSessions: 5` (`server/dependencies.go:642`), shared by
triage, review, one-shot PR creation, and autonomous approval classification. Inside the pool,
acquiring a slot was itself bound by the caller's own context:

```go
// session/headless/caller.go, before this fix
select {
case p.concurrencySem <- struct{}{}:
case <-ctx.Done():
    p.decrementCallCount(key)
    close(ch)
    return ch, ctx.Err()
}
```

A call that lost the race for one of 5 pool slots didn't wait indefinitely — it waited against
its own 30-minute budget. If it was still queued when that budget expired, `ctx.Err()` was
`context.DeadlineExceeded`, which `classifyHeadlessCallError` bucketed as `"timeout"` —
**structurally indistinguishable from a genuine 30-minute-long hung LLM call.** A burst of 7-8
items created within milliseconds of each other trivially exceeds the pool's 5 slots (the
8-wide `s.triageSem`, `server/services/backlog_service.go:419`, doesn't prevent this — it's sized
larger than the pool it feeds into), so several of each burst's triage calls very likely never
reached the LLM at all, just starved out waiting for a slot already occupied by their siblings
from the same burst.

**Why retries didn't help**: items created in one burst share nearly the same original
timestamp, so they also share nearly the same backoff schedule (`remediationBackoffSchedule`,
30m/2h/8h/24h/72h) — every retry attempt for the whole batch recreated the same 5-slot
contention. All 20 items exhausted their full 5-attempt budget and parked, confirmed live via
`staplersquad.log`: every one of the 20 items sitting in "orphaned_triage remediation backoff not
yet due, skipping retry" on today's sweep — genuinely parked, not silently dropped; the
remediation machinery itself (BUG-083's cold-retry fix) was working exactly as designed on top
of a false signal.

## Fix Applied

`session/headless/pool.go`: new `maxQueueWait` var (2 minutes, a `var` not a `const` so tests can
shrink it rather than waiting out the real window — mirrors `remediationBackoffSchedule`/
`MaxRemediationAttempts` in `session/backlog_remediation.go`).

`session/headless/runner.go`: new `ErrPoolSaturated` sentinel alongside the existing
`ErrClaudeNotFound`/`ErrSubprocessStart`.

`session/headless/caller.go`'s `call()`: the semaphore-acquire select is now bounded by a
`queueCtx` derived from the caller's `ctx` with `maxQueueWait` (`context.WithTimeout` takes the
earlier of the two deadlines, so a caller whose own budget is already shorter than
`maxQueueWait`, or whose ctx is genuinely cancelled, is unaffected). When `queueCtx.Done()` fires:
if the caller's own `ctx.Err()` is non-nil, that real signal (shutdown, or a short-lived caller)
is preserved unchanged; only when `ctx` itself has NOT yet expired does the call return
`ErrPoolSaturated` — the only way `queueCtx` could have fired in that case is its own added cap.

`server/services/backlog_service_triage.go`'s `classifyHeadlessCallError`: new `"pool_saturated"`
bucket, checked **before** the existing `"timeout"` case (including its `budget-elapsed <
5*time.Second` fallback heuristic, which would otherwise still misclassify a saturated-pool
error as `"timeout"` since the wrapped `ctx.Err()` genuinely is near-budget).

Deliberately **not** changed, per explicit scope from the routing conversation: `MaxConcurrentSessions`
(pool sizing is a separate resource/cost tradeoff), and the remediation-attempt penalty semantics
in `session/backlog_lifecycle_triage.go` (unlike the `"shutdown"` bucket's existing zero-penalty
immediate-respawn carve-out, `pool_saturated` is left going through the normal backoff path —
sustained saturation retried with no penalty could otherwise turn into an unthrottled retry storm;
this fix's main effect is that each attempt now fails in ~2 minutes instead of silently occupying
30 minutes while producing a misleading diagnosis).

## Regression Tests

`session/headless/pool_test.go`:
- `TestPool_QueueWaitTimeout_DuringSemaphoreWait_ReturnsErrPoolSaturated` — occupies the pool's one
  slot, calls with a long-lived (1 hour) ctx, shrinks `maxQueueWait` to 20ms, asserts the call
  returns `ErrPoolSaturated` (not `context.DeadlineExceeded`) and that `callCount` is decremented.
- `TestPool_CallerCtxShorterThanQueueWait_PreservesRealCtxError` — the inverse: `maxQueueWait` set
  to an hour, caller's own ctx times out in 20ms; asserts the real `context.DeadlineExceeded` is
  preserved and NOT relabeled as `ErrPoolSaturated`.

`server/services/backlog_service_triage_test.go` (`TestClassifyHeadlessCallError_...`): 3 new
table cases — bare `ErrPoolSaturated`, wrapped `ErrPoolSaturated`, and `ErrPoolSaturated` with
`elapsed` deliberately within 1 second of `triageCallBudget` (proves the new case is checked
before the existing near-budget "timeout" fallback heuristic, which would otherwise still catch
it).

## Verification

```
$ make build
✅ Web UI built and copied successfully
✅ stapler-squad built successfully

$ go vet ./session/... ./server/...
(clean)

$ go test ./session/headless/... -v
--- PASS: TestPool_QueueWaitTimeout_DuringSemaphoreWait_ReturnsErrPoolSaturated
--- PASS: TestPool_CallerCtxShorterThanQueueWait_PreservesRealCtxError
--- PASS: TestPool_CtxCancel_DuringSemaphoreWait_DecrementsCallCount
--- PASS: TestPool_ConcurrencySemaphore_LimitsToMax
PASS
ok  	github.com/tstapler/stapler-squad/session/headless	0.348s

$ go test ./server/services/... -run 'TestTriggerTriage|TestClassifyHeadlessCallError|TestApplyTriageResultToUpdate|TestReconcileOrphanedTriage|TestMaybeTriggerTriage|TestAutoRespawnTriage' -v
PASS (all triage-path tests, including all TestClassifyHeadlessCallError subtests)
ok  	github.com/tstapler/stapler-squad/server/services	3.273s

$ go test ./session -timeout=150s
ok  	github.com/tstapler/stapler-squad/session	71.668s

$ golangci-lint run ./session/headless/... ./server/services/... --new-from-rev=origin/main
0 issues.
```

**Not run**: the full `go test ./server/services/...` suite — it hit a pre-existing, unrelated
20-minute hang in a tymux/session-creation goroutine
(`tymuxGRPCSession.RestoreWithWorkDir`/`cacheFromSession`, `session/tymux/session.go:270,351`)
during an unrelated background-shell CI verification pass this session, matching the known
`server.BuildDependencies()`-makes-real-network-calls flakiness already tracked in this repo's
memory (BUG-087..090). The full triage/classifier-scoped subset above, plus the full `./session`
and `./session/headless` packages, cover every file this fix touches.

## Related

- BUG-083 (`docs/bugs/fixed/BUG-083-parked-remediation-rows-never-automatically-retry.md`) — the
  cold-retry heartbeat that correctly kept retrying these items once parked; that machinery
  worked exactly as designed here, on top of a false `timeout` signal this fix corrects at the
  source.
- 2026-08-21 audit entry (`docs/tasks/backlog-feature-improvement.md`) — a *different* prior
  incident that also parked 20 `orphaned_triage` items via a bulk import, root-caused to
  `classifyHeadlessCallError` misclassifying subprocess-start errors as `"other"` (fixed by PR
  #535). This bug is a distinct mechanism (queue-wait counted against budget, not misclassified
  process-exit) that produces the same downstream symptom.
- `docs/tasks/backlog-feature-improvement.md`'s "Update — 2026-09-08" entry — the audit finding
  this bug was filed from.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap. `Pool.call`'s semaphore-acquire implicitly assumed the
caller's `ctx` deadline existed only to bound the LLM subprocess call itself; every caller (11
call sites across triage, review, PR creation, approval classification, and every
`session/headless/features.go` helper) that builds a long-lived fixed-budget context and passes
it straight into `CallBlocking` was silently exposed to the same "queue wait counts against my
budget" gap, not just `TriggerTriage`.

**Earliest achievable enforcement**: the regression tests are close to the earliest achievable
level — this is a runtime timing/concurrency behavior (a semaphore race against two different
deadlines), not something a compile-time type or lint rule can express. Fixed at the single
shared implementation (`Pool.call`) rather than the one call site (`TriggerTriage`) the live
incident happened to surface it through, so all 11 current call sites — and any future one — get
the fix for free without needing their own awareness of the gap.

**Recurring-shape check**: adjacent to, but distinct from, this doc's most-tracked shape ("fix
closes the write side of a gap but not the recovery side," 6 prior instances per
`docs/tasks/backlog-feature-improvement.md`) — this is instead a **fixed-budget context racing an
unrelated resource-acquisition step it wasn't scoped to bound**, which then gets misclassified
identically to the failure the budget was originally meant to catch. Worth watching for at other
`context.WithTimeout(...)`-then-shared-resource-acquire call sites in this codebase (e.g. any
future addition to the same 11-call-site headless pool, or an analogous pattern against a
different shared semaphore) — the general form is: a context's deadline is meant to bound step N,
but a caller unconditionally threads it through steps 1..N-1 too, and whichever step actually
consumes the budget gets misdiagnosed as the point of failure.
