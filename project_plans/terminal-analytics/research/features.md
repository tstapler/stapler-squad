# Features Research: Terminal Escape Code Analytics

## 1. Terminal Multiplexers — Escape Sequence Passthrough vs. Interpretation

### How tmux Handles Sequences

tmux is the key multiplexer in this project. It operates in two distinct modes relevant
to analytics:

**Interpretation mode (default):** tmux fully parses and re-emits escape sequences. It
maintains its own internal virtual terminal state machine (the `input_parse` state machine
in `input.c`). When a client attaches, tmux *re-synthesizes* the output from its internal
state — it does **not** replay the raw bytes from the PTY. This is the root cause of
potential mangling: sequences that were valid from the subprocess can be transformed,
omitted, or regenerated differently when tmux writes to its clients.

Key transformation points in tmux:
- **Alternate screen sequences** (`?1049h`/`?1049l`) are tracked in tmux state but not
  forwarded verbatim to clients — tmux manages the screen buffer itself.
- **OSC sequences** (window title `OSC 0`, clipboard `OSC 52`, etc.) are intercepted and
  conditionally forwarded based on `allow-passthrough` and `set-titles` settings.
- **Mouse tracking sequences** (`?1000h` etc.) are mediated by tmux's own mouse handler.
- **DCS sequences** (e.g., Sixel graphics `DCS ... ST`, `tmux;...ST` passthrough) require
  explicit `allow-passthrough on` to be forwarded.

**`capture-pane` semantics:** `tmux capture-pane -p` returns the *rendered text state*,
not the original escape byte stream. The escape sequences in its output are re-synthesized
from tmux's internal cell attributes. This means SGR attributes are reconstructed (e.g.,
`\x1b[0m\x1b[32m` rather than the original sequence), and absolute positioning codes
reflect the captured grid, not the original cursor path. This is documented in the
`connectrpc_websocket.go` codebase via `sanitizeInitialContent` and
`prepareSnapshotContent` which strip and rewrite these re-synthesized sequences.

**Instrumentation capability:** tmux has no built-in logging of escape sequences at the
wire level. The only hook points are:
- `pipe-pane` command: routes raw PTY output to a child process. This captures bytes
  *after* tmux has processed them from the client (subprocess), so it sees the raw stream
  at the pane level — before tmux's own rendering, but after any PTY translation.
- The `tmux;...ST` passthrough DCS sequence (requires `allow-passthrough on`): allows
  applications inside tmux to send arbitrary sequences to the outer terminal.

**Screen and Zellij:** Screen behaves similarly to tmux — it maintains a virtual terminal
and re-synthesizes output. Zellij (Rust-based, newer) has more explicit plugin API hooks
and uses a state machine in `zellij-utils/src/input/parse_keys.rs`. Neither exposes an
analytics/debug API comparable to xterm.js's `IParser`.

### What This Means for Instrumentation

The right place to observe the *true* raw bytes from the subprocess PTY is:
- **Before** tmux re-synthesizes: the PTY file descriptor read loop
  (`session/pty_access.go` `Read()`)
- **After** tmux re-synthesizes (for comparison): the control-mode update channel
  (`SubscribeControlModeUpdates()`) or the `capture-pane` output

The existing `session/circular_buffer.go` and `session/pty_access.go` already sit at
Stage 1. Wrapping `PTYAccess.Read()` with the `EscapeCodeParser` is the correct
instrumentation point.

---

## 2. Terminal Emulators — Debug/Analytics APIs

### xterm.js (the project's renderer)

xterm.js v6 (the version in use: `@xterm/xterm: ^6.0.0`) exposes a formal `IParser` API
as a **proposed API** (requires `allowProposedApi: true`, already set in
`XtermTerminal.tsx`).

**`terminal.parser` (IParser interface):**
- `registerCsiHandler(id: IFunctionIdentifier, callback)` — fires for every CSI sequence
  (cursor movement, SGR, erase, etc.). The callback receives params and intermediates.
- `registerOscHandler(ident: number, callback)` — fires for every OSC sequence. Callback
  receives the raw OSC data string.
- `registerDcsHandler(id: IFunctionIdentifier, callback)` — fires for DCS sequences.
  Callback provides a `IDcsHandler` with `hook(params, intermediates)`, `put(data)`, and
  `unhook(success)`.
- `registerEscHandler(id: IFunctionIdentifier, callback)` — fires for simple ESC+char
  sequences.

These handlers are **additive** — they do not replace the built-in processing. The
terminal still renders normally; the callbacks are called concurrently with parsing. This
makes them ideal for zero-side-effect observation.

**Key characteristics:**
- Callbacks are synchronous and called on the xterm.js internal parsing thread (microtask
  queue). They must be fast to avoid blocking rendering.
- `registerCsiHandler` receives a parsed params object (`IParams`), not raw bytes — the
  codepoint/parameter extraction is already done.
- Raw bytes are not available through the public parser API; only structured params are
  exposed. To get raw bytes at the browser side, you would need to intercept `terminal.write()`
  at the call site (in `TerminalStreamManager.ts`), which is straightforward.
- The `onData` event (`terminal.onData`) fires on **input** (user keystrokes), not on
  output sequences — not useful for observing server-sent sequences.

**Practical pattern for Phase 2 browser instrumentation:**

```typescript
// In XtermTerminal.tsx after terminal is created
const parser = terminal.parser; // IParser (proposed API)

parser.registerCsiHandler({ final: 'm' }, (params) => {
  // SGR sequence — params.toArray() gives the parameter array
  reportSequence('CSI', 'm', params.toArray());
  return false; // false = let xterm.js also handle it normally
});

parser.registerOscHandler(0, (data) => {
  // OSC 0 — window title
  reportSequence('OSC', '0', data);
  return false;
});
```

Returning `false` from a handler means "continue with default handling." Returning `true`
means "suppress default handling" — use `false` for passive observation.

### Alacritty and WezTerm

Neither Alacritty nor WezTerm expose an analytics API. They are native terminal emulators
without plugin/addon systems comparable to xterm.js. WezTerm has a Lua plugin API for
event handling, but it targets key bindings, UI events, and tab management — not escape
sequence interception. There is no standard protocol for terminal emulator analytics.

---

## 3. Open-Source Tools for Terminal Output Diffing and Sequence Inspection

### ttyrec / ttyplay

`ttyrec` records a PTY session by wrapping the `write(2)` calls into timestamped frames
stored in a binary file (format: `{uint32 sec, uint32 usec, uint32 len, bytes}`). `ttyplay`
replays the recording at original or adjusted speed. **Not useful for sequence diffing**
— it records raw bytes only; no parsing or categorization.

Relevant to this project: `ttyrec` could be used as a low-level raw-bytes capture
alternative, but the project already has `CircularBuffer` which serves the same role.

### termdbg / term-dump tools

- **`termdbg`** (Python): A PTY wrapper that intercepts bytes and decodes them to human-readable
  descriptions (similar to what `escape_code_descriptions.go` does). Outputs a trace like
  `CSI A` (cursor up). Limited to interactive use; no programmatic API or batch mode.
- **`infocmp`/`tput`**: Query terminfo databases; useful for understanding what sequences a
  terminal *should* emit but not for runtime observation.
- **`showkey -a`**: Shows raw key codes on input; not useful for output sequences.
- **`hexdump`/`xxd` via `script`**: `script -q /dev/null` creates a PTY, `xxd` on the
  output shows raw bytes. Usable for manual inspection but not automated analytics.

### vttest / VT-100 test suite

`vttest` is a conformance tester for VT100/VT220 terminals. It emits known sequences and
expects specific visual outcomes. Useful for building a test corpus (requirement AC-1) but
not for runtime analytics.

### asciinema

`asciinema` records terminal sessions in a structured JSON format (`v2` format). Each
frame is `[time, "o", data]` where data is the raw byte string (UTF-8). The `data` field
contains the escape sequences as emitted — making asciinema recordings a useful **test
corpus source** for `EscapeParser` validation (AC-1).

The asciinema project includes `asciinema-player` (a JS/Web player) that uses a custom
escape sequence parser (`src/vt/lib/` in the asciinema-player repository). This parser is
written in Rust and compiled to WASM, implementing a full VT100/VT220 state machine. It
could be referenced for parser correctness, though the project's Go implementation in
`pkg/analytics/escape_code_parser.go` is already well-structured.

### termsnap / terminal-recorder libraries

- **`termrec`** (Go): Similar to ttyrec, captures raw PTY bytes.
- **`gotty`** / **`ttyd`**: Web-based terminal sharing; relevant only for showing that
  browser-side terminals (xterm.js) can receive real PTY output over WebSocket — which
  this project already does.

### Escape Sequence Diffing

No established open-source tool performs "escape sequence diffing" in the sense required
by FR-7 (Stage 1 vs Stage 2 comparison by session_seq). The closest approximation is:
- **`colordiff`** / **`delta`**: Diff with ANSI color support — they *consume* escape
  sequences for display, they don't inspect or diff them.
- Custom approaches in the wild typically use `xxd | diff` on raw recordings.

**Conclusion:** The project needs to build its own sequence diffing infrastructure (FR-7).
No off-the-shelf tool covers the two-stage pipeline comparison use case.

---

## 4. xterm.js Parser Hooks — Detailed API for Browser-Side Observation

### IParser Interface (xterm.js Proposed API)

Accessed via `terminal.parser` after setting `allowProposedApi: true`. Both `XtermTerminal.tsx`
and the test terminal page already have this flag set.

**Handler registration methods:**

| Method | Trigger | Callback Signature | Notes |
|---|---|---|---|
| `registerCsiHandler(id, cb)` | Every CSI sequence | `(params: IParams) => boolean` | `id = { final: 'm' }` for SGR |
| `registerOscHandler(ident, cb)` | OSC sequences | `(data: string) => boolean` | `ident` is the numeric OSC command |
| `registerDcsHandler(id, cb)` | DCS sequences | `IDcsHandler` object | Streaming: hook/put/unhook |
| `registerEscHandler(id, cb)` | Simple ESC+char | `() => boolean` | `id = { final: '7' }` for ESC 7 |

**IParams interface:**
- `params.length: number` — number of parameters
- `params.params[i]: number` — integer value of parameter i
- `params.toArray(): number[]` — all params as array
- Sub-parameters via `params.subParamsLength(i)` and `params.subParams(i)`

**Handler return value semantics:**
- `return false` — allow xterm.js default handling to proceed (use this for observation)
- `return true` — suppress xterm.js default handling (use only for override scenarios)

**Disposable pattern:**
```typescript
const disposable = parser.registerCsiHandler({ final: 'm' }, handler);
// On cleanup:
disposable.dispose();
```

**Performance characteristics:**
- Handlers run synchronously in the xterm.js parsing loop, called from `InputHandler`
- Budget per handler call should be < 1µs to avoid blocking the render pipeline
- For batch reporting, buffer observations locally and flush asynchronously

**Raw byte access at browser side:**

The `IParser` API does **not** provide raw bytes — only structured params. To intercept
raw bytes at the browser side (for hash-based comparison with Go Stage 1/2 observations),
the approach is to wrap `terminal.write()` in `TerminalStreamManager.ts`:

```typescript
// In TerminalStreamManager.ts — wrap write to capture raw bytes
const originalWrite = terminal.write.bind(terminal);
terminal.write = (data: string | Uint8Array, callback?: () => void) => {
  captureBytes(data); // inspect raw bytes here
  return originalWrite(data, callback);
};
```

This is the only way to see the exact bytes delivered to xterm.js, which corresponds
to Stage 3 in the pipeline (after ConnectRPC deserialization).

**Integration path for Phase 2:**

The project already has `EscapeSequenceParser.ts` in `web-app/src/lib/terminal/` which
performs partial-sequence buffering. This is the right place to add observation hooks —
it sits at the write boundary to xterm.js and already processes the byte stream chunk
by chunk. Augmenting `EscapeSequenceParser.processChunk()` to emit sequence observations
before returning would require minimal structural change.

A lightweight `BatchedEscapeReporter` could accumulate observations in memory and POST
them to a new `/api/v1/escape-analytics/report` endpoint every 500ms or on page unload
(`visibilitychange` + `pagehide` events for unload safety).

---

## Summary of Key Findings for Implementation

### Existing Infrastructure to Leverage

1. `pkg/analytics/escape_code_parser.go` — complete Go escape sequence parser already
   exists with partial-sequence buffering, CSI/OSC/DCS/DEC-private categorization, and
   the `EscapeCodeStore` for in-memory aggregation. This is the basis for FR-1 / FR-2.

2. `server/analytics/sqlite_provider.go` — SQLite-backed ent ORM analytics writer exists.
   The `escape_event` schema entity just needs to be added following the same pattern as
   `analytics_event` in `session/ent/schema/analytics_event.go`.

3. `web-app/src/lib/terminal/EscapeSequenceParser.ts` — client-side partial-sequence
   parser is the ideal hook point for Phase 2 browser-side instrumentation. It processes
   every chunk written to xterm.js.

4. `XtermTerminal.tsx` already sets `allowProposedApi: true`, enabling `terminal.parser`
   IParser hooks without any configuration change.

### Critical Architecture Observation

The tmux `capture-pane` path (used for initial snapshot delivery in `connectrpc_websocket.go`)
produces **re-synthesized** escape sequences, not the original PTY bytes. Sequences
captured here will structurally differ from Stage 1 (raw PTY) observations even in the
absence of bugs. The mangle detection logic (FR-7) must account for this by only comparing
Stage 1 vs Stage 2 on the **live streaming path** (control mode `SubscribeControlModeUpdates`
channel bytes), not on the capture-pane snapshot path.

### xterm.js IParser Recommendation

For Phase 2, use `registerCsiHandler` / `registerOscHandler` (returning `false` for
passive observation) rather than wrapping `terminal.write()`. The structured params
interface is easier to serialize into proto messages. For raw-byte hash comparison,
wrap `TerminalStreamManager`'s write call at the `processChunk` output boundary.

### Test Corpus

Use `asciinema` v2 recording format as the source for the `EscapeParser` test corpus
(AC-1). The JSON format is easy to parse in Go tests; existing recordings of vim, htop,
and ncurses programs are available from the asciinema public library.
