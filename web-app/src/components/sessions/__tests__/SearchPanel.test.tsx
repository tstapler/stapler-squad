import React from "react";
import { act, render, screen, within } from "@testing-library/react";
import type { SearchHit } from "@/lib/files/searchClient";
import { SearchResults } from "../files/SearchResults";
import { accessibleName, buildRows, type ResultRow } from "../files/searchRows";

const hit = (path: string, line: number, text = `func foo_bar(x int) // ${line}`): SearchHit => ({ path, line, text, start: text.indexOf("foo_bar"), end: text.indexOf("foo_bar") + 7, byteOffset: 0 });

/** 27 matches over 6 files: 5, 5, 5, 5, 4, 3. */
function sample(): SearchHit[] {
  const counts = [5, 5, 5, 5, 4, 3];
  return counts.flatMap((n, f) => Array.from({ length: n }, (_, i) => hit(`pkg/f${f + 1}.go`, 10 + i)));
}

/** jsdom has no layout; give the scroll container a viewport so the virtualizer mounts a window. */
function withViewport<T>(fn: () => T): T {
  const proto = HTMLElement.prototype;
  const old = { h: Object.getOwnPropertyDescriptor(proto, "offsetHeight"), w: Object.getOwnPropertyDescriptor(proto, "offsetWidth") };
  Object.defineProperty(proto, "offsetHeight", { configurable: true, get: () => 400 });
  Object.defineProperty(proto, "offsetWidth", { configurable: true, get: () => 300 });
  try { return fn(); } finally {
    if (old.h) Object.defineProperty(proto, "offsetHeight", old.h); else delete (proto as { offsetHeight?: unknown }).offsetHeight;
    if (old.w) Object.defineProperty(proto, "offsetWidth", old.w); else delete (proto as { offsetWidth?: unknown }).offsetWidth;
  }
}

const baseProps = { stale: false, busy: false, activeKey: null as string | null, dirtyPaths: new Set<string>(), coarse: false, treeId: "t1", onToggleGroup: jest.fn(), onActivate: jest.fn() };

describe("buildRows", () => {
  it("buildRows_should_GroupByFileInFirstSeenOrderWithGlobalOrdinals", () => {
    const rows = buildRows(sample(), new Set());
    const groups = rows.filter((r): r is Extract<ResultRow, { kind: "group" }> => r.kind === "group");
    expect(groups.map((g) => g.count)).toEqual([5, 5, 5, 5, 4, 3]);
    expect(groups.map((g) => [g.posinset, g.setsize])).toEqual([[1, 6], [2, 6], [3, 6], [4, 6], [5, 6], [6, 6]]);
    const hits = rows.filter((r): r is Extract<ResultRow, { kind: "hit" }> => r.kind === "hit");
    expect(hits).toHaveLength(27);
    expect(hits[0]).toMatchObject({ posinset: 1, setsize: 5, result: 1, total: 27 });
    expect(hits[26]).toMatchObject({ posinset: 3, setsize: 3, result: 27, total: 27 });
  });

  it("buildRows_should_OmitHitsOfCollapsedGroupsButKeepNumbering", () => {
    const rows = buildRows(sample(), new Set(["pkg/f1.go"]));
    expect(rows.filter((r) => r.kind === "hit")).toHaveLength(22);
    const first = rows.find((r) => r.kind === "hit") as Extract<ResultRow, { kind: "hit" }>;
    expect(first.result).toBe(6);
    expect(first.total).toBe(27);
  });

  it("accessibleName_should_FollowTheDocumentedFormat", () => {
    const rows = buildRows([hit("src/a.go", 12), hit("src/a.go", 40)], new Set());
    const row = rows[1] as Extract<ResultRow, { kind: "hit" }>;
    expect(accessibleName(row, false)).toBe("src/a.go line 12, func foo_bar(x int) // 12, match 1 of 2, result 1 of 2");
    expect(accessibleName(row, true)).toBe("src/a.go line 12, func foo_bar(x int) // 12, match 1 of 2, result 1 of 2, buffer has unsaved changes; line may differ");
    expect(accessibleName(rows[0] as Extract<ResultRow, { kind: "group" }>, false, false)).toBe("src/a.go, 2 matches, expanded");
    expect(accessibleName(rows[0] as Extract<ResultRow, { kind: "group" }>, false, true)).toBe("src/a.go, 2 matches, collapsed");
  });
});

describe("SearchResults", () => {
  it("searchResults_should_GroupAndAnnounceCounts_When_27MatchesIn6Files (rows half)", () => {
    withViewport(() => {
      const rows = buildRows(sample(), new Set());
      render(<SearchResults {...baseProps} rows={rows} />);
    });
    const tree = screen.getByRole("tree", { name: "Search results" });
    const first = within(tree).getAllByRole("treeitem")[0];
    expect(first).toHaveAttribute("aria-level", "1");
    expect(first).toHaveAttribute("aria-expanded", "true");
    const mark = within(tree).getAllByText("foo_bar")[0];
    expect(mark.tagName).toBe("MARK");
  });

  it("searchResults_should_ExposeValidTreeAria_When_VirtualizedWindowMounted", () => {
    const many = Array.from({ length: 5000 }, (_, i) => hit(`pkg/f${Math.floor(i / 50)}.go`, (i % 50) + 1));
    const rows = buildRows(many, new Set());
    expect(rows.length).toBe(5000 + 100);
    let view!: ReturnType<typeof render>;
    withViewport(() => {
      view = render(<SearchResults {...baseProps} rows={rows} busy />);
    });
    const tree = screen.getByRole("tree", { name: "Search results" });
    expect(tree).toHaveAttribute("aria-busy", "true");
    const items = within(tree).getAllByRole("treeitem");
    expect(items.length).toBeGreaterThan(0);
    expect(items.length).toBeLessThan(100);
    for (const el of items) {
      expect(el).toHaveAttribute("aria-level");
      expect(el).toHaveAttribute("aria-setsize");
      expect(el).toHaveAttribute("aria-posinset");
      expect(el.getAttribute("aria-label")).toMatch(/pkg\/f\d+\.go/);
    }
    const hitRow = items.find((el) => el.getAttribute("aria-level") === "2")!;
    expect(hitRow.getAttribute("aria-label")).toMatch(/^pkg\/f0\.go line 1, .*, match 1 of 50, result 1 of 5000$/);
    // The axe run (no aria-required-*, aria-valid-attr-value, nested-interactive) is the Playwright case of Task 1.5.3b:
    // axe-core is not a declared web-app dependency. These are the same three rules, checked structurally.
    const stray = Array.from(view.container.querySelectorAll("[role]")).filter((el) => !["tree", "treeitem"].includes(el.getAttribute("role")!));
    expect(stray).toHaveLength(0);
    expect(tree.querySelectorAll("button, a, input, select, textarea, [tabindex='0']")).toHaveLength(0);
    for (const el of items) {
      expect(el.hasAttribute("aria-expanded")).toBe(el.getAttribute("aria-level") === "1");
      expect(Number(el.getAttribute("aria-posinset"))).toBeLessThanOrEqual(Number(el.getAttribute("aria-setsize")));
      expect(el.id).toMatch(/^t1-[gh]:/);
    }
    const ids = items.map((el) => el.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("searchResults_should_MakeRowsFocusableAnd44px_When_PointerCoarse", () => {
    withViewport(() => {
      render(<SearchResults {...baseProps} coarse rows={buildRows(sample(), new Set())} />);
    });
    const hitRow = screen.getAllByRole("treeitem").find((el) => el.getAttribute("aria-level") === "2")!;
    expect(hitRow).toHaveAttribute("tabindex", "-1");
    expect(hitRow).toHaveStyle({ minHeight: "44px" });
  });

  it("searchResults_should_MarkDirtyBufferHits_When_PathHasUnsavedBuffer", () => {
    withViewport(() => {
      render(<SearchResults {...baseProps} dirtyPaths={new Set(["pkg/f1.go"])} rows={buildRows(sample(), new Set())} />);
    });
    const dirtyRow = screen.getAllByRole("treeitem").find((el) => el.getAttribute("aria-level") === "2")!;
    expect(dirtyRow.getAttribute("aria-label")).toContain("buffer has unsaved changes; line may differ");
    expect(within(dirtyRow).getByText(/buffer has unsaved changes; line may differ/)).toBeInTheDocument();
  });

  it("searchResults_should_ReportActivationAndToggle_When_RowsClicked", async () => {
    const onActivate = jest.fn();
    const onToggleGroup = jest.fn();
    withViewport(() => {
      render(<SearchResults {...baseProps} onActivate={onActivate} onToggleGroup={onToggleGroup} rows={buildRows(sample(), new Set())} />);
    });
    const [group, firstHit] = screen.getAllByRole("treeitem");
    await act(async () => { group.click(); firstHit.click(); });
    expect(onToggleGroup).toHaveBeenCalledWith("pkg/f1.go");
    expect(onActivate).toHaveBeenCalledWith(expect.objectContaining({ line: 10, path: "pkg/f1.go" }), 0, "tap");
  });
});
