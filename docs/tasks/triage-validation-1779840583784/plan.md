# Plan -- Triage Pipeline Validation

## Executive Summary

Three bugs fixed in commit 19ef4431: (1) prompt passed via --append-system-prompt instead of positional arg -- Claude never received triage task; (2) ended_at never written for non-work roles -- TriageReviewPanel never rendered; (3) OneShot never added -p flag -- Claude exited without running prompt. All three now fixed.

No new code needed. Validation only.

## Implementation Approach

Validate across three layers: (1) static verification of buildLaunchCommand, (2) unit tests, (3) live E2E test with real Claude agent.

## Task Breakdown

T1: Verify buildLaunchCommand (15m) -- session/instance_tmux.go lines 39-70. Confirm OneShot appends -p, prompt is positional arg, --mcp-config has X-Stapler-Session-UUID.

T2: Run unit tests (30m) -- make build then go test ./server/services/... ./session/... ./server/mcp/... Expected: TestTriggerTriage_DoubleTriggerGuard, TestItemSessionToProto_MapsTriageResult, TestSubmitTriageResult_PublishesNotificationOnSuccess all pass.

T3: Live E2E test (6-15m) -- TRIAGE_VALIDATION=true TEST_SERVER_URL=http://localhost:8543 npx playwright test triage-pipeline-validation.spec.ts. Success: triage-review-panel visible with non-empty summary.

T4: Verify server logs (10m) -- confirm [TriggerTriage] spawned, [mcp:submit_triage_result] triage_result=..., no PERMISSION_DENIED.

## Dependencies Blockers

Server running at http://localhost:8543, ANTHROPIC_API_KEY or Bedrock credentials for T3. No code changes required.