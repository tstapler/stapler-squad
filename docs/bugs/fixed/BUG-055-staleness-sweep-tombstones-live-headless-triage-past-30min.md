# BUG-055: Periodic Staleness Sweep Tombstones a Genuinely-Live Headless Triage Call Past 30 Minutes [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-08-01)
**Discovered**: 2026-08-01, live — while verifying whether 6 items retried for BUG-053/BUG-054 had resolved cleanly. One (`98f006f2`) had not: its retried triage session ended with no matching "failed"/"complete" log line anywhere, at almost exactly 31 minutes after it started.
**Fixed**: 2026-08-01 — `server/services/backlog_service_triage.go`, `session/backlog_lifecycle.go`.
**Impact**: `reconcileOrphanedTriageItems`' shape-1 branch (the periodic sweep's "still open, is it stale?" check) tombstones ANY headless triage session open longer than `maxHeadlessTriageSessionStaleness` — previously exactly 30 minutes, identical to the real call's own timeout budget with zero margin — with no check for whether the call is actually still running. Confirmed live: an item's triage call ran ~27–31 minutes across three consecutive attempts, meaning it collided with this exact threshold on every single attempt.

## Problem Description

While confirming the 6 items retried for BUG-053 had resolved, `98f006f2`'s retried session (`headless-triage-39b2bb30-...`, created 11:53:41) showed `endedAt: 2026-08-01T19:24:19Z` — 30m38s later — but **no** "headless triage failed" or "headless triage complete" log line exists anywhere for that session, in either the live log or any rotated one. Only one code path sets `ended_at` without logging either of those: `reconcileOrphanedTriageItems`'s shape-1 branch (`session/backlog_lifecycle.go`), which tombstones any triage session still open past `maxHeadlessTriageSessionStaleness` unconditionally:

```go
staleness := maxWorkSessionStaleness
if strings.HasPrefix(latestTriage.SessionUUID, headlessTriageSessionUUIDPrefix) {
    staleness = maxHeadlessTriageSessionStaleness // 30 * time.Minute
}
if time.Since(latestTriage.CreatedAt) <= staleness {
    continue // still plausibly running
}
// tombstoned unconditionally past staleness — no liveness check
```

`maxHeadlessTriageSessionStaleness` was `30 * time.Minute` — **identical** to `TriggerTriage`'s own `triageCtx` budget (`context.WithTimeout(s.shutdownCtx, 30*time.Minute)`), giving zero margin. The constant's own doc comment claimed headless calls "routinely run 7-15 minutes," but this incident's actual data contradicts that: across this item's last 3 attempts, the real calls took 27m41s, 27m53s, and 30m38s — right at or past the sweep's threshold every time, guaranteeing a race between the sweep's ~60s-interval tick and the call's own natural completion/timeout on every slow attempt.

**Root cause**: this is the exact same class of bug as BUG-054 (fixed earlier the same day) — a headless triage session has no live tmux instance to query, so "no liveness check" defaulted to "assume dead past a threshold" — but in a *different* call site than the one BUG-054 fixed (`tombstoneOrphanTriageSessions`, invoked only when `TriggerTriage` is called again for an item). This sweep runs unconditionally on every reconciliation tick regardless of whether anyone ever retriggers triage, so it needed the identical fix independently.

## Fix Applied

1. **Centralized the liveness signal** (the user's ask, alongside the fix): rather than each call site inventing its own "is this headless call alive" answer, `BacklogService.IsTriageLive(itemID string) bool` (`server/services/backlog_service_triage.go`) is now the single source of truth — it reads the same `triageInFlight` registry BUG-054 added. `tombstoneOrphanTriageSessions` was updated to call this method too (previously it read `s.triageInFlight` directly inline) — both call sites now go through one method, not two independent copies of the same check.
2. Extended the existing `session.TriageRespawner` interface (already satisfied by `*BacklogService`, already wired via `SetTriageRespawner`) with `IsTriageLive(itemID string) bool`, so `reconcileOrphanedTriageItems`'s shape-1 branch can consult it without a new setter/field/wiring path — reuses the existing cross-package seam instead of adding a second one.
3. Shape-1 branch: for a headless session past staleness, checks `respawner.IsTriageLive(item.ID)` before tombstoning; a live call is left alone (`continue`) instead of being marked ended. Non-headless (tmux-backed) sessions are unchanged — this fix, like BUG-054, is scoped to headless calls, since that's the only case where no liveness signal previously existed at all.
4. **Increased the staleness margin** (requested directly, as a defense-in-depth measure independent of the liveness-check fix): `maxHeadlessTriageSessionStaleness` raised from 30m to **35m** — a real 5-minute margin over the call's own 30-minute budget (`triageCallBudget`, newly named in `server/services/backlog_service_triage.go` — previously an inline `30*time.Minute` literal duplicated at two call sites, now one named constant referenced from both `TriggerTriage`'s `triageCtx` timeout and `classifyHeadlessCallError`'s own timeout-detection window). Even with `IsTriageLive` as the structural fix, there's no reason to keep courting a race that was previously guaranteed on every slow call.

Per `.claude/rules/interface-pollution-checklist.md`: `IsTriageLive` extends an existing, already-scoped interface (`TriageRespawner`) rather than adding a new one — the two capabilities ("respawn triage" and "is triage live") are both properties of the same underlying object and consumer, so widening the existing seam is correct here, not interface proliferation.

## Regression Tests

`session/backlog_lifecycle_stuck_test.go`:

- `TestReconcileOrphanedTriageItems_should_notTombstone_When_HeadlessSessionStaleButGenuinelyLive` — a headless session backdated 45 minutes (past the new 35m threshold) with a `fakeTriageRespawner` reporting it live must **not** be tombstoned, marked stuck, or notified.
- `TestMaxHeadlessTriageSessionStaleness_should_ExceedRealTriageCallBudgetWithMargin` — asserts the constant exceeds the known 30m call budget by a real margin (2m+), so a future edit can't silently reintroduce the zero-margin race even if `IsTriageLive` itself is later removed or bypassed.

Existing test confirming no regression: `TestReconcileOrphanedTriageItems_should_flagHeadlessSession_After30Min` (no respawner wired, so `IsTriageLive` is never consulted — a session with no liveness signal at all is still tombstoned past staleness exactly as before).

`server/services/backlog_service_test.go`: existing `TestTriggerTriage_*` suite (see BUG-054's doc) re-verified unaffected by routing `tombstoneOrphanTriageSessions` through the new `IsTriageLive` method instead of the raw map read.

**Verified to fail against pre-fix code**: before `IsTriageLive` and the `TriageRespawner` extension existed, the shape-1 branch had no liveness check to consult at all — `TestReconcileOrphanedTriageItems_should_notTombstone_When_HeadlessSessionStaleButGenuinelyLive`'s scenario (a stale-but-live session) would have been tombstoned unconditionally, same as the always-tombstone assertion the now-passing `After30Min` test already covers for the genuinely-dead case.

## Verification

```
$ go build ./...
(clean)

$ gofmt -l server/services/backlog_service_triage.go session/backlog_lifecycle.go session/backlog_lifecycle_stuck_test.go
(clean — no output)

$ go test ./session/ -run 'TestReconcileOrphanedTriageItems|TestMaxHeadlessTriageSessionStaleness' -v
=== RUN   TestReconcileOrphanedTriageItems_should_writeDurableRowNotifyOnce_When_TriageSessionStale
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_tombstoneStaleSession_When_Detected
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_notFlag_When_TriageSessionRecent
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_flagHeadlessSession_After30Min
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_notTombstone_When_HeadlessSessionStaleButGenuinelyLive
--- PASS
=== RUN   TestMaxHeadlessTriageSessionStaleness_should_ExceedRealTriageCallBudgetWithMargin
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_flagImmediately_When_TriageSessionEndedWithoutTransition
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_respawnImmediatelyWithNoPenalty_When_EndedByGracefulShutdown
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_preferNewerOpenSession_When_OlderEndedSessionExists
--- PASS
=== RUN   TestReconcileOrphanedTriageItems_should_notFlag_When_NoTriageSessionEverRan
--- PASS
PASS
ok  	github.com/tstapler/stapler-squad/session	1.395s
```

```
$ go test ./session/... ./server/services/...
ok  	github.com/tstapler/stapler-squad/session	(cached)
ok  	github.com/tstapler/stapler-squad/session/git	(cached)
ok  	github.com/tstapler/stapler-squad/session/headless	(cached)
ok  	github.com/tstapler/stapler-squad/session/memory	(cached)
ok  	github.com/tstapler/stapler-squad/session/mux	(cached)
ok  	github.com/tstapler/stapler-squad/session/namegen	(cached)
ok  	github.com/tstapler/stapler-squad/session/prompts	(cached)
ok  	github.com/tstapler/stapler-squad/session/queue	(cached)
ok  	github.com/tstapler/stapler-squad/session/scrollback	(cached)
ok  	github.com/tstapler/stapler-squad/session/search	(cached)
ok  	github.com/tstapler/stapler-squad/session/tmux	(cached)
ok  	github.com/tstapler/stapler-squad/session/tokens	(cached)
ok  	github.com/tstapler/stapler-squad/session/unfinished	(cached)
ok  	github.com/tstapler/stapler-squad/session/unfinished/gogitstore	60.976s
ok  	github.com/tstapler/stapler-squad/session/vc	(cached)
ok  	github.com/tstapler/stapler-squad/session/vnc	(cached)
ok  	github.com/tstapler/stapler-squad/session/workspace	(cached)
ok  	github.com/tstapler/stapler-squad/server/services	66.690s
```

17 packages exercised, zero `FAIL` lines.

```
$ golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet ./session/... ./server/services/...
0 issues.
```

## Related

- `docs/bugs/fixed/BUG-054-triage-retrigger-duplicates-genuinely-live-headless-call.md` — the sibling instance of this exact shape, in the retrigger-time code path instead of the periodic sweep. Both now share one liveness source (`IsTriageLive`/`triageInFlight`) rather than each guessing independently.
- `docs/bugs/fixed/BUG-053-graceful-shutdown-kills-inflight-triage-treated-as-real-failure.md` — same incident, different mechanism: BUG-053 is about correctly classifying a kill *this process itself caused*; BUG-054/055 are about correctly recognizing a call *this process itself is still running*.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Integration Gap, same as BUG-054 — two independent call sites each needed the same missing fact ("is this headless call actually still running") and each defaulted to guessing "no" rather than having anywhere to ask.

**Recurring shape — explicitly named, per this repo's own convention for a class that resurfaces**: this is the **second** instance in one incident of "headless call liveness has no signal, so the code assumes dead." `SpawnSessionFromItem`'s `spawnInFlight`, `TriggerTriage`'s `triageInFlight`/`IsTriageLive` are now the established pattern (in-process `sync.Map`/registry, `LoadOrStore`-or-check on entry, `Delete` via defer on exit) for this shape — if a third headless/async backlog action (e.g. the review path, which has its own `AutoRespawnReview` alongside a `ReviewRespawner` interface with the identical structural shape) is ever found to have the same silent-tombstone-of-a-live-call risk, reach for this exact pattern rather than re-deriving a bespoke one. A follow-up audit should specifically check whether `reconcileUnprocessedReviewVerdicts`/`markAbandonedReview`'s own staleness gates have the equivalent gap for headless review calls — out of scope here since this incident's evidence was specifically about triage, but the shape is now well-established enough to actively look for elsewhere rather than wait for another live incident to surface it.

**Earliest achievable enforcement**: `TestMaxHeadlessTriageSessionStaleness_should_ExceedRealTriageCallBudgetWithMargin` is a small but real step up the ladder from "just a regression test for this scenario" — it enforces the *invariant* (staleness threshold must exceed the real call budget with margin) rather than only the specific bug instance, so a future edit that shrinks the margin fails a test immediately rather than waiting for another live collision to surface it.
