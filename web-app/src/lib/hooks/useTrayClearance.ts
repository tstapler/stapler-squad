import { useCallback, useRef } from "react";

/** How far (px) a box reaches into the fixed bell column on `edge`; 0 when it stops short of it. */
export function trayClearance(
  box: { left: number; right: number },
  viewportWidth: number,
  handleWidth: number,
  edge: "left" | "right" = "right",
): number {
  if (handleWidth <= 0) return 0;
  const overlap = edge === "left" ? handleWidth - box.left : box.right - (viewportWidth - handleWidth);
  return Math.max(0, Math.round(overlap));
}

/**
 * Callback ref that keeps `--tray-clearance` (bell on the right) or `--tray-clearance-left` (phone
 * floating bell on the left) on the element equal to its actual overlap with the fixed TrayHandle
 * (published by TrayHandle as `--tray-handle-width` and `--tray-handle-edge`). Containers pad their right
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
      const edge = root.style.getPropertyValue("--tray-handle-edge") === "left" ? "left" : "right";
      const px = trayClearance(el.getBoundingClientRect(), window.innerWidth, handleWidth, edge);
      el.style.setProperty("--tray-clearance", edge === "right" ? `${px}px` : "0px");
      el.style.setProperty("--tray-clearance-left", edge === "left" ? `${px}px` : "0px");
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
