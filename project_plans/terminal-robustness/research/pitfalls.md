# Terminal Robustness — Failure Modes, Traps, and Pitfalls

Research date: 2026-05-09. Based on codebase audit of XtermTerminal.tsx, TerminalOutput.tsx,
useMobileTerminalGestures.ts, useTouchScroll.ts, session/tmux/tmux.go, session/scrollback/manager.go,
web-app/src/lib/hooks/useTerminalStream.ts, and web searches.

---

## 1. xterm.js Scrollback Write Ordering

### Can you write to xterm.js before `terminal.open()`?

No. Calling `terminal.write()` before `terminal.open(containerEl)` throws internally because the
parser and buffer are not yet attached to a DOM node. The write call will be silently dropped or
produce a "Terminal must be opened" runtime error depending on xterm.js version. All writes must
happen after `open()` returns.

### Can you inject historical lines into xterm.js scrollback?

xterm.js does not expose a backfill API. Scrollback is append-only: every call to `terminal.write()`
appends to the end of the buffer; there is no way to insert lines at an arbitrary scrollback
position. This has three consequences for the R2 scrollback plan:

1. **Write order is presentation order.** To show history before live output, the server must send
   history *first*, then live bytes second. Any architecture that sends initial pane content and
   live stream on parallel channels risks live bytes arriving and being written before the history
   batch finishes rendering.

2. **`writeInitialContent` clears the terminal first** (see `TerminalStreamManager.writeInitialContent`
   line 234: `this.terminal.clear()`). If live bytes arrive during the async `enqueueWrite`, they
   will be written into a partial/cleared buffer — producing blank-then-flash artifacts. The
   `isInitializing` flag or a write-lock must block live output until initial content is fully
   flushed.

3. **`clear()` resets the scrollback buffer.** Calling `terminal.clear()` does not just clear the
   viewport — it also empties the scrollback. Therefore calling `clear()` immediately before writing
   history (as the current code does) is correct for the first load, but calling it on reconnect
   after the user has already scrolled up will destroy history the user was reading.

### Ordering race: `scrollbackResponse` vs live `output` on the same WebSocket stream

ConnectRPC over a single WebSocket provides FIFO ordering within that connection (TCP guarantees
in-order delivery; ConnectRPC framing is sequential within a stream). However:

- If the server sends a `scrollbackResponse` message followed by `output` messages, the client
  processes them strictly in arrival order.
- The risk is **not** interleaving at the protocol level; the risk is in the client handler. The
  current `onScrollbackReceived` callback calls `getOrCreateStreamManager()` and then
  `await manager.writeInitialContent(scrollback)`. Because `writeInitialContent` is async
  (`enqueueWrite` uses `Promise`), and JavaScript is single-threaded but yielding at each `await`,
  any `output` messages that arrive and are processed synchronously by the stream loop before the
  `writeInitialContent` Promise resolves will be passed to `manager.write()` immediately — racing
  with the still-enqueued initial content.
- **Concrete risk**: if the server sends `scrollbackResponse` then immediately streams live bytes
  (normal for a busy session), the live bytes can be written into `xterm` before the queued
  scrollback chunks have been flushed. The result: user sees live output first, then history is
  appended after it — the buffer appears out of order.
- **Mitigation**: set a boolean flag `isWritingInitialContent` on `TerminalStreamManager`; in
  `write()`, if that flag is set, buffer the live bytes and flush them only after
  `writeInitialContent` resolves. Clear the flag in the `finally` block.

---

## 2. xterm.js Resize Race Conditions

### The current flow and where it breaks

```
ResizeObserver fires → setTimeout(debounce) → rAF → rAF → fitAddon.fit()
  → terminal.onResize fires → handleTerminalResize() → resize() RPC
    → server: ioctl SIGWINCH → tmux reflows → capture-pane → snapshot sent
      → client: onOutput callback → terminal.write(snapshot)
```

**Race 1 — Double fit on mount**: The current code calls `fitAddon.fit()` twice:
- First call: inside the double-rAF block at lines 222–244 of `XtermTerminal.tsx`.
- Second call: the `setTimeout(..., 100)` at line 239–244.
Each `fit()` call that changes cols/rows fires `terminal.onResize`, which calls the server resize
RPC. This produces two resize events within 100 ms on every fresh mount, creating two
capture-pane cycles. Requirements R1.3 and R1.2 explicitly prohibit this; the second fit must
be removed.

**Race 2 — Adaptive debounce fires too early**: The ResizeObserver handler uses a 10 ms debounce
for the first 3 resizes and 250 ms thereafter. 10 ms is shorter than a single animation frame
(~16 ms). The browser resize event, the ResizeObserver callback, the setTimeout, and two rAFs
can all complete before tmux has finished responding to the previous `SIGWINCH`. The server then
receives a resize for the old size it just reflowed — triggering a second unnecessary reflow.
Requirement R1.2 mandates ≥150 ms; the 10 ms fast-path violates it.

**Race 3 — Snapshot arrives before fit is stable**: The server sends a post-resize snapshot
(capture-pane output). If the snapshot arrives while the client is still in the middle of a
second fit cycle (second rAF of the double-rAF at lines 328–329), `terminal.write(snapshot)`
updates the buffer at the old column width. The subsequent `fit()` then changes cols/rows, making
tmux's snapshot render at the wrong width. Symptoms: lines truncated or bleeding into the next.
Mitigation: the client must ignore (queue) incoming output messages while a resize is in-flight
and resume writing only after `fitAddon.fit()` completes and the next `onResize` fires with the
confirmed new dimensions.

**Race 4 — ResizeObserver and visualViewport both active**: `TerminalOutput.tsx` installs a
`visualViewport.resize` listener (line 656) that calls `xtermRef.current?.fit()` directly on the
`XtermTerminalHandle`. This bypasses the debounce in `XtermTerminal.tsx` completely. When the
virtual keyboard appears on iOS, both the ResizeObserver (container shrinks) and visualViewport
(viewport height changes) fire nearly simultaneously, causing two uncorrelated `fit()` calls with
no deduplication. The `isFittingRef` guard only blocks the `visualViewport` re-entrant call, not
the ResizeObserver one.

**Race 5 — Snapshot before quiescence**: The server has no tmux quiescence wait. After `resize-window`
the server calls `capture-pane` immediately (or after the next control-mode notification). tmux
reflows wrapped lines after SIGWINCH, but the reflow is not instantaneous — it happens in the
tmux event loop on the next iteration after the resize ioctl. A capture-pane that runs before tmux
finishes reflowing sees a mixed-width buffer: some lines at old width, some at new width.
Requirement R1.1 mandates ≥100 ms quiescence before capture-pane.

---

## 3. tmux `capture-pane -e` Known Limitations

### Alternate screen (vi, less, htop)

`tmux capture-pane -e` captures the *current* pane buffer. By default it captures the **normal**
buffer, not the alternate screen. When a program like `vi`, `less`, or `htop` is running in the
alternate screen (`\x1b[?1049h`), `capture-pane` without the `-a` flag returns the **contents of
the normal buffer** (i.e., the shell history before the TUI launched) — not the current TUI
screen. The `-a` flag exists to capture the alternate screen explicitly.

Consequence: if the code sends a post-resize snapshot while `vi` is open, it may send shell
history instead of the current `vi` buffer. The user sees the correct display locally (via the PTY
stream) but the post-resize "refresh" snapshot shows wrong content.

Additionally, `capture-pane -e` with the `-J` flag (join wrapped lines) **strips cursor-positioning
escape codes**. This is explicitly noted in `CapturePaneContentRaw` (tmux.go line 1692–1694):
"The -J flag strips cursor positioning codes, breaking TUI rendering." Post-resize snapshots that
use the `-J` variant will produce incorrect TUI display for any app using relative cursor movement.

### Behavior immediately after resize

After a `resize-window` command, tmux reflowing the history is CPU-intensive and asynchronous
within the tmux event loop. Benchmarks from the tmux issue tracker show that panes with long
history (thousands of lines) can take tens to hundreds of milliseconds to reflow before the new
dimensions are stable. A `capture-pane` issued within this window returns partially reflowed
content — lines at the old width before the reflow point, new width after. This is the direct
source of the "corrupted resize" bug described in Problem 1.

There is no tmux API to wait for reflow completion. The only reliable approach is a quiescence
timer: poll `capture-pane` and compare successive outputs until they are identical, or use a
fixed wait of ≥100 ms as specified in R1.1.

### History reflow performance

When a pane has a large scrollback history, `resize-window` triggers a full history reflow. For a
session with 10,000 lines of history and a width change, this can stall the tmux server for
100–500 ms. During this window, all output from the PTY is delayed (tmux buffers it). The
`stapler-squad` server should debounce resize RPCs at the backend level (R1.5: coalesce identical
cols/rows within 50 ms) to avoid issuing redundant `resize-window` calls.

---

## 4. iOS Safari Clipboard API

### `navigator.clipboard.writeText()` requires a synchronous user gesture

On iOS Safari, `clipboard.writeText()` must be called **synchronously within the call stack of a
user gesture event handler** (touch, click, pointerup). The browser considers the user activation
"consumed" after any `await` or microtask yield. Once the activation is consumed, subsequent
clipboard calls throw `NotAllowedError: The request is not allowed by the user agent or the
platform in the current context`.

**Current code violations**:
- `XtermTerminal.tsx` line 267: `navigator.clipboard.writeText(selection).catch(() => {})` is
  called inside `terminal.onSelectionChange` — this is an xterm.js event, not a direct DOM user
  gesture. iOS Safari does not recognize internal xterm events as user gestures. This will fail
  silently on iOS.
- `TerminalOutput.tsx` line 789: `handleCopyOutput` calls `navigator.clipboard.writeText` — this
  is triggered by a button click, which is a valid user gesture. However, if the button click
  handler does any async work (e.g., fetching the selection from an async source) before the
  clipboard call, it will fail.
- `handlePaste` (line 802) uses `await navigator.clipboard.read()` — this is async after a user
  tap. On iOS Safari, `clipboard.read()` and `clipboard.readText()` may succeed if the clipboard
  permission is pre-granted, but the async context chain from button tap → `handlePaste` →
  `await clipboard.read()` may lose the gesture on some iOS versions.

### The async chain pitfall

The Safari-specific failure pattern is:
```
async function handleCopy() {
  const text = await getSelectionAsync(); // <-- activation consumed here
  await navigator.clipboard.writeText(text); // <-- throws NotAllowedError
}
```
The fix: acquire all data synchronously before the first await. If you must await before writing,
use the `ClipboardItem` + `new Promise` trick to hand Safari a Promise that resolves within the
gesture window:
```js
// Correct pattern for Safari
const text = terminal.getSelection(); // synchronous
await navigator.clipboard.writeText(text); // call immediately, no prior await
```

### WKWebView (Capacitor/Cordova)

In WKWebView (the iOS wrapper used by Capacitor, Cordova, and all iOS browsers), the Clipboard API
is blocked by default unless the app explicitly sets `WKWebViewConfiguration` to allow it and the
view is configured with `allowsInlineMediaPlayback`. Many Capacitor apps must use
`Capacitor.Plugins.Clipboard` instead of the web `navigator.clipboard` API.

Additionally, `navigator.clipboard` may be `undefined` in older WKWebView contexts (iOS < 16) or
in non-HTTPS iframes. The code must guard: `if (navigator.clipboard?.writeText)`.

### Fallback chain

The recommended fallback for iOS clipboard copy:
1. `navigator.clipboard.writeText(text)` — works in Safari 13.1+ if triggered synchronously from
   a user gesture.
2. `document.execCommand('copy')` — deprecated but still works in older iOS WebViews; requires
   that the text be in a selected `<input>` or `<textarea>` element.
3. Display the text in a modal with `<input readonly>` and instruct the user to use the system
   long-press "Copy" on the text field.

---

## 5. iOS Safari Touch Events vs Pointer Events

### Conflict between `useTouchScroll` and `useMobileTerminalGestures`

Both hooks are active simultaneously on the same container element. They both handle `touchmove`:

| Hook | touchmove behavior |
|---|---|
| `useTouchScroll` | Calls `terminal.scrollLines()` and `event.preventDefault()` for vertical swipes |
| `useMobileTerminalGestures` | Calls `event.preventDefault()` and dispatches synthetic `mousemove` during selection drag |

Because both listeners are registered on the same element with `{ passive: false }` for
`touchmove`, they both receive every `touchmove` event. The execution order is registration order.
In `XtermTerminal.tsx`, `useTouchScroll` is called at line 115 and `useMobileTerminalGestures` at
line 136–141. So `useTouchScroll`'s handler fires first.

**Concurrency trap**: when the long-press timer fires in `useMobileTerminalGestures` and sets
`isSelecting = true`, subsequent `touchmove` events must be handled by the gesture hook (synthetic
`mousemove`) and NOT by `useTouchScroll` (scroll). But `useTouchScroll` has no visibility into
`isSelecting`. It will call `terminal.scrollLines()` on every vertical touchmove regardless of
whether a selection drag is in progress — scrolling the terminal while the user is trying to extend
a selection. This is the conflict described in Problem 4 / R4.3.

### `preventDefault()` on iOS 15+: the passive listener breakage

In iOS 15, browsers changed behavior: `touchmove` listeners registered as `{ passive: true }`
cannot call `preventDefault()`. If called, the browser emits a console warning but otherwise
ignores it. Both hooks currently register `touchmove` with `{ passive: false }` — correct for
`preventDefault()` to work. However, if either hook is refactored to `{ passive: true }` for
performance, `preventDefault()` calls inside it will silently stop working, allowing the browser
to scroll the page under the terminal container — a regression that is hard to notice in desktop
testing.

### `preventDefault()` blocking scroll breaks standard iOS scroll behavior

When `useTouchScroll` calls `event.preventDefault()` on every vertical touchmove (line 44–48 of
`useTouchScroll.ts`), it prevents the native browser momentum scroll (deceleration/bounce) from
working. iOS uses its own momentum physics after a fast swipe; `preventDefault()` cancels the
native scroll gesture, meaning the terminal only scrolls by the exact delta of each `touchmove`
event without momentum. Users experience "sticky" scroll with no inertia. This is expected for a
terminal (where xterm.js handles scroll), but care must be taken: if the `scrollLines` call is
slightly off in granularity, the terminal can stutter.

### Pointer events vs Touch events on iOS

iOS Safari supports both `TouchEvent` and `PointerEvent`. For the tap-to-cursor feature (R4.1),
using `PointerEvent` (`pointerdown`, `pointerup`) is preferred over `TouchEvent` because:
- `PointerEvent.clientX/Y` is consistently available at the point of the event; `Touch.clientX/Y`
  from `TouchEvent.changedTouches` can be stale if the event pool is recycled (Safari recycles
  Touch objects). The comment at line 74 of `useMobileTerminalGestures.ts` already documents this:
  "Touch objects are event-scoped and may be recycled/invalid after the handler returns."
- `PointerEvent` has a `pressure` field that can distinguish pen input from touch, which may be
  useful for future stylus support.
- However: `pointercancel` fires on iOS when a scroll gesture is detected, cancelling a
  `pointerdown` that was tracking a tap. This means a long press implemented with `PointerEvent`
  must tolerate `pointercancel` as a timer-cancel signal.

### Known iOS selection interaction trap

The current long-press selection path dispatches synthetic `MouseEvent` to `.xterm-screen` (lines
86–89 of `useMobileTerminalGestures.ts`). xterm.js handles these events internally and triggers
its selection logic. However, when `mouseTracking` is not `'none'` (e.g., during a Claude Code
session that sets `vt200`), xterm.js forwards mouse events to the PTY rather than using them for
selection. The guard at line 84 (`if (getMouseTracking() !== 'none') return`) prevents the
selection dispatch — correct behavior. But the user gets no feedback that selection is unavailable
in `vt200` mode; the long press timer fires and then silently does nothing.

---

## 6. xterm.js Private API Breakage: `_core._renderService.dimensions`

### What version broke it

The path `terminal._core._renderService.dimensions.css.cell.height` accesses a deeply private
internal. It has been present in approximately this form since xterm.js 3.x, but the exact shape
has changed at major version boundaries:

- **xterm.js 4.x**: path was `_core._renderService.dimensions.actualCellHeight` (no `.css` nesting).
- **xterm.js 5.x**: path became `_core._renderService.dimensions.css.cell.height` after the
  renderer refactor for WebGL support.
- **@xterm/xterm 6.x** (current: `^6.0.0` per package.json): The `_renderService` may not exist
  until after the first `requestAnimationFrame` following `open()`. If accessed synchronously
  inside `open()` or before the first render frame, it returns `undefined`, causing
  `undefined.css.cell.height` → TypeError.

The VS Code repository has a documented issue (microsoft/vscode #304945) confirming that
`_core._renderService.dimensions` can be `undefined` in the `@xterm/xterm` 6.x series.

### The current code pattern and its failure modes

In `XtermTerminal.tsx` lines 215–218:
```ts
const dims = (terminal as any)._core?._renderService?.dimensions;
if (dims?.css?.cell) { ... }
```
The optional chaining guards against `undefined` — this is correct defensive coding. It will not
throw. However, if `dims` is `undefined` (not yet initialized), the code falls through to the
`console.warn` and the cell dimensions are never logged. More critically, `TerminalOutput.tsx`
line 447 performs the same access:
```ts
const cell = (xtermRef.current?.terminal as any)?._core?._renderService?.dimensions?.css?.cell;
```
This is used to save cell dimensions to `localStorage` for pre-sizing. If it returns `undefined`
(which it will before the first render frame completes), the cache entry is saved without
`cellWidth`/`cellHeight`, and the pre-sizing optimization never activates for that session.

### Public API alternative

Requirement R3.4 and R4.4 mandate using the public API:
```ts
const estimatedCellHeight = terminal.options.fontSize * (terminal.options.lineHeight ?? 1.0);
```
This is purely a calculation, always available, and never `undefined`. Its accuracy depends on the
font rendering but it is within ~5% of the actual rendered cell height for monospace fonts.

For `FitAddon.proposeDimensions()`, the addon itself computes pixel dimensions internally using
`_core._renderService` — but this is the addon's concern, not the caller's. The addon handles the
version differences internally.

A more accurate (but still non-private) DOM measurement:
```ts
const rowEl = containerEl.querySelector('.xterm-rows > div') as HTMLElement | null;
const actualCellHeight = rowEl?.clientHeight ?? (terminal.options.fontSize * 1.2);
```
This queries the rendered DOM directly; it requires the terminal to be open and at least one row
rendered, but is immune to xterm.js internal restructuring.

---

## 7. Memory with Large Scrollback (5,000 Lines)

### Memory cost

Empirical data from xterm.js issue #791 (Buffer performance improvements, 2017–2018): a 160×24
terminal with 5,000 scrollback lines fully populated uses approximately **34 MB** of JS heap.
This was measured before truecolor support; with truecolor attributes enabled, memory doubles
to ~68 MB. At the time, the dominant cost was the per-cell object (`Cell`) allocated for every
character position in the buffer.

Since xterm.js 4.x, the buffer was rewritten to use `Uint32Array` typed arrays (one 32-bit integer
per cell for character + 8-bit integer for attributes), significantly reducing memory:
- Old (3.x): ~500–700 bytes per row overhead (JS objects).
- New (4.x+): ~(cols × 4 bytes) for the character array + ~(cols × 1 byte) for the flags array.
  For a 220-column terminal (the default in `tmux.go` line 37), that is ~220 × 5 bytes = ~1.1 KB
  per line. For 5,000 lines: **~5.5 MB**. Plus per-line metadata (start/end/length) and the
  attribute spans (variable; can double this for colorful output).

**Practical estimate for this codebase**: with a 220-column terminal, 5,000-line scrollback, and
typical Claude Code colorized output: **10–20 MB** of JS heap for the buffer. This is within the
budget of modern mobile devices (512 MB+ available to the browser tab on iOS 15+).

### Memory leak risk

xterm.js does NOT leak scrollback memory incrementally as new lines are appended. When the buffer
exceeds `scrollback` lines, the oldest lines are dropped (circular buffer semantics). The 5,000
line limit is enforced correctly.

However, there is a **known leak vector**: the `SerializeAddon` (used by the checkpoint feature
if present) serializes the entire buffer to a string. If the serialized string is stored
indefinitely, the buffer is effectively duplicated in heap. Check that serialization is not called
on every frame or on a tight timer.

A second leak vector: the debug monkey-patch in `TerminalStreamManager.installDebugMonitor()`
holds `originalWrite` and `originalRefresh` references on the class. If the terminal is disposed
while the manager is still alive, these references prevent GC of the closed terminal. Ensure
`cleanup()` clears `originalWrite` and `originalRefresh` to `null`.

---

## 8. ConnectRPC WebSocket Scrollback Ordering

### Protocol-level guarantee

ConnectRPC's bidirectional streaming over WebSocket inherits TCP's FIFO delivery guarantee within
a single connection. Messages cannot arrive out-of-order at the framing layer. The server sends
messages sequentially: it writes frame N to the socket before frame N+1, and the client's stream
loop processes them in that order. There is **no interleaving risk at the transport level**.

### Application-level ordering risk

The ordering risk is entirely in the application layer — specifically, the async write path:

**Scenario** (problematic):
1. Server sends `scrollbackResponse` (large, e.g., 500 lines).
2. Server immediately sends `output` message (live bytes from PTY).
3. Client stream loop receives `scrollbackResponse` first (correct), calls `onScrollbackReceived`.
4. `onScrollbackReceived` calls `await manager.writeInitialContent(scrollback)` (async).
5. While `writeInitialContent` is awaiting `enqueueWrite` (chunked, yields to event loop), the
   stream loop continues and receives the `output` message.
6. `onOutput` is called synchronously, passes data to `manager.write()`.
7. `manager.write()` calls `handleProcessedOutput()` which calls `terminal.write()` — the live
   output is written to xterm.js **before** the queued scrollback chunks have been rendered.

Result: the user sees live output appended before history. The buffer order is:
`[live bytes] [500 lines of history]` — history appears below the current prompt.

**The fix** is a write-lock flag on `TerminalStreamManager`:
- Set `isWritingInitialContent = true` at the start of `writeInitialContent`.
- In `write()`, if `isWritingInitialContent`, push to a `pendingLiveWrites` queue.
- In `writeInitialContent`'s finally block, flush `pendingLiveWrites` in order, then set the flag false.

### Scrollback-on-demand (paging) ordering

When the client requests additional scrollback pages (R2.3: scroll near top → request older batch),
the server sends a `scrollbackResponse`. Live output continues to arrive as `output` messages on
the same stream. The same async ordering hazard applies: if the client writes the older batch
asynchronously while live bytes keep arriving synchronously, the older batch ends up appended
*after* the current live position in xterm.js — which is visually incorrect (older content
appearing below newer).

For on-demand paging, the write strategy must be different from `writeInitialContent`. Instead of
clearing and rewriting, the paged batch must be **prepended** to the xterm.js scrollback — which
xterm.js does not support natively. The correct approach is to use `terminal.scrollToLine()` plus
a DOM overlay, or to restructure the session reconnect so that a "scroll-up to load" operation
disconnects, refetches with a larger initial window, and reconnects — avoiding the prepend problem
entirely.

---

## Summary Table

| # | Area | Key Pitfall | Severity |
|---|---|---|---|
| 1 | Scrollback write order | Live bytes interleave with async initial-content write; history appears below live output | High |
| 2 | Resize race | Double `fitAddon.fit()` on mount + 10 ms debounce fires before tmux quiescence | High |
| 3 | tmux `capture-pane` | Alternate screen not captured without `-a`; `-J` strips cursor codes; reflow race after resize | High |
| 4 | iOS clipboard | `onSelectionChange` is not a user gesture on iOS; async await before `writeText` fails | High |
| 5 | Touch conflict | `useTouchScroll` and `useMobileTerminalGestures` both handle `touchmove`; scroll fires during selection drag | High |
| 6 | Private API | `_core._renderService.dimensions` is `undefined` until first render frame in xterm.js 6.x | Medium |
| 7 | Scrollback memory | 5,000 lines ≈ 10–20 MB; no leak from circular eviction; leak risk in SerializeAddon + debug monkey-patch | Low |
| 8 | ConnectRPC ordering | TCP guarantees frame order; app-level async write path loses ordering between `scrollbackResponse` and `output` | High |
