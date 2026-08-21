# Validation Plan: backlog-agent-communication

**Date**: 2026-07-23

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Dim 1 (fwd handoff): request_review persists structured fields | `server/mcp/tools_backlog_test.go` | `TestRequestReview_should_PersistHandoffContext_When_FieldsProvided` | Unit | Happy path |
| Dim 1 (fwd handoff): handoff context round-trips | `session/domain/backlog_test.go` | `TestHandoffContextJSON_should_RoundTrip_When_Serialized` | Unit | Happy path |
| Dim 1 (fwd handoff): review prompt renders it | `server/mcp/tools_backlog_test.go` | `TestBuildReviewPrompt_should_RenderKnownLimitations_When_HandoffContextSet` | Unit | Happy path |
| Dim 1 (bwd findings): submit_review_verdict persists findings | `server/mcp/tools_backlog_test.go` | `TestSubmitReviewVerdict_should_PersistStructuredFindings_When_FindingsProvided` | Unit | Happy path |
| Dim 1 (bwd findings): findings surfaced to next agent | `server/mcp/tools_backlog_test.go` | `TestGetBacklogItem_should_IncludeFindings_When_LatestVerdictHasThem` | Unit | Happy path |
| Dim 1 (bwd findings): severity enum validated | `session/domain/backlog_test.go` | `TestFindingSeverity_should_RejectUnknownValue_When_Validated` | Unit | Error path |
| Dim 1 (bwd findings): findings JSON round-trips | `session/domain/backlog_test.go` | `TestFindingsJSON_should_RoundTrip_When_Serialized` | Unit | Happy path |
| Dim 1: findings/handoff backward-compatible with empty input | `server/mcp/tools_backlog_test.go` | `TestSubmitReviewVerdict_should_AcceptEmptyFindings_When_NotProvided` | Unit | Edge case |
| Pain point A: report_pr_created transitions on valid PR | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_TransitionToPRPending_When_ValidPR` | Unit | Happy path |
| Pain point A: primary write failure surfaces as error | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_ReturnError_When_PersistFails` | Unit | Error path |
| Pain point A: idempotent re-report | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_NoOp_When_AlreadyPRPendingSamePR` | Unit | Edge case |
| Pain point A: role guard | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork` | Unit | Error path |
| Pain point A: GitHub verification rejects mismatch | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_RejectCall_When_BranchMismatch` | Unit | Error path |
| Pain point A: transient GitHub error is retryable, not silently skipped | `server/mcp/tools_backlog_test.go` | `TestReportPRCreated_should_ReturnRetryableError_When_GitHubLookupTransientlyFails` | Unit | Error path |
| Pain point A: reconciliation backstop links orphaned PR | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedAgentPRs_should_LinkPR_When_ReviewStatusNoLiveSessionPRExists` | Integration | Reconciler + storage |
| Pain point A: reconciliation backstop no-ops correctly | `session/backlog_lifecycle_test.go` | `TestReconcileOrphanedAgentPRs_should_NoOp_When_NoMatchingPR` | Integration | Negative case |
| Dim 2: infra report creates open row | `server/mcp/tools_infra_test.go` | `TestReportInfraIssue_should_CreateOpenReport_When_ValidInput` | Unit | Happy path |
| Dim 2: high/critical severity notifies | `server/mcp/tools_infra_test.go` | `TestReportInfraIssue_should_Notify_When_SeverityHighOrCritical` | Unit | Happy path |
| Dim 2: low/medium severity does not notify | `server/mcp/tools_infra_test.go` | `TestReportInfraIssue_should_NotNotify_When_SeverityLowOrMedium` | Unit | Edge case |
| Dim 2: dedup increments existing row | `server/mcp/tools_infra_test.go` | `TestReportInfraIssue_should_IncrementOccurrenceCount_When_DuplicateWithinCooldown` | Unit | Edge case |
| Dim 2: dedup expires after cooldown | `server/mcp/tools_infra_test.go` | `TestReportInfraIssue_should_CreateNewRow_When_SameCategoryPastCooldown` | Unit | Edge case |
| Dim 2: RPC surface lists/acknowledges/resolves | `server/services/backlog_service_infra_test.go` | `TestListInfraIssueReports_should_ReturnOpenReports_When_Queried` | Integration | RPC + storage |
| Dim 2: UI e2e | `tests/e2e/infra-issue-reports.spec.ts` | `infra-issue-reports > operator can acknowledge and resolve a report` | E2E | Full flow |
| Dim 3: request_help marks item stuck | `server/mcp/tools_backlog_test.go` | `TestRequestHelp_should_MarkItemStuck_When_ValidCall` | Unit | Happy path |
| Dim 3: duplicate call rejected | `server/mcp/tools_backlog_test.go` | `TestRequestHelp_should_RejectDuplicateCall_When_RowAlreadyOpen` | Unit | Error path |
| Dim 3: escalation bypasses periodic-sweep delay | `server/mcp/tools_backlog_test.go` | `TestRequestHelp_should_NotifyImmediately_When_Called` | Unit | Happy path |
| Dim 3: new StuckReason round-trips through proto | `server/services/backlog_stuck_rpc_test.go` | `TestToProtoStuckReason_should_MapHelpRequested_When_RoundTripped` | Unit | Happy path |
| Dim 3/4: response delivered to live session | `server/services/backlog_service_stuck_test.go` | `TestRespondToHelpRequest_should_DeliverToLiveSession_When_SessionStillRunning` | Integration | Happy path |
| Dim 3/4: response spawns fresh session when none live | `server/services/backlog_service_stuck_test.go` | `TestRespondToHelpRequest_should_SpawnFreshSession_When_NoLiveSessionAndResumeTrue` | Integration | Happy path |
| Dim 3/4: response persists without spawn when declined | `server/services/backlog_service_stuck_test.go` | `TestRespondToHelpRequest_should_OnlyPersist_When_NoLiveSessionAndResumeFalse` | Integration | Edge case |
| Dim 3: UI e2e | `tests/e2e/help-request-response.spec.ts` | `help-request-response > operator can respond and resume the session` | E2E | Full flow |
| Pain point B: dispute created on FAIL-class verdict | `server/mcp/tools_backlog_test.go` | `TestDisputeReviewVerdict_should_CreateOpenDispute_When_LatestVerdictIsFail` | Unit | Happy path |
| Pain point B: dispute rejected on PASS verdict | `server/mcp/tools_backlog_test.go` | `TestDisputeReviewVerdict_should_Reject_When_VerdictIsPass` | Unit | Error path |
| Pain point B: dispute cap enforced | `server/mcp/tools_backlog_test.go` | `TestDisputeReviewVerdict_should_Reject_When_CapExceeded` | Unit | Error path |
| Pain point B: auto-rework paused by open dispute | `session/backlog_lifecycle_test.go` | `TestAutoReopenAfterFailedReview_should_NoOp_When_OpenDisputeExists` | Integration | Control-flow regression |
| Pain point B: uphold/overturn adjudication | `server/services/backlog_service_dispute_test.go` | `TestAdjudicateDispute_should_ReturnCriteriaToPass_When_Overturned` | Integration | Happy path |
| Pain point B: re-review refuses when worktree gone | `server/services/backlog_service_dispute_test.go` | `TestAdjudicateDispute_should_RejectRereview_When_WorktreeGone` | Integration | Error path (BUG-045 guard) |
| Pain point B: UI e2e | `tests/e2e/verdict-dispute.spec.ts` | `verdict-dispute > operator can adjudicate a disputed verdict` | E2E | Full flow |
| Dim 4 (human UI): handoff context visible on item detail | `web-app/src/components/backlog/__tests__/BacklogItemDetail.handoffContext.test.tsx` | `BacklogItemDetail_should_RenderKnownLimitations_When_HandoffContextPresent` | Unit (RTL) | Happy path |
| Dim 4 (human UI): findings visible on item detail | `web-app/src/components/backlog/__tests__/BacklogItemDetail.findings.test.tsx` | `BacklogItemDetail_should_RenderSeverityGroupedFindings_When_VerdictHasThem` | Unit (RTL) | Happy path |
| Notification volume discipline (Epic 7.1) | `session/backlog_lifecycle_test.go` | `TestNewNotifierCallSites_should_FireAtMostOnce_When_ConditionRepeats` | Unit | Regression against alert-fatigue pitfall |

## Test Stack

- **Unit (Go)**: standard library `testing` + this repo's existing table-driven
  conventions (see `session/backlog_remediation_test.go` and
  `session/backlog_lifecycle_test.go` for the house style — fake
  spawners/notifiers/checkers as small local structs, not a mocking framework).
- **Integration (Go)**: same `testing` package, against a real (in-memory or
  temp-file) SQLite-backed `EntRepository`, following the existing pattern in
  `session/backlog_lifecycle_test.go` (e.g. `TestReconcilePRPending_*`) — no
  separate integration test runner.
- **Unit (Frontend)**: Jest + React Testing Library (`cd web-app && npx jest`),
  matching existing `BacklogItemDetail.*.test.tsx` naming convention already
  present in this repo (e.g. `BacklogItemDetail.lastReviewResult.test.tsx`,
  `BacklogItemDetail.shipPR.test.tsx` — this plan's new
  `BacklogItemDetail.handoffContext.test.tsx`/`.findings.test.tsx` follow the
  same per-feature-suffix file-naming convention).
- **E2E**: Playwright, per `.claude/rules/e2e-test-conventions.md` — feature
  annotation header, `data-testid`/ARIA locators only, no `waitForTimeout`, new
  page helpers under `tests/e2e/pages/` if the new flows need multi-step
  navigation helpers (the three new e2e specs above are single-page-detail flows
  and may not need new helpers — confirm at implementation time).

## Coverage Targets

- Unit test coverage: ≥80% line coverage on every new/modified Go file listed in
  `plan.md`'s per-task `Files:` lines.
- Every new MCP tool handler (`report_pr_created`, `report_infra_issue`,
  `request_help`, `dispute_review_verdict`) has: a happy-path test, a
  caller-not-linked/role-mismatch error-path test (mirroring the existing
  `errResult(ErrPermissionDenied, ...)` pattern every current tool test covers),
  and at least one edge case specific to that tool (idempotency, dedup, cap, or
  duplicate-rejection, per the table above).
- Every new `StuckReason` value (`StuckReasonHelpRequested`,
  `StuckReasonVerdictDisputed`) has a proto round-trip test extending
  `TestToProtoStuckReason_should_mapToUnspecified_When_UnknownString`'s existing
  exhaustiveness pattern, and a frontend `stuckReason.ts` exhaustiveness test run
  (`cd web-app && npx jest --testPathPatterns=stuckReason`) confirming the
  `Record<StuckReason, T>` maps compile-check clean for the new values.
- Every new `Notifier.Notify` call site introduced by Epics 4–6 has a test
  asserting it fires at most once per triggering condition (not once per
  reconciler tick or per duplicate call) — the concrete regression test for
  Epic 7.1's audit finding.
- The two BUG-045-adjacent guards (`report_pr_created`'s GitHub verification,
  `AdjudicateDispute`'s re-review worktree-existence check) each have an explicit
  negative-path test — these are the two places in this plan doing double duty
  as regression coverage for *related* open bugs, not just this plan's own new
  code.
