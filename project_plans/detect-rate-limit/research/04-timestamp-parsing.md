# Research: Timestamp Parsing and Rate Limit Message Formats

## The Missing Claude Pattern

The requirements describe this exact message from Claude Code:
```
You've hit your limit - resets 11pm (America/Los_Angeles)
/extra-usage to finish what you're working on.
```

### Why it fails today

**Step 1 — Rate limit pattern check**: `defaultRateLimitPatterns` does not include `You've hit your limit`. The patterns that exist:
```
(?i)/rate-limit-options
(?i)rate limit.*exceeded
(?i)429.*Too Many Requests
(?i)rate_limit_error
(?i)Usage limit reached        ← would match "Usage limit reached for claude-3-opus"
(?i)rate limit reached
(?i)quota exceeded
```
`"You've hit your limit"` matches NONE of these. Detection returns `nil` before reaching timestamp parsing.

**Step 2 — Continue pattern check**: The message does not contain "Keep trying", "press enter", "[y/n]", or "Access resets at". The `/extra-usage` line is not in `defaultContinuePatterns`. So even if we add the rate-limit pattern, detection would still fail at the continue check.

**Step 3 — Timestamp pattern check**: Even if both above pass, the timestamp pattern `(?i)(?:reset at|Access resets at) (.+?)(?:\s*$|PT|PDT)` would not match `"resets 11pm (America/Los_Angeles)"` because it looks for "reset at" or "Access resets at", not bare "resets".

---

## Required Pattern Additions

### New rate limit pattern
```go
regexp.MustCompile(`(?i)You'?ve hit your limit`)
```
or more broadly:
```go
regexp.MustCompile(`(?i)(?:you'?ve hit|you have hit) your (usage )?limit`)
```

### New continue pattern
```go
regexp.MustCompile(`(?i)/extra-usage`)
```
or keep it general (the message always contains a slash-command option line).

### New timestamp pattern
```go
regexp.MustCompile(`(?i)resets\s+(\d{1,2}(?::\d{2})?(?:am|pm))\s*\(?([A-Za-z/_]+)\)?`)
```
This captures:
- Group 1: `11pm` or `11:30pm`
- Group 2: `America/Los_Angeles` or `Pacific` or `PDT`

Alternative single-capture approach that captures the full time+tz string:
```go
regexp.MustCompile(`(?i)resets\s+([\d:apm]+\s*[\w/]+)`)
```

---

## Timezone-Aware Parsing — Current State

`parseTimestamp()` uses `time.Parse(format, input)` with formats like `"3:04 PM"`. This uses `time.UTC` implicitly. The function does NOT call `time.LoadLocation()`.

When the captured group is `"11pm (America/Los_Angeles)"` or `"11pm America/Los_Angeles"`, `time.Parse("3pm", "11pm")` would succeed but the location would be UTC.

**Result**: reset time is computed in UTC, which could be off by up to 18 hours from the actual wall-clock reset time in the user's timezone.

---

## Fixing Timezone-Aware Parsing

### Approach 1: Parse time + location separately

```go
func parseTimeWithTZ(timeStr, tzStr string) time.Time {
    // Clean up tz string
    tzStr = strings.Trim(tzStr, "() ")
    
    // Try IANA name first (America/Los_Angeles)
    loc, err := time.LoadLocation(tzStr)
    if err != nil {
        // Try abbreviation lookup table
        loc = abbreviationToLocation(tzStr)
    }
    if loc == nil {
        loc = time.Local
    }
    
    // Parse wall-clock time
    formats := []string{"3pm", "3:04pm", "3 PM", "3:04 PM"}
    for _, f := range formats {
        if t, err := time.ParseInLocation(f, timeStr, loc); err == nil {
            // Anchor to today in that location
            now := time.Now().In(loc)
            t = time.Date(now.Year(), now.Month(), now.Day(),
                t.Hour(), t.Minute(), 0, 0, loc)
            if t.Before(now) {
                t = t.AddDate(0, 0, 1)
            }
            return t
        }
    }
    return time.Time{}
}
```

The key function is `time.ParseInLocation(format, value, loc)` — this is available in Go's standard library and correctly handles DST.

### Abbreviation mapping for common cases

Claude Code shows timezone abbreviations like `PDT`, `PST`, `EST`, `EDT`, `CST`, `CDT`, `MST`, `MDT`. Go's `time.LoadLocation` does not accept these abbreviations; they must be mapped manually:

```go
var tzAbbreviations = map[string]string{
    "PST": "America/Los_Angeles",
    "PDT": "America/Los_Angeles",
    "MST": "America/Denver",
    "MDT": "America/Denver",
    "CST": "America/Chicago",
    "CDT": "America/Chicago",
    "EST": "America/New_York",
    "EDT": "America/New_York",
    "UTC": "UTC",
    "GMT": "UTC",
    // Pacific/Mountain/Central/Eastern common names
    "Pacific":  "America/Los_Angeles",
    "Mountain": "America/Denver",
    "Central":  "America/Chicago",
    "Eastern":  "America/New_York",
}
```

Note: The IANA tzdata must be available at runtime. In Go 1.17+, `time/tzdata` can be embedded:
```go
import _ "time/tzdata"
```
This adds ~500KB to the binary and is the recommended approach for deployment environments without system tzdata (e.g., Alpine Linux Docker images).

---

## Confirmed Test Patterns in `detector_test.go`

All existing test cases use `"Access resets at 2:53 PM PDT"` format:

```go
output := `Usage limit reached for claude-3-opus.
Access resets at 2:53 PM PDT.
1. Keep trying
2. Stop`
```

The timestamp pattern `(?i)(?:reset at|Access resets at) (.+?)(?:\s*$|PT|PDT)` captures `"2:53 PM "` (with trailing space) when timezone is `PDT`. Then `parseTimestamp("2:53 PM")` calls `time.Parse("3:04 PM", "2:53 PM")` — **this succeeds but in UTC**, not PDT.

**TestParseTimestamp_SpecificTime** is not timezone-aware:
```go
hour := resetTime.Hour()
if hour != 15 && hour != 3 {
    t.Errorf("expected hour 15 (3 PM) or 3, got %d", hour)
}
```
It accepts either `3` or `15` because the test doesn't know if the parse returns local or UTC time.

---

## No Rate-Limit Testdata Files

`session/detection/testdata/` contains only general detection fixtures:
- `claude_active.txt`, `claude_idle_ready.txt`, `claude_input_required.txt`, `claude_needs_approval.txt`
- `gemini_active.txt`, `gemini_idle.txt`, `gemini_needs_approval.txt`
- `aider_active.txt`, `aider_needs_approval.txt`
- `opencode_active.txt`, `opencode_input_required.txt`, `opencode_needs_approval.txt`
- `gradle_numbered_output.txt`, `markdown_blockquote_numbered.txt`

**No `claude_rate_limited.txt` or similar.** The rate limit detector tests use inline strings only.

---

## Recommended Changes to `detector.go`

### 1. Add to `defaultRateLimitPatterns`
```go
regexp.MustCompile(`(?i)you'?ve hit your (usage )?limit`),
```

### 2. Add to `defaultContinuePatterns`  
```go
regexp.MustCompile(`(?i)/extra-usage`),
```
(or make this a third pattern list: "message is self-contained, no explicit continue prompt needed")

### 3. Add new timestamp capture pattern to `defaultTimestampPatterns`
```go
// "resets 11pm (America/Los_Angeles)" or "resets 11:30pm Pacific"
regexp.MustCompile(`(?i)resets\s+(\d{1,2}(?::\d{2})?(?:am|pm))\s*\(?([\w/]+)\)?`),
```
This requires a 2-capture-group variant — the current loop only uses `matches[1]`. Need a new path that passes both captures to a timezone-aware parser.

### 4. New `parseTimestampWithTZ(timeStr, tzStr string) time.Time` function
Using `time.ParseInLocation` + abbreviation map + IANA LoadLocation fallback.

### 5. Embed tzdata
```go
import _ "time/tzdata"
```
in `detector.go` or the package init.

### 6. Fix `DefaultResetBuffer` fallback
Current: 5 seconds. Requirements say 30-minute fallback when no time parsed.
```go
const DefaultFallbackWait = 30 * time.Minute
```
When `resetTime.IsZero()`, scheduler should use `DefaultFallbackWait` not `DefaultResetBuffer`.

---

## Re-Detection After Recovery

Current: After `executeRecovery()` succeeds, `detector.SetState(StateRecovered)`. `ProcessOutput()` then blocks on `currentState != StateNone && currentState != StateWaiting` — returns early forever.

Fix: After recovery executes, reset `currentState` to `StateNone` (with a new cooldown period) so the detector can catch a re-rate-limit. The `lastDetection` timestamp + cooldown mechanism already provides the re-detection throttle.

```go
// In Manager.executeRecovery() on success:
if detector != nil {
    detector.SetState(StateNone)  // Allow re-detection
    // lastDetection was set on the original detection; 
    // cooldown prevents immediate re-trigger
}
```

---

## Test Data to Create

Recommend adding `session/detection/ratelimit/testdata/claude_rate_limit_new_format.txt`:
```
You've hit your limit - resets 11pm (America/Los_Angeles)
/extra-usage to finish what you're working on.
```

And test cases:
- `TestDetector_ClaudeNewFormat_DetectsRateLimit`
- `TestParseTimestamp_TZAware_IANA` — `"11pm America/Los_Angeles"` → correct UTC time
- `TestParseTimestamp_TZAware_Abbreviation` — `"11pm PDT"` → correct UTC time
- `TestDetector_ReDetectionAfterRecovery`
