import { parseDiff, type DiffLine } from "@/lib/utils/parseDiff";
import { buildHunkList, hunkDomId, pairSplitRows, stepHunkIndex } from "./diffNavigation";

const l = (type: DiffLine["type"], content: string): DiffLine => ({ type, content });

describe("buildHunkList", () => {
  it("flattens hunks across files in order", () => {
    const files = parseDiff(
      [
        "diff --git a/a.ts b/a.ts", "--- a/a.ts", "+++ b/a.ts", "@@ -1,1 +1,1 @@", "-x", "+y", "@@ -9,1 +9,1 @@", "-p", "+q",
        "diff --git a/b.ts b/b.ts", "--- a/b.ts", "+++ b/b.ts", "@@ -1,1 +1,1 @@", "-m", "+n",
      ].join("\n"),
    );
    expect(buildHunkList(files)).toEqual([
      { fileIndex: 0, hunkIndex: 0 },
      { fileIndex: 0, hunkIndex: 1 },
      { fileIndex: 1, hunkIndex: 0 },
    ]);
  });
  it("is empty for no files", () => expect(buildHunkList([])).toEqual([]));
});

describe("stepHunkIndex", () => {
  it("starts at the first hunk going next and the last going prev", () => {
    expect(stepHunkIndex(null, 3, "next")).toBe(0);
    expect(stepHunkIndex(null, 3, "prev")).toBe(2);
  });
  it("wraps in both directions", () => {
    expect(stepHunkIndex(2, 3, "next")).toBe(0);
    expect(stepHunkIndex(0, 3, "prev")).toBe(2);
  });
  it("moves one step otherwise", () => {
    expect(stepHunkIndex(0, 3, "next")).toBe(1);
    expect(stepHunkIndex(2, 3, "prev")).toBe(1);
  });
  it("returns null with no hunks and recovers from a stale out-of-range index", () => {
    expect(stepHunkIndex(1, 0, "next")).toBeNull();
    expect(stepHunkIndex(7, 3, "next")).toBe(0);
  });
});

describe("hunkDomId", () => {
  it("encodes file and hunk index", () => expect(hunkDomId(2, 5)).toBe("diff-hunk-2-5"));
});

describe("pairSplitRows", () => {
  it("duplicates context onto both sides", () => {
    const c = l("context", " a");
    expect(pairSplitRows([c])).toEqual([{ left: c, right: c, leftIdx: 0, rightIdx: 0 }]);
  });
  it("pads the right side for delete-only runs", () => {
    const d = l("delete", "-a");
    expect(pairSplitRows([d])).toEqual([{ left: d, right: null, leftIdx: 0, rightIdx: -1 }]);
  });
  it("pads the left side for add-only runs", () => {
    const a = l("add", "+a");
    expect(pairSplitRows([a])).toEqual([{ left: null, right: a, leftIdx: -1, rightIdx: 0 }]);
  });
  it("pairs a mixed 3-delete / 1-add run index by index", () => {
    const [d1, d2, d3] = [l("delete", "-1"), l("delete", "-2"), l("delete", "-3")];
    const a1 = l("add", "+1");
    expect(pairSplitRows([d1, d2, d3, a1])).toEqual([
      { left: d1, right: a1, leftIdx: 0, rightIdx: 3 },
      { left: d2, right: null, leftIdx: 1, rightIdx: -1 },
      { left: d3, right: null, leftIdx: 2, rightIdx: -1 },
    ]);
  });
  it("keeps rows aligned after an asymmetric block", () => {
    const c = l("context", " c");
    const rows = pairSplitRows([l("delete", "-1"), l("delete", "-2"), l("add", "+1"), c]);
    expect(rows).toHaveLength(3);
    expect(rows[2]).toEqual({ left: c, right: c, leftIdx: 3, rightIdx: 3 });
  });
});
