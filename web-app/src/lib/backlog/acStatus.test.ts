import { isAcSatisfied } from "./acStatus";

describe("isAcSatisfied", () => {
  it("counts done and waived criteria toward the done total, not pending", () => {
    const rows = [{ status: "done" }, { status: "waived" }, { status: "pending" }] as { status: never }[];
    expect(rows.filter(isAcSatisfied).length).toBe(2);
  });
});
