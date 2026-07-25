# Validation Plan -- Triage Pipeline

## Test Coverage Map

REQ-1: Prompt injection fixed (positional arg not --append-system-prompt)
  T1: Static code review of session/instance_tmux.go
  T2: Unit tests (TestTriggerTriage_DoubleTriggerGuard confirms session created)
  T3: E2E confirms TriageReviewPanel appears (proves Claude received and executed prompt)

REQ-2: ended_at written for triage sessions
  T2: TestItemSessionToProto_MapsTriageResult (verifies field mapping path)
  T4: Server log check -- session exit log confirms ended_at written
  T3: E2E triageStatus=completed requires ended_at (implicit validation)

REQ-3: OneShot adds -p flag
  T1: Code review confirms -p in buildLaunchCommand for OneShot=true path
  T3: E2E success requires Claude to have received and completed prompt (proves -p effective)

## Test Cases

TC-1 (unit): go test ./server/services/... -- TestTriggerTriage_DoubleTriggerGuard PASS
TC-2 (unit): go test ./server/services/... -- TestItemSessionToProto_MapsTriageResult PASS
TC-3 (unit): go test ./server/mcp/... -- TestSubmitTriageResult_PublishesNotificationOnSuccess PASS
TC-4 (static): buildLaunchCommand OneShot path contains -p flag at lines 44-46
TC-5 (static): buildLaunchCommand prompt is positional arg at lines 47-49
TC-6 (e2e): TRIAGE_VALIDATION=true -- triage-review-panel visible with summary len > 10

## Acceptance Criteria Verification

AC-1: All unit tests pass including double-trigger guard -- verified by TC-1,TC-2,TC-3
AC-2: buildLaunchCommand assembles correct CLI -- verified by TC-4,TC-5
AC-3: Full pipeline produces TriageReviewPanel -- verified by TC-6 (live E2E)

## Prerequisites

- stapler-squad server running at http://localhost:8543
- ANTHROPIC_API_KEY or AWS Bedrock credentials
- npm install in tests/e2e

## Risk Mitigations

If E2E prerequisites unavailable: TC-1 through TC-5 provide 80pct confidence via static + unit validation. Document gap in triage result.
If PERMISSION_DENIED in E2E: Check item_sessions table; insert missing row if needed. See architecture.md for UUID threading details.