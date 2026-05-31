# Pitfalls Research: Known Issues and Gotchas for Terminal Copy/Select

## Pitfall 1: onSelectionChange fires at 60fps during drag → React setState storm

**Confirmed**: `onSelectionChange` fires on every mousemove event while the user drags a selection. The current code at XtermTerminal.tsx line 250-265 calls `setCopyButtonPos` inside this callback. That's a React state update at up to 60fps.

**Performance impact**: each `setState` schedules a synchronous or micro-task re-render. During drag:
- `loadTerminalConfig()` (reads localStorage) is called per render (line 85, function component body).
- React reconciles the entire `XtermTerminal` tree.
- Estimate: ~3-5ms per render × 60fps = ~180-300ms/s of React work during selection drag. At 16ms frame budget this is 18-31% overhead.

**Fix**: Use `useRef` + direct DOM mutation as described in architecture.md. Zero re-renders.

**Existing test evidence**: `XtermTerminalBug.test.tsx` and `TerminalOutputBug.test.tsx` demonstrate the team's awareness of xterm re-render timing issues (Bugs 1, 2, 3). The selection re-render storm is a similar class of problem not yet captured in a test.

## Pitfall 2: iOS Safari clipboard API constraints

**What requires a synchronous user gesture on iOS Safari**:
- `navigator.clipboard.writeText()` — requires the call to happen in the **synchronous** call stack of a trusted user interaction event (click, pointerdown, touchend with no await before the call).
- `document.execCommand('copy')` — same requirement.

**What does NOT count as a user gesture on iOS Safari**:
- `onSelectionChange` callback — this is an xterm.js internal event, not a DOM user gesture. Clipboard writes inside it will throw `NotAllowedError`.
- A Promise `.then()` callback — even if the Promise resolved from a user gesture, the `.then()` is async and loses the gesture context.

**`navigator.clipboard.writeText` availability on iOS Safari 15+**: Yes, it works in iOS Safari 15.0+ but requires HTTPS and a user gesture. The current code already handles this correctly: the clipboard write is in `onPointerDown` (a synchronous user gesture handler at line 466).

**`document.execCommand('copy')` fallback**: still works on iOS Safari as of 2024/2025. The current code uses it as a fallback (lines 470-477). Note: requires creating a `<textarea>`, appending to body, selecting, then `execCommand('copy')` — currently implemented correctly at lines 470-478.

**Gotcha**: `onPointerDown` on the floating Copy button fires BEFORE the button gets focus. The selection should still be preserved at that point because xterm.js does not clear the selection on pointerdown unless the user clicks inside the terminal content area.

## Pitfall 3: React Strict Mode double-invocation

**Risk**: React Strict Mode double-invokes effects (mount → unmount → mount) in development. The terminal initialization effect at XtermTerminal.tsx line 130 has a guard: `if (!containerRef.current || terminalRef.current) return;` — but the cleanup sets `terminalRef.current = null` (line 354). This means:
- First mount: terminal created, refs set.
- Strict Mode unmount: cleanup fires, terminal disposed, `terminalRef.current = null`.
- Second mount: terminal re-created. ✓ (guard allows it)

**Current status**: the guard at line 137 prevents double-init. The double-terminal bug (two xterm instances in the same container) was one of the documented bugs this team has already addressed (see XtermTerminalBug.test.tsx "Bug 1" comments). The Strict Mode case is handled.

**Residual risk for selection**: the `selectionDisposable` (line 250) and the `onPointerDown` handler (line 466) both reference `terminalRef.current`. After Strict Mode remount, both reference the NEW terminal instance correctly (they're in the same effect closure). No selection state is corrupted.

## Pitfall 4: `rightClickSelectsWord: true` interference with custom context menu

**What actually happens**:
1. User right-clicks → mousedown fires → xterm fires `rightClickSelectsWord` logic (selects word if no selection).
2. `onSelectionChange` fires (because a selection was made).
3. `contextmenu` event fires on the element.

If we add a `contextmenu` listener and call `e.preventDefault()`, the browser's native menu is suppressed. The xterm word selection from step 1 **is preserved** — `preventDefault()` on `contextmenu` does not undo the xterm selection.

**Conflict risk**: if a custom context menu calls `terminal.clearSelection()` on dismiss, that's fine. But if the context menu is dismissed by clicking outside (which fires another mousedown on the terminal), xterm may clear the selection before the user can act on the context menu item.

**Recommendation**: show the context menu, let the word remain selected, and only call `terminal.clearSelection()` after the user copies or explicitly dismisses.

## Pitfall 5: Mouse tracking mode conflict with right-click and contextmenu

**When vim/tmux is running**, `terminal.modes.mouseTrackingMode !== 'none'`. In this state:
- xterm.js intercepts right-click (mousedown button=2) and forwards it as a VT mouse sequence to the PTY.
- The `contextmenu` DOM event still fires (it fires after mousedown), but by that point xterm may have consumed the event.
- **Critical**: if we attach `contextmenu` listener and call `e.preventDefault()`, we prevent the browser menu. But the click was already forwarded to vim/tmux. Vim may have acted on it (e.g., right-click paste in some modes).

**Detection**: check `isMouseTracking()` (from `useTerminalGestures.ts` line 87) before showing the custom context menu. If mouse tracking is active, either:
a. Don't show the custom menu (let vim/tmux handle it), or
b. Show a minimal menu without "Paste" (since the click was already sent to the PTY).

**Current codebase**: the `useTerminalGestures` hook already implements this check at line 112-127 for touch selection. The same pattern should be applied for the contextmenu handler.

## Pitfall 6: Canvas/WebGL rendering and DOM overlays

**Fixed-position elements above WebGL canvas**: the floating Copy button uses `position: fixed; z-index: 9999`. This renders correctly above the WebGL canvas on all major browsers. WebGL canvas elements do not create their own stacking context that would block fixed children.

**However**: the CSS architecture rules (`.claude/rules/css-architecture.md`) explicitly state: "Always use `createPortal(..., document.body)` for overlays that must escape the component tree." If any ancestor of `XtermTerminal` applies a CSS `transform`, `filter`, or `will-change`, `position: fixed` will be relative to that ancestor, not the viewport. This is a latent bug.

**Confirmed risk**: `TerminalOutput.tsx` renders `XtermTerminal` inside several `<div>` containers. If any future parent adds `transform` for animations (e.g., slide-in panel), the floating button will snap to the wrong position. Use `createPortal` to be safe.

## Pitfall 7: Existing test patterns and known bugs

From `XtermTerminalBug.test.tsx`:
- **Bug 1** (currently failing tests): `onResize` fires with xterm defaults (80×24) BEFORE `fitAddon.fit()` runs — corrupts dimension cache.
- **Bug 3** (currently failing tests): `ResizeObserver` calls `fit()` on zero-size container → fires `onResize(80, 24)` again.

These bugs affect the dimension cache and connection initialization, not selection. But the test infrastructure (mocking xterm, capturing `onSelectionChange`) is already established.

**Test gap for selection**: there are NO existing tests for:
- `onSelectionChange` callback behavior.
- Floating Copy button show/hide.
- `onPointerDown` clipboard copy.
- Ctrl+C interception.
- Context menu display.

**From `TerminalOutputBug.test.tsx`**: tests for the output queuing (`RESIZING` → `STABLE` flush) and scrollback paging are comprehensive. The test mock for `XtermTerminal` captures `onResize` but not `onSelectionChange` — this mock would need to be extended for selection tests.

## Pitfall 8: `useMobileTerminalGestures` dead code

`useMobileTerminalGestures.ts` exists and is a complete implementation (146 lines). However, `XtermTerminal.tsx` imports and uses `useTerminalGestures` (the newer unified hook) instead. The old hook is not used anywhere in XtermTerminal.tsx anymore. It is **dead code** but still present. The `XtermTerminalBug.test.tsx` mocks it (`jest.mock('@/lib/hooks/useMobileTerminalGestures', ...)`) — suggesting it may still be imported by older test files.

This creates confusion: any future change to selection touch behavior should be in `useTerminalGestures.ts`, not `useMobileTerminalGestures.ts`.

## Summary of Critical Pitfalls

| # | Pitfall | Severity | Current State |
|---|---------|----------|---------------|
| 1 | `onSelectionChange` → 60fps React setState | High | Active (will cause drag jank) |
| 2 | iOS clipboard requires sync user gesture | High | Correctly handled (onPointerDown) |
| 3 | React Strict Mode double-init | Medium | Handled by existing guard |
| 4 | rightClickSelectsWord + custom contextmenu | Medium | No context menu yet — no conflict |
| 5 | Mouse tracking mode + contextmenu conflict | High | Must detect before showing custom menu |
| 6 | position:fixed without createPortal | Medium | Latent bug, violates CSS architecture rules |
| 7 | No selection tests exist | High | Test coverage gap |
| 8 | useMobileTerminalGestures dead code | Low | Cleanup needed to avoid confusion |
