# Terminal Stack Research: xterm.js API & Browser Terminal Libraries

## 1. Installed xterm.js Version

| Package | Version (package.json spec) | Version (installed) |
|---|---|---|
| `@xterm/xterm` | `^6.0.0` | `6.0.0` |
| `@xterm/addon-fit` | `^0.11.0` | `0.11.0` |
| `@xterm/addon-search` | `^0.16.0` | `0.16.0` |
| `@xterm/addon-serialize` | `^0.14.0` | `0.14.0` |
| `@xterm/addon-web-links` | `^0.12.0` | `0.12.0` |
| `@xterm/addon-webgl` | `^0.19.0` | `0.19.0` |

xterm.js 6.0.0 is the current major release (published early 2024). The `@xterm/` scoped packages replaced the old `xterm` unscoped package.

---

## 2. xterm.js Public API Reference (v6.0.0)

### 2.1 Scrollback Loading

`terminal.write(data: string | Uint8Array, callback?: () => void): void`

- Writing scrollback history **before** `terminal.open()` is NOT supported. The terminal buffer and render pipeline are not initialized until `open()` is called.
- The correct approach to load historical scrollback:
  1. Call `terminal.open(container)` first.
  2. Write historical lines via `terminal.write(scrollbackData)` immediately after open, before streaming live output.
  3. xterm.js processes writes asynchronously (chunked per animation frame). Use the `callback` argument on the final history write to know when it has been parsed and to start live streaming.
- There is **no `prepend` or insert-before-buffer API**. Lines can only be appended to the current scroll position via `write()`. To inject history before live content, write it first (while the buffer is empty) then write live data after.
- xterm.js 6 does not expose `clearScrollback()` as a public method. `terminal.clear()` clears the entire buffer (making the prompt the first line), which is different from clearing only scrollback. There is no public "clear scrollback only" method; the closest approach is `terminal.reset()` (full terminal reset).

### 2.2 Selection Control

All public, stable API (no `allowProposedApi` required):

```ts
terminal.select(column: number, row: number, length: number): void
terminal.getSelection(): string
terminal.getSelectionPosition(): IBufferRange | undefined
terminal.hasSelection(): boolean
terminal.clearSelection(): void
terminal.selectAll(): void
terminal.selectLines(start: number, end: number): void
terminal.onSelectionChange: IEvent<void>  // fires on any selection change
```

`getSelectionPosition()` returns `IBufferRange` with `{ start: { x, y }, end: { x, y } }` in buffer coordinates (1-based). This is the correct way to locate the selection for positioning a floating "Copy" button.

### 2.3 Cell Size / Dimensions

**There is no public cell size API in xterm.js 6.0.0.**

The type declarations confirm that `terminal.options` exposes only the *input* values (`fontSize`, `lineHeight`, `letterSpacing`, etc.), not the computed rendered dimensions. The only public dimension-related surface is:

- `terminal.rows` — number of terminal rows (viewport)
- `terminal.cols` — number of terminal columns (viewport)
- `FitAddon.proposeDimensions(): ITerminalDimensions | undefined` — returns `{ rows, cols }` of what would fit; also only character counts, not pixel values.

The private API currently used in the codebase — `terminal._core._renderService.dimensions.css.cell.height` — is the only way to get actual CSS pixel cell dimensions. This is an internal implementation detail that can break across minor versions.

**Recommended public alternative (as per requirements R3.4 / R4.4):**

```ts
// Approximation using public options:
const cellHeight = terminal.options.fontSize * (terminal.options.lineHeight ?? 1.2);
const cellWidth  = terminal.options.fontSize * 0.6; // monospace approximation
```

This is the approach mandated in R3.4. It is imprecise for non-standard fonts but avoids private API breakage. A more accurate approach: measure the container and divide:

```ts
const container = terminal.element; // public: HTMLElement | undefined
const cellHeight = container.clientHeight / terminal.rows;
const cellWidth  = container.clientWidth  / terminal.cols;
```

This is fully public and accurate after `fit()` has run.

### 2.4 `onSelectionChange` Event

```ts
terminal.onSelectionChange: IEvent<void>
```

Fires with no arguments — the event itself carries no data. You must call `terminal.getSelection()` and `terminal.getSelectionPosition()` inside the handler to read the current state. This is a stable public event (not experimental). Example:

```ts
terminal.onSelectionChange(() => {
  const text = terminal.getSelection();
  const pos  = terminal.getSelectionPosition();
  // show floating Copy button at pos.end if text is non-empty
});
```

### 2.5 `clearScrollback()`

No public `clearScrollback()` method exists. Options:

- `terminal.clear()` — clears both viewport and scrollback, placing cursor at top. Equivalent to the `clear` shell command.
- `terminal.reset()` — full terminal reset (RIS sequence), wipes all state.
- Writing a new snapshot after `clear()` is the correct pattern for "replace buffer with fresh scrollback + live screen" scenarios.

### 2.6 Mouse Tracking Mode (Public API)

**xterm.js 6.0.0 exposes mouse tracking mode via the public `terminal.modes` interface:**

```ts
terminal.modes.mouseTrackingMode: 'none' | 'x10' | 'vt200' | 'drag' | 'any'
```

This is a **stable, public, read-only** property. It reflects the current mode as set by VT escape sequences from the PTY (e.g., Claude Code sets `vt200` via `CSI ? 1000 h`).

**Critical finding**: The current codebase reads `mouseTracking` from a prop/config and tries to set it as a terminal option (`terminal.options.mouseTracking`), but `mouseTrackingMode` in `terminal.modes` is what the PTY controls at runtime. The correct approach for gesture detection is:

```ts
const isMouseTracking = terminal.modes.mouseTrackingMode !== 'none';
```

This replaces the prop-based `getMouseTracking()` callback in `useMobileTerminalGestures.ts`.

---

## 3. Scrollback: Prepend / History Injection

xterm.js has **no prepend-scrollback API**. The buffer is append-only. Historical lines cannot be inserted before existing content.

**Correct architecture for scrollback loading (R2.x):**

1. **On initial connect**: Write historical lines first (before live streaming). Since the buffer starts empty, writing N lines of history then writing current screen content produces the correct order.
2. **On "load more" (scroll-to-top trigger)**: Not directly supportable by prepending. Options:
   - **Option A (recommended)**: Maintain server-side scrollback; on "load more" request, respond with older lines. Write them by doing: save current buffer state with `SerializeAddon`, `terminal.clear()`, write older history, write saved state. This is complex but correct.
   - **Option B**: Use a custom overlay element (e.g., a `<div>` above the terminal canvas) that renders older history as styled HTML text when scrolled above the terminal's top boundary. This is used by some terminal apps (e.g., Warp) but requires tracking the two scroll contexts.
   - **Option C**: Pre-load a larger scrollback on initial connect (R2.3 says 500 lines default). With `scrollback: 5000` in xterm.js, this may be sufficient without needing dynamic loading for most sessions.

The `@xterm/addon-serialize` addon (v0.14.0 installed) can serialize the current terminal buffer to a VT sequence string. This enables Option A: serialize → clear → write-prepend → restore.

---

## 4. Mobile Touch / Addon Support

**There is no official `@xterm/addon-mobile` or touch addon in the xterm.js ecosystem.**

The official addons as of xterm.js 6.x are:
- `addon-fit` — resize to container
- `addon-search` — text search
- `addon-serialize` — buffer serialization
- `addon-web-links` — clickable links
- `addon-webgl` — WebGL renderer
- `addon-canvas` — Canvas renderer (alternative to WebGL)
- `addon-image` — sixel/iTerm2 image protocol (not installed)
- `addon-unicode11` / `addon-unicode-graphemes` — Unicode support

**Mobile touch support must be implemented manually.** The codebase already does this with `useMobileTerminalGestures.ts` and `useTouchScroll.ts`. Key issues with the current implementation:

1. **Two conflicting `touchmove` handlers**: `useTouchScroll.ts` and `useMobileTerminalGestures.ts` both register `touchmove`. `useTouchScroll` uses `passive: false` and calls `preventDefault()`, which will block `useMobileTerminalGestures`'s scroll path since they share the same element. The hooks need to be merged into one unified gesture recognizer (R4.3).

2. **Selection via synthetic mouse events**: Only works when `mouseTrackingMode === 'none'`. When Claude Code sets `vt200` tracking, the selection must use `terminal.select()` directly with pixel-to-cell coordinate math.

3. **Cell height calculation**: Currently uses the private API `terminal._core._renderService.dimensions.css.cell.height`. Must be replaced with `terminal.element.clientHeight / terminal.rows` (public).

4. **No tap-to-cursor support**: A tap must be converted to VT mouse escape sequences when `mouseTrackingMode !== 'none'` (R4.1). xterm.js does not expose an API to do this; the escape sequences must be manually constructed and sent via `terminal.input()` (or the PTY `onData` callback).

---

## 5. FitAddon: Events and Fit Completion

**FitAddon does NOT fire any events.** Its public API is:

```ts
class FitAddon {
  fit(): void;                                      // no return value, no event
  proposeDimensions(): ITerminalDimensions | undefined; // dry-run
  activate(terminal: Terminal): void;
  dispose(): void;
}
```

`fit()` is synchronous: it calls `terminal.resize(cols, rows)` internally. The `terminal.onResize` event fires synchronously during `fit()`. Therefore, to know when fit is "complete" (i.e., xterm.js has updated its col/row count):

```ts
// Listen for resize event to know fit is done
const d = terminal.onResize(({ cols, rows }) => {
  // fit is complete; new dimensions are cols × rows
});
fitAddon.fit();
// After this line, terminal.cols and terminal.rows are already updated
d.dispose();
```

Note: `fit()` does NOT guarantee that the PTY has been notified of the resize or that the render has completed. The `terminal.onResize` callback fires at the point xterm.js updates its internal state, but the WebGL/canvas render happens on the next animation frame.

---

## 6. Mouse Tracking Mode: Public API (Summary)

As documented in section 2.6:

```ts
terminal.modes.mouseTrackingMode  // 'none' | 'x10' | 'vt200' | 'drag' | 'any'
```

This is fully public in xterm.js 6.0.0. The current codebase does NOT use this — it instead passes `mouseTracking` as a component prop and reads it from a ref (`mouseTrackingRef.current`). The prop value reflects the *configured* mode, not the *runtime* mode set by the PTY. When Claude Code starts, it sends `CSI ? 1000 h` which sets `terminal.modes.mouseTrackingMode = 'vt200'`, regardless of what the prop says.

**This is a critical bug**: the gesture handlers check `getMouseTracking() !== 'none'` where `getMouseTracking` returns the configured prop value, not the actual runtime mode. Long-press selection will behave incorrectly because it checks the wrong source of truth.

---

## 7. Alternatives to xterm.js

### hterm (Google)
- Used in Chrome OS terminal, Secure Shell extension.
- Very mature (Google-maintained), but tightly coupled to Chrome OS.
- No npm package; requires bundling from source.
- Mobile touch support: minimal (Chrome OS has keyboard; no mobile touch focus).
- **Verdict**: Not suitable. Poor mobile support, no npm distribution, Google-internal dependency chain.

### terminal.js
- No major production terminal library by this exact name. The npm package `terminal.js` is unmaintained (last publish 2015). Not viable.

### xterminal
- Small hobby project on npm; not production-grade. No meaningful community.

### Hterm alternatives worth considering

**xterm.js remains the clear choice** for this codebase. It is:
- The dominant browser terminal (used by VS Code, JetBrains Fleet, GitHub Codespaces, Gitpod, etc.)
- Actively maintained by the Ptyxis/GNOME team and Microsoft
- Has the `@xterm/addon-webgl` for hardware acceleration
- Version 6.0.0 introduced the `terminal.modes` public API (which solves the mouse tracking problem)

No alternative offers equivalent feature coverage, WebGL acceleration, and active maintenance.

---

## 8. Current Codebase Findings

### XtermTerminal.tsx Issues (against xterm.js 6.0.0 public API)

| Issue | Location | Impact |
|---|---|---|
| `scrollback: 0` (or prop-default `0`) | `XtermTerminal.tsx:96` | **Critical**: Problem 2. xterm.js default scrollback is 1000; setting to 0 disables it. Must be ≥5000. |
| Double `fitAddon.fit()` call | Lines 222–244 | **High**: Problem 1. Initial fit + 100ms delayed fit causes two resize events. The secondary fit at line 239 is the ±1 hack referenced in requirements. |
| Adaptive debounce (10ms for first 3 resizes) | Lines 311–313 | **High**: Problem 1. 10ms is faster than tmux reflow. R1.2 requires 150ms uniform debounce. |
| `(terminal as any).options.mouseTracking` write | Lines 127–129 | **Medium**: `mouseTracking` is not a valid `ITerminalOptions` field. Setting it as an option is a no-op; the actual tracking mode is PTY-driven via VT sequences and read from `terminal.modes.mouseTrackingMode`. |
| Private API `_core._renderService.dimensions.css.cell.height` | `useMobileTerminalGestures.ts:114` | **Medium**: Problem 3/4. Must replace with `terminal.element.clientHeight / terminal.rows`. |
| `useTouchScroll` + `useMobileTerminalGestures` both handle `touchmove` | `XtermTerminal.tsx:115,136` | **Medium**: Problem 4. Conflicting handlers. Must merge (R4.3). |
| `getMouseTracking()` reads prop not `terminal.modes` | `useMobileTerminalGestures.ts:84` | **High**: Long-press selection broken when PTY enables mouse tracking. Must read `terminal.modes.mouseTrackingMode`. |
| No `handleScrollbackReceived()` for historical scrollback | Per requirements doc | **Critical**: Problem 2. R2.7 requires enabling this. |
| `localStorage` cell dimension caching | Per requirements doc | **Medium**: Problem 1. R1.6 requires validation against current font. |

### Key Public API Mapping for Implementation

| Requirement | Private API (current) | Public API (target) |
|---|---|---|
| Cell height for scroll math | `terminal._core._renderService.dimensions.css.cell.height` | `terminal.element!.clientHeight / terminal.rows` |
| Mouse tracking mode check | `mouseTrackingRef.current` (prop) | `terminal.modes.mouseTrackingMode` |
| Selection start/end position for Copy button | N/A (not implemented) | `terminal.getSelectionPosition()` |
| Selection change notification | N/A | `terminal.onSelectionChange` |
| Select text via API | N/A | `terminal.select(col, row, length)` |
| Send mouse click to PTY | N/A | `terminal.input(escapeSequence, false)` |
