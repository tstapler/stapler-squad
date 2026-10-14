import { blockedByLabel, isBlockerUnresolved, unresolvedBlockers } from "./dependencies";

const dep = (title: string, status: string) => ({ itemId: title, publicId: "", title, status });

describe("blockedByLabel", () => {
  it("returns undefined when there are no blockers", () => {
    expect(blockedByLabel(undefined)).toBeUndefined();
    expect(blockedByLabel([])).toBeUndefined();
  });

  it("names the blocker's title", () => {
    expect(blockedByLabel([dep("Fix auth", "in_progress")])).toBe("blocked by Fix auth");
  });

  it("clears when every blocker is done or archived", () => {
    expect(blockedByLabel([dep("A", "done"), dep("B", "archived")])).toBeUndefined();
  });

  it("counts extra unresolved blockers and ignores resolved ones", () => {
    expect(blockedByLabel([dep("A", "done"), dep("B", "queued"), dep("C", "idea")])).toBe("blocked by B +1");
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
