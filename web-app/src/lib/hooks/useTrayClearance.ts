import { useCallback, useRef } from "react";

/** How far (px) a box's right edge reaches into the fixed bell column; 0 when it ends before it. */
export function trayClearance(boxRight: number, viewportWidth: number, handleWidth: number): number {
  if (handleWidth <= 0) return 0;
  return Math.max(0, Math.round(boxRight - (viewportWidth - handleWidth)));
}

/**
 * Callback ref that keeps `--tray-clearance` on the element equal to its actual overlap with the
 * fixed TrayHandle (published by TrayHandle as `--tray-handle-width`). Containers pad their right
 * edge by it, so a narrow split pane that never reaches the bell keeps its full width.
 */
export function useTrayClearanceRef(): (el: HTMLElement | null) => void {
  const cleanup = useRef<(() => void) | null>(null);
  return useCallback((el: HTMLElement | null) => {
    cleanup.current?.();
    cleanup.current = null;
    if (!el) return;
    const root = document.documentElement;
    const apply = () => {
      const handleWidth = parseFloat(root.style.getPropertyValue("--tray-handle-width")) || 0;
      const px = trayClearance(el.getBoundingClientRect().right, window.innerWidth, handleWidth);
      el.style.setProperty("--tray-clearance", `${px}px`);
    };
    apply();
    const ro = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(apply);
    ro?.observe(el);
    const mo = new MutationObserver(apply);
    mo.observe(root, { attributes: true, attributeFilter: ["style"] });
    window.addEventListener("resize", apply);
    cleanup.current = () => {
      ro?.disconnect();
      mo.disconnect();
      window.removeEventListener("resize", apply);
    };
  }, []);
}
