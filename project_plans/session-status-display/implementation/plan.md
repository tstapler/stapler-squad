# Implementation Plan: Session Status Display

## Overview

8 functional requirements across 3 layers: Go detection backend, ConnectRPC/proto API, and React UI. Requirements map to 7 files modified and 2 files created. Total task count: 34 tasks.

**Key constraint**: Do NOT add bare `⏺` to the `claude_thinking_verb` character class or any Active-priority pattern without an anchor (see FR-2 below and pitfalls research).

---

## Work Streams

The 8 FRs decompose into 4 independent work streams that can be executed in parallel after T-0 (ring buffer new file):

| Stream | Tasks | Description |
|---|---|---|
| A | FR-1, FR-5a | Regex fix + unit tests |
| B | FR-2, FR-5b | Screen-overwrite detection + unit tests |
| C | FR-6, FR-7, FR-8 | Ring buffer → proto → RPC handler → debug UI |
| D | FR-3, FR-4 | Frontend chip + row highlight |

Stream C has an internal dependency chain (FR-6 → FR-7 → FR-8). Streams A, B, D are independent. Streams A and B both touch `detector.go` and `detector_test.go` — serialize those tasks or resolve with separate sections.

---

## FR-1: Fix `✦` in `claude_thinking_verb` Pattern

### Problem
`session/detection/detector.go` line 568, the `claude_thinking_verb` Active pattern has character class `[·✢✳✶✻✽●*]` — missing `✦` (U+2726 BLACK FOUR POINTED STAR). Claude Code uses `✦` as its primary thinking spinner, producing lines like `✦ Thinking… (2m 5s · ↓ 6.4k tokens)`. These are never detected as `StatusActive`.

**Note from pitfalls research**: `✦` already appears in the `gemini_working` Processing pattern at line 435 as `(?:✦|⏲).*(?:Working|working)`. Adding it to the Active `claude_thinking_verb` char class does NOT conflict — the two patterns target different formats (`✦ <CapVerb>…` vs `✦.*Working`). Active has higher priority than Processing, so if both would match, Active wins (correct behavior).

**Do NOT add `⏺` (U+23FA) to the `claude_thinking_verb` char class or any Active-priority pattern**. The pitfalls research confirms stale `⏺` lines remain in the 4096-byte tail window after tool calls complete, causing false Active detections. The FR-2 screen-overwrite approach handles the actual tool-in-progress case more safely.

### Tasks

**T-1.1** — Edit `session/detection/detector.go` line 568:

Change:
```go
Pattern: `(?m)^[ \t]*[·✢✳✶✻✽●*][ \t]+[A-Z][a-zA-Z'\-éèêàâùûôîïëüöäÿæœ]*(?:…|\.{1,3})`,
```

To:
```go
Pattern: `(?m)^[ \t]*[·✢✳✶✻✽●*✦][ \t]+[A-Z][a-zA-Z'\-éèêàâùûôîïëüöäÿæœ]*(?:…|\.{1,3})`,
```

Update the comment at lines 563–567 to include `✦ (Claude Code U+2726)`:
```go
// Full spinner frame set: · ✢ ✳ ✶ ✻ ✽ (macOS bounce cycle), * (legacy),
// ● (reduced-motion), ✦ (Claude Code primary spinner U+2726).
// Verb char class extends \w with hyphens (Dilly-dallying), apostrophes (Beboppin'),
// and Latin-1 accented chars (Flambéing, Sautéing) — Go RE2 \w = [0-9A-Za-z_] only.
// Direct UTF-8 embedding in [...] is valid RE2; \uXXXX escapes are NOT supported by RE2.
// [ \t]* allows leading whitespace so indented spinners (e.g. task manager sub-items)
// are detected: "  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)"
```

**Dependencies**: None — standalone regex change.

**Risk**: Low. ✦ appears in frontend UI strings (`web-app/`) only; those never enter PTY detection. The `gemini_working` pattern at line 435 also references `✦` but for Processing, not Active — no conflict.

---

## FR-2: Screen-Overwrite CR/Cursor-Up Detection as Active Signal

### Problem
`collapseCarriageReturns()` at line 211 discards all CR-overwrite evidence before pattern matching. Spinner-based TUIs (e.g., braille spinners `⠋⠙⠹…`) that animate by writing `\r` never produce text keywords — they produce no Active signal at all. Similarly, ANSI cursor-up sequences (`\x1b[A`, `\x1b[NNA`) indicate active redraw.

The fix: check the raw `output []byte` **before** calling `collapseCarriageReturns()` in `Detect()` and `DetectWithContext()`. If the raw bytes contain `\r` not followed by `\n` (i.e., a true overwrite, not a Windows newline `\r\n`), or contain ANSI cursor-up sequences, treat this as a `StatusActive` signal — but only if no higher-priority pattern already matched.

**Critical constraint from pitfalls research**: Do NOT use a bare `⏺` character class to detect screen-overwrites. The detection must be triggered by the presence of `\r` (mid-line CR) or ANSI cursor-up in the raw input, not by any specific character in the content.

### Logic

Insert a `hasScreenOverwrite(raw []byte) bool` helper:

```go
// hasScreenOverwrite reports whether raw PTY bytes contain evidence of an
// in-progress spinner: a bare carriage return (not part of \r\n) or an
// ANSI cursor-up escape (\x1b[A or \x1b[NA).
// Must be called on the raw output before collapseCarriageReturns().
var cursorUpRegex = regexp.MustCompile(`\x1b\[\d*A`)

func hasScreenOverwrite(raw []byte) bool {
    s := string(raw)
    // \r not followed by \n
    for i, c := range s {
        if c == '\r' && (i+1 >= len(s) || s[i+1] != '\n') {
            return true
        }
    }
    return cursorUpRegex.Match(raw)
}
```

Then in `Detect()` (line 239) and `DetectWithContext()` (line 311), the screen-overwrite check is inserted **after** all text-based pattern checks, as a final fallback when result would otherwise be `StatusUnknown`:

```go
func (sd *StatusDetector) Detect(output []byte) DetectedStatus {
    // [all existing pattern checks unchanged — lines 241-306]
    // ... existing code returns early for Error, TestsFailing, Success, etc. ...

    // Fallback: raw screen-overwrite as active signal
    // Only fires when nothing else matched (StatusUnknown would be returned).
    if hasScreenOverwrite(output) {
        return StatusActive
    }
    return StatusUnknown
}
```

Apply the identical pattern to `DetectWithContext()`, returning `(StatusActive, "Screen overwrite — spinner actively redrawing")` when `hasScreenOverwrite(output)` is true.

**Placement**: The screen-overwrite check goes at lines 306-307 (after all regex checks, before `return StatusUnknown`). This preserves the existing priority order — any real pattern match wins over the screen-overwrite heuristic.

### Tasks

**T-2.1** — Add `cursorUpRegex` package-level var and `hasScreenOverwrite()` helper in `session/detection/detector.go`, just before the `StatusDetectionTailBytes` const (around line 232). Place after `ansiStripRegex` and `stripANSI`:

```go
// cursorUpRegex matches ANSI cursor-up sequences (\x1b[A or \x1b[NA).
var cursorUpRegex = regexp.MustCompile(`\x1b\[\d*A`)

// hasScreenOverwrite reports whether raw PTY bytes contain an active screen-overwrite
// signal: a bare carriage return (not part of \r\n, which is a Windows line ending)
// or an ANSI cursor-up escape sequence. Must be called on raw output before
// collapseCarriageReturns() so the overwrite evidence has not been discarded.
func hasScreenOverwrite(raw []byte) bool {
    s := string(raw)
    for i := 0; i < len(s)-1; i++ {
        if s[i] == '\r' && s[i+1] != '\n' {
            return true
        }
    }
    // Check last byte for lone \r
    if len(s) > 0 && s[len(s)-1] == '\r' {
        return true
    }
    return cursorUpRegex.Match(raw)
}
```

**T-2.2** — In `Detect()` at line 306 (just before `return StatusUnknown`), insert:

```go
    // Fallback: raw screen-overwrite evidence (bare \r or ANSI cursor-up) signals
    // an actively-animating spinner. Only fires when no text pattern matched.
    if hasScreenOverwrite(output) {
        return StatusActive
    }
```

**T-2.3** — In `DetectWithContext()` at line 377 (just before `return StatusUnknown, ""`), insert:

```go
    if hasScreenOverwrite(output) {
        return StatusActive, "Screen overwrite — spinner actively redrawing"
    }
```

**Dependencies**: T-2.1 must complete before T-2.2 and T-2.3.

**Risk**: Medium. `collapseCarriageReturns` at line 211 preserves trailing `\r` (Windows newlines) — the helper must not false-positive on `\r\n`. The implementation above checks `s[i+1] != '\n'` before flagging, which correctly handles this. Edge case: a line that is exactly `\r` (bare) also fires correctly due to the last-byte check.

---

## FR-3: Show Idle State in SubStatusChip

### Problem
`web-app/src/components/sessions/SubStatusChip.tsx` line 85: `case SubStatus.IDLE: return null`. SessionRow line 174 also has `session.subStatus !== SubStatus.IDLE` as a guard condition that prevents the chip from being passed to SubStatusChip at all.

Both must be changed.

### Tasks

**T-3.1** — Edit `web-app/src/components/sessions/SubStatusChip.css.ts`. Add after the `chipRateLimited` export (line 65):

```typescript
export const chipIdle = style([
  chip,
  {
    background: vars.color.surfaceSecondary ?? vars.color.hoverBackground,
    color: vars.color.textMuted,
    border: `1px solid ${vars.color.borderColor}`,
  },
]);
```

Note: Use `vars.color.textMuted` and `vars.color.borderColor` — these are defined in `theme-contract.css.ts`. Do not use undefined tokens. Check `web-app/src/styles/theme-contract.css.ts` for the exact available token names before writing. If `surfaceSecondary` is not defined, use `vars.color.hoverBackground` as the background.

**T-3.2** — Edit `web-app/src/components/sessions/SubStatusChip.tsx`:

1. Add `chipIdle` to the import from `"./SubStatusChip.css"` (line 4–11).
2. Update the JSDoc comment at line 19 to: `"Returns null for UNSPECIFIED only — IDLE renders a muted 'Waiting…' chip."`
3. Replace line 85–88 (`case SubStatus.IDLE:` through `default: return null`) with:

```typescript
    case SubStatus.IDLE:
      return (
        <span
          className={chipIdle}
          role="status"
          aria-label="Waiting for input"
          title="Session is idle — waiting for input"
        >
          ● Waiting…
        </span>
      );

    case SubStatus.UNSPECIFIED:
    default:
      return null;
```

**T-3.3** — Edit `web-app/src/components/sessions/SessionRow.tsx` lines 172–177. Remove the `session.subStatus !== SubStatus.IDLE &&` guard:

Before:
```tsx
{session.status === SessionStatus.ACTIVE &&
  session.subStatus !== SubStatus.UNSPECIFIED &&
  session.subStatus !== SubStatus.IDLE &&
  !(suppressApprovalSubStatus && session.subStatus === SubStatus.NEEDS_APPROVAL) && (
    <SubStatusChip subStatus={session.subStatus} />
  )}
```

After:
```tsx
{session.status === SessionStatus.ACTIVE &&
  session.subStatus !== SubStatus.UNSPECIFIED &&
  !(suppressApprovalSubStatus && session.subStatus === SubStatus.NEEDS_APPROVAL) && (
    <SubStatusChip subStatus={session.subStatus} />
  )}
```

**Dependencies**: T-3.1 must complete before T-3.2 (imports chipIdle). T-3.2 and T-3.3 are independent.

**Risk**: Low. The `toProtoSubStatus()` in `server/adapters/instance_adapter.go:191` already maps `StatusReady | StatusIdle → SUB_STATUS_IDLE`. The proto enum already has `SUB_STATUS_IDLE = 1`. No backend changes needed.

---

## FR-4: Row Highlight for Active/Processing Sessions

### Problem
Sessions with `subStatus === SUB_STATUS_PROCESSING` are visually identical to idle sessions in the list. A subtle left-border or background pulse should distinguish them at a glance.

### Tasks

**T-4.1** — Edit `web-app/src/components/sessions/SessionRow.css.ts`. Add a new `rowActive` export after the `rowPaused` export (line 200):

```typescript
// Applied to <li> when session subStatus is PROCESSING — left-border accent
// with optional pulse animation (disabled for reduced-motion users).
const rowActivePulse = keyframes({
  "0%": { borderLeftColor: vars.color.primary },
  "50%": { borderLeftColor: `${vars.color.primary}66` },
  "100%": { borderLeftColor: vars.color.primary },
});

export const rowActive = style({
  borderLeft: `3px solid ${vars.color.primary}`,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: rowActivePulse,
      animationDuration: "2s",
      animationIterationCount: "infinite",
      animationTimingFunction: "ease-in-out",
    },
  },
});
```

Note: `vars.color.primary` is the accent color used in `chipProcessing` (line 35 of SubStatusChip.css.ts). This ensures visual consistency between the row border and the chip color.

**T-4.2** — Edit `web-app/src/components/sessions/SessionRow.tsx`:

1. Add `rowActive` to the imports from `"./SessionRow.css"` (line 8–24).
2. Add `SubStatus` is already imported from `"@/gen/session/v1/types_pb"` at line 4.
3. Update the `className` array at lines 142–146:

Before:
```tsx
className={[
  row,
  memMB > 500 ? rowMemoryPressure : "",
  isPaused ? rowPaused : "",
].filter(Boolean).join(" ")}
```

After:
```tsx
className={[
  row,
  memMB > 500 ? rowMemoryPressure : "",
  isPaused ? rowPaused : "",
  session.status === SessionStatus.ACTIVE && session.subStatus === SubStatus.PROCESSING ? rowActive : "",
].filter(Boolean).join(" ")}
```

**Dependencies**: T-4.1 must complete before T-4.2.

**Risk**: Low. `vars.color.primary` is defined in `theme-contract.css.ts`. Check the exact token name — it may be `vars.color.actionPrimary` or `vars.color.primary`. Use the token that matches the `chipProcessing` style in `SubStatusChip.css.ts` line 35 (`vars.color.primary`) for consistency.

---

## FR-5: Pattern Unit Tests

### Tasks

**T-5.1** — Add tests for FR-1 (✦ detection) in `session/detection/detector_test.go`. Append to `TestStatusDetector_DetectActive`:

```go
func TestStatusDetector_DetectActive_StarFourPointed(t *testing.T) {
    sd := NewStatusDetector()
    testCases := []struct {
        input string
        desc  string
    }{
        {
            "✦ Thinking… (2m 5s · ↓ 6.4k tokens)\n",
            "claude thinking verb with star four pointed spinner",
        },
        {
            "  ✦ Searching…\n",
            "indented thinking verb",
        },
        {
            "✦ Compiling...\n",
            "dot ellipsis variant",
        },
    }
    for _, tc := range testCases {
        t.Run(tc.desc, func(t *testing.T) {
            status := sd.Detect([]byte(tc.input))
            if status != StatusActive {
                t.Errorf("Detect(%q) = %v, want StatusActive", tc.input, status)
            }
        })
    }
}
```

**T-5.2** — Add tests for FR-2 (screen-overwrite) in `session/detection/detector_test.go`. Add a new test function:

```go
func TestStatusDetector_DetectActive_ScreenOverwrite(t *testing.T) {
    sd := NewStatusDetector()

    // Pure CR spinner — no keywords, detected via hasScreenOverwrite
    input := "⠋ Thinking\r⠙ Thinking\r⠹ Thinking\n"
    status := sd.Detect([]byte(input))
    if status != StatusActive {
        t.Errorf("Detect(CR spinner) = %v, want StatusActive", status)
    }

    // ANSI cursor-up spinner
    input2 := "Working...\x1b[AWorking..\n"
    status2 := sd.Detect([]byte(input2))
    if status2 != StatusActive {
        t.Errorf("Detect(cursor-up spinner) = %v, want StatusActive", status2)
    }

    // Windows newline \r\n must NOT trigger screen-overwrite
    input3 := "Normal line\r\nAnother line\r\n"
    // Should not be StatusActive (no real overwrite present)
    status3 := sd.Detect([]byte(input3))
    if status3 == StatusActive {
        t.Errorf("Detect(CRLF newlines) = StatusActive, want anything else (CRLF is not a screen overwrite)")
    }

    // Higher-priority pattern wins over screen-overwrite
    input4 := "Error: connection refused\r⠋ Retrying\r"
    status4 := sd.Detect([]byte(input4))
    if status4 != StatusError {
        t.Errorf("Detect(error+overwrite) = %v, want StatusError (higher priority wins)", status4)
    }
}
```

**T-5.3** — Verify existing tests still pass. Run:
```
cd <repo_root> && go test ./session/detection/... -v -run "TestStatusDetector"
```

No existing tests should regress.

**Dependencies**: T-5.1 depends on T-1.1 completing. T-5.2 depends on T-2.1–T-2.3 completing. T-5.3 depends on all previous tasks.

---

## FR-6: DetectionEvent Ring Buffer + RecentEvents() Method

### Problem
No observability exists for why patterns match or don't match. A bounded ring buffer of `DetectionEvent` structs gives debugging data without growing unboundedly.

### New File: `session/detection/events.go`

**T-6.1** — Create `session/detection/events.go`:

```go
package detection

import (
    "sync"
    "time"
)

// DetectionEvent records a single invocation of Detect() or DetectWithContext().
type DetectionEvent struct {
    SessionID        string
    Timestamp        time.Time
    MatchedPattern   string         // Pattern Name field, or "<none>" if StatusUnknown
    MatchedCategory  string         // "active", "processing", "idle", "error", "success", "needs_approval", "input_required", "tests_failing", "ready", or "unknown"
    ResultStatus     DetectedStatus
    TailSnippet      string         // Last 512 bytes of cleaned terminal output (post-strip, post-collapse)
}

const (
    // EventRingCap is the maximum number of DetectionEvents retained per StatusDetector.
    EventRingCap    = 500
    // TailSnippetBytes is the maximum bytes captured in TailSnippet.
    TailSnippetBytes = 512
)

// eventRing is a fixed-capacity ring buffer of DetectionEvents.
type eventRing struct {
    mu     sync.Mutex
    events [EventRingCap]DetectionEvent
    head   int // next write position
    count  int // total filled slots (capped at EventRingCap)
}

// push adds an event to the ring, overwriting the oldest if full.
func (r *eventRing) push(e DetectionEvent) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.events[r.head] = e
    r.head = (r.head + 1) % EventRingCap
    if r.count < EventRingCap {
        r.count++
    }
}

// recent returns up to n most-recent events, newest-first.
func (r *eventRing) recent(n int) []DetectionEvent {
    r.mu.Lock()
    defer r.mu.Unlock()
    if n <= 0 || r.count == 0 {
        return nil
    }
    if n > r.count {
        n = r.count
    }
    out := make([]DetectionEvent, n)
    // head-1 is the most recently written slot
    for i := 0; i < n; i++ {
        idx := (r.head - 1 - i + EventRingCap) % EventRingCap
        out[i] = r.events[idx]
    }
    return out
}
```

**T-6.2** — Add `ring eventRing` field to `StatusDetector` struct in `detector.go` (after line 63):

```go
type StatusDetector struct {
    patterns StatusPatterns
    // ... existing regex fields unchanged ...
    ring eventRing  // in-memory detection event ring buffer; shared across sessions
}
```

Note: The ring buffer is per-`StatusDetector` instance. Since each `ClaudeController` owns its own `StatusDetector` (created in `claude_controller.go:154`), the ring is naturally per-session.

**T-6.3** — Add `categoryName()` helper in `events.go` (converts `DetectedStatus` to the category string stored in `DetectionEvent.MatchedCategory`):

```go
// categoryName returns the MatchedCategory string for a DetectedStatus.
func categoryName(s DetectedStatus) string {
    switch s {
    case StatusActive:
        return "active"
    case StatusProcessing:
        return "processing"
    case StatusIdle:
        return "idle"
    case StatusError:
        return "error"
    case StatusSuccess:
        return "success"
    case StatusNeedsApproval:
        return "needs_approval"
    case StatusInputRequired:
        return "input_required"
    case StatusTestsFailing:
        return "tests_failing"
    case StatusReady:
        return "ready"
    default:
        return "unknown"
    }
}
```

**T-6.4** — Add `appendDetectionEvent()` private helper in `detector.go` (after `DetectWithContext()`, around line 379). This helper is called from both `Detect()` and `DetectWithContext()` after the result is determined:

```go
// appendDetectionEvent records a detection outcome to the ring buffer.
// sessionID may be empty when called from DetectRecent/DetectForProgram paths
// that do not have session context; in that case the event is still recorded
// with an empty SessionID.
func (sd *StatusDetector) appendDetectionEvent(sessionID string, status DetectedStatus, patternName, cleanedText string) {
    snippet := cleanedText
    if len(snippet) > TailSnippetBytes {
        snippet = snippet[len(snippet)-TailSnippetBytes:]
    }
    category := categoryName(status)
    if patternName == "" {
        patternName = "<none>"
    }
    sd.ring.push(DetectionEvent{
        SessionID:       sessionID,
        Timestamp:       time.Now(),
        MatchedPattern:  patternName,
        MatchedCategory: category,
        ResultStatus:    status,
        TailSnippet:     snippet,
    })
}
```

**T-6.5** — Modify `Detect()` to append events. The method currently returns early on each match. Refactor into a two-phase approach:

Rename the existing body of `Detect()` into a private `detect()` that returns `(DetectedStatus, string)` (status + matched pattern name). Then wrap in `Detect()`:

```go
func (sd *StatusDetector) Detect(output []byte) DetectedStatus {
    text := stripANSI(collapseCarriageReturns(string(output)))
    status, patternName := sd.detectFromText(text, output)
    sd.appendDetectionEvent("", status, patternName, text)
    return status
}
```

Where `detectFromText(text string, raw []byte) (DetectedStatus, string)` contains all the existing pattern-matching logic (moved verbatim from the current `Detect()` body), plus the screen-overwrite fallback from T-2.2, and returns both the status and the name of the matched pattern.

**Important**: `DetectWithContext()` must also be refactored to call the shared `detectFromText()` and then `appendDetectionEvent()`. This avoids duplicating the pattern-matching logic.

**T-6.6** — Add `RecentEvents(n int) []DetectionEvent` public method to `StatusDetector` in `detector.go` (or in `events.go`):

```go
// RecentEvents returns up to n most-recent DetectionEvents, newest-first.
func (sd *StatusDetector) RecentEvents(n int) []DetectionEvent {
    return sd.ring.recent(n)
}
```

**T-6.7** — The `ClaudeController` creates its `StatusDetector` at line 154: `cc.statusDetector = detection.NewStatusDetector()`. The controller needs to inject the session ID into events. Two options:

Option A (preferred): Add a `SetSessionID(id string)` method to `StatusDetector` that stores the ID and prefixes all future `appendDetectionEvent()` calls with it:

```go
// In StatusDetector struct, add:
sessionID string

// Add method:
func (sd *StatusDetector) SetSessionID(id string) {
    sd.sessionID = id
}

// In appendDetectionEvent, use sd.sessionID instead of the parameter.
```

Then in `claude_controller.go` after line 154 (where the detector is created):
```go
cc.statusDetector.SetSessionID(instance.GetTitle())
```

**Dependencies**: T-6.1 (new file) must be created before T-6.2–T-6.7. T-6.4 depends on T-6.3. T-6.5 is the most complex refactor and should be done last within this FR to avoid merge conflicts with T-2.2/T-2.3.

**Risk**: High for T-6.5 (refactor of hot-path `Detect()` function). The implementation agent should read the full `Detect()` and `DetectWithContext()` bodies before refactoring. The two functions have nearly identical pattern-matching loops — extract `detectFromText()` carefully to share the logic while preserving both public signatures.

---

## FR-7: GetDetectionEvents ConnectRPC Endpoint

### Proto Changes

**T-7.1** — Edit `proto/session/v1/session.proto`. Add a new RPC to the `SessionService` service (after the last `rpc RunWorkflow` line around line 370):

```protobuf
  // GetDetectionEvents returns recent status detection events for a session (debug use).
  rpc GetDetectionEvents(GetDetectionEventsRequest) returns (GetDetectionEventsResponse) {}
```

**T-7.2** — Add new message definitions in `proto/session/v1/session.proto` (at the bottom, before the closing brace of the file):

```protobuf
// DetectionEventProto is the wire representation of a single status detection event.
message DetectionEventProto {
  string session_id = 1;
  google.protobuf.Timestamp timestamp = 2;
  string matched_pattern = 3;
  string matched_category = 4;
  int32 result_status = 5;   // maps to DetectedStatus int value
  string tail_snippet = 6;
}

message GetDetectionEventsRequest {
  string session_id = 1;
  int32 limit = 2;  // max events to return; capped at 100 server-side; 0 means default (20)
}

message GetDetectionEventsResponse {
  repeated DetectionEventProto events = 1;
}
```

Verify that `google/protobuf/timestamp.proto` is already imported in `session.proto`. If not, add:
```protobuf
import "google/protobuf/timestamp.proto";
```

Check existing imports at the top of the file before adding.

**T-7.3** — Run `make generate-proto` from the repo root. This regenerates:
- `session/gen/session/v1/*.go`
- `web-app/src/gen/session/v1/*_pb.ts`

Commit all generated files together with the `.proto` source change.

**T-7.4** — Implement the handler in `server/services/session_service.go`. Add a new method after the last `RunWorkflow` handler (find it by searching for `func (s *SessionService) RunWorkflow`):

```go
// GetDetectionEvents returns recent status-detection events for a session.
// Used by the debug panel (FR-8) — not intended for production UI.
func (s *SessionService) GetDetectionEvents(
    ctx context.Context,
    req *connect.Request[sessionv1.GetDetectionEventsRequest],
) (*connect.Response[sessionv1.GetDetectionEventsResponse], error) {
    inst := s.findInstance(req.Msg.SessionId)
    if inst == nil {
        return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session %q not found", req.Msg.SessionId))
    }

    limit := int(req.Msg.Limit)
    if limit <= 0 {
        limit = 20
    }
    if limit > 100 {
        limit = 100
    }

    // Access the controller's status detector via InstanceStatusManager.
    controller, ok := s.statusManager.GetController(inst.Title)
    if !ok || controller == nil {
        return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{}), nil
    }

    events := controller.GetStatusDetector().RecentEvents(limit)
    protoEvents := make([]*sessionv1.DetectionEventProto, 0, len(events))
    for _, e := range events {
        protoEvents = append(protoEvents, &sessionv1.DetectionEventProto{
            SessionId:       e.SessionID,
            Timestamp:       timestamppb.New(e.Timestamp),
            MatchedPattern:  e.MatchedPattern,
            MatchedCategory: e.MatchedCategory,
            ResultStatus:    int32(e.ResultStatus),
            TailSnippet:     e.TailSnippet,
        })
    }
    return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{Events: protoEvents}), nil
}
```

The `controller.GetStatusDetector()` method needs to be added to `ClaudeController` (see T-7.5).

**T-7.5** — Add `GetStatusDetector()` accessor to `ClaudeController` in `session/claude_controller.go`:

```go
// GetStatusDetector returns the status detector used by this controller.
// Used by GetDetectionEvents to retrieve recent detection events.
func (cc *ClaudeController) GetStatusDetector() *detection.StatusDetector {
    return cc.statusDetector
}
```

**T-7.6** — Add required imports to `session_service.go` if not already present:
- `"google.golang.org/protobuf/types/known/timestamppb"`
- The generated proto package alias (already imported as `sessionv1` or similar — check existing imports at line 1–56)

**Dependencies**: T-7.1 and T-7.2 must be done together (they're edits to the same file). T-7.3 (make generate-proto) depends on T-7.1+T-7.2. T-7.4 depends on T-7.3 (generated Go types needed). T-7.5 depends on FR-6 completing (needs `GetStatusDetector()` to exist on the detector). T-7.6 is part of T-7.4.

**Risk**: Medium. The `statusManager` field on `SessionService` may be nil during startup (sessions loaded before wiring). The handler already guards for this: `if !ok || controller == nil { return empty response }`. Additionally, not all instances have a controller (e.g., paused or hibernated sessions) — the nil check handles this.

---

## FR-8: Debug Panel in Session Detail View

### Tasks

**T-8.1** — Create `web-app/src/components/sessions/DetectionEventsPanel.tsx`:

```tsx
"use client";

import { useEffect, useState, useCallback } from "react";
import { useSessionClient } from "@/lib/hooks/useSessionClient";
import type { DetectionEventProto } from "@/gen/session/v1/session_pb";

interface DetectionEventsPanelProps {
  sessionId: string;
}

/**
 * DetectionEventsPanel renders the last 20 status-detection events for a session.
 * Only shown when ?debug=1 is present in the URL query string.
 */
export function DetectionEventsPanel({ sessionId }: DetectionEventsPanelProps) {
  const client = useSessionClient();
  const [events, setEvents] = useState<DetectionEventProto[]>([]);
  const [error, setError] = useState<string | null>(null);

  const fetchEvents = useCallback(async () => {
    try {
      const response = await client.getDetectionEvents({
        sessionId,
        limit: 20,
      });
      setEvents(response.events);
    } catch (e) {
      setError(String(e));
    }
  }, [client, sessionId]);

  useEffect(() => {
    void fetchEvents();
    const interval = setInterval(() => void fetchEvents(), 3000);
    return () => clearInterval(interval);
  }, [fetchEvents]);

  if (error) {
    return <div style={{ color: "red", padding: "8px" }}>Detection events error: {error}</div>;
  }

  return (
    <section style={{ marginTop: "16px", fontFamily: "monospace", fontSize: "12px" }}>
      <h4 style={{ marginBottom: "8px" }}>Detection Events (debug)</h4>
      {events.length === 0 ? (
        <p style={{ color: "#888" }}>No events yet.</p>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th style={{ textAlign: "left", padding: "4px" }}>Time</th>
              <th style={{ textAlign: "left", padding: "4px" }}>Pattern</th>
              <th style={{ textAlign: "left", padding: "4px" }}>Category</th>
              <th style={{ textAlign: "left", padding: "4px" }}>Status</th>
              <th style={{ textAlign: "left", padding: "4px" }}>Snippet</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e, i) => (
              <tr key={i} style={{ borderTop: "1px solid #333" }}>
                <td style={{ padding: "4px", whiteSpace: "nowrap" }}>
                  {e.timestamp ? new Date(Number(e.timestamp.seconds) * 1000).toLocaleTimeString() : "—"}
                </td>
                <td style={{ padding: "4px" }}>{e.matchedPattern}</td>
                <td style={{ padding: "4px" }}>{e.matchedCategory}</td>
                <td style={{ padding: "4px" }}>{e.resultStatus}</td>
                <td
                  style={{ padding: "4px", maxWidth: "300px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
                  title={e.tailSnippet}
                >
                  {e.tailSnippet.slice(0, 80)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
```

Note: Check the actual import path for `useSessionClient` — it may be `@/lib/hooks/useSessionClient` or similar. Check existing session detail page imports before writing this file.

**T-8.2** — Find the session detail view component. Search for the file that renders individual session details (likely `web-app/src/components/sessions/SessionDetail.tsx` or similar — check with `grep -rn "sessionId\|session\.id" web-app/src/app` for the detail page). Once found:

1. Add import: `import { DetectionEventsPanel } from "../sessions/DetectionEventsPanel";`
2. Add `?debug=1` check at the component level:
   ```tsx
   const isDebugMode = typeof window !== "undefined" && new URLSearchParams(window.location.search).get("debug") === "1";
   ```
3. Add at the bottom of the render output:
   ```tsx
   {isDebugMode && <DetectionEventsPanel sessionId={session.id} />}
   ```

**T-8.3** — Verify the `getDetectionEvents` method is available on the session client. The client is generated from the proto service definition by `make generate-proto` (T-7.3). After T-7.3 completes, the method will be available as `client.getDetectionEvents(...)`.

**Dependencies**: T-8.1 can be written immediately (self-contained component). T-8.2 depends on locating the correct session detail file (read the file first). T-8.3 depends on T-7.3 (proto generation). The full debug panel is not functional until FR-6 and FR-7 are complete.

**Risk**: Low for T-8.1. Medium for T-8.2 — the session detail view must be found first; if it doesn't exist as a standalone component, the panel may need to be inserted into a different container. Also check whether `useSessionClient` is the correct hook name for making RPC calls.

---

## Execution Order

```
Parallel track A: T-1.1 → T-5.1
Parallel track B: T-2.1 → T-2.2 → T-2.3 → T-5.2
Parallel track D: T-3.1 → T-3.2 → T-3.3 (FR-3); T-4.1 → T-4.2 (FR-4)

Sequential track C (FR-6 → FR-7 → FR-8):
  T-6.1 (new file) → T-6.2 → T-6.3 → T-6.4 → T-6.5 → T-6.6 → T-6.7
  → T-7.1 + T-7.2 (proto edits, same file) → T-7.3 (make generate-proto)
  → T-7.4 + T-7.5 (handler + accessor) → T-7.6
  → T-8.1 + T-8.2 → T-8.3

Final: T-5.3 (verify all tests pass)
```

Tracks A, B, D can run concurrently. Track C must be serialized. A and B both write to `detector.go` and `detector_test.go` — if running in parallel, resolve with a clear section boundary: A owns the `getDefaultPatterns()` function; B owns the area after `stripANSI` and inside `Detect()`/`DetectWithContext()`.

---

## Files Affected

| File | Tasks | Type |
|---|---|---|
| `session/detection/detector.go` | T-1.1, T-2.1–T-2.3, T-6.2, T-6.4–T-6.7, T-7.5 | Modify |
| `session/detection/events.go` | T-6.1, T-6.3, T-6.6 | **Create** |
| `session/detection/detector_test.go` | T-5.1, T-5.2 | Modify |
| `session/claude_controller.go` | T-6.7 (SetSessionID call), T-7.5 (GetStatusDetector) | Modify |
| `proto/session/v1/session.proto` | T-7.1, T-7.2 | Modify |
| `session/gen/session/v1/*.go` | T-7.3 (generated) | Auto-generated |
| `web-app/src/gen/session/v1/*_pb.ts` | T-7.3 (generated) | Auto-generated |
| `server/services/session_service.go` | T-7.4, T-7.6 | Modify |
| `web-app/src/components/sessions/SubStatusChip.css.ts` | T-3.1 | Modify |
| `web-app/src/components/sessions/SubStatusChip.tsx` | T-3.2 | Modify |
| `web-app/src/components/sessions/SessionRow.tsx` | T-3.3, T-4.2 | Modify |
| `web-app/src/components/sessions/SessionRow.css.ts` | T-4.1 | Modify |
| `web-app/src/components/sessions/DetectionEventsPanel.tsx` | T-8.1 | **Create** |
| Session detail view (TBD path) | T-8.2 | Modify |

---

## Risk and Issue Table

| ID | Risk | Severity | Mitigation |
|---|---|---|---|
| R-1 | `⏺` false positives — stale scrollback fires Active when session is idle | HIGH | Do NOT add bare `⏺` to Active patterns. FR-2 uses structural `\r` detection, not character matching. |
| R-2 | `hasScreenOverwrite()` false-positive on `\r\n` (Windows newlines) | MEDIUM | Implement `if s[i] == '\r' && i+1 < len(s) && s[i+1] != '\n'` check. Add regression test (T-5.2 includes this). |
| R-3 | `Detect()` refactor (T-6.5) breaks existing behavior | HIGH | Extract `detectFromText()` preserving all early-return semantics. Run full test suite (`go test ./session/detection/...`) before and after. |
| R-4 | `vars.color.*` token name mismatch in vanilla-extract files | LOW | Check `web-app/src/styles/theme-contract.css.ts` for exact token names before writing new styles. |
| R-5 | `GetStatusDetector()` accessor exposes internal detector — not thread-safe for concurrent callers | MEDIUM | The `RecentEvents()` method acquires `ring.mu`. The accessor itself just returns a pointer; no additional locking needed since `StatusDetector` is used from a single controller goroutine. Document this in the method. |
| R-6 | `make generate-proto` fails if `google/protobuf/timestamp.proto` not imported | MEDIUM | Check existing imports in `session.proto` before adding the timestamp message field. If already imported, no action needed. |
| R-7 | Session detail view location unknown at plan time (T-8.2) | LOW | Implementation agent must `grep -rn "sessionId\|session detail"` in `web-app/src/app` to locate the correct component before modifying it. |
| R-8 | `statusManager` is nil during startup — `GetDetectionEvents` handler will return empty | LOW | Guard with `if s.statusManager == nil { return empty response }` in T-7.4. Acceptable: debug panel shows "No events yet" during startup. |
| R-9 | `time.Now()` import missing in `events.go` | LOW | Import `"time"` at the top of the new file. |
| R-10 | `✦` in `gemini_working` Processing pattern (line 435) conflicts with new Active match | LOW | No conflict: Active is checked before Processing; `✦ Thinking…` matches Active's `claude_thinking_verb`; `✦ Working...` matches both but Active wins (correct). |

---

## Acceptance Criteria Checklist

- [ ] `sd.Detect([]byte("✦ Thinking… (2m 5s · ↓ 6.4k tokens)\n"))` returns `StatusActive`
- [ ] `sd.Detect([]byte("⠋ Thinking\r⠙ Thinking\r⠹ Thinking\n"))` returns `StatusActive`
- [ ] `sd.Detect([]byte("Normal line\r\nAnother line\r\n"))` does NOT return `StatusActive`
- [ ] All existing detection tests pass: `go test ./session/detection/...`
- [ ] `go build ./...` succeeds after `make generate-proto`
- [ ] Session with `SUB_STATUS_IDLE` shows "● Waiting…" chip in the session list
- [ ] Session with `SUB_STATUS_PROCESSING` renders row with left-border accent
- [ ] Row animation is absent when `prefers-reduced-motion: reduce` is set
- [ ] `GetDetectionEvents` RPC returns ≥5 events for an active session after 5 detection cycles
- [ ] `?debug=1` in URL shows "Detection Events" table in session detail view
- [ ] TailSnippet column is truncated to 80 chars in the table; full value visible on hover
