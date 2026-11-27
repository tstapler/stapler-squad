import { useLayoutEffect, useState } from "react";
import type { RefObject } from "react";

const VIEWPORT_MARGIN = 8;
const ANCHOR_GAP = 4;

/** Top for a fixed menu: below the anchor if it fits, else flipped above it, clamped on-screen. */
export function fitMenuTop(top: number, anchorTop: number, height: number, viewportHeight: number): number {
  if (top + height <= viewportHeight - VIEWPORT_MARGIN) return top;
  return Math.max(VIEWPORT_MARGIN, anchorTop - height);
}

/**
 * Viewport position for a `position: fixed` menu portaled away from its trigger.
 * Re-measured every frame while open: content above the trigger can move it without a
 * scroll or resize event, and a fixed menu left behind cannot be scrolled into view.
 */
export function useAnchoredMenuPosition(
  anchorRef: RefObject<HTMLElement | null>,
  menuRef: RefObject<HTMLElement | null>,
  open: boolean
): { top: number; left: number } {
  const [pos, setPos] = useState({ top: 0, left: 0 });

  useLayoutEffect(() => {
    if (!open) return;
    let frame = 0;
    const place = () => {
      frame = requestAnimationFrame(place);
      const anchor = anchorRef.current?.getBoundingClientRect();
      const menu = menuRef.current?.getBoundingClientRect();
      if (!anchor || !menu) return;
      const { clientWidth, clientHeight } = document.documentElement;
      const fitted = fitMenuTop(anchor.bottom + ANCHOR_GAP, anchor.top - ANCHOR_GAP, menu.height, clientHeight);
      // The anchor itself may be scrolled off-screen, so clamp the bottom edge too.
      const top = Math.max(VIEWPORT_MARGIN, Math.min(fitted, clientHeight - VIEWPORT_MARGIN - menu.height));
      const left = Math.max(VIEWPORT_MARGIN, Math.min(anchor.left, clientWidth - VIEWPORT_MARGIN - menu.width));
      setPos((prev) => (prev.top === top && prev.left === left ? prev : { top, left }));
    };
    place();
    return () => cancelAnimationFrame(frame);
  }, [open, anchorRef, menuRef]);

  return pos;
}
