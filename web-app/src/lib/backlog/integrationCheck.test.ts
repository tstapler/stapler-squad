import { integrationCheckState } from "./integrationCheck";

describe("integrationCheckState", () => {
  it("is awaiting when the child is unknown or in flight", () => {
    expect(integrationCheckState(undefined)).toBe("awaiting");
    expect(integrationCheckState({ status: "in_progress", acCriteria: [] })).toBe("awaiting");
  });
  it("is passed only when the child is done", () => {
    expect(integrationCheckState({ status: "done", acCriteria: [] })).toBe("passed");
  });
  it("is passed when the child was archived, matching the backend done gate", () => {
    expect(integrationCheckState({ status: "archived", acCriteria: [] })).toBe("passed");
  });
  it("is failed when a criterion failed and the child is not resolved", () => {
    expect(
      integrationCheckState({ status: "review", acCriteria: [{ index: 0, text: "x", status: "fail" as never }] }),
    ).toBe("failed");
  });
});
