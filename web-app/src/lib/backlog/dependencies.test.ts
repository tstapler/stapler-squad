import { dependencyLabel, isBlockerUnresolved, unresolvedBlockers } from "./dependencies";

const dep = (title: string, status: string) => ({ itemId: title, publicId: "", title, status });

describe("dependencyLabel", () => {
  it("falls back from title to publicId to itemId", () => {
    expect(dependencyLabel({ itemId: "id", publicId: "BL-1", title: "T" })).toBe("T");
    expect(dependencyLabel({ itemId: "id", publicId: "BL-1", title: "" })).toBe("BL-1");
    expect(dependencyLabel({ itemId: "id", publicId: "", title: "" })).toBe("id");
  });
});

describe("isBlockerUnresolved", () => {
  it.each([
    ["done", false],
    ["archived", false],
    ["idea", true],
    ["in_progress", true],
  ])("status %s -> unresolved=%s", (status, expected) => {
    expect(isBlockerUnresolved({ status })).toBe(expected);
    expect(unresolvedBlockers([dep("x", status)])).toHaveLength(expected ? 1 : 0);
  });
});
