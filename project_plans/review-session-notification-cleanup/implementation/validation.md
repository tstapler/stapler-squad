# Validation Plan: review-session-notification-cleanup

**Date**: 2026-07-25

## Happy Path Scenario

Given a `Hidden` review session (`Instance{Title: "review:153f8eac", Hidden: true}` spawned by
`SpawnReviewSession`, backed by an `ItemSession{SessionRole: "review"}` linked to backlog item
`153f8eac-c454-4fa3-a8f4-83b070b9a035`), when it reaches `TASK_COMPLETE` (or goes `Idle`/`Stale`),
then zero `events.EventNotification`s are published and the Notifications page gains no entry —
while a real, non-`Hidden` backlog-linked work session reaching the identical detected condition
produces exactly one notification, correctly stamped with `metadata["item_id"] =
"153f8eac-c454-4fa3-a8f4-83b070b9a035"` and `metadata["session_scoped"] = "true"`, so the frontend
routes it to "View in Backlog" instead of a dead "View Session" link.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1: Hidden/`review`/`triage` sessions no longer generate generic TASK_COMPLETE/Idle/Stale notifications (narrowed, reason-scoped per revised Design Decision 1) | `session/review_queue_determiner_test.go` | `TestDetermine_ReturnsSkip_When_InstanceHiddenAndReasonIsTaskCompleteIdleOrStale` (Task 1.1.1b) | Unit | Happy path — `Hidden: true` inst reaching `ReasonTaskComplete`/`ReasonIdle`/`ReasonStale` returns `DetectionActionSkip` |
| AC1 (cont., error-class safety net) | `session/review_queue_determiner_test.go` | Same test's `ReasonErrorState` case (Task 1.1.1b) | Unit | Error/negative path — a `Hidden` instance reaching `ReasonErrorState` (or `ReasonTestsFailing`) still returns `DetectionActionAdd` — proves the narrowing does not over-suppress the one reason class with no other durable stuck-detector (pre-mortem Failure #3) |
| AC1 (cont.) | `session/review_queue_determiner_test.go` | Existing `Determine()` tests for non-Hidden instances (approval/error/idle detection) | Unit | Error/negative path — confirms the narrowed gate does not regress any existing non-Hidden detection branch; re-run as-is, no new assertions needed |
| AC1 (cont., no source change, regression coverage only) | `session/startup_scanner_test.go` | `TestScan_SkipsHiddenInstance_ForSuppressedReasonsOnly` (Task 1.2.1a) | Unit | Happy path + error path — `Hidden: true` instance with `ReasonTaskComplete`-shaped status never reaches `queue.Add`; the same instance with `ReasonErrorState`-shaped status *does* reach `queue.Add` — proves `Scan` delegates to `Determine()`'s narrowed gate rather than reintroducing a separate, reason-blind skip (Design Decision 5, revised: the originally-planned `StartupScanner.Scan` source change was dropped) |
| AC1 (cont., defense-in-depth at publish) | `server/review_queue_manager_test.go` | `TestOnItemAdded_SuppressesNotification_When_SessionHidden` | Unit | Happy path — `Hidden: true` instance resolved via `poller.SetInstances`, `ReasonTaskComplete` `ReviewItem` → `mgr.OnItemAdded` never calls `rqm.eventBus.Publish` (bounded 300ms `select`) |
| AC1 (cont., all three reasons) | `server/review_queue_manager_test.go` | `TestOnItemAdded_SuppressesNotification_When_SessionHidden` (table/`t.Run` subtests per Task 2.3.1b) | Unit | Happy path — same suppression asserted for `ReasonIdle` and `ReasonStale`, not just `ReasonTaskComplete` |
| AC1 (cont., negative control / error path) | `server/review_queue_manager_test.go` | `TestOnItemAdded_PublishesNotification_When_SessionNotHidden_EvenIfBacklogLinked` | Unit | Error/negative path — `Hidden: false` instance still produces a notification (proves the suppression predicate doesn't over-fire and would catch a regression that swallows real sessions) |
| AC1 (cont., integration — real `ItemSession` row) | `server/review_queue_manager_test.go` | `TestOnItemAdded_SuppressesNotification_When_SessionHidden` (via `newReactiveQueueTestSetupWithStorage`) | Integration | Happy path — uses a real ent-backed `*session.Storage` with a created backlog item + `storage.CreateItemSession(ctx, session.ItemSessionData{SessionRole: "review", ...})`, proving suppression holds through the actual `GetItemSessionBySessionUUID` lookup, not a mocked storage |
| AC1 (cont., second notifier) | `server/services/autonomous_orchestration_service_test.go` | `TestOnAutonomousDriverComplete_SuppressesGenericNotification_When_InstanceHidden` | Unit | Happy path — `Hidden: true` `Instance` on the `AutonomousDriver`-run path means the generic done/stuck `a.bus.Publish` is never reached |
| AC2: notifications tied to a backlog-linked session include `item_id` metadata | `server/review_queue_manager_test.go` | `TestOnItemAdded_PublishesNotification_When_SessionNotHidden_EvenIfBacklogLinked` | Unit | Happy path — non-Hidden, backlog-linked session's published event carries `metadata["item_id"] == "153f8eac-c454-4fa3-a8f4-83b070b9a035"` and `metadata["session_scoped"] == "true"` |
| AC2 (cont., error path — lookup failure) | `server/review_queue_manager_test.go` | `TestOnItemAdded_NotificationUsesStableID` / `TestOnItemAdded_NotificationFallsBackToTitleWhenNoMatch` (Task 2.2.1b, verification-only) | Unit | Error path — `rqm.storage == nil` (no linkage lookup possible) or lookup returns `ErrNotFound`: no `item_id` is stamped, no `Warn` log, and the existing assertions on `e.SessionID` continue to pass unmodified |
| AC2 (cont., second notifier) | `server/services/autonomous_orchestration_service_test.go` | `TestOnAutonomousDriverComplete_StampsItemID_When_TriageStuck` (+ non-Hidden positive-metadata case for the generic notifier, Task 3.3.1a) | Unit | Happy path — "Triage stuck" notice's metadata becomes `{"item_id": item.ID}` (was `nil`); generic notifier's metadata becomes `events.SessionScopedMetadata(nil, linkedItemID)` for a non-Hidden backlog-linked session |
| AC2 (cont., integration — real storage lookup) | `server/review_queue_manager_test.go` | `TestOnItemAdded_SuppressesNotification_When_SessionHidden` / `TestOnItemAdded_PublishesNotification_When_SessionNotHidden_EvenIfBacklogLinked` (via `newReactiveQueueTestSetupWithStorage`) | Integration | Happy path — `item_id` enrichment resolved through the real `*session.Storage`-backed `GetItemSessionBySessionUUID` call, not a mock, confirming the `ItemSession.session_uuid` → `backlog_item` edge traversal actually works end to end |
| AC3: notifications referencing a gone session/instance are pruned, not left for 7 days | `server/notifications/store_test.go` | `TestPruneOrphaned_RemovesEligibleRecord_KeepsItemLinkedAndNonSessionScoped` | Unit | Happy path — a `SessionScoped: true`, no-`item_id` record whose `SessionID` is absent from a stub `existingSessionIDs()` set is removed (count `1`); a second `SessionScoped: true` record *with* `item_id` is kept; a third `SessionScoped: false` record with the same dead `SessionID` is kept (SessionID-overload trap) — the stub is asserted to have been called exactly once |
| AC3 (cont., error/defensive path — nil sentinel) | `server/notifications/store_test.go` | `TestPruneOrphaned_PrunesNothing_When_ExistingSessionIDsReturnsNil` | Unit | Error/defensive path — `existingSessionIDs` returning `nil` (the `pruneOrphanedMinUptime`-not-elapsed or batch-fetch-failure case) prunes zero records regardless of eligibility, proving the `nil`-vs-empty-map sentinel distinction holds |
| AC3 (cont., cadence gating) | `server/notifications/store_test.go` | `TestEnforceRetention_GatesOrphanSweep_ByOrphanPruneInterval` | Unit | Error/regression-guard path — two `Append()` calls within `orphanPruneInterval` invoke the existence-check stub at most once; advancing past the interval and calling `Append()` again triggers a second invocation — proves the sweep is decoupled from firing on every `Append()` |
| AC3 (cont., `SessionScoped` wiring) | `server/notifications/subscriber_test.go` | `eventToRecord` `SessionScoped` tests (Task 4.4.1d — positive: `metadata[events.MetadataKeySessionScoped] == "true"` → `SessionScoped == true`; negative: key absent → `SessionScoped == false`) | Unit | Happy path (positive case) + error/negative path (key absent, e.g. `backlog_notifier.go`'s `EventBusNotifier.Notify` events) |
| AC3 (cont., integration — real instance-store existence check) | `server/server_test.go` or manual/no dedicated integration test called out in plan.md; covered functionally by `Task 4.3.1a`'s Given-When-Then | Integration | N/A — plan.md does not specify a dedicated `server/server_test.go` test for the wired `SetSessionExistenceLookup` closure itself (uptime gate + `ListInstanceData()` set-building); the closure's two behaviors (uptime gate returns `nil`; post-uptime returns a set keyed by stable ID + title) are stated as acceptance criteria in Story 4.3 but not assigned an explicit task-level test name — **flagged as a minor gap below** | — |
| AC4: regression test confirms a headless review session completing/going stale produces no Notifications-page entry | `server/review_queue_manager_test.go` | `TestOnItemAdded_SuppressesNotification_When_SessionHidden` (Tasks 2.3.1a/2.3.1b/2.3.1c, using `newReactiveQueueTestSetupWithStorage`) | Unit + Integration | Happy path — this *is* AC1's defense-in-depth test, reused as AC4's primary regression proof per the plan's own cross-reference ("Story 2.3: AC4 regression tests (primary)"); asserts zero `events.EventNotification` on the bus for `ReasonTaskComplete`/`ReasonIdle`/`ReasonStale` against a `Hidden` review session with a real created backlog item + `ItemSession` row |
| AC4 (cont., negative control proving the harness itself works) | `server/review_queue_manager_test.go` | `TestOnItemAdded_PublishesNotification_When_SessionNotHidden_EvenIfBacklogLinked` | Unit | Error/negative path — same harness with `Hidden: false` *does* produce a notification, proving a suppression regression (over- or under-firing) would be caught by this test suite either direction |
| AC4 (cont., error-class safety net, via `OnItemAdded`) | `server/review_queue_manager_test.go` | Task 2.3.1d's `ReasonErrorState`/`ReasonTestsFailing` narrowing case | Unit | Error/negative path — a `Hidden: true` instance with `Reason: ReasonErrorState` still produces a notification within 300ms via `OnItemAdded`, mirroring Task 1.1.1b's `Determine()`-level safety-net case one layer up |
| AC4 (cont., "review/triage" wording — headless-triage negative proof, Finding 3) | `server/services/backlog_service_triage_test.go` (or equivalent existing test file for `TriggerTriage`) | `TestTriggerTriage_NeverPublishesUntaggedNotification_OnHeadlessPoolFailureOrSuccess` (Task 2.4.1a, Story 2.4) | Unit | Happy path + error path — three sub-cases (LLM error, parse failure, persist failure) against a `fakeHeadlessPool` fixture, asserting `TriggerTriage`'s async goroutine never publishes an untagged generic notification for any outcome — the provable negative closing AC4's "review/triage" wording for the headless-triage path, which structurally never constructs a `session.Instance` (features.md/architecture.md) |
| AC1/AC2 (cont., third notification producer, Finding 1) | `server/services/session_service_test.go` (or equivalent existing test file for `wireRateLimitCallbacks`) | `TestWireRateLimitCallbacks_SuppressesNotification_When_InstanceHidden` (+ metadata test, Task 5.2.1a) | Unit | Happy path — a `Hidden: true` instance's rate-limit detected/recovery events are suppressed entirely; a non-`Hidden`, backlog-linked instance's rate-limit notification carries `item_id`/`session_scoped` metadata via `events.SessionScopedMetadata` (previously `nil`) |

## UX Acceptance Tests

N/A — no user-facing surface (backend-only fix, confirmed by `research/ux.md` —
`NotificationsPage.tsx`'s metadata-driven "View in Backlog" vs. "View Session" link routing
(`web-app/src/app/notifications/NotificationsPage.tsx:377-393`) already prefers
`notification.metadata["item_id"]` when present; this plan only changes which backend metadata
producers populate that key and which producers are suppressed, not the frontend routing logic
itself). There is no `project_plans/review-session-notification-cleanup/design/ux.md` file, per
Phase 2's `research/ux.md` scoping this as backend-only.

## Test Stack

- **Unit**: Go stdlib `testing`, no testify (per repo convention confirmed in Phase 2 `stack.md`
  research)
- **Integration**: Go stdlib `testing` with real ent-backed `*session.Storage` test fixtures
  (`newReactiveQueueTestSetupWithStorage`, `server/review_queue_manager_test.go:777`), used for
  the AC1/AC2/AC4 tests that need a real `ItemSession` row created via `storage.CreateItemSession`
  and resolved via the real `GetItemSessionBySessionUUID` query path
- **E2E / UX**: N/A

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- All public/exported methods touched by this plan (`PruneOrphaned`, `SetSessionExistenceLookup`,
  `SessionScopedMetadata`, `Determine`, `OnItemAdded`, `onAutonomousDriverComplete`): happy path +
  error paths covered
- All external integrations (ent-backed `GetItemSessionBySessionUUID`, `ListInstanceData`): unit +
  at least one integration-style test using real storage fixtures
  (`newReactiveQueueTestSetupWithStorage` for `GetItemSessionBySessionUUID`; no dedicated
  integration test exercises `ListInstanceData()` through the actual wired
  `SetSessionExistenceLookup` closure in `server/server.go` — see Coverage Gap Notes below)

## Coverage Gap Notes (validator's own findings, not blocking)

- **AC3's `server.go` wiring (Story 4.3/Task 4.3.1a) has acceptance criteria but no named test
  task.** Every other story in the plan has an explicit `Task N.N.1x: Add Test<Name>` entry;
  Story 4.3 states two Given-When-Then examples (uptime-gate-returns-nil;
  past-uptime-returns-stable-ID-and-title set) but the plan's task list jumps straight from Task
  4.3.1a (implementation) to Story 4.4's tests, none of which target `server/server.go` or
  `wireDepsIntoServer` directly — Story 4.4's tests all live in
  `server/notifications/store_test.go`/`subscriber_test.go` and test `PruneOrphaned` /
  `pruneOrphanedRecords` / `eventToRecord` against a stubbed `existingSessionIDs` closure, not the
  real one built in `server.go`. This is a real coverage gap for AC3's specific "wired against
  `ListInstanceData()` with the uptime guard" behavior, though the underlying logic
  (`pruneOrphanedRecords`'s `nil`-sentinel handling, ID/title-set construction pattern) is
  exercised indirectly via `TestPruneOrphaned_PrunesNothing_When_ExistingSessionIDsReturnsNil` and
  the plan's own code comments. Recommend a follow-up unit test in a new or existing
  `server/server_test.go` asserting: (a) the closure returns `nil` before `pruneOrphanedMinUptime`
  elapses without calling `storage.ListInstanceData()`, and (b) after that window it returns a set
  containing both `GetStableID()` and `Title` for each stored instance. Not a blocker for
  implementation — the four ACs are otherwise fully covered — but flagging so it isn't silently
  dropped.
- All 4 acceptance criteria have at least one happy-path and one error/negative-path test per the
  mapping table above; AC3 additionally has three distinct negative/defensive tests (nil-sentinel,
  item-linked-kept, non-session-scoped-kept) reflecting its higher risk profile (accidental mass
  deletion) called out in the plan's own Risk Control table.
