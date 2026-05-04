# Implementation Plan: Rate Limit Detection & Auto-Resume

**Project**: detect-rate-limit
**Date**: 2026-05-02
**Status**: Ready for implementation

---

## Overview

Six epics covering Go detection improvements, state propagation, notification wiring, re-detection
resilience, frontend UI, and tests. The work is layered bottom-up: correct detection must exist before
state can propagate, state propagation must exist before frontend renders correctly, and notifications
ride on top of that pipeline.

---

## Epic 1: Detection Patterns & Timestamp Parsing

**Goal**: Make the Claude "You've hit your limit - resets 11pm (America/Los_Angeles)" message trigger
detection reliably with a correct timezone-aware reset time.

### Story 1.1 — Add missing detection patterns

**File**: `session/detection/ratelimit/detector.go`

| # | What | Where / How |
|---|---|---|
| 1.1.1 | Add rate-limit regex to `defaultRateLimitPatterns` | Append `regexp.MustCompile("(?i)you'?ve hit your (usage )?limit")` to the slice literal |
| 1.1.2 | Add continue regex to `defaultContinuePatterns` | Append `regexp.MustCompile("(?i)/extra-usage")` |
| 1.1.3 | Add two-group timestamp regex to `defaultTimestampPatterns` | Append `regexp.MustCompile("(?i)resets\\s+(\\d{1,2}(?::\\d{2})?(?:am\|pm))\\s*\\(?([\\w/]+)\\)?")` as the last entry; this needs special handling because it has two capture groups (see 1.2.3) |

### Story 1.2 — Timezone-aware timestamp parsing

**File**: `session/detection/ratelimit/detector.go`

| # | What | Where / How |
|---|---|---|
| 1.2.1 | Embed IANA tzdata | Add `import _ "time/tzdata"` to the import block so `time.LoadLocation` works on systems without system tzdata |
| 1.2.2 | Add `tzAbbreviations` map | New package-level `var tzAbbreviations = map[string]string{ "PST": "America/Los_Angeles", "PDT": "America/Los_Angeles", "MST": "America/Denver", "MDT": "America/Denver", "CST": "America/Chicago", "CDT": "America/Chicago", "EST": "America/New_York", "EDT": "America/New_York", "UTC": "UTC", "GMT": "UTC", "Pacific": "America/Los_Angeles", "Mountain": "America/Denver", "Central": "America/Chicago", "Eastern": "America/New_York" }` |
| 1.2.3 | Add `parseTimeWithTZ(timeStr, tzStr string) time.Time` | New private function; cleans up parentheses from tzStr, tries `time.LoadLocation(tzStr)` first, falls back to `tzAbbreviations` lookup, defaults to `time.Local`; calls `time.ParseInLocation` with formats `{"3pm","3:04pm","3 PM","3:04 PM"}`; anchors result to today-in-that-location; adds 24h if result is in the past |
| 1.2.4 | Update `extractTimestamp()` to call `parseTimeWithTZ` | When a timestamp regex has exactly 2 non-empty submatches, call `parseTimeWithTZ(matches[1], matches[2])` instead of `parseTimestamp(matches[1])` |

### Story 1.3 — Fix 30-minute fallback

**File**: `session/detection/ratelimit/scheduler.go`

| # | What | Where / How |
|---|---|---|
| 1.3.1 | Add `DefaultFallbackWait = 30 * time.Minute` constant | New exported const alongside `DefaultResetBuffer` |
| 1.3.2 | Use `DefaultFallbackWait` when `resetTime.IsZero()` | In `ScheduleRecovery`, change `time.AfterFunc(s.bufferSeconds, ...)` fallback to `time.AfterFunc(DefaultFallbackWait + s.bufferDuration(), ...)` so the fallback is 30 min + buffer, not just `bufferSeconds` |

### Story 1.4 — Testdata file

**File**: `session/detection/ratelimit/testdata/claude_rate_limit_new_format.txt` (new file)

| # | What | Where / How |
|---|---|---|
| 1.4.1 | Create testdata file | Content: `"You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.\n"` |

---

## Epic 2: State Propagation Pipeline

**Goal**: Ensure `RateLimitState` and reset time are surfaced all the way from `Detector` through
`Instance` to the proto `Session` message sent to the frontend.

### Story 2.1 — Expose reset time from Detector and Manager

**File**: `session/detection/ratelimit/detector.go`

| # | What | Where / How |
|---|---|---|
| 2.1.1 | Add `GetResetTime() time.Time` method to `Detector` | Returns `d.currentResetTime` (already stored in the struct); requires a read lock |

**File**: `session/detection/ratelimit/manager.go`

| # | What | Where / How |
|---|---|---|
| 2.1.2 | Add `GetResetTime() time.Time` method to `Manager` | Delegates to `m.detector.GetResetTime()` under the existing mutex pattern |

**File**: `session/detection/ratelimit/integration.go`

| # | What | Where / How |
|---|---|---|
| 2.1.3 | Add `GetResetTime() time.Time` method to `PTYConsumer` | Delegates to `p.manager.GetResetTime()` |

### Story 2.2 — Thread reset time through ClaudeController and Instance

**File**: `session/claude_controller.go`

| # | What | Where / How |
|---|---|---|
| 2.2.1 | Add `GetRateLimitResetTime() time.Time` method | Delegates to `c.rateLimitHandler.GetResetTime()` if handler is non-nil; returns zero value otherwise |

**File**: `session/instance.go`

| # | What | Where / How |
|---|---|---|
| 2.2.2 | Add `GetRateLimitResetTime() time.Time` method | Calls through to `ctrl.GetRateLimitResetTime()` following the same guard pattern as `GetRateLimitState()` at line ~2718 |

### Story 2.3 — Add proto fields

**File**: `proto/session/v1/types.proto`

| # | What | Where / How |
|---|---|---|
| 2.3.1 | Add `google.protobuf.Timestamp rate_limit_reset_time` field | Add `import "google/protobuf/timestamp.proto";` if not present; add `google.protobuf.Timestamp rate_limit_reset_time = 41;` to the `Session` message (next available field after `rate_limit_state = 40`) |
| 2.3.2 | Add `bool rate_limit_enabled` field | Add `bool rate_limit_enabled = 42;` to the `Session` message |
| 2.3.3 | Run `make generate-proto` | Regenerates `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` |

### Story 2.4 — Populate adapter

**File**: `server/adapters/instance_adapter.go`

| # | What | Where / How |
|---|---|---|
| 2.4.1 | Add `rateLimitStateToProto` helper | New private function mapping `ratelimit.StateNone→RATE_LIMIT_STATE_NONE`, `StateWaiting→RATE_LIMIT_STATE_WAITING`, `StateRecovering→RATE_LIMIT_STATE_RECOVERING`, `StateRecovered→RATE_LIMIT_STATE_RECOVERED`, `StateFailed→RATE_LIMIT_STATE_FAILED`; default to NONE |
| 2.4.2 | Populate `RateLimitState` in `InstanceToProto()` | Add `protoSession.RateLimitState = rateLimitStateToProto(ratelimit.RateLimitState(inst.GetRateLimitState()))` |
| 2.4.3 | Populate `RateLimitResetTime` in `InstanceToProto()` | Add: if `t := inst.GetRateLimitResetTime(); !t.IsZero() { protoSession.RateLimitResetTime = timestamppb.New(t) }` (requires `import "google.golang.org/protobuf/types/known/timestamppb"`) |
| 2.4.4 | Populate `RateLimitEnabled` in `InstanceToProto()` | Add `protoSession.RateLimitEnabled = inst.IsRateLimitEnabled()` |

---

## Epic 3: Notifications & Event Bus Wiring

**Goal**: Publish server-level events when rate limit is detected or recovery completes, so WebSocket
clients receive `SessionUpdated` events and desktop/toast notifications fire.

### Story 3.1 — Add callback hooks to Integration

**File**: `session/detection/ratelimit/integration.go`

| # | What | Where / How |
|---|---|---|
| 3.1.1 | Add `OnDetection func(Detection)` field to `Integration` struct | New exported function field; nil-safe before call |
| 3.1.2 | Add `OnRecovery func(success bool, det Detection)` field to `Integration` struct | New exported function field; nil-safe before call |
| 3.1.3 | Wire `OnDetection` callback in `Manager.handleDetection()` | After publishing to internal bus, call `i.OnDetection(det)` if non-nil; `Integration` must pass itself a callback from its `Start()` method |

**Note**: The cleanest wiring is for `Manager` to accept optional callbacks rather than callbacks on
`Integration`. Either approach works; callbacks on `Integration` avoids changing the `Manager`
constructor signature, which is the lower-risk choice given existing tests.

**File**: `session/detection/ratelimit/manager.go`

| # | What | Where / How |
|---|---|---|
| 3.1.4 | Add `SetDetectionCallback(fn func(Detection))` to `Manager` | Stores fn in a field; called in `handleDetection` after internal bus publish |
| 3.1.5 | Add `SetRecoveryCallback(fn func(success bool, det Detection))` to `Manager` | Called in `executeRecovery` after `eventRecoveryDone` / `eventRecoveryFail` |

### Story 3.2 — Wire callbacks from session.Instance

**File**: `session/claude_controller.go`

| # | What | Where / How |
|---|---|---|
| 3.2.1 | Add `SetRateLimitCallbacks(onDetect func(ratelimit.Detection), onRecover func(bool, ratelimit.Detection))` | Delegates to `c.rateLimitHandler.manager.SetDetectionCallback` and `SetRecoveryCallback` |

**File**: `session/instance.go`

| # | What | Where / How |
|---|---|---|
| 3.2.2 | Call `SetRateLimitCallbacks` after controller creation | In the `Start()` method or wherever `ctrl` is initialized; pass closures that reference `inst.instanceEventPublisher` |
| 3.2.3 | Add `instanceEventPublisher` field | A `func(eventType string, payload interface{})` or typed interface; set at construction time by the server layer |

### Story 3.3 — Publish server events from Instance callbacks

**File**: `session/instance.go`

| # | What | Where / How |
|---|---|---|
| 3.3.1 | Add `SetEventBus(bus *events.EventBus)` method to `Instance` | Stores reference; used by callbacks |
| 3.3.2 | Implement detection callback | Closure that calls `events.NewNotificationEvent(...)` for "Session X rate limited — resumes at Y" and publishes it; also publishes `EventSessionUpdated` so `RateLimitState` badge updates in real time |
| 3.3.3 | Implement recovery callback | Closure publishing success notification ("Session X resumed") or failure notification ("Session X failed to resume"); also publishes `EventSessionUpdated` |

**File**: `server/services/session_service.go` (or wherever sessions are created)

| # | What | Where / How |
|---|---|---|
| 3.3.4 | Call `inst.SetEventBus(serverEventBus)` after creating a session instance | The server already has `*events.EventBus`; wire it in at session creation time |

### Story 3.4 — Add notification event types (optional but cleaner)

**File**: `server/events/types.go`

| # | What | Where / How |
|---|---|---|
| 3.4.1 | Add `EventRateLimitDetected EventType = "session.rate_limit_detected"` | Optional — rate limit notifications can reuse `EventNotification`; add this only if distinguishing event types in logging/metrics is desired |
| 3.4.2 | Add `EventRateLimitRecovered EventType = "session.rate_limit_recovered"` | Same caveat as 3.4.1 |

---

## Epic 4: Re-detection After Recovery

**Goal**: After recovery input is sent, the detector must return to `StateNone` so a subsequent
rate-limit pattern in output triggers a fresh detection cycle with the new reset time.

### Story 4.1 — Reset state after recovery

**File**: `session/detection/ratelimit/manager.go`

| # | What | Where / How |
|---|---|---|
| 4.1.1 | Call `m.detector.SetState(StateNone)` after successful recovery | In `executeRecovery()`, after publishing `eventRecoveryDone`, reset detector state; the existing `lastDetection` timestamp + cooldown already throttles immediate re-triggers |
| 4.1.2 | Call `m.detector.SetState(StateNone)` after failed recovery too | In the failure branch as well, so the detector watches for the next rate-limit message regardless of whether this attempt succeeded |

**File**: `session/detection/ratelimit/detector.go`

| # | What | Where / How |
|---|---|---|
| 4.1.3 | Verify `SetState(StateNone)` clears `currentResetTime` | In `SetState`, if the new state is `StateNone`, also zero out `d.currentResetTime`; this prevents stale reset time from leaking into the proto after recovery |

### Story 4.2 — Validate cooldown prevents tight loop

**File**: `session/detection/ratelimit/detector.go`

| # | What | Where / How |
|---|---|---|
| 4.2.1 | Confirm cooldown check fires before state check | In `ProcessOutput`, the `lastDetection + cooldown` guard must run before state is inspected; verify order is correct and document with a comment |

---

## Epic 5: Frontend UI

**Goal**: Surface reset time in the badge, add per-session toggle to the overflow menu, and verify the
full pipeline works end-to-end.

### Story 5.1 — Display reset time in badge

**File**: `web-app/src/components/sessions/SessionCard.tsx`

| # | What | Where / How |
|---|---|---|
| 5.1.1 | Import `Timestamp` from generated proto | Add `Timestamp` to imports from `@/gen/session/v1/types_pb` or use `session.rateLimitResetTime` directly as a `Timestamp` object |
| 5.1.2 | Add `formatResetTime(ts: Timestamp \| undefined): string` helper | Returns `""` if undefined or zero; formats as `"until 11:00 PM"` using `new Date(Number(ts.seconds) * 1000).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'})` |
| 5.1.3 | Update badge text for WAITING state | Change `getRateLimitStateText` for `WAITING` to return `"Rate limited" + (resetStr ? " " + resetStr : "")` where `resetStr = formatResetTime(session.rateLimitResetTime)` |

### Story 5.2 — Per-session toggle

**File**: `proto/session/v1/session.proto`

| # | What | Where / How |
|---|---|---|
| 5.2.1 | Add `SetRateLimitEnabled` RPC | Add `rpc SetRateLimitEnabled(SetRateLimitEnabledRequest) returns (SetRateLimitEnabledResponse)` to `SessionService`; request has `string session_id = 1; bool enabled = 2;`; response has `Session session = 1;` |
| 5.2.2 | Run `make generate-proto` | Regenerates bindings |

**File**: `server/services/session_service.go`

| # | What | Where / How |
|---|---|---|
| 5.2.3 | Implement `SetRateLimitEnabled` handler | Looks up session by ID, calls `inst.SetRateLimitEnabled(req.Enabled)`, cancels scheduled recovery if `enabled=false` via `inst.ctrl.rateLimitHandler.manager.scheduler.CancelRecovery()`, publishes `EventSessionUpdated`, returns updated `InstanceToProto()` |

**File**: `server/server.go`

| # | What | Where / How |
|---|---|---|
| 5.2.4 | Register new RPC handler | Wire `SetRateLimitEnabled` path in the ConnectRPC mount block following existing pattern |

**File**: `web-app/src/lib/hooks/useSessionService.ts`

| # | What | Where / How |
|---|---|---|
| 5.2.5 | Add `setRateLimitEnabled(sessionId: string, enabled: boolean): Promise<Session \| null>` | Calls the new RPC; follows existing hook pattern for `pauseSession`/`resumeSession` |

**File**: `web-app/src/lib/contexts/SessionServiceContext.tsx`

| # | What | Where / How |
|---|---|---|
| 5.2.6 | Expose `setRateLimitEnabled` from context | Add to `SessionServiceContextValue` interface and to the context provider value object |

**File**: `web-app/src/components/sessions/SessionCard.tsx`

| # | What | Where / How |
|---|---|---|
| 5.2.7 | Add toggle to overflow menu | Add a menu item "Disable auto-resume" / "Enable auto-resume" that calls `setRateLimitEnabled(session.id, !session.rateLimitEnabled)` following the existing menu item pattern |

---

## Epic 6: Tests

**Goal**: Prevent regressions and document the new behavior with focused, fast unit tests.

### Story 6.1 — Detector pattern tests

**File**: `session/detection/ratelimit/detector_test.go`

| # | What | Where / How |
|---|---|---|
| 6.1.1 | `TestDetector_ClaudeNewFormat_DetectsRateLimit` | Feed `"You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage..."` to `ProcessOutput`; assert state becomes `StateWaiting` |
| 6.1.2 | `TestDetector_ClaudeNewFormat_ParsesResetTime` | Assert `GetResetTime()` returns a non-zero time whose UTC hour matches 11pm America/Los_Angeles adjusted to UTC |

### Story 6.2 — Timestamp parsing tests

**File**: `session/detection/ratelimit/detector_test.go`

| # | What | Where / How |
|---|---|---|
| 6.2.1 | `TestParseTimeWithTZ_IANAName` | `parseTimeWithTZ("11pm", "America/Los_Angeles")` → non-zero, hour == 23 in LA timezone |
| 6.2.2 | `TestParseTimeWithTZ_Abbreviation_PDT` | `parseTimeWithTZ("11pm", "PDT")` → non-zero, same hour check |
| 6.2.3 | `TestParseTimeWithTZ_CommonName_Pacific` | `parseTimeWithTZ("11:30pm", "Pacific")` → non-zero, hour==23 minute==30 |
| 6.2.4 | `TestParseTimeWithTZ_UnknownTZ_FallsBackToLocal` | `parseTimeWithTZ("11pm", "FakeZone")` → non-zero (falls back to Local) |
| 6.2.5 | `TestParseTimeWithTZ_PastTimeGetsNextDay` | If 11pm has already passed today, returned time is tomorrow |

### Story 6.3 — Scheduler fallback test

**File**: `session/detection/ratelimit/detector_test.go`

| # | What | Where / How |
|---|---|---|
| 6.3.1 | `TestScheduler_FallbackIs30Min` | Create a `Scheduler`, call `ScheduleRecovery(time.Time{})` (zero = no reset time), assert scheduled time is approximately now + 30 min |

### Story 6.4 — Re-detection after recovery test

**File**: `session/detection/ratelimit/detector_test.go`

| # | What | Where / How |
|---|---|---|
| 6.4.1 | `TestDetector_ReDetectionAfterRecovery` | Trigger detection, call `SetState(StateRecovered)` then `SetState(StateNone)` (simulating recovery), feed rate-limit output again; assert state returns to `StateWaiting` (proves re-detection works) |
| 6.4.2 | `TestDetector_CooldownPreventsImmediateReDetection` | After forced reset to `StateNone` with cooldown set to 60s, immediately feed rate-limit output; assert state stays `StateNone` |

### Story 6.5 — Adapter unit test

**File**: `server/adapters/instance_adapter_test.go` (new or existing)

| # | What | Where / How |
|---|---|---|
| 6.5.1 | `TestInstanceToProto_RateLimitState_Waiting` | Mock `Instance` with `GetRateLimitState()=StateWaiting` and `GetRateLimitResetTime()=time.Now().Add(1h)`; call `InstanceToProto()`; assert `proto.RateLimitState == RATE_LIMIT_STATE_WAITING` and `proto.RateLimitResetTime` non-nil |
| 6.5.2 | `TestInstanceToProto_RateLimitEnabled` | Assert `proto.RateLimitEnabled == true` when `IsRateLimitEnabled()` returns true |

### Story 6.6 — Integration smoke test (optional / CI-gated)

**File**: `server/services/session_service_test.go` or `tests/e2e/`

| # | What | Where / How |
|---|---|---|
| 6.6.1 | `TestSetRateLimitEnabled_RPC` | Call `SetRateLimitEnabled` RPC with `enabled=false`; assert returned `Session.rateLimitEnabled == false` |

---

## Dependency Order

```
Epic 1 (patterns + parsing)
  → Epic 4 (re-detection, depends on SetState behavior in detector.go)
  → Epic 2 (GetResetTime chain, depends on detector storing parsed time)
    → Epic 3 (callbacks reference Instance; InstanceToProto must be complete)
      → Epic 5 (frontend reads proto fields; toggle RPC depends on session_service.go)
        → Epic 6 (tests validate the full stack)
```

Epics 1 and 4 can be implemented together in the same PR since both touch `detector.go` / `manager.go`.
Epic 2 and 3 are best combined (both touch `instance.go`, `claude_controller.go`, and the adapter).
Epic 5 and 6 require `make generate-proto` to have run first.

---

## Flagged Choices and Risks

### 1. Two-capture-group timestamp regex handling (Story 1.2.4)
**Risk**: `extractTimestamp()` currently iterates patterns and uses `matches[1]` uniformly. The new
two-group regex requires a branch based on `len(matches)`. Must be handled carefully to avoid
index-out-of-range panics on single-group patterns.
**Decision**: Check `len(matches) == 3` (full match + 2 groups) to route to `parseTimeWithTZ`.

### 2. Callback vs. direct EventBus injection into Manager (Story 3.1)
**Risk**: Injecting `*events.EventBus` directly into the `ratelimit` package creates an import cycle
(`server/events` → `session` → `server/events` is possible depending on how packages are structured).
**Decision**: Use callbacks on `Integration`/`Manager` (option C from research). The ratelimit package
stays dependency-free; the server layer wires closures that capture the event bus. This is the
lower-risk approach.

### 3. `SetRateLimitEnabled` as a new RPC vs. reusing `UpdateSession` (Story 5.2.1)
**Risk**: Adding a dedicated RPC increases proto surface; reusing `UpdateSession` with a field mask
would be more consistent with existing patterns.
**Decision**: Add dedicated `SetRateLimitEnabled` RPC for clarity and testability. The field mask
approach can be considered in a follow-up refactor.

### 4. `import _ "time/tzdata"` binary size impact (Story 1.2.1)
**Risk**: Adds ~500 KB to the binary. On Alpine/slim containers this is the only way `LoadLocation`
works.
**Decision**: Accept the size increase. Document in a comment near the import.

### 5. Re-detection cooldown after recovery (Story 4.1)
**Risk**: If Claude re-shows the rate limit message immediately after recovery input is sent (e.g.,
the session was still rate-limited), and the cooldown is 30s, the re-detection will be delayed 30s.
This is acceptable per requirements ("cooldown to prevent tight loops").
**Decision**: Keep the existing 30s cooldown. No change needed to the cooldown value.

### 6. Notification spam — rate limiter scope
**Risk**: The existing `NotificationRateLimiter` gates the `SendNotification` RPC (HTTP path). If we
publish `EventNotification` directly to the event bus, we bypass that gate.
**Decision**: In the detection/recovery callbacks (Story 3.3.2/3.3.3), implement a simple per-session
dedup: check if the last notification for this session was sent less than N seconds ago before
publishing. Alternatively, expose a helper on `NotificationRateLimiter` callable from the session
layer.

### 7. Proto field numbers (Story 2.3)
**Risk**: Proto fields 41 and 42 are assumed free. Must verify against the full `Session` message
in `proto/session/v1/types.proto` before assigning.
**Decision**: Verify field numbers at implementation time; adjust if occupied.

---

## Summary

| Metric | Count |
|---|---|
| Epics | 6 |
| Stories | 18 |
| Tasks | 52 |

**Files changed (Go)**: `detector.go`, `manager.go`, `scheduler.go`, `integration.go`,
`claude_controller.go`, `instance.go`, `instance_adapter.go`, `session_service.go`, `server.go`,
`events/types.go` (optional)

**Files changed (Proto)**: `proto/session/v1/types.proto`, `proto/session/v1/session.proto`

**Files changed (Frontend)**: `SessionCard.tsx`, `useSessionService.ts`,
`SessionServiceContext.tsx`

**New files**: `testdata/claude_rate_limit_new_format.txt`, `instance_adapter_test.go` (if not
existing)
