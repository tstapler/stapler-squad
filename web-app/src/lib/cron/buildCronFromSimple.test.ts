import { buildCronFromSimple, parseCronToSimple } from "./buildCronFromSimple";

describe("buildCronFromSimple", () => {
  it("builds daily", () => {
    expect(buildCronFromSimple({ frequency: "daily", hour: 9, minute: 0 })).toBe("0 9 * * *");
  });
  it("builds weekdays", () => {
    expect(buildCronFromSimple({ frequency: "weekdays", hour: 9, minute: 0 })).toBe("0 9 * * 1-5");
  });
  it("builds weekly", () => {
    expect(buildCronFromSimple({ frequency: "weekly", hour: 9, minute: 30, dayOfWeek: 3 })).toBe(
      "30 9 * * 3"
    );
  });
  it("builds monthly", () => {
    expect(buildCronFromSimple({ frequency: "monthly", hour: 9, minute: 0, dayOfMonth: 15 })).toBe(
      "0 9 15 * *"
    );
  });
});

describe("parseCronToSimple", () => {
  it("round-trips every frequency the builder can produce", () => {
    const schedules = [
      { frequency: "daily" as const, hour: 9, minute: 0 },
      { frequency: "weekdays" as const, hour: 8, minute: 15 },
      { frequency: "weekly" as const, hour: 17, minute: 45, dayOfWeek: 5 },
      { frequency: "monthly" as const, hour: 0, minute: 0, dayOfMonth: 1 },
    ];
    for (const s of schedules) {
      expect(parseCronToSimple(buildCronFromSimple(s))).toEqual(s);
    }
  });

  it("returns null for step values, ranges, and lists it can't represent", () => {
    expect(parseCronToSimple("*/15 9-17 * * 1-5")).toBeNull();
    expect(parseCronToSimple("0 9,13,17 * * *")).toBeNull();
    expect(parseCronToSimple("0 9 15 * 1")).toBeNull(); // both dom and dow restricted
  });

  it("returns null for an empty or malformed expression", () => {
    expect(parseCronToSimple("")).toBeNull();
    expect(parseCronToSimple("not a cron")).toBeNull();
  });

  it("returns null when the month field is restricted (not representable by Simple)", () => {
    expect(parseCronToSimple("0 9 1 6 *")).toBeNull();
  });
});
