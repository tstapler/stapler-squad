# Requirements: terminal-redraw-corruption

**Date**: 2026-08-06
**Type**: bug fix

## Problem Statement

The xterm.js-rendered web terminal (used to view live AI agent sessions, e.g. Claude Code
running in a tmux pane) displays garbled/overlapping status lines. Screenshot evidence
(session "stapler-squad-perf", captured 2026-08-06) shows stray leading character
fragments from a PREVIOUS, longer status line surviving underneath a NEW, shorter line
drawn on the same terminal row:

```
r Running... Bash(grep -n "github\.com\.|IsGitHubURL\.|ParseGithubURL\.|url_parser\.|urlparser\.|ParseURL"
ss Running... Bash(sed -n '1290,1430p')
x) Running...
o) Read(web-app/src/components/sessions/Omnibar.tsx)
```

Claude Code's TUI redraws its spinner status line (e.g. "✻ Running... (esc to interrupt)")
on every tool call, and the line's length varies. The leading fragments ("r ", "ss ", "x) ",
"o) ") match exactly what you'd see if a terminal row is repositioned-to and overwritten with
shorter text without an erase-in-line (`\x1b[K`) first — old glyphs at the tail of the row
are never cleared.

This is a **different bug class** from previously-fixed issues in this codebase: the escape
codes here are being rendered/interpreted, not stripped or leaked as literal text. Old
content is surviving a redraw that should have cleared it.

## Users / Consumers

Any user viewing a live agent session's terminal pane in the stapler-squad web UI
(`web-app`, ConnectRPC-streamed terminal via `XtermTerminal.tsx`). Affects the primary
"watch my agent work" use case — corrupted status lines make it hard to read what a
session is currently doing.

## Success Metrics

- Bug is gone: rapid, varying-length spinner redraws (as produced by Claude Code's TUI)
  render cleanly in the web terminal, with no stale tail characters from a prior, longer
  line surviving under a new, shorter one.
- A regression test (or reproducible manual verification procedure, if an automated repro
  proves impractical) exists that would have caught this.
- Root cause is identified and stated explicitly (per this repo's "no fix without root
  cause" discipline) before any fix is proposed.

## Constraints

- Research-first: confirm a root-cause hypothesis (ideally via reproduction) before
  designing a fix.
- This SDD run covers phases 1-4 only (ideate, research, plan, validate) — no
  implementation in this run. The user wants to review the plan before a fix proceeds.
- Must not touch `server/mcp/ansi.go`'s `stripANSI` — that's the MCP tool-output path,
  unrelated to terminal rendering.
- Must not resume or re-validate the stale `project_plans/new-renderer/` plan — its
  diagnosis (a MOSH-style state-sync protocol) targeted code deleted in commit
  `0ac0ca1dad9f102d7072857c87714fb2d1905e05` (PR #163) because it had zero live callers.
  This project supersedes the *relevant, still-live* parts of that investigation with a
  correct diagnosis, but does not delete or rewrite `new-renderer`'s artifacts — only
  references/links them and marks the superseded parts obsolete.
- Must reconcile with, not duplicate, prior related work: `docs/tasks/terminal-jank.md`
  (Stories 1-2 shipped, Story 3 pending), commits `0ac0ca1da` (PR #163, CSI terminator
  fix), `9651edd5e` (PR #272, resize-loop/RPC dedup/WebGL fallback), `96990ce12`
  (visibility/focus resync), and sibling in-flight plans `terminal-robustness/`,
  `terminal-visibility-resync/`, `terminal-resize-fit-loop/`.

## Scope

### In Scope
- Diagnosing why stale tail characters from a longer previous line survive when a
  shorter line is redrawn over the same terminal row in the web-rendered xterm.js
  terminal.
- Investigating the full output pipeline: tmux capture/streaming
  (`server/services/connectrpc_websocket.go` quiescence/snapshot logic), any
  chunk-buffering/throttling between the server and the browser, the frontend
  `EscapeSequenceParser.ts` (ED3 filtering and any other regex-based transforms), and
  xterm.js's own handling of the escape sequences Claude Code's TUI actually emits.
- Confirming (via reproduction if feasible) whether the missing erase-in-line hypothesis
  is the true root cause, or identifying the actual one if it differs.
- Producing a validated implementation plan and test plan for the fix. Not implementing
  the fix itself.

### Out of Scope
- Implementing the fix (deferred to a future Phase 5 run in a fresh session).
- `server/mcp/ansi.go`'s `stripANSI` (MCP tool-output path).
- Re-litigating or resurrecting `project_plans/new-renderer/`'s MOSH state-sync design.
- Terminal instance pooling (`terminal-jank.md` Story 3) unless research finds it's
  directly implicated in this symptom.
- Unrelated terminal issues already tracked elsewhere (resize loop, visibility/focus
  resync, WebGL fallback) unless research finds they interact with this bug.

## Open Questions (for Research phase)

- Does Claude Code's TUI actually emit `\x1b[K` (EL) or a full CUP+EL sequence when
  redrawing its status line, and is that sequence present, intact, and correctly
  interpreted by the time it reaches xterm.js's `Terminal.write()`?
- Is there any chunk-boundary buffering/throttling in the streaming pipeline (server
  quiescence capture, or a frontend redraw throttler) that could coalesce or reorder two
  redraw frames such that a shorter frame's erase is dropped or applied out of order?
- Could the ED3 (`\x1b[3J`) filtering regex in `EscapeSequenceParser.ts`, or any other
  regex-based escape transform, be incidentally matching/stripping an EL (`\x1b[K`)
  sequence?
- Could a stale cached tmux snapshot (Story 2 quiescence-capture logic) be concatenated
  with fresh output instead of replacing it, e.g. on session switch, tab visibility
  resync, or reconnect?
- Is this reproducible via a manual second instance (per CLAUDE.md's "Manual/interactive
  testing without touching the live deployed instance" guidance) watching a real Claude
  Code session's spinner redraw?
