# BUG-083: Parked Remediation Rows Never Automatically Retry — Requires an Operator to Correlate a Fix With Previously-Parked Items [SEVERITY: High]

**Status**: ✅ FIXED (2026-08-21)
**Discovered**: 2026-08-21, `backlog-feature-improvement` audit (`docs/tasks/backlog-feature-improvement.md`, "Update — 2026-08-21" section) — 20 `orphaned_triage` items found parked for up to 2 weeks.
**Fixed**: 2026-08-21 — `session/backlog_remediation.go`, `session/ent/schema/backlog_stuck_state.go`.
**Impact**: `session/backlog_remediation.go`'s `RemediationDue` — the shared automated-remediation backoff gate every `StuckReason` (`orphaned_triage`, `bouncing`, `push_failed`, `stale_work`, `abandoned_review`, ...) goes through.

## Problem Description

Per `docs/tasks/backlog-stuck-item-auto-remediation.md`'s design (2026-07-20), an open
`BacklogStuckState` row gets up to 5 automated remediation attempts on an exponential backoff
(30m/2h/8h/24h/72h). Once `remediation_attempts >= MaxRemediationAttempts` (5), the row
"parks": `evaluateRemediation` returns `remediationSkippedParked` **unconditionally** —

```go
// session/backlog_remediation.go, before this fix
func evaluateRemediation(row OpenStuckStateData, now, bootTime time.Time) remediationDecision {
	if row.RemediationAttempts >= MaxRemediationAttempts {
		return remediationSkippedParked
	}
	...
```

— with no time-based reprieve of any kind. `RemediationDue` (the caller every reason-specific
remediation dispatcher goes through, e.g. `retryOrphanedTriageWithBackoffGate` in
`session/backlog_lifecycle_triage.go`) then returns `due=false` **forever** for that row. The
only way `due` ever becomes `true` again is an operator explicitly calling
`ResetStuckRemediation`/`BulkResetStuckRemediation`.

This is fine by design for a row parked by a genuinely unrecoverable condition an operator
needs to look at. It is NOT fine for the case that actually happened live: PR #535 (merged
2026-08-18) fixed `classifyHeadlessCallError` in `server/services/backlog_service_triage.go`,
which had been swallowing subprocess-start failures into an undiagnosable `errType=other`
bucket — the root cause behind a batch of `orphaned_triage` items repeatedly failing and
parking. Once that fix landed, every one of those 20 already-parked items should have started
succeeding again on their very next retry — but nothing ever gave them one. They sat stuck for
up to 2 weeks until the 2026-08-21 audit pass happened to notice the correlation between "code
fix merged 2026-08-18" and "a batch of items parked around then" and manually ran
`BulkResetStuckRemediation`.

**Root cause**: `evaluateRemediation`'s parked branch is a dead end with no automatic path out
— it treats "the fast backoff schedule is exhausted" and "this item can never self-heal again"
as the same fact, when they are not: a park only proves the fast schedule (30m to 72h,
~4.5 days total) wasn't long enough to outlast whatever was failing. It says nothing about
whether the underlying cause is still present a week later, and in the confirmed live case, it
already wasn't.

## Recurring Shape (Phase D, before-the-fact)

This is the **sixth recorded instance** of "a fix closes the write side of a gap but not the
recovery side," tracked in `docs/tasks/backlog-feature-improvement.md` since 2026-07-27
(siblings: silent no-op spawn, self-defeating exclusion guard, event lost across restart,
notify-once never resolved, degraded-fallback-masks-error). Per `.claude/rules/fix-flaky-tests-dont-defer.md`'s
"no blast-radius exception" and `quality:reflect-and-fix`'s taxonomy, the fix below is scoped to
the shared gate (`RemediationDue`), not the one call site (`retryOrphanedTriageWithBackoffGate`)
the live incident happened to hit, so it closes this shape for every `StuckReason`, not just
`orphaned_triage`.

## Fix Applied

`session/backlog_remediation.go`:

1. New `remediationColdRetryInterval` constant (7 days) — checked for an existing "reset
   interval"/"heartbeat"/config knob first (`grep -rin "cold.retry\|remediation.*interval\|heartbeat"`
   across the repo turned up only unrelated session/UI heartbeat concepts); none existed, so
   this is a new, deliberately unconfigurable constant, matching `remediationBackoffSchedule`'s
   own style. 7 days because a park is only reached after ~4.5 days of the fast schedule
   already failing — whatever's broken is very likely not transient infra noise (the fast
   schedule + restart-grace already absorb that case), so a much slower heartbeat avoids
   re-hammering a durably-broken external dependency while still guaranteeing automatic
   recovery within about a week of a fix landing.
2. New `remediationGrantedColdRetry` decision in `evaluateRemediation`: once parked,
   `next_remediation_at` is repurposed to hold the cold-retry deadline (instead of going
   unused, as `nextRemediationAt`'s old doc comment noted it did) — if that deadline has
   passed, grant one more attempt.
3. `RemediationDue` handles the new case: grants the attempt, **pins** `remediation_attempts`
   at the cap (does not increment past it — that's what keeps the row eligible for the *next*
   cold retry too, turning this into a repeating heartbeat rather than a one-shot reprieve),
   and pushes `next_remediation_at` another `remediationColdRetryInterval` out. Returns
   `justParked=false` so the caller does not re-send the one-time "auto-remediation exhausted"
   notification for a row that already got it the first time it parked.
4. The attempt that actually parks a row (`RemediationDue`'s normal-grant branch, and
   `RecordManualRemediationAttempt`'s operator-triggered "Retry now" path) now seeds
   `next_remediation_at` with the cold-retry deadline via a new shared helper
   (`nextRemediationAtForAttempt`) instead of the fast schedule's dead last entry — both call
   sites that can record the parking attempt route through one helper, so a manually-triggered
   park can't silently skip seeding the cold-retry deadline the way duplicating this logic at
   each site would risk.

`session/ent/schema/backlog_stuck_state.go`: updated `remediation_attempts`/`next_remediation_at`
field comments to document the repurposed semantics (no schema/migration change — reuses the
existing nullable `next_remediation_at` column rather than adding a new one).

`ResetStuckRemediation`/`BulkResetStuckRemediation` are unchanged — an operator who wants a
row's full fast budget back immediately (rather than waiting up to 7 days for the next
heartbeat) still has that path.

## Regression Tests

`session/backlog_remediation_test.go`:

- `TestRemediationDue_should_grantColdRetry_When_ParkedRowsHeartbeatDeadlineElapses` — the
  primary regression test: a row parked with attempts pinned at the cap and its cold-retry
  deadline already in the past (simulating the live incident's exact shape — parked long
  before the code fix landed) becomes `due=true` from `RemediationDue` **with no
  `ResetStuckRemediation`/`BulkResetStuckRemediation` call anywhere in the test**, keeps
  `remediation_attempts` pinned at the cap across 3 simulated heartbeat cycles, does not
  re-fire `justParked`, and does not fire again before its next interval elapses.
- `TestEvaluateRemediation_should_returnExpectedDecision_When_GivenRowState` — 3 new table
  cases: parked + cold-retry deadline in the future → still parked; parked + deadline in the
  past → `remediationGrantedColdRetry`; parked + deadline exactly `now` → granted (boundary,
  matching the existing "not before now" convention for the fast-schedule case).
- `TestRemediationDue_should_advanceThroughFullScheduleThenPark_When_EachAttemptIsForcedDue`
  (existing test, updated): now asserts the parking attempt (5th) seeds `next_remediation_at`
  with `now + remediationColdRetryInterval` rather than the fast schedule's stale 72h entry,
  and that a 6th call *before* that (much later) deadline still stays parked.

**Verified to fail against pre-fix code**: before this fix, `evaluateRemediation` had no
`remediationGrantedColdRetry` case at all — the new table cases in
`TestEvaluateRemediation_...` would not compile against the old enum, and
`TestRemediationDue_should_grantColdRetry_...`'s core assertion (`due=true` with no manual
reset) is exactly the behavior `TestRemediationDue_should_advanceThroughFullScheduleThenPark_...`'s
OLD final assertion (`assert.False(t, due, "a parked row must never become due again
automatically")`) explicitly asserted could never happen.

## Verification

```
$ go build ./...
(clean)

$ gofmt -l session/backlog_remediation.go session/backlog_remediation_test.go session/ent/schema/backlog_stuck_state.go
(clean — no output)

$ go test ./session/... -run 'Remediation' -v
--- PASS: TestEvaluateRemediation_should_returnExpectedDecision_When_GivenRowState (13 subtests, all PASS)
--- PASS: TestRemediationDue_should_grantColdRetry_When_ParkedRowsHeartbeatDeadlineElapses
--- PASS: TestRemediationDue_should_capAtFiveAttemptsWithDelayedRetries_When_StuckRapidlyInSuccession
--- PASS: TestRemediationDue_should_advanceThroughFullScheduleThenPark_When_EachAttemptIsForcedDue
--- PASS: TestRecordManualRemediationAttempt_should_error_When_NoOpenRowExists
--- PASS: TestRecordManualRemediationAttempt_should_incrementLikeANormalAttempt_When_RowIsOpenAndNotParked
--- PASS: TestRecordManualRemediationAttempt_should_rejectParked_When_AttemptsAtCap
--- PASS: TestResetStuckRemediation_should_clearCountersAndNotifiedAt_When_RowIsOpen
--- PASS: TestBulkResetStuckRemediation_should_onlyResetParkedRows_When_OnlyParkedTrue
--- PASS: TestReconcileOrphanedTriageRemediation_* (all)
--- PASS: TestAttemptPushRemediation_* (all)
PASS
ok  	github.com/tstapler/stapler-squad/session	0.355s

$ go test ./session/...
ok  	github.com/tstapler/stapler-squad/session	(cached)
(all other session/... subpackages ok)

$ go test ./server/services/...
ok  	github.com/tstapler/stapler-squad/server/services	32.459s

$ golangci-lint run ./session/...
0 issues.
```

## Related

- PR #535 (merged 2026-08-18) — fixed the `classifyHeadlessCallError` bug that had parked the
  20 live `orphaned_triage` items this bug's live evidence comes from. That fix was correct and
  is unaffected by this change; this bug is about the items it left stranded, not about
  reverting or altering it.
- `docs/tasks/backlog-stuck-item-auto-remediation.md` — original design doc; updated in place
  with a 2026-08-21 addendum documenting this change rather than left silently stale.
- `docs/tasks/backlog-feature-improvement.md` — tracks the recurring "write side fixed, recovery
  side missing" shape this is the sixth instance of.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap. `RemediationDue`'s contract ("caller invokes its action
iff due") was implicitly assumed by every one of its ~5 call sites to also mean "and this will
eventually happen again on its own if the underlying condition changes," which was never
actually true once a row parked — the gate silently broke that assumption for every caller at
once without any of them individually doing anything wrong.

**Earliest achievable enforcement**: the regression test is close to the earliest achievable
level here — this is inherently a runtime backoff-timing behavior (a duration comparison
against wall-clock time), not something a compile-time type or a lint rule can express. A
table-driven test of `evaluateRemediation`'s pure decision function (already the established
pattern in this file, see `TestEvaluateRemediation_...`) is the right altitude: it pins the
exact decision for every reachable `(remediation_attempts, next_remediation_at)` combination,
including the new cold-retry cases, without needing real wall-clock sleeps.

**Recurring-shape check**: yes — sixth instance of "fix closes the write side of a gap but not
the recovery side" (see above). Closed at the shared-gate level (`RemediationDue`/
`evaluateRemediation`) rather than the one call site (`orphaned_triage`) the live incident
happened to surface it through, so every current and future `StuckReason` that goes through
this gate gets the same automatic recovery path without needing its own fix.
