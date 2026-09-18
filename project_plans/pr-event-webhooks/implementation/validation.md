# Validation Plan: pr-event-webhooks

**Date**: 2026-08-24

## Happy Path Scenario
Given a `pr_pending` backlog item tracking PR #189 in `tstapler/stapler-squad` whose CI job just failed, when GitHub delivers a signed `check_run` webhook event (`action=completed`, `conclusion=failure`) to `POST /webhooks/github` with the `pr_event_webhooks` flag enabled, then `handlePRFixEvent` extracts the event, matches it to the tracked item via `TriggerPRFixForEvent`, immediately reconciles it via `reconcilePRPendingItem` → `remediatePRFixWithBackoffGate` → `AutoReopenForPRFix` (instead of waiting up to `PRStatusPoller`'s 60s tick), and persists a `TriggerFireEvent` row with `outcome: "fired_success"`.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1a: Goal 1 — `X-GitHub-Event` routing in `Handle` (Story 2.1.1) | `server/services/github_webhook_handler_test.go` | `TestGitHubWebhookHandler_should_DispatchToHandlePRFixEvent_When_EventTypeIsCheckRunWorkflowRunReviewOrComment` | Unit | Happy path — 4 new event-type headers route to `handlePRFixEvent`, not the push-matching logic |
| REQ-1a | `server/services/github_webhook_handler_test.go` | `TestGitHubWebhookHandler_should_Return200NoAuditRow_When_EventTypeIsUnrecognized` | Unit | Error/edge path — `X-GitHub-Event: ping` returns `200` with zero `TriggerFireEvent` rows persisted |
| REQ-1a | `server/services/github_webhook_handler_test.go` | All 9 existing `TestGitHubWebhookHandler_*` functions, unmodified | Integration | Given no header set or `push`, confirms byte-for-byte parity with today's behavior (Task 2.1.1b) |
| REQ-1b: Goal 1 — payload extractors for the 4 new event types (Story 2.1.2) | `server/services/github_webhook_pr_fix_test.go` (new) | `TestExtractCheckRunEvent_should_ReturnActionableTrue_When_ActionCompletedAndConclusionFailure` | Unit | Happy path — pinned failure-shaped conclusion → `actionable=true`, `prNumbers=[189]` |
| REQ-1b | `server/services/github_webhook_pr_fix_test.go` (new) | `TestExtractCheckRunEvent_should_ReturnOkFalse_When_RepositoryFullNameMissing` | Unit | Error path — `repository.full_name` absent → `ok=false`, mirrors `extractGitHubRepoAndBranch`'s degrade-gracefully contract |
| REQ-1b | `server/services/github_webhook_pr_fix_test.go` (new) | `TestExtractPullRequestReviewEvent_should_HandleAllStatesAndTestExtractIssueCommentEvent_should_HandleNonPRIssues` (table-driven, one table per extractor per Task 2.1.2e) | Integration | Full 4-extractor sweep incl. GitHub's non-triggering enum values (`neutral`, `skipped`, `stale`, `cancelled` marked actionable per `CIFailing` parity) against realistic multi-field payload fixtures |
| REQ-2a: Goal 2 — `reconcilePRPendingItem` extraction, zero behavior change (Story 1.1.1) | `session/backlog_lifecycle_test.go` | `TestReconcilePRPending_should_TransitionToDone_When_PRMerged` (existing, unmodified) | Unit | Happy path — regression: merge → ship-snapshot → `done` transition still occurs via the new call path |
| REQ-2a | `session/backlog_lifecycle_stuck_test.go`, `session/backlog_lifecycle_superseded_test.go`, `session/backlog_lifecycle_pr_branch_guard_test.go` (existing, unmodified) | `go test ./session/... -run 'ReconcilePRPending\|PRPending' -v` (Task 1.1.1c) | Unit | Error/edge paths — stuck, superseded, and branch-guard rejection paths all preserved after extraction |
| REQ-2a | `session/backlog_lifecycle_pr_trigger_test.go` (new) | `TestReconcilePRPendingItem_should_ProduceIdenticalOutcome_When_CalledViaLoopOrViaDirectInvocation` | Integration | Confirms `ReconcilePRPending`'s `for` loop and a direct single-item call from `TriggerPRFixForEvent` produce identical side effects |
| REQ-2b: Goal 2 — `findPRPendingItemForEvent` lookup (Story 1.2.1) | `session/backlog_lifecycle_pr_trigger_test.go` (new) | `TestFindPRPendingItemForEvent_should_ReturnItemAndTrue_When_PrNumberAndRepoMatch` | Unit | Happy path |
| REQ-2b | `session/backlog_lifecycle_pr_trigger_test.go` (new) | `TestFindPRPendingItemForEvent_should_ReturnNilFalse_When_PrNumberMatchesButRepoDiffers` | Unit | Error/edge path — PR-number collision across two tracked repos; repo identity must also match |
| REQ-2b | `session/backlog_lifecycle_pr_trigger_test.go` (new) | `TestFindPRPendingItemForEvent_should_ReturnNilFalse_When_ZeroPrPendingItemsExist` | Integration | Exercises `FindPRPendingItems` against the fake `EntRepository` with an empty result set |
| REQ-2c: Goal 2 — `TriggerPRFixForEvent` entry point (Story 1.2.2) | `session/backlog_lifecycle_pr_trigger_test.go` (new) | `TestTriggerPRFixForEvent_should_ReconcileItemAndReturnMatchedTrue_When_ItemFound` | Unit | Happy path — matched item reconciles via fake `prPendingChecker`/`PRFixSpawner` |
| REQ-2c | `session/backlog_lifecycle_pr_trigger_test.go` (new) | `TestTriggerPRFixForEvent_should_ReturnFalseNilWithoutQuerying_When_ListenerDisabled` | Unit | Error/edge path — `l.enabled.Load() == false` short-circuits before any `FindPRPendingItems` query |
| REQ-2c | `session/backlog_lifecycle_pr_trigger_test.go` (new) | `TestTriggerPRFixForEvent_should_ReturnFalseNil_When_NoPrPendingItemMatches` | Integration | End-to-end through `findPRPendingItemForEvent` with no match |
| REQ-2d: Goal 2 — `PRFixEventRouter` interface + constructor wiring (Story 2.3.1) | `server/services/github_webhook_handler_test.go` | `TestNewGitHubWebhookHandler_should_AcceptPRFixEventRouterAndSelfLoginCache_When_Constructed` | Unit | Happy path — constructor accepts and stores the new fields |
| REQ-2d | `server/services/github_webhook_handler_test.go` | Existing 9 test functions updated to pass `nil` for `PRFixEventRouter` (Task 2.3.1b) | Unit | Error/edge path — `nil` router is safe for `push`-only tests since they never reach `handlePRFixEvent` |
| REQ-2d | `server/dependencies_test.go` | `TestBuildRuntimeDeps_should_PopulateBacklogLifecycleListener_When_ServerBoots` (Task 3.1.1d verification target) | Integration | `deps.BacklogLifecycleListener` is non-nil and identical to the object `SetPRFixSpawner`/`SetAutoReopener` wired onto |
| REQ-2e: Goal 2 — `handlePRFixEvent` dispatch + outcome persistence (Story 2.3.2) | `server/services/github_webhook_pr_fix_test.go` (new) | `TestHandlePRFixEvent_should_CallRouterAndPersistFiredSuccess_When_DeliveryActionableAndMatched` | Unit | Happy path |
| REQ-2e | `server/services/github_webhook_pr_fix_test.go` (new) | `TestHandlePRFixEvent_should_PersistFiredFailed_When_PRFixRouterIsNilConfigured` | Unit | Error path — router-wiring gap still returns `200`, persists `fired_failed` |
| REQ-2e | `server/services/github_webhook_pr_fix_test.go` (new) | `TestHandlePRFixEvent_should_PersistNoMatchWithoutCallingRouter_When_DeliveryNotActionable` | Integration | Non-actionable delivery (e.g. `conclusion: "success"`) short-circuits before the DB lookup/router call |
| REQ-2e | `server/services/github_webhook_pr_fix_test.go` (new) | `TestHandlePRFixEvent_should_Return401AndRecordRejected_When_WorkflowSecretEmpty` (Story 3.3.1) | Integration | Fail-closed parity: empty `WebhookSecretEncrypted` on the matching `github_push` Workflow row → `401` + `outcome: "rejected"`, same as the `push` path |
| REQ-2f: Goal 2 — self-actor/bot-loop filter (Story 2.2.1, ADR-001) | `server/services/github_webhook_pr_fix_test.go` (new) | `TestSelfLoginCache_should_SuppressActionable_When_CommentAuthorMatchesCachedSelfLogin` | Unit | Happy path |
| REQ-2f | `server/services/github_webhook_pr_fix_test.go` (new) | `TestSelfLoginCache_should_NotSuppressAnything_When_GetCurrentUserLoginErrorsOrEmpty` | Unit | Error path — fails open per ADR-001, one `Warn` log per cache refresh, not per event |
| REQ-2f | `server/services/github_webhook_pr_fix_test.go` (new) | `TestHandlePRFixEvent_should_NeverApplySelfFilter_When_EventTypeIsCheckRunOrWorkflowRun` | Integration | Confirms the self-filter is scoped to `issue_comment`/`pull_request_review` only |
| REQ-2g: Goal 2 — CAS/duplicate-delivery safety for concurrent webhook fanout (Story 3.3.2, review-only per plan) | `session/ent_repository_backlog_transition_test.go`, `server/services/backlog_service_lifecycle_test.go` (existing, unmodified) | `TestTransitionBacklogItemStatus_should_letExactlyOneWinnerThrough_When_TwoWritersRaceConcurrently`, `TestTransitionBacklogItemStatus_should_FailCASForLoser_When_ConcurrentOverrideRaces` | Review | Task 3.3.2a: confirms `TriggerPRFixForEvent`'s call path (`reconcilePRPendingItem` → `remediatePRFixWithBackoffGate` → `AutoReopenForPRFix`) is the exact path these existing CAS tests already exercise — no new test required, no coverage gap |
| REQ-3: Goal 3 — `PRStatusPoller` remains an unmodified backstop (Story 3.4.1) | N/A | Diff review: zero changed lines in `session/pr_status_poller.go`; existing `session/pr_status_poller_test.go` suite passes unmodified | Review | Enforced by PR diff inspection, not a new test — `PRStatusPoller` has no reference to `EntRepository`/`BacklogLifecycleListener` today and must not gain one |
| REQ-4: Goal 4 — public-reachability documentation for `/webhooks/github` (Story 3.2.1) | N/A | N/A — doc-only deliverable (`.claude/docs/github-webhook-public-reachability.md`) | N/A | No source-code test applies; verified against Story 3.2.1's content checklist (path-scoping, flag states, `github_push` Workflow-row prerequisite) during doc review |
| REQ-5: Goal 5 — `pr_event_webhooks` feature flag gates the new event handling (Story 2.1.3) | `server/services/github_webhook_pr_fix_test.go` (new) | `TestHandlePRFixEvent_should_ProceedToExtraction_When_FeatureFlagEnabled` | Unit | Happy path |
| REQ-5 | `server/services/github_webhook_pr_fix_test.go` (new) | `TestHandlePRFixEvent_should_Return200WithNoAuditRow_When_FeatureFlagDisabled` | Unit | Error path — flag off is a true no-op, not even a `no_match` row persisted |
| REQ-5 | `server/services/github_webhook_pr_fix_test.go` (new) | `TestHandlePRFixEvent_should_Return200NoAuditRow_When_FeatureFlagDisabledEvenForValidSignedDelivery` | Integration | Confirms the flag check precedes signature verification and extraction end-to-end for an otherwise-valid delivery |

## UX Acceptance Tests
N/A — pure infrastructure feature, no user-facing surface.

## Test Stack
- **Unit**: Go stdlib `testing` + table-driven tests (matches this repo's existing style, e.g. `github_webhook_handler_test.go`'s per-candidate signature-verification tests)
- **Integration**: Go stdlib `testing` with fake/in-memory repos and fake `PRFixEventRouter`/`TriggerFireEventRepository` doubles (matches `github_webhook_handler_test.go`'s and `backlog_lifecycle_test.go`'s existing fake-based pattern — no real Postgres/ent DB or live GitHub API in these tests)
- **E2E / UX**: N/A

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- All public service methods (`TriggerPRFixForEvent`, `handlePRFixEvent`, the 4 extractors, `selfLoginCache.Get`): happy path + error paths covered
- All external integrations (GitHub webhook signature verification, `AutoReopenForPRFix`/`PRFixSpawner`, `TriggerFireEventRepository`): unit mocked + at least one integration test per REQ-2 sub-row above
- `session/pr_status_poller.go`: **zero new test surface** — its existing suite is the only check, per REQ-3's explicit non-change

## Notes on Coverage Gaps Intentionally Left Open

- REQ-2g and REQ-3 are **review-only** by design (per plan.md Stories 3.3.2 and 3.4.1) — they verify an existing invariant still holds rather than adding new test files. Flagging this explicitly here so a later reviewer doesn't mistake the absence of a new test file for a missed requirement.
- REQ-4 has no `go test` equivalent because it is a documentation deliverable; its "test" is the acceptance-criteria checklist embedded in plan.md Story 3.2.1.
- Migration test: **N/A** — plan.md's Migration Plan section confirms no ent schema changes are needed (`TriggerFireEvent.workflow_id` already nullable, `outcome` already unconstrained `NotEmpty()` string), so no migration-specific test row is included in this plan.
