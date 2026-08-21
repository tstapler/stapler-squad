import cronstrue from "cronstrue";
import { explainCron } from "./explainCron";

describe("explainCron", () => {
  it("shows a neutral placeholder for an empty expression", () => {
    expect(explainCron("")).toBe("Enter a schedule above");
    expect(explainCron("   ")).toBe("Enter a schedule above");
  });

  it("shows a still-typing message for an in-progress expression", () => {
    expect(explainCron("0 9 * *")).toBe("Still typing…");
  });

  it("explains a daily schedule", () => {
    expect(explainCron("0 9 * * *")).toMatch(/09:00 AM/);
  });

  it("explains a weekly schedule", () => {
    expect(explainCron("0 9 * * 1")).toMatch(/Monday/);
  });

  it("explains a monthly schedule", () => {
    const text = explainCron("0 9 1 * *");
    expect(text).toMatch(/09:00 AM/);
    expect(text).toMatch(/day 1 of the month/);
  });

  it("renders day-of-month + day-of-week as OR, not AND", () => {
    const text = explainCron("0 9 15 * 1");
    expect(text).toMatch(/or/i);
    expect(text).not.toMatch(/\band\b/i);
    expect(text).toMatch(/day 15 of the month/);
    expect(text).toMatch(/Monday/);
  });

  it("shows an inline error for an invalid expression", () => {
    expect(explainCron("99 9 * * *")).toMatch(/^Invalid:/);
  });

  it("flags @ descriptors as invalid since the backend doesn't accept them", () => {
    expect(explainCron("@daily")).toMatch(/^Invalid:/);
  });

  it("flags Quartz syntax as invalid", () => {
    expect(explainCron("0 9 * * 1L")).toMatch(/^Invalid:/);
  });

  it("falls back to a generic message when cronstrue throws on a grammar-valid expression", () => {
    const spy = jest.spyOn(cronstrue, "toString").mockImplementation(() => {
      throw new Error("boom");
    });
    expect(explainCron("0 9 * * *")).toBe("Unable to explain this expression");
    spy.mockRestore();
  });
});
