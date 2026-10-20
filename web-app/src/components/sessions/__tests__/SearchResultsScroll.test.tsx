import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { SearchResults } from "../files/SearchResults";
import { buildRows } from "../files/searchRows";

let resize: ((entries: { contentRect: { height: number } }[]) => void) | null = null;
const originalRO = globalThis.ResizeObserver;

beforeEach(() => {
  globalThis.ResizeObserver = class {
    constructor(cb: never) { resize = cb; }
    observe() {}
    disconnect() {}
    unobserve() {}
  } as never;
});
afterEach(() => { globalThis.ResizeObserver = originalRO; });

const hits = Array.from({ length: 40 }, (_, i) => ({ path: `pkg/f${i}.go`, line: i + 1, text: "needle_fn()", start: 0, end: 9 }));

describe("SearchResults scroll", () => {
  it("searchResults_should_RestoreScrollOffset_When_PaneShownAgainAfterDisplayNone", () => {
    render(
      <SearchResults
        rows={buildRows(hits as never, new Set())}
        stale={false} busy={false} activeKey={null} dirtyPaths={new Set()} coarse treeId="t"
        onToggleGroup={jest.fn()} onActivate={jest.fn()}
      />,
    );
    const tree = screen.getByTestId("search-results");
    // Visible: the user scrolls to 480.
    Object.defineProperty(tree, "clientHeight", { configurable: true, value: 600 });
    tree.scrollTop = 480;
    fireEvent.scroll(tree);

    // Hidden by the pane switch: the browser drops the offset.
    act(() => resize?.([{ contentRect: { height: 0 } }]));
    Object.defineProperty(tree, "clientHeight", { configurable: true, value: 0 });
    tree.scrollTop = 0;
    fireEvent.scroll(tree);

    // Shown again: the offset comes back.
    act(() => resize?.([{ contentRect: { height: 600 } }]));
    expect(tree.scrollTop).toBe(480);
  });
});
