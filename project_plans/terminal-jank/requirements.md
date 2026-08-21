# Requirements: Terminal Jank Elimination

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-09

## Problem Statement

When switching between sessions in Stapler Squad's web UI, the terminal experience is noticeably degraded compared to opening Claude Code directly in a local terminal (iTerm2/Terminal.app). Symptoms:

- **Session switching lag**: No output visible immediately on switch; a loading state is shown while scrollback is fetched and replayed
- **Scrollback loading jank**: When returning to a session, the scrollback loads and scrolls in a way Claude Code locally never does
- **Cursor/input corruption**: Claude's TUI output sometimes corrupts the xterm.js cursor position and input rendering
- **Rendering artifacts**: Stale content, wrong scroll position, or garbled output visible during/after switch

Root cause (preliminary): Every session switch destroys and recreates the xterm.js terminal instance. The new instance must:
1. Wait for container size to stabilize (double RAF + 100ms timeout)
2. Establish a new WebSocket connection
3. Fetch scrollback via `currentPaneRequest`
4. Replay history into a blank terminal
5. Scroll to bottom

This is fundamentally different from a local terminal where the PTY state is persistent and always available.

## Success Criteria

1. **Zero loading overlay on session switch** — switching to any session shows a live terminal immediately, no spinner, no blank state
2. **Persistent scroll position** — returning to a session shows it exactly as left (cursor position, scroll offset, visible content)
3. **No input/cursor corruption** — typing and cursor rendering work correctly even after Claude's full-screen TUI redraws
4. **Local terminal parity** — the subjective feel of switching sessions matches switching tmux windows in a local terminal
5. **No regression on initial session load** — first-time session open can still show a brief loading state

## Scope

### Must Have (MoSCoW)
- Eliminate the loading overlay and scrollback replay on session switch (terminal pool)
- Fix cursor/input corruption and scroll-to-top from Claude's screen redraws (ED3 filter + xterm.js 6.0.0)
- Persist terminal visual state (scroll position, content) across switches (keep-alive pool)
- Immediate display of terminal content when switching to a previously-viewed session
- On reconnect, request only the **visible screen** from tmux — not full scrollback replay
- Replace 250ms hard-coded Go sleep with output quiescence detector
- Per-session snapshot cache (invalidate on control-mode `%output`) to eliminate repeated capture-pane cost
- `scrollback: 5000` on pooled xterm.js instances (currently 0 — users have no scrollback at all)

### Should Have
- Infinite-scroll / lazy-load pattern for historical scrollback above current viewport
- Line-granular scrollback API (`GetScrollbackByLines`) enabling reliable page offsets
- Wire up existing `requestScrollback` / `scrollbackResponse` proto (currently ignored client-side)

### Out of Scope
- TUI keyboard input improvements (arrow keys, etc.) — separate concern
- Mobile/touch support
- Terminal search UI improvements

## Constraints

Tech stack: Go backend + React frontend + xterm.js + ConnectRPC/WebSocket streaming
No hard constraints — willing to change libraries, architecture, or approach if needed
Must not break existing session management (tmux, worktrees, scrollback storage)

## Context

### Current Architecture
```
Session switch
  → TerminalOutput unmounts (destroys xterm.js Terminal instance)
  → TerminalOutput mounts for new session
  → XtermTerminal initializes new Terminal, runs double-RAF + 100ms fit wait
  → useTerminalStream connects WebSocket
  → Server sends currentPaneResponse (scrollback replay)
  → TerminalStreamManager.writeInitialContent() clears + replays scrollback
  → setIsLoadingInitialContent(false) hides loading overlay
```

Key files:
- `web-app/src/components/sessions/TerminalOutput.tsx` — orchestration, loading state, session binding
- `web-app/src/components/sessions/XtermTerminal.tsx` — xterm.js wrapper with size stabilization
- `web-app/src/lib/hooks/useTerminalStream.ts` — WebSocket/ConnectRPC streaming hook
- `web-app/src/lib/terminal/TerminalStreamManager.ts` — write buffering, flow control, redraw throttling
- `session/scrollback/manager.go` — scrollback storage (circular buffer + compressed disk storage)

### Existing Work
No prior investigation. First pass.

### Stakeholders
All Stapler Squad users (OSS project). Terminal UX is the primary daily interaction surface.

## Research Dimensions

- [x] Stack — `research/stack.md` — keep-alive + `visibility:hidden`, VS Code detach/attach pattern, WebGL pool cap 6–8
- [x] Features — `research/features.md` — tmux sends visible viewport only (~2-4KB), `@xterm/addon-serialize {scrollback:0}`, WaveTerm prior art
- [x] Architecture — `research/architecture.md` — terminal pool design, quiescence detector, hybrid warm/cold strategy, 500–1200ms bottleneck breakdown
- [x] Pitfalls — `research/pitfalls.md` — `display:none` breaks FitAddon, WebGL context loss handler needed, ED2+ED3 scroll-to-top, `CLAUDE_CODE_NO_FLICKER` unreliable/buggy
- [x] Scrollback & cold-start API — `research/scrollback-and-cold-start.md` — entry vs line granularity, quiescence detector, snapshot cache, ED3 filter, lazy load design, P0–P3 API change priority table
