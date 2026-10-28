# Research: Mobile Session List Scroll Position Restore

## Codebase Findings

### How mobile pane switching works

File: `web-app/src/components/pane/PaneSplitRenderer.tsx`

`PaneSplitComponent` (lines 145–200) handles both desktop and mobile. On mobile:
```tsx
if (isMobile) {
  const focusedInFirst = containsPaneId(pane.first, state.focusedPaneId);
  const visibleNode = focusedInFirst ? pane.first : pane.second;
  return <PaneNodeComponent node={visibleNode} ... />;
}
```
Only the focused pane's subtree is rendered. Switching focus via `MobilePaneTabStrip` dispatches `FOCUS_PANE`, which changes `state.focusedPaneId`, causing `visibleNode` to change and React to **unmount** the previous pane and **mount** the new one.

### The session list scroll container

`SessionListPaneBody` (lines 212–317) renders a `<div className={sessionListScroll} data-testid="session-list-scroll">` around both `SessionList` and `SessionBoard`. The `sessionListScroll` style from `paneSplit.css.ts` sets `overflowY: "auto"`.

When this pane unmounts (because focus switches to a detail pane), the DOM node is destroyed. When it remounts, scrollTop = 0.

### Existing scroll wrapper tests

`web-app/src/components/pane/__tests__/SessionListScrollWrapper.test.tsx` — tests that:
- The scroll container `data-testid="session-list-scroll"` exists
- `SessionList` is a descendant of it
- `overflowX` is not set inline
- Non-session-list panes don't get the wrapper

These tests pass with isMobile=true mocked. They don't test scroll-position restoration.

### Pane ID stability

`pane.id` is stable across focus changes — it is assigned when the pane is created and does not change when focus switches. It can safely be used as a key for scroll position storage.

### Session storage conventions

The codebase uses `storageKeyPrefix: \`pane-${pane.id}.\`` for session-level per-pane persistence (passed to `SessionList`). Module-level Maps are used elsewhere for in-memory state that shouldn't cross page reloads.

## Considered Approaches

### Option A: Keep both panes in DOM (display:none for hidden)

Render both `pane.first` and `pane.second` always; hide the non-focused one with CSS (`display:none` or `visibility:hidden`). Scroll position is preserved by the browser automatically.

**Pros:** Zero code in session list; works for all pane content.  
**Cons:** Terminal pane (xterm.js) is mounted while hidden — ResizeObserver fires, WebGL/canvas can be active, memory higher. A hidden terminal pane still processes incoming output. Terminal size may be wrong when hidden (xterm measures container width). Existing tests mock `isMobile=true` and expect only one rendered pane — would need test updates.

### Option B: `useScrollRestore` hook on the scroll container (chosen)

A lightweight hook attached to the `sessionListScroll` div that:
1. On mount: read saved `scrollTop` from a module-level `Map<string, number>` keyed by `pane.id` and apply it synchronously in a `useLayoutEffect`.
2. On scroll: update the map entry (debounced to avoid thrashing).
3. On unmount: write current `scrollTop` to the map (cleanup function).

**Pros:** Surgical — only restores session-list scroll, not terminal/detail. No impact on terminal pane lifecycle. No memory overhead from hidden xterm. Aligns with `storageKeyPrefix` pattern.  
**Cons:** In-memory only (clears on page reload — acceptable per requirements scope). Requires a small new hook and wiring it into `SessionListPaneBody`.

### Option C: Lift scroll position into pane state (Redux/reducer)

Store `scrollTop` in `PaneState` (the pane reducer). Restore on render.

**Cons:** Scroll position is view-only state — putting it in the shared reducer over-engineers the solution. Every scroll event would dispatch an action and potentially cause re-renders in unrelated panes.

## Decision

**Option B** — `useScrollRestore` hook. It is the narrowest, most maintainable fix. The scroll map persists for the session's lifetime, which is exactly the scope the user experiences (they don't expect scroll to survive a page reload).

## Analogous patterns in the codebase

- `storageKeyPrefix: \`pane-${pane.id}.\`` (SessionList persistence): uses pane.id as stable key.
- `useRef<HTMLDivElement>(null)` pattern for DOM refs: already used extensively in `PaneSplitRenderer.tsx`.
- `useLayoutEffect` for DOM measurements: used in the `ResizeObserver` strip-height effect (line 446).
