import { barReducer, CLOSED_STATE, filterRows, virtualWindow, VIRTUALIZE_THRESHOLD, type PaletteRow } from "../commandBarState";

const row = (label: string, keys: string[] = [], group = ""): PaletteRow => ({ id: label, label, group, keys, run: () => {} });

describe("barReducer", () => {
  it("open_then_descend_then_back_should_walkLevels", () => {
    let s = barReducer(CLOSED_STATE, { type: "open", path: [] });
    s = barReducer(s, { type: "descend", key: "w" });
    expect(s.path).toEqual(["w"]);
    s = barReducer(s, { type: "back" });
    expect(s.path).toEqual([]);
  });

  it("move_should_wrapAround", () => {
    let s = barReducer(CLOSED_STATE, { type: "open", path: [] });
    s = barReducer(s, { type: "move", delta: -1, count: 3 });
    expect(s.activeIndex).toBe(2);
    s = barReducer(s, { type: "move", delta: 1, count: 3 });
    expect(s.activeIndex).toBe(0);
  });
});

describe("filterRows", () => {
  const rows = [row("Toggle Quiet mode", ["n", "q"], "Notifications"), row("Next window", ["w", "n"], "Windows"), row("Rename window")];

  it("should_matchCaseInsensitiveSubstring", () => {
    expect(filterRows(rows, "QUIET").map((r) => r.label)).toEqual(["Toggle Quiet mode"]);
  });

  it("should_rankLabelPrefixFirst", () => {
    expect(filterRows(rows, "n").map((r) => r.label)[0]).toBe("Next window");
  });

  it("should_returnAllOnEmptyQuery", () => {
    expect(filterRows(rows, "")).toHaveLength(3);
  });
});

describe("virtualWindow", () => {
  it("should_renderEverythingUnderThreshold", () => {
    expect(virtualWindow(VIRTUALIZE_THRESHOLD, 500, 352, 44)).toEqual({ start: 0, end: VIRTUALIZE_THRESHOLD, padTop: 0, padBottom: 0 });
  });

  it("should_renderOnlyWindowWhenOverThreshold", () => {
    const w = virtualWindow(1000, 4400, 352, 44, 105);
    expect(w.end - w.start).toBeLessThan(40);
    expect(w.padTop + (w.end - w.start) * 44 + w.padBottom).toBe(1000 * 44);
  });
});

import { interpretKey, type Row } from "../commandBarState";
import type { ShortcutTree } from "@/lib/shortcuts/shortcutRegistry";

describe("interpretKey", () => {
  const tree: ShortcutTree = [
    { key: "w", label: "Windows", children: [{ key: "n", label: "Next window", children: [], shortcut: { label: "Next window", context: "global", action: () => {} } }] },
  ];
  const rootRows: Row[] = [{ id: "w", label: "Windows", group: "", keys: ["w"], branchKey: "w" }];
  const k = (key: string) => ({ key, ctrlKey: false, metaKey: false, altKey: false });
  const open = { ...CLOSED_STATE, open: true };

  it("boundBranchKey_should_executeRow", () => {
    expect(interpretKey(k("w"), open, rootRows, tree)).toEqual({ type: "execute", row: rootRows[0] });
  });

  it("unboundPrintable_should_seedFuzzyQuery", () => {
    expect(interpretKey(k("z"), open, rootRows, tree)).toEqual({ type: "seedQuery", char: "z" });
  });

  it("jk_should_navigateInWhichKeyButTypeInFuzzy", () => {
    expect(interpretKey(k("j"), open, rootRows, tree)).toEqual({ type: "action", action: { type: "move", delta: 1, count: 1 } });
    expect(interpretKey(k("j"), { ...open, query: "a" }, rootRows, tree)).toEqual({ type: "action", action: { type: "setQuery", query: "aj" } });
  });

  it("escape_should_backOutThenClose", () => {
    expect(interpretKey(k("Escape"), { ...open, path: ["w"] }, rootRows, tree)).toEqual({ type: "action", action: { type: "back" } });
    expect(interpretKey(k("Escape"), open, rootRows, tree)).toEqual({ type: "action", action: { type: "close" } });
  });

  it("unknownNonPrintable_should_beNone", () => {
    expect(interpretKey(k("F5"), open, rootRows, tree)).toEqual({ type: "none" });
  });
});
