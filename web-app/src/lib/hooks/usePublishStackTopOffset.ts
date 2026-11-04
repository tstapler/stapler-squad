"use client";

import { useEffect, useMemo, type RefObject } from "react";
import { sharedBodyMeasurer } from "@/lib/layout/sharedBodyMeasurer";
import { setDeckTopOffset, setStackTopOffset } from "@/lib/utils/toastDock";

/**
 * Publishes `getBoundingClientRect().bottom` of the session tab row.
 *
 * The row's own size rarely changes when the memory banner above it appears,
 * wraps or is dismissed; its position does. So this re-measures on the row's and
 * the body's resize, on DOM insertions and removals, and on window resize,
 * coalescing bursts into one frame through the shared body measurer (one set of
 * observers for every subscriber).
 */
export function usePublishStackTopOffset(ref: RefObject<HTMLElement | null>, enabled = true): void {
  usePublishBottomEdge(ref, undefined, enabled, setStackTopOffset, false);
}

/**
 * Publishes the lowest bottom edge of the terminal toolbar and, on a phone, the
 * overflow row under it. A hidden toolbar (display: none, an inactive pane)
 * measures 0 tall and publishes nothing, so the deck falls back to the tab-row
 * anchor instead of jumping to the screen top. Pass `enabled = false` for a
 * terminal that is not the visible one.
 */
export function usePublishDeckTopOffset(
  ref: RefObject<HTMLElement | null>,
  enabled = true,
  extraRef?: RefObject<HTMLElement | null>,
): void {
  const publish = useMemo(() => {
    const owner = Symbol("deck-top-offset");
    return (px: number | null) => setDeckTopOffset(px, owner);
  }, []);
  usePublishBottomEdge(ref, extraRef, enabled, publish, true);
}

function usePublishBottomEdge(
  ref: RefObject<HTMLElement | null>,
  extraRef: RefObject<HTMLElement | null> | undefined,
  enabled: boolean,
  publish: (px: number | null) => void,
  nullWhenHidden: boolean,
): void {
  useEffect(() => {
    const el = ref.current;
    if (!enabled || !el) return;

    const measure = () => {
      const rect = el.getBoundingClientRect();
      const extra = extraRef?.current?.getBoundingClientRect();
      const bottom = extra && extra.height > 0 ? Math.max(rect.bottom, extra.bottom) : rect.bottom;
      publish(nullWhenHidden && rect.height === 0 ? null : Math.round(bottom));
    };
    const elements = extraRef?.current ? [el, extraRef.current] : [el];
    const unsubscribe = sharedBodyMeasurer.subscribe({ elements, measure });
    return () => {
      unsubscribe();
      publish(null);
    };
  }, [ref, extraRef, enabled, publish, nullWhenHidden]);
}
