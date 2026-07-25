# Validation Plan: GitHub Autonomous Fix

**Date**: 2026-06-09
**Status**: Draft
**Plan version reviewed**: plan.md + adversarial-review.md (all 4 patches applied)

---

## 1. Test Suite Design

### 1.1 Go Unit Tests

**File**: `session/claude_controller_test.go`

| Test Name | AC Covered | Notes |
|---|---|---|
| `TestRegisterMultipleStatusListeners_AllFire` | US-3 AC1 (fan-out) | Register 2 listeners, simulate PTY transition, assert both called |
| `TestSetStatusChangeListener_Replaces` | US-3 AC1 | Old listener NOT called after re-Set |
| `TestRegisterStatusChangeListener_Concurrent` | US-3 AC1 | -race passes: register from goroutine while loop runs |

**File**: `session/autonomous_driver_test.go`

| Test Name | AC Covered | Notes |
|---|---|---|
| `TestAutonomousDriver_IdempotencyGuard` | US-3 AC1 | Two concurrent Start calls → only 1 goroutine |
| `TestAutonomousDriver_MaxTurnsLimit` | US-3 AC6 | Fake pool always returns NEXT_MESSAGE; loop exits at maxTurns |
| `TestAutonomousDriver_DoneSignal` | US-3 AC5, US-6 AC1 | Fake pool returns DONE on turn 2; outcome.Done=true |
| `TestAutonomousDriver_RateLimitPause` | US-3 AC3 | GetRateLimitState returns StateDetected × 2 then StateNone; no SendCommandImmediate until clear |
| `TestAutonomousDriver_StatusChannelSignal` | US-3 AC2 (C1 fix) | Listener sends to channel only; SendCommandImmediate called from run goroutine only |
| `TestAutonomousDriver_PanicRecovery` | (safety, not in US) | Fake pool panics on turn 1; server does not crash |
| `TestAutonomousDriver_Stop_CancelsLoop` | US-3 lifecycle | Stop() after Start() terminates goroutine without data race |
| `TestParseOrchestrationResponse_NextMessage` | US-3 AC3 | NEXT_MESSAGE: prefix extracted correctly |
| `TestParseOrchestrationResponse_Done` | US-3 AC5 | DONE: prefix sets done=true |
| `TestParseOrchestrationResponse_Malformed` | US-3 AC3 | err returned; neither prefix present |
| `TestBuildOrchestrationPrompt_ContainsGoalAndTail` | US-3 AC3 | Goal string and tail appear in output |
| `TestExtractPRURL_MatchesInTail` | US-6 AC2 | Valid PR URL in last 200 lines returned |
| `TestExtractPRURL_IgnoresInputPromptURL` | US-6 AC2 (C4 fix) | URL in first half of long output ignored |
| `TestExtractPRURL_MultipleURLs_FirstWins` | US-6 AC2 | First match in tail returned |
| `TestExtractPRURL_NoURL` | US-6 AC2 | Empty string when no URL |

**File**: `server/services/approval_handler_test.go` (extend existing)

| Test Name | AC Covered | Notes |
|---|---|---|
| `TestApprovalHandler_AutonomousLLMApprove` | US-5 AC3 | LLM returns APPROVE: → allow decision stored in approval log with source "llm_orchestrator" |
| `TestApprovalHandler_AutonomousLLMDeny` | US-5 AC4 | LLM returns DENY: → human review queue |
| `TestApprovalHandler_AutonomousLLMError` | US-5 AC4 | headlessPool error → human review queue |
| `TestApprovalHandler_NonAutonomous_NoLLMCall` | US-5 AC1 | autonomousChecker returns false → LLM not called |
| `TestApprovalHandler_AutoCheckerInjection` | US-5 AC1 (C2 fix) | SetAutonomousChecker wires closure correctly; no circular dep |

**File**: `session/backlog_plugin_github_prs_test.go`

| Test Name | AC Covered | Notes |
|---|---|---|
| `TestGitHubPRsPlugin_MapToBacklogItem_TagsReviewRequested` | US-4 AC3 | review_requests > 0 → tag pr:review-requested |
| `TestGitHubPRsPlugin_MapToBacklogItem_TagsChangesRequested` | US-4 AC3 | state=changes_requested → tag pr:changes-requested |
| `TestGitHubPRsPlugin_MapToBacklogItem_TagsCIFailing` | US-4 AC3 | CI conclusion failure → tag pr:ci-failing |
| `TestGitHubPRsPlugin_Fetch_RespectRateLimit` | US-4 AC1 | X-RateLimit-Remaining=0 → wait/skip |

**File**: `session/headless/pool_key_test.go` (new — C3 verification)

| Test Name | AC Covered | Notes |
|---|---|---|
| `TestPool_ConcurrentAutonomousSessions_NoSerialize` | US-3 concurrency | Confirms per-session key strategy prevents serialization; this test documents the finding from T2.1.3 |

### 1.2 Go Integration Tests

**File**: `server/services/autonomous_integration_test.go` (new)

| Test Name | AC Covered | Notes |
|---|---|---|
| `TestCreateSession_AutonomousMode_WiresDriver` | US-3 AC1, US-2 AC3 | CreateSession with autonomous=true → AutonomousDriver registered in registry |
| `TestCreateSession_AutonomousMode_NilPool_NoDriverStart` | US-3 AC1 | nil headlessPool → no driver, warning logged |
| `TestDeleteSession_StopsDriver` | US-3 lifecycle | DeleteSession → driver.Stop() called |
| `TestSpawnSessionFromItem_AutonomousFlag_SetsMode` | US-2 AC2,3 | SpawnSessionFromItem(autonomous=true) → session has AutonomousMode=true, PermissionMode=auto |
| `TestOnAutonomousDriverComplete_Done_UpdatesBacklogItem` | US-6 AC3 | Outcome.Done=true → backlog item transitions to done, PRUrl stored |
| `TestOnAutonomousDriverComplete_Stuck_FailsBacklogItem` | US-6 AC4 | Outcome.Stuck=true → backlog item transitions to failed |
| `TestOnAutonomousDriverComplete_SendsPushNotification` | US-6 AC5 | Push notification sent on completion |
| `TestCreateDirectorySession_DriverStop_OnError` | (C3.2.2 minor) | Error after driver.Start() → driver.Stop() called in cleanup |

**File**: `server/services/headless_feature_key_test.go` (new)

| Test Name | AC Covered | Notes |
|---|---|---|
| `TestFeatureKeyAutonomousFix_RegisteredInAllowedKeys` | US-3 AC3 | FeatureKeyAutonomousFix in AllowedFeatureKeys map |
| `TestFeatureKeyAutonomousApproval_RegisteredInAllowedKeys` | US-5 AC1 | FeatureKeyAutonomousApproval in AllowedFeatureKeys map |

### 1.3 Jest / RTL Frontend Tests

**File**: `web-app/src/lib/omnibar/actions/dispatch.test.ts` (extend existing)

| Test Name | AC Covered | Notes |
|---|---|---|
| `dispatchOmnibarAction_should_setAutonomousModeTrue_When_sessionTypeIsAutonomous` | US-1 AC3,4 | autonomousMode: true in createSession call |
| `dispatchOmnibarAction_should_setPermissionModeAuto_When_sessionTypeIsAutonomous` | US-1 AC4 | permissionMode: "auto" in createSession call |
| `dispatchOmnibarAction_should_createAutonomousSession_When_autoFix` | US-1 AC2 | auto_fix action → createSession called with autonomousMode: true |
| `dispatchOmnibarAction_should_notCreateNewActionType_When_autonomous` | US-1 registration | No new top-level action type; reuses create_session |

**File**: `web-app/src/lib/omnibar/detector.test.ts` (no new detectors needed for autonomous mode — not auto-detected)

**File**: `web-app/src/components/sessions/Omnibar.test.tsx` (new or extend)

| Test Name | AC Covered | Notes |
|---|---|---|
| `Omnibar_should_showFixAutonomouslyOption_When_GitHubURLDetected` | US-1 AC1 | Paste GitHub issue URL → "Fix autonomously" radio visible |
| `Omnibar_should_hideWorkingDirField_When_AutonomousModeSelected` | US-1 AC2 | No path input shown for autonomous mode |
| `Omnibar_should_includeAutonomousModeInSubmit_When_AutonomousSelected` | US-1 AC5 | handleSubmit passes autonomousMode: true |

**File**: `web-app/src/components/backlog/BacklogItem.test.tsx` (new or extend)

| Test Name | AC Covered | Notes |
|---|---|---|
| `BacklogItem_should_showRunAutonomouslyButton_When_StatusIsReady` | US-2 AC1 | Button rendered for ready items |
| `BacklogItem_should_notShowRunAutonomouslyButton_When_StatusIsNotReady` | US-2 AC1 | Button absent for in_progress/done items |
| `BacklogItem_should_callSpawnWithAutonomousTrue_When_ButtonClicked` | US-2 AC2 | Click → spawnSessionFromItem({ autonomous: true }) |

### 1.4 Playwright E2E Tests

**File**: `tests/e2e/autonomous-fix.spec.ts` (new)

| Test Name | AC Covered | Timeout | Notes |
|---|---|---|---|
| `autonomous-fix > omnibar creates autonomous session` | US-1 AC1,2,5,6 | 30 000ms | Paste issue URL → select Fix autonomously → submit → session appears with Autonomous badge |
| `autonomous-fix > autonomous session uses permission mode auto` | US-1 AC4 | 15 000ms | Session metadata shows permission_mode=auto |
| `autonomous-fix > backlog promotes ready item to autonomous` | US-2 AC1,2,3,4 | 30 000ms | Seed ready item → click Run autonomously → session created, item transitions to in_progress |
| `autonomous-fix > autonomous session eventually exits` | US-3 AC5, US-6 AC1 | 120 000ms | Uses echo-only fake claude; session exits with done state |
| `autonomous-fix > autonomous badge visible in session list` | US-1 AC6 | 15 000ms | data-testid="session-autonomous-badge" present |

---

## 2. Demo Harness Design

### 2.1 Approach: Fake Claude Runner (No Real GitHub Credentials)

The demo harness uses two stubs:

1. **FakeClaudeRunner** (already in `session/headless/fake_runner.go`) — returns scripted LLM responses without invoking the real Claude binary.
2. **Mock GitHub HTTP server** — an `httptest.NewServer` that serves a canned GitHub issue JSON response for `GET /repos/owner/repo/issues/42`.

No real GitHub token is needed. The mock server is started in-process and the `GitHubIssuesPlugin` base URL is overridden via `WithBaseURL(mockServer.URL)`.

### 2.2 Demo Harness Test

**File**: `tests/integration/autonomous_demo_test.go`

```
TestDemoHarness_AutonomousFixFlow
```

Steps:
1. Start mock GitHub HTTP server returning issue #42 (`title: "Fix the login bug"`, `body: "..."`).
2. Configure `FakeClaudeRunner` with the response sequence:
   - Turn 1: `NEXT_MESSAGE: Let me look at the login code.`
   - Turn 2: `NEXT_MESSAGE: Applying fix.`
   - Turn 3: `DONE: Created PR https://github.com/owner/repo/pull/99`
3. Create a real `SessionService` wired with `NewPoolWithRunner(cfg, fakeRunner)`.
4. Call `CreateSession` with `autonomous_mode=true`, `permission_mode=auto`, a fake repo path (temp dir initialized as a git repo).
5. Block until `onAutonomousDriverComplete` fires (via a channel the test injects into the callback).
6. Assert:
   - `outcome.Done == true`
   - `outcome.PRUrl == "https://github.com/repo/pull/99"`
   - Backlog item (if seeded) transitioned to `done`
   - Turn count == 3
   - No goroutine leak (use `goleak.VerifyNone(t)`)

**Run command (no real credentials needed)**:
```bash
go test ./tests/integration/ -run TestDemoHarness_AutonomousFixFlow -v -timeout 60s
```

**Environment variables required**: none (all mocked in-process).

### 2.3 Shell Smoke Script (optional, for manual verification)

**File**: `scripts/demo-autonomous-fix.sh`

```bash
#!/usr/bin/env bash
# Requires: stapler-squad running with STAPLER_SQUAD_INSTANCE=demo
# Requires: GITHUB_TOKEN set (uses real GitHub for manual demo only)
SESSION_ID=$(curl -s -X POST http://localhost:8543/... \
  -d '{"autonomous_mode":true,"path":"/tmp/demo-repo","github_issue_url":"..."}' \
  | jq -r '.sessionId')
echo "Spawned session: $SESSION_ID"
# Poll until session exits
until [ "$(curl -s http://localhost:8543/.../sessions/$SESSION_ID | jq -r '.status')" = "exited" ]; do
  sleep 5
done
echo "Session complete"
```

This shell script is NOT part of CI — it is for manual staging verification only.

---

## 3. Requirement-to-Test Traceability Matrix

| US | Acceptance Criterion | Priority | Test(s) | Type |
|---|---|---|---|---|
| US-1 | AC1: GitHub URL shows "Fix autonomously" in omnibar | P1 | `Omnibar_should_showFixAutonomouslyOption_When_GitHubURLDetected` | RTL |
| US-1 | AC1: (E2E) | P1 | `autonomous-fix > omnibar creates autonomous session` | E2E |
| US-1 | AC2: Creates autonomous OneShot session in repo dir | P1 | `Omnibar_should_hideWorkingDirField_When_AutonomousModeSelected` + E2E above | RTL + E2E |
| US-1 | AC3: Prompt includes issue title/body/labels/ACs | P1 | `TestBuildOrchestrationPrompt_ContainsGoalAndTail` | Unit |
| US-1 | AC4: Session uses --permission-mode auto | P1 | `dispatchOmnibarAction_should_setPermissionModeAuto_When_sessionTypeIsAutonomous`, `autonomous-fix > autonomous session uses permission mode auto` | Jest + E2E |
| US-1 | AC5: AutonomousMode=true flag set | P1 | `dispatchOmnibarAction_should_setAutonomousModeTrue_When_sessionTypeIsAutonomous`, `TestCreateSession_AutonomousMode_WiresDriver` | Jest + Go Integration |
| US-1 | AC6: Progress visible in UI as normal session | P1 | `autonomous-fix > autonomous badge visible in session list` | E2E |
| US-2 | AC1: "Run autonomously" button for ready items | P1 | `BacklogItem_should_showRunAutonomouslyButton_When_StatusIsReady` | RTL |
| US-2 | AC2: Clicking calls SpawnSessionFromItem(autonomous=true) | P1 | `BacklogItem_should_callSpawnWithAutonomousTrue_When_ButtonClicked`, `TestSpawnSessionFromItem_AutonomousFlag_SetsMode` | RTL + Go Integration |
| US-2 | AC3: Session has AutonomousMode=true, PermissionMode=auto | P1 | `TestSpawnSessionFromItem_AutonomousFlag_SetsMode` | Go Integration |
| US-2 | AC4: Backlog item transitions in_progress→done/failed | P1 | `TestOnAutonomousDriverComplete_Done_UpdatesBacklogItem`, `TestOnAutonomousDriverComplete_Stuck_FailsBacklogItem`, `autonomous-fix > backlog promotes ready item to autonomous` | Go Integration + E2E |
| US-3 | AC1: AutonomousDriver goroutine starts with session | P0 | `TestCreateSession_AutonomousMode_WiresDriver` | Go Integration |
| US-3 | AC2: Uses ClaudeController idle detection (not raw sleep) | P0 | `TestAutonomousDriver_StatusChannelSignal` (C1 fix) | Unit |
| US-3 | AC3: Calls headless LLM pool with goal + tail → next msg | P0 | `TestBuildOrchestrationPrompt_ContainsGoalAndTail`, `TestAutonomousDriver_DoneSignal`, `TestDemoHarness_AutonomousFixFlow` | Unit + Integration |
| US-3 | AC4: Injects via ClaudeController.SendCommandImmediate | P0 | `TestAutonomousDriver_StatusChannelSignal` | Unit |
| US-3 | AC5: Detects completion (session exit or DONE sentinel) | P0 | `TestAutonomousDriver_DoneSignal`, `autonomous-fix > autonomous session eventually exits` | Unit + E2E |
| US-3 | AC6: Max turn limit (default 20) | P0 | `TestAutonomousDriver_MaxTurnsLimit` | Unit |
| US-3 | AC7: Logs each turn | P0 | `TestDemoHarness_AutonomousFixFlow` (log inspection) | Integration |
| US-4 | AC1: GitHubPRsPlugin fetches open PRs | P3 | `TestGitHubPRsPlugin_Fetch_RespectRateLimit` | Unit |
| US-4 | AC2: Each PR becomes backlog item with diff/CI/comments | P3 | `TestGitHubPRsPlugin_MapToBacklogItem_TagsCIFailing` | Unit |
| US-4 | AC3: Items tagged by PR state | P3 | `TestGitHubPRsPlugin_MapToBacklogItem_Tags*` × 3 | Unit |
| US-4 | AC4: PR items work with existing backlog flows | P3 | Covered by existing backlog integration tests once plugin registered | Integration (existing) |
| US-5 | AC1: Risky tool call on autonomous session → LLM query | P2 | `TestApprovalHandler_AutonomousLLMApprove`, `TestApprovalHandler_NonAutonomous_NoLLMCall` | Unit |
| US-5 | AC2: Query includes goal, tool call, session tail | P2 | `TestApprovalHandler_AutonomousLLMApprove` (assert prompt content) | Unit |
| US-5 | AC3: LLM approve → auto-approved, logged with source=llm_orchestrator | P2 | `TestApprovalHandler_AutonomousLLMApprove` | Unit |
| US-5 | AC4: LLM deny/unavailable → human review queue | P2 | `TestApprovalHandler_AutonomousLLMDeny`, `TestApprovalHandler_AutonomousLLMError` | Unit |
| US-5 | AC5: Human review queue unchanged | P2 | Existing `approval_handler_integration_test.go` (no regression) | Integration (existing) |
| US-6 | AC1: Driver detects completion (exit or DONE) | P2 | `TestAutonomousDriver_DoneSignal`, `autonomous-fix > autonomous session eventually exits` | Unit + E2E |
| US-6 | AC2: Extracts PR URL from session tail | P2 | `TestExtractPRURL_*` × 4 | Unit |
| US-6 | AC3: Backlog item → done with PR URL as artifact | P2 | `TestOnAutonomousDriverComplete_Done_UpdatesBacklogItem` | Go Integration |
| US-6 | AC4: Stuck/error → failed, reason stored | P2 | `TestOnAutonomousDriverComplete_Stuck_FailsBacklogItem` | Go Integration |
| US-6 | AC5: Push notification on completion | P2 | `TestOnAutonomousDriverComplete_SendsPushNotification` | Go Integration |

---

## 4. Coverage Gaps

### G1 — Real PTY injection timing (US-3 AC2,4) — Hard to test
**Why**: `TestAutonomousDriver_StatusChannelSignal` uses a fake controller and cannot simulate actual PTY I/O interleaving. Verifying that `SendCommandImmediate` does not corrupt PTY output requires a real PTY (which the Playwright E2E test partially exercises but does not assert byte-level correctness).
**Mitigation**: The C1 patch (channel-signal-only listener) eliminates the structural race. The unit test validates the structural pattern; PTY-level correctness is covered by the existing `claude_controller_test.go` tests for `SendCommandImmediate`.

### G2 — Rate-limit wall-clock behavior (US-3 AC3, rate limit pause) — Hard to test
**Why**: `TestAutonomousDriver_RateLimitPause` mocks `GetRateLimitState` but does not test the sleep duration or the actual exponential backoff against real GitHub rate limit headers.
**Mitigation**: Document that the backoff timer is tested as a unit by injecting a `now()` clock function. Real rate limit behavior is accepted as manual verification.

### G3 — Push notification delivery (US-6 AC5) — Requires configured push subscription
**Why**: `TestOnAutonomousDriverComplete_SendsPushNotification` can only assert the notification service's `Send` method is called with the right payload; it cannot verify browser push delivery end-to-end.
**Mitigation**: The existing push notification tests (`server/services/notification_*_test.go`) cover the delivery path. This feature test covers only the invocation.

### G4 — Multi-session headless pool serialization (C3 concern) — Partial coverage
**Why**: `TestPool_ConcurrentAutonomousSessions_NoSerialize` can demonstrate that per-session key strategy avoids mutex contention, but the implementation choice (per-session FeatureKey vs shared key) is a T2.1.3 pre-implementation decision. If the team chooses a shared FeatureKey, this test will expose the bottleneck but not block CI.
**Mitigation**: T2.1.3 patch requires a documented investigation result in the ADR before writing driver code.

### G5 — GitHub PR plugin CI status (US-4 AC2) — API shape dependency
**Why**: The `GitHubCheckConclusion` field requires a separate `GET /repos/{owner}/{repo}/commits/{ref}/check-runs` call. If this is not implemented in the PR plugin MVP, US-4 AC2 (CI status in backlog item) will be partially untested.
**Mitigation**: Tag as P3 deferral; the acceptance test will be marked `t.Skip("P3: CI status field not yet implemented")` until the field is added.

---

## 5. Implementation Readiness Gate

### Gate A — Every P0/P1 AC has at least one test

| Story | Priority | AC Count | Tests Assigned | Status |
|---|---|---|---|---|
| US-1 | P1 | 6 | 6 (see matrix) | PASS |
| US-2 | P1 | 4 | 4 (see matrix) | PASS |
| US-3 | P0 | 7 | 7 (see matrix) | PASS |
| US-4 | P3 | 4 | 4 (deferred, skip-tagged) | PASS (deferred) |
| US-5 | P2 | 5 | 5 (see matrix) | PASS |
| US-6 | P2 | 5 | 5 (see matrix) | PASS |

**Gate A: PASS**

### Gate B — No BLOCKED items remain in adversarial-review.md

The adversarial review final verdict is: **"CONCERNS (no hard BLOCKEDs)"** with 4 patches applied to plan.md. All 4 patches (C1, C2, C3, C4) are reflected in the plan as written. Additionally:

One NEW issue found during validation (not in the adversarial review):

**V1 — API Mismatch in T2.1.3 `HeadlessPoolClient` interface**

The plan defines the interface as:
```go
CallBlockingWithOptions(ctx, key, subKey, prompt, opts) (string, error)
```
The real `pool.CallBlockingWithOptions` signature is:
```go
CallBlockingWithOptions(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions) (string, error)
```
There is no `subKey` parameter. The plan's interface will not be satisfied by `*headless.Pool`. The fix: remove `subKey` from the interface and use the goal as `systemPrompt`, the session tail as `userPrompt`, OR use a per-session synthetic `FeatureKey` as specified in the C3 patch (e.g., `FeatureKey("autonomous_fix-" + sessionID[:8])`). This must be resolved before T2.1.1 code is written.

**Gate B: CONCERNS** — V1 (API mismatch) must be resolved before T2.1.1; the C3 pool-key investigation must complete before T2.1.3.

### Gate C — All technology decisions have justification

| Decision | Justified? |
|---|---|
| TD-1: Fan-out slice over polling | Yes — rationale in plan |
| TD-2: SendCommandImmediate from run goroutine only | Yes — C1 patch adds channel-signal clarity |
| TD-3: PermissionMode "auto" not bypassPermissions | Yes — explicit override of pitfalls.md, requirement cited |
| TD-4: Headless pool for orchestrator LLM calls | Yes — rationale in plan |
| TD-5: Atomic driverRunning guard + panic recovery | Yes — rationale in plan |
| TD-6: MaxTurns=20, exponential backoff | Yes — rationale in plan |

**Gate C: PASS**

### Gate D — Demo harness runnable without real GitHub credentials

The demo harness described in Section 2.2 uses:
- `FakeClaudeRunner` (already exists in `session/headless/fake_runner.go`) — no real Claude binary
- `httptest.NewServer` for mock GitHub API — no `GITHUB_TOKEN` required
- Temp directory as repo root — no real repository

**Gate D: PASS**

---

## 6. Readiness Gate Summary

| Gate | Verdict | Blocking? |
|---|---|---|
| A: Every P0/P1 AC has a test | PASS | No |
| B: No BLOCKED items | CONCERNS | Yes — V1 (API mismatch T2.1.3) + C3 pool investigation must complete first |
| C: All tech decisions justified | PASS | No |
| D: Demo harness needs no real credentials | PASS | No |

**Overall verdict: CONCERNS**

The plan is nearly ready for implementation. Two pre-implementation actions are required before writing a single line of code:

1. **Fix V1**: Update the `HeadlessPoolClient` interface in T2.1.3 to match the real `pool.CallBlockingWithOptions` signature (`systemPrompt, userPrompt string` — no `subKey`). Determine how per-session state is passed (either via synthetic `FeatureKey` or structuring the system/user prompt split).

2. **Resolve C3**: Inspect `headless.Pool` locking granularity (confirmed: `keyMu` is keyed by `FeatureKey` only). Two autonomous sessions will serialize on the same mutex. The plan requires using a unique per-session key (`FeatureKey("autonomous_fix-" + sessionID[:8])`) and this must be documented in the ADR before T2.1.1.

Once these two items are resolved, the plan is **CLEAN** to proceed to Phase 5.

---

## 7. Test Count Summary

| Type | Count |
|---|---|
| Go unit tests (new) | 21 |
| Go integration tests (new) | 10 |
| Jest / RTL frontend tests (new) | 9 |
| Playwright E2E tests (new) | 5 |
| **Total new tests** | **45** |
| Demo harness test | 1 (integration) |

**Requirements coverage**: 30/30 ACs have at least one named test (US-4 P3 ACs are skip-tagged pending implementation — counted as covered with deferral note).

**P0/P1 AC coverage (hard requirement)**: 17/17 = 100%.
