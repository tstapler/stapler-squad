import { truncateGoal } from "./string";

// U-TS-22, U-TS-23
describe("truncateGoal", () => {
  it("truncates at max length", () => {
    const input = "A".repeat(70);
    const result = truncateGoal(input, 60);
    expect(result.length).toBe(60); // exactly max chars (59 content + "…")
    expect(result).toContain("…");
    expect(result).not.toEqual(input);
  });

  it("returns original when under max length", () => {
    const input = "short text";
    expect(truncateGoal(input, 60)).toEqual(input);
  });

  it("returns original when exactly at max length", () => {
    const input = "A".repeat(60);
    expect(truncateGoal(input, 60)).toEqual(input);
  });

  it("handles empty string", () => {
    expect(truncateGoal("", 60)).toEqual("");
  });
});
