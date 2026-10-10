"use client";

import { useEffect, type RefObject } from "react";
import { setStackTopOffset } from "@/lib/utils/toastDock";

/**
 * Publishes `getBoundingClientRect().bottom` of the session tab row.
 *
 * The row's own size rarely changes when the memory banner above it appears,
 * wraps or is dismissed; its position does. So this re-measures on the row's and
 * the body's resize, on DOM insertions and removals, and on window resize,
 * coalescing bursts into one frame.
 */
export function usePublishStackTopOffset(ref: RefObject<HTMLElement | null>, enabled = true): void {
  useEffect(() => {
    const el = ref.current;
    if (!enabled || !el) return;

    let frame = 0;
    const measure = () => {
      frame = 0;
      setStackTopOffset(Math.round(el.getBoundingClientRect().bottom));
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
      setStackTopOffset(null);
    };
  }, [ref, enabled]);
}
