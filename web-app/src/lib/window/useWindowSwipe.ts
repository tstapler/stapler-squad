"use client";

import { useEffect, useRef, RefObject } from "react";
import { GESTURE_MOVEMENT_THRESHOLD_PX } from "@/lib/window/windowGestureConstants";

export type SwipeDirection = "next" | "prev";

interface UseWindowSwipeOptions {
  onSwipe: (direction: SwipeDirection) => void;
}

// A touch starting within this many px of either viewport edge is ignored
// entirely, so iOS Safari's native back/forward edge-swipe keeps working.
const EDGE_AVOIDANCE_PX = 30;
const SWIPE_DISTANCE_THRESHOLD_PX = 60;
const SWIPE_TIME_THRESHOLD_MS = 300;

function isInEdgeZone(clientX: number): boolean {
  return clientX < EDGE_AVOIDANCE_PX || clientX > window.innerWidth - EDGE_AVOIDANCE_PX;
}

type Axis = "horizontal" | "vertical";

function classifyAxis(dx: number, dy: number): Axis {
  return Math.abs(dx) > Math.abs(dy) ? "horizontal" : "vertical";
}

// Fires only when every guard agrees: horizontal axis, past the
// distance/time thresholds, and — if the strip can actually scroll — its
// scrollLeft never moved during the gesture (pre-mortem.md P2 #5).
function shouldFireSwipe(params: {
  axis: Axis | null;
  dx: number;
  dy: number;
  elapsedMs: number;
  isOverflowing: boolean;
  scrollMoved: boolean;
}): boolean {
  const { axis, dx, dy, elapsedMs, isOverflowing, scrollMoved } = params;
  const isHorizontal = axis === null ? classifyAxis(dx, dy) === "horizontal" : axis === "horizontal";
  if (!isHorizontal) return false;
  if (Math.abs(dx) <= SWIPE_DISTANCE_THRESHOLD_PX) return false;
  if (elapsedMs >= SWIPE_TIME_THRESHOLD_MS) return false;
  return !(isOverflowing && scrollMoved);
}

/**
 * Detects a horizontal swipe on `containerRef` and calls `onSwipe("next" | "prev")`.
 *
 * Three things must all agree before firing, per plan.md Epic 3.2 / pre-mortem.md
 * P2 #5: the touch didn't start in the edge-avoidance zone, the gesture axis-locked
 * to horizontal (so a vertical scroll is never misclassified), and — when the
 * container itself is horizontally overflowing — its `scrollLeft` never actually
 * moved (so scrolling the strip to reveal off-screen tabs never also switches
 * the active window).
 */
export function useWindowSwipe(
  containerRef: RefObject<HTMLElement | null>,
  options: UseWindowSwipeOptions
): void {
  const onSwipeRef = useRef(options.onSwipe);
  useEffect(() => {
    onSwipeRef.current = options.onSwipe;
  }, [options.onSwipe]);

  useEffect(() => {
    const containerEl = containerRef.current;
    if (!containerEl) return;

    let active = false;
    let startX = 0;
    let startY = 0;
    let startTime = 0;
    let startScrollLeft = 0;
    let scrollMoved = false;
    let axisLock: "horizontal" | "vertical" | null = null;

    const onTouchStart = (e: TouchEvent) => {
      const touch = e.touches[0];
      if (!touch) return;

      if (isInEdgeZone(touch.clientX)) {
        active = false;
        return;
      }

      active = true;
      axisLock = null;
      scrollMoved = false;
      startX = touch.clientX;
      startY = touch.clientY;
      startTime = Date.now();
      startScrollLeft = containerEl.scrollLeft;
    };

    const updateAxisLock = (touch: Touch) => {
      if (axisLock !== null) return;
      const dx = touch.clientX - startX;
      const dy = touch.clientY - startY;
      if (Math.abs(dx) > GESTURE_MOVEMENT_THRESHOLD_PX || Math.abs(dy) > GESTURE_MOVEMENT_THRESHOLD_PX) {
        axisLock = classifyAxis(dx, dy);
      }
    };

    const onTouchMove = (e: TouchEvent) => {
      if (!active) return;
      const touch = e.touches[0];
      if (!touch) return;

      if (containerEl.scrollLeft !== startScrollLeft) {
        scrollMoved = true;
      }
      updateAxisLock(touch);
    };

    const onTouchEnd = (e: TouchEvent) => {
      if (!active) return;
      active = false;

      const touch = e.changedTouches[0];
      if (!touch) return;

      const dx = touch.clientX - startX;
      const dy = touch.clientY - startY;
      const fire = shouldFireSwipe({
        axis: axisLock,
        dx,
        dy,
        elapsedMs: Date.now() - startTime,
        isOverflowing: containerEl.scrollWidth > containerEl.clientWidth,
        scrollMoved: scrollMoved || containerEl.scrollLeft !== startScrollLeft,
      });
      if (fire) onSwipeRef.current(dx < 0 ? "next" : "prev");
    };

    // passive: true — this hook never calls preventDefault, so native scroll
    // and iOS edge-swipe are left fully intact; it only observes.
    containerEl.addEventListener("touchstart", onTouchStart, { passive: true });
    containerEl.addEventListener("touchmove", onTouchMove, { passive: true });
    containerEl.addEventListener("touchend", onTouchEnd, { passive: true });

    return () => {
      containerEl.removeEventListener("touchstart", onTouchStart);
      containerEl.removeEventListener("touchmove", onTouchMove);
      containerEl.removeEventListener("touchend", onTouchEnd);
    };
  }, [containerRef]);
}
