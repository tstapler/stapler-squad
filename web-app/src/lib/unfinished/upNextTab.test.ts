import {
  readStoredTab,
  resolveInitialTab,
  UP_NEXT_TAB_STORAGE_KEY,
} from "./upNextTab";

describe("resolveInitialTab", () => {
  it.each([
    [{ item: "abc", tab: "queue" }, "worktrees", "stuck"],
    [{ tab: "queue" }, "worktrees", "queue"],
    [{}, "worktrees", "worktrees"],
    [{}, null, "prs"],
    [{ item: "abc" }, null, "stuck"],
  ])("resolveInitialTab_should_PreferItemOverTabOverStoredOverPRs_When_AllCombinations %j stored=%s", (params, stored, expected) => {
    expect(resolveInitialTab(params, stored)).toBe(expected);
  });

  it("resolveInitialTab_should_ReturnPRs_When_TabBogusStorageMalformedOrGetItemThrows", () => {
    expect(resolveInitialTab({ tab: "bogus" }, "{not json")).toBe("prs");

    const spy = jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });
    try {
      expect(() => readStoredTab()).not.toThrow();
      expect(resolveInitialTab({}, readStoredTab())).toBe("prs");
    } finally {
      spy.mockRestore();
    }
  });

  it("reads the stored tab from localStorage", () => {
    window.localStorage.setItem(UP_NEXT_TAB_STORAGE_KEY, "queue");
    expect(readStoredTab()).toBe("queue");
    window.localStorage.clear();
  });
});
