/**
 * Interaction-safety windows (design/ux.md 7.1 and 2.4), in one place so tests can pin them.
 * `jest.setup.js` zeroes the settle window and the commit delay for suites that click right
 * after render; the interaction-safety suites restore the real values.
 */
export interface InteractionTiming {
  /** Engagement ends, and digits stay inert, this long after the last trigger. */
  graceMs: number;
  /** A touch holds its card engaged this long. */
  touchHoldMs: number;
  /** A just-rendered or moved control ignores activation this long. */
  settleMs: number;
  /** A touch beginning this soon after a scroll event is a fling-stop, not a tap. */
  flingGuardMs: number;
  /** Approve, Send back and Mark done wait this long before sending. 0 sends at once. */
  commitDelayMs: number;
}

export const DEFAULT_TIMING: InteractionTiming = {
  graceMs: 750,
  touchHoldMs: 5_000,
  settleMs: 300,
  flingGuardMs: 120,
  commitDelayMs: 3_000,
};

let timing: InteractionTiming = DEFAULT_TIMING;

export const getTiming = (): InteractionTiming => timing;

/** Test seam: pass nothing to restore the defaults. */
export function setTimingForTests(over: Partial<InteractionTiming> = {}): void {
  timing = { ...DEFAULT_TIMING, ...over };
}
