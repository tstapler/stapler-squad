# Testing Plan: Detect and Address Rate Limits

## Overview

This testing plan provides comprehensive coverage for the rate limit detection and auto-recovery feature. Following the test pyramid principles, we prioritize unit tests at the base, integration tests in the middle, and end-to-end tests at the top.

## Test Pyramid Structure

```
        /\
       /  \      E2E Tests (5%)
      /----\
     /      \    Integration Tests (15%)
    /--------\
   /          \  Unit Tests (80%)
  /____________\
```

## Critical Paths

| Priority | Path | Known Issue Risk |
|----------|------|------------------|
| 1 | Terminal output → Pattern match → Parse timestamp → Schedule timer → Send recovery input | Bug 4: Timing issues |
| 2 | Concurrent detection → Timer management → Recovery | Bug 1: Race condition |
| 3 | Rate limit appears → User responds → System tries to respond | Bug 5: User input conflict |
| 4 | Session pauses during wait → Timer fires → Session state | Bug 6: Session state changes |

---

# Section 1: Unit Tests (80%)

## 1.1 Pattern Matching Tests

### Test Group: RateLimitPatternDetector

**Purpose**: Verify accurate detection of rate limit dialogs in terminal output.

#### TC-UT-001: Anthropic Rate Limit Detection
```
Input:  "You exceeded the rate limit. The API will reset at 3:45 PM."
Expect: Match = true, Provider = "anthropic", HasResetTime = true
```

#### TC-UT-002: OpenAI Rate Limit Detection
```
Input:  "Error 429: Too Many Requests - Rate limit exceeded. Retry after 60 seconds."
Expect: Match = true, Provider = "openai", RetrySeconds = 60
```

#### TC-UT-003: Google Rate Limit Detection
```
Input:  "rate_limit_error: quota exceeded. Reset at 2024-01-15T14:30:00Z"
Expect: Match = true, Provider = "google"
```

#### TC-UT-004: Aider Rate Limit Detection
```
Input:  "Rate limit hit, waiting 30 seconds..."
Expect: Match = true, Provider = "aider"
```

#### TC-UT-005: No Match for Normal Output
```
Input:  "The rate of change is acceptable. Let me limit the scope."
Expect: Match = false
Coverage: Bug 2 mitigation
```

#### TC-UT-006: ANSI Code Stripping
```
Input:  "\x1b[32m\x1b[1mRate limit\x1b[0m exceeded, please wait..."
Expect: Match = true (ANSI codes stripped before matching)
```

#### TC-UT-007: Multi-Line Rate Limit Dialog
```
Input:  "You hit a rate limit
        The API will reset at 4:00 PM
        Press Enter to continue"
Expect: Match = true, Provider = "anthropic"
```

### Test Group: ContinuePromptPatternDetector

**Purpose**: Verify detection of interactive prompts that follow rate limits.

#### TC-UT-008: Continue Prompt Detection
```
Input:  "Press Enter to continue..."
Expect: Match = true
```

#### TC-UT-009: Yes/No Prompt Detection
```
Input:  "Continue? [y/n]"
Expect: Match = true
```

#### TC-UT-010: Aider Enter Prompt
```
Input:  "Rate limited...hit enter to retry"
Expect: Match = true
```

---

## 1.2 Timestamp Parsing Tests

### Test Group: ResetTimeParser

**Purpose**: Verify accurate parsing of various timestamp formats.

#### TC-UT-011: 12-Hour Format with AM/PM
```
Input:  "reset at 3:45 PM"
Expect: ParsedTime within 1 second of actual time for "3:45 PM" today
```

#### TC-UT-012: 24-Hour Format
```
Input:  "reset at 15:45"
Expect: ParsedTime hour = 15, minute = 45
```

#### TC-UT-013: Time with Seconds
```
Input:  "reset at 3:45:30 PM"
Expect: ParsedTime with second = 30
```

#### TC-UT-014: ISO 8601 Timestamp
```
Input:  "retry after 2024-01-15T14:30:00Z"
Expect: ParsedTime matches ISO format
```

#### TC-UT-015: Relative Seconds
```
Input:  "retry after 60 seconds"
Expect: WaitDuration = 60 seconds
```

#### TC-UT-016: Relative Minutes
```
Input:  "retry after 5 minutes"
Expect: WaitDuration = 5 minutes = 300 seconds
```

#### TC-UT-017: Relative Hours
```
Input:  "retry after 2 hours"
Expect: WaitDuration = 2 hours = 7200 seconds
```

#### TC-UT-018: Past Reset Time
```
Input:  "reset at 1:00 PM" (current time 3:00 PM)
Expect: Immediate recovery (wait duration = 0 or very short)
Coverage: Bug 4 mitigation
```

#### TC-UT-019: Invalid Timestamp Format
```
Input:  "reset sometime soon"
Expect: ParseError, HasResetTime = false
```

#### TC-UT-020: Empty Timestamp
```
Input:  "rate limit hit"
Expect: No timestamp parsed, use default wait time
```

---

## 1.3 State Machine Tests

### Test Group: RateLimitStateTransitions

**Purpose**: Verify correct state transitions in the rate limit state machine.

#### TC-UT-021: None → Waiting (Detection)
```
Given:   CurrentState = StateNone
When:    RateLimitDetected event received
Then:    NewState = StateWaiting, TimerScheduled = true
```

#### TC-UT-022: Waiting → ReadyToRecover (Timer Fire)
```
Given:   CurrentState = StateWaiting
When:    Timer fires (reset time elapsed)
Then:    NewState = StateReadyToRecover
```

#### TC-UT-023: ReadyToRecover → Recovering (Trigger)
```
Given:   CurrentState = StateReadyToRecover
When:    RecoveryTriggered
Then:    NewState = StateRecovering, InputSent = true
```

#### TC-UT-024: Recovering → Recovered (Success)
```
Given:   CurrentState = StateRecovering
When:    InputSend succeeds
Then:    NewState = StateRecovered
```

#### TC-UT-025: Recovering → Failed (Input Send Error)
```
Given:   CurrentState = StateRecovering
When:    InputSend fails
Then:    NewState = StateFailed, RetryScheduled = true (if retries left)
```

#### TC-UT-026: Waiting → None (Cancel)
```
Given:   CurrentState = StateWaiting
When:    Cancel event (session pause/stop)
Then:    NewState = StateNone, TimerCancelled = true
```

#### TC-UT-027: Invalid Transition Rejection
```
Given:   CurrentState = StateNone
When:    RecoveryTriggered (no detection yet)
Then:    Error/TransitionRejected
```

---

## 1.4 Timer Scheduler Tests

### Test Group: TimerSchedulerBehavior

**Purpose**: Verify timer management logic handles all scenarios correctly.

#### TC-UT-028: Single Timer Scheduling
```
Given:   No existing timer for session
When:    ScheduleTimer(resetTime = now + 30s)
Then:    Timer created, fires after ~30s
```

#### TC-UT-029: Timer Replacement
```
Given:   Existing timer scheduled for T1
When:    ScheduleTimer(resetTime = T2) where T2 > T1
Then:    T1 cancelled, T2 scheduled
Coverage: Bug 1 mitigation
```

#### TC-UT-030: Concurrent Timer Requests
```
Given:   No existing timer
When:    ScheduleTimer called from 2 goroutines simultaneously
Then:    Only one timer created, no duplicate timers
Coverage: Bug 1 - race condition prevention
```

#### TC-UT-031: Timer Cancellation
```
Given:   Timer scheduled
When:    CancelTimer called
Then:    Timer cancelled, no callback executed
```

#### TC-UT-032: Timer Fire After Cancellation
```
Given:   Timer scheduled, then cancelled
When:    Original fire time passes
Then:    No callback executed
```

#### TC-UT-033: Multiple Sessions Isolated
```
Given:   Timers scheduled for session A and session B
When:    Each timer fires
Then:    Correct session identified for each callback
```

#### TC-UT-034: Very Long Wait Time
```
Input:   Reset time = now + 24 hours
Expect:  Timer scheduled correctly, no overflow
```

---

## 1.5 Recovery Input Tests

### Test Group: RecoveryInputMapping

**Purpose**: Verify correct input mapping per provider.

#### TC-UT-035: Anthropic Recovery Input
```
Given:   Provider = "anthropic"
When:    GetRecoveryInput()
Then:    Returns "c\n" (continue)
```

#### TC-UT-036: OpenAI Recovery Input
```
Given:   Provider = "openai"
When:    GetRecoveryInput()
Then:    Returns "r\n" (retry)
```

#### TC-UT-037: Google Recovery Input
```
Given:   Provider = "google"
When:    GetRecoveryInput()
Then:    Returns "y\n" (yes)
```

#### TC-UT-038: Aider Recovery Input
```
Given:   Provider = "aider"
When:    GetRecoveryInput()
Then:    Returns "\n" (enter)
```

#### TC-UT-039: Unknown Provider Default
```
Given:   Provider = "unknown_provider"
When:    GetRecoveryInput()
Then:    Returns "\n" (generic enter)
```

#### TC-UT-040: Empty Provider
```
Given:   Provider = ""
When:    GetRecoveryInput()
Then:    Returns "\n" (generic enter)
```

---

## 1.6 Configuration Tests

### Test Group: ConfigValidation

**Purpose**: Verify configuration parsing and validation.

#### TC-UT-041: Default Configuration
```
Input:   nil or empty config
Expect:  Enabled = true, RetryCount = 3, BufferSeconds = 5
```

#### TC-UT-042: Custom Configuration
```
Input:   RateLimitConfig{Enabled: false, RetryCount: 5}
Expect:  Config.Enabled = false, Config.RetryCount = 5
```

#### TC-UT-043: Invalid Retry Count
```
Input:   RetryCount = -1
Expect:  ValidationError
```

#### TC-UT-044: Invalid Buffer Seconds
```
Input:   BufferSeconds = 0
Expect:  ValidationError (must be > 0)
```

#### TC-UT-045: Per-Session Override
```
Input:   PerSessionOverride = true
Expect:  Override respected in session context
```

---

## 1.7 False Positive Prevention Tests

### Test Group: FalsePositiveMitigation

**Purpose**: Ensure Bug 2 (false positives) is properly mitigated.

#### TC-UT-046: Generic "Rate" Word
```
Input:  "The rate of change is linear."
Expect: No match (requires both patterns)
```

#### TC-UT-047: Generic "Limit" Word
```
Input:  "There is no limit to what we can achieve."
Expect: No match
```

#### TC-UT-048: Generic "Try Again" 
```
Input:  "Please try again later with different parameters."
Expect: No match (requires rate limit pattern)
```

#### TC-UT-049: Combined Patterns Required
```
Input:  "Rate limit exceeded" (no continue prompt)
Expect: Partial match logged but no full detection
```

#### TC-UT-050: Cooldown Enforcement
```
Given:   RateLimitDetected at T=0
When:    Same pattern detected at T=10s (within cooldown)
Then:    Detection suppressed (cooldown = 30s)
Coverage: Bug 2 mitigation
```

#### TC-UT-051: Confidence Threshold
```
Input:  Low confidence: "rate limit?" (question mark only)
Expect: Detection requires confidence > threshold
```

---

## 1.8 Quota vs Rate Limit Tests

### Test Group: QuotaDifferentiation

**Purpose**: Ensure Bug 7 (quota vs rate limit) is properly handled.

#### TC-UT-052: Quota Exceeded Pattern
```
Input:  "Quota exceeded. Please upgrade your plan."
Expect: Match = false for rate limit, separate handling
Coverage: Bug 7 mitigation
```

#### TC-UT-053: Rate Limit with Reset Time
```
Input:  "Rate limit exceeded, resets in 30 seconds"
Expect: Match = true (has reset time = rate limit)
```

#### TC-UT-054: Billing Issue Differentiation
```
Input:  "Billing limit reached. Contact support."
Expect: Match = false for rate limit patterns
```

---

# Section 2: Integration Tests (15%)

## 2.1 Component Interaction Tests

### Test Group: DetectorToSchedulerIntegration

**Purpose**: Verify smooth handoff between detection and timer scheduling.

#### TC-INT-001: Detection Schedules Timer
```
Given:   MockExternalStreamer with rate limit output
When:    Detector receives output chunk
Then:    Timer scheduled with correct reset time
Verify:  Scheduler.GetTimer(sessionID) returns timer
```

#### TC-INT-002: Timer Fires Triggers Recovery
```
Given:   Timer scheduled for session
When:    Timer fires (simulated)
Then:    RecoveryHandler.SendInput called
Verify:  Input sent to correct session
```

#### TC-INT-003: Detector Registers with EventBus
```
Given:   New detector created
When:    Rate limit detected
Then:    Event published to EventBus
Verify:  Subscribers receive event
```

### Test Group: SessionStateIntegration

**Purpose**: Verify integration with session lifecycle.

#### TC-INT-004: Session Pause Cancels Timer
```
Given:   Timer scheduled
When:    Session pauses
Then:    Timer cancelled
Verify:  No recovery input sent while paused
Coverage: Bug 6 mitigation
```

#### TC-INT-005: Session Resume Re-evaluates
```
Given:   Session was rate limited, paused, now resumed
When:    Session resumes
Then:    Check if still rate limited, reschedule if needed
```

#### TC-INT-006: Session Stop Cancels Timer
```
Given:   Timer scheduled
When:    Session terminates
Then:    Timer cancelled, no recovery attempted
```

#### TC-INT-007: Session Detach During Wait
```
Given:   Timer scheduled, session detached
When:    Timer fires
Then:    Recovery deferred until reattached
```

### Test Group: ExternalStreamerIntegration

**Purpose**: Verify detector integration with terminal output pipeline.

#### TC-INT-008: Output Consumer Receives Data
```
Given:   Detector registered as OutputConsumer
When:    Terminal outputs rate limit dialog
Then:    Detector receives output via Consume method
```

#### TC-INT-009: Multiple Output Chunks
```
Given:   Rate limit dialog spans multiple chunks
When:    Each chunk processed
Then:    Full dialog detected (sliding window)
Coverage: Bug 3 mitigation
```

#### TC-INT-010: Rapid Output Processing
```
Given:   High-frequency output stream
When:    Detector processes each chunk
Then:    No missed detections, no blocking
```

### Test Group: RecoveryHandlerIntegration

**Purpose**: Verify recovery input integration with tmux.

#### TC-INT-011: WriteToPTYCalled
```
Given:   Session in StateReadyToRecover
When:    Recovery triggered
Then:    Instance.WriteToPTY called with correct input
```

#### TC-INT-012: Input Send Failure Retry
```
Given:   First input send fails
When:    Retry triggered
Then:    Input sent again (up to RetryCount)
```

#### TC-INT-013: Successful Recovery State Transition
```
Given:   Recovery succeeds
When:    After input send
Then:    State = StateRecovered, Event emitted
```

### Test Group: EventBusIntegration

**Purpose**: Verify event distribution works correctly.

#### TC-INT-014: Event Subscription
```
Given:   Handler subscribes to RateLimitDetected
When:    Event published
Then:    Handler receives event
```

#### TC-INT-015: Multiple Subscribers
```
Given:   Multiple handlers subscribed
When:    Event published
Then:    All handlers receive event
```

#### TC-INT-016: Event Unsubscribe
```
Given:   Handler subscribed, then unsubscribed
When:    Event published
Then:    Handler does not receive event
```

---

## 2.2 Concurrent Session Tests

### Test Group: ConcurrentSessionHandling

**Purpose**: Verify Bug 1 (race condition) is fully mitigated.

#### TC-INT-017: Concurrent Detection Multiple Sessions
```
Given:   10 sessions with simultaneous rate limits
When:    Detection runs for each
Then:    Each session gets exactly one timer
Verify:  No duplicate timers, correct session mapping
```

#### TC-INT-018: Rapid State Changes
```
Given:   Session in StateWaiting
When:    New rate limit detected while timer pending
Then:    Old timer cancelled, new timer scheduled
Verify:  Only one recovery attempt
```

#### TC-INT-019: Timer Race Condition
```
Given:   Concurrent ScheduleTimer calls for same session
When:    Both execute simultaneously
Then:    Only one timer created
Verify:  No timer leak, correct timing
```

---

## 2.3 User Input Conflict Tests

### Test Group: UserInputConflictHandling

**Purpose**: Verify Bug 5 (user input conflict) is mitigated.

#### TC-INT-020: User Responds Before Recovery
```
Given:   Rate limit detected, timer scheduled
When:    User sends input manually before timer fires
Then:    System detects user input, skips automated recovery
Verify:  No duplicate input sent
Coverage: Bug 5 mitigation
```

#### TC-INT-021: Recovery After User Timeout
```
Given:   User input detected recently
When:    Timer fires
Then:    Recovery skipped (user handled it)
```

#### TC-INT-022: No Recent User Input
```
Given:   No user input for 60+ seconds
When:    Timer fires
Then:    Recovery proceeds normally
```

---

# Section 3: End-to-End Tests (5%)

## 3.1 Complete Flow Tests

### Test Group: FullRecoveryFlows

**Purpose**: Verify complete user-facing scenarios.

#### TC-E2E-001: Complete Claude Rate Limit Recovery
```
Scenario: Claude Code session hits rate limit
Steps:
  1. Terminal outputs: "You exceeded the rate limit. Reset at 3:45 PM"
  2. System detects pattern (within 500ms)
  3. Timer scheduled for reset time + buffer
  4. Timer fires, recovery triggered
  5. "c\n" sent to tmux session
  6. Session continues normally
Expect: Recovery success, session functional
```

#### TC-E2E-002: Complete Aider Rate Limit Recovery
```
Scenario: Aider session hits rate limit
Steps:
  1. Terminal outputs: "Rate limit hit, waiting 60 seconds..."
  2. System detects pattern
  3. Timer scheduled for 60s
  4. Timer fires, recovery triggered
  5. "\n" sent to tmux session
Expect: Recovery success
```

#### TC-E2E-003: Rate Limit with Relative Time
```
Scenario: Rate limit with "retry after X minutes"
Steps:
  1. Terminal outputs: "Rate limit exceeded. Retry after 5 minutes."
  2. Timer scheduled for 5 minutes
  3. After 5 minutes + buffer, recovery triggered
Expect: Recovery after correct duration
```

#### TC-E2E-004: Multiple Rate Limits in Session
```
Scenario: Same session hits rate limit multiple times
Steps:
  1. First rate limit, recovery succeeds
  2. Second rate limit 10 minutes later
  3. Recovery succeeds again
Expect: Both handled correctly, no interference
```

### Test Group: ErrorRecoveryE2E

**Purpose**: Verify system handles failure scenarios gracefully.

#### TC-E2E-005: Recovery Failure with Retry
```
Scenario: Recovery input send fails
Steps:
  1. Rate limit detected
  2. First recovery attempt fails (tmux error)
  3. Retry after 5 seconds
  4. Second attempt succeeds
Expect: Final recovery success
```

#### TC-E2E-006: Recovery Failure Exhausts Retries
```
Scenario: All recovery attempts fail
Steps:
  1. Rate limit detected
  2. 3 recovery attempts all fail
  3. State = StateFailed
  4. Alert/log for manual intervention
Expect: Proper failure handling, no crash
```

#### TC-E2E-007: Session Crashes During Wait
```
Scenario: Session terminates during wait
Steps:
  1. Rate limit detected, timer scheduled
  2. Session crashes/terminates
  3. Timer fires, detects session gone
  4. Timer cancelled, no action
Expect: No error, clean cleanup
```

### Test Group: DisabledAutoRecovery

**Purpose**: Verify per-session override works.

#### TC-E2E-008: Per-Session Disable
```
Scenario: User disables auto-recovery for session
Steps:
  1. Session config has auto-recovery disabled
  2. Rate limit detected
  3. Detection logged, no timer scheduled
  4. User manually responds
Expect: No automated action, user manages recovery
```

---

## 3.2 Performance E2E Tests

### Test Group: PerformanceUnderLoad

**Purpose**: Verify non-functional requirements.

#### TC-E2E-009: Detection Latency
```
Scenario: Multiple sessions with rate limits
Metric:  Time from output to detection
Target:  < 500ms p95
```

#### TC-E2E-010: Memory Overhead
```
Scenario: 50 sessions monitored
Metric:  Memory per session
Target:  < 10MB overhead
```

#### TC-E2E-011: Timer Precision
```
Scenario: Timer scheduled for X seconds
Metric:  Actual vs scheduled fire time
Target:  Within 1 second accuracy
```

---

# Section 4: Property-Based Tests

## 4.1 Fuzzing Input Generation

### Test Group: InputFuzzing

**Purpose**: Discover unexpected patterns via random input.

#### TC-PBT-001: Random Terminal Output
```
Property: No crash on any valid UTF-8 input
Method:   Generate 10,000 random strings, feed to detector
Expect:   No panic, returns result (match or no match)
```

#### TC-PBT-002: Random Timestamp Formats
```
Property: Parser handles arbitrary date/time strings
Method:   Generate 1,000 random timestamp-like strings
Expect:   No panic, graceful handling (parsed or error)
```

#### TC-PBT-003: Invalid ANSI Sequences
```
Property: ANSI stripper handles malformed sequences
Method:   Generate random ANSI escape sequences
Expect:   No crash, output cleaned appropriately
```

## 4.2 Stateful Property Tests

### Test Group: StatefulPropertyTests

**Purpose**: Verify state machine invariants.

#### TC-PBT-004: State Transition Invariants
```
Property: Invalid transitions always rejected
Method:   Random sequence of state transitions
Expect:   Only valid transitions accepted
```

#### TC-PBT-005: Timer Firing Invariants
```
Property: Timer fires at most once per schedule
Method:   Random schedule/cancel sequences
Expect:   Callback executed at most once
```

---

# Section 5: Edge Case Coverage

## 5.1 Timing Edge Cases

| TC-ID | Edge Case | Expected Behavior |
|-------|-----------|-------------------|
| TC-EDGE-001 | Reset time exactly now | Immediate recovery trigger |
| TC-EDGE-002 | Reset time 1 second in future | Recovery after ~1s + buffer |
| TC-EDGE-003 | Reset time 1 millisecond in past | Immediate recovery (treat as past) |
| TC-EDGE-004 | System clock change during wait | Timer adjusts, recovery correct |
| TC-EDGE-005 | Leap year date | Date parsing handles Feb 29 |
| TC-EDGE-006 | Timezone in timestamp | Parse correctly, convert to local |

## 5.2 Data Edge Cases

| TC-ID | Edge Case | Expected Behavior |
|-------|-----------|-------------------|
| TC-EDGE-007 | Empty terminal output | No match, no error |
| TC-EDGE-008 | Null bytes in output | Handle gracefully |
| TC-EDGE-009 | Unicode in output | Match works with unicode |
| TC-EDGE-010 | Very long output (1MB) | Process efficiently, pattern found |
| TC-EDGE-011 | Binary data in output | Ignore, focus on text |
| TC-EDGE-012 | Output ends with partial pattern | Wait for complete pattern |

## 5.3 Session Edge Cases

| TC-ID | Edge Case | Expected Behavior |
|-------|-----------|-------------------|
| TC-EDGE-013 | Session pauses immediately after detection | Timer cancelled |
| TC-EDGE-014 | Session resumes after cooldown expires | Check if still needed |
| TC-EDGE-015 | Rate limit cleared before timer fires | No action, dialog gone |
| TC-EDGE-016 | Multiple providers in same output | Primary provider wins |
| TC-EDGE-017 | Session renamed during wait | Timer tracks by ID, not name |

## 5.4 Recovery Edge Cases

| TC-ID | Edge Case | Expected Behavior |
|-------|-----------|-------------------|
| TC-EDGE-018 | Input send to dead tmux socket | Retry fails, move to failed |
| TC-EDGE-019 | Session locked by another process | Retry with backoff |
| TC-EDGE-020 | tmux session in different state | Validate before send |

---

# Section 6: Test Data Management

## 6.1 Test Fixtures

```go
// Rate limit dialog fixtures per provider
var anthropicDialogs = []string{
    "You exceeded the rate limit. The API will reset at 3:45 PM.",
    "Rate limit exceeded. Press Enter to continue...",
    "You've hit a rate limit. The model will reset in 5 minutes.",
}

var openAIDialogs = []string{
    "Error 429: Too Many Requests",
    "Rate limit exceeded. Retry after 60 seconds.",
    "You exceeded your rate limit. Please retry after 2 minutes.",
}

var googleDialogs = []string{
    "rate_limit_error: quota exceeded",
    "429 Too Many Requests - rate limit",
    "Resource has been exhausted. Retry at 15:30:00.",
}

var aiderDialogs = []string{
    "Rate limit hit, waiting 30 seconds...",
    "API rate limit, retry in 1 minute",
    "Slowdown detected, throttling for 45s",
}
```

## 6.2 Mock Implementations

```go
// MockTimer for scheduler testing
type MockTimer struct {
    FireAt    time.Time
    Cancelled bool
    Fired     bool
}

// MockExternalStreamer for output consumer testing
type MockExternalStreamer struct {
    OutputChan chan string
    Consumed   []string
}

// MockInstance for recovery handler testing
type MockInstance struct {
    WriteInputCalls [][]byte
    SessionState    string
}
```

---

# Section 7: Test Execution Strategy

## 7.1 Execution Order

1. **Unit Tests** - Run in parallel, fast feedback
2. **Integration Tests** - Run after unit tests pass
3. **E2E Tests** - Run after integration tests pass
4. **Property Tests** - Run periodically and in CI

## 7.2 CI Pipeline Integration

```yaml
# Example CI configuration
test:
  script:
    - go test ./session/detection/ratelimit/... -v -race
    - go test ./session/detection/ratelimit/... -tags=integration
    - go test ./session/detection/ratelimit/... -tags=e2e
    - go test ./session/detection/ratelimit/... -tags=property
  coverage: /session\/detection\/ratelimit/
```

## 7.3 Test Naming Convention

```
TC-{LEVEL}-{GROUP}-{NUMBER}
TC-UT-001  = Unit Test, Group 001
TC-INT-001 = Integration Test, Group 001
TC-E2E-001 = End-to-End Test, Group 001
TC-PBT-001 = Property-Based Test, Group 001
TC-EDGE-001 = Edge Case Test, Group 001
```

---

# Section 8: Coverage Targets

## 8.1 Code Coverage Goals

| Test Level | Target Coverage | Critical Paths |
|------------|-----------------|----------------|
| Unit Tests | > 90% | All pattern matching, parsing, timer logic |
| Integration | > 80% | Component interactions |
| E2E | > 70% | Full user flows |

## 8.2 Known Issue Coverage

| Bug ID | Prevention Test Coverage |
|--------|-------------------------|
| Bug 1: Race Condition | TC-INT-017, TC-INT-018, TC-INT-019, TC-UT-030 |
| Bug 2: False Positives | TC-UT-046 through TC-UT-051 |
| Bug 3: Missing Dialogs | TC-INT-009, TC-INT-010 |
| Bug 4: Timing Issues | TC-UT-011 through TC-UT-020, TC-E2E-003 |
| Bug 5: User Input Conflict | TC-INT-020, TC-INT-021, TC-INT-022 |
| Bug 6: Session State Changes | TC-INT-004, TC-INT-005, TC-INT-006, TC-E2E-007 |
| Bug 7: Quota vs Rate Limit | TC-UT-052, TC-UT-053, TC-UT-054 |

---

# Section 9: Test Maintenance

## 9.1 Updating Tests

When adding new provider support:
1. Add provider patterns to Pattern Matching Tests
2. Add provider to Recovery Input Tests
3. Add provider dialog to fixtures
4. Add E2E test for new provider

When modifying timer logic:
1. Update Timer Scheduler Tests
2. Add new timing edge cases
3. Verify race condition tests still pass

## 9.2 Test Review Checklist

- [ ] Each known bug has corresponding prevention tests
- [ ] All state machine transitions tested
- [ ] All timing edge cases covered
- [ ] Property-based tests run periodically
- [ ] E2E tests cover primary user flows
- [ ] Test execution time < 5 minutes for unit tests
- [ ] Tests are deterministic (no flaky tests)

---

# Appendix: Quick Reference

## Test Matrix

| Category | Count | Execution Time |
|----------|-------|----------------|
| Unit Tests | 54 | ~30 seconds |
| Integration Tests | 22 | ~2 minutes |
| E2E Tests | 11 | ~5 minutes |
| Property Tests | 5 | ~1 minute |
| Edge Cases | 20 | ~30 seconds |
| **Total** | **112** | **~9 minutes** |

## Priority Test Execution

For rapid feedback during development:
1. Run unit tests (TC-UT-001 to TC-UT-030)
2. Run false positive tests (TC-UT-046 to TC-UT-051)
3. Run race condition tests (TC-INT-017 to TC-INT-019)
