"use client";

import { useCallback, useRef, type MouseEvent } from "react";
import { getTiming } from "./interactionTiming";

/**
 * Fling guard (design/ux.md 7.1, UX-AC62): a pointer or touch that begins within 120 ms of a
 * `scroll` event only stopped a fling, so it activates nothing and its click is swallowed.
 * Spread the handlers on the scroll region; the capture handlers run before any control's own.
 */
export function useScrollSettle() {
  const lastScrollAt = useRef(Number.NEGATIVE_INFINITY);
  const swallowClick = useRef(false);

  const onScroll = useCallback(() => {
    lastScrollAt.current = Date.now();
  }, []);
  const onPressStart = useCallback(() => {
    swallowClick.current = Date.now() - lastScrollAt.current < getTiming().flingGuardMs;
  }, []);
  const onClickCapture = useCallback((e: MouseEvent) => {
    if (!swallowClick.current) return;
    swallowClick.current = false;
    e.preventDefault();
    e.stopPropagation();
  }, []);

  return { onScroll, onPointerDownCapture: onPressStart, onTouchStartCapture: onPressStart, onClickCapture };
}
