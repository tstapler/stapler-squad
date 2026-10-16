# Architecture Review: notification-revamp
**Date**: 2026-09-04
**Verdict**: CONCERNS

## Repair Verification (Iteration 1 → 2)

All 5 items the repair pass claimed to fix were checked against the actual
current `plan.md` text (not just the repair's own summary) and are
confirmed resolved:

1. **Blocker — `GetByID` concurrent map read/write race: RESOLVED.** Task
   2.1.1b (plan.md:543-583) now specifies `GetByID` under `s.mu.RLock()`
   (matching `List`/`GetUnreadCount`'s convention — also closes the
   iteration-1 Nitpick about `Lock()` vs `RLock()` in the same fix) and
   returns a deep copy: `cp := *r` plus a freshly allocated, element-copied
   `cp.Metadata` map, so the returned pointer never aliases the store's live
   record. Verified against current `server/notifications/store.go`
   (`SetMetadata` at line 411 still takes `s.mu.Lock()` and mutates
   `r.Metadata` in place) — `RLock`/`Lock` are mutually exclusive on the same
   `sync.RWMutex`, so the race is closed, not just narrowed.
2. **Concern — `RulesService.approvalSvc` concrete type: RESOLVED.** Task
   2.1.2a (plan.md:665-692) defines `type reconciliationResolver interface {
   ListPendingApprovalsInternal() []*PendingApproval;
   ResolveApprovalReconciled(ctx context.Context, approvalID, decision,
   ruleName string) error }` and types the field against it;
   `SetApprovalService(as reconciliationResolver)` accepts the interface, not
   `*ApprovalService`. Task 2.2.2a/b/d now explicitly reserve the real
   `ApprovalService`+`ApprovalStore` fixture only for tests that need real
   persisted state, using a lightweight fake for pure branching logic —
   the testability gain iteration 1 asked for is actually exercised, not just
   declared possible.
3. **Concern — `RemovalInfo` illegal-state pairing: RESOLVED.** Task 2.3.1b
   (plan.md:1017-1066) makes `reason`/`ruleName` unexported fields on
   `RemovalInfo`, accessed only via `Reason()`/`RuleName()`, constructed only
   via `UserActionRemoval()` and `AutoResolvedByRuleRemoval(ruleName string)`.
   The `Reason()`/`RuleName()` pairing can no longer be mismatched through
   either constructor. (Go's structural typing still allows a bare
   `RemovalInfo{}` zero-value literal within the `queue` package itself — an
   inherent limitation of the unexported-field technique, not something this
   repair introduced or could close without a full sum type, and the plan
   explicitly chose the cheaper technique for exactly two call sites. Not
   re-flagging.)
4. **Concern — string-literal `reconciled` metadata checks: RESOLVED.**
   `NotificationRecord.IsReconciled()` (Task 2.1.1c, plan.md:585-601) and TS
   `isReconciledNotification()` (Task 2.3.2b, plan.md:1179-1181) are now used
   at all four read sites named in the original concern: the `ResolveApproval`
   not-found branch (plan.md:629), the `NotificationItem.tsx` badge branch
   (plan.md:1187), and both of Task 3.1.3a's `NotificationsPage.tsx` filter
   changes (plan.md:1429-1435) — no inline `metadata["reconciled"] == "true"`
   comparison remains at any of the four.
5. **Concern/Nitpick — Task 2.3.1c wrong file reference: RESOLVED.** Task
   2.3.1c (plan.md:1068-1098) now defines `ReviewQueueRemover` directly in
   `session/review_queue.go` against `*ReviewQueue`. Verified against the
   actual current file layout (`grep -n "type ReviewQueue\b"
   session/review_queue.go` → `type ReviewQueue = queue.ReviewQueue` at
   review_queue.go:44; `ReviewQueueWriter` is hand-written at
   review_queue.go:63) — the corrected instruction matches where every
   sibling single-method interface for this type actually lives today, not
   just what the plan claims.

## Blockers

None.

## Concerns

- [ ] **Task 2.1.1b (`server/notifications/store.go` /
  `server/services/approval_handler.go`) — widening the shared
  `approvalNotificationStamper` interface to add `GetByID` breaks two
  existing test doubles that satisfy it structurally, with no task in the
  plan updating them.** `approvalNotificationStamper` (`approval_handler.go:52-55`)
  is the interface type behind *both* `ApprovalHandler.notificationStamper`
  and `ApprovalService.notificationStore` (`approval_service.go:26`) — it is
  shared, not private to the one call site Task 2.1.1c touches. Two existing
  fakes satisfy it today by implementing only `SetMetadata`/`MarkRead`:
  `spyStamper` (`approval_handler_integration_test.go:631-644`, passed to
  `h.SetNotificationStamper(spy)` at lines 747/812/854) and
  `spyNotificationStore` (`approval_service_test.go:449-463`, passed to
  `svc.SetNotificationStore(spy)` at line 472). Once `GetByID` is added to
  the interface, both fakes stop satisfying it — `go build`/`go test` for
  `server/services` fails to compile with "does not implement
  approvalNotificationStamper (missing method GetByID)" at every one of
  those five call sites, the moment Task 2.1.1b is implemented as written.
  Task 2.1.1d ("Add tests for the new `ApprovalService` methods") compounds
  this silently: its `TestResolveApproval_AfterReconciled_ReturnsFailedPrecondition`
  needs `GetByID` to return a record whose `IsReconciled()` is `true` in
  order to exercise the not-found-but-reconciled branch at all — but nothing
  in the task instructs adding that behavior to `spyNotificationStore`, so
  even after fixing the compile break with a stub, the stub as a bare stub
  wouldn't make that specific test meaningful. Contrast with Task 2.3.1b/c's
  `ReviewQueueObserver.OnItemRemoved` signature change, which explicitly
  calls out updating its own existing test double
  (`session/review_queue_test.go`'s `testObserver.OnItemRemoved`,
  plan.md:1096-1097) — the same discipline needed to be applied to this
  interface widening one epic earlier and wasn't.
  - **Remediation**: extend Task 2.1.1b (or add a new sub-bullet to it) to:
    (1) add a `GetByID(id string) (*notifications.NotificationRecord, bool)`
    stub to `spyStamper` (can be a fixed `return nil, false` — `ApprovalHandler`
    itself never calls `GetByID`, only `ApprovalService` does, so the stub
    only needs to satisfy the interface, not do anything real); and (2) add a
    stateful `GetByID` to `spyNotificationStore` that returns a record
    reflecting the `SetMetadata` calls already recorded in `callLog` (or a
    small settable field) so Task 2.1.1d's reconciled-race test can actually
    assert against it, matching the fixture-reuse pattern Task 2.2.2a already
    established elsewhere in this plan (reserve the real store only where a
    test needs real persisted state; a stateful fake is sufficient here since
    this is a unit test of `ApprovalService`'s own not-found branch, not an
    integration test of `NotificationHistoryStore`).

## Nitpicks

- ADR-004's fire-and-forget `go rs.reconcilePendingApprovals()` per rebuild
  still has no upper bound on concurrently in-flight reconciliation passes.
  Unchanged since iteration 1 — still an accepted, explicitly-reasoned
  trade-off at this project's single-operator Appetite, not a new issue from
  the repair pass. Flagging only so it's revisited if this ever serves more
  than one concurrent operator.
- The corrected Task 2.3.1c placement (`session/review_queue.go`, not
  `queue.go`) is also internally consistent with the plan's own re-export
  lines immediately below it (`type RemovalInfo = queue.RemovalInfo`, `var
  UserActionRemoval = queue.UserActionRemoval`) — confirmed there's no
  leftover reference anywhere else in the plan (Domain Glossary, Pattern
  Decisions table) still pointing at the old, incorrect `queue.go` location.
