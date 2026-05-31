# Validation Plan: terminal-copy-select

**Date**: 2026-05-30
**Phase**: 4 (Validation)
**Readiness Gate Verdict**: PASS (with noted concerns addressed)

---

## Requirements Traceability Matrix

| Req ID | Requirement | Test ID(s) | Test Name(s) | Type |
|--------|-------------|------------|--------------|------|
| R1 | Selection does NOT cause React re-render | T-UNIT-001, T-UNIT-002 | onSelectionChange_should_NOT_rerender_When_selectionChangesRapidly, floatingButton_should_mutateDOM_NOT_callSetState | Unit |
| R2 | Right-click shows context menu (desktop, no mouse tracking mode) | T-UNIT-003, T-UNIT-004 | contextMenu_should_appear_When_rightClickAndNoMouseTracking, contextMenu_should_preventDefaultBrowserMenu | Unit |
| R3 | Right-click suppressed in mouse tracking mode | T-UNIT-005 | contextMenu_should_NOT_appear_When_mouseTrackingModeActive | Unit |
| R4 | Double-click selects word (xterm built-in still works) | T-UNIT-006 | dblclick_should_triggerWordSelection_When_dispatchedToXtermScreen | Unit |
| R5 | Triple-click selects line (xterm built-in still works) | T-UNIT-007 | tripleClick_should_selectEntireLine_When_dispatchedToXtermScreen | Unit |
| R6 | Ctrl+C with selection copies to clipboard, clears selection, does NOT send to PTY | T-UNIT-008, T-UNIT-009 | ctrlC_should_copyAndClearSelection_When_selectionExists, ctrlC_should_NOT_sendSIGINT_When_selectionExists | Unit |
| R7 | Ctrl+C without selection passes to PTY as SIGINT | T-UNIT-010 | ctrlC_should_passThroughToPTY_When_noSelection | Unit |
| R8 | Cmd+C behaves same as Ctrl+C (Mac users) | T-UNIT-011, T-UNIT-012 | cmdC_should_copyAndClearSelection_When_selectionExists, cmdC_should_passThroughToPTY_When_noSelection | Unit |
| R9 | Ctrl+A / Cmd+A selects all terminal buffer content | T-UNIT-013, T-UNIT-014 | ctrlA_should_callSelectAll_When_pressed, cmdA_should_callSelectAll_When_pressed | Unit |
| R10 | Copy button appears after selection without re-render | T-UNIT-001, T-UNIT-015 | (see R1), copyButton_should_becomeVisible_via_DOM_When_selectionChanges | Unit |
| R11 | Copy button clipboard write works (+ execCommand fallback on iOS) | T-UNIT-016, T-UNIT-017 | copyButton_should_writeToClipboard_When_pointerDown, copyButton_should_fallbackToExecCommand_When_clipboardAPIFails | Unit |
| R12 | Mobile long-press initiates selection (SELECTING state) | T-UNIT-018 | longPress_should_transitionToSELECTING_When_heldFor400ms | Unit |
| R13 | Mobile drag extends selection | T-UNIT-019 | drag_should_extendSelection_When_inSELECTINGState | Unit |
| R14 | Context menu Copy action writes to clipboard | T-UNIT-020 | contextMenuCopy_should_writeToClipboard_When_selectionExists | Unit |
| R15 | Context menu Select All calls terminal.selectAll() | T-UNIT-021 | contextMenuSelectAll_should_callSelectAll | Unit |
| R16 | No regression: scrolling still works | T-UNIT-022, T-INTEG-001 | scroll_should_work_When_noSelectionActive, scrolling_regression | Unit + Integration |
| R17 | No regression: keyboard input passes through | T-UNIT-023 | keyboard_should_passThroughToPTY_When_noShortcutMatch | Unit |
| R18 | No regression: resize/fit unaffected | T-UNIT-024 | resizeObserver_should_NOT_beAffectedBySelectionChanges | Unit |

**Coverage**: 18/18 requirements covered.

---

## Test Cases by Type

### Unit Tests — File: `web-app/src/components/sessions/__tests__/XtermTerminalSelection.test.tsx`

---

#### T-UNIT-001: No re-render on rapid selection changes

**Test name**: `onSelectionChange_should_NOT_rerender_When_selectionChangesRapidly`

**What it tests**: The core regression fix — that calling `onSelectionChange` 10 times in quick succession causes zero additional React renders (the DOM mutation approach does not call setState).

**Mock setup**:
- Full xterm mock harness (see Test Infrastructure section) with `onSelectionChangeCb` capture
- `terminal.getSelection()` returns progressively longer strings
- `terminal.getSelectionPosition()` returns a valid `{start, end}` position
- `terminal.element` is a real DOM element (or mock with `getBoundingClientRect`)

**Assertions**:
```tsx
let renderCount = 0;
function TrackingWrapper() {
  renderCount++;
  return <XtermTerminal />;
}
render(<TrackingWrapper />);
const countAfterMount = renderCount;

act(() => {
  for (let i = 0; i < 10; i++) harness.triggerSelectionChange(`selection ${i}`);
});

expect(renderCount).toBe(countAfterMount); // zero new renders

const copyBtn = document.body.querySelector('[aria-label="Copy selected text"]') as HTMLButtonElement;
expect(copyBtn.style.display).toBe('block'); // button visible via DOM mutation
act(() => { harness.triggerSelectionChange(''); });
expect(copyBtn.style.display).toBe('none');  // button hidden via DOM mutation
expect(renderCount).toBe(countAfterMount);   // still no new renders
```

---

#### T-UNIT-002: Floating button uses direct DOM mutation, not setState

**Test name**: `floatingButton_should_mutateDOM_NOT_callSetState`

**What it tests**: After mount, triggering `onSelectionChange` mutates `copyButtonRef.current.style` directly and does NOT invoke any React state setter.

**Mock setup**: Same as T-UNIT-001.

**Assertions**:
- Spy on `React.useState` to capture call count at mount baseline
- Note: this spy is fragile in React 18 concurrent mode; prefer the render-counter approach (T-UNIT-001) as primary assertion. This test is secondary / complementary.
- Check `copyButtonRef.current.style.display` directly — the portal button in `document.body` becomes `'block'` or `'none'` based on selection state without any re-render.

---

#### T-UNIT-003: Context menu appears on right-click (no mouse tracking)

**Test name**: `contextMenu_should_appear_When_rightClickAndNoMouseTracking`

**What it tests**: A `contextmenu` event on `terminal.element` with `mouseTrackingMode === 'none'` renders the `TerminalContextMenu` component in the document body portal.

**Mock setup**:
- `terminal.modes.mouseTrackingMode = 'none'`
- Dispatch `new MouseEvent('contextmenu', { clientX: 100, clientY: 200, bubbles: true })` on `terminal.element`

**Assertions**:
```tsx
const menu = document.body.querySelector('[data-testid="terminal-context-menu"]');
expect(menu).not.toBeNull();
expect(menu).toBeInTheDocument();
```

---

#### T-UNIT-004: Context menu preventDefault always called

**Test name**: `contextMenu_should_preventDefaultBrowserMenu`

**What it tests**: The `contextmenu` handler always calls `e.preventDefault()` to suppress the browser's native context menu, even in mouse tracking mode.

**Mock setup**: Two sub-cases — tracking mode `'none'` and `'any'`.

**Assertions**:
```tsx
const event = new MouseEvent('contextmenu', { cancelable: true, bubbles: true });
const preventDefaultSpy = jest.spyOn(event, 'preventDefault');
terminal.element.dispatchEvent(event);
expect(preventDefaultSpy).toHaveBeenCalled();
```

---

#### T-UNIT-005: Context menu suppressed in mouse tracking mode

**Test name**: `contextMenu_should_NOT_appear_When_mouseTrackingModeActive`

**What it tests**: With `mouseTrackingMode !== 'none'`, the custom context menu does NOT render (the PTY handles the right-click via VT sequences).

**Mock setup**:
- `terminal.modes.mouseTrackingMode = 'any'` (or `'x10'`, etc.)

**Assertions**:
```tsx
expect(document.body.querySelector('[data-testid="terminal-context-menu"]')).toBeNull();
```

---

#### T-UNIT-006: Double-click triggers word selection (xterm built-in)

**Test name**: `dblclick_should_triggerWordSelection_When_dispatchedToXtermScreen`

**What it tests**: Dispatching a `dblclick` event to `.xterm-screen` within the terminal element invokes xterm's native word selection handler. (For the mobile double-tap path, `useTerminalGestures` dispatches exactly this event.)

**Mock setup**:
- `terminal.element` has a child matching `.xterm-screen`
- Mock `terminal.select` to capture calls (word selection calls this API internally via xterm)
- Alternatively: spy on the synthetic event dispatch to `.xterm-screen`

**Assertions**:
```tsx
// For the mobile double-tap path via useTerminalGestures:
harness.triggerDoubleTap(50, 100); // simulate touch double-tap
const screenEl = terminal.element.querySelector('.xterm-screen');
expect(screenEl).toHaveReceivedEvent('dblclick'); // custom matcher or spy
```
Note: since xterm's word selection is an internal behavior, we test the event dispatch, not the selection result (which would require a real xterm instance).

---

#### T-UNIT-007: Triple-click selects line (xterm built-in)

**Test name**: `tripleClick_should_selectEntireLine_When_dispatchedToXtermScreen`

**What it tests**: A triple-click (three rapid clicks) on the terminal invokes xterm's line selection. xterm v6 handles this natively via `detail: 3` on the `mousedown` event.

**Mock setup**: Spy on event dispatch or mock `terminal.select`.

**Assertions**: Event with `detail: 3` reaches `.xterm-screen` element unchanged — our code does not intercept or modify click events (only `contextmenu` and keyboard events).
```tsx
// Assert that a mousedown event with detail=3 is NOT consumed by XtermTerminal
const event = new MouseEvent('mousedown', { detail: 3, bubbles: true });
const stopPropagationSpy = jest.spyOn(event, 'stopPropagation');
terminal.element.dispatchEvent(event);
expect(stopPropagationSpy).not.toHaveBeenCalled(); // not consumed
```

---

#### T-UNIT-008: Ctrl+C with selection copies to clipboard and clears selection

**Test name**: `ctrlC_should_copyAndClearSelection_When_selectionExists`

**What it tests**: `attachCustomKeyEventHandler` — with `ctrlKey: true, key: 'c'` and non-empty `getSelection()`, the handler copies text to clipboard and calls `terminal.clearSelection()`.

**Mock setup**:
- `harness.selectedText = 'hello world'` (getSelection returns this)
- `navigator.clipboard.writeText` mocked as `jest.fn().mockResolvedValue(undefined)`

**Assertions**:
```tsx
const result = harness.triggerKey({ ctrlKey: true, key: 'c', type: 'keydown' });
expect(navigator.clipboard.writeText).toHaveBeenCalledWith('hello world');
expect(mockTerminal.clearSelection).toHaveBeenCalled();
expect(result).toBe(false); // key consumed (not sent to PTY)
```

---

#### T-UNIT-009: Ctrl+C with selection does NOT send SIGINT to PTY

**Test name**: `ctrlC_should_NOT_sendSIGINT_When_selectionExists`

**What it tests**: Companion to T-UNIT-008 — the `onData` callback (PTY write) must NOT be called when the key is consumed.

**Mock setup**: Same as T-UNIT-008, plus capture `onData` prop.

**Assertions**:
```tsx
expect(capturedOnData).not.toHaveBeenCalled();
```

---

#### T-UNIT-010: Ctrl+C without selection passes through to PTY

**Test name**: `ctrlC_should_passThroughToPTY_When_noSelection`

**What it tests**: With `getSelection()` returning `''`, Ctrl+C handler returns `true` (xterm sends the event to PTY as normal Ctrl+C / SIGINT).

**Mock setup**: `harness.selectedText = ''`

**Assertions**:
```tsx
const result = harness.triggerKey({ ctrlKey: true, key: 'c', type: 'keydown' });
expect(navigator.clipboard.writeText).not.toHaveBeenCalled();
expect(result).toBe(true); // passed to PTY
```

---

#### T-UNIT-011: Cmd+C with selection copies (Mac)

**Test name**: `cmdC_should_copyAndClearSelection_When_selectionExists`

**What it tests**: Same logic as T-UNIT-008 but with `metaKey: true` (macOS Cmd key).

**Mock setup**: `harness.selectedText = 'mac copy test'`, `metaKey: true, key: 'c'`

**Assertions**: Mirror T-UNIT-008 with `metaKey`.

---

#### T-UNIT-012: Cmd+C without selection passes through

**Test name**: `cmdC_should_passThroughToPTY_When_noSelection`

**What it tests**: Mirror of T-UNIT-010 with `metaKey: true`.

---

#### T-UNIT-013: Ctrl+A calls selectAll and consumes event

**Test name**: `ctrlA_should_callSelectAll_When_pressed`

**What it tests**: `ctrlKey: true, key: 'a'` — handler calls `terminal.selectAll()` and returns `false`.

**Mock setup**: Standard harness.

**Assertions**:
```tsx
const result = harness.triggerKey({ ctrlKey: true, key: 'a', type: 'keydown' });
expect(mockTerminal.selectAll).toHaveBeenCalled();
expect(result).toBe(false);
```

---

#### T-UNIT-014: Cmd+A calls selectAll (Mac)

**Test name**: `cmdA_should_callSelectAll_When_pressed`

**What it tests**: Mirror of T-UNIT-013 with `metaKey: true`.

---

#### T-UNIT-015: Copy button position updates via DOM mutation on selection change

**Test name**: `copyButton_should_becomeVisible_via_DOM_When_selectionChanges`

**What it tests**: After a selection change with non-empty text and a valid `getSelectionPosition()`, the portaled copy button element in `document.body` has `style.display = 'block'` and correct `left`/`top` values set by direct DOM mutation.

**Mock setup**:
- `terminal.getSelectionPosition()` returns `{ start: { x: 0, y: 2 }, end: { x: 10, y: 2 } }`
- `terminal.element.getBoundingClientRect()` returns `{ left: 50, top: 100, ... }`
- Mock `getCellDimensions` (or use the real helper if it reads from terminal options)

**Assertions**:
```tsx
const btn = document.body.querySelector('[aria-label="Copy selected text"]') as HTMLButtonElement;
expect(btn.style.display).toBe('block');
expect(btn.style.left).toBeTruthy(); // non-empty positioning set
expect(btn.style.top).toBeTruthy();
```

---

#### T-UNIT-016: Copy button writes to clipboard on pointer down

**Test name**: `copyButton_should_writeToClipboard_When_pointerDown`

**What it tests**: Firing a `pointerdown` event on the floating copy button calls `navigator.clipboard.writeText` with the current selection text, hides the button, and shows the toast.

**Mock setup**:
- Selection text set to `'clipboard test'`
- `navigator.clipboard.writeText` as `jest.fn().mockResolvedValue(undefined)`

**Assertions**:
```tsx
const btn = document.body.querySelector('[aria-label="Copy selected text"]') as HTMLButtonElement;
fireEvent.pointerDown(btn);
expect(navigator.clipboard.writeText).toHaveBeenCalledWith('clipboard test');
await waitFor(() => expect(btn.style.display).toBe('none'));
```

---

#### T-UNIT-017: Copy button falls back to execCommand on clipboard API failure

**Test name**: `copyButton_should_fallbackToExecCommand_When_clipboardAPIFails`

**What it tests**: When `navigator.clipboard.writeText` rejects, the handler falls back to `document.execCommand('copy')`.

**Mock setup**:
- `navigator.clipboard.writeText` as `jest.fn().mockRejectedValue(new DOMException('Permission denied'))`
- `document.execCommand` mocked as `jest.fn().mockReturnValue(true)`

**Assertions**:
```tsx
fireEvent.pointerDown(btn);
await waitFor(() => expect(document.execCommand).toHaveBeenCalledWith('copy'));
```

---

#### T-UNIT-018: Mobile long-press transitions to SELECTING state

**Test name**: `longPress_should_transitionToSELECTING_When_heldFor400ms`

**What it tests**: In `useTerminalGestures.ts`, a touch that holds for 400ms (the `longPressMs` threshold) without moving beyond the scroll threshold transitions the state machine to SELECTING.

**Mock setup**:
- `jest.useFakeTimers()`
- Dispatch `touchstart` on the xterm screen element
- Advance timer by 400ms

**File**: Tests for `useTerminalGestures.ts` can live in `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` (create if not exists) or in `XtermTerminalSelection.test.tsx` via the full component.

**Assertions**:
```tsx
jest.useFakeTimers();
fireEvent.touchStart(screenEl, { touches: [{ clientX: 100, clientY: 200 }] });
act(() => { jest.advanceTimersByTime(401); });
// Assert: terminal.select was called (selection was initiated) OR
// state machine exposed via test hook reports SELECTING
expect(mockTerminal.select).toHaveBeenCalled();
jest.useRealTimers();
```

---

#### T-UNIT-019: Mobile drag extends selection

**Test name**: `drag_should_extendSelection_When_inSELECTINGState`

**What it tests**: After entering SELECTING state (long press), a `touchmove` event extends the selection by calling `terminal.select(...)` with updated end coordinates.

**Mock setup**: Same as T-UNIT-018, after advancing 400ms to reach SELECTING state.

**Assertions**:
```tsx
fireEvent.touchMove(screenEl, { touches: [{ clientX: 150, clientY: 200 }] });
// terminal.select called with wider selection range than initial
expect(mockTerminal.select).toHaveBeenCalledTimes(greaterThan(1));
```

---

#### T-UNIT-020: Context menu Copy writes to clipboard

**Test name**: `contextMenuCopy_should_writeToClipboard_When_selectionExists`

**What it tests**: Clicking the "Copy" menu item in `TerminalContextMenu` calls `navigator.clipboard.writeText` with the current selection.

**Mock setup**:
- Context menu rendered with `hasSelection: true`, selection text = `'menu copy test'`
- `navigator.clipboard.writeText` mocked

**File**: Can be a separate unit test in `web-app/src/components/sessions/__tests__/TerminalContextMenu.test.tsx`.

**Assertions**:
```tsx
fireEvent.click(screen.getByText('Copy'));
expect(navigator.clipboard.writeText).toHaveBeenCalledWith('menu copy test');
```

---

#### T-UNIT-021: Context menu Select All calls terminal.selectAll

**Test name**: `contextMenuSelectAll_should_callSelectAll`

**What it tests**: Clicking "Select All" in the context menu calls the `onSelectAll` callback (which calls `terminal.selectAll()`).

**Mock setup**:
- `onSelectAll` as `jest.fn()`

**Assertions**:
```tsx
fireEvent.click(screen.getByText('Select All'));
expect(onSelectAll).toHaveBeenCalled();
```

---

#### T-UNIT-022: Scrolling unaffected by selection changes

**Test name**: `scroll_should_work_When_noSelectionActive`

**What it tests**: After the ref migration, the `onScroll` behavior (managed by xterm.js internally) is not disrupted. Specifically, no event listeners on the scroll path are mistakenly removed or blocked by the new selection event handling.

**Mock setup**: Standard harness. Dispatch `wheel` event on the xterm screen element.

**Assertions**:
```tsx
const wheelEvent = new WheelEvent('wheel', { deltaY: 100, bubbles: true, cancelable: true });
const stopPropagationSpy = jest.spyOn(wheelEvent, 'stopPropagation');
terminal.element.dispatchEvent(wheelEvent);
expect(stopPropagationSpy).not.toHaveBeenCalled(); // our code doesn't intercept scroll
```

---

#### T-UNIT-023: Non-shortcut keyboard input passes through to PTY

**Test name**: `keyboard_should_passThroughToPTY_When_noShortcutMatch`

**What it tests**: For keys that are NOT Ctrl/Cmd+C or Ctrl/Cmd+A, the custom key handler returns `true` so xterm sends them to the PTY normally.

**Mock setup**: Various key combos (plain `Enter`, `Ctrl+D`, `Alt+F`, `Shift+Tab`).

**Assertions**:
```tsx
expect(harness.triggerKey({ key: 'Enter', type: 'keydown' })).toBe(true);
expect(harness.triggerKey({ ctrlKey: true, key: 'd', type: 'keydown' })).toBe(true);
expect(harness.triggerKey({ altKey: true, key: 'f', type: 'keydown' })).toBe(true);
```

---

#### T-UNIT-024: ResizeObserver behavior unaffected by selection logic

**Test name**: `resizeObserver_should_NOT_beAffectedBySelectionChanges`

**What it tests**: Triggering selection changes does not affect the ResizeObserver / `fitAddon.fit()` path. `fitCalledCount` must remain unchanged after selection events.

**Mock setup**: Full harness including FitAddon mock (reuse `XtermTerminalBug.test.tsx` pattern).

**Assertions**:
```tsx
const fitCountBefore = harness.fitCalledCount;
act(() => {
  for (let i = 0; i < 10; i++) harness.triggerSelectionChange(`text ${i}`);
});
expect(harness.fitCalledCount).toBe(fitCountBefore);
```

---

### Integration Tests — File: `web-app/src/components/sessions/__tests__/XtermTerminalSelection.test.tsx` (additional describes)

---

#### T-INTEG-001: Full selection → copy → clipboard round trip (no re-render)

**Test name**: `selectionCopyRoundTrip_should_writeClipboardAndHideButton_When_complete`

**What it tests**: Simulates the complete user flow: selection drag → button appears (DOM mutation) → pointer down on button → clipboard write → button hidden → toast shown. Verifies no React re-renders happen during the drag phase.

**Mock setup**: Full harness + `navigator.clipboard.writeText` mock.

**Assertions**:
```tsx
const countBefore = renderCount;

// 1. Selection drag (multiple onSelectionChange fires)
act(() => { harness.triggerSelectionChange('line one\nline two'); });
expect(renderCount).toBe(countBefore);
const btn = document.body.querySelector('[aria-label="Copy selected text"]') as HTMLButtonElement;
expect(btn.style.display).toBe('block');

// 2. Copy via button
fireEvent.pointerDown(btn);
expect(navigator.clipboard.writeText).toHaveBeenCalledWith('line one\nline two');
await waitFor(() => expect(btn.style.display).toBe('none'));

// 3. Still no re-renders
expect(renderCount).toBe(countBefore);

// 4. Toast appeared via DOM mutation
const toast = document.body.querySelector('[aria-live="polite"]') as HTMLDivElement;
await waitFor(() => expect(toast.style.display).toBe('block'));
```

---

#### T-INTEG-002: Ctrl+C shortcut round trip

**Test name**: `ctrlCRoundTrip_should_copyAndShowToast_When_selectionExists`

**What it tests**: End-to-end flow of Ctrl+C: selection set → Ctrl+C pressed → clipboard written → selection cleared → toast shown.

**Mock setup**: `harness.selectedText = 'shortcut copy text'`

**Assertions**:
```tsx
harness.triggerKey({ ctrlKey: true, key: 'c', type: 'keydown' });
expect(navigator.clipboard.writeText).toHaveBeenCalledWith('shortcut copy text');
expect(mockTerminal.clearSelection).toHaveBeenCalled();
const toast = document.body.querySelector('[aria-live="polite"]') as HTMLDivElement;
await waitFor(() => expect(toast.style.display).toBe('block'));
```

---

#### T-INTEG-003: Context menu full lifecycle

**Test name**: `contextMenu_should_mountAndDismiss_When_escapePressedOrClickOutside`

**What it tests**: Right-click → menu appears → Escape → menu dismisses. Also: right-click → menu appears → click outside → menu dismisses.

**Mock setup**: `terminal.modes.mouseTrackingMode = 'none'`

**Assertions**:
```tsx
// Appear
fireEvent.contextMenu(terminal.element, { clientX: 50, clientY: 50 });
expect(screen.getByTestId('terminal-context-menu')).toBeInTheDocument();

// Escape dismiss
fireEvent.keyDown(document, { key: 'Escape' });
await waitFor(() => expect(screen.queryByTestId('terminal-context-menu')).not.toBeInTheDocument());

// Reopen + click-outside dismiss
fireEvent.contextMenu(terminal.element, { clientX: 50, clientY: 50 });
fireEvent.mouseDown(document.body);
await waitFor(() => expect(screen.queryByTestId('terminal-context-menu')).not.toBeInTheDocument());
```

---

### E2E Tests — File: `tests/e2e/terminal-copy-select.spec.ts`

---

#### T-E2E-001: Selection and Ctrl+C copy in an active session

**Test name**: `terminal-copy-select / selection_and_keyboard_copy_should_write_to_clipboard`

**What it tests**: Full browser E2E — a real session's terminal allows the user to select text by dragging (no visual re-render / layout shift), and Ctrl+C copies it to the system clipboard.

**Approach**:
- Start a session with `echo "test-clipboard-text"` written to the terminal buffer
- Use Playwright `page.locator('.xterm-screen')` to drag-select text
- Check the selection is visible (terminal has `window.getSelection()` or xterm exposes it)
- Press `Control+c` (Playwright keyboard shortcut)
- Read clipboard: `await page.evaluate(() => navigator.clipboard.readText())`
- Assert clipboard contains the selected text

**Gating note**: Requires `--allow-clipboard` Playwright context flag or browser clipboard permission grant.

---

#### T-E2E-002: Right-click context menu appears

**Test name**: `terminal-copy-select / right_click_should_show_context_menu`

**What it tests**: Right-clicking inside the terminal shows the `TerminalContextMenu` portal with "Copy" and "Select All" options.

**Assertions**:
```
await page.locator('.xterm-screen').click({ button: 'right' });
await expect(page.locator('[data-testid="terminal-context-menu"]')).toBeVisible();
await expect(page.locator('text=Copy')).toBeVisible();
await expect(page.locator('text=Select All')).toBeVisible();
```

---

#### T-E2E-003: Context menu Copy

**Test name**: `terminal-copy-select / context_menu_copy_should_write_selected_text_to_clipboard`

**What it tests**: Select text, right-click, click "Copy" in the context menu, verify clipboard contains selected text.

---

#### T-E2E-004: No re-render during selection drag (performance smoke test)

**Test name**: `terminal-copy-select / drag_selection_should_NOT_cause_visible_layout_shift`

**What it tests**: Using Playwright's layout shift reporting, drag-selecting text should produce zero CLS (Cumulative Layout Shift). Uses `page.evaluate(() => performance.getEntriesByType('layout-shift'))`.

**Assertions**:
```
const cls = await page.evaluate(() => {
  return performance.getEntriesByType('layout-shift')
    .reduce((sum: number, e: any) => sum + e.value, 0);
});
expect(cls).toBe(0);
```

---

## Test Infrastructure Notes

### Shared Xterm Mock Harness

All unit tests in `XtermTerminalSelection.test.tsx` share a mock harness. The harness extends the existing pattern from `XtermTerminalBug.test.tsx`.

**Extended harness interface** (`XtermSelectionTestHarness`):
```ts
interface XtermSelectionTestHarness {
  // Inherited from XtermTerminalBug harness:
  fitCalledCount: number;
  onResizeCb: ((p: { cols: number; rows: number }) => void) | null;
  triggerFit(cols?: number, rows?: number): void;
  reset(): void;

  // New for selection tests:
  onSelectionChangeCb: (() => void) | null;
  customKeyHandler: ((e: KeyboardEvent) => boolean) | null;
  selectedText: string;
  selectionPosition: { start: { x: number; y: number }; end: { x: number; y: number } } | undefined;
  element: HTMLDivElement; // real DOM element with .xterm-screen child

  // Trigger helpers:
  triggerSelectionChange(text?: string): void;
  triggerKey(event: Partial<KeyboardEvent>): boolean | undefined;
}
```

**Mock Terminal additions** (beyond `XtermTerminalBug.test.tsx`):
```ts
class MockTerminal {
  // ... existing fields ...
  modes = { mouseTrackingMode: 'none' as string };

  onSelectionChange(cb: () => void) {
    harness.onSelectionChangeCb = cb;
    return { dispose: jest.fn() };
  }
  getSelection() { return harness.selectedText; }
  getSelectionPosition() { return harness.selectionPosition; }
  clearSelection() { harness.selectedText = ''; }
  selectAll() { harness.selectedText = '[all]'; }
  attachCustomKeyEventHandler(fn: (e: KeyboardEvent) => boolean) {
    harness.customKeyHandler = fn;
  }
  element = (() => {
    const el = document.createElement('div');
    const screen = document.createElement('div');
    screen.className = 'xterm-screen';
    el.appendChild(screen);
    return el;
  })();
}
```

**Trigger helpers**:
```ts
triggerSelectionChange(text = 'selected text') {
  this.selectedText = text;
  this.onSelectionChangeCb?.();
},
triggerKey(event: Partial<KeyboardEvent>): boolean | undefined {
  if (!this.customKeyHandler) return undefined;
  const e = Object.assign(
    new KeyboardEvent(event.type ?? 'keydown', event as KeyboardEventInit),
    event
  );
  return this.customKeyHandler(e);
},
```

### Shared Mocks Required Across All Test Files

All selection test files need the following mocks (in addition to the xterm mock above):

```ts
// navigator.clipboard (set in beforeEach)
Object.defineProperty(navigator, 'clipboard', {
  value: {
    writeText: jest.fn().mockResolvedValue(undefined),
    readText: jest.fn().mockResolvedValue(''),
  },
  configurable: true,
  writable: true,
});

// document.execCommand (fallback path)
Object.defineProperty(document, 'execCommand', {
  value: jest.fn().mockReturnValue(true),
  configurable: true,
  writable: true,
});

// Silence console during tests
jest.spyOn(console, 'log').mockImplementation(() => {});
jest.spyOn(console, 'warn').mockImplementation(() => {});
```

Existing mocks from `XtermTerminalBug.test.tsx` to preserve:
- `@xterm/addon-fit`, `@xterm/addon-search`, `@xterm/addon-web-links`, `@xterm/addon-webgl`
- `@/lib/hooks/useMobileTerminalGestures`
- `@/lib/hooks/useTouchScroll`
- `@/lib/config/terminalConfig`

### TerminalContextMenu Unit Tests

`TerminalContextMenu.test.tsx` can be a simple React Testing Library test for the component in isolation — no xterm mock needed. It renders `<TerminalContextMenu x={50} y={50} hasSelection={true} onCopy={...} onSelectAll={...} onDismiss={...} />` and verifies keyboard/click interactions.

### isMouseTracking Utility Tests

`web-app/src/lib/terminal/__tests__/mouseTracking.test.ts` — simple unit tests for the shared utility function:

| Test | Input | Expected |
|---|---|---|
| Returns false when mode is `'none'` | `{ modes: { mouseTrackingMode: 'none' } }` | `false` |
| Returns false when modes is undefined | `{}` | `false` |
| Returns true when mode is `'any'` | `{ modes: { mouseTrackingMode: 'any' } }` | `true` |
| Returns true when mode is `'x10'` | `{ modes: { mouseTrackingMode: 'x10' } }` | `true` |
| Returns true when mode is `'normal'` | `{ modes: { mouseTrackingMode: 'normal' } }` | `true` |

---

## Readiness Gate

### Criterion 1: Requirements Coverage

**Status: PASS**

All 18 success metrics from `requirements.md` are mapped to at least one test case:
- 24 unit tests × 3 integration tests × 4 e2e tests = 31 total test cases
- Every requirement ID (R1–R18) has at least one T-UNIT entry in the traceability matrix
- No requirement is test-only-via-e2e; every behavioral invariant has a fast unit-level assertion

### Criterion 2: Plan Completeness

**Status: PASS**

- Every in-scope requirement from `requirements.md` has a corresponding Epic/Story/Task in `plan.md`
- All file paths are specified (both modified and new files listed in the File Change List)
- All API names are verified against xterm.js v6 (Technology Validation table in `plan.md` confirms `onSelectionChange`, `getSelection`, `getSelectionPosition`, `select`, `selectAll`, `clearSelection`, `attachCustomKeyEventHandler`, `modes.mouseTrackingMode`)
- `terminal.getSelection().length > 0` used correctly instead of non-public `hasSelection()`
- `allowProposedApi: true` already set at line 149 of `XtermTerminal.tsx` — no change needed

Minor gap noted: plan does not explicitly describe `getCellDimensions` helper (used in Task 1.1.2 for positioning). This appears to be an existing helper in the codebase. Implementer should verify its existence or define it before T-UNIT-015 can be written.

### Criterion 3: Risk Mitigation

**Status: PASS**

All three top pitfalls are addressed:

| Pitfall | Addressed? | Where |
|---|---|---|
| 60fps re-render storm | Yes | Epic 1 replaces useState with ref+DOM mutation; T-UNIT-001 explicitly verifies 0 re-renders for 10 rapid events |
| iOS clipboard failure | Yes | Plan documents the `onPointerDown` pattern as the iOS-safe path; `attachCustomKeyEventHandler` keyboard path limitation explicitly noted in adversarial review Finding 2; plan updated per Finding 2 |
| Mouse tracking mode conflict | Yes | `isMouseTracking()` shared utility (Task 2.1.3); T-UNIT-005 covers the suppression case; T-UNIT-004 covers `preventDefault` always being called |

Additional adversarial review findings are all addressed in plan.md:
- Finding 1 (showToastRef): plan updated to use direct `toastRef` access inside the `useEffect` closure
- Finding 3 (terminal.element availability): plan specifies listener attached after `terminal.open()`
- Finding 4 (vanilla-extract class split): two-class pattern (`copiedToast` + `copiedToastVisible`) explicitly described in Task 1.1.5
- Finding 5 (isMouseTracking duplication): shared utility file added to the File Change List
- Finding 7 (useState spy fragility): plan updated to use render-counter approach (T-UNIT-001 reflects this)
- Finding 8 (useCallback deps): empty dep array specified for `showToast`

### Criterion 4: No Open Blockers

**Status: PASS**

The adversarial review verdict is CONCERNS (not BLOCKED or FAIL). No finding was labeled as a fatal flaw or blocker. All 7 required plan updates in the adversarial review are reflected in the plan's final state:

| Finding | Severity | Patched in plan.md? |
|---|---|---|
| 1: showToastRef complexity | Medium | Yes — removed, direct toastRef access used |
| 2: async clipboard in keydown | Medium | Yes — documented as acceptable limitation (iOS has no Ctrl+C hardware key) |
| 3: element availability | Low | Yes — explicit post-`terminal.open()` placement noted |
| 4: vanilla-extract class split | Medium | Yes — two-class pattern explicit in Task 1.1.5 |
| 5: isMouseTracking duplication | Low | Yes — mouseTracking.ts added to file change list |
| 6: dblclick + rightClickSelectsWord | Low | No action needed (verified non-conflicting) |
| 7: useState spy fragility | Medium | Yes — render counter approach adopted in Task 5.1.6 |
| 8: useCallback deps | Low | Yes — empty dep array specified for showToast |

**One minor open question**: `getCellDimensions` helper is referenced in Task 1.1.2 but not defined in the plan or File Change List. If it does not already exist in `XtermTerminal.tsx`, it must be implemented as part of Epic 1. This is not a blocker (it is straightforward to derive from `terminal.options.fontSize` and `terminal.element`), but the implementer must be aware.

---

## Final Verdict: PASS

The plan is ready for implementation. All requirements are covered by tests, all xterm.js v6 APIs are validated, all adversarial findings are addressed, and no blockers remain. The one minor gap (getCellDimensions helper) should be verified at the start of Epic 1 implementation.
