"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import {
  IDLE_SWIPE,
  REVEAL_PX,
  startsInEdgeGuard,
  swipeStep,
  type SwipeSample,
  type SwipeState,
} from "@/lib/hooks/swipeToDismissMachine";

export interface UseSwipeToDismissOptions {
  /** Fired once when a release passes the threshold. */
  onDismiss: () => void;
  /** Turn the gesture off, e.g. while an approval is in flight. */
  disabled?: boolean;
}

export interface SwipeView {
  /** Horizontal translation to apply while the finger is down. */
  offset: number;
  dragging: boolean;
  /** True once the drag is far enough to show the "Dismiss" / "Move to tray" reveal. */
  revealed: boolean;
}

const NO_SWIPE: SwipeView = { offset: 0, dragging: false, revealed: false };

/**
 * Wires touch events on `ref` to the swipe machine.
 *
 * Touch events that start on the row stop propagating at the row, so the terminal
 * gesture machine's document-level touchmove/touchend listeners never see them
 * (AWAITING OPERATOR: Spike 1.2 has not recorded whether that listener double
 * handles such a swipe on a real device; stopping here is the safe default).
 * Vertical scrolling stays with the browser through `touch-action: pan-y` on the row.
 * Callbacks live in refs, so changing them never re-binds the listeners.
 */
export function useSwipeToDismiss(
  ref: RefObject<HTMLElement | null>,
  { onDismiss, disabled = false }: UseSwipeToDismissOptions,
): SwipeView {
  const [view, setView] = useState<SwipeView>(NO_SWIPE);
  const onDismissRef = useRef(onDismiss);
  onDismissRef.current = onDismiss;

  useEffect(() => {
    const el = ref.current;
    if (!el || disabled) return;

    let state: SwipeState = IDLE_SWIPE;
    let touching = false;

    const apply = (sample: SwipeSample) => {
      const step = swipeStep(state, sample);
      state = step.state;
      const dragging = state.phase === "dragging";
      setView({ offset: step.offset, dragging, revealed: dragging && Math.abs(step.offset) >= REVEAL_PX });
      if (step.outcome === "dismiss") onDismissRef.current();
    };

    const point = (e: TouchEvent) => e.touches[0] ?? e.changedTouches[0];

    const onStart = (e: TouchEvent) => {
      const p = point(e);
      if (!p || startsInEdgeGuard(p.clientX, window.innerWidth)) return;
      e.stopPropagation();
      touching = true;
      apply({ type: "start", x: p.clientX, y: p.clientY, t: e.timeStamp, width: el.getBoundingClientRect().width });
    };
    const onMove = (e: TouchEvent) => {
      if (!touching) return;
      e.stopPropagation();
      const p = point(e);
      if (p) apply({ type: "move", x: p.clientX, y: p.clientY, t: e.timeStamp });
    };
    const onEnd = (e: TouchEvent) => {
      if (!touching) return;
      e.stopPropagation();
      touching = false;
      const p = point(e);
      if (p) apply({ type: "end", x: p.clientX, y: p.clientY, t: e.timeStamp });
    };
    const onCancel = (e: TouchEvent) => {
      if (!touching) return;
      e.stopPropagation();
      touching = false;
      apply({ type: "cancel" });
    };

    el.addEventListener("touchstart", onStart, { passive: true });
    el.addEventListener("touchmove", onMove, { passive: true });
    el.addEventListener("touchend", onEnd, { passive: true });
    el.addEventListener("touchcancel", onCancel, { passive: true });
    return () => {
      el.removeEventListener("touchstart", onStart);
      el.removeEventListener("touchmove", onMove);
      el.removeEventListener("touchend", onEnd);
      el.removeEventListener("touchcancel", onCancel);
      setView(NO_SWIPE);
    };
  }, [ref, disabled]);

  return view;
}
