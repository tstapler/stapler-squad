# Architecture Patterns for Rate Limit Detection and Auto-Recovery

**Research**: Streaming pattern detection and action triggering for rate limit handling
**Date**: 2026-04-05
**Status**: Complete

## Executive Summary

This document outlines architecture patterns for detecting rate limit dialogs in streaming terminal output and automatically triggering recovery actions. The design builds on existing Stapler Squad infrastructure for terminal streaming and status detection.

## 1. Design Patterns for Streaming Data Pattern Detection

### 1.1 Observer/Consumer Pattern (Existing Infrastructure)

The existing `ExternalStreamer` in `session/external_streamer.go` implements an observer pattern for terminal output:

```go
// OutputConsumer is a callback that receives terminal output from external sessions.
type OutputConsumer func(data []byte)

// AddConsumer registers a callback to receive output data.
func (s *ExternalStreamer) AddConsumer(consumer OutputConsumer, catchUp bool)
```

**Recommendation**: Extend this pattern to add a `RateLimitDetector` consumer that receives terminal output chunks.

### 1.2 Pipeline/Filter Pattern

A pipeline architecture allows chaining of processors:

```
[Terminal Output] → [ANSI Stripper] → [Pattern Matcher] → [Action Handler]
```

Each stage transforms or filters the data:

1. **ANSI Stripper** - Removes escape sequences (already implemented in `detector.go` via `stripANSI`)
2. **Pattern Matcher** - Runs regex patterns against cleaned text
3. **Action Handler** - Triggers recovery actions based on matches

**Implementation Location**: New package `session/detection/ratelimit/`

### 1.3 Regex-Based Pattern Matching (Existing)

The existing `StatusDetector` in `session/detection/detector.go` provides a proven pattern:

- YAML-defined regex patterns with priority ordering
- Per-program pattern sets (claude, aider, gemini, opencode)
- ANSI stripping before matching

**Recommendation**: Extend with a new `RateLimitDetector` using similar architecture.

## 2. Event-Driven Architecture

### 2.1 Event Types

Define events for the rate limit lifecycle:

```go
type RateLimitEventType int

const (
    RateLimitDetected RateLimitEventType = iota
    RateLimitResetApproaching
    RateLimitReset
    RateLimitRecoveryFailed
    RateLimitRecoverySuccess
)
```

### 2.2 Event Payload Structure

```go
type RateLimitEvent struct {
    Type        RateLimitEventType
    Provider    string          // "anthropic", "openai", "google"
    ResetTime   time.Time       // When the rate limit resets
    Message     string          // Raw matched dialog text
    SessionID   string          // Associated session
    Timestamp   time.Time
}
```

### 2.3 Event Bus Implementation

Use Go channels for in-process event distribution:

```go
type RateLimitEventBus struct {
    subscribers map[string]chan RateLimitEvent
    mu          sync.RWMutex
}

func (bus *RateLimitEventBus) Subscribe(id string) chan RateLimitEvent
func (bus *RateLimitEventBus) Publish(event RateLimitEvent)
```

### 2.4 Handler Interface

```go
type RateLimitHandler interface {
    OnRateLimitDetected(event RateLimitEvent)
    OnResetApproaching(event RateLimitEvent)
    OnResetElapsed(event RateLimitEvent)
    OnRecoveryFailed(event RateLimitEvent)
}
```

## 3. Integration with Existing Infrastructure

### 3.1 Terminal Output Sources

Two existing mechanisms provide terminal output:

| Source | File | Description |
|--------|------|-------------|
| Control Mode | `session/tmux/control_mode.go` | Real-time streaming via `tmux -C attach` |
| Capture-Pane | `session/external_tmux_streamer.go` | Polling-based fallback |

**Integration Point**: Add detector as an `OutputConsumer` to both streamers.

### 3.2 Input Sending Mechanism

Existing input sending infrastructure in `session/instance.go`:

```go
// WriteToPTY writes data to the PTY, sending input to the terminal session.
func (i *Instance) WriteToPTY(data []byte) (int, error)
```

Uses `tmuxManager.SendKeys()` which maps to `tmux send-keys` command.

**Recovery Action**: Call `WriteToPTY([]byte("\n"))` or `WriteToPTY([]byte("c"))` to continue after rate limit reset.

### 3.3 Session Access

The `Instance` struct provides the necessary interface:

```go
// Methods on *session.Instance
func (i *Instance) CapturePaneContent() (string, error)
func (i *Instance) WriteToPTY(data []byte) (int, error)
```

### 3.4 Integration Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     session.Instance                            │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐     │
│  │ TmuxManager  │    │ RateLimit    │    │ Status       │     │
│  │              │───▶│ Detector     │───▶│ Detector     │     │
│  │ + Capture()  │    │ + AddConsumer│    │ + Detect()   │     │
│  │ + SendKeys() │    │ + Handle()   │    │              │     │
│  └──────────────┘    └──────────────┘    └──────────────┘     │
└─────────────────────────────────────────────────────────────────┘
```

## 4. State Management for Rate Limit Tracking

### 4.1 State Machine

Define states for rate limit lifecycle:

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

### 4.2 State Store (Per-Session)

```go
type RateLimitState struct {
    SessionID      string
    State         RateLimitState
    Provider      string
    ResetTime     time.Time
    DetectedAt    time.Time
    LastCheckAt   time.Time
    RetryCount    int
    MaxRetries    int
}
```

### 4.3 State Persistence

Store in session metadata or separate state file:

```go
// In session.Instance or separate struct
rateLimitState    map[string]*RateLimitState
rateLimitStateMu  sync.RWMutex
```

### 4.4 Timer Management

Use `time.Timer` for reset time monitoring:

```go
type RateLimitTimer struct {
    sessionID  string
    timer      *time.Timer
    resetTime  time.Time
}

func (t *RateLimitTimer) ScheduleReset(resetTime time.Time)
func (t *RateLimitTimer) Cancel()
```

**Key Behavior**: Schedule timer for `resetTime.Add(buffer)` where buffer accounts for clock skew.

## 5. Rate Limit Dialog Patterns (LLM Providers)

### 5.1 Anthropic Claude Code

Based on research from GitHub issues:

- **Error messages**: `Error 429: Rate limit exceeded. Please retry after X seconds.`
- **UI patterns**: May show countdown timer or timestamp
- **Recovery input**: Typically `Enter` or `c` to continue

### 5.2 OpenAI/Aider

- **Error format**: `API rate limit reached. Please try again later.`
- **Dialog prompts**: May show "Press Enter to continue"

### 5.3 Google Gemini

- **Format**: Standard HTTP 429 with `Retry-After` header
- **UI**: Shows "Rate limit reached" with reset time

### 5.4 Pattern Definition Strategy

Define patterns as YAML for extensibility:

```yaml
rate_limits:
  anthropic:
    - name: "rate_limit_exceeded"
      pattern: '(?i)rate limit.*exceeded.*retry after (\d+) seconds?'
      retry_pattern: '(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})'
      recovery_input: "c"
      description: "Anthropic rate limit with seconds countdown"
    
    - name: "rate_limit_timestamp"
      pattern: '(?i)rate limit.*reset at (.+)'
      recovery_input: "c"
      description: "Anthropic rate limit with timestamp"

  openai:
    - name: "api_rate_limit"
      pattern: '(?i)API rate limit.*try again'
      recovery_input: "\n"
      description: "OpenAI rate limit"

  aider:
    - name: "aider_rate_limit"
      pattern: '(?i)rate limit.*please retry'
      recovery_input: "\n"
      description: "Aider rate limit"
```

## 6. Recommended Implementation Architecture

### 6.1 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Rate Limit Auto-Recovery                         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────────┐    ┌──────────────────┐    ┌──────────────┐ │
│  │ Output Sources  │───▶│ RateLimitDetector │───▶│ EventBus     │ │
│  │                 │    │                  │    │              │ │
│  │ + ExternalStream│    │ + stripANSI()    │    │ + Subscribe()│ │
│  │ + ControlMode  │    │ + matchPatterns()│    │ + Publish()  │ │
│  │ + capture-pane  │    │ + parseResetTime()   │              │ │
│  └─────────────────┘    └──────────────────┘    └──────┬───────┘ │
│                                                         │          │
│                                                         ▼          │
│  ┌─────────────────┐    ┌──────────────────┐    ┌──────────────┐ │
│  │ StateManager    │◀───│ TimerScheduler  │◀───│ Handlers     │ │
│  │                 │    │                  │    │              │ │
│  │ + SetState()   │    │ + ScheduleReset()   │ + OnReset()  │ │
│  │ + GetState()   │    │ + Cancel()       │    │ + SendInput()│ │
│  │ + Persist()    │    │                  │    │              │ │
│  └─────────────────┘    └──────────────────┘    └──────────────┘ │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### 6.2 Component Responsibilities

| Component | Responsibility |
|-----------|---------------|
| `RateLimitDetector` | Consumes terminal output, matches patterns, parses reset times |
| `EventBus` | Distributes rate limit events to handlers |
| `TimerScheduler` | Manages timers for reset time monitoring |
| `StateManager` | Tracks state per session, handles persistence |
| `RecoveryHandler` | Executes recovery actions (sends input to tmux) |

### 6.3 Thread Safety

Use sync primitives for concurrent access:

```go
type StateManager struct {
    states map[string]*RateLimitState
    mu     sync.RWMutex
}

// Timer management per session
type TimerManager struct {
    timers map[string]*time.Timer
    mu     sync.Mutex
}
```

## 7. Configuration and Extensibility

### 7.1 Feature Toggle

Add configuration for rate limit auto-recovery:

```json
{
  "rate_limit_auto_recovery": {
    "enabled": true,
    "providers": ["anthropic", "openai", "google"],
    "retry_count": 3,
    "reset_buffer_seconds": 5
  }
}
```

### 7.2 Per-Session Override

Allow enabling/disabling per session:

```go
type SessionOpts struct {
    // ... existing fields
    RateLimitAutoRecovery *bool  // nil means use global default
}
```

### 7.3 Logging Integration

Use existing logging infrastructure:

```go
import "github.com/tstapler/stapler-squad/log"

// Log rate limit events
log.InfoLog.Printf("[rate-limit] Detected for session %s, reset at %s", sessionID, resetTime)
log.ErrorLog.Printf("[rate-limit] Recovery failed for session %s: %v", sessionID, err)
```

## 8. Testing Strategy

### 8.1 Pattern Testing

```go
func TestRateLimitDetector_AnthropicPatterns(t *testing.T) {
    detector := NewRateLimitDetector()
    
    tests := []struct {
        name     string
        output   string
        expected bool
    }{
        {"rate_limit_429", "Error 429: Rate limit exceeded. Please retry after 60 seconds.", true},
        {"normal_output", "Processing your request...", false},
    }
    // ...
}
```

### 8.2 Integration Testing

Test full flow with mocked tmux:

```go
func TestRateLimitRecovery_Integration(t *testing.T) {
    // Setup: Create mock session with rate limit output
    // Execute: Run detection loop
    // Verify: Input sent after reset time
}
```

### 8.3 Chaos Testing

- Test with partial terminal output
- Test with ANSI escape sequences
- Test with network latency on timer check

## 9. Known Pitfalls and Mitigations

### 9.1 False Positives

**Problem**: Normal output matching rate limit patterns

**Mitigation**: 
- Use specific patterns with context (e.g., require "Error 429:" prefix)
- Implement cooldown between detections
- Require multiple pattern matches before triggering

### 9.2 Timing Issues

**Problem**: Clock skew between system time and rate limit reset time

**Mitigation**:
- Add buffer (5 seconds) before triggering recovery
- Verify terminal state changed (no longer showing rate limit)

### 9.3 Chunk Boundary Problems

**Problem**: Rate limit message split across multiple output chunks

**Mitigation**:
- Maintain sliding window of recent output
- Re-scan last N bytes on each chunk
- Use `capture-pane` for full content when pattern detected

### 9.4 Session State Changes

**Problem**: Session paused/stopped during rate limit wait

**Mitigation**:
- Track session status alongside rate limit state
- Cancel timers on session pause
- Resume monitoring on session resume

### 9.5 Multiple Rate Limits

**Problem**: Rapid successive rate limits from different providers

**Mitigation**:
- Deduplicate by provider per session
- Use per-provider cooldown
- Queue recovery actions

## 10. Alternative Approaches Considered

### 10.1 Polling-Based Detection

Instead of streaming, poll `capture-pane` periodically.

**Pros**: Simpler implementation, guaranteed full content
**Cons**: Higher latency, more resource intensive

**Chosen**: Streaming-based (more responsive)

### 10.2 External Process

Run rate limit detection in separate process.

**Pros**: Isolation, simpler debugging
**Cons**: Complexity of IPC, harder state management

**Chosen**: In-process (simpler integration)

### 10.3 Webhook-Based

Use external service to monitor and trigger recovery.

**Pros**: Platform independent
**Cons**: Network dependency, latency

**Chosen**: In-process (lower latency, no external dependency)

## 11. Summary

This architecture leverages existing Stapler Squad infrastructure:

1. **Pattern Detection**: Extend existing `StatusDetector` framework
2. **Event Distribution**: Use channel-based event bus
3. **Terminal Access**: Leverage `capture-pane` and control mode streaming
4. **Input Sending**: Use existing `WriteToPTY` method
5. **State Management**: Per-session state with timer scheduling

The design is modular, testable, and integrates cleanly with existing code.
