# ADR-013: iOS Text Selection and Copy Approach

## Status: Proposed

## Context

On iOS Safari, users cannot select text or invoke the system copy sheet in the current xterm.js
terminal. There are two distinct copy scenarios:

1. **Program-initiated copy**: A program running in the terminal (e.g., Claude Code's "copy"
   command) wants to place content on the system clipboard.
2. **User-initiated selection**: The user long-presses to select visible terminal text and then
   wants to copy it manually.

The current codebase calls `navigator.clipboard.writeText(selection)` inside
`terminal.onSelectionChange` (XtermTerminal.tsx line 267). On iOS Safari, `onSelectionChange` is
an xterm.js internal event, not a direct DOM user gesture. The browser does not recognize it as a
user activation, so the clipboard write throws `NotAllowedError` silently. This makes all manual
copy operations fail on iOS.

Additionally, when xterm.js mouse tracking mode is `vt200` (set at runtime by Claude Code via
`CSI ? 1000 h`), synthetic `mousedown` events dispatched to `.xterm-screen` are forwarded to the
PTY rather than processed for selection. The current long-press handler in
`useMobileTerminalGestures.ts` has a guard (`if (getMouseTracking() !== 'none') return`) that
silently bails out — leaving the user with no feedback and no selection capability when Claude Code
is running.

Three approaches were evaluated:

**Option A — OSC 52 passthrough only**: Register an OSC 52 handler in xterm.js
(`terminal.parser.registerOscHandler(52, handler)`) and enable `set -g allow-passthrough on` in
tmux. Programs that emit `\x1b]52;c;<base64>\x07` (including Claude Code) will have their
clipboard payload decoded and written via `navigator.clipboard.writeText`. This handles
program-initiated copy but provides no mechanism for user-initiated text selection.

**Option B — Floating Copy button triggered by onSelectionChange, with clipboard write in
synchronous touchend**: Show a floating "Copy" button whenever xterm.js reports a non-empty
selection via `terminal.onSelectionChange`. The button's `touchend` handler calls
`terminal.getSelection()` synchronously and immediately calls `navigator.clipboard.writeText(text)`
without any preceding `await`. The `touchend` event IS a user gesture on iOS Safari; the clipboard
write succeeds. This handles user-initiated selection but not program-initiated copy.

**Option C — Both**: OSC 52 passthrough for program-initiated copy + floating Copy button for
user-initiated selection. The two mechanisms operate independently and cover complementary scenarios.

Key constraints from research:

- `navigator.clipboard.writeText()` on iOS Safari must be called synchronously within the call
  stack of a user gesture event handler (`touchend`, `click`, `pointerup`). Any `await` before
  the call consumes the user activation and causes `NotAllowedError` (pitfall #4).
- `onSelectionChange` is not a user gesture. The correct trigger for the clipboard write is the
  Copy button's `touchend` handler; `onSelectionChange` is used only to show/hide the button.
- OSC 52 passthrough requires a tmux configuration change (`set -g allow-passthrough on`). Without
  this, tmux consumes the DCS-wrapped OSC 52 sequence and it never reaches xterm.js.
- When `terminal.modes.mouseTrackingMode !== 'none'` (the runtime mode as exposed by the public
  xterm.js 6 API, not the prop value), `terminal.select(col, row, length)` must be used directly
  for long-press selection instead of dispatching synthetic `mousedown` events. `terminal.select()`
  bypasses mouse tracking mode entirely and is a stable public API.
- xterm.js 6.0.0 exposes `terminal.getSelectionPosition()` returning `IBufferRange` with start/end
  buffer coordinates, enabling correct floating button positioning.
- The fallback chain for older iOS WebViews: `navigator.clipboard.writeText` → `execCommand('copy')`
  with an invisible `<textarea>` → modal display with manual copy instruction.

## Decision

Adopt **Option C**: OSC 52 passthrough for program-initiated copy combined with a floating Copy
button for user-initiated selection. The two mechanisms are independent and complementary.

Concrete changes:

**OSC 52 passthrough (program-initiated):**
1. Register `terminal.parser.registerOscHandler(52, (data) => { ... })` in `XtermTerminal.tsx`
   after `terminal.open()`. Decode the base64 payload and call `navigator.clipboard.writeText(decoded)`.
   Guard with `if (navigator.clipboard?.writeText)` for older WebViews.
2. Document the required tmux configuration (`set -g allow-passthrough on`) and apply it in the
   tmux session initialization path in `session/tmux/tmux.go` (or surface it as a user-visible
   config option if allowing passthrough has unacceptable security implications for the deployment).

**Floating Copy button (user-initiated):**
1. In `XtermTerminal.tsx`, add an `onSelectionChange` listener that sets React state
   `hasSelection: boolean`. When `hasSelection` is true, render a floating `<button>Copy</button>`
   positioned using `terminal.getSelectionPosition()` converted to viewport pixel coordinates via
   `terminal.element.clientHeight / terminal.rows` and `terminal.element.clientWidth / terminal.cols`
   (public API cell dimension calculation per R3.4).
2. The Copy button's `touchend` (and `onClick` for desktop) handler calls
   `terminal.getSelection()` synchronously then immediately `await navigator.clipboard.writeText(text)`.
   No async work occurs before the clipboard write — the user activation is preserved.
3. Show a brief "Copied" toast after the write resolves (R3.2). Dismiss the Copy button and
   `terminal.clearSelection()`.
4. Implement the fallback chain: if `navigator.clipboard` is unavailable, use `execCommand('copy')`
   with a temporary `<textarea>` element.

**Long-press selection fix (prerequisite for the Copy button to have anything to copy):**
1. In the unified `useTerminalGestures` hook (per ADR-014), the `SELECTING` state uses
   `terminal.select(startCol, startRow, length)` unconditionally — regardless of
   `terminal.modes.mouseTrackingMode`. This replaces the current guard that silently exits when
   tracking mode is not `'none'`.
2. Cell coordinates are computed from touch position using
   `Math.floor((touchY - canvasTop) / (terminal.element.clientHeight / terminal.rows))` (public
   API, R3.4).

## Consequences

**Positive:**
- Covers both program-initiated copy (OSC 52) and user-initiated selection (floating button).
- The floating button's `touchend` handler correctly satisfies iOS Safari's synchronous-user-gesture
  requirement. Copy works on iOS 13.1+.
- `terminal.select()` works in all `mouseTrackingMode` values; Claude Code sessions no longer
  silently disable selection.
- The OSC 52 path is invisible to the user and requires no UI changes for programs that already
  support it.
- Public xterm.js API used throughout; no private `_core._renderService` access.

**Negative / trade-offs:**
- `set -g allow-passthrough on` in tmux enables arbitrary OSC/DCS passthrough from any program
  running in the session. This is a known security consideration (a malicious program could emit
  OSC 52 to exfiltrate clipboard content). For the stapler-squad deployment model (user-owned
  agent sessions), this is acceptable; the risk should be noted in the tmux configuration.
- The floating Copy button requires pixel-accurate positioning math. The public cell dimension
  formula (`element.clientHeight / terminal.rows`) is accurate after `fit()` but may be off by a
  fraction of a pixel at non-integer scaling ratios. This is cosmetically acceptable.
- The `SerializeAddon`-based selection state recovery needed if `terminal.clear()` is called during
  deep-history paging (ADR-012) will reset the selection, dismissing the Copy button. This is
  expected and acceptable behavior.

## Alternatives Considered

**Option A (OSC 52 only)** was rejected because it provides no mechanism for a user to select and
copy arbitrary visible terminal text. Users who want to copy command output, error messages, or
URLs cannot do so without the program explicitly emitting OSC 52 — a capability most programs do
not have.

**Option B (floating button only)** was rejected because it requires user interaction for every
copy, even for operations where the program has already identified the text to copy (e.g., Claude
Code's "copy code block" feature). OSC 52 provides a zero-friction path for these cases and is
the established standard in terminal emulators (supported by Blink Shell, iTerm2, Alacritty, etc.).
