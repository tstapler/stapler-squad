# BUG-065: Queued/Orphaned Triage Sessions Killed By Our Own Restart Are Misattributed As Genuine Failures [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-08-06, live in this repo's own deployed instance. A ~1-minute bulk import of 33 GitHub issues (each auto-triggering triage per BUG-061) plus 5 manually requeued items produced up to 38 near-simultaneous `TriggerTriage` calls against an 8-slot concurrency semaphore, then a routine service restart landed mid-batch. Afterward `backlog_stuck_states` showed 33 unresolved `orphaned_triage` rows and every affected item got a "Triage may be stuck" notification.

## Investigation Summary (what turned out NOT to be the bug)

The initial hypothesis was that `reconcileOrphanedTriageItems`' staleness gate (`session/backlog_lifecycle.go`) measures elapsed time from the `ItemSession` row's `created_at` — set synchronously, before the triage goroutine ever acquires a semaphore slot — and could misflag a session still legitimately queued behind the 8-slot cap as "stuck" purely from queue depth. This turned out to be **already defended against**: `s.triageInFlight` (backing `IsTriageLive`) is set in `TriggerTriage`'s synchronous RPC handler *before* the goroutine is even dispatched, and is only cleared via the goroutine's own `defer` once it fully exits — covering the entire wait-for-semaphore period, not just the active LLM call. `reconcileOrphanedTriageItems`' shape-1 branch already calls `IsTriageLive` before tombstoning a stale-looking-but-open session, specifically to guard against this. Also confirmed `triageCallBudget`'s 30-minute clock starts only *after* the semaphore is acquired (`server/services/backlog_service_triage.go:2319`), so queueing delay never erodes the real per-call budget.

Separately, 8 of the 33 rows really were the genuine article: real 30m-full-budget LLM call timeouts (`elapsed(created_at→ended_at)` measured 30.00–30.02 min in every case, both in this incident's cluster and in isolated single-item instances from 2026-08-02/03/04 — i.e., a pre-existing, load-independent pattern, out of scope for this fix per the task's own "don't over-engineer a self-inflicted spike" framing).

## Root Cause

The remaining 26 of 33 rows all shared `end_reason = ''` (empty) with `ended_at` clustered at exactly the moments the service actually restarted (`journalctl`/log correlation: prior process `pid-3245973` shut down at `07:49:06`, new process `pid-3573629` started `07:49:06`–`07:50:07`). These were genuinely self-inflicted by our own restart, but through **two code paths that never learned to say so**:

1. **`TriggerTriage`'s goroutine, still queued for a semaphore slot at shutdown** (`server/services/backlog_service_triage.go`, the `select { case s.triageSem <- struct{}{}: case <-s.shutdownCtx.Done(): ... } ` block). When `shutdownCtx.Done()` fires while a call is still waiting for one of the 8 concurrency slots — the common case during a bulk import, since most of 38 requests queue — it called the *plain* `s.storage.UpdateItemSessionEnded(cleanupCtx, isID, time.Now())`, leaving `EndReason` empty. Only the *other* shutdown path — a call already past the semaphore and actively in `CallBlocking` — goes through `classifyHeadlessCallError`, which correctly buckets `context.Canceled` as `"shutdown"`.

2. **`tombstoneOrphanTriageSessions`**, called at the top of every `TriggerTriage` RPC to close out a stale-looking open session before starting a fresh one. After a restart, `s.triageInFlight` is a brand-new, empty in-memory map — `IsTriageLive` truthfully cannot vouch for a session a *prior*, now-dead process instance started, so every genuinely-still-running (but now abandoned) session from before the restart reads as `!live` here too. This function *also* called the plain `UpdateItemSessionEnded`, with the same missing attribution.

`reconcileOrphanedTriageItems`' existing "shutdown carve-out" (`session/backlog_lifecycle.go:2704`, added for exactly this self-inflicted-restart scenario) only recognizes the literal string `EndReason == "shutdown"` — set correctly today only by `classifyHeadlessCallError`'s narrower "actively running, context canceled" case. Both of the above paths bypassed it, so their sessions fell through to the ordinary shape-2 "genuinely ended without producing a plan" branch: `MarkStuck`, a `"Triage may be stuck"` user notification, and entry into the penalized exponential-backoff remediation schedule (30m/2h/8h/…) — exactly the outcome the carve-out exists to avoid for a zero-evidence, self-inflicted event.

## Fix

`server/services/backlog_service_triage.go`:

1. The `case <-s.shutdownCtx.Done()` branch (still-queued-for-semaphore case) now calls `UpdateItemSessionEndedWithReason(cleanupCtx, isID, time.Now(), "shutdown")` — unambiguous, since reaching this branch can only mean `shutdownCtx` fired.

2. `tombstoneOrphanTriageSessions` now conditionally attributes `"shutdown"` via a new pure, extracted decision function:

   ```go
   func shouldAttributeTombstoneToShutdown(isHeadless, isStale, live bool, createdAt, bootTime time.Time) bool {
       return isHeadless && !isStale && !live && createdAt.Before(bootTime)
   }
   ```

   Only headless sessions that (a) predate this process's own `serverStartTime`, (b) aren't old enough to be independently explained by `maxTriageSessionAge` (genuinely hung/leaked, unrelated to any restart), and (c) aren't live per this process's own record are attributed to our restart — everything else (a same-process anomaly, an item that merely advanced past `idea`, a truly stale/hung call) keeps the plain, unclassified `UpdateItemSessionEnded` path unchanged.

## Regression Tests

`server/services/backlog_service_triage_test.go`:
- `TestTriggerTriage_should_EndWithShutdownReason_When_StillQueuedForSemaphoreDuringShutdown` — fills all 8 concurrency slots with blocking fake calls, triggers a 9th (confirmed genuinely queued, never dispatched to the pool), calls `svc.Shutdown()`, and asserts the 9th item's `ItemSession` ends with `EndReason == "shutdown"`. Verified this fails to build without the fix (references the new helper), confirming the test is wired to the change.
- `TestShouldAttributeTombstoneToShutdown_should_MatchOnlyPreBootNonStaleNotLiveHeadlessSessions` — table-driven test of the extracted pure decision function, covering: pre-boot+not-stale+not-live (→ shutdown), post-boot (→ not shutdown, genuine same-process anomaly), stale (→ not shutdown, independently explained), live (→ not shutdown, still running), and non-headless (→ not shutdown, liveness signal doesn't come from `triageInFlight`). Extracted as a pure function rather than tested via a full DB round-trip because `ItemSession.created_at` is `Immutable()` in the ent schema and this process's own `serverStartTime` is fixed before any test runs — no session created during a test run can ever actually predate it, so only a pure function with an injectable boot time is testable.

`go test ./server/services/...` and `go test ./session/...` pass; `go vet` and `golangci-lint run ./server/services/... ./session/...` are clean.

## Related

- BUG-054/BUG-055 (`session/backlog_lifecycle.go` doc comments) — introduced `triageInFlight`/`IsTriageLive` and the `maxHeadlessTriageSessionStaleness` margin this investigation confirmed are still working correctly for the "genuinely still queued, same process" case.
- The existing shutdown carve-out itself (`session/backlog_lifecycle.go:2704`, `TestReconcileOrphanedTriageItems_should_respawnImmediatelyWithNoPenalty_When_EndedByGracefulShutdown`) was already correct on the *receiving* end — this bug was entirely on the *producing* end (two of three places that end a session for shutdown-adjacent reasons never told it so).
- The remaining 8/33 genuine 30-minute LLM-call timeouts are a separate, pre-existing, load-independent pattern (reproduced on 2026-08-02/03/04, outside any bulk-import window) — not addressed here; flagged for its own investigation if it recurs.
