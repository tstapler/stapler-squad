/**
 * Tests for useWindowSwipe — horizontal swipe detection on the window tab
 * strip, with edge-avoidance and overflow-scroll disambiguation
 * (plan.md Epic 3.2, pre-mortem.md P2 #5).
 */

import { renderHook } from "@testing-library/react";
import { RefObject } from "react";

import { useWindowSwipe } from "../useWindowSwipe";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Fake container whose addEventListener/removeEventListener are jest spies
 *  delegating to a real handler map, so tests can fire events directly —
 *  mirrors useTerminalGestures.test.ts's convention for this codebase. */
function makeFakeContainer(overrides: { scrollWidth?: number; clientWidth?: number } = {}) {
  const handlers: Record<string, EventListenerOrEventListenerObject> = {};

  const el = {
    addEventListener: jest.fn((type: string, listener: EventListenerOrEventListenerObject) => {
      handlers[type] = listener;
    }),
    removeEventListener: jest.fn((type: string) => {
      delete handlers[type];
    }),
    scrollLeft: 0,
    scrollWidth: overrides.scrollWidth ?? 400,
    clientWidth: overrides.clientWidth ?? 400,
  } as unknown as HTMLElement & { scrollLeft: number };

  const fire = (type: string, event: Event) => {
    const h = handlers[type];
    if (!h) return;
    if (typeof h === "function") h(event);
    else h.handleEvent(event);
  };

  return { el, fire };
}

function makeTouchEvent(
  clientX: number,
  clientY: number,
  touchListKey: "touches" | "changedTouches" = "touches"
): TouchEvent {
  const touch = { clientX, clientY } as Touch;
  const event: Partial<TouchEvent> = {
    touches: touchListKey === "touches" ? ([touch] as unknown as TouchList) : ([] as unknown as TouchList),
    changedTouches: [touch] as unknown as TouchList,
  };
  return event as TouchEvent;
}

let now = 0;
function advanceTime(ms: number) {
  now += ms;
}

describe("useWindowSwipe", () => {
  beforeEach(() => {
    now = 0;
    jest.spyOn(Date, "now").mockImplementation(() => now);
    Object.defineProperty(window, "innerWidth", { value: 400, writable: true, configurable: true });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  function setup(overrides: { scrollWidth?: number; clientWidth?: number } = {}) {
    const { el, fire } = makeFakeContainer(overrides);
    const onSwipe = jest.fn();
    const ref = { current: el } as RefObject<HTMLElement | null>;
    renderHook(() => useWindowSwipe(ref, { onSwipe }));
    return { el, fire, onSwipe };
  }

  test("useWindowSwipe_should_callOnSwipeNext_When_leftwardSwipeExceedsDistanceAndTimeThreshold", () => {
    const { fire, onSwipe } = setup();

    fire("touchstart", makeTouchEvent(300, 100));
    fire("touchmove", makeTouchEvent(220, 100));
    advanceTime(100);
    fire("touchend", makeTouchEvent(150, 100, "changedTouches"));

    expect(onSwipe).toHaveBeenCalledTimes(1);
    expect(onSwipe).toHaveBeenCalledWith("next");
  });

  test("useWindowSwipe_should_callOnSwipePrev_When_rightwardSwipeExceedsDistanceAndTimeThreshold", () => {
    const { fire, onSwipe } = setup();

    fire("touchstart", makeTouchEvent(100, 100));
    fire("touchmove", makeTouchEvent(180, 100));
    advanceTime(100);
    fire("touchend", makeTouchEvent(250, 100, "changedTouches"));

    expect(onSwipe).toHaveBeenCalledTimes(1);
    expect(onSwipe).toHaveBeenCalledWith("prev");
  });

  test("useWindowSwipe_should_notCallOnSwipe_When_touchOriginatesInEdgeAvoidanceZone", () => {
    const { fire, onSwipe } = setup();

    // Starts at x: 10, inside the 30px edge-avoidance zone.
    fire("touchstart", makeTouchEvent(10, 100));
    fire("touchmove", makeTouchEvent(90, 100));
    advanceTime(100);
    fire("touchend", makeTouchEvent(160, 100, "changedTouches"));

    expect(onSwipe).not.toHaveBeenCalled();
  });

  test("useWindowSwipe_should_notCallOnSwipe_When_verticalMovementDominates", () => {
    const { fire, onSwipe } = setup();

    fire("touchstart", makeTouchEvent(200, 100));
    fire("touchmove", makeTouchEvent(180, 250));
    advanceTime(100);
    fire("touchend", makeTouchEvent(150, 300, "changedTouches"));

    expect(onSwipe).not.toHaveBeenCalled();
  });

  test("useWindowSwipe_should_notCallOnSwipe_When_stripIsOverflowingAndTouchScrollsIt", () => {
    const { el, fire, onSwipe } = setup({ scrollWidth: 800, clientWidth: 400 });

    fire("touchstart", makeTouchEvent(300, 100));
    el.scrollLeft = 0;
    fire("touchmove", makeTouchEvent(260, 100));
    // Native scroll engages mid-gesture.
    el.scrollLeft = 40;
    fire("touchmove", makeTouchEvent(220, 100));
    advanceTime(100);
    fire("touchend", makeTouchEvent(150, 100, "changedTouches"));

    expect(onSwipe).not.toHaveBeenCalled();
  });

  test("useWindowSwipe_should_callOnSwipe_When_stripIsOverflowingButScrollLeftIsUnchanged", () => {
    const { el, fire, onSwipe } = setup({ scrollWidth: 800, clientWidth: 400 });

    fire("touchstart", makeTouchEvent(300, 100));
    el.scrollLeft = 0;
    fire("touchmove", makeTouchEvent(260, 100));
    // Strip is already scrolled to an end — native scroll never engages.
    fire("touchmove", makeTouchEvent(220, 100));
    advanceTime(100);
    fire("touchend", makeTouchEvent(150, 100, "changedTouches"));

    expect(onSwipe).toHaveBeenCalledTimes(1);
    expect(onSwipe).toHaveBeenCalledWith("next");
  });

  test("useWindowSwipe_should_notCallOnSwipe_When_gestureExceedsTimeThreshold", () => {
    const { fire, onSwipe } = setup();

    fire("touchstart", makeTouchEvent(300, 100));
    fire("touchmove", makeTouchEvent(220, 100));
    advanceTime(500);
    fire("touchend", makeTouchEvent(150, 100, "changedTouches"));

    expect(onSwipe).not.toHaveBeenCalled();
  });

  test("useWindowSwipe_should_notCallOnSwipe_When_distanceIsBelowThreshold", () => {
    const { fire, onSwipe } = setup();

    fire("touchstart", makeTouchEvent(300, 100));
    fire("touchmove", makeTouchEvent(280, 100));
    advanceTime(100);
    fire("touchend", makeTouchEvent(260, 100, "changedTouches"));

    expect(onSwipe).not.toHaveBeenCalled();
  });
});
