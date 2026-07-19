# Validation Plan: backlog-stuck-item-visibility

**Date**: 2026-07-14

Designed test-first, before any implementation code. Type/method names use the plan.md
Domain Glossary verbatim (`BacklogStuckState`, `StuckReason`, `MarkStuck`, `ResolveStuck`,
`FindOpenStuckStates`, `MarkStuckNotified`, `stuckPRReady`, `abandonedReview`, `isBouncing`,
`CountReviewCyclesSince`, six reasons: `pr_ready_unmerged`, `rework_cap`, `abandoned_review`,
`stale_work`, `bouncing`, `push_failed`). Go test names follow the confirmed house style
`TestSubject_should_Behavior_When_Condition` (e.g. `TestSweeper_sweepResourcePressure_should_hibernateOnlyOne_When_MultipleEligible`).
ent repository tests use `NewEntRepository(WithDatabasePath(dbPath))`; restart-survival tests
mirror `TestEntRepository_UUID_SurvivesDBReopen` (open → close → reopen from the same file).

## Happy Path Scenario

**Given** the running instance has item `f9fcef32-c27e-434d-b23f-c873c18afa92` in status
`pr_pending` whose PR #148 is green + mergeable + no changes requested + not draft +
**unapproved** (`ApprovedCount == 0` — Tyler cannot self-approve his own PR, so
`prReadyToMergeSolo(info)` is true even though `github.DerivePRPriority(info)` is NOT
`PRPriorityReady`; see plan.md/ADR-001 pre-mortem F1), unmerged, and has held that state
for more than 30 minutes across reconcile ticks — **when** the 60s `ReconcileStuck` tick runs, opens a durable
`BacklogStuckState` row (`reason=pr_ready_unmerged`, `first_detected_at` persisted, one
notification fired, `notified_at` set), the service restarts, and the user opens
`/unfinished` — **then** the `StuckNavBadge` shows a non-zero count, the "Stuck Backlog Items"
section lists item `f9fcef32-…` under the "PR ready to merge" group with the correct
color+text chip and a `first_detected_at`-sourced "stuck 3d" duration that survived the
restart (not process-uptime), expanding the card reveals the "why" context + a 1-click PR
link, and no duplicate notification fires on subsequent ticks while the PR stays ready.

This anchors every test below: a stuck condition is **detected durably**, **notified once**,
**survives restart**, **surfaces in the UI with the right reason and a persisted duration**,
and **resolves cleanly** when the condition clears.

## Requirement → Test Mapping

Coverage is designed per the six `StuckReason` classes (unit-happy + unit-error + integration
each) and per cross-cutting In-Scope requirement.

### StuckReason class: `pr_ready_unmerged` (root cause #1)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Green-PR threshold arithmetic | `session/stuck_decisions_test.go` | `TestStuckPRReady_should_returnTrue_When_GreenMergeablePast30Min` | Unit (happy) | `stuckPRReady(firstDetected=now-31m, now)` → true; table-driven, no DB. |
| Threshold boundary / not-yet | `session/stuck_decisions_test.go` | `TestStuckPRReady_should_returnFalse_When_WithinThreshold` | Unit (error) | Exactly 30m and 29m → false (no premature flag). |
| Solo readiness flags an UNAPPROVED green PR (pre-mortem F1) | `session/stuck_decisions_test.go` | `TestPrReadyToMergeSolo_should_returnTrue_When_GreenMergeableUnapproved` | Unit (happy) | `prReadyToMergeSolo` on a not-draft, `ChangesRequestedCount==0`, `CheckConclusion∈{success,""}`, `Mergeable=="MERGEABLE"`, `ApprovedCount==0` PR → **true** (the flagship single-user case; `PRPriorityReady` would return false here — this test locks in that the approval gate is intentionally dropped). |
| Solo readiness still rejects genuinely-blocked PRs | `session/stuck_decisions_test.go` | `TestPrReadyToMergeSolo_should_returnFalse_When_BlockedOrConflictingOrFailing` | Unit (error) | `ChangesRequestedCount>0` / `CheckConclusion=="failure"` / `IsDraft` / `Mergeable=="CONFLICTING"` each → false (keeps every real block `DerivePRPriority` enforces; no false-positive). |
| Durable detect + notify-once on an unapproved green PR | `session/backlog_lifecycle_stuck_test.go` | `TestReconcilePRPending_should_markStuck_When_PRGreenMergeableUnapproved` | Integration (ent) | `prReadyToMergeSolo(info)` true (green, mergeable, **zero approvals**), unmerged >30m → one open `pr_ready_unmerged` row + exactly one notification; second tick fires none; asserts NO new `GetPRStatus`/`gh` call and that `DerivePRPriority`/`PRPriorityReady` is NOT the gate (regression guard for pre-mortem F1 — the flagship PR #148 must surface despite no approval). |

### StuckReason class: `rework_cap` (root cause #2)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Immediate mark on cap hit | `server/services/backlog_service_triage_test.go` | `TestNotifyReworkCapHit_should_markStuckReworkCapImmediately_When_CapHit` | Unit (happy) | `workCount>=maxAutoReworkIterations` → `MarkStuck(StuckReasonReworkCap)` called with cap-describing `context` (threshold 0). |
| Failure isolation | `server/services/backlog_service_triage_test.go` | `TestNotifyReworkCapHit_should_stillPublishNotification_When_MarkStuckReturnsError` | Unit (error) | `MarkStuck` errors → notification still published, no panic (durable write is additive, not a gate). |
| Restart-surviving persistence | `server/services/backlog_service_triage_test.go` | `TestNotifyReworkCapHit_should_persistRowSurvivingRestart_When_CapHit` | Integration (ent) | Cap hit → row present; reopen repo from same DB file → `FindOpenStuckStates` still returns it. |

### StuckReason class: `abandoned_review` (root cause #3)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| 15-min grace arithmetic | `session/stuck_decisions_test.go` | `TestAbandonedReview_should_returnTrue_When_LastReviewOlderThan15Min` | Unit (happy) | `abandonedReview(lastReviewAt=now-16m, now)` → true. |
| Grace boundary | `session/stuck_decisions_test.go` | `TestAbandonedReview_should_returnFalse_When_WithinGrace` | Unit (error) | 15m and 5m → false (one+ tick to re-spawn review gate). |
| DB-backed dedup survives restart | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileStuckReviewItems_should_writeDurableRowAndNotDedupResetAcrossRestart_When_ReviewParkedPast15Min` | Integration (ent) | Row + notify once; reopen DB → same tick does NOT re-notify (reads `notified_at` from DB, not a fresh empty map). |
| **Zombie-session review item is flagged (pre-mortem F3)** | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileStuckReviewItems_should_markAbandoned_When_OnlyActiveSessionIsDeadZombie` | Integration (ent) | `review` item whose only `EndedAt IS NULL` session has an underlying tmux/CLI session that reports **not alive** (injected liveness stub returning false) → open `abandoned_review` row written + notified once. Guards the gap where `FindStuckReviewItems` (`storage_backlog.go:519-527`) excludes any item with an un-ended session, silently leaving zombie-session items stuck-forever; asserts the existing `DoesSessionExist`/`TmuxSessionExists` liveness helper is reused (no new mechanism). |
| Zombie detector does NOT flag a genuinely-live review session | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileStuckReviewItems_should_notMarkAbandoned_When_ActiveSessionStillAlive` | Integration (ent) | `review` item with an `EndedAt IS NULL` session whose liveness stub returns **true** → no `abandoned_review` row (a real in-flight review is not a false-positive). |

### StuckReason class: `stale_work` (root cause #3)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| 2h staleness reuse | `session/stuck_decisions_test.go` | `TestStaleWork_should_returnTrue_When_LastProgressOlderThan2h` | Unit (happy) | Reuses existing `maxWorkSessionStaleness=2h`; no new constant introduced. |
| Fresh session not flagged | `session/stuck_decisions_test.go` | `TestStaleWork_should_returnFalse_When_ProgressWithin2h` | Unit (error) | `LastProgressAt=now-1h` → false. |
| Durable stale_work row | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileStaleWorkSessions_should_writeDurableStaleWorkRow_When_ActiveSessionStale` | Integration (ent) | `in_progress` item, session `LastProgressAt>2h` → open `stale_work` row, notify-once DB-backed. |

### StuckReason class: `bouncing` (root cause #4 — the flagged rabbit hole)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Cycle threshold (≥3, no PASS) | `session/stuck_decisions_test.go` | `TestIsBouncing_should_returnTrue_When_ThreeCyclesNoPass` | Unit (happy) | `isBouncing(3, hasPass=false)` → true. |
| Below threshold / PASS present | `session/stuck_decisions_test.go` | `TestIsBouncing_should_returnFalse_When_TwoCyclesOrHasPass` | Unit (error) | `isBouncing(2,false)` and `isBouncing(3,true)` → false (both branches). |
| Cycle-count query correctness | `session/backlog_lifecycle_stuck_test.go` | `TestCountReviewCyclesSince_should_countInProgressToReviewTransitions_When_WithinWindow` | Integration (ent) | Seeds `BacklogStatusEvent`s; counts only `from=in_progress,to=review` inside 24h window. |
| End-to-end bouncing detect | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileBouncingItems_should_writeBouncingRowNotifyOnce_When_ThreeCyclesIn24hNoPass` | Integration (ent) | Item `df0d5872` with ≥3 cycles/24h, no PASS → open `bouncing` row w/ cycle-count `context`, notified once. |

### StuckReason class: `push_failed` (root cause: push/PR-create failure — 6th reason)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Mark at failure site | `session/backlog_lifecycle_stuck_test.go` | `TestStayInReviewAndNotify_should_markPushFailedRow_When_PushAndCreatePRFails` | Unit (happy) | `pushAndCreatePR` fails → `MarkStuck(StuckReasonPushFailed)` + `MarkStuckNotified` alongside the existing ERROR notification. |
| Existing toast still fires | `session/backlog_lifecycle_stuck_test.go` | `TestStayInReviewAndNotify_should_stillFireErrorNotification_When_MarkStuckFails` | Unit (error) | Durable write errors → `NOTIFICATION_TYPE_ERROR` still published (additive). |
| Restart-surviving + invisible-to-PrNumberGT(0) case | `session/backlog_lifecycle_stuck_test.go` | `TestPushFailed_should_persistRowSurvivingRestart_When_ItemHasNoPrNumber` | Integration (ent) | Item left with no `pr_number` → open `push_failed` row survives DB reopen (surfaces what `FindPRPendingItems`' `PrNumberGT(0)` filter hides). |

### Cross-cutting In-Scope requirements

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Durable state survives restart (root cause #3, hard requirement) | `session/ent_repository_backlog_stuck_test.go` | `TestFindOpenStuckStates_should_returnRowsAfterDBReopen_When_ProcessRestarts` | Integration (ent) | Mirrors `TestEntRepository_UUID_SurvivesDBReopen`: open row, close repo, reopen from same file → row still open, `first_detected_at` preserved. |
| `StuckReason` validated-at-boundary enum | `session/domain/backlog_test.go` | `TestStuckReason_IsValid_should_returnTrueForKnown_And_FalseForUnknown` | Unit | Six constants → true; `StuckReason("banana")` → false. |
| `MarkStuck` fresh insert | `session/ent_repository_backlog_stuck_test.go` | `TestMarkStuck_should_insertOpenRowWithNullNotified_When_NoExistingRow` | Integration (ent) | One row, `first_detected_at`/`last_checked_at`/`context` set, `notified_at` null, `resolved_at` null. |
| `MarkStuck` no-dup / no first_detected reset | `session/ent_repository_backlog_stuck_test.go` | `TestMarkStuck_should_updateLastCheckedOnly_When_OpenRowExists` | Integration (ent) | Second `MarkStuck` on open pair → same single row, `last_checked_at` advanced, `first_detected_at` unchanged (plain 2-col unique index guarantees no dup). |
| `MarkStuck` resolve-in-place reopen | `session/ent_repository_backlog_stuck_test.go` | `TestMarkStuck_should_reopenRowInPlace_When_ExistingRowResolved` | Integration (ent) | Resolved pair re-marked → SAME row: `resolved_at`+`notified_at` cleared, `first_detected_at` reset; still exactly one row. |
| `MarkStuck` best-effort precondition | `session/ent_repository_backlog_stuck_test.go` | `TestMarkStuck_should_returnAppliedFalse_When_StatusPreconditionMismatch` | Integration (error) | `expectedStatus` != current status → `(applied=false, nil)`, no write, no error. |
| `ResolveStuck` atomic set | `session/ent_repository_backlog_stuck_test.go` | `TestResolveStuck_should_setResolvedAtOnce_When_RowOpen` | Integration (ent) | Single `UPDATE … WHERE resolved_at IS NULL`, row drops from `FindOpenStuckStates`, affected-rows=1. |
| `ResolveStuck` idempotent | `session/ent_repository_backlog_stuck_test.go` | `TestResolveStuck_should_beNoOpAndNotOverwrite_When_AlreadyResolved` | Integration (error) | Second call → affected-rows=0, no error, original `resolved_at` unchanged. |
| `FindOpenStuckStates` projection filter | `session/ent_repository_backlog_stuck_test.go` | `TestFindOpenStuckStates_should_excludeResolvedAndSnoozed_When_Queried` | Integration (ent) | 6 open, 1 snoozed-until-tomorrow, 1 resolved → returns 5 projected rows carrying title/status/pr_number/pr_url. |
| `MarkStuckNotified` durable dedup key | `session/ent_repository_backlog_stuck_test.go` | `TestMarkStuckNotified_should_setNotifiedAt_When_Null` | Integration (ent) | Sets `notified_at=now` only where null. |
| Backfill seeds DB-derivable reasons silently | `session/backlog_lifecycle_stuck_test.go` | `TestBackfillStuckStates_should_seedDBDerivableRowsWithNotifiedAt_When_ItemsParked` | Integration (ent) | 6 parked items → open rows for `rework_cap`/`abandoned_review`/`stale_work`/`bouncing`/`push_failed` with `notified_at` pre-set; first tick issues zero new notifications. |
| Backfill excludes `pr_ready_unmerged` / no GitHub call | `session/backlog_lifecycle_stuck_test.go` | `TestBackfillStuckStates_should_notCallGitHubNorSeedPRReady_When_Run` | Integration (ent) | Injected GitHub client asserts zero `GetPRStatus`/`IsPRMerged` calls; no `pr_ready_unmerged` row seeded. |
| Backfill idempotent | `session/backlog_lifecycle_stuck_test.go` | `TestBackfillStuckStates_should_beIdempotent_When_RunTwice` | Integration (ent) | Second run → no duplicate rows (unique-constraint guarded). |
| In-memory notify maps removed (root cause #3 fix) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcile_should_dedupNotificationsViaDB_When_MapsRemoved` | Integration (ent) | Behavioral equivalence: notify-once holds across a simulated restart with no `staleWorkNotified`/`stuckReviewNotified` fields present. |
| Proto reason mapping never panics | `server/services/backlog_stuck_rpc_test.go` | `TestToProtoStuckReason_should_mapToUnspecified_When_UnknownString` | Unit | Unknown DB string → `STUCK_REASON_UNSPECIFIED`, six known → their enum values. |
| `ListStuckBacklogItems` handler | `server/services/backlog_stuck_rpc_test.go` | `TestListStuckBacklogItems_should_returnMappedItems_When_OpenRowsExist` | Integration | Seeded `f9fcef32-…` green-PR row → response carries `reason=STUCK_REASON_PR_READY_UNMERGED`, `pr_number=148`, ~3d `first_detected_at`. |
| `SnoozeStuckItem` handler | `server/services/backlog_stuck_rpc_test.go` | `TestSnoozeStuckItem_should_setSnoozedUntilAndOmitFromList_When_Called` | Integration | Snooze `96cc9eaa` rework_cap until tomorrow → next `ListStuckBacklogItems` omits it. |
| Feature-registry entries present | (CI gate) | `make registry-generate` + `coverage-gaps.json` non-growth | Manual/CI | `docs/registry/features/backend/backlog-list-stuck.json` (+snooze) and `frontend/backlog-stuck-items.json` exist with `markerFound:true`; gaps count does not increase. |

### ResolveStuck wiring, self-heal sweep, panic isolation (Story 2.1.5)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Resolve on merge→done | `session/backlog_lifecycle_stuck_test.go` | `TestReconcilePRPending_should_resolvePRReadyRow_When_PRMerged` | Integration (ent) | Merge transition → `ResolveStuck(pr_ready_unmerged)` in same path, immediate. |
| Resolve on manual re-review | `server/services/backlog_service_test.go` | `TestTriggerReReview_should_resolveReworkCapAndAbandonedRows_When_ItemMovesToInProgress` | Integration | Both rows resolved on transition. |
| Resolve-in-place reopen (single row invariant) | `session/ent_repository_backlog_stuck_test.go` | `TestMarkStuck_should_keepExactlyOneRow_When_ResolvedThenRecurs` | Integration (ent) | Bouncing resolved then recurs → same row reopened, never a 2nd row per `(item_id,reason)`. |
| **C1** — self-heal anchored-reason resolve | `session/backlog_lifecycle_stuck_test.go` | `TestSelfHealSweep_should_resolveAnchoredRow_When_ItemStatusInconsistentWithReason` | Integration (ent) | Phantom `stale_work` row on a `done` item → resolved next tick. |
| **C1** — bouncing spans a status set, not one anchor | `session/backlog_lifecycle_stuck_test.go` | `TestSelfHealSweep_should_notResolveBouncingRow_When_ItemInInProgressHealthyHalfCycle` | Integration (ent) | Valid `bouncing` row + item in `in_progress` (bouncing's own half-cycle) → row NOT resolved (guards C1's over-eager map). |
| **C1** — bouncing resolves only on terminal/PASS | `session/backlog_lifecycle_stuck_test.go` | `TestSelfHealSweep_should_resolveBouncingRow_When_ItemReachesDoneOrPass` | Integration (ent) | Item moves to `done` (or PASS verdict) → `bouncing` row resolved. |
| **C1** — event-shaped reasons excluded from status sweep | `session/backlog_lifecycle_stuck_test.go` | `TestSelfHealSweep_should_notResolveEventShapedRows_When_StatusVaries` | Integration (ent) | `rework_cap`/`push_failed` rows (written `expectedStatus=<current>`, no fixed anchor) are NOT resolved by the status sweep; they rely on their event-site `ResolveStuck` only. |
| **C2** — pr_ready clears without status change | `session/backlog_lifecycle_stuck_test.go` | `TestReconcilePRPending_should_resolvePRReadyRow_When_NewCommitClearsReadinessWhileStillPrPending` | Integration (ent) | PR gets a new commit → `prReadyToMergeSolo(info)` false (CI re-running / conflict) while item stays `pr_pending` → detector's explicit else-branch calls `ResolveStuck` (sweep structurally can't see this). |
| **C2** — stale_work clears without status change | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileStaleWorkSessions_should_resolveStaleWorkRow_When_SessionResumesWhileStillInProgress` | Integration (ent) | Session resumes reporting progress, item still `in_progress` → detector else-branch resolves the row. |
| **C2** — abandoned_review clears without status change | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileStuckReviewItems_should_resolveAbandonedRow_When_ReviewGateBackInFlightWhileStillReview` | Integration (ent) | Review activity resumes while still `review` → else-branch resolves. |
| Self-heal backstops missed un-stick site / race | `session/backlog_lifecycle_stuck_test.go` | `TestSelfHealSweep_should_resolvePhantomRow_When_WriteRacedTransitionToDone` | Integration (ent) | Stale row written after a racing transition → resolved within one tick (never a permanent phantom). |
| Per-detector panic isolation | `session/backlog_lifecycle_stuck_test.go` | `TestReconcileStuck_should_completeMergeDetection_When_BouncingDetectorPanics` | Integration (ent) | Injected panic in `reconcileBouncingItems` → merge detection + other reconcilers still complete (detector-local `recover()`). |
| Pure-fn delegation (no inlined arithmetic) | `session/backlog_lifecycle_stuck_test.go` | `TestReconcilers_should_delegateThresholdDecisionsToPureFns_When_Reviewed` | Unit (structural) | Reconcilers call `stuckPRReady`/`abandonedReview`/`isBouncing`, not inlined `time.Since(...)>30m` entangled with a DB read. |

### Migration / schema test (Step 5)

This repo auto-applies schema via `client.Schema.Create(ctx)` at startup (no versioned up/down
migration files — per `research/stack.md`). The migration test is adapted to that reality: a
fresh ent client, schema-create, assert table + index, then confirm re-create is idempotent
(the "reversible/rollback" analog for an additive-only, auto-migrated schema).

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Fresh schema creates table + plain 2-col unique index | `session/backlog_stuck_migration_test.go` | `TestBacklogStuckStateSchema_should_createTableAndUniqueIndex_When_FreshClient` | Integration (ent) | New client on temp/in-memory DB, `Schema.Create` → `backlog_stuck_states` exists; unique index is exactly `(item_id, reason)` (NOT 3-col, NOT partial); inserting a duplicate `(item_id,reason)` pair conflicts (validating the `OnConflictColumns` target). |
| Cascade delete from BacklogItem | `session/backlog_stuck_migration_test.go` | `TestBacklogStuckState_should_cascadeDelete_When_ParentItemDeleted` | Integration (ent) | Item with one open row → `DeleteBacklogItem` → no `backlog_stuck_states` row remains. |
| Additive migration is reversible / idempotent | `session/backlog_stuck_migration_test.go` | `Test_migration_should_be_reversible` | Integration (ent) | (Adapted per Step 5.) Fresh in-memory client → `Schema.Create` twice is idempotent (no error, no dup index); confirm the additive table can be dropped and re-created with no effect on sibling tables (`backlog_items`, `backlog_status_events` intact) — the auto-migration analog of a clean up/down. |

## UX Acceptance Tests

24 tests, one per UX acceptance criterion in `design/ux.md` §"UX Acceptance Criteria" (criteria
21–24 added after a triad-review repair pass covering touch/mobile fallback, multi-reason
fan-out, `pr_ready_unmerged` actionability copy, and first-load badge state). E2E specs
follow `.claude/rules/e2e-test-conventions.md`: file header `// @feature backlog:list-stuck`,
no `waitForTimeout` (use `expect(locator).toHaveText/…` or `waitForSelector`), `data-testid`/ARIA
locators only, shared navigation in a `tests/e2e/pages/StuckItemsPage.ts` helper. Component-level
error/edge states (RPC failure injection, per-item degraded chip, snooze-fail) are Jest + React
Testing Library where forcing a server error in a full e2e run is unreliable; contrast is the
existing Axe Core CI gate.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. "is anything stuck?" 0 clicks; detail ≤2 clicks | `tests/e2e/backlog-stuck-items.spec.ts` | `stuck items > shows nav badge with no click and full detail within two clicks` | Playwright | Seed 1 stuck item → load any page, assert `StuckNavBadge` visible (0 clicks) → click nav → land on `/unfinished` with section visible → click card → detail shown (2 clicks total). |
| 2. Narrow to one reason in 1 click | `tests/e2e/backlog-stuck-items.spec.ts` | `stuck items > filters to a single reason class in one chip click` | Playwright | Seed mixed reasons → click "PR ready" chip (`getByRole('button',{name:/PR ready/})`) → only that group shown, `aria-pressed=true`. |
| 3. Snooze in ≤2 clicks | `tests/e2e/backlog-stuck-items.spec.ts` | `stuck items > snoozes an item in two clicks and it leaves the list` | Playwright | Hover/focus card → click "Snooze" → click "Confirm" (2 clicks) → item drops from active list. |
| 4. Reach source PR in 1 click after expand | `tests/e2e/backlog-stuck-items.spec.ts` | `stuck items > opens the source PR in one click from expanded detail` | Playwright | Expand `pr_ready_unmerged` card → assert PR link (`getByRole('link',{name:/PR #148/})`) with `target=_blank`. |
| 5. Clear filter (zero-result) in 1 click | `tests/e2e/backlog-stuck-items.spec.ts` | `stuck items > clears a zero-result filter in one click back to All` | Playwright | Select a reason chip with count 0 → filtered-empty copy naming the filter → click "Clear filter" → back to All (Surface 4). |
| 6. Fetch error → banner + Retry, stale list retained | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` | `StuckItemsSection_should_showStaleBannerAndRetainList_When_RefreshPollFails` | Jest/RTL | Mock hook: first success (list), then error → banner "Couldn't refresh… (last updated Nm ago)" + Retry; last-good list still rendered, never blanked (Surface 6). |
| 7. Per-item degraded chip, never upgrades to green on stale | `web-app/src/components/backlog-stuck/StuckItem.test.tsx` | `StuckItem_should_renderCouldntCheckChipNotGreen_When_LastCheckedOlderThan5Min` | Jest/RTL | `last_checked_at` > 5m for a pr item → ⚪ "Couldn't check PR status" chip, never 🟢, with "last checked Nm ago" (Surface 8). |
| 8. `allow_auto_merge` unknown, other fields still render | `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx` | `StuckItemDetail_should_showAutoMergeUnknownAndRenderRest_When_SettingFetchFailed` | Jest/RTL | Detail with `allowAutoMerge=undefined` → "Repo auto-merge: unknown"; why/since/last-check/PR link all still render (Surface 9). |
| 9. Snooze fail → picker stays open + Retry, not removed | `web-app/src/components/backlog-stuck/StuckItem.test.tsx` | `StuckItem_should_keepPickerOpenWithRetry_When_SnoozeStuckItemFails` | Jest/RTL | Mock `SnoozeStuckItem` reject → picker stays open with "Couldn't snooze — try again" + Retry; item not removed (Surface 10). |
| 10. No dead ends — every degraded state has a next action | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` | `StuckItemsSection_should_alwaysOfferRecoveryAction_When_InAnyDegradedState` | Jest/RTL | Parametrized over error/filtered-empty/per-item-degraded → each renders a visible Retry / Clear filter / usable-detail affordance (no reload required). |
| 11. Filtered-empty always offers Clear filter | `tests/e2e/backlog-stuck-items.spec.ts` | `stuck items > filtered-empty state always exposes Clear filter` | Playwright | Force a filter to 0 → assert "Clear filter" present and returns to All (Surface 4). |
| 12. Resolves while expanded → visible transition | `web-app/src/components/backlog-stuck/StuckItem.test.tsx` | `StuckItem_should_showResolvedConfirmationThenFade_When_ResolvesWhileExpanded` | Jest/RTL | Expanded card, hook update removes the row → in-place "was just resolved" confirmation before removal, not an abrupt yank (Surface 12). |
| 13. Full keyboard operability | `tests/e2e/backlog-stuck-items.spec.ts` | `stuck items > is fully operable by keyboard for chips cards and picker` | Playwright | Tab to chip → Space toggles; Tab to card → Enter expands, Escape collapses; Tab to Snooze → Enter opens picker, Escape closes. |
| 14. Nav badge aria-label full phrase | `web-app/src/components/backlog-stuck/StuckNavBadge.test.tsx` | `StuckNavBadge_should_haveFullPhraseAriaLabelAndHideAtZero_When_Rendered` | Jest/RTL | count=5 → `aria-label="5 items stuck"` (not bare "5"); count=0 → not rendered. |
| 15. Filter chips `aria-pressed` + `role=group` | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` | `StuckItemsSection_should_wrapChipsInRoleGroupWithAriaPressed_When_Rendered` | Jest/RTL | Chip row is `role="group"` w/ `aria-label`; each chip has `aria-pressed`. |
| 16. Cards `role=button`, `aria-expanded`, keyboard | `web-app/src/components/backlog-stuck/StuckItem.test.tsx` | `StuckItem_should_exposeButtonRoleAndAriaExpanded_When_Toggled` | Jest/RTL | `role="button"`, `aria-expanded` flips on Enter/Space, Escape collapses (verbatim `UnfinishedItem` behavior). |
| 17. Count region `aria-live=polite`, never `role=alert` | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` | `StuckItemsSection_should_useAriaLivePoliteForCount_When_CountChanges` | Jest/RTL | Count summary is `aria-live="polite"`; assert no `role="alert"` on routine poll update (Surface 11). |
| 18. Chip pairs color + text (grayscale legible) | `web-app/src/components/backlog-stuck/stuckReason.test.ts` | `getStuckReasonLabel_should_returnTextLabelForEveryReason_When_MappedExhaustively` | Jest | Every one of the six reasons + `pr_status_unknown` yields a non-empty text label paired with a class — exhaustive over the proto enum (a new reason is a compile/lint miss, not a blank chip). |
| 19. Chip contrast ≥ 4.5:1 (WCAG AA) | `tests/e2e/accessibility.spec.ts` (extend) | `accessibility > stuck-item chips pass Axe color-contrast on /unfinished` | Playwright + Axe | Existing Axe Core CI gate run against the rendered section; asserts no `color-contrast` violations for new chip pairs. |
| 20. Long title truncates with `title=` tooltip | `web-app/src/components/backlog-stuck/StuckItem.test.tsx` | `StuckItem_should_truncateWithTitleTooltip_When_TitleIsLong` | Jest/RTL | Very long title → truncated with a `title=` attribute; chip + duration not pushed off-screen (Surface 7). |
| 21. Touch device: Snooze reachable without hover | `web-app/src/components/backlog-stuck/StuckItem.test.tsx` | `StuckItem_should_showAlwaysOnSnoozeAffordance_When_HoverUnavailable` | Jest/RTL | Simulate `(hover: none), (pointer: coarse)` media query → kebab/overflow icon-button rendered always-visible (not hover-gated), ≥44×44px tap target (Surface 7 touch variant). |
| 22. Multi-reason fan-out shows cross-reference badge | `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx` | `StuckItemsSection_should_showOtherReasonsBadge_When_SameItemInMultipleGroups` | Jest/RTL | Seed item `96cc9eaa` matching 2 reasons → both cards render an "also stuck for 1 other reason" badge; badge suppressed when a chip filter narrows the view to a single card (Surface 2). |
| 23. `pr_ready_unmerged` detail states the merge action explicitly | `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx` | `StuckItemDetail_should_showExplicitMergeCopy_When_ReasonIsPrReadyUnmerged` | Jest/RTL | Detail for `pr_ready_unmerged` → renders "This PR is ready — merge it on GitHub when you're ready." alongside the PR link (Surface 9). |
| 24. First load shows neutral loading affordance, not a stale zero | `web-app/src/components/backlog-stuck/StuckNavBadge.test.tsx` | `StuckNavBadge_should_showLoadingAffordance_When_NoFetchHasSucceededYet` | Jest/RTL | Before first successful fetch resolves → badge renders a pulse/skeleton placeholder, never a bare "0" or hidden-as-if-confirmed-empty state (Surface 1). |

## Test Stack

- **Unit**: Go `testing` + `testify` (`assert`/`require`), table-driven for the pure decision
  functions (`stuck_decisions_test.go` — no ent, no DB, where the false-positive risk lives).
  Frontend: Jest + React Testing Library for maps, hooks, and component states.
- **Integration**: ent repository tests via `NewEntRepository(WithDatabasePath(dbPath))` on a
  `t.TempDir()` sqlite file; restart-survival tests reopen a second repo from the same file
  (pattern: `TestEntRepository_UUID_SurvivesDBReopen`). Reconciler tests drive `ReconcileStuck`
  with an injected fake GitHub/PR-status client (to assert zero extra API calls and to force
  green-mergeable-unapproved / error `PRInfo` field states deterministically so
  `prReadyToMergeSolo` true/false paths are covered without a real approval). RPC handler tests exercise
  `ListStuckBacklogItems`/`SnoozeStuckItem` against a seeded repo.
- **E2E / UX**: Playwright + Allure in `tests/e2e/` against the test server on `:8544`
  (`STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local`), stuck data seeded
  through the backlog store; navigation reused via `tests/e2e/pages/StuckItemsPage.ts`. Axe Core
  (existing CI gate on `web-app/src/` changes) covers contrast.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service/repository methods (`MarkStuck`, `ResolveStuck`, `FindOpenStuckStates`,
  `MarkStuckNotified`, `CountReviewCyclesSince`, `BackfillStuckStates`, `ListStuckBacklogItems`,
  `SnoozeStuckItem`): happy path + error/idempotent paths covered above.
- All external integrations: the GitHub/PR-status path is unit-mocked (fake client asserting no
  new `GetPRStatus`/`gh` call, forced green-mergeable-unapproved and error `PRInfo` states) plus exercised in the
  `pr_ready_unmerged` and `pr_status_unknown` integration/component tests.
- Every UX acceptance criterion in `design/ux.md` has a corresponding automated test (rows 1–20),
  with contrast (19) delegated to the existing Axe Core CI gate.
- Migration/schema: `Test_migration_should_be_reversible` + table/index + cascade tests
  (adapted to this repo's auto-migration reality — no versioned up/down files exist).
- Adversarial concerns C1 (self-heal reason→status map for `bouncing`/`rework_cap`/`push_failed`)
  and C2 (poll-shaped resolve paths for `pr_ready_unmerged`/`stale_work`/`abandoned_review`) each
  have dedicated regression tests in the ResolveStuck/self-heal section above.
- **Live-data verification gate (pre-mortem F1/F3, run before Phase 6 sign-off):** re-run the
  original investigation queries (status counts, per-item session-role counts, PR mergeability)
  against the shipped data source and assert **every one of the 6 originally-observed `review`
  items surfaces with a `StuckReason`**, and that green-unmerged PR #148 surfaces as
  `pr_ready_unmerged` **despite having zero approvals**. Any observed stuck item that does NOT map
  to a reason (e.g. a zombie-session item that slips past `FindZombieReviewItems`) is a blocker —
  do not ship until each is accounted for. This is the success-metric check from requirements.md,
  promoted to an explicit gate because the unit/integration tests above seed clean data and cannot
  by themselves prove the real 6 are covered.
- Feature-registry coverage is a CI gate (`make registry-generate`, `coverage-gaps.json`
  non-growth), not a unit test — noted as a manual/CI verification step.
