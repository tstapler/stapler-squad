# Stack Research: xterm.js v6 APIs for Copy/Select

## xterm.js version
`@xterm/xterm: ^6.0.0` (confirmed in `web-app/package.json`)

## Selection APIs

### `terminal.onSelectionChange(callback)`
- **Fires**: on every mousemove during a selection drag — potentially 60+ times/second.
- **Behavior**: does NOT suppress the default xterm.js selection rendering. xterm.js maintains its own selection layer independently of this callback.
- **React concern**: calling `setCopyButtonPos(...)` inside this callback triggers a React state update on every mousemove frame. This causes XtermTerminal to re-render at drag frequency — a significant performance risk. (Current code already does this at lines 250-265 of XtermTerminal.tsx.)
- **iOS Safari**: the callback is NOT a user gesture. `navigator.clipboard.writeText()` called inside it will be rejected on iOS. The current code acknowledges this with a comment and defers the clipboard write to `onPointerDown`.

### `terminal.getSelection()`
- Returns selected text as a string. Synchronous. Safe to call inside user gesture handlers (e.g., `onPointerDown`). Already used at line 469 of XtermTerminal.tsx.

### `terminal.getSelectionPosition()`
- Returns `{ start: { x, y }, end: { x, y } }` in cell coordinates (0-based columns and rows). Already used at line 253 to compute the floating button position.
- Returns `undefined` when there is no selection.

### `terminal.select(col, row, length)`
- Public API. Selects `length` characters starting at `(col, row)`. Used in `useTerminalGestures.ts` (line 126) to programmatically initiate/extend selection when mouse tracking mode is active (bypasses native mouse event routing).

### `terminal.selectAll()`
- Not explicitly used in this codebase. Available as a public API. Selects all content in the buffer.

### `terminal.clearSelection()`
- Not explicitly used. Available as a public API.

## Mouse Tracking Mode

### `terminal.modes.mouseTrackingMode`
- **Runtime-only**: set by PTY escape sequences sent by the running program (vim, tmux, etc.), not by `ITerminalOptions`.
- **Values** (per codebase comments and usage in `useTerminalGestures.ts` line 87-91):
  - `'none'` — no mouse tracking; xterm.js selection and scrolling work normally.
  - Any other value (e.g., `'any'`, `'normalTracking'`, etc.) — mouse events are intercepted by xterm.js and forwarded to the PTY as VT sequences, which suppresses native DOM selection.
- **Access**: read as `(t.modes as any)?.mouseTrackingMode` (requires type cast; `modes` is part of `allowProposedApi`).
- **Note**: the comment at XtermTerminal.tsx line 92 confirms there is no `mouseTracking` in `ITerminalOptions` for xterm.js 6 — it's purely runtime.

## `terminal.element`
- The root DOM element xterm.js creates inside the container div.
- Safe to attach `contextmenu` listeners directly to it. Used in `useTerminalGestures.ts` via `terminal.element.getBoundingClientRect()` (line 148).
- **Not available** until after `terminal.open(containerEl)` is called (effect at line 186).

## `.xterm-screen` element
- The canvas overlay element inside `terminal.element`.
- Can listen to DOM events directly. Used in `useTerminalGestures.ts` via `containerEl.querySelector('.xterm-screen')` for dispatching synthetic mousedown/mousemove/mouseup events (lines 94, 115, 204, 267).
- Required for synthetic selection events when `mouseTrackingMode === 'none'` — attaching to `.xterm-screen` is the correct level.

## `rightClickSelectsWord: true` option
- Set at line 151 of XtermTerminal.tsx. This is a native xterm.js option.
- **What it does**: on right-click, if there is no existing selection, xterm.js selects the word under the cursor. If there is already a selection, it leaves it unchanged.
- **Interaction risk**: when a custom `contextmenu` listener is added, `rightClickSelectsWord` still fires its selection logic before the `contextmenu` event propagates. The two can coexist but care is needed: calling `e.preventDefault()` in the `contextmenu` handler suppresses the browser's native menu but does NOT suppress the xterm word selection.

## WebGL Addon
- Loaded async at lines 165-182 (dynamic import). Does not affect selection behavior — it only replaces the canvas renderer. Selection is handled by a separate DOM layer (`.xterm-selection` div), not the WebGL canvas.

## SearchAddon
- Loaded at line 157. Has its own internal selection state that is separate from `terminal.getSelection()`. Calling `findNext`/`findPrevious` sets a search highlight that may conflict visually with user selections. No functional conflict with copy/select.

## Addons loaded
- `FitAddon`, `WebLinksAddon`, `SearchAddon`, `SerializeAddon`, `WebglAddon` (async).

## Terminal initialization options relevant to copy/select
```ts
new Terminal({
  allowProposedApi: true,      // required for terminal.modes access
  rightClickSelectsWord: true, // auto word-select on right-click
})
```

## Key gap identified
The `onSelectionChange` callback calls `setCopyButtonPos` on every mousemove during drag. This is a React setState at 60fps inside a xterm event handler — a known performance hazard. A ref-based approach (mutating a DOM node directly) would eliminate these re-renders.
