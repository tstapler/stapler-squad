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

  it("release_should_FreeBudget_When_SlotReleasedByOwner", () => {
    const [a, b, c] = [makeSlot(), makeSlot(), makeSlot()];
    claim(a, 2);
    claim(b, 2);
    releaseWebglSlot(a);
    claim(c, 2);
    expect(b.release).not.toHaveBeenCalled();
  });
});
