# Implementation Plan: terminal-copy-select

**Date**: 2026-05-30
**Phase**: 3 (Planning)
**Branch**: stapler-squad-terminal-copy-select

---

## Technology Validation

All APIs used in this plan are verified against `@xterm/xterm ^6.0.0` (confirmed installed):

| API | Status | Notes |
|---|---|---|
| `terminal.onSelectionChange(cb)` | Public, stable | Fires on every mousemove during drag |
| `terminal.getSelection()` | Public, stable | Synchronous string return |
| `terminal.getSelectionPosition()` | Public, stable | Returns `{start,end}` in cell coords or `undefined` |
| `terminal.select(col, row, length)` | Public, stable | Used already in `useTerminalGestures.ts` |
| `terminal.selectAll()` | Public, stable | Selects full buffer |
| `terminal.clearSelection()` | Public, stable | Clears current selection |
| `terminal.attachCustomKeyEventHandler(fn)` | Public, stable | Return `false` = consume, `true` = pass to xterm |
| `terminal.modes.mouseTrackingMode` | Proposed API (requires `allowProposedApi: true`) | Already enabled at line 149 of XtermTerminal.tsx |
| `terminal.element` | Public, stable | Available after `terminal.open()` |
| `terminal.getSelection().length > 0` | Preferred over `terminal.hasSelection()` | `hasSelection()` is not in the public v6 type definitions; using `getSelection().length > 0` is safe |

**No proposed APIs are required beyond `allowProposedApi: true` which is already set.**

---

## File Change List

### Modified Files
| File | Change |
|---|---|
| `web-app/src/components/sessions/XtermTerminal.tsx` | Remove `copyButtonPos`/`showCopiedToast` useState; add refs + direct DOM mutation; add portal; add custom key handler; add contextmenu listener |
| `web-app/src/components/sessions/XtermTerminal.css.ts` | Replace hardcoded `zIndex: 9999` with `zIndex.floatingTerminalUI` (new named slot); update styles for portaled elements |
| `web-app/src/styles/theme-contract.css.ts` | Add `floatingTerminalUI: 1085` to `zIndex` object (between toast:1080 and tooltip:1100) |
| `web-app/src/lib/hooks/useTerminalGestures.ts` | Add double-tap detection in TAPPING handler; replace private `isMouseTracking` function with import from shared utility |

### New Files
| File | Purpose |
|---|---|
| `web-app/src/components/sessions/TerminalContextMenu.tsx` | Portaled custom right-click context menu |
| `web-app/src/components/sessions/TerminalContextMenu.css.ts` | Vanilla-extract styles for context menu |
| `web-app/src/components/sessions/__tests__/XtermTerminalSelection.test.tsx` | All new selection behavior tests |
| `web-app/src/lib/terminal/mouseTracking.ts` | Shared `isMouseTracking(terminal)` utility (avoids duplication between XtermTerminal and useTerminalGestures) |

---

## Epic 1: Fix the Re-render Bug (Critical Path)

**Goal**: Eliminate React state updates during selection drag. The root cause is `setCopyButtonPos` in `terminal.onSelectionChange` — a setState called at up to 60fps during mouse drag.

### Story 1.1: Replace useState with refs + direct DOM mutation

**Context**: `XtermTerminal.tsx` has two `useState` calls (lines 105–106): `copyButtonPos` and `showCopiedToast`. Only `copyButtonPos` is the hot path (called 60fps during drag). `showCopiedToast` is called once per copy action and can remain as state, BUT the architecture rules require `position: fixed` elements to use `createPortal`.

#### Task 1.1.1: Add refs for the floating button and toast

In `XtermTerminal.tsx`, replace the two `useState` declarations:

```tsx
// REMOVE:
const [copyButtonPos, setCopyButtonPos] = useState<{ x: number; y: number } | null>(null);
const [showCopiedToast, setShowCopiedToast] = useState<'copied' | 'failed' | null>(null);

// ADD:
const copyButtonRef = useRef<HTMLButtonElement>(null);
const toastRef = useRef<HTMLDivElement>(null);
```

Also remove `useState` from the import (it will no longer be used).

#### Task 1.1.2: Replace setCopyButtonPos with direct DOM mutation

In the `selectionDisposable` handler (currently lines 250–265), replace state updates with direct DOM mutations:

```tsx
const selectionDisposable = terminal.onSelectionChange(() => {
  const btn = copyButtonRef.current;
  if (!btn) return;
  const text = terminal.getSelection();
  if (text && text.length > 0) {
    const pos = terminal.getSelectionPosition();
    if (pos && terminal.element) {
      const rect = terminal.element.getBoundingClientRect();
      const { cellH, cellW } = getCellDimensions(terminal);
      btn.style.left = `${rect.left + pos.end.x * cellW}px`;
      btn.style.top = `${rect.top + pos.end.y * cellH - 40}px`;
      btn.style.display = 'block';
    }
  } else {
    btn.style.display = 'none';
  }
});
```

**Why no React state**: DOM mutation via `ref.current.style` is ~0.01ms vs ~3ms for React reconcile. At 60fps this saves ~180ms/s of React work during active drag.

#### Task 1.1.3: Replace setShowCopiedToast with direct DOM mutation

The `showToast` helper function (to be called in onPointerDown/keyboard shortcut handlers). Use `useCallback` with an empty dependency array — it only reads refs, which are stable:

```tsx
const showToast = useCallback((status: 'copied' | 'failed') => {
  const toast = toastRef.current;
  if (!toast) return;
  toast.textContent = status === 'copied' ? 'Copied' : 'Copy failed';
  // Reset animation by removing/re-adding the `copiedToastVisible` class.
  // Two CSS classes: `copiedToast` (base layout) and `copiedToastVisible` (animation only).
  // Adding/removing `copiedToastVisible` restarts the keyframe animation.
  toast.classList.remove(styles.copiedToastVisible);
  toast.style.display = 'block';
  // Force reflow to ensure the class removal is committed before re-adding:
  void toast.offsetHeight;
  toast.classList.add(styles.copiedToastVisible);
  setTimeout(() => {
    if (toastRef.current) {
      toastRef.current.style.display = 'none';
      toastRef.current.classList.remove(styles.copiedToastVisible);
    }
  }, 1500);
}, []); // empty deps — only reads stable refs (toastRef, styles)
```

Replace all `setCopyButtonPos(null)` / `setShowCopiedToast(...)` / `setTimeout(() => setShowCopiedToast(null), 1500)` call sites with:
- `copyButtonRef.current && (copyButtonRef.current.style.display = 'none')` to hide the button
- `showToast('copied')` or `showToast('failed')` for the toast

#### Task 1.1.4: Wrap floating button and toast in createPortal

The CSS architecture rule mandates `createPortal` for `position: fixed` overlays. The button and toast are always rendered (hidden by default) so React never mounts/unmounts them during drag.

In the JSX return, replace the conditional renders:

```tsx
// REMOVE:
{copyButtonPos && (
  <button className={styles.floatingCopyButton} style={{ left: copyButtonPos.x, top: copyButtonPos.y }} ...>
    Copy
  </button>
)}
{showCopiedToast && (
  <div className={styles.copiedToast} ...>
    ...
  </div>
)}

// ADD (always rendered, hidden via display:none):
{createPortal(
  <>
    <button
      ref={copyButtonRef}
      aria-label="Copy selected text"
      className={styles.floatingCopyButton}
      style={{ display: 'none' }}
      onPointerDown={handleCopyButtonPointerDown}
    >
      Copy
    </button>
    <div
      ref={toastRef}
      className={styles.copiedToast}
      aria-live="polite"
      style={{ display: 'none' }}
    />
  </>,
  document.body
)}
```

Extract the `onPointerDown` handler to a `useCallback` named `handleCopyButtonPointerDown`:

```tsx
const handleCopyButtonPointerDown = useCallback((e: React.PointerEvent) => {
  const terminal = terminalRef.current;
  if (!terminal) return;
  const text = terminal.getSelection(); // synchronous within user gesture — iOS safe
  if (!text) return;
  // ... clipboard write logic + copyButtonRef.current.style.display = 'none' + showToast ...
  e.preventDefault();
}, []); // empty deps — only reads stable refs (terminalRef, copyButtonRef, showToast)
        // showToast is defined with useCallback([]) so it is stable
```

`showToast` defined as `useCallback(fn, [])` is stable. `terminalRef` and `copyButtonRef` are refs (stable). The dependency array is empty.

Add `createPortal` to imports: `import { ..., createPortal } from 'react'`.

#### Task 1.1.5: Update XtermTerminal.css.ts for portaled elements

In `XtermTerminal.css.ts`:

1. Add import: `import { zIndex } from '@/styles/theme-contract.css';`
2. Replace hardcoded `zIndex: 9999` in `floatingCopyButton` and `copiedToast` with `zIndex: zIndex.floatingTerminalUI`.
3. Keep `position: 'fixed'` in the class definition; only `left`/`top` are set dynamically via inline style.
4. **Split the toast into two classes** (required for animation restart via DOM class toggling):
   - `copiedToast`: base layout styles (`position: fixed`, `zIndex`, `padding`, `background`, `borderRadius`, etc.) — NO animation here.
   - `copiedToastVisible`: animation only (`animation: \`${fadeInOut} 1.5s ease-in-out forwards\``). This class is added/removed by DOM manipulation in `showToast` to restart the keyframe. Vanilla-extract hashes the class name — use `styles.copiedToastVisible` everywhere consistently.
5. Initial render sets `<div className={styles.copiedToast} style={{ display: 'none' }} />` — the `copiedToastVisible` class is absent until `showToast` is called.

In `web-app/src/styles/theme-contract.css.ts`, add to the `zIndex` object:
```ts
floatingTerminalUI: 1085,  // above toast(1080), below tooltip(1100)
```

### Story 1.2: Verify selection survives re-renders

#### Task 1.2.1: Test that onSelectionChange does NOT call useState setter

In `XtermTerminalSelection.test.tsx`:

```tsx
it('onSelectionChange_should_NOT_callSetState_When_selectionChanges', () => {
  // Spy on React.useState — if called, the test fails
  const useStateSpy = jest.spyOn(React, 'useState');
  // Mount component, capture the onSelectionChange callback from the mock
  // Trigger it multiple times
  // Assert useStateSpy was not called AFTER mount (only called during initial render)
  const callsAfterMount = useStateSpy.mock.calls.length;
  triggerSelectionChange('some text');
  triggerSelectionChange('more text');
  expect(useStateSpy.mock.calls.length).toBe(callsAfterMount); // no new useState calls
});
```

#### Task 1.2.2: Test that rapid selection changes do not re-render

```tsx
it('onSelectionChange_should_NOT_rerender_When_selectionChangesRapidly', () => {
  let renderCount = 0;
  // Wrap component to count renders
  // Trigger 10 rapid selection changes
  // Assert renderCount did not increase
});
```

---

## Epic 2: Right-click Context Menu (Desktop)

**Goal**: Replace the browser native context menu with a custom portaled menu that offers Copy, Select All, and Paste actions. Handle the mouse tracking mode conflict.

### Story 2.1: Custom context menu component

#### Task 2.1.1: Create TerminalContextMenu.tsx + TerminalContextMenu.css.ts

`TerminalContextMenu.tsx`:
- Props: `{ x: number; y: number; hasSelection: boolean; onCopy: () => void; onSelectAll: () => void; onPaste?: () => void; onDismiss: () => void }`
- Rendered via `createPortal(..., document.body)` with `position: fixed` at `{x, y}`
- Menu items: "Copy" (disabled/hidden when `!hasSelection`), "Select All", "Paste" (only when `navigator.clipboard?.readText` is available — omit on iOS where clipboard read is not available)
- Implements keyboard dismiss: `useEffect` attaches `Escape` keydown listener on mount, removes on unmount
- Implements click-outside dismiss: `useEffect` attaches `mousedown` listener on `document` with `{ once: true }`, calls `onDismiss` if click target is outside the menu ref

`TerminalContextMenu.css.ts`:
- Uses vanilla-extract
- `zIndex: zIndex.floatingTerminalUI`
- `position: 'fixed'` (coordinates set via inline style props)
- Menu container: `background: vars.color.cardBackground`, `border: vars.color.borderColor`, `borderRadius: vars.radii.md`, `boxShadow: vars.shadow.lg`
- Menu items: hover highlight, disabled state styling

#### Task 2.1.2: Attach contextmenu listener to terminal.element

Inside the terminal initialization `useEffect` in `XtermTerminal.tsx`, after `terminal.open()`:

```tsx
// Add the contextmenu listener AFTER terminal.open() — terminal.element is guaranteed
// to be non-null at this point. Do not use optional chaining silently here.
const handleContextMenu = (e: MouseEvent) => {
  e.preventDefault(); // suppress browser native menu always (Pitfall 5)
  if (isMouseTracking(terminal)) return; // Task 2.1.3 — let PTY handle right-click
  setContextMenuState({ x: e.clientX, y: e.clientY });
};
// terminal.open() has already run — terminal.element is non-null here
terminal.element!.addEventListener('contextmenu', handleContextMenu);
// cleanup (in useEffect return):
// terminal.element?.removeEventListener('contextmenu', handleContextMenu);
```

**Cleanup note**: The `terminal.element` reference used in `removeEventListener` must be the same element that was used in `addEventListener`. Capture it before the return: `const termElement = terminal.element!;` and use `termElement.removeEventListener(...)` in the cleanup function.

**Architectural note on context menu state**: Unlike the floating copy button (which fires at 60fps during drag), the context menu shows at most once per right-click. Using `useState` for `contextMenuVisible` is acceptable here — the performance constraint only applies to `onSelectionChange`. Add `const [contextMenuState, setContextMenuState] = useState<{x:number;y:number}|null>(null)` for the context menu position.

#### Task 2.1.3: Mouse tracking mode guard

Extract the `isMouseTracking` helper to a shared utility module `web-app/src/lib/terminal/mouseTracking.ts`:

```ts
// web-app/src/lib/terminal/mouseTracking.ts
import type { Terminal } from '@xterm/xterm';

export function isMouseTracking(terminal: Terminal): boolean {
  return (terminal.modes as any)?.mouseTrackingMode !== 'none' &&
         (terminal.modes as any)?.mouseTrackingMode !== undefined;
}
```

Update `useTerminalGestures.ts` to import from this module, replacing the private local function. Import in `XtermTerminal.tsx` as well. This eliminates duplication between the two files (DRY — a single place to update if xterm adds new mode values).

When `isMouseTracking(terminal)` is true: call `e.preventDefault()` (suppress browser menu) but do NOT show the custom menu (let xterm forward the right-click to the PTY as a VT sequence).

#### Task 2.1.4: Dismiss menu on click-outside, Escape, scroll

In `TerminalContextMenu.tsx`:
- Escape: `useEffect` with `keydown` listener on `document`
- Click-outside: `useEffect` with `mousedown` on `document`; check if `event.target` is inside the menu ref
- Scroll: `useEffect` with `scroll` on `window`, `{ passive: true, once: true }` → call `onDismiss`

### Story 2.2: Context menu actions

#### Task 2.2.1: Copy action

```tsx
const handleMenuCopy = useCallback(() => {
  const terminal = terminalRef.current;
  if (!terminal) return;
  const text = terminal.getSelection();
  if (!text) return;
  navigator.clipboard.writeText(text).then(() => showToast('copied')).catch(() => {
    // execCommand fallback
    const ok = execCommandCopy(text);
    showToast(ok ? 'copied' : 'failed');
  });
  setContextMenuState(null);
}, []);
```

Note: context menu copy does NOT use `onPointerDown` — it uses a click event. This means it will fail on iOS Safari if `navigator.clipboard.writeText` is not available synchronously. However, context menus are a desktop feature; on iOS the floating Copy button (which uses `onPointerDown`) is the primary copy mechanism. The `execCommand` fallback provides coverage for non-standard browsers.

#### Task 2.2.2: Select All action

```tsx
const handleMenuSelectAll = useCallback(() => {
  terminalRef.current?.selectAll();
  setContextMenuState(null);
}, []);
```

#### Task 2.2.3: Paste action (desktop only)

```tsx
const handleMenuPaste = useCallback(() => {
  navigator.clipboard.readText().then((text) => {
    onDataRef.current?.(text);
  }).catch(() => {
    // Clipboard read permission denied — silently ignore
  });
  setContextMenuState(null);
}, []);
```

Omit the Paste option when `!navigator.clipboard?.readText` (iOS Safari does not support clipboard read). Check availability at render time in `TerminalContextMenu.tsx`.

---

## Epic 3: Keyboard Shortcuts (Desktop)

**Goal**: Ctrl+C/Cmd+C copies when selection exists (does not send SIGINT); passes through when no selection. Cmd/Ctrl+A selects all without sending to PTY.

### Story 3.1: Ctrl+C / Cmd+C copy-or-SIGINT

#### Task 3.1.1: Add attachCustomKeyEventHandler in terminal initialization useEffect

After `terminal.open()` in the initialization `useEffect`:

```tsx
terminal.attachCustomKeyEventHandler((event: KeyboardEvent): boolean => {
  if (event.type !== 'keydown') return true; // only intercept keydown

  const isCopyShortcut = (event.ctrlKey || event.metaKey) && event.key === 'c';
  const isSelectAllShortcut = (event.ctrlKey || event.metaKey) && event.key === 'a';

  if (isCopyShortcut && terminal.getSelection().length > 0) {
    // Task 3.1.2: has selection → copy to clipboard, clear selection, consume event
    const text = terminal.getSelection();
    // Try navigator.clipboard first; fall back to execCommand (iOS note below).
    // iOS Safari limitation: navigator.clipboard.writeText may fail in a keydown
    // handler because the key event may not carry the same user-gesture trust as a
    // pointer event. This is acceptable — iOS users don't have hardware Ctrl+C;
    // they use the floating Copy button (onPointerDown path) instead.
    navigator.clipboard.writeText(text)
      .then(() => showToastInHandler('copied'))
      .catch(() => {
        const ok = execCommandCopy(text);
        showToastInHandler(ok ? 'copied' : 'failed');
      });
    terminal.clearSelection();
    return false; // prevent xterm from sending to PTY
  }

  if (isSelectAllShortcut) {
    // Task 3.2.1: select all terminal buffer content
    terminal.selectAll();
    return false; // prevent xterm from sending Ctrl+A to PTY
  }

  return true; // pass all other keys through to PTY normally
});
```

**Critical**: `attachCustomKeyEventHandler` does not return a disposable in xterm v6 — it replaces any previously set handler. This means only one handler can be registered. All key intercepts must be in a single handler. The handler is attached once during terminal initialization and does not need cleanup (it's tied to the terminal instance lifecycle).

**Accessing `showToast` inside the handler**: `attachCustomKeyEventHandler` is registered inside the terminal initialization `useEffect`. Since `toastRef` is a React ref (stable identity across renders), it can be read directly inside the handler closure — no intermediate `showToastRef` is needed. Define a local `showToastInHandler` function inside the `useEffect` closure itself, or inline the DOM mutation directly:

```ts
// Inside the terminal initialization useEffect, after terminal.open():
const showToastInHandler = (status: 'copied' | 'failed') => {
  const toast = toastRef.current;
  if (!toast) return;
  toast.textContent = status === 'copied' ? 'Copied' : 'Copy failed';
  toast.classList.remove(styles.copiedToastVisible);
  toast.style.display = 'block';
  void toast.offsetHeight;
  toast.classList.add(styles.copiedToastVisible);
  setTimeout(() => {
    if (toastRef.current) {
      toastRef.current.style.display = 'none';
      toastRef.current.classList.remove(styles.copiedToastVisible);
    }
  }, 1500);
};
```

The closure captures `toastRef` (a ref object with stable identity) — always valid.

#### Task 3.1.2: Selection copy + consume event

Covered in Task 3.1.1 above.

#### Task 3.1.3: Pass-through when no selection

Covered in Task 3.1.1 above (return `true`).

### Story 3.2: Cmd/Ctrl+A select all

#### Task 3.2.1: Select all in custom key handler

Covered in Task 3.1.1 above.

---

## Epic 4: Mobile Touch Selection Enhancements

**Goal**: Floating Copy button works correctly post-ref-migration. Double-tap word selection. Drag handles deferred (see risk register).

### Story 4.1: Floating Copy button (verify no re-render on mobile)

After Epic 1 is complete, the floating Copy button is ref-based and renders via portal. On mobile, `terminal.onSelectionChange` fires when `useTerminalGestures.ts` calls `terminal.select()` programmatically. The ref-based approach works identically for mobile-triggered selection — no additional changes needed.

**Verification**: manually test on iOS Safari that:
1. Long-press triggers selection
2. Copy button appears (visible via DOM inspection) without React re-render
3. Tapping Copy button writes to clipboard via the `onPointerDown` handler

The iOS clipboard write in `handleCopyButtonPointerDown` uses `navigator.clipboard.writeText()` inside `onPointerDown` — this is the correct synchronous-user-gesture pattern. Preserve it exactly.

### Story 4.2: Selection drag handles

**Status: Deferred to a follow-up PR.** See Risk #2 in the risk register. The mobile drag handle implementation requires a custom selection API that extends beyond the current `terminal.select(col, row, length)` — specifically, re-querying the selection position after a handle drag to compute the new `(col, row, length)` tuple. This is non-trivial and carries significant test surface. The floating Copy button already provides adequate mobile copy UX.

### Story 4.3: Double-tap word selection (mobile)

#### Task 4.3.1: Double-tap detection in useTerminalGestures TAPPING handler

In `useTerminalGestures.ts`, add double-tap tracking variables near the other state variables:

```ts
let lastTapTime = 0;
let lastTapX = 0;
let lastTapY = 0;
const DOUBLE_TAP_MS = 300;
const DOUBLE_TAP_RADIUS_PX = 20;
```

In the TAPPING handler in `onTouchEnd`:

```ts
if (state === 'PENDING' && totalDy < 8 && elapsed < longPressMsRef.current) {
  clearLongPressTimer();
  state = 'TAPPING';

  const now = Date.now();
  const isDoubleTap =
    (now - lastTapTime) < DOUBLE_TAP_MS &&
    Math.abs(tapX - lastTapX) < DOUBLE_TAP_RADIUS_PX &&
    Math.abs(tapY - lastTapY) < DOUBLE_TAP_RADIUS_PX;

  if (isDoubleTap && !isMouseTracking()) {
    // Double-tap: select word under tap position using synthetic dblclick
    getScreenEl()?.dispatchEvent(new MouseEvent('dblclick', {
      clientX: tapX,
      clientY: tapY,
      bubbles: true,
      cancelable: true,
      button: 0,
      buttons: 1,
      detail: 2,
    }));
  } else if (!isDoubleTap) {
    // Normal tap handling (existing logic)
    // ...
  }

  lastTapTime = now;
  lastTapX = tapX;
  lastTapY = tapY;

  state = 'IDLE';
  return;
}
```

**Why synthetic dblclick**: xterm.js v6 has native double-click word selection already implemented in its mouse event handler on `.xterm-screen`. Dispatching a synthetic `dblclick` to `.xterm-screen` triggers xterm's own word selection logic — no need to re-implement word boundary detection.

---

## Epic 5: Tests

**Goal**: Regression coverage for all new behavior. All tests go in `web-app/src/components/sessions/__tests__/XtermTerminalSelection.test.tsx`.

**Test infrastructure**: Reuse the xterm mock pattern from `XtermTerminalBug.test.tsx`. The mock's `onSelectionChange` must capture the callback so tests can trigger it. Extend the mock harness to also capture the `attachCustomKeyEventHandler` callback.

### Story 5.1: Unit tests

The mock harness needs these additions:
```ts
interface XtermTestHarness {
  // existing...
  onSelectionChangeCb: (() => void) | null;
  customKeyHandler: ((e: KeyboardEvent) => boolean) | null;
  selectedText: string;
  triggerSelectionChange(text?: string): void;
  triggerKey(event: Partial<KeyboardEvent>): boolean | undefined;
}
```

#### Task 5.1.1: Ctrl+C with selection → copies, clears, does not send to PTY

```tsx
it('Ctrl+C with selection should copy to clipboard and NOT send SIGINT to PTY', () => {
  // Set harness.selectedText = 'hello world'
  // Trigger keydown for Ctrl+C via harness.triggerKey({ ctrlKey: true, key: 'c', type: 'keydown' })
  // Assert: navigator.clipboard.writeText called with 'hello world'
  // Assert: terminal.clearSelection called
  // Assert: return value is false (key consumed)
  // Assert: onData NOT called (SIGINT not sent)
});
```

#### Task 5.1.2: Ctrl+C without selection → passes to PTY

```tsx
it('Ctrl+C without selection should NOT copy and should pass event to PTY', () => {
  // harness.selectedText = '' (no selection)
  // Trigger keydown Ctrl+C
  // Assert: navigator.clipboard.writeText NOT called
  // Assert: return value is true (key passed through)
});
```

#### Task 5.1.3: Cmd+A → selectAll called

```tsx
it('Cmd+A should call terminal.selectAll and consume the event', () => {
  // Trigger keydown { metaKey: true, key: 'a', type: 'keydown' }
  // Assert: terminal.selectAll called
  // Assert: return value is false
});
```

#### Task 5.1.4: Context menu appears on right-click when not in mouse tracking mode

```tsx
it('contextmenu event should show TerminalContextMenu when not in mouse tracking mode', () => {
  // Set harness terminal.modes.mouseTrackingMode = 'none'
  // Trigger contextmenu event on terminal.element
  // Assert: TerminalContextMenu is rendered in document.body portal
  // Assert: e.preventDefault was called
});
```

#### Task 5.1.5: Context menu suppressed when in mouse tracking mode

```tsx
it('contextmenu event should NOT show menu when mouse tracking mode is active', () => {
  // Set harness terminal.modes.mouseTrackingMode = 'any'
  // Trigger contextmenu event
  // Assert: TerminalContextMenu is NOT rendered
  // Assert: e.preventDefault still called (to suppress browser menu)
});
```

#### Task 5.1.6: onSelectionChange does NOT cause re-renders (re-render prevention)

Do NOT use `jest.spyOn(React, 'useState')` — this is fragile in React 18 (concurrent mode may call useState during render preparations). Instead, use a render counter wrapper:

```tsx
it('onSelectionChange_should_NOT_rerender_When_selectionChangesRapidly', () => {
  let renderCount = 0;
  function TrackingWrapper() {
    renderCount++;
    return <XtermTerminal />;
  }
  render(<TrackingWrapper />);
  const countAfterMount = renderCount;

  // Trigger 10 rapid onSelectionChange events via harness
  act(() => {
    for (let i = 0; i < 10; i++) harness.triggerSelectionChange(`selection ${i}`);
  });

  // Assert: no additional renders caused by selection changes
  expect(renderCount).toBe(countAfterMount);

  // Assert: floating button DOM state is correct (ref-based mutation worked)
  const copyBtn = document.body.querySelector('[aria-label="Copy selected text"]') as HTMLButtonElement;
  expect(copyBtn).not.toBeNull();
  // After last selection change with non-empty text, button should be visible
  expect(copyBtn.style.display).toBe('block');

  // Trigger empty selection
  act(() => { harness.triggerSelectionChange(''); });
  expect(copyBtn.style.display).toBe('none');
  // Still no additional renders
  expect(renderCount).toBe(countAfterMount);
});
```

---

## Risk Register

### Risk 1: `attachCustomKeyEventHandler` replaces previous handler silently

**Severity**: High
**Description**: `terminal.attachCustomKeyEventHandler` does not return a disposable and replaces any previously set handler. If any existing code (including addons like `SearchAddon`) also calls `attachCustomKeyEventHandler`, our handler will override it or be overridden.

**Investigation**: Search codebase confirms no current code calls `attachCustomKeyEventHandler` in `XtermTerminal.tsx`. `SearchAddon` uses its own internal keydown listeners, not `attachCustomKeyEventHandler`. Risk is low for the current codebase.

**Mitigation**: Add a comment in the code documenting this constraint. If `SearchAddon` key handling breaks after this PR, the fix is to chain the handlers: capture existing handler before calling `attachCustomKeyEventHandler`, then call the old handler from within the new one.

### Risk 2: Mobile drag handles deferred — selection accuracy on small screens

**Severity**: Medium
**Description**: Story 4.2 (drag handles) is deferred. Without drag handles, mobile users cannot adjust selection boundaries after the initial long-press drag. This is a UX gap vs. native iOS Safari.

**Mitigation**: The floating Copy button provides copy access post-selection. The deferred work is additive (does not regress current behavior). Filed as a follow-up task.

### Risk 3: Context menu clipboard write fails on some browsers without user-gesture context

**Severity**: Low-Medium
**Description**: The context menu Copy action fires from a `click` event on a menu item. On most browsers, this is a user gesture sufficient for `navigator.clipboard.writeText`. However, on some browser configurations (Firefox with strict permissions), clipboard write from a delegated click may be denied. The `execCommand('copy')` fallback covers this case but is deprecated.

**Mitigation**: The `execCommand` fallback is already implemented in the existing `copyText` helper and will be reused. Additionally, the Copy action is most often accessed via the keyboard shortcut (Ctrl+C), which uses `attachCustomKeyEventHandler` where the user gesture is unambiguous. Document the limitation in a code comment.

---

## Implementation Order

1. **Epic 1** (blocking — fixes re-render bug, required before testing anything else)
   - Task 1.1.1 → 1.1.2 → 1.1.3 → 1.1.4 → 1.1.5 in sequence
   - Task 1.2.1, 1.2.2 (tests can be written in parallel with implementation)

2. **Epic 3** (keyboard shortcuts — no UI dependencies, can be implemented immediately after Epic 1)

3. **Epic 2** (context menu — depends on Epic 1 portal infrastructure being in place)

4. **Epic 4** (mobile — Task 4.3.1 is independent of Epics 2/3; Story 4.1 is verification only)

5. **Epic 5** (tests — write alongside each epic, finalize before PR)

---

## Out-of-Scope Cleanup Notes

- `useMobileTerminalGestures.ts` (dead code, Pitfall 8 from research): remove in a separate cleanup PR to avoid scope creep. It is still mocked in `XtermTerminalBug.test.tsx` line 108 — removing the mock must happen simultaneously with removing the import or Jest will throw.
