import { integrationCheckState } from "./integrationCheck";

const archivals = (...from: string[]) => ({
  status: "archived" as const,
  acCriteria: [],
  statusEvents: from.map((fromStatus, i) => ({ id: String(i), fromStatus, toStatus: "archived", triggeredBy: "user" })),
});

describe("integrationCheckState", () => {
  it("is awaiting when the child is unknown or in flight", () => {
    expect(integrationCheckState(undefined)).toBe("awaiting");
    expect(integrationCheckState({ status: "in_progress", acCriteria: [] })).toBe("awaiting");
  });
  it("is passed when the child is done", () => {
    expect(integrationCheckState({ status: "done", acCriteria: [] })).toBe("passed");
  });
  it("is not passed when the child was archived without passing, matching the backend done gate", () => {
    expect(integrationCheckState({ status: "archived", acCriteria: [] })).toBe("archived");
    expect(integrationCheckState(archivals("review"))).toBe("archived");
    expect(integrationCheckState(archivals("done", "idea"))).toBe("archived");
  });
  it("is passed when the child was archived out of done, even if archived again", () => {
    expect(integrationCheckState(archivals("done"))).toBe("passed");
    expect(integrationCheckState(archivals("done", "archived"))).toBe("passed");
  });
  it("is failed when a criterion failed and the child is not resolved", () => {
    expect(
      integrationCheckState({ status: "review", acCriteria: [{ index: 0, text: "x", status: "fail" as never }] }),
    ).toBe("failed");
  });
});
