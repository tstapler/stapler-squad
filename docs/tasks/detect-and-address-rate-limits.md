# Feature Plan: Detect and Address Rate Limits

**Implementation Status**: Core stories 1-4 implemented (`session/detection/ratelimit/`). Story 5 (config/per-session disable) and UX improvements (`rate-limit-ux-improvements.md`) remain pending.
**Updated**: 2026-05-02

## Executive Summary

This feature enables automatic detection and resolution of LLM API rate limits in terminal sessions. When rate limit dialogs appear in sessions running Claude Code, Aider, or other LLM programs, the system will automatically detect them, wait for the reset time to elapse, and send the appropriate input to resume without manual intervention.

## Problem Statement

When LLM programs (Claude Code, Aider, etc.) hit API rate limits, they display a dialog requiring user interaction to continue once the rate limit reset time has elapsed. This requires manual intervention to press "continue" or "keep trying" - interrupting workflows and requiring users to monitor sessions.

## Requirements Analysis

### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-1 | System SHALL detect rate limit dialogs from terminal output for common LLM providers (Anthropic, OpenAI, Google, etc.) | Must Have |
| FR-2 | System SHALL parse rate limit reset timestamp from dialog messages | Must Have |
| FR-3 | System SHALL automatically send input to tmux session to continue/keep trying when reset time elapses | Must Have |
| FR-4 | System SHALL support multiple LLM programs (Claude Code, Aider, etc.) | Must Have |

### Non-Functional Requirements

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-1 | Rate limit dialog detection latency | < 500ms from dialog appearance |
| NFR-2 | False positive rate | < 5% |
| NFR-3 | Recovery success rate | > 95% |
| NFR-4 | Memory overhead per monitored session | < 10MB |

## Architecture

### Design Pattern: Observer/Pipeline

The rate limit detection system follows a pipeline architecture:

```
[Terminal Output] → [ANSI Stripper] → [Pattern Matcher] → [Action Handler]
```

**Components:**

| Component | Responsibility | Location |
|-----------|---------------|----------|
| `RateLimitDetector` | Consumes terminal output, matches patterns, parses reset times | `session/detection/ratelimit/detector.go` |
| `EventBus` | Distributes rate limit events to handlers | `session/detection/ratelimit/eventbus.go` |
| `TimerScheduler` | Manages timers for reset time monitoring | `session/detection/ratelimit/scheduler.go` |
| `RecoveryHandler` | Executes recovery actions (sends input to tmux) | `session/detection/ratelimit/recovery.go` |

### Integration Points

- **Terminal Output**: Add detector as `OutputConsumer` to existing `ExternalStreamer` in `session/external_streamer.go`
- **Input Sending**: Use existing `WriteToPTY()` method in `session/instance.go`
- **Session State**: Track rate limit state per session in `Instance` struct

### State Machine

```go
type RateLimitState int

const (
    StateNone RateLimitState = iota       // No rate limit detected
    StateWaiting                           // Rate limit detected, waiting for reset
    StateReadyToRecover                   // Reset time elapsed, ready to send input
    StateRecovering                       // Sending recovery input
    StateRecovered                        // Successfully recovered
    StateFailed                           // Recovery failed
)
```

## Implementation Plan

### Story 1: Rate Limit Detector Core

**Description**: Implement the core detection logic that identifies rate limit dialogs in terminal output.

**Acceptance Criteria**:
- Given terminal output containing rate limit dialog, when detection runs, then dialog is identified within 500ms
- Given output without rate limit, when detection runs, then no false positive is triggered
- Given multiple provider formats, when detection runs, then appropriate provider is identified

**Tasks**:
1. Create `session/detection/ratelimit/detector.go` with pattern matching
2. Define regex patterns for Anthropic, OpenAI, Google, Aider formats
3. Implement ANSI stripping before matching
4. Add provider identification from output content
5. Create unit tests for pattern matching

**Pattern Set**:
```go
var rateLimitPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)/rate-limit-options`),
    regexp.MustCompile(`(?i)rate limit.*exceeded`),
    regexp.MustCompile(`(?i)429.*Too Many Requests`),
    regexp.MustCompile(`(?i)rate_limit_error`),
}

var continuePromptPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)press.*enter.*continue`),
    regexp.MustCompile(`(?i)continue.*\?.*\[y/n\]`),
}

var timestampPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)reset at (\d{1,2}:\d{2}(?::\d{2})?)`),
    regexp.MustCompile(`(?i)retry after (\d+)\s*(second|minute|hour)`),
    regexp.MustCompile(`(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})`),
}
```

### Story 2: Reset Time Parsing and Scheduling

**Description**: Parse the reset timestamp from detected dialogs and schedule automatic recovery.

**Acceptance Criteria**:
- Given rate limit dialog with timestamp, when detected, then reset time is parsed correctly
- Given parsed reset time, when timer fires, then recovery action is triggered
- Given reset time in the past, when detected, then recovery triggers immediately

**Tasks**:
1. Implement timestamp parsing for multiple formats
2. Create `TimerScheduler` for managing reset time timers
3. Add buffer time (5 seconds) before triggering recovery
4. Implement state persistence for timers across restarts
5. Create tests for time parsing and scheduling

### Story 3: Recovery Action Execution

**Description**: Execute the appropriate recovery action when reset time elapses.

**Acceptance Criteria**:
- Given timer fires for Claude session, when recovery triggers, then "continue" is sent
- Given timer fires for Aider session, when recovery triggers, then Enter is sent
- Given session is paused, when timer fires, then recovery is deferred

**Tasks**:
1. Map provider types to recovery inputs
2. Implement `sendInputToTmux` integration (reuse existing)
3. Add state validation before sending input
4. Handle session pause/resume during wait period
5. Add retry logic for failed input sends

**Recovery Input Mapping**:
```go
var rateLimitInputs = map[string][]byte{
    "anthropic": []byte("c\n"),       // "continue" for Claude Code
    "openai":    []byte("r\n"),       // "retry" option
    "google":    []byte("y\n"),       // "yes" to retry
    "aider":     []byte("\n"),        // generic enter
}
```

### Story 4: Event System and Logging

**Description**: Implement event distribution and comprehensive logging for debugging.

**Acceptance Criteria**:
- Given rate limit event occurs, when system processes it, then appropriate log entries are created
- Given recovery succeeds/fails, when event occurs, then notification is emitted
- Given user wants debug information, when logging enabled, then full trace is available

**Tasks**:
1. Define `RateLimitEvent` struct and event types
2. Implement `EventBus` with subscriber management
3. Add structured logging with appropriate log levels
4. Create integration with existing logging infrastructure
5. Add metrics for detection and recovery rates

### Story 5: Configuration and Extensibility

**Description**: Add configuration options for fine-tuning behavior and enabling/disabling per session.

**Acceptance Criteria**:
- Given global config sets auto-recovery enabled, when rate limit detected, then auto-recovery runs
- Given session has auto-recovery disabled, when rate limit detected, then detection logs but takes no action
- Given provider not in default list, when rate limit detected, then configurable patterns apply

**Tasks**:
1. Add `RateLimitAutoRecovery` to config struct
2. Implement per-session override option
3. Add YAML pattern definition support for extensibility
4. Create configuration documentation
5. Add validation for configuration values

**Config Structure**:
```go
type RateLimitConfig struct {
    Enabled           bool        `json:"enabled"`
    Providers         []string    `json:"providers"`
    RetryCount        int         `json:"retry_count"`
    ResetBufferSeconds int        `json:"reset_buffer_seconds"`
    PerSessionOverride *bool     `json:"per_session_override"`
}
```

### Story 6: Integration Testing

**Description**: Test the full detection-to-recovery flow with mocked tmux sessions.

**Acceptance Criteria**:
- Given complete rate limit flow, when tested, then recovery succeeds
- Given detection without actual rate limit, when tested, then no false positive
- Given recovery during session pause, when tested, then deferred properly

**Tasks**:
1. Create integration tests with mocked sessions
2. Test various provider dialog formats
3. Test timing edge cases
4. Add to CI pipeline
5. Document test scenarios

## Known Issues

### Potential Bugs Identified During Planning

#### Bug 1: Race Condition in Timer Management [SEVERITY: High]

**Description**: Concurrent rate limit detections for the same session may create duplicate timers, causing multiple recovery attempts.

**Mitigation**:
- Use session-level mutex for timer management
- Track pending timer per session
- Cancel existing timer before scheduling new one
- Add integration tests with concurrent detection

**Files Likely Affected**:
- `session/detection/ratelimit/scheduler.go`
- `session/detection/ratelimit/detector.go`

**Prevention Strategy**:
- Design timer management with thread-safety from start
- Use sync.Map for timer storage
- Add state machine transitions to prevent duplicates

#### Bug 2: False Positives from Generic Output [SEVERITY: Medium]

**Description**: Output containing words like "rate", "limit", "try again" in normal context may trigger detection erroneously.

**Mitigation**:
- Require multiple pattern matches before triggering
- Implement cooldown between detections (30s)
- Add confidence threshold requiring provider context
- Test with various normal output patterns

**Files Likely Affected**:
- `session/detection/ratelimit/detector.go`

**Prevention Strategy**:
- Require both rate limit pattern AND continue prompt
- Add provider-specific validation
- Implement detection confidence scoring

#### Bug 3: Missing Rate Limit Dialogs [SEVERITY: Medium]

**Description**: Dialog appears and is dismissed before detection runs on the output chunk.

**Mitigation**:
- Use existing polling mechanism (100ms interval)
- Maintain sliding window of recent terminal output
- Re-scan last N bytes on each chunk arrival
- Use capture-pane for full content verification

**Files Likely Affected**:
- `session/detection/ratelimit/detector.go`
- `session/external_streamer.go`

**Prevention Strategy**:
- Process output in real-time via OutputConsumer
- Maintain buffer of recent output for pattern matching
- Verify dialog presence before scheduling recovery

#### Bug 4: Timing Issues with Reset Time [SEVERITY: High]

**Description**: Parsed timestamp format variations may cause incorrect wait times, sending input too early or too late.

**Mitigation**:
- Support multiple timestamp format patterns
- Add parsing confidence validation
- Use max(parsed time, current time + buffer)
- Add 5-10 second safety buffer

**Files Likely Affected**:
- `session/detection/ratelimit/detector.go`

**Prevention Strategy**:
- Test with various timestamp formats
- Add buffer time before recovery
- Verify dialog still present before sending input

#### Bug 5: User Input Conflict [SEVERITY: Medium]

**Description**: User manually responds to rate limit before automated system triggers, causing duplicate input.

**Mitigation**:
- Implement detection cooldown (don't auto-resolve immediately)
- Check for recent user input before sending
- Allow user to disable auto-resolution per session
- Add logging when manual intervention detected

**Files Likely Affected**:
- `session/detection/ratelimit/recovery.go`

**Prevention Strategy**:
- Add delay before recovery (e.g., 2 seconds after reset)
- Check session state immediately before sending
- Log for debugging user conflicts

#### Bug 6: Session State Changes During Wait [SEVERITY: Medium]

**Description**: Session is paused, detached, or terminates during the wait period before auto-resolution.

**Mitigation**:
- Verify session is running before sending input
- Handle session pause/detach gracefully
- Resume monitoring after session reattach
- Cancel timers on session pause

**Files Likely Affected**:
- `session/detection/ratelimit/scheduler.go`
- `session/instance.go`

**Prevention Strategy**:
- Subscribe to session state changes
- Cancel timers when session pauses
- Re-evaluate pending recovery on session resume
- Log failures for debugging

#### Bug 7: Quota Exhaustion vs Rate Limiting [SEVERITY: Low]

**Description**: "Quota exceeded" (billing issue) is different from rate limiting (temporary) but may match similar patterns.

**Mitigation**:
- Separate pattern sets for "quota" vs "rate limit"
- Don't attempt auto-resolution for quota exhaustion
- Log and alert for quota exhaustion errors
- Distinguish via presence of reset timer display

**Files Likely Affected**:
- `session/detection/ratelimit/detector.go`

**Prevention Strategy**:
- Add quota-specific patterns
- Check for "quota" keyword vs "rate limit"
- Require timestamp/reset info for rate limit detection

## Testing Strategy

### Unit Tests

- Pattern matching for each provider format
- Timestamp parsing accuracy
- State machine transitions
- Timer scheduling edge cases
- Configuration validation

### Integration Tests

- End-to-end detection and recovery flow
- Concurrent session monitoring
- Session pause/resume handling
- WebSocket event broadcasting

### Performance Tests

- Detection latency under load
- Memory usage with many sessions
- Timer precision accuracy

## Dependencies

### Internal Dependencies

- `session/external_streamer.go` - Terminal output access
- `session/instance.go` - Session state and input sending
- Existing logging infrastructure

### External Dependencies

- None (uses Go standard library regexp)

## Rollout Plan

### Phase 1: Core Detection (Story 1-2)

- Implement detector with pattern matching
- Add timestamp parsing and scheduling
- Create unit tests

### Phase 2: Recovery Actions (Story 3-4)

- Implement recovery handler
- Add event system and logging
- Integration testing

### Phase 3: Configuration (Story 5-6)

- Add configuration options
- Integration testing
- Documentation

## Success Metrics

- Rate limit dialogs detected within 500ms of appearance
- Automatic recovery成功率 > 95%
- False positive rate < 5%
- No manual intervention required for rate limit recovery
- Memory overhead < 10MB per monitored session
