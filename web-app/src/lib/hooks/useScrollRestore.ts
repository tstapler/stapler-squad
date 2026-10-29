import { useCallback, useRef } from "react";

// Module-level so positions survive the scroll container unmounting (mobile
// pane switch unmounts the session list). In-memory only; lost on page reload.
const scrollPositions = new Map<string, number>();

// Virtualized lists size their content a frame or two after mount; until then
// the browser clamps scrollTop, so retry the restore for a bounded time.
const MAX_RESTORE_FRAMES = 30;

/**
 * Returns a callback ref for a scroll container. On attach it restores the
 * last scrollTop saved under `key` (callback refs run in the commit phase,
 * before paint, so there is no flash at 0), retrying per animation frame until
 * the content is tall enough to reach it. While attached it records scrolls,
 * except those caused by the restore itself or by layout clamping, which would
 * overwrite the saved position with a smaller one. Recording on scroll rather
 * than in an unmount cleanup avoids reading scrollTop from a detached element.
 */
export function useScrollRestore(key: string): (el: HTMLElement | null) => void {
  const cleanupRef = useRef<(() => void) | null>(null);

  return useCallback(
    (el: HTMLElement | null) => {
      cleanupRef.current?.();
      cleanupRef.current = null;
      if (!el) return;

      const target = scrollPositions.get(key);
      let restoring = target !== undefined;
      let frame = 0;
      let rafId = 0;

      const settle = () => {
        restoring = false;
        cancelAnimationFrame(rafId);
      };
      const attempt = () => {
        if (!restoring || target === undefined) return;
        el.scrollTop = target;
        if (Math.abs(el.scrollTop - target) < 1 || ++frame >= MAX_RESTORE_FRAMES) {
          settle();
          return;
        }
        rafId = requestAnimationFrame(attempt);
      };
      attempt();

      const onScroll = () => {
        if (!restoring) scrollPositions.set(key, el.scrollTop);
      };
      // A real user gesture means they own the position now; stop restoring.
      const events = ["wheel", "touchstart", "pointerdown", "keydown"] as const;
      events.forEach((e) => el.addEventListener(e, settle, { passive: true }));
      el.addEventListener("scroll", onScroll, { passive: true });

      cleanupRef.current = () => {
        settle();
        events.forEach((e) => el.removeEventListener(e, settle));
        el.removeEventListener("scroll", onScroll);
      };
    },
    [key],
  );
}
