import { claimWebglSlot, touchWebglSlot, releaseWebglSlot, type WebglSlot } from "../webglBudget";

const makeSlot = (visible = true): WebglSlot & { release: jest.Mock } => ({
  isVisible: () => visible,
  release: jest.fn(),
});

describe("webglBudget", () => {
  const live: WebglSlot[] = [];
  const claim = (s: WebglSlot, max: number) => {
    live.push(s);
    claimWebglSlot(s, max);
  };
  afterEach(() => live.splice(0).forEach(releaseWebglSlot));

  it("claim_should_EvictOldest_When_OverBudgetAndAllVisible", () => {
    const [a, b, c] = [makeSlot(), makeSlot(), makeSlot()];
    claim(a, 2);
    claim(b, 2);
    claim(c, 2);
    expect(a.release).toHaveBeenCalledTimes(1);
    expect(b.release).not.toHaveBeenCalled();
    expect(c.release).not.toHaveBeenCalled();
  });

  it("claim_should_EvictHiddenBeforeOlderVisible_When_OverBudget", () => {
    const [a, hidden, c] = [makeSlot(), makeSlot(false), makeSlot()];
    claim(a, 2);
    claim(hidden, 2);
    claim(c, 2);
    expect(hidden.release).toHaveBeenCalledTimes(1);
    expect(a.release).not.toHaveBeenCalled();
  });

  it("touch_should_ProtectFocusedSlot_When_OverBudget", () => {
    const [a, b, c] = [makeSlot(), makeSlot(), makeSlot()];
    claim(a, 2);
    claim(b, 2);
    touchWebglSlot(a);
    claim(c, 2);
    expect(b.release).toHaveBeenCalledTimes(1);
    expect(a.release).not.toHaveBeenCalled();
  });

  it("claim_should_NeverEvictTheClaimingSlot_When_BudgetIsOne", () => {
    const [a, b] = [makeSlot(), makeSlot()];
    claim(a, 1);
    claim(b, 1);
    expect(b.release).not.toHaveBeenCalled();
    expect(a.release).toHaveBeenCalledTimes(1);
  });

  it("claim_should_StillEvictRemainingAndNotThrow_When_VictimReleaseThrows", () => {
    const spy = jest.spyOn(console, "error").mockImplementation(() => {});
    const [a, b, c] = [makeSlot(), makeSlot(), makeSlot()];
    a.release.mockImplementation(() => {
      throw new Error("dispose failed");
    });
    claim(a, 2);
    claim(b, 2);
    expect(() => claim(c, 2)).not.toThrow();
    expect(a.release).toHaveBeenCalledTimes(1);
    spy.mockRestore();
  });

  it("release_should_FreeBudget_When_SlotReleasedByOwner", () => {
    const [a, b, c] = [makeSlot(), makeSlot(), makeSlot()];
    claim(a, 2);
    claim(b, 2);
    releaseWebglSlot(a);
    claim(c, 2);
    expect(b.release).not.toHaveBeenCalled();
  });

  it("webglBudget_should_EvictHiddenFirst_And_KeepContextsAtMost8_When_12TerminalsMounted", () => {
    // 3 windows x 4 panes; window 0 is active, windows 1 and 2 are hidden (not visible).
    const windows = [0, 1, 2].map((w) => Array.from({ length: 4 }, () => makeSlot(w === 0)));
    // Mount order: hidden windows were visited first, the active window mounts last.
    [windows[1], windows[2], windows[0]].flat().forEach((s) => claim(s, 8));
    const evicted = windows.flat().filter((s) => s.release.mock.calls.length > 0);
    expect(evicted).toHaveLength(4); // 12 claimed, budget 8
    expect(evicted.every((s) => !windows[0].includes(s))).toBe(true); // the active window keeps WebGL
    expect(windows[0].every((s) => s.release.mock.calls.length === 0)).toBe(true);
  });
});
