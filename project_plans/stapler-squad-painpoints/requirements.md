# Requirements: Stapler Squad Pain Points

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-16

## Problem Statement

Daily use of Stapler Squad surfaces a cluster of UX friction points that slow down the developer workflow. The primary user is the project author (Tyler), using Stapler Squad as a daily driver for managing multiple concurrent Claude Code sessions. The pain points span session creation ergonomics, terminal quality on mobile, session metadata management, inconsistent data presentation across views, perceived slowness on interaction (with no instrumentation to diagnose it), and a fundamental architectural issue: large sessions transmit the full scrollback buffer to the browser even when the user is only viewing the last screen of output.

## Success Criteria

- New session dialog: branch autocomplete works when a repo is selected; all fields are keyboard-navigable (arrows, Tab, Enter to submit); dialog can be completed without touching the mouse.
- Worktree state is visible: each session card shows an inline badge (unmerged commits, dirty status); selecting a session shows full commit list / diff vs main in the detail panel.
- Sessions can be renamed/retagged without multiple navigations.
- Mobile terminal: no layout jumping; uses full available horizontal space on wide screens; an optional custom keyboard overlay with arrow keys + Enter is available.
- Every view that displays session data uses the same data fields (no view missing info that another shows).
- Frontend interaction latency is instrumented: slow clicks/loads are observable via OpenTelemetry or equivalent; analytics exist to show which interactions users perform most.
- Large sessions load fast: only the visible tail of scrollback is sent on initial attach; older history is fetched lazily as the user scrolls up.
- Mobile terminal scrolling is comfortable: touch scroll works smoothly, doesn't fight the page, and doesn't accidentally trigger other gestures.
- Each fix ships as a standalone PR; nothing waits on the full bundle.

## Scope

### Must Have (MoSCoW)
- Branch autocomplete in new session dialog (populated from selected repo's remote/local branches)
- Full keyboard navigation in new session dialog (Tab through fields, arrow keys in dropdowns/autocomplete, Enter to submit)
- Worktree state badge on session card: unmerged commit count, dirty-files indicator
- Worktree detail in session panel: commit list ahead of main, diff summary
- Quick rename / retag UX for sessions (inline edit or single-click modal)

### Should Have
- Bulk session actions (pause, delete, tag multiple sessions at once)
- Mobile terminal layout fixes (no jump, fills available width on landscape)
- Mobile custom keyboard overlay (arrow keys, Enter, Escape)
- Terminal auto-focus on session attach

### Must Have
- Frontend + interaction observability: instrument click-to-render latency, RPC duration, and navigation events so slow interactions are diagnosable
- User interaction analytics: track which features/views are used most (click events, page dwell time) to guide future iteration
- Lazy/paginated scrollback: send only the last N lines on initial terminal attach; fetch older lines on demand as the user scrolls up (virtual scrollback)
- Mobile touch scrolling: smooth, non-conflicting touch scroll inside the terminal; doesn't compete with page scroll or trigger browser chrome gestures

### Could Have
- Unified data presentation layer / component system so all views show consistent fields
- Session creation speed improvements (reduce round-trips)

### Out of Scope
- New session types beyond what already exists
- Desktop app / Electron packaging
- Multi-user / shared session support

## Constraints

- **Tech stack**: Go backend, React + TypeScript + vanilla-extract frontend, ConnectRPC, tmux, git worktrees
- **Timeline**: Incremental — each fix ships as its own PR as soon as it's ready
- **Dependencies**: Branch list requires a backend API endpoint to enumerate git refs for a given repo path; worktree state requires git commands (ahead/behind, `git status --short`)
- **CSS**: New components must use vanilla-extract (`.css.ts`); existing `.module.css` edits must use defined tokens from `globals.css`
- **Observability**: Go backend already has OpenTelemetry wired (`OTEL_ENABLED=true`); frontend has no equivalent yet — need to evaluate OpenTelemetry JS SDK, Sentry, or a lightweight custom solution

## Context

### Existing Work
- New session dialog exists but has no branch autocomplete or keyboard shortcut handling
- Worktrees are created per session but their state (unmerged commits, dirty files) is not surfaced in the UI
- Mobile UX improvements project (`project_plans/mobile-ux-improvements/`) already exists — review ADRs 001–004 before duplicating work
- Tag management modal already exists for adding/removing tags; rename/retag pain is about discoverability / speed of access
- Session cards and detail panel exist; data completeness across views is inconsistent
- Go backend already ships OpenTelemetry instrumentation (HTTP, ConnectRPC, cache, search) — see CLAUDE.md; no frontend telemetry exists yet
- Perceived slowness on click is reported by the user but has no instrumentation to identify root cause (RPC latency? React render? hydration?)
- Current scrollback implementation uses a circular buffer (`session/scrollback/`) that is sent in full on attach; large sessions (long-running Claude tasks) can produce MBs of terminal data that block the initial load
- xterm.js (the terminal renderer) supports virtual scrollback via `IBufferLine` API and `loadAddon` — the architecture for lazy loading exists but is not implemented

### Stakeholders
- Tyler (primary user and author) — daily driver, wants frictionless keyboard-driven workflow and mobile usability

## Research Dimensions Needed

- [ ] Stack — backend APIs for branch listing + worktree status; frontend autocomplete options; OTel JS SDK vs Sentry vs custom; xterm.js virtual scrollback / lazy buffer APIs
- [ ] Features — patterns from comparable tools for: autocomplete dialogs, worktree status, bulk actions, interaction analytics, virtual/paginated terminal scrollback (iTerm2, VS Code terminal, ttyd)
- [ ] Architecture — branch autocomplete wiring; worktree status polling; frontend telemetry pipeline; lazy scrollback (cursor-based paging API, xterm.js integration, scroll-up trigger); mobile touch scroll isolation
- [ ] Pitfalls — mobile keyboard resize, git latency on large repos, OTel JS bundle size, xterm.js virtual scrollback ANSI rehydration edge cases, touch event conflicts between xterm and browser scroll
