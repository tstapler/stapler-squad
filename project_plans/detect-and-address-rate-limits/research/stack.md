# Stack Research: Rate Limit Detection and Auto-Resolution

**Research Date**: 2026-04-05  
**Status**: Complete

## Executive Summary

This research evaluates technology options for detecting and automatically addressing rate limit dialogs in streaming terminal output. The system must detect rate limit dialogs from LLM providers, parse reset timestamps, and programmatically send input to tmux sessions when the reset time elapses.

---

## 1. Pattern Matching in Streaming Terminal Output

### Option A: Go `regexp` (Standard Library)
**Recommendation: Preferred for initial implementation**

- **Pros**:
  - No external dependencies
  - Well-tested and maintained
  - Pre-compiled `regexp.MustCompile()` at module init for zero runtime overhead
  - Sufficient performance for terminal output rates (~10-100 KB/s)

- **Cons**:
  - Slower than specialized parsers for complex patterns
  - Backtracking can be problematic with pathological inputs

- **Performance Notes**:
  - Go's `regexp` is known to be slower than PCRE/re2 in some benchmarks
  - For streaming terminal output, this is unlikely to be a bottleneck
  - Pre-compile patterns at startup; avoid `regexp.Compile()` in hot paths
  - Consider `regexp.MustCompile()` at module level for static patterns

- **Usage Pattern**:
  ```go
  // Compile once at startup
  var (
      anthropicRateLimitRE = regexp.MustCompile(`(?i)(rate limit|too many requests|429).*?(?:retry|wait|reset).*?(\d{1,2}:\d{2}(?::\d{2})?)`)
  )
  
  // Match in streaming handler
  func handleOutput(output string) {
      if match := anthropicRateLimitRE.FindStringSubmatch(output); match != nil {
          // Extract timestamp from match[1], schedule retry
      }
  }
  ```

### Option B: Dedicated Parsers (re2, ragel)
**Recommendation: Consider for advanced needs**

- **re2**: Google's RE2 library, faster than Go's regexp, but Go's regexp is already based on RE2
- **ragel**: State machine compiler for complex parsing; overkill for this use case

### Option C: Aho-Corasick Multi-Pattern Matching
**Recommendation: Consider if detecting many provider patterns**

- Use when you need to match many patterns simultaneously
- Can detect multiple provider rate limit formats in one pass
- Library: `github.com/petermattis/goid/gc` or implement custom

### Timing Strategies for Reset Detection

1. **Passive Detection via Terminal Polling**:
   - Use existing `tmux capture-pane` polling mechanism (already implemented)
   - Run pattern matching on each poll cycle (~100ms interval)
   - Extract timestamp from matched dialog

2. **Timestamp Parsing**:
   - Common formats: `HH:MM:SS`, Unix timestamp, "in X seconds"
   - Use Go's `time.Parse()` with multiple format layouts
   - Schedule timer for absolute time, not relative duration

3. **Scheduled Action**:
   ```go
   func scheduleRetry(resetTime time.Time) {
       duration := resetTime.Sub(time.Now())
       if duration <= 0 {
           sendRetryInput() // Already past
           return
       }
       time.AfterFunc(duration, sendRetryInput)
   }
   ```

---

## 2. ANSI Escape Sequence Parsing

### Existing Infrastructure in Stapler Squad

The application already has terminal output handling via:
- `tmux capture-pane` for polling terminal output
- xterm.js in the web UI for rendering

For rate limit detection, we need to parse the **text content** of the terminal, not the full ANSI structure. The captured pane output from tmux is already partially stripped.

### Option A: Simple Text Stripping
**Recommendation: Preferred for this use case**

- Rate limit dialogs are typically plain text, not styled
- Use `stripansi` or similar for basic ANSI removal if needed
- Library: `github.com/accraze/go-strip-ansi` or implement simple regex

```go
// Simple ANSI strip function
func stripANSI(s string) string {
    ansiRE := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
    return ansiRE.ReplaceAllString(s, "")
}
```

### Option B: Full ANSI Parser
**Recommendation: Not needed**

Full parsers like `github.com/charmbracelet/x/ansi` are designed for:
- Terminal UI rendering
- Cursor position tracking
- Color/style extraction

For rate limit detection, we only need the text content. The existing `capture-pane` already returns plain-ish text.

### Option C: xterm.js Integration
**Recommendation: Not applicable**

The web UI uses xterm.js for rendering, but rate limit detection happens server-side in Go, not in the browser.

---

## 3. Sending Input to tmux Sessions

### Existing Implementation

The codebase already has `tmux send-keys` functionality in `server/services/connectrpc_websocket.go`:

```go
// sendInputToTmux sends input bytes to a tmux session using tmux send-keys.
// Each byte is sent individually using -H (hex) format to handle special characters properly.
func sendInputToTmux(tmuxSessionName string, data []byte) error {
    args := []string{"send-keys", "-t", tmuxSessionName, "-H"}
    for _, b := range data {
        args = append(args, fmt.Sprintf("%02x", b))
    }
    cmd := exec.Command("tmux", args...)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("tmux send-keys failed: %w", err)
    }
    return nil
}
```

### Rate Limit Dialog Common Inputs

Based on research of LLM provider dialogs:

| Provider | Typical Input | Description |
|----------|---------------|-------------|
| Claude Code | Enter | "Press Enter to continue" |
| Claude Code | `c` | "Continue" option |
| Aider | Enter | Generic "press enter" |
| OpenAI CLI | `r` | "Retry" option |
| Google AI | `y` | "Yes, retry" |

### Implementation Pattern

```go
// Known rate limit input patterns
var rateLimitInputs = map[string][]byte{
    "anthropic": []byte("enter"),
    "openai":    []byte("r"),
    "google":    []byte("y"),
}

// Detect program type from terminal content or session metadata
func sendRetryInput(sessionName, programType string) error {
    input, ok := rateLimitInputs[programType]
    if !ok {
        input = []byte("enter") // Default fallback
    }
    return sendInputToTmux(sessionName, input)
}
```

---

## 4. Timing and Scheduling Strategies

### Option A: Go `time.AfterFunc` (Recommended)

```go
func scheduleAutoRetry(sessionName string, resetTime time.Time) {
    duration := resetTime.Sub(time.Now())
    
    if duration <= 0 {
        // Reset time already passed, retry immediately
        go sendRetryInput(sessionName, detectProgramType(sessionName))
        return
    }
    
    // Schedule for future
    time.AfterFunc(duration, func() {
        log.InfoLog.Printf("[RateLimit] Reset time elapsed for session '%s', sending retry input", sessionName)
        if err := sendRetryInput(sessionName, detectProgramType(sessionName)); err != nil {
            log.ErrorLog.Printf("[RateLimit] Failed to send retry input: %v", err)
        }
    })
}
```

**Pros**:
- Simple, built-in
- Non-blocking
- Handles timeouts gracefully

**Cons**:
- No persistence (lost on restart)
- Single timer per session

### Option B: Scheduler with Persistence

If robustness across restarts is needed:
- Store pending retries in session state
- On startup, reschedule any pending retries
- Use `time.Ticker` for periodic re-evaluation

### Option C: External Cron/Job System

Overkill for this use case. The simple timer approach is sufficient.

### Timing Considerations

1. **Buffer Time**: Add small buffer (e.g., 2-5 seconds) before sending retry
2. **Polling vs Push**: Use existing `capture-pane` polling (100ms) to detect dialog, then schedule timer
3. **Race Conditions**: Dialog might be cleared before timer fires — verify dialog still present before sending input

---

## 5. Rate Limit Dialog Formats by Provider

### Anthropic Claude Code

Common patterns observed in user reports:
- `Error 429: Rate limit exceeded. Please retry after X seconds.`
- `API Error: Rate limit exceeded (89%)` with dialog: "Press Enter to continue"
- Various "rate limit" messages during high-usage periods

Key patterns to detect:
- `(?i)(rate limit|429|too many requests)`
- `(?i)press.*enter.*continue`
- `(?i)will be reset at (\d{1,2}:\d{2})`

### OpenAI

- `Error code: 429 - {'error': {'message': 'Rate limit exceeded'}}`
- Often includes retry-after header / message

### Google AI (Gemini)

- `429 Resource exhausted`
- May include specific quota information

### Aider

- Uses litellm under the hood
- Displays various provider errors
- Common: `litellm.RateLimitError`

### Recommended Regex Patterns

```go
// Rate limit detection patterns
var (
    // General rate limit indicators
    rateLimitPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)rate limit.*exceeded`),
        regexp.MustCompile(`(?i)429.*too many`),
        regexp.MustCompile(`(?i)retry.*after.*(\d{1,2}:\d{2}(?::\d{2})?)`),
    }
    
    // Continue prompt detection  
    continuePromptPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)press.*enter.*continue`),
        regexp.MustCompile(`(?i)continue.*\?.*\[y/n\]`),
        regexp.MustCompile(`(?i)would you like to retry`),
    }
    
    // Timestamp extraction
    timestampPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)reset at (\d{1,2}:\d{2}(?::\d{2})?)`),
        regexp.MustCompile(`(?i)retry after (\d+)\s*(second|minute|hour)`),
        regexp.MustCompile(`(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})`),
    }
)
```

---

## 6. Architecture Patterns

### Pattern A: Polling-Based Detection (Recommended)

```
┌─────────────────────────────────────────────────────────────┐
│                     Rate Limit Detector                     │
├─────────────────────────────────────────────────────────────┤
│  1. On each capture-pane poll cycle (existing mechanism)   │
│     → Run pattern matching on new terminal content          │
│                                                              │
│  2. If rate limit dialog detected:                         │
│     → Parse reset timestamp from dialog                    │
│     → Determine program type (from session config/args)   │
│     → Schedule timer for reset time                        │
│                                                              │
│  3. When timer fires:                                       │
│     → Send configured input via tmux send-keys             │
│     → Log success/failure                                  │
└─────────────────────────────────────────────────────────────┘
```

**Integration Points**:
- Add detection logic to `streamViaTmuxCapturePane` or create dedicated goroutine
- Use existing session metadata to determine program type
- Reuse existing `sendInputToTmux` function

### Pattern B: Event-Driven Detection

- Subscribe to terminal output events
- More complex, requires event bus or channel-based architecture

### Implementation Location

Recommend adding to existing session management in `session/` package:
- New file: `session/rate_limit_detector.go`
- Or integrate into existing `tmux_process_manager.go`

---

## 7. Known Pitfalls and Failure Modes

### False Positives
- Normal "rate limit" mentions in documentation or code comments
- User-originated rate limit discussions
- **Mitigation**: Verify presence of "continue" or "retry" prompts before scheduling action

### Missed Detections
- Dialog appears and is dismissed before next poll
- **Mitigation**: Poll frequently (100ms is already implemented); check recent history

### Timing Issues
- Timestamp parsing failures due to unexpected formats
- Timer fires but rate limit hasn't actually reset
- **Mitigation**: Add buffer time (5 seconds); verify dialog still present before sending input

### Program Type Detection
- Can't determine which program created the rate limit
- **Mitigation**: Use session metadata (command arguments, working directory patterns)

### Multiple Rate Limits
- Session hits rate limit while already waiting for one
- **Mitigation**: Track pending retries; don't schedule duplicate timers

### tmux send-keys Failures
- Session died, tmux session gone
- **Mitigation**: Check session exists before sending; log failures

---

## 8. Recommended Tech Stack

| Component | Recommendation | Rationale |
|-----------|----------------|-----------|
| Pattern Matching | Go `regexp.MustCompile()` | No external deps, sufficient performance |
| ANSI Handling | Simple strip function | Dialogs are plain text |
| Input to tmux | Reuse existing `sendInputToTmux` | Already implemented |
| Scheduling | `time.AfterFunc` | Simple, built-in, non-blocking |
| Detection | Polling via existing capture-pane | Reuses existing infrastructure |

---

## 9. Next Steps

1. **Features Research**: Survey actual rate limit dialog formats from different LLM programs
2. **Architecture Design**: Create detailed design for detection and scheduling
3. **Implementation**: Add rate limit detector to session management
4. **Testing**: Test with various LLM programs and rate limit scenarios

---

## Sources

- Go standard library `regexp` documentation
- Existing `sendInputToTmux` implementation in `server/services/connectrpc_websocket.go`
- tmux `capture-pane` and `send-keys` documentation
- GitHub issues on Claude Code and Aider rate limit handling
- Reddit discussions on Claude rate limits
- Various blog posts on Go regex performance