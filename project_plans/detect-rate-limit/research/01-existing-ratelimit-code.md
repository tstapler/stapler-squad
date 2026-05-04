# Research: Existing Rate Limit Infrastructure

## Package: `session/detection/ratelimit/`

### Files
- `detector.go` (8.2K) — core pattern matching and timestamp parsing
- `manager.go` (5.7K) — orchestrates detector + scheduler + recovery + internal event bus
- `scheduler.go` (2.4K) — time.AfterFunc-based recovery scheduling
- `recovery.go` (761B) — thin wrapper that calls `sendInput([]byte)`
- `integration.go` (2.7K) — `Integration` and `PTYConsumer` (polling loop wrapper)
- `detector_test.go` (14.8K) — comprehensive unit tests for all components

---

## `Detector` — Pattern Matching

### State Machine (in `detector.go`)
```
StateNone → StateWaiting → StateRecovering → StateRecovered | StateFailed
```

Go enum: `StateNone=0, StateWaiting=1, StateRecovering=2, StateRecovered=3, StateFailed=4`

### Existing `defaultRateLimitPatterns` (trigger detection)
```
(?i)/rate-limit-options
(?i)rate limit.*exceeded
(?i)429.*Too Many Requests
(?i)rate_limit_error
(?i)Usage limit reached
(?i)rate limit reached
(?i)quota exceeded
```
**Missing**: `You've hit your limit` — the exact Claude Code usage-limit message is NOT matched.

### Existing `defaultContinuePatterns` (required for detection to fire)
```
(?i)1\.\s*Keep trying
(?i)press.*enter.*continue
(?i)continue.*\?.*\[y/n\]
(?i)\*?\s*\d+\.\s*(Keep|Try|Continue|Retry)
(?i)Access resets at
```
These patterns ARE present for Claude Code output, so detection fires once a rate-limit pattern also matches.

### `defaultTimestampPatterns`
```
(?i)(?:reset at|Access resets at) (.+?)(?:\s*$|PT|PDT)
(?i)retry\s*after\s*(\d+)\s*(second|minute|hour)s?
(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})
```
**Missing**: `resets Xpm (America/Los_Angeles)` format from "You've hit your limit - resets 11pm (America/Los_Angeles)". The first pattern would not match because the text says "resets" not "resets at" or "Access resets at".

### `parseTimestamp()` — time format support
Handles:
- Pure number → seconds from now
- `N second/minute/hour` → relative duration
- `retry after N unit` → relative duration
- `"3:04 PM"`, `"3:04:05 PM"`, `"15:04"`, `"15:04:05"`, `"2006-01-02T15:04:05"` → wall clock

**NOT handled**: timezone-qualified formats like `"11pm America/Los_Angeles"`, `"11pm (America/Los_Angeles)"`, `"11:30pm Pacific"`.
All `time.Parse` calls use the zero location; timezone info in captured string is silently dropped.

---

## `Manager` — Orchestration (`manager.go`)

- `NewManager(sessionID, SessionAccessor)` — creates detector, scheduler, recovery, internal EventBus
- `ProcessOutput(data []byte)` — routes to detector
- `handleDetection(det Detection)` — publishes internal `eventDetected`, calls `scheduler.ScheduleRecovery(det.ResetTime)`
- `executeRecovery()` — publishes `eventRecoveryStart/Done/Fail`, calls `recovery.Execute(input)`
- Internal `EventBus` is separate from the server-level `events.EventBus` — **no server events are published**

### Per-session enable/disable
`SetEnabled(bool)` / `IsEnabled() bool` on both `Manager` and `PTYConsumer`.
`SetCooldown(duration)` / `SetResetBuffer(seconds)` — configurable.

### Missing: no notification hook
`Manager.handleDetection()` publishes to the internal `EventBus` only. There is no connection to the server-level `events.EventBus` (and thus no WebSocket events, no desktop notifications).

---

## `Scheduler` (`scheduler.go`)

- Uses `time.AfterFunc` with `resetTime + bufferSeconds`
- Falls back to `bufferSeconds` wait if `resetTime.IsZero()` — but `DefaultResetBuffer = 5`, so a 5-second fallback instead of 30-minute fallback required by US-2
- `CancelRecovery()` / `IsScheduled()` / `GetScheduledTime()` — all present
- Re-detection after recovery: **not implemented** — after recovery executes, detector state is set to `StateRecovered` and ProcessOutput blocks on `currentState != StateNone && currentState != StateWaiting`. State never resets to `StateNone` after recovery, so re-detection loop cannot trigger.

---

## `Integration` / `PTYConsumer` (`integration.go`)

```go
type PTYConsumer struct {
    buffer       BufferReader
    manager      *Manager
    pollInterval time.Duration  // 500ms default
    running      bool
    stopCh       chan struct{}
}
```

Polls `buffer.GetRecentOutput(4096)` every 500ms and feeds to `manager.ProcessOutput()`.
`GetRateLimitState()`, `SetEnabled()`, `IsEnabled()` are available on `PTYConsumer`.

---

## `ClaudeController` (`session/claude_controller.go`)

```go
rateLimitHandler *ratelimit.PTYConsumer  // created at line ~117-118
```

- `rateLimitHandler` is started in `Start()` (line ~233), stopped in `Stop()` (line ~268)
- `GetRateLimitState() ratelimit.RateLimitState` — returns state from PTYConsumer
- `SetRateLimitEnabled(bool)` / `IsRateLimitEnabled() bool` — delegate to PTYConsumer

---

## `session.Instance` (`session/instance.go`)

- `GetRateLimitState() int` at line 2718-2724 — calls `ctrl.GetRateLimitState()`
- `SetRateLimitEnabled(bool)` at line 2727-2731
- `IsRateLimitEnabled() bool` at line 2735-2741

---

## The Adapter Gap (`server/adapters/instance_adapter.go`)

`InstanceToProto()` at line 10-91 builds the proto `Session` message from a Go `session.Instance`.

**`RateLimitState` is NOT populated.** The proto field `rate_limit_state = 40` exists in the proto definition and the frontend imports `RateLimitState` from the generated types, but `InstanceToProto()` never calls `inst.GetRateLimitState()` and never sets `protoSession.RateLimitState`.

Fix required:
```go
protoSession.RateLimitState = rateLimitStateToProto(ratelimit.RateLimitState(inst.GetRateLimitState()))
```
Plus a new helper function mapping `ratelimit.StateNone → RATE_LIMIT_STATE_NONE`, etc.

---

## Tests

`detector_test.go` covers:
- `TestDetector_ProcessOutput_AnthropicRateLimit` — uses "Usage limit reached for claude-3-opus" + "Access resets at 2:53 PM PDT" + "1. Keep trying"
- `TestDetector_ProcessOutput_GeminiRateLimit` — same format, boxed with `│` characters
- `TestDetector_Cooldown`, `TestDetector_StateTransitions`, `TestDetector_ConcurrentProcessOutput`
- `TestParseTimestamp_RetryAfter` — "retry after 60 second"
- `TestParseTimestamp_SpecificTime` — "Access resets at 3:00 PM"
- `TestScheduler_ScheduleRecovery` / `TestScheduler_CancelRecovery`
- `TestRecoveryHandler_Execute`

**Missing tests**: 
- `"You've hit your limit - resets 11pm (America/Los_Angeles)"` format
- Timezone-aware parsing: `"11pm America/Los_Angeles"`, `"11:30pm Pacific"`
- Re-detection after recovery attempt
- Per-session enable/disable toggle

---

## Summary of Gaps vs Requirements

| Requirement | Status |
|---|---|
| Pattern: "You've hit your limit - resets Xpm (Timezone)" | MISSING from `defaultRateLimitPatterns` |
| Timestamp parsing: `resets 11pm (America/Los_Angeles)` | MISSING — no timezone-aware parse |
| Adapter: `RateLimitState` in proto | MISSING — `InstanceToProto()` never sets it |
| Notifications on detect/recover | MISSING — no server event published from Manager |
| Re-detection after failed recovery | MISSING — state stuck at `StateRecovered`/`StateFailed` |
| Per-session toggle | Exists in Go layer; not exposed to UI or proto |
| Fallback to 30-min on unparseable time | PARTIALLY: falls back to 5s (DefaultResetBuffer) not 30min |
