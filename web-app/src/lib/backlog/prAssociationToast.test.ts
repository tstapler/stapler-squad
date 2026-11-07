import { prAssociationToast } from "./prAssociationToast";

describe("prAssociationToast", () => {
  it("claims the move only when the server moved the item", () => {
    expect(prAssociationToast("linked_and_moved", 7)).toBe("PR #7 linked — item moved to pr_pending.");
  });

  it("says nothing changed when the PR was already tracked", () => {
    const text = prAssociationToast("already_tracked", 7);
    expect(text).toBe("Already tracking PR #7 — nothing changed.");
    expect(text).not.toMatch(/moved/);
  });

  it("makes no move claim for an unspecified result", () => {
    expect(prAssociationToast("unspecified", 7)).toBe("PR #7 linked.");
  });
});
