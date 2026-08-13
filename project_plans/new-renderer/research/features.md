# Feature Landscape: new-renderer (terminal rendering bug)

**Date**: 2026-06-24
**Status**: Research complete
**Companion**: See `architecture.md` for the full pipeline map.

---

## 1. What Changed: The "New Renderer"

No commits to this repository introduce a "new renderer." The branch name `stapler-squad-new-renderer` is the investigation branch, not a completed feature branch. The renderer change is in **Claude Code itself** (external dependency), not in stapler-squad.

The most recent terminal-adjacent commits in this repo:

| Commit | Description |
|---|---|
| `e158b305` | feat(server): startup fast restore, terminal snapshot fix, build warnings |
| `4104b01d` | perf: reduce goroutine wakeups, lazy-load xterm.js, memoize SessionCard |
| `da5794eb` | feat(analytics): terminal escape code analytics pipeline |
| `acb07507` | fix(terminal): prevent premature resize from corrupting dimension cache |
| `e88a8f0c` | Add comprehensive tests for EscapeCodeStore |

No commit modifies the core PTY reading or xterm.js write path. The "new renderer" therefore refers to a Claude Code version change that emits different escape sequences — likely a migration from a legacy terminal renderer to one based on Ink (React for CLIs) or a similar TUI framework that emits cursor-up-based full-screen redraws on every update.

---

## 2. Terminal-Analytics Pipeline: Implementation Status

**Status: Fully implemented.** Confirmed present via `pkg/analytics/` directory and commit `da5794eb`.

### Go-side (server)

| File | Feature | Status |
|---|---|---|
| `pkg/analytics/escape_code_parser.go` | `EscapeCodeParser` — parses CSI/OSC/DCS/PM/APC/SOS/C1/Simple/DECPriv/SGR/Cursor/Erase/Scroll/Charset categories | Implemented |
| `pkg/analytics/escape_code_store.go` | SQLite-backed `EscapeCodeStore` — persists escape events per session | Implemented |
| `pkg/analytics/escape_event_writer.go` | `EscapeEventWriter` interface + batch writer | Implemented |
| `pkg/analytics/mangle_correlator.go` | `MangleCorrelator` — compares Stage 1 vs Stage 2 payloads using `(sessionID, sessionSeq, payloadHash)` to detect pipeline corruption | Implemented |
| `pkg/analytics/escape_code_descriptions.go` | Human-readable descriptions for all escape categories | Implemented |

The `EscapeCodeParser` covers categories that the **JavaScript `EscapeSequenceParser` does not**: DCS (`\x1bP`), PM (`\x1b^`), APC (`\x1b_`), SOS (`\x1bX`). If Claude Code's new renderer emits any of these (DCS passthrough is common in newer TUI frameworks), the Go analytics parser will see them but the JS parser will not recognize them as complete sequences and may incorrectly buffer or pass them through raw.

### Stage 1 and Stage 2 wiring

Both taps are **already wired in production code**:

- **Stage 1** (`session/response_stream.go` ~line 278): `escapeParser.Parse(chunk.Data, sessionSeq)` — runs before the circular buffer write, on raw PTY bytes
- **Stage 2** (`server/services/connectrpc_websocket.go` ~line 765): `escapeParser.ParseStage2(buf, instance.GetTotalBytesWritten())` — runs on the coalesced transport frame before WebSocket send

The `MangleCorrelator` can diff Stage 1 vs Stage 2 payloads in real time. It is wired but may need to be enabled via config (check `session/response_stream.go` `newEscapeParserForSession()` for the config gate).

### JavaScript-side (browser)

No analytics instrumentation exists in the browser pipeline. The `EscapeSequenceParser` and `RedrawThrottler` in `TerminalStreamManager.ts` emit no telemetry. This is the **gap** — Go-side corruption is detectable, but browser-side dropping is invisible.

---

## 3. JavaScript Terminal Feature Map

All files in `web-app/src/lib/terminal/`:

| File | Feature | Relevance to Bug |
|---|---|---|
| `EscapeSequenceParser.ts` | Partial-sequence boundary buffering; ED3 stripping | **Primary suspect** — strips `\x1b[3J` and buffers partials via 20-char lookback |
| `TerminalStreamManager.ts` | Write pipeline: `RedrawThrottler` → `EscapeSequenceParser` → chunked writes | **Primary suspect** — `RedrawThrottler` drops frames that start with `\x1b[\d+A` |
| `StateApplicator.ts` | Applies terminal state diffs | Unknown — not read |
| `DeltaApplicator.ts` | Delta-based terminal update | Unknown — not read |
| `MessageQueue.ts` | Write serialization queue | Low risk |
| `CircularBuffer.ts` | JS-side circular buffer | Low risk |
| `AnsiCodes.ts` | ANSI code constants | Reference only |
| `mouseTracking.ts` | Mouse tracking mode management | Low risk |
| `TerminalDimensionCache.ts` | Terminal size caching | Low risk |
| `EchoOverlay.ts` | Input echo for latency masking | Low risk |

### `EscapeSequenceParser` feature inventory

Supported sequence types (complete detection):
- CSI (`\x1b[`) — terminates on letter A-Z/a-z
- OSC (`\x1b]`) — terminates on BEL `\x07` or ESC-backslash
- Simple 2-char (`\x1b` + any non-`[`/`]` char)
- C1 codes (`\x1b` + char in 0x40–0x5F)

**Not supported** (will not detect as partial — may be incorrectly passed through or split):
- DCS (`\x1bP`) — Device Control String, used in tmux passthrough and Sixel graphics
- PM (`\x1b^`) — Privacy Message
- APC (`\x1b_`) — Application Program Command, used by iTerm2/kitty for image protocols
- SOS (`\x1bX`) — Start of String
- SS2/SS3 (`\x1bN`/`\x1bO`) — Single Shift for G2/G3 character sets

Additional active transformation:
- ED3 filter: `\x1b[2J\x1b[3J` → `\x1b[2J` (strips erase-scrollback when paired with erase-visible)

Partial lookback window: `Math.min(20, data.length)` characters. Sequences longer than 20 bytes (OSC sequences with payloads, DCS passthrough strings) may not be detected as partial if the chunk break falls within the payload rather than the `\x1b` prefix.

---

## 4. Existing Tests

### Frontend tests

| Test file | What it covers | Gap |
|---|---|---|
| `__tests__/EscapeSequenceParser.test.ts` | CSI buffering, OSC buffering, simple escapes, ED3 filter, multi-sequence chunks | No tests for DCS/PM/APC/SOS detection; no tests with chunks larger than 20-char lookback window |
| `__tests__/TerminalStreamManager.test.ts` | Write queuing, flow control | Unknown whether RedrawThrottler behavior is tested |
| `__tests__/flow-control-stress.test.ts` | Stress test for write pipeline under load | May catch RedrawThrottler dropping |
| `__tests__/EscapeSequenceParser.bench.test.ts` | Performance benchmarks | Not a correctness test |
| `__tests__/MessageQueue.test.ts` | Queue ordering | Not escape-related |

### Go tests

| Test file | What it covers |
|---|---|
| `pkg/analytics/escape_code_parser_test.go` | `EscapeCodeParser` parsing accuracy |
| `pkg/analytics/escape_code_store_test.go` | SQLite persistence |
| `pkg/analytics/mangle_correlator_test.go` | Stage 1 vs Stage 2 comparison logic |
| `session/terminal_state_test.go` | Terminal state machine |
| `session/terminal_state_integration_test.go` | End-to-end terminal state transitions |
| `session/tmux/control_mode.go` (no separate test file found) | `decodeControlModeOutput` octal decoding — not explicitly tested |

### Notable test gap: `decodeControlModeOutput`

No dedicated test was found for `session/tmux/control_mode.go`'s `decodeControlModeOutput` function. This function decodes tmux control mode's octal encoding (`\ooo` → byte). If Claude Code's new renderer emits multibyte UTF-8 sequences or longer escape strings, tmux wraps each byte in an octal escape, and `decodeControlModeOutput` must reconstruct them correctly. A sequence split across an `%output` line boundary could produce a partial escape that looks valid in octal but decodes to a corrupted byte sequence.

---

## 5. Key Unknowns Requiring Active Investigation

1. **What sequences does the new Claude Code renderer emit that the old one did not?**
   The most likely answer is full-screen Ink/React-based redraws: frames starting with `\x1b[H\x1b[2J` (home+erase) or `\x1b[\d+A\x1b[2K` (cursor-up+clear-line loops). These interact with `RedrawThrottler`'s `/^\x1b\[\d+A/` regex and `sanitizeInitialContent`'s erase-screen stripping.

2. **Is the `MangleCorrelator` actively enabled in the config?**
   Check `config/config.go` for the escape-analytics config gate and whether the feature flag is enabled on this branch.

3. **Does the new renderer use DCS passthrough or APC sequences?**
   Some newer CLI frameworks (Kitty graphics, iTerm2 inline images) use APC/DCS. The JS `EscapeSequenceParser` does not handle these — they would be written unsplit to xterm.js only by coincidence (if the chunk boundary does not fall within the sequence).

4. **Does `StateApplicator.ts` or `DeltaApplicator.ts` modify bytes before or after `EscapeSequenceParser`?**
   These files were not read. If they sit in the write pipeline, they are additional stripping candidates.

---

## 6. Immediate Diagnostic Steps

In priority order:

1. **Read `TerminalStreamManager.ts` `write()` method** — confirm whether `StateApplicator` / `DeltaApplicator` are called in the write path or only in a snapshot/diff path.

2. **Enable the MangleCorrelator** (if not already enabled) — it will log any bytes present at Stage 1 that are absent at Stage 2, pointing directly at Go-side corruption.

3. **Add a 2-line JS diagnostic to `EscapeSequenceParser.processChunk`**:
   ```typescript
   if (data.length !== filtered.length || partial.length > 0)
     console.debug('[ESP]', { in: data.length, out: filtered.length, buffered: partial });
   ```
   This exposes any stripping or buffering happening before xterm.js.

4. **Run existing tests against a Claude Code session** to confirm the test-harness at `web-app/src/app/test-terminal` is still functional. The docs archive reference at `docs/archive/tasks/obsolete/terminal-streaming-test-harness.md` suggests a test harness existed and was archived — check whether it can be revived.
