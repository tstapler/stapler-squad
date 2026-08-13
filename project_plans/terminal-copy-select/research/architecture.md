# Architecture Research: React Rendering and Re-render Avoidance During Selection

## XtermTerminal.tsx useState calls

All `useState` calls in `/web-app/src/components/sessions/XtermTerminal.tsx`:

| Line | State | Type | Re-render trigger |
|------|-------|------|-------------------|
| 105 | `copyButtonPos` | `{ x: number; y: number } \| null` | Set by `onSelectionChange` on every mousemove during drag |
| 106 | `showCopiedToast` | `'copied' \| 'failed' \| null` | Set after clipboard write, then cleared after 1.5s |

**These are the only two useState calls in XtermTerminal.tsx.** All other mutable state uses `useRef`.

## Does re-render affect xterm.js?

When `setCopyButtonPos` is called:
1. React schedules a re-render of `XtermTerminal`.
2. The component function re-runs. `loadTerminalConfig()` is called again (line 85) — reads localStorage. This is a synchronous I/O call per render.
3. The JSX returned includes the `<div ref={containerRef}>` (the xterm container) and the floating button.
4. React does NOT re-initialize xterm.js — the terminal initialization is inside a `useEffect(..., [scrollback])` which only re-runs if `scrollback` changes.
5. However, React does reconcile the DOM. The `<div ref={containerRef}>` is stable, but the conditional `{copyButtonPos && <button ...>}` mounts/unmounts or moves.

**Critical performance risk**: `onSelectionChange` fires on every mousemove during drag (potentially 60fps). Each call to `setCopyButtonPos` triggers a React re-render. During the re-render, `loadTerminalConfig()` reads localStorage, and React reconciles the JSX. This is ~2-5ms per frame, consuming ~12-30% of a 16ms frame budget during active selection drag.

**Does it affect xterm.js rendering?** No — xterm.js renders on its own rAF loop independent of React. But React re-renders during drag can cause jank in the broader UI.

## Parent component: TerminalOutput.tsx

`XtermTerminal` is used in `TerminalOutput.tsx` at line 1439:
```tsx
<XtermTerminal
  ref={xtermRef}
  onData={handleTerminalData}
  onResize={handleTerminalResize}
  theme={theme}
  fontSize={14}
  scrollback={5000}
/>
```

- The `XtermTerminal` component is **not** wrapped in `React.memo` anywhere in the codebase.
- Props passed: `theme`, `fontSize`, `scrollback` are stable (hardcoded or rarely-changing state). `onData` and `onResize` are `useCallback`-stabilized.
- When `setCopyButtonPos` causes `XtermTerminal` to re-render, it does **not** re-render `TerminalOutput` (state is local to `XtermTerminal`).

`TerminalOutput` itself has ~20 `useState` calls (connection state, toolbar state, keyboard state, etc.) and is a large component (~1500 lines). None of those states interact with the selection floating button.

## Options to Eliminate Re-renders During Selection

### Option A: Ref-based DOM mutation (recommended)
Keep the floating button always in the DOM, hidden by default. In `onSelectionChange`, mutate it directly:

```tsx
const floatingCopyButtonRef = useRef<HTMLButtonElement>(null);

// In onSelectionChange:
const btn = floatingCopyButtonRef.current;
if (text && pos && terminal.element) {
  const rect = terminal.element.getBoundingClientRect();
  const { cellH, cellW } = getCellDimensions(terminal);
  btn.style.left = `${rect.left + pos.end.x * cellW}px`;
  btn.style.top = `${rect.top + pos.end.y * cellH - 40}px`;
  btn.style.display = 'block';
} else {
  btn.style.display = 'none';
}
```

**Pros**: Zero React re-renders during drag. Direct DOM mutation is ~0.01ms vs ~3ms for React reconcile.
**Cons**: Bypasses React's declarative model. The button's event handler must be attached in a `useEffect` (or via React's `onPointerDown` if the button is always rendered).

### Option B: React Portal
```tsx
{createPortal(<button ref={floatingCopyButtonRef} className={...} />, document.body)}
```
- Portal renders outside the XtermTerminal tree so it doesn't affect xterm's DOM.
- Still requires `useState` or `useRef` mutation to control visibility/position.
- If using `useState` for position, still triggers re-renders. If using `useRef` + direct style mutation, same as Option A but within a portal.
- **Required by CSS architecture rules** (from `.claude/rules/css-architecture.md`): `position: fixed` modals/overlays MUST use `createPortal` to avoid `transform`/`filter` ancestor interference. The current floating button uses `position: fixed` but is NOT in a portal — this is a bug risk.

### Option C: Sibling React component
Move the floating button to a sibling of `XtermTerminal` that reads from a shared ref:
- `XtermTerminal` writes position to a shared ref.
- A `FloatingCopyButton` component polls/subscribes to the ref.
- Requires some subscription mechanism (e.g., `useSyncExternalStore` with a ref-based store).
- More complex than Option A.

### Option D: CSS-only (not viable)
No CSS selector can react to xterm.js's selection state (it's not exposed as a DOM class or attribute). Not applicable.

## XtermTerminal.css.ts Architecture Notes

The CSS file (`XtermTerminal.css.ts`) uses vanilla-extract (conforming to ADR-009). Key findings:
- `floatingCopyButton` uses `position: fixed` with **hardcoded `zIndex: 9999`** — violates the CSS architecture rule that forbids hardcoded zIndex numbers (should use a named slot in `theme-contract.css.ts`).
- `copiedToast` also uses `position: fixed` with `zIndex: 9999` — same violation.
- Neither uses `createPortal` — violates the CSS architecture rule that `position: fixed` elements must use `createPortal` when inside component trees that may have CSS `transform` ancestors.

## React.memo / useMemo Wrapping
- No `React.memo` wrapping found for `XtermTerminal` in any file.
- `TerminalOutput` renders `XtermTerminal` inside a `<Suspense>` with a lazy import (line 35). The lazy boundary only affects initial load.
- No `useMemo` wrapping of the `<XtermTerminal>` JSX element in `TerminalOutput`.

## Recommended Architecture Change
1. Replace `useState copyButtonPos` with `useRef<HTMLButtonElement>` + direct DOM mutation in `onSelectionChange`.
2. Wrap the floating button in `createPortal(..., document.body)` to escape any `transform` ancestors.
3. Replace hardcoded `zIndex: 9999` with a named token from `theme-contract.css.ts`.
4. Keep `showCopiedToast` as `useState` (only fires once per copy action, not during drag).

This change eliminates all React re-renders during selection drag and fixes two CSS architecture violations.
