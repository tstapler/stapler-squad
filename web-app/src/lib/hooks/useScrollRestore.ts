import { useCallback, useRef } from "react";

// Module-level so positions survive the scroll container unmounting (mobile
// pane switch unmounts the session list). In-memory only; lost on page reload.
const scrollPositions = new Map<string, number>();

/**
 * Returns a callback ref for a scroll container. On attach it restores the
 * last scrollTop saved under `key` (callback refs run in the commit phase,
 * before paint, so there is no flash at 0); while attached it records every
 * scroll. Recording on scroll rather than in an unmount cleanup avoids reading
 * scrollTop from an already-detached element.
 */
export function useScrollRestore(key: string): (el: HTMLElement | null) => void {
  const cleanupRef = useRef<(() => void) | null>(null);

  return useCallback(
    (el: HTMLElement | null) => {
      cleanupRef.current?.();
      cleanupRef.current = null;
      if (!el) return;

      const saved = scrollPositions.get(key);
      if (saved !== undefined) el.scrollTop = saved;

      const onScroll = () => scrollPositions.set(key, el.scrollTop);
      el.addEventListener("scroll", onScroll, { passive: true });
      cleanupRef.current = () => el.removeEventListener("scroll", onScroll);
    },
    [key],
  );
}
