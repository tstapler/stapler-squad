# New Renderer: Pitfalls and Risks

Research date: 2026-06-24  
Scope: Escape code stripping/corruption in the new SSP-based terminal renderer pipeline

---

## 1. Common Causes of Escape Code Corruption and xterm.js Failure Modes

### Background: What the new renderer does

The new renderer implements a MOSH-style State Synchronization Protocol (SSP). Claude Code PTY output flows through:

```
Claude PTY output
  → tmux capture-pane (raw bytes)
  → Go SSP coordinator (StateGenerator / DeltaGenerator)
  → TerminalState / TerminalDiff proto messages over ConnectRPC binary stream
  → TypeScript StateApplicator / DeltaApplicator
  → xterm.js .write(text)
```

This is fundamentally different from the old raw streaming path, which delivered PTY bytes directly to xterm.js. Every transformation point in the new path is a potential source of escape sequence corruption.

### Known xterm.js failure modes when sequences are missing or corrupt

- **SGR colors rendered as literal characters**: If `\x1b[` is stripped but the rest of the sequence (`32m`, `0m`, etc.) remains, xterm.js outputs the parameter bytes as text garbage.
- **Cursor positioning breaks**: CSI cursor sequences (`\x1b[H`, `\x1b[A`, etc.) used by Claude Code's interactive UI (progress bars, diff views) are lost, causing all output to append rather than overwrite.
- **Character set corruption**: If a sequence terminator byte (`m`, `J`, `K`, etc.) is consumed but the `\x1b[` prefix is missing, xterm.js may partially interpret the leftover characters, flickering or corrupting subsequent output.
- **Replacement character flood (U+FFFD)**: Any bytes that are not valid UTF-8 AND not handled via `stream: true` will be decoded as U+FFFD, creating visible artifacts in terminal output.
- **Hyperlink/OSC sequence silent drop**: OSC sequences (e.g., `\x1b]8;;url\x1b\\`) that span a newline boundary will be split across lines and silently lost.

---

## 2. `server/mcp/ansi.go`: Confirmed NOT on the Display Path

**File**: `/server/mcp/ansi.go`

This file contains the only explicit ANSI stripping function in the codebase: `stripANSI(b []byte) []byte`. It strips CSI sequences (`\x1b[...letter`), OSC sequences (`\x1b]...\x1b\\` or `\x1b]...\x07`), and simple ESC sequences (`\x1b` + one byte). It also replaces invalid UTF-8 bytes with U+FFFD.

**Where it is called**: Only from MCP tool handlers in `/server/mcp/tools_terminal.go`:
- `read_session_output` handler (when `strip_ansi=true`, which is the default)
- `waitForOutput` handler
- `runCommand` handler

These tools serve MCP consumers (i.e., Claude Code itself reading its own session output for the `run_command` tool). They are completely separate from the terminal display pipeline.

**Verdict**: `server/mcp/ansi.go` cannot be the cause of the rendering regression. It operates on a parallel path and is never called during display stream processing.

---

## 3. xterm.js Version and Known Issues

**Version in use**: `@xterm/xterm ^6.0.0` (file: `/web-app/package.json`)

### Key facts about xterm v6

- Version 6 is the first release under the new `@xterm` scoped package namespace (previously `xterm`). This is a **major breaking change** from the consumer perspective — import paths changed.
- The **WebGL renderer is now the default** in v6 (previously an optional addon). This changes rendering performance characteristics and may expose previously-hidden issues with rapid escape sequence delivery.
- Version 6 changed how the `Terminal.write()` callback works; writes are now always asynchronous (buffered internally). Code that assumed synchronous write completion will behave differently.
- The `stream: true` flag on `TextDecoder.decode()` is load-bearing for stateful multi-byte sequences. xterm.js itself does not buffer incomplete escape sequences passed to `.write()` in a way that heals across two separate calls — each call is decoded independently at the JS level before being handed to the parser.

### TextDecoder `stream: true` — the critical flag

The `TextDecoder` API has a `stream` mode that keeps internal state between calls so that multi-byte UTF-8 sequences split across two buffers are reassembled correctly. Without it, the second call starts fresh, emitting U+FFFD for the dangling bytes.

ANSI escape sequences are pure ASCII (bytes < 128), so a split escape sequence will not cause a U+FFFD substitution. However, **a split multi-byte UTF-8 character embedded inside an escape sequence parameter** (rare but possible in OSC title strings) will corrupt the sequence.

More importantly, when the ConnectRPC framing splits a sequence of bytes across two `TerminalDiff` messages and the decoder does not use `stream: true`, the reassembly across calls fails.

---

## 4. Base64 / JSON / String Conversion Risks

### Proto field types (all bytes — no corruption at this layer)

After reviewing `/proto/session/v1/events.proto`:

- `TerminalData.output` → field type: `bytes data` (raw PTY bytes)
- `TerminalDiff.diff_bytes` → field type: `bytes` (raw ANSI sequences passed to xterm.js)
- `TerminalLine.content` → field type: `bytes` (raw line content)
- `TerminalState.lines[].content` → field type: `bytes`

In proto3, `bytes` fields are transmitted as binary and deserialized as `Uint8Array` on the TypeScript side. **No base64 encoding, no UTF-8 validation, no string coercion happens at the proto layer.** This is correct.

The ConnectRPC binary format (connect binary protocol) encodes the entire proto message as binary framing with a 5-byte length prefix — no JSON, no base64. This preserves all byte values including `\x1b`, `\x00`, `\x08`, and bytes > 127.

**Verdict**: No escape sequence corruption occurs at the proto serialization or ConnectRPC transport layer.

### Where string conversion DOES happen (and risks)

**`/session/terminal_state.go`**: The server-side terminal state emulator does `text := string(data)` on raw PTY bytes when processing output for the MOSH SSP coordinator. In Go, `string([]byte)` is a no-op copy that does NOT validate UTF-8. This is safe for the conversion itself, but any downstream code that treats the resulting `string` as valid UTF-8 (e.g., regexp matching) may produce unexpected results on bytes > 127.

---

## 5. ConnectRPC / Protobuf Bytes vs String Fields

Proto3 rule: **`string` fields MUST be valid UTF-8**. If a `string` field contains bytes that are not valid UTF-8, the behavior is undefined — some implementations silently replace them with U+FFFD, others reject the message. Since terminal output routinely contains arbitrary bytes (color codes, cursor positioning, box-drawing characters), using `string` for terminal content would be catastrophically incorrect.

All terminal content fields in this codebase correctly use `bytes`. **This is not the source of the regression.**

The risk materializes only if someone changes a `bytes` field to `string` in the proto definition. No such change was observed in the current proto files.

---

## 6. New Renderer Code: Identified Bugs

### BUG 1 (HIGH): `StateApplicator.ts` — missing `stream: true` on TextDecoder

**File**: `/web-app/src/lib/terminal/StateApplicator.ts`

```typescript
private textDecoder: TextDecoder = new TextDecoder(); // instance-level, no options

const diffStr = this.textDecoder.decode(diff.diffBytes); // NO stream: true
this.terminal.write(diffStr);

const lineText = this.textDecoder.decode(line.content); // NO stream: true
```

The `TextDecoder` instance is reused across multiple `decode()` calls, but without `{ stream: true }` each call finalizes the decoder state. If a multi-byte UTF-8 character or a UTF-8 encoded string inside an OSC sequence is split across two consecutive `TerminalDiff` proto messages, the first call emits U+FFFD for the incomplete byte sequence, and the second call has no memory of the prior incomplete state.

**Fix**: Change all decode calls in StateApplicator to `this.textDecoder.decode(bytes, { stream: true })`.

### BUG 2 (HIGH): `sanitizeUTF8Bytes()` in StateGenerator silently destroys control characters

**File**: `/server/terminal/state.go`, lines 294–352

The sanitizer runs on every line in `splitIntoTerminalLines()`. It correctly sets `inEscape = true` when it sees `\x1b` and passes through escape sequence bytes. However:

1. **Termination logic is fragile**: `inEscape` is cleared when any byte in `[A-Za-z]` is seen. This is correct for most CSI sequences (which terminate with a letter), but intermediate bytes like `?` and `>` (used in private-mode sequences like `\x1b[?1049h` — alternate screen mode) are passed through while `inEscape = true`. The letter `h` at the end correctly clears the flag. However, if an escape sequence contains digits and semicolons only (no intermediate A-Z/a-z byte until the final terminator), the sequence is handled correctly. **The real risk is DCS sequences (`\x1bP...ST`) and PM/SOS sequences** where the string terminator is `\x1b\\` (two bytes). The letter detection fires on `\x1b` (no, this byte sets `inEscape = true` again, restarting), then on `\\` (not a letter) — the DCS string body is passed through `inEscape = true` without issue until the final `\\`. This is actually fine for DCS, but non-obvious.

2. **Critical**: Any control byte `r < 32` that is not `\t`, `\n`, `\r`, `\x07` (BEL), or `\x08` (BS) is **replaced with a space**. This includes:
   - `\x0f` (SI - shift in character set) and `\x0e` (SO - shift out), used by some terminal programs
   - `\x1c`–`\x1f` (file/group/record/unit separators)
   - `\x00`–`\x06`, `\x09`, `\x0b`, `\x0c`, `\x10`–`\x1a`, `\x1c`–`\x1f`
   
   These are silently replaced, which may break terminal programs that rely on them.

3. **The `\n` split problem** (see BUG 3 below) means OSC sequences containing newlines will already be broken before sanitization runs.

**Fix**: The sanitizer should not be applied to lines that contain ANSI sequences at all, or should use a proper ANSI-aware parser instead of a state machine that only handles CSI sequences.

### BUG 3 (MEDIUM): `splitIntoBytesLines()` and `splitIntoTerminalLines()` split on `\n`

**Files**: `/server/terminal/delta.go` (line 249), `/server/terminal/state.go` (line 181)

Both functions use `bytes.Split(output, []byte("\n"))` to split raw terminal output into lines. This is correct for normal line-delimited output, but **OSC sequences can legitimately contain `\n` bytes** in their parameter strings (e.g., OSC 52 clipboard, OSC 133 shell integration markers, multi-line OSC 8 hyperlinks). Splitting on `\n` truncates these sequences mid-parameter.

Additionally, Claude Code's output may include sequences from the new rendering engine that were not present in older versions, making this a regression risk even if it was not a problem before.

**Fix**: Parse the output as a stream of characters and escape sequences, only splitting on bare `\n` bytes that occur outside of escape sequences.

### BUG 4 (MEDIUM): Scrollback chunks in `useTerminalStream.ts` decoded without `stream: true`

**File**: `/web-app/src/lib/hooks/useTerminalStream.ts`, line 308

```typescript
for (const chunk of msg.data.value.chunks) {
    const text = textDecoderRef.current.decode(chunk.data); // NO stream: true
    chunks.push(text);
}
```

The shared `textDecoderRef` is used with `{ stream: true }` on line 271 for the live output path. But for scrollback chunk decoding (line 308), `stream: true` is omitted. Since `textDecoderRef.current` is shared, a previous call with `{ stream: true }` may have left incomplete state in the decoder, and the scrollback calls without `stream: true` will finalize (flush) that state unexpectedly, potentially corrupting the live output path.

**Fix**: Either use a separate `TextDecoder` instance for scrollback decoding, or add `{ stream: true }` to all scrollback chunk decode calls.

### BUG 5 (LOW): `stripANSIBytes()` in delta.go / state.go uses simplified ESC detection

**Files**: `/server/terminal/delta.go` lines 261–283, `/server/terminal/state.go` lines 267–289

This function is used ONLY for cursor column calculation (counting visible characters), not for content sent to the frontend. However, it will mis-count visible columns for:
- Sequences containing `[` followed by digits and `;` — the `[` is not a letter, so `inEscape` stays true, and the digit bytes are silently consumed (correct for CSI). But `\x1b` without a following `[` (simple two-byte sequences) resets `inEscape = true` but then the next non-letter byte may clear it incorrectly.
- OSC sequences: `\x1b]` — the `]` is not A-Z/a-z, so `inEscape` stays true for the entire OSC string body. The ST terminator `\x1b\\` causes `inEscape` to be set `true` again on `\x1b`, and cleared on `\\` (not a letter). This means OSC sequence content leaks into the visible character count.

Since this only affects cursor column display (not content rendering), the impact is cosmetic unless cursor positioning relies on the column count for alignment.

---

## 7. Priority Summary

| Priority | Location | Issue |
|----------|----------|-------|
| HIGH | `StateApplicator.ts` | `textDecoder.decode(diff.diffBytes)` missing `{ stream: true }` — corrupts multi-byte sequences at proto message boundaries |
| HIGH | `server/terminal/state.go` `sanitizeUTF8Bytes()` | Replaces non-`\x1b` control characters with spaces, including SO/SI and others that terminal programs may rely on |
| MEDIUM | `server/terminal/delta.go`, `state.go` | `bytes.Split(output, "\n")` splits OSC sequences that contain newlines |
| MEDIUM | `useTerminalStream.ts` line 308 | Scrollback chunk decoding omits `{ stream: true }` on a shared TextDecoder, polluting decoder state for the live path |
| LOW | `stripANSIBytes()` in both Go files | Simplified escape detection mis-counts visible columns for OSC sequences and some edge-case ESC sequences |
| NOT APPLICABLE | `server/mcp/ansi.go` | Confirmed off the display path — only used by MCP tool handlers |
| NOT APPLICABLE | Proto `bytes` vs `string` | All terminal content fields use `bytes`; no corruption at serialization layer |

---

## 8. Recommended Approach

1. **Fix `StateApplicator.ts` first** — this is the most likely cause of visible escape code corruption in the new renderer since it is on the hot path for every TerminalDiff message.

2. **Audit `sanitizeUTF8Bytes()`** — determine whether it is even necessary in the new renderer path, or whether the sanitization should be moved to a point where it can be properly bypassed for escape sequence content.

3. **Replace naive `\n` splitting** with an escape-sequence-aware line splitter, or document that OSC sequences containing `\n` are deliberately not supported.

4. **Fix scrollback chunk decoding** — use a dedicated `TextDecoder` instance for scrollback to avoid contaminating the live stream decoder state.
