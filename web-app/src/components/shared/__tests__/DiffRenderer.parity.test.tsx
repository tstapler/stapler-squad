/**
 * DiffRenderer IntelliJ-parity features: split view, active-hunk highlight,
 * hunk nav buttons, context-expand controls, syntax highlighting.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { DiffRenderer } from "../DiffRenderer";
import type { Highlighter } from "@/components/sessions/files/engine/Highlighter";

// Hunk 1 mixes 2 deletes with 1 add (asymmetric); hunk 2 starts at line 20 so lines 9..19 are hidden.
const DIFF = [
  "diff --git a/foo.go b/foo.go",
  "--- a/foo.go",
  "+++ b/foo.go",
  "@@ -3,4 +3,3 @@",
  " ctx",
  "-old one",
  "-old two",
  "+new one",
  " ctx2",
  "@@ -20,2 +19,3 @@",
  " tail",
  "+added tail",
  " end",
  "",
].join("\n");

const base = { content: DIFF, added: 2, removed: 2 };

describe("DiffRenderer split view", () => {
  it("DiffRenderer_should_switchToSplitGrid_When_SplitClicked_And_BackToUnified", () => {
    render(<DiffRenderer {...base} />);
    expect(screen.queryAllByTestId("diff-split-grid")).toHaveLength(0);

    fireEvent.click(screen.getByTestId("diff-mode-split"));
    expect(screen.getAllByTestId("diff-split-grid")).toHaveLength(2);
    expect(screen.getByTestId("diff-mode-split")).toHaveAttribute("aria-pressed", "true");

    fireEvent.click(screen.getByTestId("diff-mode-unified"));
    expect(screen.queryAllByTestId("diff-split-grid")).toHaveLength(0);
  });

  it("DiffRenderer_should_putOldOnLeftNewOnRightWithPlaceholder_When_HunkIsAsymmetric", () => {
    render(<DiffRenderer {...base} />);
    fireEvent.click(screen.getByTestId("diff-mode-split"));

    const grid = screen.getAllByTestId("diff-split-grid")[0];
    const left = Array.from(grid.querySelectorAll('[data-side="left"]')).map((e) => e.textContent);
    const right = Array.from(grid.querySelectorAll('[data-side="right"]')).map((e) => e.textContent);
    expect(left).toEqual(["ctx", "old one", "old two", "ctx2"]);
    expect(right).toEqual(["ctx", "new one", "", "ctx2"]);
    // 2 deletes vs 1 add leaves exactly one right-side placeholder in this hunk.
    expect(grid.querySelectorAll('[data-testid="diff-split-placeholder"][data-side="right"]')).toHaveLength(1);
    expect(grid.querySelectorAll('[data-testid="diff-split-placeholder"][data-side="left"]')).toHaveLength(0);
  });

  it("DiffRenderer_should_callOnViewModeChange_When_Controlled", () => {
    const onViewModeChange = jest.fn();
    render(<DiffRenderer {...base} viewMode="unified" onViewModeChange={onViewModeChange} />);
    fireEvent.click(screen.getByTestId("diff-mode-split"));
    expect(onViewModeChange).toHaveBeenCalledWith("split");
    expect(screen.queryAllByTestId("diff-split-grid")).toHaveLength(0); // parent owns the state
  });
});

describe("DiffRenderer hunk navigation", () => {
  it("DiffRenderer_should_markActiveHunkAndShowPosition_When_ActiveHunkIdGiven", () => {
    render(<DiffRenderer {...base} activeHunkId="diff-hunk-0-1" onPrevHunk={jest.fn()} onNextHunk={jest.fn()} />);
    expect(screen.getByTestId("diff-hunk-0-1")).toHaveAttribute("data-active", "true");
    expect(screen.getByTestId("diff-hunk-0-0")).not.toHaveAttribute("data-active");
    expect(screen.getByTestId("diff-hunk-counter")).toHaveTextContent("2/2");
  });

  it("DiffRenderer_should_invokeCallbacks_When_PrevNextButtonsClicked", () => {
    const onPrev = jest.fn();
    const onNext = jest.fn();
    render(<DiffRenderer {...base} onPrevHunk={onPrev} onNextHunk={onNext} />);
    fireEvent.click(screen.getByTestId("diff-next-hunk"));
    fireEvent.click(screen.getByTestId("diff-prev-hunk"));
    expect(onNext).toHaveBeenCalledTimes(1);
    expect(onPrev).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("diff-hunk-counter")).toHaveTextContent("–/2");
  });

  it("DiffRenderer_should_omitNavButtons_When_NoCallbacks", () => {
    render(<DiffRenderer {...base} />);
    expect(screen.queryByTestId("diff-next-hunk")).not.toBeInTheDocument();
  });
});

describe("DiffRenderer context expansion", () => {
  it("DiffRenderer_should_offerHiddenLineCounts_And_RequestMoreContext_When_Clicked", () => {
    const onChange = jest.fn();
    render(<DiffRenderer {...base} onContextLinesChange={onChange} />);

    // Above hunk 1: 2 lines hidden (lines 1-2). Above hunk 2: lines 9..19 = 11 hidden, capped to 10.
    const above = screen.getAllByTestId("diff-expand-above");
    expect(above[0]).toHaveTextContent("Show 2 more lines above");
    expect(above[1]).toHaveTextContent("Show 10 more lines above");
    expect(screen.getAllByTestId("diff-expand-below")).toHaveLength(1); // last hunk of the file only

    fireEvent.click(above[1]);
    expect(onChange).toHaveBeenCalledWith(13); // default 3 + step 10
  });

  it("DiffRenderer_should_requestWholeFile_When_ExpandAllClicked_And_ResetWhenCollapsed", () => {
    const onChange = jest.fn();
    const { rerender } = render(<DiffRenderer {...base} onContextLinesChange={onChange} />);
    fireEvent.click(screen.getByTestId("diff-expand-all"));
    expect(onChange).toHaveBeenLastCalledWith(-1);

    rerender(<DiffRenderer {...base} contextLines={-1} onContextLinesChange={onChange} />);
    expect(screen.queryByTestId("diff-expand-above")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("diff-expand-all"));
    expect(onChange).toHaveBeenLastCalledWith(0);
  });

  it("DiffRenderer_should_omitExpandControls_When_NoHandler", () => {
    render(<DiffRenderer {...base} />);
    expect(screen.queryByTestId("diff-expand-all")).not.toBeInTheDocument();
    expect(screen.queryByTestId("diff-expand-above")).not.toBeInTheDocument();
  });
});

describe("DiffRenderer syntax highlighting", () => {
  const SIMPLE = ["diff --git a/a.go b/a.go", "--- a/a.go", "+++ b/a.go", "@@ -1,1 +1,2 @@", " package a", "+var x = 1", ""].join("\n");

  // Highlights the first 7 chars of the hunk text ("package").
  const highlighter: Highlighter = {
    tokenize: jest.fn().mockResolvedValue([{ from: 0, to: 7, className: "ssq-tok-keyword" }]),
  };

  it.each(["unified", "split"] as const)("DiffRenderer_should_wrapTokens_When_%sModeAndHighlighterProvided", async (mode) => {
    const { container } = render(
      <DiffRenderer content={SIMPLE} added={1} removed={0} highlighter={highlighter} languageForFile={() => "go"} />,
    );
    if (mode === "split") fireEvent.click(screen.getByTestId("diff-mode-split"));

    await waitFor(() => expect(container.querySelector(".ssq-tok-keyword")).not.toBeNull());
    expect(container.querySelector(".ssq-tok-keyword")).toHaveTextContent("package");
    expect(highlighter.tokenize).toHaveBeenCalledWith(expect.objectContaining({ text: "package a\nvar x = 1" }), "go");
  });

  it("DiffRenderer_should_renderPlainText_When_HighlighterRejects", async () => {
    const failing: Highlighter = { tokenize: jest.fn().mockRejectedValue(new Error("boom")) };
    const { container } = render(
      <DiffRenderer content={SIMPLE} added={1} removed={0} highlighter={failing} languageForFile={() => "go"} />,
    );
    await waitFor(() => expect(failing.tokenize).toHaveBeenCalled());
    expect(container.textContent).toContain("package a");
  });
});
