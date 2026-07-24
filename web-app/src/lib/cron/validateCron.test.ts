import { validateCron } from "./validateCron";

describe("validateCron", () => {
  it.each([
    ["0 9 * * *", "daily"],
    ["0 9 * * 1-5", "weekdays range"],
    ["0 9 * * 1", "weekly single day"],
    ["0 9 1 * *", "monthly"],
    ["*/15 9-17 * * 1-5", "step + range"],
    ["0 9,13,17 * * *", "list"],
    ["0 9 15 * 1", "dom + dow both restricted"],
    ["0 0 1 JAN *", "month name"],
    ["0 0 * * SUN", "day name"],
  ])("accepts valid expression %s (%s)", (expr) => {
    expect(validateCron(expr)).toEqual({ valid: true });
  });

  it("rejects an empty expression", () => {
    expect(validateCron("").valid).toBe(false);
    expect(validateCron("   ").valid).toBe(false);
  });

  it("rejects expressions with a seconds field (6 fields)", () => {
    expect(validateCron("0 0 9 * * *").valid).toBe(false);
  });

  it("rejects @ descriptors since the backend parser has no Descriptor option", () => {
    expect(validateCron("@daily").valid).toBe(false);
    expect(validateCron("@hourly").valid).toBe(false);
  });

  it.each(["? * * * * ?", "0 9 * * 1L", "0 9 1W * *", "0 9 * * 5#3"])(
    "rejects Quartz-only syntax: %s",
    (expr) => {
      expect(validateCron(expr).valid).toBe(false);
    }
  );

  it("rejects out-of-range values", () => {
    expect(validateCron("60 9 * * *").valid).toBe(false); // minute max 59
    expect(validateCron("0 24 * * *").valid).toBe(false); // hour max 23
    expect(validateCron("0 9 32 * *").valid).toBe(false); // dom max 31
    expect(validateCron("0 9 0 * *").valid).toBe(false); // dom min 1
    expect(validateCron("0 9 * 13 *").valid).toBe(false); // month max 12
    expect(validateCron("0 9 * * 7").valid).toBe(false); // dow max 6
  });

  it("rejects a start greater than end in a range", () => {
    expect(validateCron("0 9 * * 5-1").valid).toBe(false);
  });

  it("accepts a TZ= prefix by stripping it before field validation", () => {
    expect(validateCron("TZ=America/New_York 0 9 * * *").valid).toBe(true);
  });
});
