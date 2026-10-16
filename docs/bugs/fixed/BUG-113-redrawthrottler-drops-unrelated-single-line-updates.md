# BUG-113: RedrawThrottler silently drops an unrelated single-line update when two arrive within its 33ms coalescing window [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-09-21, dispatched investigation into "additional gaps in terminal rendering"
beyond `docs/bugs/open/BUG-101-terminal-resize-and-control-mode-teardown-fixed-by-repeated-narrow-patches.md`
(user report: rendering gets garbled/messed up while Claude Code streams output or while typing).

## Problem Description

`RedrawThrottler` (`web-app/src/lib/terminal/TerminalStreamManager.ts`) coalesces rapid full-screen
redraws to ~30fps to stop Claude's TUI from flickering. It classifies a chunk as a coalescable
"full redraw" via `/^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/` — cursor-up N immediately followed by an
erase sequence. `\x1b[2K` (erase-in-line) only erases the *current* line, though — it's the exact
same leading-byte shape an Ink-style multi-line TUI uses both for a genuine full-viewport repaint
(many `\x1b[2K` pairs bundled into one `write()`) *and* for a single targeted line update — a
spinner tick, a token counter — where N just picks out that one line. The throttler has no way to
tell those apart from the first 32 bytes, and treated any two "redraw-shaped" chunks arriving
within its window as fully interchangeable: `process()` unconditionally overwrote
`this.pendingRedraw` with the newer chunk and discarded the older one.

That discard is safe when both chunks are repeated repaints of the *same* region — the throttler's
intended case, and the case its whole existing test suite happens to exercise (every existing
`RedrawThrottler` test uses the same cursor-up count for both candidate chunks). It is not safe
when the two chunks target *different* lines: a token-counter update on line 4 arriving 10ms after
an unrelated spinner-tick update on line 2 silently erased the spinner update from the pipeline —
nothing ever resends it, so that line goes stale on screen until something else happens to redraw
it. Verified with a real `@xterm/xterm` `Terminal` (not the string-recording `MockTerminal`, which
can't reveal this): sending `\x1b[2A\x1b[2KSPINNER-TICK-1...` then, 10ms later,
`\x1b[4A\x1b[2KTOKENS: 42...`, only `TOKENS: 42` ever reached the terminal buffer.

## Root Cause

The `isFullRedraw` heuristic answers "does this chunk look like the start of a redraw?" but the
throttler needs a different question answered before it can safely coalesce two candidates: "are
these two redraws of the *same* screen region?" The regex match's own capture group (the cursor-up
count) already carries that information; it just wasn't being compared before one candidate
replaced another.

## Fix

`RedrawThrottler.process()` now captures the cursor-up count (`\d+` in `\x1b\[(\d+)A`) and tracks
it alongside `pendingRedraw` (`pendingRedrawUpCount`). A new candidate only overwrites the pending
one when its cursor-up count matches; a different count flushes the pending redraw immediately
(writing it through) before the new candidate starts its own throttle window. Same-region repeated
redraws — the original flicker case — still coalesce exactly as before, since every real repeated
redraw of one screen region reuses the same cursor-up count on every frame.

Coverage: `web-app/src/lib/terminal/__tests__/RedrawThrottler.region-safety.test.ts`, real
`@xterm/xterm` `Terminal`, no mocks — one test confirms two different-region updates within the
window both land on screen; a second pins the original coalescing behavior (same-region repeats
still collapse to the latest). All pre-existing `RedrawThrottler`/`TerminalStreamManager` tests
pass unmodified, since they all already used matching cursor-up counts between candidate chunks.

## Scope Decision

This closes one concrete content-loss gap in the client-side write pipeline. It does not touch
`BUG-101`'s resize-oscillation/control-mode-teardown chain — a separate mechanism (client resize
settling) that remains open under its own tracking. `BUG-086`'s control-mode refcounting race
(server-side tmux control-mode lifecycle) has since been fixed separately — see
`docs/bugs/fixed/BUG-086-tmux-control-mode-refcounting-race-under-concurrent-start-stop.md`. Other
hypotheses investigated and ruled out during this pass (see below) are not fixes because they were
not, in fact, bugs.

## Hypotheses investigated and ruled out (documented so they aren't re-investigated)

- **Large-write chunking bypassing `EscapeSequenceParser`**: `TerminalStreamManager.enqueueWrite`'s
  >16KB chunked-write path slices already-safe output with a raw `data.slice(i, i+CHUNK_SIZE)`,
  with no escape-sequence-boundary awareness. This looked like it could split a CSI sequence
  mid-stream across two separate `terminal.write()` calls. Verified empirically against a real
  `@xterm/xterm` `Terminal` (SGR color split mid-sequence, and split one byte before the
  terminator) — xterm.js's own parser is stateful across `write()` calls and reassembles the
  sequence correctly either way. Not a bug.
- **Multi-byte UTF-8 splitting across WebSocket/proto frames**: the live-output decode path
  (`useTerminalStream.ts`) uses a persistent `TextDecoder` ref with `{ stream: true }`, which
  correctly buffers a partial multi-byte character across separate `TerminalOutput` messages. The
  `scrollbackResponse` chunk path does the same per-response. Server-side, `control_mode.go`'s
  `decodeControlModeOutput` (tmux control-mode octal-escape decoding) preserves raw bytes exactly
  with no re-splitting. Not a bug.
- **`prependScrollbackBatch`'s cursor-corruption risk** (flagged speculatively in
  `docs/tasks/stapler-squad-painpoints.md` for a then-unbuilt "Phase 2"): the shipped
  implementation uses a serialize-clear-rewrite pattern (ADR-010) — serialize current buffer,
  clear, write history, write the serialized content back — which sidesteps the risk the pain-point
  doc described (raw history data's cursor-positioning sequences running over live content) rather
  than hitting it. Not a live bug.

## Files Affected

- `web-app/src/lib/terminal/TerminalStreamManager.ts`
- `web-app/src/lib/terminal/__tests__/RedrawThrottler.region-safety.test.ts` (new)

## Related

- `docs/bugs/open/BUG-101-terminal-resize-and-control-mode-teardown-fixed-by-repeated-narrow-patches.md`
- `docs/bugs/fixed/BUG-086-tmux-control-mode-refcounting-race-under-concurrent-start-stop.md`
- `docs/tasks/terminal-jank.md` (Story 1 introduced the `isFullRedraw` heuristic this bug refines)
