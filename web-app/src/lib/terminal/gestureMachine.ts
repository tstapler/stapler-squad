/**
 * Pure touch-gesture state machine (design/ux.md S9). No DOM, React or timers:
 * the hook feeds it events and executes the returned effects in order.
 */

/**
 * COASTING: momentum running after a fling release (no finger down).
 * CANCELLED: the touch is ignored until the next touchstart (design/ux.md S9).
 */
export type GestureState = "IDLE" | "PENDING" | "SCROLLING" | "SELECTING" | "TAPPING" | "COASTING" | "CANCELLED";

export type GestureEvent =
  | { type: "touchstart"; touchCount: number }
  | { type: "touchmove"; touchCount: number; absDx: number; absDy: number; slopPx: number }
  | {
      type: "touchend";
      elapsedMs: number;
      totalDy: number;
      longPressMs: number;
      tapTolerancePx: number;
      /** A SCROLLING release that starts momentum (decided by the hook's MomentumTracker). */
      flinging: boolean;
      /** This touch began as a touch-to-stop during COASTING. */
      consumedByCoast: boolean;
      /** The terminal has an active selection. */
      selectionActive: boolean;
    }
  | { type: "touchcancel" }
  | { type: "longPress" }
  /** Momentum ran out (decay, edge, page cap) with no finger down. */
  | { type: "momentumEnd" }
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
  /** A tap while a selection is active only clears it; no focus. */
  | "clearSelection"
  | "startMomentum"
  /** Remember that the matching touchend is a touch-to-stop, not a tap. */
  | "setConsumedByCoast"
  | "preventDefault";

export interface GestureTransition {
  state: GestureState;
  effects: readonly GestureEffect[];
}

const NONE: readonly GestureEffect[] = [];

const stay = (state: GestureState): GestureTransition => ({ state, effects: NONE });
const abort = (): GestureTransition => ({ state: "IDLE", effects: ["abort"] });

function onTouchStart(state: GestureState, touchCount: number): GestureTransition {
  // Multi-touch parks the gesture in CANCELLED so the remaining fingers' moves are ignored until a lone touchstart.
  if (touchCount !== 1) return { state: "CANCELLED", effects: ["abort"] };
  // abort cancels momentum; no long-press timer, a touch-to-stop must not become a selection.
  if (state === "COASTING") return { state: "PENDING", effects: ["abort", "setConsumedByCoast"] };
  return { state: "PENDING", effects: ["startLongPressTimer"] };
}

function onTouchMove(
  state: GestureState,
  e: Extract<GestureEvent, { type: "touchmove" }>,
): GestureTransition {
  if (e.touchCount !== 1) return state === "CANCELLED" ? stay(state) : abort();
  switch (state) {
    case "PENDING":
      if (e.absDx <= e.slopPx && e.absDy <= e.slopPx) return stay(state);
      // Horizontal-first: leave the gesture to the browser (Android back-swipe), no scroll, no tap.
      if (e.absDx > e.absDy) return { state: "CANCELLED", effects: ["abort"] };
      return { state: "SCROLLING", effects: ["clearLongPressTimer", "beginScroll", "preventDefault"] };
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
  if (state === "COASTING") return stay(state);
  if (state === "PENDING" && e.consumedByCoast) return { state: "IDLE", effects: ["abort", "preventDefault"] };
  if (state === "PENDING" && e.totalDy < e.tapTolerancePx && e.elapsedMs < e.longPressMs) {
    return { state: "IDLE", effects: ["clearLongPressTimer", e.selectionActive ? "clearSelection" : "tap"] };
  }
  if (state === "SELECTING") return { state: "IDLE", effects: ["endSelecting", "abort"] };
  // preventDefault stops Chrome synthesizing a click (and focus) after a scroll.
  if (state === "SCROLLING") {
    return e.flinging
      ? { state: "COASTING", effects: ["startMomentum", "preventDefault"] }
      : { state: "IDLE", effects: ["abort", "preventDefault"] };
  }
  return abort();
}

export function reduce(state: GestureState, event: GestureEvent): GestureTransition {
  switch (event.type) {
    case "touchstart":
      return onTouchStart(state, event.touchCount);
    case "touchmove":
      return onTouchMove(state, event);
    case "touchend":
      return onTouchEnd(state, event);
    case "touchcancel":
      // Intentionally IDLE (not CANCELLED as in ux.md S9 for SCROLLING): plan AC 1.2.10 specifies IDLE, and
      // the next touchstart begins a fresh PENDING from either state, so the two are observably identical.
      return abort();
    case "longPress":
      return state === "PENDING" ? { state: "SELECTING", effects: ["enterSelecting"] } : stay(state);
    case "momentumEnd":
      return state === "COASTING" ? abort() : stay(state);
    case "interrupt":
      return state === "PENDING" || state === "SCROLLING" || state === "COASTING" ? { state: "CANCELLED", effects: ["abort"] } : stay(state);
  }
}
