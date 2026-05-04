# Requirements: Review Queue Working-State Detection

## Problem Statement

Sessions in the review queue that are actively working (Claude is generating output, running tools, or thinking) are misclassified as "idle" or "stale" and surface in the queue as if they need human attention. This causes the user to waste time cycling through sessions that are just busy — connecting to them adds noise and interrupts work in progress.

The root cause is a systemic gap in how the app parses terminal output and classifies session state. The current detection does not reliably distinguish "actively running" from "waiting for user input."

## Screenshots / Context

From the screenshots provided (2026-05-02):
- Review queue shows sessions labeled "Session idle - ready for next task" and "No activity for 3m 29s - session may be stuck or waiting"
- One of those sessions was actually fully active: terminal showed Claude mid-turn with "Ebbing... (7m 5s... 12.9k tokens... almost done thinking)" and tool call output streaming

## Goals

1. **Filter running sessions from the review queue** — sessions that are detectably active should be excluded from the queue until they finish
2. **Reliable working-state detection** — build detection that uses multiple signals and degrades gracefully when individual signals are absent or change
3. **Structured state-change events** — the backend should emit discrete events (session entered idle, session started working) that the review queue subscribes to, rather than polling-based heuristics inline in the queue
4. **Golden-state capture infrastructure** — tooling to label and save terminal scrollback snapshots as positive/negative examples, forming a corpus to validate and tune detection heuristics over time

## Non-Goals

- Perfect detection (signals change as Claude Code evolves; good-enough + tunable is acceptable)
- Detecting state in non-Claude programs (initial focus is Claude Code; architecture should be extensible)
- Real-time millisecond accuracy (second-level granularity is fine)

## Functional Requirements

### FR-1: Working-State Detection

The system must classify a session as **working** when any of the following are observed in recent terminal output:

| Signal | Pattern | Notes |
|--------|---------|-------|
| Recent output | Any new terminal lines within the last N seconds (configurable, default 15s) | Primary signal; most reliable |
| Spinner/progress lines | Lines containing "Thinking...", "Ebbing...", "▲", animated spinner characters | Indicates Claude mid-turn |
| Tool call output | Lines matching tool invocation patterns (e.g. "Bash(", "Read(", "Write(") | Claude is executing tools |
| Interrupt hint present | "esc to interrupt" text visible in scrollback tail | Claude is running and interruptible |

The system must classify a session as **idle / ready for input** when:

| Signal | Pattern |
|--------|---------|
| Prompt appeared | Claude Code's `> ` input prompt is visible at the bottom of scrollback |
| Cost summary line | A line matching `\$\d+\.\d+ •` (turn-complete summary) appeared |
| Silence threshold | No new output for N seconds AND none of the working signals are present |

### FR-2: Review Queue Filtering

- Sessions in **working** state must be excluded from the review queue
- Sessions transition back into the queue when their state changes to **idle**
- The queue must not require a page refresh to pick up state changes — state transitions should push updates to the UI in real time

### FR-3: Structured State-Change Events

The backend must emit typed events for session state transitions:
- `session.state.working` — session entered working state
- `session.state.idle` — session entered idle state (ready for input)
- `session.state.stuck` — session has been silent longer than a "stuck" threshold (configurable, default 5 min) without showing an idle prompt

These events must be:
- Subscribable by the review queue via the existing ConnectRPC streaming mechanism
- Persisted (at minimum, last known state) so the queue can reconstruct state on reconnect

### FR-4: Golden-State Capture Infrastructure

- Provide a UI affordance (button or keyboard shortcut) in the session terminal view to **mark the current scrollback snapshot** with a state label (working / idle / stuck)
- Store labeled snapshots in `~/.stapler-squad/state-corpus/` as JSON (timestamp, session ID, label, scrollback tail)
- Provide a CLI or make target to run the current heuristics against the corpus and report true-positive / false-positive rates
- This is an internal developer tool, not a user-facing feature

## UX Requirements

### UXR-1: Queue Badge / Status

- The review queue header should show counts by state: e.g. "3 waiting · 5 working · 2 stuck"
- "Working" sessions may optionally appear in a collapsed/dimmed section so the user can see them if needed, but they are not the primary focus

### UXR-2: Manual Override

- The user can manually force a session into the queue (e.g., "mark as waiting") regardless of detected state — useful when detection fails

### UXR-3: Transition Notification

- When a session that was working transitions to idle, trigger a notification (existing push notification / alert mechanism) so the user knows it's ready

## Technical Constraints

- Detection runs in the Go backend (session watcher goroutine), not in the React frontend
- Signals are parsed from tmux scrollback — the same source currently used for session status text
- The event system must integrate with the existing ConnectRPC streaming; no new transport layer
- Detection parameters (silence threshold, stuck threshold, pattern list) must be configurable without recompilation — either via `config.json` or a runtime flag

## Acceptance Criteria

1. A session actively running Claude (visible "Ebbing", tool calls, or output within 15s) does NOT appear as "idle" in the review queue
2. When that session finishes its turn (prompt appears or cost summary visible), it re-enters the queue within 5 seconds
3. The review queue updates in real time without a page refresh
4. The state corpus tool can record a labeled snapshot from the terminal view
5. Running the corpus validator against ≥10 labeled examples outputs a report
6. Existing review queue behavior for genuinely idle and stuck sessions is unchanged
