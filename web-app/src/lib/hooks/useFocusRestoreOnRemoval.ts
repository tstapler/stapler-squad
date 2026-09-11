import { useEffect, useRef, type MutableRefObject, type RefObject } from "react";

/**
 * Restores keyboard focus to a sensible sibling item when the currently
 * focused item disappears from `ids` (e.g. after auto-resolution or
 * dismissal), falling back to `fallbackRef` when no sibling remains.
 *
 * Callers track which item currently holds focus by wiring the returned ref
 * to each row's `onFocus`/`onBlur` handlers (see ReviewQueuePanel.tsx's
 * `renderQueueItem` and NotificationItem.tsx's `NeedsDecisionSection` for the
 * two call sites this was extracted from).
 *
 * `resolveElement` is looked up fresh inside the effect rather than cached,
 * so it should be stable across renders (e.g. wrapped in `useCallback`) to
 * avoid re-running the effect on every render.
 */
export function useFocusRestoreOnRemoval(
  ids: string[],
  resolveElement: (id: string) => HTMLElement | null,
  fallbackRef: RefObject<HTMLElement | null>
): MutableRefObject<string | null> {
  const previousIdsRef = useRef<string[]>(ids);
  const focusedIdRef = useRef<string | null>(null);

  useEffect(() => {
    const currentIds = ids;
    const currentIdSet = new Set(currentIds);
    const previousIds = previousIdsRef.current;

    const focusedId = focusedIdRef.current;
    let rafId: number | undefined;

    if (focusedId && previousIds.includes(focusedId) && !currentIdSet.has(focusedId)) {
      const removedIndex = previousIds.indexOf(focusedId);
      const remainingIds = previousIds.filter((id) => currentIdSet.has(id));
      const nextFocusId = remainingIds[removedIndex] ?? remainingIds[removedIndex - 1];
      focusedIdRef.current = null;

      // Wait a tick for the removed row's DOM node to actually unmount.
      rafId = requestAnimationFrame(() => {
        const el = nextFocusId ? resolveElement(nextFocusId) : null;
        if (el) {
          el.focus();
        } else {
          fallbackRef.current?.focus();
        }
      });
    }

    previousIdsRef.current = currentIds;

    return () => {
      if (rafId !== undefined) cancelAnimationFrame(rafId);
    };
  }, [ids, resolveElement, fallbackRef]);

  return focusedIdRef;
}
