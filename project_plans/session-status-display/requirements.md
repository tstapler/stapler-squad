# Requirements: Session Status Display

## Problem Statement

The session list page shows no activity indicators (SubStatusChip) even when sessions are actively thinking or need approval. Root causes identified:

1. **`✦` (U+2726) missing from pattern**: Claude Code uses `✦` as its primary thinking spinner, but `claude_thinking_verb` only includes `[·✢✳✶✻✽●*]` — missing `✦` entirely. This means the most common "Thinking…" state is never detected as `StatusActive`.
2. **Idle state intentionally hidden but users want it**: `SubStatusChip` returns null for `SUB_STATUS_IDLE` and `SUB_STATUS_UNSPECIFIED`. The user wants "Waiting for input" to be visible.
3. **No row-level visual affordance**: Active/thinking sessions look identical to stopped ones at a glance. Users want a pulsing row highlight for at-a-glance scanning.
4. **Token/timing context lines not used**: Claude Code outputs `(↓ 2.8k tokens · 5s)` alongside thinking verbs — a strong activity signal that isn't used to confirm active state.

## User Priorities (confirmed via interview)

- **Must surface**: Thinking/Processing, Needs Approval, Idle/Waiting for input
- **Root cause confirmed**: "All status icons missing" — SubStatusChip not appearing on any sessions
- **Visual design**: Chip (precise state) + subtle row highlight (at-a-glance scan)

## Functional Requirements

### FR-1: Fix `✦` detection gap

The `claude_thinking_verb` active pattern in `session/detection/detector.go:getDefaultPatterns()` must include `✦` (U+2726) in its spinner character class. Post-fix, any terminal line matching `✦ <CapVerb>…` must return `StatusActive`.

**Acceptance**: A unit test with input `"✦ Thinking… (2m 5s · ↓ 6.4k tokens)\n"` must produce `StatusActive`.

### FR-2: Add Claude Code tool-action active patterns

Claude Code emits `⏺ Reading /path/to/file` and `✓ Read /path/to/file` lines during tool execution. The `⏺` (U+23FA) bullet is not in any current pattern. Add it to the `Active` set so tool-use phases are detected.

**Acceptance**: Input `"⏺ Bash(go test ./...)\n(esc to interrupt)\n"` must produce `StatusActive`.

### FR-3: Show Idle state in SubStatusChip

`SubStatusChip.tsx` currently returns `null` for `SUB_STATUS_IDLE`. Add a visual treatment for idle:
- Label: "Waiting…" 
- Icon: `●` (or equivalent muted indicator)
- Style: muted/secondary color (not attention-grabbing — it's the default resting state)

`SessionRow.tsx` condition must be updated to allow `SUB_STATUS_IDLE` through (remove the `subStatus !== SubStatus.IDLE` guard).

**Acceptance**: An ACTIVE session with detected idle state shows a muted "Waiting…" chip, not a blank.

### FR-4: Row highlight for active sessions

`SessionRow.css.ts` must add a `rowActive` style: a subtle left-border highlight or background pulse animation for sessions with `subStatus === SUB_STATUS_PROCESSING`.

Apply in `SessionRow.tsx` alongside the existing `rowPaused` / `rowMemoryPressure` class logic.

**Acceptance**: A session in PROCESSING sub-status renders with a visually distinct row compared to IDLE sessions (no animation for reduced-motion users).

### FR-5: Pattern test coverage

All new patterns in FR-1 and FR-2 must have Go unit tests in `session/detection/detector_test.go`. Each test must assert the correct `DetectedStatus` for a representative terminal output snippet.

### FR-6: Pattern observability — match logging

Add a `DetectionEvent` struct and an in-memory ring buffer (capped at 500 events per session) in the detection package. Every call to `Detect()` / `DetectWithContext()` appends one event containing:
- `SessionID` string
- `Timestamp` time.Time
- `MatchedPattern` string (pattern `Name` field, or `"<none>"` if `StatusUnknown`)
- `MatchedCategory` string (`"active"`, `"processing"`, `"idle"`, etc., or `"unknown"`)
- `ResultStatus` DetectedStatus
- `TailSnippet` string (last 512 bytes of cleaned terminal output, for no-match debugging)

The `StatusDetector` must expose a `RecentEvents(n int) []DetectionEvent` method. A new `GetDetectionEvents(sessionID string, limit int)` ConnectRPC endpoint on `SessionService` exposes events to the UI. Events are in-memory only — no persistence across restarts.

**Acceptance**: After 5 detection cycles on an active session, `GetDetectionEvents` returns ≥5 events. For a session producing `StatusUnknown`, events include a non-empty `TailSnippet`.

### FR-7: Screen-overwrite (CR/cursor-up) detection as active signal

The existing `collapseCarriageReturns()` function silently discards overwrite evidence. Instead, `Detect()` must check whether the raw input (before collapse) contains `\r` (carriage return) or ANSI cursor-up sequences (`\x1b[A`, `\x1b[<n>A`) indicating a spinner is actively redrawing a line. If overwrite activity is present AND no higher-priority status matched, treat the session as `StatusActive`.

This catches spinner-based UI patterns (any agent) that use CR to animate in-place without emitting text-based keywords — the most common cause of missed "thinking" detection.

Additionally, log overwrite detections as `DetectionEvent` entries with `MatchedPattern = "screen_overwrite"` and `MatchedCategory = "active"` so the observability system (FR-6) captures raw data for future heuristic development.

**Acceptance**: A unit test with input `"⠋ Thinking\r⠙ Thinking\r⠹ Thinking\n"` (pure CR spinner, no keywords) must produce `StatusActive`.

### FR-8: Detection event debug endpoint (admin UI hook)

Expose detection events per-session via a new proto RPC `GetDetectionEvents(session_id, limit)` returning a list of `DetectionEvent` messages. Wire a minimal debug panel in the session detail view (behind a `?debug=1` query param or a dev-only collapsed section) showing the last 20 events as a table: timestamp, matched pattern, category, result status. The `TailSnippet` column is truncated to 80 chars in the UI but available in full on hover.

**Acceptance**: With `?debug=1`, the session detail page shows a "Detection Events" section. The first visible event shows correct pattern name and status for the current session.

## Non-Functional Requirements

- Pattern regex changes must not increase false-positive rate for existing test cases
- Row animation must respect `prefers-reduced-motion` (use `@media (prefers-reduced-motion: reduce)` to disable pulse)
- Detection event ring buffer is capped: 500 events × ~1KB ≈ 500KB max per session; bounded by session count
- No persistence: events are in-memory only, lost on restart (acceptable for debugging)
- Proto changes for FR-8 require `make generate-proto` before building

## Out of Scope

- Improving detection for non-Claude agents (Aider, Gemini) — existing patterns sufficient
- Changing the SubStatus proto enum values or adding new SubStatus values
- Detection latency improvements (polling interval is acceptable)
- StatusBadge component (legacy — focus on SubStatusChip which is the wired component)
- Persisting detection events to disk or database

## Files Affected

| File | Change |
|---|---|
| `session/detection/detector.go` | Add `✦` to `claude_thinking_verb`; add screen-overwrite detection; expose `RecentEvents()` |
| `session/detection/events.go` | New: `DetectionEvent` struct + ring buffer |
| `session/detection/detector_test.go` | New tests for FR-1, FR-2, FR-7 |
| `proto/session/v1/session.proto` | New `GetDetectionEvents` RPC + `DetectionEvent` message |
| `server/services/session_service.go` | Implement `GetDetectionEvents` handler |
| `web-app/src/components/sessions/SubStatusChip.tsx` | Add IDLE render case |
| `web-app/src/components/sessions/SubStatusChip.css.ts` | Add idle chip style |
| `web-app/src/components/sessions/SessionRow.tsx` | Remove IDLE guard, add `rowActive` class |
| `web-app/src/components/sessions/SessionRow.css.ts` | Add `rowActive` style with pulse animation |
| `web-app/src/components/sessions/DetectionEventsPanel.tsx` | New: debug panel component |
