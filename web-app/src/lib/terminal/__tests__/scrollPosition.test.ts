import {
  isAwayFromLive,
  createNetPagesUpTracker,
  NET_PAGES_UP_LIMIT,
  type InvalidationReason,
} from "../scrollPosition";

describe("isAwayFromLive", () => {
  it("isAwayFromLive_should_BeTrueOnlyWhenViewportAboveBase", () => {
    expect(isAwayFromLive({ viewportY: 10, baseY: 40 })).toBe(true);
    expect(isAwayFromLive({ viewportY: 40, baseY: 40 })).toBe(false);
    expect(isAwayFromLive({ viewportY: 0, baseY: 0 })).toBe(false);
  });
});

describe("netPagesUp tracker", () => {
  it("netPagesUp_should_FloorAtZero_When_MorePgDnThanPgUp", () => {
    const t = createNetPagesUpTracker();
    t.pageUp();
    t.pageUp();
    t.pageDown();
    expect(t.getState()).toEqual({ pages: 1, valid: true });
    t.pageDown();
    t.pageDown();
    t.pageDown();
    expect(t.getState()).toEqual({ pages: 0, valid: true });
  });

  it("netPagesUp_should_BeInvalid_When_KeystrokeResizeModeChangeReconnectOrOverFive", () => {
    const reasons: InvalidationReason[] = ["keystroke", "resize", "mode-change", "reconnect"];
    for (const reason of reasons) {
      const t = createNetPagesUpTracker();
      t.pageUp();
      t.invalidate(reason);
      expect(t.getState()).toEqual({ pages: 0, valid: false, reason });
    }

    const overflow = createNetPagesUpTracker();
    for (let i = 0; i < NET_PAGES_UP_LIMIT; i++) overflow.pageUp();
    expect(overflow.getState()).toEqual({ pages: NET_PAGES_UP_LIMIT, valid: true });
    overflow.pageUp();
    expect(overflow.getState()).toEqual({ pages: 0, valid: false, reason: "overflow" });
  });

  it("netPagesUp_should_RestartFromLive_When_PageUpAfterKeystroke_And_StayInvalidAfterOverflow", () => {
    const t = createNetPagesUpTracker();
    t.invalidate("keystroke");
    t.pageDown(); // nothing to go down from
    expect(t.getState().valid).toBe(false);
    t.pageUp();
    expect(t.getState()).toEqual({ pages: 1, valid: true });

    for (let i = 0; i < NET_PAGES_UP_LIMIT; i++) t.pageUp();
    expect(t.getState().reason).toBe("overflow");
    t.pageUp();
    expect(t.getState().valid).toBe(false); // only markLive() recovers from overflow
    t.markLive();
    expect(t.getState()).toEqual({ pages: 0, valid: true });
  });

  it("netPagesUp_should_NotifyOnlyOnStateChange", () => {
    const t = createNetPagesUpTracker();
    const listener = jest.fn();
    t.subscribe(listener);
    t.markLive();
    t.pageDown(); // already 0
    expect(listener).not.toHaveBeenCalled();
    t.pageUp();
    expect(listener).toHaveBeenCalledTimes(1);
  });
});
