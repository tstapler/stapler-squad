# Stack Research: terminal-redraw-corruption

Date: 2026-08-06. Research only — no implementation.

## 1. Installed xterm.js version

`/Users/tstapler/code/github.com/tstapler/stapler-squad/web-app/package.json:74`:

```
"@xterm/xterm": "^6.0.0",
```

Addons (lines 68-73): `@xterm/addon-canvas` `^0.7.0`, `@xterm/addon-fit` `^0.11.0`,
`@xterm/addon-search` `^0.16.0`, `@xterm/addon-serialize` `^0.14.0`,
`@xterm/addon-web-links` `^0.12.0`, `@xterm/addon-webgl` `^0.19.0`.

**Confirmed**: the sibling doc `project_plans/terminal-resize-fit-loop/research/stack.md`'s
`^5.5.0` note is stale — the project has since upgraded to xterm.js 6.0.0 (this matches
`docs/tasks/terminal-jank.md` Story 1's "xterm 6.0 Upgrade" task, completed 2026-04-20).

## 2. EscapeSequenceParser.ts — transforms applied before writing to xterm.js

Full file read: `web-app/src/lib/terminal/EscapeSequenceParser.ts` (233 lines).

**Only one transform exists today, and it does nothing to the byte stream**:

```ts
// Line 39
const filtered = fullData; // No sequence stripping - xterm.js v6 handles ED2+ED3 correctly
```

The rest of the class (`findPartialEscapeAtEnd`, `isCompleteEscapeSequence`,
`hasCSITerminator`, `hasOSCTerminator`) only detects and **buffers** an incomplete escape
sequence at the end of a chunk so it isn't split across two `.write()` calls — it never
rewrites, strips, or drops bytes from a *complete* sequence.

### The ED3 filter described in terminal-jank.md Story 1 is gone from the current code

`docs/tasks/terminal-jank.md:57-68` (Task 1.1, marked ✅ COMPLETE 2026-04-20) documents adding:

```ts
const filtered = fullData.replace(/\x1b\[2J\x1b\[3J/g, "\x1b[2J");
```

That regex is **not present** in the current `EscapeSequenceParser.ts` — line 39 shows it
was later replaced with a no-op plus the comment "xterm.js v6 handles ED2+ED3 correctly"
(i.e. the filter was intentionally removed once the xterm.js 6.0 upgrade made it
unnecessary, presumably because xterm.js 6's own ED3 handling stopped resetting
`viewportY` in a jank-causing way). `git log -p --follow` on the file shows no matching
commit touching this specific regex string in the file's history as currently tracked,
so the removal predates or was folded into a squash — not independently confirmed via
blame, but the current source is unambiguous: no ED2+ED3 stripping regex exists today.

**Relevance to this bug**: since no regex here matches `\x1b[2J`/`\x1b[3J` combos, and no
regex anywhere in this file references `[KJ]` erase-in-line codes at all, **EL sequences
(`\x1b[K`, `\x1b[0K`, `\x1b[1K`, `\x1b[2K`) are not touched, stripped, or corrupted by
EscapeSequenceParser**. The only way this file could corrupt an EL sequence is via its
partial-sequence buffering logic if `hasCSITerminator` misclassified a complete `\x1b[K`
as incomplete — but `hasCSITerminator` (lines 170-196) correctly recognizes `K` (0x4B) as
a valid CSI terminator letter (falls in the `0x41-0x5A` range checked at line 178), so a
complete `\x1b[K` is never buffered/withheld. **Conclusion for item 2: this file passes
EL sequences through intact.**

## 3. Where raw bytes are written to xterm.js — buffering/throttling pipeline

`web-app/src/components/sessions/XtermTerminal.tsx:1194-1196` exposes a thin imperative
`write` that calls `terminalRef.current?.write(data)` directly — but the *actual* pipeline
used for streaming output is `web-app/src/lib/terminal/TerminalStreamManager.ts` (581
lines, read in full), which sits in front of the raw xterm.js `Terminal.write()` calls
(`TerminalStreamManager.ts:373,429,461,536`).

The manager applies **two distinct coalescing/throttling layers** before data reaches
`EscapeSequenceParser` and then xterm.js:

### 3a. `RedrawThrottler` (lines 42-92) — drops, not just delays, whole chunks

```ts
// lines 52-76
process(chunk: string): string | null {
  const isFullRedraw = /^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/.test(chunk.substring(0, 32));
  if (!isFullRedraw) {
    this.flushPending();
    return chunk;
  }
  this.pendingRedraw = chunk;   // <-- overwrites any previously pending chunk
  if (!this.throttleTimer) {
    this.throttleTimer = setTimeout(() => this.flushPending(), this.throttleMs); // 33ms
  }
  return null; // Don't output yet
}
```

If a chunk matches the "full redraw" pattern (cursor-up N rows immediately followed by
`\x1b[2K` or `\x1b[J`) and another matching chunk arrives within the same 33ms window,
**the earlier chunk is discarded outright** — `this.pendingRedraw = chunk` at line 67
overwrites the previous value with no flush in between. This is a genuine "byte sequence
dropped entirely" mechanism (answers part of item 5's question, though this code lives in
the frontend, not `connectrpc_websocket.go`). This is the most concrete corruption
mechanism found in the codebase: if Claude's TUI redraw is relative-cursor-addressed
(assumes a known prior cursor position from the previous redraw), and one redraw in a
rapid burst is silently dropped, every subsequent relative cursor-up in later redraws
lands on the wrong row — producing exactly the "new shorter text overwrites only part of
old text" symptom in the bug report. **Caveat**: the regex requires the chunk to *start*
with `\x1b[<N>A` — a single spinner-line status update that repositions via `\r` or
`\x1b[<row>;<col>H` rather than a cursor-up would not match this path and would be
written through immediately via `flushPending()`/direct return, bypassing this mechanism.
Whether Claude Code's actual spinner-line emission matches this pattern was not verified
against captured bytes in this research pass — flagged as a gap.

### 3b. Chunking/flow-control (`writeDirectWithFlowControl`, `enqueueWrite`, lines 361-495)

Splits any single output blob larger than `CHUNK_SIZE` (16384 bytes, line 33) into
16KB pieces and writes them via sequential `terminal.write(chunk, callback)` calls,
yielding to the event loop (`requestAnimationFrame`) between chunks. Because
`EscapeSequenceParser.processChunk` already runs *before* this chunking (called once at
`TerminalStreamManager.write():245` / `handleProcessedOutput`), by the time chunking
happens the data has already had its trailing partial-escape byte withheld — so
`handleProcessedOutput`'s 16KB re-chunking should never itself re-split an escape
sequence, since it operates on the whole safe string in one call and only decides where
to slice for UI-yielding purposes. However, note it re-slices at fixed byte offsets
(`data.slice(i, i+CHUNK_SIZE)`, line 449) with **no escape-sequence awareness at the
slice boundary itself** — if a >16KB chunk contains an EL sequence straddling a
16384-byte offset, it would be split across two separate `terminal.write()` calls. This
is a secondary, lower-probability mechanism (requires a single output blob >16KB with an
EL exactly at the boundary) compared to 3a's outright drop.

### 3c. `writeStateBatched` (lines 520-528) — RAF-batched but non-lossy

Used only in "state"/"hybrid" streaming modes; concatenates into `writeBuffer` and
flushes once per animation frame via `flushWriteBuffer()`. This path does not drop data —
it only delays and merges it — so it's not a corruption mechanism, only a latency one.

## 4. xterm.js documented behavior on partial writes / EL handling (WebSearch)

- xterm.js's own parser is designed to survive a `write()` call ending mid-escape-sequence
  — the VT parser is a state machine that persists across `write()` invocations, so a
  split sequence is not itself a documented xterm.js bug *provided every byte is
  eventually delivered in order*. The risk class documented in the community is the
  inverse: **dropping** a byte (or writing bytes out of order) is what corrupts state,
  not merely splitting a complete call into two writes.
- [xtermjs/xterm.js#943 "clear() is not correctly clearing the buffers"](https://github.com/xtermjs/xterm.js/issues/943)
  describes a race where an `exit` event can fire before a PTY's final `data` buffer is
  flushed, so "the next call to write will prefix the first line with the unfinished
  final line of the previous write" — i.e., a dropped/reordered write producing exactly
  the stray-leading-fragment symptom class in this bug report, though in a different
  triggering context (process exit racing output, not redraw throttling).
- [xtermjs/xterm.js#2979 "handling of EL when cursor is in deferred EOL position"](https://github.com/xtermjs/xterm.js/issues/2979)
  is the most on-point open question for EL specifically: it asks whether `\x1b[K`
  behaves correctly when the cursor is sitting in xterm's "deferred end-of-line" wrap
  state (the cursor is logically past the last column, pending a wrap on the next
  printable character). This is a plausible, version-spanning ambiguity (reported across
  xterm.js 3.14.5 through 4.7) but was not confirmed as fixed or reproduced against 6.0.0
  in this research pass — worth a targeted reproduction against the actual Claude Code
  spinner-line bytes before concluding it's xterm.js's own bug rather than this
  project's pipeline.
- [xtermjs/xterm.js#145 "broken escape sequence parser states"](https://github.com/xtermjs/xterm.js/issues/145)
  is a broader report that certain "execute" characters (e.g. `\n`, `\r`) don't reset
  parser sub-states the way some CSI intermediate states expect, which could in principle
  interact with an EL sequence embedded near a line-feed, but is not EL-specific.
- The official [xterm.js Supported Terminal Sequences](https://xtermjs.org/docs/api/vtfeatures/)
  reference documents EL (`CSI Ps K`) as supported with all three parameter forms (`0`/
  omitted = to end of line, `1` = to start of line, `2` = whole line) — no caveat is
  listed there about partial-write handling.

Sources:
- [broken escape sequence parser states · Issue #145](https://github.com/xtermjs/xterm.js/issues/145)
- [clear() is not correctly clearing the buffers · Issue #943](https://github.com/xtermjs/xterm.js/issues/943)
- [Question: xterm.js 4.0+ handling of EL when cursor is in deferred EOL position · Issue #2979](https://github.com/xtermjs/xterm.js/issues/2979)
- [Supported Terminal Sequences](https://xtermjs.org/docs/api/vtfeatures/)

## 5. Server-side snapshot/quiescence logic — `server/services/connectrpc_websocket.go`

Read in full for the relevant sections (lines 90-330, 554-673).

### Two independent output paths exist, both used

1. **Cold-start snapshot via `tmux capture-pane`**: `getOrRefreshSnapshot` (lines 268-296)
   caches capture-pane output per session (`sessionSnapshot`, lines 184-190), refreshed
   via a `captureFn` closure. `sanitizeInitialContent`/`prepareSnapshotContent`
   (lines 134-156) strip a specific set of *context-dependent* escape sequences —
   `rePositionCodes` (lines 106-112) matches absolute cursor positioning (`ESC[H`,
   `ESC[n;mH`/`f`), screen clears (`ESC[nJ`), private mode set/reset (`ESC[?nh/l`),
   and DEC/CSI save-restore cursor (`ESC7`/`ESC8`, `ESC[s`/`ESC[u]`) — **but this regex
   does not match `\x1b\[K` (EL) at all**, by design (the comment at line 105 explicitly
   says SGR color codes are "intentionally NOT matched"; EL isn't mentioned either way,
   and the pattern has no `K` alternative). So EL sequences inside a capture-pane
   snapshot pass through `sanitizeInitialContent` unmodified.
2. **Live streaming via tmux control mode**: `streamViaControlMode` (line 554) starts
   control mode (`streamer.StartControlMode()`, line 621) and forwards output through an
   output-forwarding goroutine (referenced at lines 630-632, 1756, not the primary path
   read in this pass) that also feeds `quiescenceCh` inline per frame.

### Quiescence detection: currently degenerates to a fixed timer, not real quiescence

`waitForQuiescence` (lines 215-240) is a generic "wait until N ms without an update, or
timeout" helper. The code's own comment at `streamViaControlMode:663-671` states this
explicitly:

> "nothing signals quiescenceCh yet at this point — its only producer is the
> output-forwarding goroutine started further down — so this currently degenerates to a
> fixed quietFor settle rather than real quiescence detection... quietFor is therefore
> set to match the 200ms post-nudge settle."

This means the resize-triggered "wait for the TUI to finish redrawing before capturing a
snapshot" behavior (used for cold-start captures after a dimension-nudge, lines 635-673)
is really just "wait 200ms," not an actual guarantee the TUI's redraw is complete. This
is a **timing risk for item 5's question** — if Claude's TUI takes longer than 200ms to
finish a multi-frame redraw (e.g. a burst of resize-triggered relative-cursor redraws),
`capture-pane` could be invoked mid-redraw, capturing a *partial* screen state including
a status line whose EL hasn't been applied by tmux's own emulation yet. This is a
plausible but unconfirmed mechanism — it wasn't traced end-to-end to a capture-pane call
mid-redraw in this pass.

### Could a chunk-split ever separate an EL from the content that follows it, server-side?

Not found in the code read here. The control-mode output-forwarding goroutine
(referenced but not fully read at lines 630-632/1756) is the piece that would need
verification for true byte-level splitting; based on the comment structure it forwards
control-mode frames as received without describable additional chunking logic in this
file. **Gap, named explicitly**: the actual byte-read loop for control-mode output
(`session/tmux/control_mode.go`, referenced in `.claude/rules` docs) was not read in this
research pass and is a candidate for the next investigation lap if the frontend
`RedrawThrottler` drop mechanism (§3a) doesn't fully explain the observed corruption.

## Summary answer to the explicit request

**A concrete mechanism was found that can drop a complete escape sequence + following
content wholesale**: `RedrawThrottler.process()` in
`web-app/src/lib/terminal/TerminalStreamManager.ts:52-76` silently discards a pending
"full redraw" chunk (`this.pendingRedraw = chunk` with no flush of the prior value) if a
second matching chunk arrives within its 33ms coalescing window. This is the leading,
evidence-backed hypothesis: relative-cursor-addressed redraws depend on every prior frame
having been applied in order, and dropping one desyncs the assumed cursor row for every
subsequent frame, producing stale trailing glyphs exactly matching the bug report's
screenshot.

The `EscapeSequenceParser` (§2) and the server-side `rePositionCodes` sanitizer (§5) both
pass EL sequences through **intact** — neither strips, matches, nor corrupts `\x1b[K` /
`\x1b[0K` / `\x1b[1K` / `\x1b[2K` in any code path read. If §3a's drop mechanism is ruled
out during implementation (e.g. because the actual spinner-line bytes don't match the
`isFullRedraw` regex), the next most likely candidates are: (a) xterm.js 6.0.0's own EL
handling under the "deferred EOL" cursor state (xtermjs/xterm.js#2979, unconfirmed against
this version), or (b) the server's degenerate 200ms fixed-quiescence capture racing a
still-in-progress TUI redraw (§5), not a stripping/corruption bug in this project's own
transform code.
