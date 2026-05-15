# Logs UX — Datadog/Splunk-Like Experience

## Problem Statement

The current log viewing experience in stapler-squad is poor on both desktop and mobile (iOS/Android phone browsers):
- Too few log lines are visible at once, especially on narrow screens
- Long lines wrap instead of scrolling horizontally, creating visual noise
- No search or keyword filtering capability
- No visual differentiation between log levels (ERROR vs WARN vs INFO vs DEBUG)

Users cannot effectively monitor, debug, or triage sessions through the current log UI.

## Scope

**Both** views need improvement:
- **Session logs**: Output logs shown when viewing a specific session (terminal/scrollback)
- **Application logs**: stapler-squad system logs from `~/.stapler-squad/logs/`

## User Goals

1. Quickly scan recent log output without excessive scrolling
2. Find a specific error or keyword in a large log stream
3. Understand severity at a glance (visual log levels)
4. Drill into a specific log line for structured detail
5. Use this efficiently on an iPhone/Android browser with touch navigation

## Functional Requirements

### FR-1: Live Tail with Scroll-Back
- Logs stream in real-time (live tail), auto-scrolling to the newest line
- When user scrolls up, auto-scroll pauses (like Datadog's "live tail paused" indicator)
- A "Jump to Latest" button/indicator appears when the user is not at the bottom
- Resuming scroll-to-bottom re-activates live tail

### FR-2: Horizontal Scroll, No Line Wrapping
- Log lines MUST NOT wrap; they scroll horizontally
- The line number gutter and log level badge must be sticky (don't scroll horizontally)
- The timestamp and message body scroll together horizontally

### FR-3: Search & Highlight
- A search bar (keyboard shortcut: `/` or `Cmd+F`) filters visible log lines in real-time
- Matching terms are highlighted with a distinct background color inline
- Shows match count (e.g., "12 / 47 matches")
- Clear button resets search; ESC also clears

### FR-4: Log Level Coloring
- Detect log level from common patterns: `ERROR`, `ERR`, `WARN`, `WARNING`, `INFO`, `DEBUG`, `TRACE`
- Color scheme (accessible, high contrast):
  - ERROR / ERR → red (`--error` token or equivalent)
  - WARN / WARNING → yellow/amber
  - INFO → default / dim white
  - DEBUG / TRACE → gray / muted
- The entire row gets a subtle background tint; the level badge uses full color

### FR-5: Expandable Row Detail
- Clicking / tapping a log row expands it to show full structured detail
- If the log line is JSON, pretty-print the parsed JSON
- If plain text, show the raw full line with copy button
- Expanded rows collapse when clicked again (accordion behavior)
- On mobile: expand on tap; on desktop: expand on click

### FR-6: Mobile-First Responsive Layout
- Touch targets ≥ 44px (Apple HIG standard)
- Search bar collapses to an icon on very narrow screens, expands on tap
- Log level filter chips (ALL / ERROR / WARN / INFO / DEBUG) reachable by thumb
- Horizontal swipe on a log line reveals quick actions (copy, share)
- Pinch-to-zoom on expanded row detail

### FR-7: Log Level Filter
- Filter chips above the log list: `ALL`, `ERROR`, `WARN`, `INFO`, `DEBUG`
- Active filter persists across live tail updates
- Multi-select allowed (e.g., show ERROR + WARN only)

## Non-Functional Requirements

### NFR-1: Performance
- Virtual scrolling for log lists with > 1000 lines (don't render off-screen rows)
- Search filter must run in < 100ms for up to 10,000 lines
- Live tail must not degrade UI responsiveness (use requestAnimationFrame or batched updates)

### NFR-2: Accessibility
- WCAG AA color contrast on all level badges and row tints
- Keyboard navigable (arrow keys to move between rows, Enter to expand)
- Screen reader announcements for new log lines in live tail (with throttle)

### NFR-3: No New Backend Endpoints Required (preferred)
- Use existing ConnectRPC streaming/polling for session logs
- Use existing log file access for application logs
- New backend endpoints acceptable if required for significant UX improvement

## Out of Scope

- Log aggregation / centralization across multiple instances
- Log alerting / notifications based on patterns
- Log retention policy management
- Export to external systems (Datadog, Splunk, etc.)

## Success Criteria

1. A user on an iPhone can see ≥ 20 log lines at once without horizontal wrapping
2. Searching for "error" highlights all matching lines within 100ms
3. ERROR-level lines are visually distinct from INFO lines at a glance
4. Clicking a log line reveals its full content / structured data
5. Live tail pauses on scroll-up and resumes on scroll-to-bottom tap
6. All existing Playwright e2e tests continue to pass

## Date

2026-05-14
