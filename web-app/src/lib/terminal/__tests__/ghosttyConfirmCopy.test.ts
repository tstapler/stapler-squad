import { disableGhosttyAutoCopy } from "../ghosttyConfirmCopy";

describe("disableGhosttyAutoCopy", () => {
  it("replaces the selection manager's copy with a no-op", async () => {
    const copied: string[] = [];
    const term = { selectionManager: { copyToClipboard: async (t: string) => void copied.push(t) } };

    expect(disableGhosttyAutoCopy(term)).toBe(true);
    await term.selectionManager.copyToClipboard("selected");

    expect(copied).toEqual([]);
  });

  it("reports false, without throwing, when the private API is gone", () => {
    expect(disableGhosttyAutoCopy({})).toBe(false);
    expect(disableGhosttyAutoCopy({ selectionManager: {} })).toBe(false);
  });
});
