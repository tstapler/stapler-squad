# Validation Plan: notification-revamp

**Date**: 2026-09-04

## Happy Path Scenario

Given the Baseline (`requirements.md`: notifications accumulate unbounded past
first read, idle review-queue items reappear after being skipped, a
newly-added rule leaves already-escalated approvals stuck, both pages are
flat undifferentiated lists), when a `Bash: rm -rf /tmp/scratch` tool call
escalates to `PendingApproval`, the user adds an approval rule that covers
it, and the rule-reconciliation pass runs, then the pending approval
auto-resolves with a visible "Auto-resolved by rule: `<name>`" audit note (no
manual click needed), the Notifications page's "Needs a decision" section
count drops accordingly, and the item disappears from the Review Queue's
needs-a-decision tier — proving the reconciliation pipeline (Epic 2.1→2.2→2.3)
end to end, since every other in-scope fix (dedup, idle-ack, working-state,
IA sectioning) is a precondition or a display concern around this same
pipeline, not an independent flow.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Scope 1 / Epic 1.1 — dedup collapses regardless of read state | `server/notifications/store_test.go` | `TestAppendDedup_ReadThenRecur_CollapsesAndUnreads` | Unit (happy) | Read `TASK_COMPLETE` record recurs → collapses in place, `OccurrenceCount++`, `IsRead` resets to `false`. |
| Scope 1 / Epic 1.1 — AUTO_APPROVED carve-out | `server/notifications/store_test.go` | `TestAppendDedup_AutoApprovedStaysReadAcrossRecurrence` | Unit (error/edge — the one case that must *not* follow the default rule) | Two `AppendAutoApproved` calls for same session/tool → `OccurrenceCount: 2`, `IsRead` stays `true`. |
| Scope 1 / Epic 1.1 — collapse path sweeps retention | `server/notifications/store_test.go` | `TestAppendDedup_CollapsePathTriggersRetention` | Integration (real `NotificationHistoryStore`, JSON-file-backed) | Seed an aged-out unrelated record, trigger a collapse via `Append()`, assert `enforceRetention()` fires on the collapse branch too, not only the new-record branch. |
| Scope 1 / Epic 1.1 — retention keyed on `LastOccurredAt` | `server/notifications/store_test.go` | `TestEnforceRetention_KeyedByLastOccurredAt_SurvivesOldCreatedAt` | Unit (happy) | `CreatedAt: now-10d`, `LastOccurredAt: now-2m`, `OccurrenceCount: 47` → record survives `enforceRetention()`. |
| Scope 1 / Epic 1.1 — retention still prunes cold records | `server/notifications/store_test.go` | `TestEnforceRetention_NoRecentOccurrence_StillPruned` | Unit (error/edge) | `CreatedAt: now-10d`, `LastOccurredAt: now-9d` → pruned. |
| Scope 1 / Epic 1.1 — retention fallback for old data (**gap — not named in plan, add it**) | `server/notifications/store_test.go` | `TestEnforceRetention_NilLastOccurredAt_FallsBackToCreatedAt` | Unit (edge) | Pre-migration record with `LastOccurredAt: nil` → cutoff check uses `CreatedAt`, matching pre-fix behavior exactly (plan's Story 1.1.2 AC lists this case but Task 1.1.2b only names two of the three tests). |
| Scope 1 / Epic 1.1 — `deduplicateExisting()` parity | `server/notifications/store_test.go` | `TestDeduplicateExisting_ReadAndUnreadDuplicates_Consolidate` | Integration (runs via `NewNotificationHistoryStore()` load from disk) | On-disk read+unread duplicates for the same `(sessionID, notificationType)` consolidate into one record with summed `OccurrenceCount` on next load. |
| Scope 1 — `APPROVAL_NEEDED` ID-reassignment unaffected | `server/notifications/store_test.go` | `TestAppendDedup_ApprovalNeeded_IDReassignmentUnaffectedByCollapse` (**gap — add**) | Unit (regression/error path) | Second approval request collapses into existing record, but `ID` still reassigns to the incoming UUID and `IsRead` becomes `false` — pins the one documented side-effect the widened predicate must not break. |
| Scope 2 / Epic 1.2 — idle ack-suppression (no-controller) | `session/review_queue_determiner_test.go` | `TestDefaultStatusDeterminer_IdleAckSuppression_StaysOutUntilNewOutput` | Unit (happy) | Acknowledged idle session stays `DetectionActionSkip` at t0+10s with no new output; new output at t0+15s then re-idling produces `ReasonIdle` again. |
| Scope 2 / Epic 1.2 — idle ack-suppression (controller-active) | `session/review_queue_determiner_test.go` | `TestDefaultStatusDeterminer_IdleAckSuppression_ControllerActive_StaysOutUntilNewOutput` | Unit (happy, second site) | Same assertion via the controller-active `IdleStateTimeout` case, proving both sites share `suppressedByAck`. |
| Scope 2 / Epic 1.2 — suppression never masks a real reason | `session/review_queue_determiner_test.go` | `TestDefaultStatusDeterminer_ApprovalPending_NeverReachesIdleSuppression` | Unit (error/edge) | Session with `StatusNeedsApproval` and a stale ack timestamp still returns `ReasonApprovalPending`/`PriorityHigh` — idle-suppression branch is provably unreached. |
| Scope 2 / Epic 1.2 — suppression survives a real poll cycle | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_SkipIdleSession_StaysSuppressedAcrossPolls` (**gap — add**) | Integration (real `ReviewQueuePoller` + `Instance`, multiple `checkSessionsSafe()` ticks) | Skip action calls `MarkAcknowledged()` on a real instance; two subsequent poller ticks with no new output never re-add it — this is the actual "doesn't reappear on the very next poll tick" success metric, not just the pure-function unit test. Must not regress `TestReviewQueuePoller*` per Constraints. |
| Scope 3 / Epic 1.3 — WaitingForAgent trusted during grace period | `session/review_queue_determiner_test.go` | `TestDefaultStatusDeterminer_ControllerWaitingForAgent_RecentlyUpdated_Removed` | Unit (happy) | `IsControllerActive: true`, `StatusWaitingForAgent`, `UpdatedAt: now-2m` → `DetectionActionRemove`. |
| Scope 3 / Epic 1.3 — stale WaitingForAgent falls through | `session/review_queue_determiner_test.go` | `TestDefaultStatusDeterminer_ControllerWaitingForAgent_Stale_FallsThroughToIdle` | Unit (error/edge) | Same status but `UpdatedAt: now-45m` (past `waitingForAgentStuckThreshold`) → falls through to normal idle/stale checks, not trusted forever. |
| Scope 4 / Epic 2.1 — reconciled resolution stamps metadata | `server/services/approval_service_test.go` | `TestResolveApprovalReconciled_StampsMetadata` | Unit/Integration (real `ApprovalStore` + `NotificationHistoryStore` fixture, per Task 2.2.2a's guidance) | `ResolveApprovalReconciled` resolves via the same path as a human click, then stamps `classifier_rule_name` + `reconciled: "true"`. |
| Scope 4 / Epic 2.1 — losing human's click after reconciliation | `server/services/approval_service_test.go` | `TestResolveApproval_AfterReconciled_ReturnsFailedPrecondition` | Unit (error path) | A human's `ResolveApproval` on an already-reconciled ID gets `connect.CodeFailedPrecondition` naming the rule, not the generic `CodeNotFound`. |
| Scope 4 / Epic 2.1 — human-vs-reconciliation arbitration primitives | `server/services/approval_service_test.go` | `TestApprovalStore_HumanResolving_MarksAndClears`, `TestResolveApprovalReconciled_DefersToInFlightHumanResolution`, `TestIsApprovalPending_ReflectsStoreState` | Unit (error/edge, pins the primitives the hard concurrency test below depends on) | Mark/clear round-trip; a marked-in-flight ID makes `ResolveApprovalReconciled` return `CodeAborted` without touching the store; `IsApprovalPending` reflects live store state. |
| Scope 4 / Epic 2.2 — auto-resolves a newly-covered pending item | `server/services/rules_service_test.go` | `TestReconcilePendingApprovals_AutoAllowResolves` | Integration (real `ApprovalService`+`ApprovalStore`+`NotificationHistoryStore`, temp-file fixture) | New rule added via `UpsertApprovalRule` → previously-`Escalate` pending item resolves `"allow"`, notification carries `reconciled: "true"`. |
| Scope 4 / Epic 2.2 — still-escalating item left untouched | `server/services/rules_service_test.go` | `TestReconcilePendingApprovals_StillEscalates_LeftUntouched` | Unit (happy — no-op branch, lightweight fake resolver) | Rule doesn't match the pending item → `ResolveApprovalReconciled` never called, item remains pending. |
| Scope 4 / Epic 2.2 — **human-vs-reconciliation concurrent race (hardest case)** | `server/services/rules_service_test.go` | `TestReconcilePendingApprovals_HumanWinsRace` | Integration — real concurrency (goroutines + channels, not sleeps) | See dedicated **Concurrency Test Design** section below. |
| Scope 4 / Epic 2.2 — soft cap on runaway reconciliation | `server/services/rules_service_test.go` | `TestReconcilePendingApprovals_CapsAtMaxPerPass` | Unit (edge, lightweight fake resolver) | `maxReconcileAutoResolvesPerPass + 5` matching items → exactly the cap resolved, rest skip, "capped" summary log fires. |
| Scope 4 / Epic 2.2 — **panic-recovery reconciliation pass (hardest case)** | `server/services/rules_service_test.go` | `TestReconcilePendingApprovals_PanicIsRecovered` | Unit (error path, fault injection) | See dedicated **Panic-Recovery Test Design** section below. |
| Scope 4 / Epic 2.2 — CI-red-guard decline counted/logged distinctly | `server/services/rules_service_test.go` | `TestReconcilePendingApprovals_CIRedGuardDecline_LoggedAndCountedSeparately` | Integration (real approval-service CI-block fixture) | Failing-CI session + matching new rule → `CodeFailedPrecondition`, item still present, counted as `declined_by_ci_guard_count`, not `lost_to_concurrent_pass_count`. |
| Scope 4 / Epic 2.2 — lost-race-to-concurrent-pass disambiguated from CI guard | `server/services/rules_service_test.go` | `TestReconcilePendingApprovals_LostToConcurrentPass_CountedSeparatelyFromCIGuard` | Unit (edge, lightweight fake resolver simulating a stale snapshot) | Same error code (`CodeFailedPrecondition`) but `IsApprovalPending == false` → counted as `lost_to_concurrent_pass_count`, not `declined_by_ci_guard_count` — counterpart assertion to the row above. |
| Scope 4 / Epic 2.1.2 — service wiring (**gap — add**) | `server/services/session_service_test.go` | `TestNewSessionService_WiresApprovalServiceIntoRulesService` | Unit (construction-order regression) | After `NewSessionService` returns, `rulesSvc`'s `approvalSvc` field is non-nil (via a package-internal accessor or by triggering a rebuild and observing a reconciliation attempt) — guards against the setter call being silently dropped in a future edit. |
| Scope 4 / Epic 2.3.1 — removal event carries rule name | `session/review_queue_test.go` | `TestReviewQueue_RemoveWithInfo_CarriesRuleName`, `TestReviewQueue_Remove_StillReportsUserAction` | Unit (happy + regression) | `AutoResolvedByRuleRemoval("x")` reaches the observer with `Reason()=="auto_resolved_by_rule"`/`RuleName()=="x"`; plain `Remove()` still reports `UserActionRemoval()`'s values unchanged. |
| Scope 5 / Epic 3.1.1 — "info" filter is an allow-list, not an exclusion-list | `web-app/src/lib/utils/notificationMapping.test.ts` | `test("info category never includes question")` | Unit (happy) | `notificationTypeFilter("info", ["question","info","auto_approved"])` excludes `"question"`. |
| Scope 5 / Epic 3.1.1 — unknown future type defaults to informational | `web-app/src/lib/utils/notificationMapping.test.ts` | `test("info category includes an unrecognized-but-mapped type")` | Unit (edge) | A hypothetical `"reminder"` type not in `ACTIONABLE_TYPES` still returns from `"info"`. |
| Scope 5 / Epic 3.1.1 — exhaustiveness across the full type union | `web-app/src/lib/utils/notificationMapping.test.ts` | `test("every UIType is classified by isActionableNotification, none fall through both")` | Unit (error/edge — regression for the found `"task_complete"` gap) | Every member of the 13-type `notificationType` union is either in `ACTIONABLE_TYPES` or returned by `notificationTypeFilter("info", [type])`, never neither. |
| Scope 5 / Epic 3.1.2 — Needs-a-decision section renders unread actionable items on top | `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx` | `test("unread actionable notifications render in NeedsDecisionSection above informational list")` | Unit (happy, component render) | One unread `approval_needed` + three read `task_complete` → the approval item renders inside `NeedsDecisionSection`, the three `task_complete` items only appear in the collapsed informational list. |
| Scope 5 / Epic 3.1.2 — calm empty state, golden fixture | `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx` | `test("NeedsDecisionSection shows All caught up when empty, informational stays visible collapsed")` | Unit (edge — golden fixture, per Task 3.1.2d) | Zero unread actionable items → "All caught up" + checkmark, never "No items found"; informational section still rendered, collapsed. |
| Scope 5 / Epic 3.1.3 — reconciled item appears only in Auto-handled | `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx` | `test("reconciled approval_needed record lands in AutoHandledSection, excluded from main feed")` | Unit (happy + implicit error-path: excluded elsewhere) | `metadata: {reconciled: "true"}` record → present in `autoHandledNotifications`, absent from `filteredNotifications`. |
| Scope 5 / Epic 3.1.4 — header count caps at 99+ | `web-app/src/app/notifications/__tests__/NotificationsPage.test.tsx` | `test("header unread count caps at 99+ above 99, shows literal number at/below 99")` | Unit (happy + edge, boundary at exactly 99/100) | `unreadCount: 152` → "99+"; `unreadCount: 99` → "99"; `unreadCount: 7` → "7". |
| Scope 6 / Epic 3.2.1 — priority-tier partition | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `test("PriorityUrgent/High/Medium render in expanded Needs-a-decision tier, PriorityLow in collapsed Informational tier")` | Unit (happy, golden fixture per Task 3.2.1c) | Mixed-priority fixture renders as specified; empty needs-decision tier shows the calm empty state, informational tier stays visible-collapsed. |
| Scope 6 / Epic 3.2.2 — idle items excluded from list and both counts | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`, `web-app/src/lib/hooks/useReviewQueue.test.ts` | `test("idle-reason item never renders and nav badge count excludes it")`, `test("useReviewQueue totalItems excludes idle items")` | Unit (error/edge — the count-vs-list mismatch class of bug this project targets) | Two-item fixture (`idle` + `approval_pending`) → panel list length 1, nav badge 1, headline `totalItems` reads "1 item in queue" (the filtered hook value), never the raw backend stat of 2 (adversarial-review.md Blocker 2). |
| Scope 6 / Epic 3.2.2 — idle chip un-suppressed | `web-app/src/components/sessions/SessionRow.test.tsx` | `test("SubStatusChip renders for IDLE substatus")` (flip of existing suppression assertion) | Unit (regression) | `SessionRow` renders the "● Idle" chip once the `SubStatus.IDLE` exclusion term is removed. |
| Scope 6 / Epic 2.3.2 — frontend labeling of a reconciled resolution | `web-app/src/components/ui/NotificationItem.test.tsx`, `web-app/src/lib/hooks/useApprovalResolution.test.ts` (new) | `test("reconciled approval shows 'Auto-resolved by rule: <name>' not '✓ Approved'")`, `test("FailedPrecondition with reconciliation message lands in blockedApprovals, not the generic expired path")` | Unit (happy + error path) | Reconciled metadata → distinct badge copy; a losing human click's `CodeFailedPrecondition` (any message shape) routes into `blockedApprovals`, never the generic `catch` → `"expired"` branch. |
| Scope 6 / Epic 2.3.2 — review-queue panel disables in place | `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` | `test("item_removed with autoResolvedByRule disables the row in place for ~5s before removal")` | Unit (happy — the "disable, don't hide" spec) | Row stays present-but-disabled with the rule-attribution banner for the display window, then is removed once the timer elapses. |

**Migration test**: N/A — per Step 5, no schema/Migration Plan section exists (JSON-file-backed stores, additive fields only, `deduplicateExisting()` is covered above as an ordinary integration test, not a migration test).

---

## Concurrency Test Design — `TestReconcilePendingApprovals_HumanWinsRace`

This is the plan's Task 2.2.2c, and the single highest-risk test in this
plan: an earlier draft (flagged as a Blocker in adversarial review) resolved
the approval via a human call *before* triggering reconciliation — a
sequential simulation that never actually exercises the arbitration
mechanism. The design below uses real goroutines synchronized by unbuffered
channels, never `time.Sleep`, so the interleaving is deterministic.

**Fixture**: real `ApprovalStore` (temp-file backed) + real `ApprovalService`
+ real `RulesService`, matching Task 2.2.2a's "reserve the real store for
tests asserting persisted state end-to-end" guidance — this test needs to
read `ApprovalStore.mu`-guarded state (`humanResolving`) and a
package-internal `PendingApproval.decisionCh`, so a fake resolver cannot
stand in.

```go
func TestReconcilePendingApprovals_HumanWinsRace(t *testing.T) {
    // 1. Seed: pending approval "appr-race" (ToolInput: {"command": "rm -rf /tmp/scratch"}),
    //    keep the *PendingApproval value in scope for its unexported decisionCh.
    //    Also seed a rule that would AutoAllow it, wired into rulesSvc's classifier.

    humanClaimed := make(chan struct{})   // closed once MarkHumanResolving has landed
    releaseHuman := make(chan struct{})   // closed by the test to let the human's Resolve proceed
    var wg sync.WaitGroup
    wg.Add(1)

    // 2. Human-side goroutine — deliberately isolated to the ApprovalStore
    //    primitive calls resolveApproval's human branch makes, so the sync
    //    point lands exactly on the arbitration boundary, not inside
    //    unrelated CI-guard/stamping I/O.
    go func() {
        defer wg.Done()
        approvalStore.MarkHumanResolving("appr-race")
        close(humanClaimed) // happens-before: reconciliation's IsHumanResolving check
        <-releaseHuman
        _ = approvalStore.Resolve("appr-race", ApprovalDecision{Behavior: "deny", Message: "blocking this"})
        approvalStore.ClearHumanResolving("appr-race")
    }()

    // 3. Main goroutine: wait for the claim, THEN attempt reconciliation —
    //    this ordering is what makes the race deterministic instead of hoped-for.
    <-humanClaimed
    err := approvalSvc.ResolveApprovalReconciled(ctx, "appr-race", "allow", "Auto-allow safe git status checks")
    require.Error(t, err)
    require.Equal(t, connect.CodeAborted, connect.CodeOf(err))
    require.Contains(t, approvalStore.ListAll(), /* appr-race still pending */)

    // 4. Let the human's Resolve proceed, then join.
    close(releaseHuman)
    wg.Wait()

    // 5. Ground-truth assertion: read the delivered decision directly off
    //    the approval's own channel — not inferred from error codes or
    //    store absence alone.
    decision := <-pendingApproval.decisionCh // buffered cap 1, already delivered
    require.Equal(t, "deny", decision.Behavior)
    require.Equal(t, "blocking this", decision.Message)
    require.NotContains(t, approvalStore.ListAll(), /* appr-race gone: resolved */)

    // 6. Re-run the identical interleaving through the full
    //    reconcilePendingApprovals loop (not just a direct call) to confirm
    //    counts.deferredToHuman increments and the "deferred to in-flight
    //    human decision" log line fires — pins the loop's own branch, not
    //    just the primitive ResolveApprovalReconciled already covered above.
}
```

Why this is non-flaky: the only synchronization primitives are channel
send/close/receive (`humanClaimed`, `releaseHuman`) — there is no `sleep`,
no polling loop, no timing assumption. The `<-humanClaimed` receive is a
real happens-before edge guaranteeing `MarkHumanResolving` completed before
`ResolveApprovalReconciled` is even called, so the outcome is deterministic
on every run, not just "usually right." `go test -race` should be run on
this test explicitly given the shared-map/mutex primitives under test.

---

## Panic-Recovery Test Design — `TestReconcilePendingApprovals_PanicIsRecovered`

Covers Task 2.2.1a: the recovery wrapper (`reconcilePendingApprovalsSafe`)
must (1) never let a panic escape to crash the process, (2) log the panic,
and (3) still log a truthful partial-progress summary — the critical
correctness property adversarial review flagged, since the counters must
live in the *wrapper's* stack frame, not the panicking function's, or the
deferred summary log reads zeroed/stale counts.

```go
func TestReconcilePendingApprovals_PanicIsRecovered(t *testing.T) {
    // Fake reconciliationResolver returning 3 items: two resolvable via the
    // seeded rule, then one whose ToolInput is malformed such that
    // Classify() (or the fake's own stand-in) panics — e.g. a deliberate
    // fake that panics on the 3rd ListPendingApprovalsInternal() item to
    // simulate a nil-map access mid-loop, rather than a real classifier bug.
    fake := &fakeReconciliationResolver{
        items: []*PendingApproval{item1, item2, panicItem},
    }
    rulesSvc.approvalSvc = fake

    logs := captureLogs(t) // this file's existing log-capture convention (Task 2.2.2d)

    done := make(chan struct{})
    rulesSvc.reconcileDoneHook = func() { close(done) }

    require.NotPanics(t, func() {
        rulesSvc.reconcilePendingApprovalsSafe()
    })
    <-done // deterministic completion signal, not a sleep

    // Process didn't crash (require.NotPanics above) and:
    require.True(t, logs.Contains("panic in reconcilePendingApprovals recovered"))
    require.True(t, logs.Contains("reconciliation pass complete"))
    summary := logs.Last("reconciliation pass complete")
    require.Equal(t, 2, summary.Int("resolved_count")) // item1+item2 resolved BEFORE the panic
    require.Equal(t, true, summary.Bool("panicked"))
    // item1/item2 actually resolved (real side effects), not just counted:
    require.True(t, fake.resolved("item1"))
    require.True(t, fake.resolved("item2"))
}
```

The property this test exists to pin — and the exact bug adversarial review
caught in an earlier draft — is that `resolved_count`/`skipped_count`/etc.
are declared in `reconcilePendingApprovalsSafe`'s frame and passed into the
loop by pointer (`*reconcileCounts`), so a panic unwinding out of
`reconcilePendingApprovals` mid-loop leaves the wrapper's `defer` reading
real partial counts, not zero values from a fresh struct. A version of this
test that only asserted "it doesn't crash" would pass against the buggy
zeroed-counter version too — the `resolved_count == 2` assertion is what
actually exercises the fix.

---

## Dedup ↔ Retention Interaction — `TestAppendDedup_CollapsePathTriggersRetention`

The third named hard case: Task 1.1.1a adds `s.enforceRetention()` to the
collapse branch of `Append()`, which previously only ran from the
new-record branch. Before this fix, a frequently-recurring notification
(exactly the "Claude turn complete" case this whole epic targets) could go
arbitrarily long between retention sweeps, since a sweep only happened when
some *other*, unrelated notification arrived anywhere in the store.

```go
func TestAppendDedup_CollapsePathTriggersRetention(t *testing.T) {
    store := newTestStore(t) // real NotificationHistoryStore, temp JSON file

    // Seed an unrelated record already past MaxNotificationAge with no
    // recent occurrence — this is the record retention should sweep.
    stale := &NotificationRecord{
        ID: "stale-1", SessionID: "sess-other", NotificationType: NOTIFICATION_TYPE_PROGRESS,
        CreatedAt: now.Add(-8 * 24 * time.Hour),
        LastOccurredAt: ptr(now.Add(-8 * 24 * time.Hour)),
    }
    store.seed(stale)

    // Trigger the COLLAPSE branch specifically (not the new-record branch):
    // append the SAME (sessionID, notificationType) pair twice so the
    // second call exercises findDuplicate's collapse path.
    store.Append(newRecord("sess-a1b2c3", NOTIFICATION_TYPE_TASK_COMPLETE))
    require.Len(t, store.records, 2) // first append: new-record branch, stale still present
    store.Append(newRecord("sess-a1b2c3", NOTIFICATION_TYPE_TASK_COMPLETE)) // collapse branch

    // The collapse branch's enforceRetention() call must have swept "stale-1"
    // even though nothing about this Append() call touched sess-other at all.
    _, found := store.GetByID("stale-1")
    require.False(t, found, "collapse branch must call enforceRetention() too, not just the new-record branch")
}
```

This test is the concrete proof that "the fix keeps a recurring record from
going a long time between sweeps" is actually true, rather than trusting the
task description's claim that adding one line is sufficient — a reviewer
reading only the diff cannot see that the collapse branch previously
returned early at the old line 164, before any retention call.

---

## UX Acceptance Tests

Per the `ui-playwright` skill as the implementation model and this repo's
`e2e-test-conventions` skill (every spec starts with `// @feature`, no
`waitForTimeout`, `data-testid`/ARIA-role locators only). Backend test-mode
seeds no notification/review-queue history and exposes no RPC to inject
server-computed records directly (confirmed: `notifications-responsive.spec.ts`'s
header comment documents this exact constraint) — every spec below follows
that file's precedent of mocking via ConnectRPC route interception
(`page.route(...)`), not real backend state.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Needs-a-decision visible in 0 clicks | `tests/e2e/notifications-needs-decision.spec.ts` (new) | `"needs-a-decision section is visible above the fold on page load"` | Playwright | Mock `GetNotificationHistory` with 1 unread `approval_needed` + 3 read items → navigate to `/notifications` → assert the section is visible via `getByRole('region', {name: /needs a decision/i})` with no prior click. |
| 2. Act on an item in ≤2 clicks | `tests/e2e/notifications-needs-decision.spec.ts` | `"approve an item in two clicks from page load"` | Playwright | Navigate → click `getByRole('button', {name: 'Approve'})` on the first card → assert `ResolveApproval` RPC fired and the item leaves the section (`expect(locator).toHaveCount(0)`, no `waitForTimeout`). |
| 3. Failed approve/deny shows retry, no dead end | `tests/e2e/notifications-needs-decision.spec.ts` | `"failed approve shows retry message and keeps buttons active"` | Playwright | Mock `ResolveApproval` to reject → click Approve → assert text "Couldn't record your decision — try again." and `getByRole('button', {name: 'Approve'})` still `toBeEnabled()`. |
| 4. Header count and BottomNav badge agree | `tests/e2e/notifications-needs-decision.spec.ts` | `"header count and BottomNav bell badge match within one poll cycle"` | Playwright | Mock history with N unread actionable items → assert header badge text equals `BottomNav`'s Alerts badge text via `expect.poll` (bounded, not `waitForTimeout`) within one poll interval. |
| 5. Header caps at 99+ | `tests/e2e/notifications-needs-decision.spec.ts` | `"header badge shows 99+ above 99, literal number at or below 99"` | Playwright | Mock `unreadCount: 152` → assert "99+"; mock `unreadCount: 99` → assert "99". |
| 6. Collapsed by default, never remembered | `tests/e2e/notifications-needs-decision.spec.ts` | `"recent activity and auto-handled sections are collapsed on every fresh load"` | Playwright | Expand a section → reload the page → assert it renders collapsed again (`aria-expanded="false"`). |
| 7. Sections expand via real keyboard-focusable button | `tests/e2e/accessibility.spec.ts` (extend) | `"collapsible section headers are Tab-reachable and toggle via Enter/Space"` | Playwright | `page.keyboard.press('Tab')` to the header, assert focused element has `role=button`/`aria-expanded`, press `Enter`, assert `aria-expanded="true"`. |
| 8. Expand/collapse never changes counts | `tests/e2e/notifications-needs-decision.spec.ts` | `"expanding recent activity does not change needs-decision or badge counts"` | Playwright | Record header/badge text, expand Recent Activity, assert text unchanged. |
| 9. Auto-approved vs. reconciled labels distinct | `tests/e2e/notifications-needs-decision.spec.ts` | `"live auto-approval and rule-reconciled item show different Auto-handled labels"` | Playwright | Mock one record `notificationType: auto_approved` and one with `metadata.reconciled: "true"` → assert "Auto-approved" and "Auto-resolved by rule: `<name>`" both render, never identical text. |
| 10. Reconciled item appears exactly once | `tests/e2e/notifications-needs-decision.spec.ts` | `"reconciled item never appears in Needs a Decision or Recent Activity"` | Playwright | Same fixture as above → assert the reconciled item's session ID is absent from both other sections' locators. |
| 11. Empty-state copy exact | `tests/e2e/notifications-needs-decision.spec.ts` | `"empty needs-decision state shows All caught up, never No items found"` | Playwright | Mock zero unread actionable items → assert exact text "All caught up" + "Nothing needs your attention right now" and a checkmark icon; assert absence of "No items found". |
| 12. Informational section stays visible under empty state | `tests/e2e/notifications-needs-decision.spec.ts` | `"informational section remains visible and collapsed when needs-decision is empty"` | Playwright | Same fixture → assert `getByRole('button', {name: /recent activity/i})` is visible with `aria-expanded="false"`. |
| 13. Never shows "caught up" during initial load | `tests/e2e/notifications-needs-decision.spec.ts` | `"loading indicator shows before first poll response, never a false All caught up"` | Playwright | Delay the mocked `GetNotificationHistory` response → assert a loading skeleton renders first, "All caught up" only appears after the delayed response resolves (`await expect(...).toBeVisible()`, not a fixed wait). |
| 14. Reconciliation-race buttons disabled, not removed | `tests/e2e/approval-reconciliation-race.spec.ts` (new) | `"reconciled-while-open item's action buttons become disabled, never silently removed"` | Playwright | Notifications page open with an `approval_needed` item; simulate the reconciliation event landing (mocked poll response now carries `reconciled: "true"`) → assert Approve/Deny become `toBeDisabled()`, still present in the DOM (not `toHaveCount(0)`). |
| 15. Banner names the specific rule | `tests/e2e/approval-reconciliation-race.spec.ts` | `"mid-race banner names the specific rule, never generic 'This item was resolved'"` | Playwright | Same fixture → assert text contains "Auto-resolved by rule: Auto-allow safe git status checks"; assert absence of generic "This item was resolved". |
| 16. Losing-race message is specific | `tests/e2e/approval-reconciliation-race.spec.ts` | `"human's losing click shows 'already auto-resolved by rule ... no action needed'"` | Playwright | Mock `ResolveApproval` to reject with `CodeFailedPrecondition` + reconciliation message → click Approve → assert exact substring, assert absence of "Expired" or a raw RPC error string. |
| 17. No "Approve anyway" on a race message | `tests/e2e/approval-reconciliation-race.spec.ts` | `"reconciliation-race message offers only Deny, never Approve anyway"` | Playwright | Same fixture → assert `getByRole('button', {name: /approve anyway/i})` has zero count; `getByRole('button', {name: 'Deny'})` present (or no actions at all). |
| 18. Idle chip appears within one poll cycle | `tests/e2e/session-board-view.spec.ts` (extend) or `tests/e2e/idle-session-chip.spec.ts` (new) | `"idle SubStatus chip renders on Sessions list within one poll cycle of going idle"` | Playwright | Mock a session transitioning to idle substatus mid-test → `expect.poll` for "● Idle" chip visible within the poll interval. |
| 19. Idle session never also in Review Queue | `tests/e2e/review-queue-priority-tiers.spec.ts` (new) | `"idle session shows on Sessions list chip and is absent from Review Queue simultaneously"` | Playwright | Same mocked session → assert "● Idle" visible on `/sessions` and absent (`toHaveCount(0)`) from `/review-queue`'s rendered rows in the same test run. |
| 6 (RQ). Needs-a-decision tier priority partition | `tests/e2e/review-queue-priority-tiers.spec.ts` | `"Urgent/High items render expanded, Low items collapsed under Informational"` | Playwright | Mock mixed-priority queue → assert urgent/high rows visible without interaction; Low row only visible after expanding "Informational". |
| Bulk skip scoped to visible tier | `tests/e2e/review-queue-priority-tiers.spec.ts` | `"Skip all only targets the Needs a Decision tier"` | Playwright | Mock 2 needs-decision + 3 informational items → click "Skip all (2)" → assert only the 2 needs-decision sessions receive `AcknowledgeSessions` calls. |
| 8 (RQ). Review Queue empty state | `tests/e2e/review-queue-priority-tiers.spec.ts` | `"Review Queue empty needs-decision state matches Notifications page's calm treatment"` | Playwright | Mock zero needs-decision items → assert identical "All caught up" copy/icon; Informational stays visible collapsed. |
| 20. Full keyboard navigation, no mouse-only interaction | `tests/e2e/accessibility.spec.ts` (extend) | `"every actionable control on Notifications and Review Queue is Tab-reachable and Enter/Space-activatable"` | Playwright | Tab through the full interactive set (Approve/Deny/Skip/Open session/Create rule/collapsible headers) on both pages; assert each activates via keyboard alone. |
| 21. Priority badges carry descriptive aria-label | `tests/e2e/accessibility.spec.ts` (extend) | `"priority badges expose aria-label text, not icon/color alone"` | Playwright | Query each `ReviewQueueBadge` instance → assert non-empty `aria-label` matching e.g. "URGENT priority: approval pending". |
| 22. WCAG AA contrast for priority tokens | Axe Core CI gate (existing, per repo's UX-analysis CI) + `tests/e2e/accessibility.spec.ts` (extend) | `"axe-core reports no color-contrast violations on priority/status badges"` | Playwright + `@axe-core/playwright` | Run Axe against both pages in light and dark themes; assert zero `color-contrast` violations scoped to badge selectors — this criterion is a human spot-check backstopped by the existing CI gate, not a new gate. |
| 23. Non-color priority encoding | `tests/e2e/accessibility.spec.ts` (extend) | `"every priority indicator combines icon, text abbreviation, and aria-label"` | Playwright | For each priority badge, assert presence of an icon element, a text node matching the abbreviation, and a non-empty `aria-label` simultaneously. |
| 24. `aria-live="polite"` discipline | `tests/e2e/accessibility.spec.ts` (extend) | `"needs-a-decision live region is polite, mounted from first paint, announces only a short count"` | Playwright | Assert `[aria-live="polite"]` exists in the DOM immediately on page load (before any items exist) and its text content is a short count string, never the full list markup. |
| 25. Collapsible semantics | `tests/e2e/accessibility.spec.ts` (extend) | `"collapsible triggers are real buttons with aria-expanded and a full accessible name"` | Playwright | Assert `getByRole('button', {name: /recent activity, \d+ items, collapsed/i})` exists — not a bare "24" with no label. |
| 26. No dead ends on any error state | `tests/e2e/notifications-needs-decision.spec.ts` + `tests/e2e/approval-reconciliation-race.spec.ts` | (covered by criteria 3, 16, 17's own assertions — each error-state test above already asserts a next action exists) | Playwright | Cross-referenced, not a separate test — every error-state test in this table asserts a visible next action as part of its own scenario. |
| 27. Mobile touch targets ≥44×44pt | `tests/e2e/notifications-responsive.spec.ts` (extend) | `"Approve/Deny/Skip/Create rule/Open session buttons meet 44x44pt minimum on mobile viewport"` | Playwright | Set a mobile viewport (existing pattern in this file) → for each actionable button, assert `boundingBox()` width/height ≥44 CSS px; assert no hover-only affordance (all actions reachable via tap alone). |

## Test Stack

- **Unit**: Go `testing` + `testify` (existing convention in
  `server/notifications/store_test.go`, `session/review_queue_determiner_test.go`,
  `server/services/*_test.go`); TypeScript/Jest + React Testing Library for
  `web-app/src/**/*.test.ts(x)`.
- **Integration**: Go tests against real, temp-file-backed
  `NotificationHistoryStore`/`ApprovalStore`/`ReviewQueue`/`ReviewQueuePoller`
  instances (no mocks for the store layer itself) — per this repo's existing
  fixture pattern in `approval_service_test.go`/`rules_service_test.go`.
  Concurrency-sensitive cases (`TestReconcilePendingApprovals_HumanWinsRace`)
  must run under `go test -race`.
- **E2E / UX**: Playwright (`tests/e2e/`), per the `ui-playwright` skill and
  this repo's `e2e-test-conventions` skill — `// @feature` annotation on
  every new spec file, `data-testid`/ARIA-role locators only, no
  `waitForTimeout` (use `expect(locator).toHaveValue/toBeVisible/toBeDisabled`
  or `expect.poll` with a bounded timeout instead). Backend test-mode has no
  seeded notification/review-queue data and no RPC to inject records
  directly, so every new spec mocks via ConnectRPC route interception,
  following `notifications-responsive.spec.ts`'s documented precedent.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods: happy path + error paths covered —
  `NotificationHistoryStore.Append/GetByID/enforceRetention/deduplicateExisting`,
  `ApprovalService.ResolveApproval/ResolveApprovalReconciled/IsApprovalPending`,
  `ApprovalStore.MarkHumanResolving/ClearHumanResolving/IsHumanResolving`,
  `RulesService.reconcilePendingApprovals(Safe)`,
  `DefaultStatusDeterminer.Determine/suppressedByAck`, `ReviewQueue.RemoveWithInfo`.
- All external integrations: unit mocked (lightweight `reconciliationResolver`
  fake for pure branch logic) + at least one integration test against the
  real store/service for every path above that touches persisted state.
- UX acceptance criteria: every one of the 27 criteria in `design/ux.md`
  has a corresponding Playwright test above (criterion 26 is cross-referenced
  to existing per-error-state tests rather than duplicated) — none rely on a
  manual-only verification step.
- **Regression gate (per plan's own checklist)**: `go test ./session/...
  ./server/...` must not regress `TestReviewQueue*`/`TestReviewQueuePoller*`;
  `cd web-app && npx jest --no-coverage` must not regress
  `NotificationsPage.test.tsx`, `ReviewQueue.focus.test.tsx`,
  `notification-policy.test.ts`, `notifications.test.ts`; `make quick-check`
  before opening the PR, `make ready` before pushing.
