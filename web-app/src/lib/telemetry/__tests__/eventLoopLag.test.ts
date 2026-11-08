import type { AnalyticsEvent } from "@/lib/analytics/types";
import { observeEventLoopLag, LAG_WINDOW_MS, LAG_TICK_MS } from "../eventLoopLag";

function setup(hidden = false) {
  let t = 0;
  const events: AnalyticsEvent[] = [];
  let tick: (() => void) | null = null;
  let flush: (() => void) | null = null;
  const stop = observeEventLoopLag({
    track: (e) => events.push(e),
    now: () => t,
    isHidden: () => hidden,
    setInterval: (cb, ms) => {
      if (ms === LAG_TICK_MS) tick = cb;
      else flush = cb;
      return ms;
    },
    clearInterval: () => undefined,
  });
  return {
    events,
    stop,
    // One timer firing `lateBy` ms after it was due.
    fire: (lateBy: number) => {
      t += LAG_TICK_MS + lateBy;
      tick?.();
    },
    flush: () => flush?.(),
  };
}

describe("event loop lag sampler", () => {
  it("reports the worst timer drift of the window as eventloop_lag", () => {
    const { fire, flush, events } = setup();
    fire(3);
    fire(180);
    fire(7);
    flush();
    expect(events).toHaveLength(1);
    expect(events[0].labels?.stage).toBe("eventloop_lag");
    expect(events[0].durationMs).toBe(180);
  });

  it("stays quiet when the loop is healthy", () => {
    const { fire, flush, events } = setup();
    fire(0);
    fire(2);
    flush();
    expect(events).toHaveLength(0);
  });

  it("does not sample a hidden tab, whose timers the browser throttles", () => {
    const { fire, flush, events } = setup(true);
    fire(900);
    flush();
    expect(events).toHaveLength(0);
  });

  it("flushes on the window cadence", () => {
    expect(LAG_WINDOW_MS).toBeGreaterThan(LAG_TICK_MS);
  });
});
