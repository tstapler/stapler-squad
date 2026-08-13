# Feature: AskUserQuestion Rich UI

## Overview

When Claude uses the `AskUserQuestion` tool, Stapler Squad currently fires a plain ❓ toast
notification with the question text and tells the user to check the terminal. The question's
numbered options are ignored. This feature improves that experience by surfacing the full
question — prompt + options — in a purpose-built UI panel that updates the session's status
in real-time.

The hook response still defers to Claude Code's native terminal dialog (empty body), so the
user answers in the terminal as normal. The UI is **informational only** — it shows what
Claude is asking so the user doesn't have to watch the raw terminal.

## Payload Structure

Claude Code sends the following `tool_input` fields on the `PreToolUse` hook for `AskUserQuestion`:

```json
{
  "tool_name": "AskUserQuestion",
  "tool_input": {
    "prompt": "Which database should I use?",
    "options": ["PostgreSQL", "SQLite", "Skip for now"]
  },
  "session_id": "...",
  "cwd": "/path/to/project"
}
```

- `prompt` — the question text (always present)
- `options` — array of string choices (present when Claude provides a menu; may be absent for
  free-text questions)

Both fields are available in `payload.ToolInput` inside `broadcastQuestionNotification`.

## Stories

### Story 1: Extract options from payload and include in notification event

**Status**: Ready  
**Files**: `server/services/approval_handler.go`, notification event schema

- Extract `tool_input["options"]` alongside `prompt` in `broadcastQuestionNotification`
- Add an `Options []string` field to the notification event metadata (passed via the `nil`
  metadata map currently sent to `events.NewNotificationEvent`)
- Keep the existing toast title ("Claude has a question") and message (the prompt text)
- No frontend changes in this story — just make the data available in the event

### Story 2: Session status reflects pending question

**Status**: Ready  
**Files**: `server/services/approval_handler.go`, session status model

- While a question notification is live, mark the session's status as `StatusInputRequired`
  (mirrors the detection-based status already used by `session/detection/detector.go`)
- Clear the status when the next hook fires for that session (any tool use resets it)
- This makes the session list show the ❓ badge automatically, consistent with the
  terminal-detection path

### Story 3: Question panel in session detail view

**Status**: Blocked on Story 1  
**Files**: `web-app/src/components/` (new component)

Render a `QuestionPanel` component in the session detail view when the session has a pending
`INPUT_REQUIRED` notification that came from the hook (not terminal detection):

```
┌─────────────────────────────────────────────────┐
│ ❓ Claude has a question                         │
│                                                 │
│  Which database should I use?                   │
│                                                 │
│  1. PostgreSQL                                  │
│  2. SQLite                                      │
│  3. Skip for now                                │
│                                                 │
│  ↩ Answer in terminal                           │
└─────────────────────────────────────────────────┘
```

- Options rendered as a numbered read-only list (not clickable — answer goes to terminal)
- "Answer in terminal" link focuses/attaches to the session's terminal pane
- Panel dismissed automatically when a new hook event arrives for the session
- Falls back gracefully when `options` is absent (free-text question): show prompt only

## Implementation Notes

### Why not intercept the answer in the UI?

Taking over the response would require the hook to block until the UI sends back a choice,
which adds round-trip latency and requires a new bidirectional channel. Claude Code's native
terminal dialog is instant and well-tested. The informational approach gives visibility
without risk.

### Distinguishing hook-sourced vs terminal-detected questions

`StatusInputRequired` can be set by both the terminal detector and this hook path. To avoid
conflating them in the UI, tag hook-sourced notifications with a `source: "hook"` metadata
field so the `QuestionPanel` only appears for hook events.

### Options field availability

`options` is included when Claude calls `AskUserQuestion` with explicit choices. For
free-text questions (no choices), only `prompt` is present. The UI must handle both cases.
