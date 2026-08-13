# BUG-013: xterm.js Viewport Jumps to Top During Claude Rendering [SEVERITY: High]

**Status**: ✅ Fixed
**Discovered**: 2026-04-09
**Fixed**: 2026-04-20
**Impact**: Terminal in the web UI constantly jumped to the top of the viewport during long Claude Code sessions.

## Problem Description

Claude Code's TUI sends `\x1b[2J\x1b[3J` (ED2 + ED3: clear screen + erase scrollback) during streaming repaints. The ED3 sequence (`\x1b[3J`) resets xterm.js's internal `viewportY` to 0, causing the visible viewport to snap to the top every time Claude redraws.

## Root Cause

Claude Code emits `\x1b[2J\x1b[3J` (ED2 followed immediately by ED3) during streaming repaints. xterm.js treats ED3 as "erase scrollback", which resets `viewportY` to 0. Confirmed upstream in anthropics/claude-code#36582.

## Fix

Added an ED3 filter in `web-app/src/lib/terminal/EscapeSequenceParser.ts` inside `processChunk()`. The filter strips ED3 only when immediately preceded by ED2 (the Claude repaint pattern), leaving standalone ED3 sequences (e.g. user-initiated `clear`) intact:

```ts
// Strip ED3 (erase scrollback) when paired with ED2 (clear screen).
// Claude Code emits \x1b[2J\x1b[3J on every TUI repaint; ED3 resets
// xterm.js viewportY to 0, causing scroll-to-top jank.
const filtered = fullData.replace(/\x1b\[2J\x1b\[3J/g, "\x1b[2J");
```

Also removed the `\x1b[2J` from the cold-start snapshot prefix (`connectrpc_websocket.go`) — the prefix is now `\x1b[H` (cursor home only), eliminating a redundant clear-screen flash on initial connect.

## Files Changed

- `web-app/src/lib/terminal/EscapeSequenceParser.ts` — ED3 filter in `processChunk()`
- `server/services/connectrpc_websocket.go` — `clearAndHome` changed from `"\x1b[2J\x1b[H"` to `"\x1b[H"`

## Tests Added

- `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.test.ts` — ED3 filter suite covering: paired ED2+ED3, standalone ED3, standalone ED2, multiple pairs, split-across-chunk boundary
- `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.bench.test.ts` — throughput benchmark (≥10 MB/s) confirming filter adds negligible overhead
