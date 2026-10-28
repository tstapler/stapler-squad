# Requirements: Mobile Session List Scroll Position Restore

## Problem Statement

On mobile, every time a user switches between the terminal session view and the session list, the session list resets to the top of its scroll position. This makes it difficult to navigate when the user has many sessions and has scrolled down to find a particular one — switching to a terminal view to check work, then switching back forces them to re-scroll.

## Root Cause (established via code archaeology)

`PaneSplitComponent` in `web-app/src/components/pane/PaneSplitRenderer.tsx` (lines 151–163) uses a conditional render on mobile that shows **only the focused pane**:

```tsx
if (isMobile) {
  const focusedInFirst = containsPaneId(pane.first, state.focusedPaneId);
  const visibleNode = focusedInFirst ? pane.first : pane.second;
  return <PaneNodeComponent node={visibleNode} ... />;
}
```

When the user switches focus from the session-detail pane to the session-list pane (or vice versa), the currently-visible `PaneNodeComponent` is **unmounted** and the other is **mounted fresh**. The `sessionListScroll` div (defined in `SessionListPaneBody`) is a new DOM node with `scrollTop = 0`.

There is no scroll-restoration mechanism anywhere in the codebase — confirmed by searching for `scrollTop`, `scrollPosition`, `useScrollRestore`, `saveScroll`, `restoreScroll`, and `localStorage/sessionStorage` scroll-related keys.

## Scope

- **In scope:** Restoring scroll position of the session list pane on mobile when switching back from a session-detail pane.
- **Out of scope:** Desktop (panes are always visible, no unmount occurs). Board view scroll (separate, less-used). Persisting scroll across full page reloads.

## Acceptance Criteria

1. When a user scrolls the session list on mobile and then switches to a terminal view (session-detail pane), the session list retains its scroll position in memory.
2. When the user switches back to the session list pane, the scroll position is restored to where they left off (±1 row tolerance for fractional positions).
3. The scroll restoration is instantaneous — no visible scroll-jump animation after the pane is visible.
4. If the user navigates to a different session group or the session list is re-filtered, scroll resets to 0 (fresh list).
5. The fix works for both list view and board view scroll within the `sessionListScroll` container.
6. The behavior is covered by at least one automated test (Jest component test or Playwright E2E).
7. No regression in existing `SessionListScrollWrapper.test.tsx` tests.
8. No performance regression — the session-detail terminal pane is not kept in DOM while hidden on mobile.
