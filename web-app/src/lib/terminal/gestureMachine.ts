/**
 * Pure touch-gesture state machine (design/ux.md S9). No DOM, React or timers:
 * the hook feeds it events and executes the returned effects in order.
 */

/** CANCELLED: the touch is ignored until the next touchstart (design/ux.md S9). */
export type GestureState = "IDLE" | "PENDING" | "SCROLLING" | "SELECTING" | "TAPPING" | "CANCELLED";

export type GestureEvent =
  | { type: "touchstart"; touchCount: number }
  | { type: "touchmove"; touchCount: number; absDy: number; slopPx: number }
  | {
      type: "touchend";
      elapsedMs: number;
      totalDy: number;
      longPressMs: number;
      tapTolerancePx: number;
    }
  | { type: "touchcancel" }
  | { type: "longPress" }
  /** Keyboard/viewport resize, orientation, override or connectionEpoch change. */
  | { type: "interrupt" };

export type GestureEffect =
  /** Clear the long-press timer and drop pending animation frames. */
  | "abort"
  | "startLongPressTimer"
  | "clearLongPressTimer"
  | "beginScroll"
  | "continueScroll"
  | "continueSelect"
  | "enterSelecting"
  | "endSelecting"
  | "tap"
  | "preventDefault";

export interface GestureTransition {
  state: GestureState;
  effects: readonly GestureEffect[];
}

const NONE: readonly GestureEffect[] = [];

const stay = (state: GestureState): GestureTransition => ({ state, effects: NONE });
const abort = (): GestureTransition => ({ state: "IDLE", effects: ["abort"] });

function onTouchStart(touchCount: number): GestureTransition {
  if (touchCount !== 1) return abort();
  return { state: "PENDING", effects: ["startLongPressTimer"] };
}

function onTouchMove(
  state: GestureState,
  e: Extract<GestureEvent, { type: "touchmove" }>,
): GestureTransition {
  if (e.touchCount !== 1) return abort();
  switch (state) {
    case "PENDING":
      return e.absDy > e.slopPx
        ? { state: "SCROLLING", effects: ["clearLongPressTimer", "beginScroll", "preventDefault"] }
        : stay(state);
    case "SCROLLING":
      return { state, effects: ["continueScroll", "preventDefault"] };
    case "SELECTING":
      return { state, effects: ["continueSelect", "preventDefault"] };
    default:
      return stay(state);
  }
}

function onTouchEnd(
  state: GestureState,
  e: Extract<GestureEvent, { type: "touchend" }>,
): GestureTransition {
  if (state === "PENDING" && e.totalDy < e.tapTolerancePx && e.elapsedMs < e.longPressMs) {
    return { state: "IDLE", effects: ["clearLongPressTimer", "tap"] };
  }
  if (state === "SELECTING") return { state: "IDLE", effects: ["endSelecting", "abort"] };
  // preventDefault stops Chrome synthesizing a click (and focus) after a scroll.
  if (state === "SCROLLING") return { state: "IDLE", effects: ["abort", "preventDefault"] };
  return abort();
}

export function reduce(state: GestureState, event: GestureEvent): GestureTransition {
  switch (event.type) {
    case "touchstart":
      return onTouchStart(event.touchCount);
    case "touchmove":
      return onTouchMove(state, event);
    case "touchend":
      return onTouchEnd(state, event);
    case "touchcancel":
      return abort();
    case "longPress":
      return state === "PENDING" ? { state: "SELECTING", effects: ["enterSelecting"] } : stay(state);
    case "interrupt":
      return state === "PENDING" || state === "SCROLLING" ? { state: "CANCELLED", effects: ["abort"] } : stay(state);
  }
}
