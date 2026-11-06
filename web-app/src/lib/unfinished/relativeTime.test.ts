import { formatRelativeTime } from "./relativeTime";

describe("formatRelativeTime", () => {
  it.each([
    [0, "just now"],
    [59_999, "just now"],
    [60_000, "1 min ago"],
    [5 * 60_000, "5 min ago"],
    [60 * 60_000, "1 h ago"],
    [25 * 60 * 60_000, "1 d ago"],
    [-5_000, "just now"],
  ])("%d ms ago reads %s", (delta, expected) => {
    expect(formatRelativeTime(1_000_000, 1_000_000 + delta)).toBe(expected);
  });
});
