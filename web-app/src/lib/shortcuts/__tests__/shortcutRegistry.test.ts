import { ShortcutRegistry } from "../shortcutRegistry";

describe("ShortcutRegistry sequences", () => {
  let reg: ShortcutRegistry;
  beforeEach(() => {
    reg = new ShortcutRegistry();
  });
  afterEach(() => reg.destroy());

  const base = { context: "global" as const, action: () => {} };

  it("getTree_should_nestActionsUnderGroupedFirstKey", () => {
    reg.register("a", { ...base, label: "Next window", sequence: ["w", "n"], group: "Windows" });
    reg.register("b", { ...base, label: "Prev window", sequence: ["w", "p"], group: "Windows" });
    reg.register("c", { ...base, label: "Toggle tray", sequence: ["n", "t"], group: "Notifications" });
    const tree = reg.getTree();
    expect(tree.map((n) => [n.key, n.label])).toEqual([["w", "Windows"], ["n", "Notifications"]]);
    expect(tree[0].children.map((n) => n.key)).toEqual(["n", "p"]);
    expect(tree[0].children[0].shortcut?.label).toBe("Next window");
  });

  it("getAll_should_keepSignatureAndExcludeHidden", () => {
    reg.register("v", { ...base, key: "x", label: "Visible" });
    reg.register("h", { ...base, key: "y", label: "Hidden", hidden: true });
    const all = reg.getAll();
    expect(Object.keys(all).sort()).toEqual(["approval", "cockpit", "global", "omnibar", "session-list", "terminal"]);
    expect(all.global.map((s) => s.label)).toEqual(["Visible"]);
  });

  it("register_should_rejectReservedNavigationKeysInSequence", () => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => {});
    reg.register("bad", { ...base, label: "Bad", sequence: ["w", "j"] });
    expect(reg.getTree()).toEqual([]);
    warn.mockRestore();
  });

  it("getPalette_should_includeUnsequencedActionsButNotHidden", () => {
    reg.register("direct", { ...base, key: "[", label: "Toggle drawer" });
    reg.register("seq", { ...base, label: "Seq", sequence: ["s", "x"] });
    reg.register("h", { ...base, key: "q", label: "H", hidden: true });
    expect(reg.getPalette().map((e) => e.id)).toEqual(["direct", "seq"]);
  });

  it("sequenceOnlyActions_should_neverFireFromDirectKeydown", () => {
    const action = jest.fn();
    reg.register("seq", { ...base, label: "Seq", sequence: ["s", "x"], action });
    document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "s", bubbles: true }));
    expect(action).not.toHaveBeenCalled();
  });

  it("codeBinding_should_matchWhenKeyIsASymbol", () => {
    const action = jest.fn();
    reg.register("w", { ...base, key: "w", code: "KeyW", modifiers: { alt: true }, label: "W", action });
    document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "∑", code: "KeyW", altKey: true, bubbles: true }));
    expect(action).toHaveBeenCalledTimes(1);
  });

  it("lazyProviders_should_registerOnlyWhenResolvedAndReleaseAfter", () => {
    const provider = jest.fn(() => [{ id: "r1", shortcut: { ...base, label: "Reply 1", sequence: ["n", "r", "1"] } }]);
    reg.registerLazy("reply", ["n"], provider);
    expect(provider).not.toHaveBeenCalled();
    reg.resolveLazy(["w"]);
    expect(provider).not.toHaveBeenCalled();
    reg.resolveLazy(["n"]);
    expect(reg.getPalette().map((e) => e.id)).toEqual(["r1"]);
    reg.releaseLazy();
    expect(reg.getPalette()).toEqual([]);
  });

  it.each([["leafFirst"], ["branchFirst"]])("getTree_should_preferBranchOverPrefixLeaf_%s", (order) => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => {});
    const leaf = () => reg.register("leaf", { ...base, label: "Leaf", sequence: ["n", "r"] });
    const branch = () => reg.register("branch", { ...base, label: "Reply 1", sequence: ["n", "r", "1"] });
    if (order === "leafFirst") { leaf(); branch(); } else { branch(); leaf(); }
    const r = reg.getTree()[0].children[0];
    expect(r.key).toBe("r");
    expect(r.children.map((c) => c.key)).toEqual(["1"]);
    expect(r.shortcut).toBeUndefined();
    warn.mockRestore();
  });
});
