"use client";

import { useEffect, type RefObject } from "react";
import { setDeckTopOffset, setStackTopOffset } from "@/lib/utils/toastDock";

/**
 * Publishes `getBoundingClientRect().bottom` of the session tab row.
 *
 * The row's own size rarely changes when the memory banner above it appears,
 * wraps or is dismissed; its position does. So this re-measures on the row's and
 * the body's resize, on DOM insertions and removals, and on window resize,
 * coalescing bursts into one frame.
 */
export function usePublishStackTopOffset(ref: RefObject<HTMLElement | null>, enabled = true): void {
  usePublishBottomEdge(ref, enabled, setStackTopOffset, false);
}

/**
 * Publishes the bottom edge of the terminal toolbar for the phone deck. A hidden
 * toolbar (display: none, an inactive pane) measures 0 tall and publishes nothing,
 * so the deck falls back to the tab-row anchor instead of jumping to the screen top.
 */
export function usePublishDeckTopOffset(ref: RefObject<HTMLElement | null>, enabled = true): void {
  usePublishBottomEdge(ref, enabled, setDeckTopOffset, true);
}

function usePublishBottomEdge(
  ref: RefObject<HTMLElement | null>,
  enabled: boolean,
  publish: (px: number | null) => void,
  nullWhenHidden: boolean,
): void {
  useEffect(() => {
    const el = ref.current;
    if (!enabled || !el) return;

    let frame = 0;
    const measure = () => {
      frame = 0;
      const rect = el.getBoundingClientRect();
      publish(nullWhenHidden && rect.height === 0 ? null : Math.round(rect.bottom));
    };
    const schedule = () => {
      if (frame === 0) frame = requestAnimationFrame(measure);
    };

    const resizeObserver = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(schedule);
    resizeObserver?.observe(el);
    resizeObserver?.observe(document.body);
    const mutationObserver = new MutationObserver(schedule);
    mutationObserver.observe(document.body, { childList: true, subtree: true });
    window.addEventListener("resize", schedule);
    measure();

    return () => {
      if (frame !== 0) cancelAnimationFrame(frame);
      resizeObserver?.disconnect();
      mutationObserver.disconnect();
      window.removeEventListener("resize", schedule);
      publish(null);
    };
  }, [ref, enabled, publish, nullWhenHidden]);
}
