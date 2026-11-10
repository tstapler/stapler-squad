import { formatRatio } from "./flowFormatters";

describe("flowFormatters", () => {
  it.each([
    [32, 105, "32/105 (30.5%)"],
    [0, 14, "0/14 (0.0%)"],
    [1234, 5000, "1,234/5,000 (24.7%)"],
    [5, 0, "no data"],
  ])("formatRatio_should_PrintCountFirst_When_AnyRatio(%d/%d)", (num, den, expected) => {
    expect(formatRatio(num, den)).toBe(expected);
  });
});
