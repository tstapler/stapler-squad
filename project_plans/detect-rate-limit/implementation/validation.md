# Validation Plan: Rate Limit Detection & Auto-Resume

**Project**: detect-rate-limit  
**Date**: 2026-05-02  
**Status**: Ready for implementation

---

## Summary Counts

| Type | Count |
|---|---|
| Unit tests (Go) | 22 |
| Unit tests (Frontend/Jest) | 7 |
| Integration tests | 5 |
| Manual verification steps | 6 |
| **Total automated** | **34** |
| Requirements coverage | 5/5 (100%) |

---

## 1. Requirement → Test Traceability Matrix

| User Story | Acceptance Criteria | Test(s) |
|---|---|---|
| US-1 | Pattern matches "You've hit your limit - resets Xpm (Timezone)" | `TestDetector_ClaudeNewFormat_DetectsRateLimit`, `TestDetector_ClaudeNewFormat_ParsesResetTime` |
| US-1 | Existing patterns still match (no regression) | `TestDetector_ProcessOutput_AnthropicRateLimit`, `TestDetector_ProcessOutput_GeminiRateLimit` |
| US-1 | Timezone-aware timestamp: "11pm America/Los_Angeles" | `TestParseTimeWithTZ_IANAName` |
| US-1 | Timezone-aware timestamp: "11pm PDT" | `TestParseTimeWithTZ_Abbreviation_PDT` |
| US-1 | Timezone-aware timestamp: "11:30pm Pacific" | `TestParseTimeWithTZ_CommonName_Pacific` |
| US-1 | Unknown timezone falls back to Local | `TestParseTimeWithTZ_UnknownTZ_FallsBackToLocal` |
| US-1 | Past time rolls to next day | `TestParseTimeWithTZ_PastTimeGetsNextDay` |
| US-1 | Detection within 2s | `TestDetector_ClaudeNewFormat_DetectsRateLimit` (synchronous assertion) |
| US-1 | Duplicate suppressed during cooldown | `TestDetector_CooldownPreventsImmediateReDetection`, `TestDetector_Cooldown` (existing) |
| US-2 | Fallback to 30-minute wait when reset time is zero | `TestScheduler_FallbackIs30Min` |
| US-2 | Recovery input sent at scheduled time | `TestScheduler_ScheduleRecovery` (existing) |
| US-2 | Do not send recovery if session stopped/paused | `TestScheduler_SkipsRecovery_WhenSessionNotRunning` |
| US-2 | Re-detect after recovery, reschedule with new time | `TestDetector_ReDetectionAfterRecovery` |
| US-2 | Cooldown prevents tight re-detection loop | `TestDetector_CooldownPreventsImmediateReDetection` |
| US-2 | SetState(StateNone) clears currentResetTime | `TestDetector_SetStateNone_ClearsResetTime` |
| US-3 | Detection callback fires server event bus notification | `TestManager_EventBusWiring_OnDetection` |
| US-3 | Recovery callback fires server event bus notification | `TestManager_EventBusWiring_OnRecovery` |
| US-3 | Notification deduplication (no spam) | `TestInstance_RateLimitNotificationDedup` (integration) |
| US-4 | Toggle off cancels pending scheduled recovery | `TestManager_SetEnabled_False_CancelsScheduler` |
| US-4 | Toggle persisted through restart | Manual-1 |
| US-5 | `InstanceToProto()` populates `RateLimitState` | `TestInstanceToProto_RateLimitState_Waiting` |
| US-5 | `InstanceToProto()` populates `RateLimitResetTime` | `TestInstanceToProto_RateLimitState_Waiting` |
| US-5 | `InstanceToProto()` populates `RateLimitEnabled` | `TestInstanceToProto_RateLimitEnabled` |
| US-5 | SessionCard renders badge text with reset time | `SessionCard_getRateLimitStateText_showsResetTime` |
| US-5 | SessionCard clears badge when state returns to None | `SessionCard_getRateLimitStateText_clearsOnNone` |
| US-5 | `SetRateLimitEnabled` RPC returns updated session | `TestSetRateLimitEnabled_RPC` (integration) |

---

## 2. Unit Tests

### Package: `session/detection/ratelimit` — file: `detector_test.go`

All tests in this package are **white-box** (same package `ratelimit`), matching the existing test file convention.

---

#### 2.1 Detection pattern tests (Epic 1, Story 1.1)

**`TestDetector_ClaudeNewFormat_DetectsRateLimit`**  
Package: `ratelimit`  
What it asserts:
- Feed the string `"You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.\n"` to `detector.detectInOutput(output)`.
- Result must be non-nil.
- `result.State` must equal `StateWaiting`.
- `result.Provider` must equal `ProviderAnthropic` (pattern `/rate-limit-options` is absent, but "hit your limit" maps to Anthropic provider patterns after Story 1.1.1 adds the regex).
- Asserts synchronously; no goroutines needed.

**`TestDetector_ClaudeNewFormat_ParsesResetTime`**  
Package: `ratelimit`  
What it asserts:
- Call `detector.detectInOutput(sameOutput)`.
- `result.ResetTime` must not be zero.
- `result.ResetTime.In(la).Hour()` must equal 23 (11pm).
- `result.ResetTime.In(la).Minute()` must equal 0.
- Helper: `la, _ := time.LoadLocation("America/Los_Angeles")`.

---

#### 2.2 Timestamp parsing tests (Epic 1, Story 1.2)

**`TestParseTimeWithTZ_IANAName`**  
Package: `ratelimit`  
What it asserts:
- Call `parseTimeWithTZ("11pm", "America/Los_Angeles")` (private function, accessible within package test).
- Result must not be zero.
- `result.In(la).Hour()` must equal 23.

**`TestParseTimeWithTZ_Abbreviation_PDT`**  
Package: `ratelimit`  
What it asserts:
- Call `parseTimeWithTZ("11pm", "PDT")`.
- Result must not be zero.
- `result.In(la).Hour()` must equal 23.
- Verifies that the `tzAbbreviations` map (Story 1.2.2) correctly maps PDT → America/Los_Angeles.

**`TestParseTimeWithTZ_Abbreviation_PST`**  
Package: `ratelimit`  
What it asserts:
- Call `parseTimeWithTZ("11pm", "PST")`.
- Result must not be zero.
- `result.In(la).Hour()` must equal 23.

**`TestParseTimeWithTZ_CommonName_Pacific`**  
Package: `ratelimit`  
What it asserts:
- Call `parseTimeWithTZ("11:30pm", "Pacific")`.
- Result must not be zero.
- `result.In(la).Hour()` must equal 23, `.Minute()` must equal 30.

**`TestParseTimeWithTZ_UnknownTZ_FallsBackToLocal`**  
Package: `ratelimit`  
What it asserts:
- Call `parseTimeWithTZ("11pm", "FakeZone")`.
- Result must not be zero (fallback to `time.Local` produces a valid time).
- Does NOT panic.

**`TestParseTimeWithTZ_PastTimeGetsNextDay`**  
Package: `ratelimit`  
What it asserts:
- Construct a time string for an hour that has already passed today (e.g., `"1am"` if the test runs after 1am).
- Call `parseTimeWithTZ("1am", "America/Los_Angeles")`.
- `result` must be after `time.Now()` (next-day rollover logic in Story 1.2.3).
- Note: to make this deterministic, the test can use a fixed "now" via a package-level `nowFunc` hook or test at an hour guaranteed to be in the past (midnight-1am edge). The simpler approach: call `parseTimeWithTZ` with the current hour minus 1 and assert result > now.

**`TestParseTimeWithTZ_ParenthesesAreStripped`**  
Package: `ratelimit`  
What it asserts:
- Call `parseTimeWithTZ("11pm", "(America/Los_Angeles)")` (with parens as they appear in the raw message).
- Result must not be zero and hour must be 23 in LA.
- Verifies Story 1.2.3 parenthesis-cleanup logic.

---

#### 2.3 Scheduler fallback test (Epic 1, Story 1.3)

**`TestScheduler_FallbackIs30Min`**  
Package: `ratelimit`  
What it asserts:
- Create `scheduler := NewScheduler("test")`.
- Set a recovery callback that records `time.Now()` when invoked.
- Call `scheduler.ScheduleRecovery(time.Time{})` (zero reset time).
- Call `scheduler.GetScheduledTime()` immediately.
- The scheduled time minus `time.Now()` must be within [29m55s, 30m10s].
- Does NOT wait for the timer to fire (just inspects `resetTime` and internal state).
- Verifies that the bug described in Story 1.3.2 is fixed: zero reset time now triggers 30-minute fallback, not `bufferSeconds` (5s).

**`TestScheduler_SkipsRecovery_WhenSessionNotRunning`**  
Package: `ratelimit`  
What it asserts:
- Create scheduler with `SetSessionStatusCheck(func() bool { return false })`.
- `SetRecoveryCallback` records whether it was called.
- Call `ScheduleRecovery(time.Now().Add(10ms))`.
- Wait 100ms.
- Assert recovery callback was NOT called.

---

#### 2.4 Re-detection after recovery tests (Epic 4)

**`TestDetector_ReDetectionAfterRecovery`**  
Package: `ratelimit`  
What it asserts:
- Create detector, set cooldown to 0 (or 1ms) for speed.
- Call `ProcessOutput` with Claude rate limit message → state becomes `StateWaiting`.
- Simulate recovery: call `detector.SetState(StateNone)`.
- Zero out `detector.lastDetection` (direct field access inside same package).
- Call `ProcessOutput` again with same message.
- Assert `detector.GetState() == StateWaiting` (re-detection succeeded).
- This proves Story 4.1.1/4.1.2 works.

**`TestDetector_CooldownPreventsImmediateReDetection`**  
Package: `ratelimit`  
What it asserts:
- Create detector with `SetCooldown(60 * time.Second)`.
- Trigger first detection (puts state to `StateWaiting`).
- Simulate recovery by calling `SetState(StateNone)` but leave `lastDetection = time.Now()`.
- Immediately call `ProcessOutput` with rate limit message again.
- Assert `detector.GetState() == StateNone` (cooldown blocked re-detection).
- This verifies the anti-tight-loop guarantee from Story 4.2.1.

**`TestDetector_SetStateNone_ClearsResetTime`**  
Package: `ratelimit`  
What it asserts:
- Create detector.
- Call `detectInOutput` to populate `currentResetTime`.
- Assert `GetResetTime()` is non-zero.
- Call `SetState(StateNone)`.
- Assert `GetResetTime().IsZero() == true`.
- Verifies Story 4.1.3: no stale reset time leaks into proto after recovery.

---

#### 2.5 Manager callback wiring tests (Epic 3, Story 3.1)

**`TestManager_EventBusWiring_OnDetection`**  
Package: `ratelimit`  
What it asserts:
- Create manager with a mock `SessionAccessor`.
- Register an external detection callback via `manager.SetDetectionCallback(fn)` (Story 3.1.4).
- Call `manager.ProcessOutput(claudeRateLimitMessage)`.
- Assert the external callback was called with a `Detection` whose `Provider == ProviderAnthropic` (or similar).
- Assert `manager.GetState() == StateWaiting`.
- This is distinct from the existing `TestManager_ProcessOutput` which only checks state; this checks the external callback fires.

**`TestManager_EventBusWiring_OnRecovery`**  
Package: `ratelimit`  
What it asserts:
- Create manager with nil instance (no PTY; recovery will fail gracefully).
- Register external recovery callback via `manager.SetRecoveryCallback(fn)`.
- Directly invoke `manager.executeRecovery()`.
- Assert recovery callback was called with `success=false` (nil instance = write fails).
- Verifies Story 3.1.5.

**`TestManager_SetEnabled_False_CancelsScheduler`**  
Package: `ratelimit`  
What it asserts:
- Create manager, trigger detection to start a scheduled recovery.
- Assert `manager.GetScheduler().IsScheduled() == true`.
- Call `manager.SetEnabled(false)`.
- Wait 50ms.
- Assert `manager.GetScheduler().IsScheduled() == false` (scheduler cancelled).
- Verifies US-4 acceptance criterion: toggling off cancels pending recovery.

---

#### 2.6 Manager GetResetTime delegation (Epic 2, Story 2.1)

**`TestManager_GetResetTime_DelegatesToDetector`**  
Package: `ratelimit`  
What it asserts:
- Create manager, trigger detection with Claude new-format message.
- Call `manager.GetResetTime()` (Story 2.1.2 new method).
- Assert result is non-zero.
- Assert result matches `manager.GetDetector().GetResetTime()`.

---

### Package: `server/adapters` — file: `instance_adapter_test.go` (new file)

These tests use a `fakeInstance` struct that implements only the interface methods needed by `InstanceToProto`.

**`TestInstanceToProto_RateLimitState_Waiting`**  
Package: `adapters`  
What it asserts:
- Construct a `session.Instance` (or use `fakeInstance` stub) where `GetRateLimitState()` returns `int(ratelimit.StateWaiting)` and `GetRateLimitResetTime()` returns `time.Now().Add(1 * time.Hour)`.
- Call `InstanceToProto(inst)`.
- Assert `proto.RateLimitState == sessionv1.RateLimitState_RATE_LIMIT_STATE_WAITING`.
- Assert `proto.RateLimitResetTime` is non-nil.
- Assert `proto.RateLimitResetTime.AsTime()` is within 5 seconds of the input time.
- Covers Stories 2.4.2 and 2.4.3.

**`TestInstanceToProto_RateLimitState_None`**  
Package: `adapters`  
What it asserts:
- `GetRateLimitState()` returns `int(ratelimit.StateNone)`, `GetRateLimitResetTime()` returns zero.
- Assert `proto.RateLimitState == sessionv1.RateLimitState_RATE_LIMIT_STATE_NONE`.
- Assert `proto.RateLimitResetTime` is nil (zero time must not emit a timestamp).

**`TestInstanceToProto_RateLimitEnabled`**  
Package: `adapters`  
What it asserts:
- `IsRateLimitEnabled()` returns `true`.
- Assert `proto.RateLimitEnabled == true`.
- Repeat with `false`.
- Covers Story 2.4.4.

**`TestInstanceToProto_RateLimitStateToProto_AllStates`**  
Package: `adapters`  
What it asserts:
- Table-driven test over all five `ratelimit.RateLimitState` constants.
- For each state, call `rateLimitStateToProto(state)` directly (private function; test is in same package) and assert it maps to the expected proto enum value.
- Covers Story 2.4.1.

---

### Package: `web-app/src/components/sessions` — file: `SessionCard.test.tsx` (new file)

**`SessionCard_getRateLimitStateText_showsResetTime`**  
Framework: Jest + React Testing Library  
What it asserts:
- Render `SessionCard` with a session proto where `rateLimitState = RateLimitState.WAITING` and `rateLimitResetTime = { seconds: BigInt(future epoch), nanos: 0 }`.
- Assert the badge element contains text matching `/Rate limited until \d{1,2}:\d{2}/i`.
- Verifies Story 5.1.3.

**`SessionCard_getRateLimitStateText_clearsOnNone`**  
Framework: Jest + React Testing Library  
What it asserts:
- Render with `rateLimitState = RateLimitState.NONE`.
- Assert no badge element with rate-limit text is present in the DOM.
- Verifies US-5 acceptance criterion: badge cleared when state is None.

**`SessionCard_getRateLimitStateText_showsRecovering`**  
Framework: Jest + React Testing Library  
What it asserts:
- Render with `rateLimitState = RateLimitState.RECOVERING`.
- Assert badge text is "Recovering..." (no crash if `rateLimitResetTime` is undefined).

**`SessionCard_formatResetTime_handlesUndefined`**  
Framework: Jest (unit, not rendering)  
What it asserts:
- Import `formatResetTime` as an exported helper (or test it indirectly via render).
- Call with `undefined` → returns `""`.
- Call with zero timestamp `{ seconds: BigInt(0), nanos: 0 }` → returns `""`.
- Prevents runtime crashes when proto field absent.
- Covers Story 5.1.2.

**`SessionCard_autoResumeToggle_callsSetRateLimitEnabled`**  
Framework: Jest + React Testing Library  
What it asserts:
- Provide a mock `setRateLimitEnabled` via `SessionServiceContext`.
- Render `SessionCard` with `rateLimitEnabled = true`.
- Find the overflow menu item "Disable auto-resume".
- Click it.
- Assert `mockSetRateLimitEnabled` was called with `(sessionId, false)`.
- Covers Story 5.2.7.

**`SessionCard_autoResumeToggle_labelFlipsWhenDisabled`**  
Framework: Jest + React Testing Library  
What it asserts:
- Render with `rateLimitEnabled = false`.
- Assert overflow menu contains "Enable auto-resume" (not "Disable").
- Confirms label inversion logic is correct.

**`useSessionService_setRateLimitEnabled_callsRPC`**  
Framework: Jest  
What it asserts:
- Mock the ConnectRPC transport.
- Call `useSessionService().setRateLimitEnabled("session-id", false)`.
- Assert the RPC was called with `{ sessionId: "session-id", enabled: false }`.
- Covers Story 5.2.5.

---

## 3. Integration Tests

Integration tests require multiple components wired together. They live in `server/services/` or a dedicated `_integration_test.go` file tagged with `//go:build integration` or run as part of standard `go test ./...` with no build tag (whichever matches the project convention for existing integration tests in `approval_handler_integration_test.go`).

---

**`TestSetRateLimitEnabled_RPC`**  
File: `server/services/session_service_ratelimit_test.go`  
Components involved: `SessionService` handler + in-memory `SessionStorage` + `session.Instance`  
What it asserts:
- Create a minimal session instance with rate limit enabled.
- Call `SetRateLimitEnabled` ConnectRPC handler with `{ sessionId, enabled: false }`.
- Assert returned `Session.rateLimitEnabled == false`.
- Assert `inst.IsRateLimitEnabled() == false`.
- Covers Story 5.2.3 and US-4 persistence.

**`TestInstance_RateLimitNotificationDedup`**  
File: `server/services/session_service_ratelimit_test.go`  
Components involved: `session.Instance` + `server/events.EventBus`  
What it asserts:
- Wire a real `EventBus` to an instance via `SetEventBus`.
- Subscribe to `EventNotification`.
- Trigger two rate limit detections within the dedup window (< N seconds).
- Assert only ONE `EventNotification` event is published.
- Verifies US-3 acceptance criterion: "Notifications respect existing NotificationRateLimiter".

**`TestRateLimitPipeline_DetectionToProto`**  
File: `server/services/session_service_ratelimit_test.go`  
Components involved: `ratelimit.Manager` + `session.Instance` + `adapters.InstanceToProto`  
What it asserts:
- Feed Claude rate limit text to `manager.ProcessOutput`.
- Allow state to propagate to instance (via callback wiring from Epic 3).
- Call `InstanceToProto(inst)`.
- Assert `proto.RateLimitState == RATE_LIMIT_STATE_WAITING`.
- Assert `proto.RateLimitResetTime` non-nil, within 10s of expected.
- This is the end-to-end pipeline test for US-5.

**`TestRateLimitPipeline_RecoveryResetsState`**  
File: `server/services/session_service_ratelimit_test.go`  
Components involved: `ratelimit.Manager` + `ratelimit.Scheduler` + `session.Instance`  
What it asserts:
- Trigger detection (state → Waiting).
- Force execute recovery via `manager.executeRecovery()` (simulate PTY write succeeding via mock).
- Assert `manager.GetState() == StateRecovered` immediately after.
- Assert `manager.GetDetector().GetResetTime().IsZero() == true` (cleared per Story 4.1.3).
- Set cooldown to 0, feed rate limit message again.
- Assert state transitions back to `StateWaiting` (re-detection works end-to-end).
- Covers US-2 "continue monitoring after recovery" and US-2 re-detection loop.

**`TestRateLimitPipeline_EventBusNotificationFired`**  
File: `server/services/session_service_ratelimit_test.go`  
Components involved: `session.Instance` + `server/events.EventBus` + `ratelimit.Manager` callbacks  
What it asserts:
- Subscribe to `EventNotification` and `EventSessionUpdated` on the bus.
- Feed rate limit message to instance's PTY consumer.
- Assert `EventNotification` received with title containing "rate limited".
- Assert `EventSessionUpdated` received (so WebSocket clients see updated state).
- Covers US-3 notification and US-5 real-time state stream.

---

## 4. Tests That Must NOT Regress

The following tests exist in `session/detection/ratelimit/detector_test.go` and must remain green after all Epic 1–4 changes. Any failure in these indicates a regression:

| Test Name | What it Guards |
|---|---|
| `TestDetector_ProcessOutput_AnthropicRateLimit` | Existing Anthropic pattern still fires |
| `TestDetector_ProcessOutput_GeminiRateLimit` | Existing Gemini pattern still fires with parsed reset time |
| `TestDetector_ProcessOutput_NoRateLimit` | Normal output does not trigger false positive |
| `TestDetector_ProcessOutput_FalsePositive` | Docs-style rate-limit mention does not trigger |
| `TestDetector_Cooldown` | Cooldown logic not broken by new patterns |
| `TestDetector_IdentifyProvider_Anthropic` | Provider identification patterns not altered |
| `TestDetector_IdentifyProvider_OpenAI` | Provider identification patterns not altered |
| `TestScheduler_ScheduleRecovery` | Scheduler fires callback at scheduled time |
| `TestScheduler_CancelRecovery` | Cancel prevents callback execution |
| `TestRecoveryHandler_Execute` | Recovery handler sends correct input |
| `TestRecoveryHandler_Execute_Error` | Recovery handler propagates errors |
| `TestEventBus_Subscribe_Publish` | Internal event bus still routes messages |
| `TestManager_ProcessOutput` | Manager delegates to detector correctly |
| `TestManager_Disable` | Disabled manager ignores output |
| `TestStripANSI` | ANSI stripping not broken |
| `TestDetector_StateTransitions` | All state transitions still valid |
| `TestParseTimestamp_RetryAfter` | Retry-after duration parsing still works |
| `TestParseTimestamp_SpecificTime` | Time-of-day parsing still works |
| `TestDetector_ConcurrentProcessOutput` | No data races under concurrency |

Additionally, the following must remain green as they test adjacent layers:

| Package / File | Tests |
|---|---|
| `server/events/bus_test.go` | All existing bus tests |
| `server/services/session_service_create_test.go` | All existing session creation tests |
| `server/services/approval_service_test.go` | All existing approval tests |
| `server/adapters/instance_adapter_test.go` | All existing adapter tests (once file exists) |

Run `make build && make test` to verify no regressions before opening a PR.

---

## 5. Manual Verification Steps

These scenarios involve timing, live PTY sessions, or frontend interaction that cannot be fully automated in CI.

**Manual-1: Per-session toggle persists through restart**  
Steps:
1. Start the server: `make restart-web`.
2. Create a session via the web UI.
3. Open the session card overflow menu, click "Disable auto-resume".
4. Verify the menu item changes to "Enable auto-resume".
5. Stop and restart the server: `make restart-web`.
6. Open the same session card.
7. Verify the overflow menu still shows "Enable auto-resume" (state was persisted).  
Expected: Toggle state survives server restart.  
Covers: US-4 "Toggle state persisted through session restart".

**Manual-2: End-to-end rate limit detection in live session**  
Steps:
1. Start `make restart-web`.
2. Create a Claude Code session.
3. In the session terminal, paste: `You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.`
4. Within 2 seconds, observe the session card badge changes to "Rate limited until 11:00 PM".
5. Verify the status badge shows the clock icon.  
Expected: Detection fires within 2s, badge reflects WAITING state with reset time.  
Covers: US-1 (2s detection SLA), US-5 (badge rendering).

**Manual-3: Desktop notification on detection**  
Steps:
1. Ensure browser notification permission is granted for `localhost:8543`.
2. Repeat Manual-2 steps 1–4.
3. Observe a desktop push notification titled approximately "Session [name] rate limited — resumes at 11pm".  
Expected: Desktop notification fires once within 5 seconds of detection.  
Covers: US-3 "Desktop push notification on detection".

**Manual-4: Auto-resume at scheduled time**  
Steps:
1. Create a session.
2. Inject the rate-limit message with a reset time 2 minutes in the future (e.g., paste a message saying "resets at [now+2min]").
3. Wait for the scheduler to fire.
4. Observe `1\n` being written to the terminal (visible as a "1" + Enter in the session output).
5. Observe a desktop notification: "Session [name] resumed after rate limit".  
Expected: Recovery input sent at scheduled time, notification fires.  
Covers: US-2, US-3.

**Manual-5: Toggle off prevents auto-resume**  
Steps:
1. Create a session, inject a rate-limit message with reset time 5 minutes out.
2. Immediately click "Disable auto-resume" in the overflow menu.
3. Wait 5+ minutes.
4. Verify no `1\n` is sent to the terminal and no recovery notification appears.  
Expected: Cancellation of pending scheduler.  
Covers: US-4 "Toggling off cancels any pending scheduled recovery".

**Manual-6: Re-detection after failed recovery**  
Steps:
1. Inject rate-limit message.
2. Allow scheduler to fire and simulate recovery failure (e.g., session is paused so `WriteToPTY` returns an error).
3. Inject the rate-limit message again.
4. Verify the session card re-enters WAITING state with the new reset time.
5. Verify a desktop notification fires for the second detection (after cooldown period).  
Expected: Re-detection loop works; cooldown prevents immediate spam but fires after 30s.  
Covers: US-2 re-detection, US-3 notification on re-detection.

---

## 6. Test Infrastructure Notes

### Determinism for time-sensitive tests
- `TestParseTimeWithTZ_PastTimeGetsNextDay`: inject a package-level `var nowFunc = time.Now` into `detector.go` so tests can set `nowFunc = func() time.Time { return fixedTime }`. The implementation plan does not specify this hook; add it if the test proves flaky.
- `TestScheduler_FallbackIs30Min`: inspect `scheduler.resetTime` internal field (accessible within same package) rather than waiting 30 minutes for the timer to fire.
- `TestDetector_ReDetectionAfterRecovery`: zero out `detector.lastDetection` directly (same-package white-box access), or use `detector.SetCooldown(0)` to skip the cooldown.

### Proto regeneration requirement
Stories 2.3 and 5.2 add proto fields. Tests in `server/adapters/instance_adapter_test.go` and `server/services/session_service_ratelimit_test.go` will not compile until `make generate-proto` has been run. Run this as part of Epic 2 setup before writing those tests.

### Frontend test setup
`SessionCard.test.tsx` requires:
- A `SessionServiceContext` provider wrapper with mocked `setRateLimitEnabled`.
- A minimal session proto factory that sets `rateLimitState`, `rateLimitResetTime`, and `rateLimitEnabled`.
- Follow the pattern established in `ResumeSessionModal.test.tsx` for context provider wrapping.

### `instance_adapter_test.go` stub approach
Because `session.Instance` has a large surface area, use a `fakeInstance` struct that embeds `session.Instance` by value and overrides only `GetRateLimitState()`, `GetRateLimitResetTime()`, and `IsRateLimitEnabled()` via method promotion or a separate interface. Alternatively, call the real `InstanceToProto` after constructing a minimal `session.Instance` via the existing test helpers used in `session_service_create_test.go`.
