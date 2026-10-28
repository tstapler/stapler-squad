import { pollIntervalMs } from "../pollMode";

describe("pollIntervalMs", () => {
  it("pollIntervalMs_should_MapModeToCadence_When_OffSlowFast", () => {
    expect(pollIntervalMs("off", 10, 100)).toBeNull();
    expect(pollIntervalMs("slow", 10, 100)).toBe(100);
    expect(pollIntervalMs("fast", 10, 100)).toBe(10);
  });
});
