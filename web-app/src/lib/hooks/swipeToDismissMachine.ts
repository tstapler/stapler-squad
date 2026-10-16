import { GESTURE_MOVEMENT_THRESHOLD_PX } from "@/lib/window/windowGestureConstants";

/**
 * Pure state machine for swipe-to-dismiss on a toast or tray row. No React and no
 * DOM: it reduces touch samples to a drag offset and, on release, an outcome.
 *
 *  - A horizontal drag locks after GESTURE_MOVEMENT_THRESHOLD_PX of travel.
 *  - Vertical drift of the same threshold cancels at any time, so the page or
 *    list scrolls instead.
 *  - Release dismisses when the drag passed DISMISS_FRACTION of the row's width,
 *    or was a fling: at least FLING_MIN_PX at FLING_MIN_VELOCITY px/ms.
 */
export const VERTICAL_CANCEL_PX = GESTURE_MOVEMENT_THRESHOLD_PX;
export const DISMISS_FRACTION = 0.4;
export const FLING_MIN_PX = 60;
export const FLING_MIN_VELOCITY = 0.3;
/** A touch starting this close to a screen edge is the OS back gesture, never a dismiss. */
export const EDGE_GUARD_PX = 30;
/** Past this the reveal layer ("Dismiss" or "Move to tray") shows behind the row. */
export const REVEAL_PX = 24;

export function startsInEdgeGuard(x: number, viewportWidth: number): boolean {
  return x < EDGE_GUARD_PX || x > viewportWidth - EDGE_GUARD_PX;
}

export type SwipePhase = "idle" | "tracking" | "dragging" | "done";
export type SwipeOutcome = "dismiss" | "cancel";

export interface SwipeState {
  phase: SwipePhase;
  startX: number;
  startY: number;
  startT: number;
  width: number;
}

export type SwipeSample =
  | { type: "start"; x: number; y: number; t: number; width: number }
  | { type: "move"; x: number; y: number; t: number }
  | { type: "end"; x: number; y: number; t: number }
  | { type: "cancel" };

export interface SwipeStep {
  state: SwipeState;
  /** Horizontal translation to render while dragging; 0 otherwise. */
  offset: number;
  outcome?: SwipeOutcome;
}

export const IDLE_SWIPE: SwipeState = { phase: "idle", startX: 0, startY: 0, startT: 0, width: 0 };

function finish(outcome?: SwipeOutcome): SwipeStep {
  return { state: IDLE_SWIPE, offset: 0, outcome };
}

export function swipeStep(state: SwipeState, sample: SwipeSample): SwipeStep {
  if (sample.type === "start") {
    return {
      state: { phase: "tracking", startX: sample.x, startY: sample.y, startT: sample.t, width: sample.width },
      offset: 0,
    };
  }
  if (sample.type === "cancel") {
    return finish(state.phase === "dragging" ? "cancel" : undefined);
  }
  if (state.phase === "idle" || state.phase === "done") return { state, offset: 0 };

  const dx = sample.x - state.startX;
  const dy = sample.y - state.startY;

  if (Math.abs(dy) >= VERTICAL_CANCEL_PX) return { state: { ...state, phase: "done" }, offset: 0, outcome: "cancel" };

  if (sample.type === "move") {
    if (state.phase === "tracking" && Math.abs(dx) < GESTURE_MOVEMENT_THRESHOLD_PX) return { state, offset: 0 };
    return { state: { ...state, phase: "dragging" }, offset: dx };
  }

  // end
  if (state.phase !== "dragging") return finish();
  const elapsed = Math.max(1, sample.t - state.startT);
  const farEnough = Math.abs(dx) >= state.width * DISMISS_FRACTION;
  const fling = Math.abs(dx) >= FLING_MIN_PX && Math.abs(dx) / elapsed >= FLING_MIN_VELOCITY;
  return finish(farEnough || fling ? "dismiss" : "cancel");
}
