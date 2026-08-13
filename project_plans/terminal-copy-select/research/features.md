# Features Research: Best-in-Class Web Terminal Copy/Select Patterns

## xterm.js Internal Selection Architecture
xterm.js maintains selection in a **separate DOM layer** (`.xterm-selection` div with RGBA background spans), not on the WebGL/canvas layer. This means:
- Selection is always rendered correctly even with WebGL acceleration active.
- DOM overlays (floating buttons, context menus) placed above the container will render above the canvas correctly.
- The selection layer lives inside `terminal.element`, alongside `.xterm-screen`.

## Current codebase state (what already exists)
- **Floating Copy button**: implemented at XtermTerminal.tsx lines 457-503. Uses `onSelectionChange` to show/hide. Uses `onPointerDown` (not `onClick`) to avoid iOS clipboard timing issues.
- **Toolbar Copy button**: TerminalOutput.tsx line 920-927 has `handleCopyOutput` that calls `getSelection()` and `navigator.clipboard.writeText()`.
- **Mouse mode toggle**: TerminalOutput.tsx line 244-250 has `mouseMode` state ('none'/'any') and a toolbar button to toggle. Mobile defaults to `'none'`, desktop to `'any'`.
- **No `contextmenu` listener**: grep found zero uses of `contextmenu` event in the sessions components. Right-click behavior is entirely controlled by xterm's `rightClickSelectsWord: true`.

## Double-click / Triple-click Selection
xterm.js v6 has **built-in** double-click word selection and triple-click line selection. These are native behaviors — no custom implementation needed. They work when `mouseTrackingMode === 'none'` (desktop without vim/tmux, or when mouse mode is toggled off).

## Ctrl+C Conflict: Copy vs SIGINT
**The problem**: Ctrl+C sends `\x03` (SIGINT) via `terminal.onData`. When the user has selected text, they expect Ctrl+C to copy; when no text is selected, they expect SIGINT.

**Patterns from other terminals**:
- **VS Code integrated terminal**: intercepts `keydown` for Ctrl+C when `terminal.hasSelection()` is true, copies to clipboard, and prevents the default (does NOT send to PTY). When no selection, passes through normally.
- **Hyper / Tabby**: same approach — `keydown` listener checks selection, copies or passes through.
- **ttyd**: uses xterm's `customKeyEventHandler` option which intercepts keys before xterm processes them.

**xterm.js mechanism**: `terminal.attachCustomKeyEventHandler(event => boolean)`. Return `false` to let xterm handle the key (send to PTY); return `true` to suppress xterm's handling. Check `terminal.hasSelection()` inside:
```ts
terminal.attachCustomKeyEventHandler((event) => {
  if (event.type === 'keydown' && event.ctrlKey && event.key === 'c') {
    if (terminal.hasSelection()) {
      navigator.clipboard.writeText(terminal.getSelection());
      terminal.clearSelection();
      return false; // prevent PTY send
    }
  }
  return true; // let xterm handle it
});
```
Note: `terminal.hasSelection()` is not documented in the public API but is present in xterm v6 source. Alternatively check `terminal.getSelection().length > 0`.

**Current state**: the codebase does NOT implement this Ctrl+C duality. TerminalOutput.tsx sends all `onData` through `handleTerminalData` without any Ctrl+C interception.

## Right-click Context Menu Patterns
**Custom DOM menu** (recommended pattern):
1. Listen to `contextmenu` on `terminal.element` (after `terminal.open()`).
2. Call `e.preventDefault()` to suppress the browser native menu.
3. Show a custom `<div>` positioned at `{ clientX, clientY }` with options: Copy, Paste, Select All, Clear.
4. Close on click-outside (`document.addEventListener('click', close, { once: true })`).

**`rightClickSelectsWord: true`** (already set): this fires first (on mousedown) to select the word, before the `contextmenu` event fires. The context menu then appears with the word already selected.

**Current state**: No custom context menu. Right-click selects a word (via xterm option) and then shows the browser's native context menu (which may offer "Copy" depending on browser). No unified UX.

## Floating "Copy" Button — Avoiding React Re-renders
**Current approach** (suboptimal): `setCopyButtonPos` in `onSelectionChange` → React state update → full XtermTerminal re-render at 60fps drag frequency.

**Better approach** (DOM-direct / ref-based):
- Keep a `floatingButtonRef = useRef<HTMLButtonElement>(null)`.
- In `onSelectionChange`, directly mutate `floatingButtonRef.current.style.left/top/display` — no React setState, no re-render.
- The button is always in the DOM but hidden (via `display: none` or `visibility: hidden`).
- This pattern bypasses React's reconciler entirely for the hot path.

**Alternative**: React Portal — `createPortal(<button>, document.body)` renders the button outside the XtermTerminal tree. But the state update still causes re-renders within the portal source component.

## Mobile Selection Patterns

### iOS Safari
- Long-press on text triggers native iOS selection handles (drag handles). However, inside a canvas/WebGL element, the native selection is suppressed. xterm.js requires synthetic mouse events dispatched to `.xterm-screen` (already implemented in `useTerminalGestures.ts`).
- Long-press timer is 400ms (configurable) to match Termux.
- The Copy button must appear after selection ends (touchend), not during drag.

### Android Chrome
- Similar to iOS but PointerEvent is more reliable. The codebase uses TouchEvent exclusively (ADR-012 in `useTerminalGestures.ts` comment) for consistency with iOS.

### Drag handles
- True iOS/Android drag handles (blue resize handles) are not available inside canvas. The floating Copy button serves as the tap-to-copy affordance post-selection.

## Summary of Gaps in Current Implementation
1. **No Ctrl+C interception** — selecting text then pressing Ctrl+C sends SIGINT instead of copying.
2. **No `contextmenu` handler** — right-click shows browser native menu or nothing useful.
3. **Floating Copy button causes 60fps re-renders** during drag (uses React state instead of DOM ref).
4. **No keyboard shortcut** for select-all (Ctrl+A in terminal mode is ambiguous — it sends to PTY).
5. **`useMobileTerminalGestures.ts` is dead code** — replaced by `useTerminalGestures.ts` but not removed.
