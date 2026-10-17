import React from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { Code, ConnectError } from "@connectrpc/connect";
import type { SearchHit } from "@/lib/files/searchClient";
import { createFilesTestHarness, installVirtualViewport as installViewport } from "@/lib/files/__tests__/fixtures";
import { controllableSearchStream, searchHit, searchStats } from "@/lib/files/__tests__/fixtures/controllableSearchStream";
import { SearchPanel, type SearchPanelHandle } from "../files/SearchPanel";
import { QuickfixChip, SearchResults, type QuickfixView } from "../files/SearchResults";
import { accessibleName, buildRows, type ResultRow } from "../files/searchRows";

const hit = (path: string, line: number, text = `func foo_bar(x int) // ${line}`): SearchHit => ({ path, line, text, start: text.indexOf("foo_bar"), end: text.indexOf("foo_bar") + 7, byteOffset: 0 });

/** 27 matches over 6 files: 5, 5, 5, 5, 4, 3. */
function sample(): SearchHit[] {
  const counts = [5, 5, 5, 5, 4, 3];
  return counts.flatMap((n, f) => Array.from({ length: n }, (_, i) => hit(`pkg/f${f + 1}.go`, 10 + i)));
}

function withViewport<T>(fn: () => T): T {
  const restore = installViewport();
  try { return fn(); } finally { restore(); }
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


describe("SearchPanel", () => {
  const flush = () => act(async () => { await Promise.resolve(); await Promise.resolve(); });

  function mount(extra: Partial<React.ComponentProps<typeof SearchPanel>> = {}) {
    const h = createFilesTestHarness();
    const c = controllableSearchStream();
    const openHit = jest.fn(() => false);
    const onEscape = jest.fn();
    const panelRef = React.createRef<SearchPanelHandle>();
    const quickfix: { last: QuickfixView | null } = { last: null };
    const onQuickfix = jest.fn((q: QuickfixView | null) => { quickfix.last = q; });
    const props = { sessionId: "s1", sessionName: "my-session", visible: true, focusToken: 0, dirtyPaths: new Set<string>(), openHit, onEscape, onQuickfix, stream: c.stream, ...extra };
    const view = render(<SearchPanel ref={panelRef} {...props} />, { wrapper: h.wrapper });
    const input = screen.getByRole("searchbox", { name: "Search file contents" });
    const type = (value: string) => fireEvent.change(input, { target: { value } });
    const fire = async (value: string) => { type(value); act(() => { jest.advanceTimersByTime(300); }); await flush(); };
    return { h, c, openHit, onEscape, input, type, fire, view, panelRef, quickfix, rerender: (p: Partial<typeof props>) => view.rerender(<SearchPanel ref={panelRef} {...props} {...p} />) };
  }
  const status = () => screen.getByTestId("search-status");

  let restoreViewport: () => void = () => undefined;
  beforeEach(() => { jest.useFakeTimers(); restoreViewport = installViewport(); });
  afterEach(() => { restoreViewport(); jest.useRealTimers(); });

  it("searchResults_should_GroupAndAnnounceCounts_When_27MatchesIn6Files", async () => {
    const { c, fire, input } = mount();
    await fire("foo");
    input.focus();
    act(() => c.requests[0].push({ type: "batch", matches: sample() }));
    act(() => c.requests[0].push(searchStats({ filesScanned: 40 })));
    await flush();
    expect(status()).toHaveTextContent("27 results in 6 files");
    expect(status()).toHaveAttribute("aria-live", "polite");
    expect(screen.getByRole("tree", { name: "Search results" })).toBeInTheDocument();
    expect(document.activeElement).toBe(input);
  });

  it("searchPanel_should_ShowMinCharsHintAndSendOneRequest_When_TypingOneThenFoo", async () => {
    const { c, type, fire } = mount();
    expect(screen.getByText("Search file contents in my-session")).toBeInTheDocument();
    type("f");
    act(() => { jest.advanceTimersByTime(1000); });
    expect(screen.getByText("Type at least 2 characters")).toBeInTheDocument();
    expect(c.requests).toHaveLength(0);
    type("fo"); type("foo");
    act(() => { jest.advanceTimersByTime(300); });
    await flush();
    expect(c.requests.map((r) => r.pattern)).toEqual(["foo"]);
    void fire;
  });

  it("searchPanel_should_ShowInlineErrorKeepResults_When_InvalidRegex", async () => {
    const { c, fire } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1)] }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    fireEvent.click(screen.getByRole("button", { name: "Use regular expression" }));
    await flush();
    act(() => c.requests[1].fail(new ConnectError("error parsing regexp: missing closing )", Code.InvalidArgument)));
    await flush();
    expect(screen.getByText(/Invalid regular expression: error parsing regexp: missing closing \)/)).toBeInTheDocument();
    expect(screen.getAllByRole("treeitem").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Use regular expression" }));
    await flush();
    expect(screen.queryByText(/Invalid regular expression/)).toBeNull();
    expect(c.requests[2].query.regex).toBe(false);
  });

  it("searchPanel_should_ListActiveTogglesAndClearFilters_When_NoMatches", async () => {
    const { c, fire } = mount();
    fireEvent.click(screen.getByRole("button", { name: "Match case" }));
    fireEvent.click(screen.getByRole("button", { name: "Use regular expression" }));
    await fire("foo");
    act(() => c.requests[0].push(searchStats()));
    await flush();
    expect(screen.getByText("No matches for `foo` (case-sensitive, regex)")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    await flush();
    expect(c.requests.at(-1)!.query).toMatchObject({ pattern: "foo", caseSensitive: false, regex: false });
    expect(screen.getByRole("button", { name: "Match case" })).toHaveAttribute("aria-pressed", "false");
  });

  it("searchPanel_should_FocusIncludeField_When_AddFiltersClickedOnTruncated", async () => {
    const { c, fire } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1)] }));
    act(() => c.requests[0].push(searchStats({ truncated: true })));
    await flush();
    expect(screen.getByText("Showing first 1000 - refine your query")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add filters" }));
    expect(screen.getByLabelText("Files to include")).toHaveFocus();
  });

  it("searchPanel_should_ShowPausedStateWithResume_When_WorktreeUnavailable", async () => {
    const onResumeSession = jest.fn();
    const { c, fire } = mount({ onResumeSession });
    await fire("foo");
    act(() => c.requests[0].fail(new ConnectError("WorktreeMissing: session worktree is not on disk", Code.FailedPrecondition)));
    await flush();
    expect(screen.getByText("Session paused; worktree unavailable")).toBeInTheDocument();
    expect(screen.queryByRole("tree")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Resume session" }));
    expect(onResumeSession).toHaveBeenCalledTimes(1);
  });

  it("searchPanel_should_ShowWaitingAheadAndCancel_When_QueuedState", async () => {
    const { c, fire } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1)] }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    await fire("foob");
    act(() => c.requests[1].push({ type: "queued", ahead: 1 }));
    await flush();
    expect(status()).toHaveTextContent("Waiting for other searches (1 ahead)...");
    expect(screen.getByRole("tree")).toHaveClass("treeDimmed");
    fireEvent.click(screen.getByTestId("search-cancel"));
    expect(c.requests[1].signal.aborted).toBe(true);
    expect(screen.queryByTestId("search-cancel")).toBeNull();
  });

  it("searchPanel_should_KeepPartialResultsAndSayCancelled_When_CancelPressed", async () => {
    const { c, fire } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: Array.from({ length: 312 }, (_, i) => searchHit(`p/f${i % 6}.go`, i + 1)) }));
    await flush();
    fireEvent.click(screen.getByTestId("search-cancel"));
    expect(status()).toHaveTextContent("Search cancelled - 312 results so far");
    expect(screen.getAllByRole("treeitem").length).toBeGreaterThan(0);
  });

  it("searchPanel_should_ShowRetryAndKeepResults_When_RpcFailsOrBusy", async () => {
    const { c, fire } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1)] }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    await fire("foob");
    act(() => c.requests[1].fail(new ConnectError("down", Code.Unavailable)));
    await flush();
    expect(screen.getByText("Search failed - check your connection")).toBeInTheDocument();
    expect(screen.getAllByRole("treeitem").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await flush();
    act(() => c.requests[2].fail(new ConnectError("busy", Code.ResourceExhausted)));
    await flush();
    expect(screen.getByText("Too many searches are running - try again in a moment")).toBeInTheDocument();
    act(() => { jest.advanceTimersByTime(60_000); });
    expect(c.requests).toHaveLength(3);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await flush();
    expect(c.requests).toHaveLength(4);
  });

  it("searchPanel_should_OpenActiveHitOnEnterAndEmitOpenHitWithThreeInteractions_When_OpenTypeEnter", async () => {
    const openHit = jest.fn(() => true);
    const { h, c, fire, input } = mount({ openHit });
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 12), searchHit("a.go", 30)] }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(input).toHaveAttribute("aria-activedescendant", screen.getAllByRole("treeitem")[1].id);
    fireEvent.keyDown(input, { key: "Enter" });
    expect(openHit).toHaveBeenCalledWith(expect.objectContaining({ path: "a.go", line: 12 }), "enter");
    const open = h.analytics.events.filter((e) => e.name === "files.search.open_hit");
    expect(open).toHaveLength(1);
    expect(open[0].labels).toMatchObject({ rank: "1", interactions: "3", via: "enter", reused_tab: "true" });
    expect(h.analytics.events.filter((e) => e.name === "files.search.results")).toHaveLength(1);
  });

  it("searchPanel_should_EmitNoOpenHit_When_FlowCancelledBeforeAHit", async () => {
    const { h, c, fire } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1)] }));
    await flush();
    fireEvent.click(screen.getByTestId("search-cancel"));
    expect(h.analytics.events.filter((e) => e.name === "files.search.open_hit")).toHaveLength(0);
  });

  it("searchPanel_should_CollapseGroupsAndReturnFocusToEditor_When_ToggledOrEscape", async () => {
    const { c, fire, input, onEscape } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: sample() }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    expect(screen.getAllByRole("treeitem")[1]).toHaveAttribute("aria-level", "2");
    fireEvent.click(screen.getAllByRole("treeitem")[0]);
    expect(screen.getAllByRole("treeitem")[0]).toHaveAttribute("aria-expanded", "false");
    expect(screen.getAllByRole("treeitem")[1]).toHaveAttribute("aria-level", "1");
    fireEvent.keyDown(input, { key: "Escape" });
    expect(onEscape).toHaveBeenCalledTimes(1);
  });

  it("searchPanel_should_FocusQueryAndRetainState_When_FocusTokenBumpedAndPanelHiddenAndShown", async () => {
    const { c, fire, input, rerender } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1)] }));
    await flush();
    rerender({ visible: false });
    expect(c.requests[0].signal.aborted).toBe(true);
    rerender({ visible: true, focusToken: 1 });
    expect(input).toHaveFocus();
    expect(input).toHaveValue("foo");
    expect(screen.getAllByRole("treeitem").length).toBeGreaterThan(0);
  });

  it("searchPanel_should_ShowIdleHelperOrDimmedRetainedResults_When_NoQuery", async () => {
    const { c, fire, type, h } = mount();
    expect(screen.getByTestId("search-idle")).toHaveTextContent("Type at least 2 characters");
    expect(screen.getByTestId("search-idle")).toHaveTextContent("Search file contents in my-session");
    expect(c.requests).toHaveLength(0);
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1)] }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    type("");
    act(() => { jest.advanceTimersByTime(300); });
    await flush();
    expect(screen.queryByTestId("search-idle")).toBeNull();
    expect(screen.getByTestId("search-retained")).toHaveTextContent("Results for `foo` - edit to search again");
    expect(screen.getByTestId("search-results")).toHaveClass("treeDimmed");
    expect(c.requests).toHaveLength(1);
    expect(h.analytics.events.filter((e) => e.name === "files.search.run")).toHaveLength(1);
  });

  it("searchPanel_should_OpenOnTapWhileQueryFocused_And_IgnoreScroll_When_KeyboardOpen", async () => {
    class P extends MouseEvent { pointerId: number; constructor(t: string, i: MouseEventInit & { pointerId?: number } = {}) { super(t, i); this.pointerId = i.pointerId ?? 1; } }
    (window as unknown as { PointerEvent: unknown }).PointerEvent = P;
    const openHit = jest.fn(() => false);
    const { c, fire, input } = mount({ openHit });
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1), searchHit("a.go", 2), searchHit("a.go", 3)] }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    input.focus();
    const row3 = screen.getAllByRole("treeitem")[3];
    fireEvent.pointerDown(row3, { clientX: 5, clientY: 5, pointerId: 1 });
    expect(document.activeElement).not.toBe(input);
    fireEvent.pointerUp(row3, { clientX: 5, clientY: 5, pointerId: 1 });
    fireEvent.click(row3);
    expect(openHit).toHaveBeenCalledTimes(1);
    expect(openHit).toHaveBeenCalledWith(expect.objectContaining({ line: 3 }), "tap");
    const row1 = screen.getAllByRole("treeitem")[1];
    fireEvent.pointerDown(row1, { clientX: 5, clientY: 5, pointerId: 1 });
    fireEvent.pointerUp(row1, { clientX: 5, clientY: 40, pointerId: 1 });
    expect(openHit).toHaveBeenCalledTimes(1);
  });

  it("quickfixCursor_should_StepHits_When_F4AndShiftF4Pressed", async () => {
    const { c, fire, openHit, panelRef, quickfix } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: sample() }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    // open hit 3 of 27 by tapping its row (rows: group, hit1, hit2, hit3 ...)
    fireEvent.click(screen.getAllByRole("treeitem")[3]);
    expect(openHit).toHaveBeenLastCalledWith(expect.objectContaining({ path: "pkg/f1.go", line: 12 }), "tap");
    expect(quickfix.last).toMatchObject({ position: 3, total: 27 });
    act(() => panelRef.current!.step(1));
    expect(openHit).toHaveBeenLastCalledWith(expect.objectContaining({ path: "pkg/f1.go", line: 13 }), "quickfix");
    expect(quickfix.last).toMatchObject({ position: 4, total: 27 });
    act(() => panelRef.current!.step(-1));
    expect(openHit).toHaveBeenLastCalledWith(expect.objectContaining({ line: 12 }), "quickfix");
    expect(quickfix.last).toMatchObject({ position: 3 });
  });

  it("quickfix_should_StepAndAnnounceWrap_When_F4ShiftF4OrChipChevrons", async () => {
    const { c, fire, panelRef, quickfix } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1), searchHit("a.go", 2), searchHit("b.go", 3)] }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    act(() => panelRef.current!.step(-1));
    expect(quickfix.last).toMatchObject({ position: 3, total: 3 });
    act(() => panelRef.current!.step(1));
    expect(quickfix.last).toMatchObject({ position: 1, note: "Wrapped to first hit" });
    act(() => panelRef.current!.step(-1));
    expect(quickfix.last).toMatchObject({ position: 3, note: "Wrapped to last hit" });
    act(() => quickfix.last!.dismiss());
    expect(quickfix.last).toBeNull();
  });

  it("quickfix_should_ResetWhenANewSearchStarts_And_RenderChipControls", async () => {
    const { c, fire, panelRef, quickfix } = mount();
    await fire("foo");
    act(() => c.requests[0].push({ type: "batch", matches: [searchHit("a.go", 1), searchHit("a.go", 2)] }));
    act(() => c.requests[0].push(searchStats()));
    await flush();
    act(() => panelRef.current!.step(1));
    const onStep = jest.fn();
    const onDismiss = jest.fn();
    const chip = render(<QuickfixChip view={{ position: 3, total: 27, note: null, step: onStep, dismiss: onDismiss }} />);
    expect(chip.getByText("Hit 3/27")).toBeInTheDocument();
    fireEvent.click(chip.getByRole("button", { name: "Next hit" }));
    fireEvent.click(chip.getByRole("button", { name: "Previous hit" }));
    fireEvent.click(chip.getByRole("button", { name: "Dismiss hit navigation" }));
    expect(onStep.mock.calls).toEqual([[1], [-1]]);
    expect(onDismiss).toHaveBeenCalledTimes(1);
    await fire("foob");
    act(() => c.requests[1].push({ type: "batch", matches: [searchHit("a.go", 9)] }));
    await flush();
    expect(quickfix.last).toBeNull();
  });
});
