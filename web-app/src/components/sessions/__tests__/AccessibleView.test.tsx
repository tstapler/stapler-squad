import { act, render, screen, fireEvent } from "@testing-library/react";
import { AccessibleView } from "../files/AccessibleView";
import type { ReadViewHandle } from "../files/readViewHandle";
import { paginateLines, textPageSource } from "@/lib/files/pageSource";

const doc = (n: number) => Array.from({ length: n }, (_, i) => `row ${i + 1}`).join("\n");

function mount(props: Partial<React.ComponentProps<typeof AccessibleView>> = {}, lines = 1200) {
  let handle: ReadViewHandle | undefined;
  const utils = render(
    <AccessibleView path="pkg/b.go" source={textPageSource(doc(lines))} onReady={(h) => { handle = h; }} {...props} />,
  );
  return { ...utils, handle: () => handle as ReadViewHandle };
}

describe("AccessibleView", () => {
  it("accessibleView_should_FocusAndAnnounceLine_When_SearchHitGoToLineOrQuickfixTargetsAnotherPage", () => {
    const { handle } = mount();
    act(() => handle().gotoLine(1034));
    expect(screen.getByTestId("accessible-view-label").textContent).toBe("Lines 1001-1200 of 1200");
    const line = document.querySelector<HTMLElement>('[data-line="1034"]');
    expect(line).not.toBeNull();
    expect(document.activeElement).toBe(line);
    expect(screen.getByTestId("accessible-view-announce").textContent).toBe("Line 1034");
    expect(handle().getCursor().line).toBe(1034);

    // Jumplist Back arrives as another gotoLine for the earlier location: previous page and line return.
    act(() => handle().gotoLine(7));
    expect(screen.getByTestId("accessible-view-label").textContent).toBe("Lines 1-500 of 1200");
    expect(document.activeElement).toBe(document.querySelector('[data-line="7"]'));
    expect(screen.getByTestId("accessible-view-announce").textContent).toBe("Line 7");

    // A line past the end clamps to the last line.
    act(() => handle().gotoLine(99999));
    expect(document.activeElement).toBe(document.querySelector('[data-line="1200"]'));

    // The Go to line field takes the same path.
    fireEvent.change(screen.getByLabelText("Go to line"), { target: { value: "501" } });
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    expect(screen.getByTestId("accessible-view-label").textContent).toBe("Lines 501-1000 of 1200");
    expect(screen.getByTestId("accessible-view-announce").textContent).toBe("Line 501");
  });

  it("accessibleView_should_RenderDraftWithUnsavedLabelAndSave_When_BufferDirty", () => {
    const draft = "edited line\nsecond";
    const { rerender } = render(<AccessibleView path="pkg/b.go" source={textPageSource(draft)} dirty saveDisabledReason="saving is not available yet" />);
    expect(screen.getByRole("region", { name: "b.go, accessible view, unsaved changes" })).toBeInTheDocument();
    expect(screen.getAllByTestId("accessible-view-line")[0].textContent).toBe("1 edited line");
    expect(screen.getByText("Unsaved changes")).toBeVisible();
    const save = screen.getByRole("button", { name: "Save" });
    expect(save).toHaveStyle({ minHeight: "44px", minWidth: "44px" });
    expect(save).toHaveAttribute("aria-disabled", "true");
    expect(save).toHaveAccessibleDescription("saving is not available yet");

    const onSave = jest.fn();
    rerender(<AccessibleView path="pkg/b.go" source={textPageSource(draft)} dirty onSave={onSave} />);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSave).toHaveBeenCalledTimes(1);

    rerender(<AccessibleView path="pkg/b.go" source={textPageSource(draft)} />);
    expect(screen.getByRole("region", { name: "b.go, accessible view" })).toBeInTheDocument();
    expect(screen.queryByText("Unsaved changes")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("accessibleView_should_RenderEditButtonOnlyWhenOnEditRequestedPassed_InWave6Stub", () => {
    const { unmount } = mount();
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
    unmount();

    const onEditRequested = jest.fn();
    const { handle } = mount({ onEditRequested });
    act(() => handle().gotoLine(42));
    const edit = screen.getByRole("button", { name: "Edit" });
    expect(edit).toHaveStyle({ minHeight: "44px", minWidth: "44px" });
    fireEvent.click(edit);
    expect(onEditRequested).toHaveBeenCalledTimes(1);
    expect(onEditRequested).toHaveBeenCalledWith(42);
  });

  it("accessibleView_should_RenderEveryPageLineAsTextWithNumber_FocusTarget_And_PassAxe", () => {
    // axe-core is not a declared web-app dependency (see SearchPanel.test.tsx); the real axe run is the
    // Playwright case in files-a11y.spec.ts. These are the structural rules that run guards.
    mount({ dirty: true, onEditRequested: jest.fn() });
    const rows = screen.getAllByTestId("accessible-view-line");
    expect(rows).toHaveLength(500);
    rows.forEach((r, i) => expect(r.textContent).toBe(`${i + 1} row ${i + 1}`));

    const region = screen.getByRole("region");
    expect(region).toHaveAccessibleName();
    expect(screen.getByRole("navigation", { name: "Pager" })).toBeInTheDocument();
    for (const control of screen.getAllByRole("button")) expect(control).toHaveAccessibleName();
    expect(screen.getByLabelText("Go to line")).toBeInTheDocument();
    expect(document.querySelectorAll("[id]").length).toBe(new Set([...document.querySelectorAll("[id]")].map((n) => n.id)).size);
    // no interactive element nested inside another, and the only tab stop in the text is the <pre>
    expect(screen.getByTestId("accessible-view-page").querySelectorAll("button, a, input")).toHaveLength(0);
    expect(rows.every((r) => r.getAttribute("tabindex") === "-1")).toBe(true);
    expect(screen.getByTestId("accessible-view-announce")).toHaveAttribute("aria-live", "polite");
    act(() => (document.querySelector('[data-line="3"]') as HTMLElement).focus());
  });

  it("accessibleView_should_ExposeLastFocusedLineAndText_ThroughTheReadViewHandle", () => {
    const { handle } = mount({}, 50);
    expect(handle().getText()).toBe(doc(50));
    expect(handle().getCursor()).toEqual({ line: 1, col: 0 });
    act(() => (document.querySelector('[data-line="12"]') as HTMLElement).focus());
    expect(handle().getCursor().line).toBe(12);
    act(() => handle().highlightRange({ line: 20, from: 0, to: 3 }));
    expect(document.querySelector('[data-line="20"]')).toHaveAttribute("data-hit", "true");
    act(() => handle().highlightRange(null));
    expect(document.querySelector('[data-hit="true"]')).toBeNull();
  });

  it("accessibleView_should_PutOnlyOnePageInTheDom_And_ChangePageQuickly_When_1MbFile", () => {
    const big = Array.from({ length: 20_000 }, (_, i) => `line ${i} ${"x".repeat(40)}`).join("\n");
    expect(big.length).toBeGreaterThan(900_000);
    render(<AccessibleView path="big.log" source={textPageSource(big)} />);
    expect(screen.getAllByTestId("accessible-view-line")).toHaveLength(500);
    const t0 = performance.now();
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(performance.now() - t0).toBeLessThan(500);
    expect(screen.getAllByTestId("accessible-view-line")).toHaveLength(500);
    expect(screen.getByTestId("accessible-view-label").textContent).toBe("Lines 501-1000 of 20000");
  });
});

describe("paginateLines and textPageSource", () => {
  it("paginateLines_should_ClampPagesAndStripCarriageReturns", () => {
    const src = textPageSource("a\r\nb\r\nc");
    expect(src.lineCount).toBe(3);
    expect(paginateLines(src, 0, 2).lines).toEqual(["a", "b"]);
    const last = paginateLines(src, 9, 2);
    expect(last).toMatchObject({ first: 3, last: 3, pages: 2, label: "Lines 3-3 of 3" });
    expect(paginateLines(textPageSource(""), 0)).toMatchObject({ total: 1, label: "Lines 1-1 of 1", lines: [""] });
  });
});
