import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { DiffPane } from "../DiffPane";
import { registry } from "@/lib/shortcuts/shortcutRegistry";

jest.mock("@/components/sessions/files/engine/LezerHighlighter", () => ({
  LezerHighlighter: class {
    tokenize() {
      return Promise.resolve([]);
    }
  },
}));

const DIFF = [
  "diff --git a/a.ts b/a.ts", "--- a/a.ts", "+++ b/a.ts", "@@ -1,1 +1,1 @@", "-a", "+b", "@@ -9,1 +9,1 @@", "-c", "+d",
  "diff --git a/b.ts b/b.ts", "--- a/b.ts", "+++ b/b.ts", "@@ -1,1 +1,1 @@", "-e", "+f", "",
].join("\n");

const press = (key: string) => fireEvent.keyDown(document.activeElement ?? document.body, { key });
const activeIds = () =>
  Array.from(document.querySelectorAll('[data-active="true"]')).map((e) => e.id);

describe("DiffPane", () => {
  it("DiffPane_should_stepAcrossFilesAndWrap_When_NAndPPressed", () => {
    render(<DiffPane content={DIFF} added={3} removed={3} />);
    expect(screen.getByTestId("diff-pane")).toHaveFocus();
    expect(activeIds()).toEqual([]);

    press("n");
    expect(activeIds()).toEqual(["diff-hunk-0-0"]);
    press("n");
    press("n");
    expect(activeIds()).toEqual(["diff-hunk-1-0"]); // crossed into file 2
    press("n");
    expect(activeIds()).toEqual(["diff-hunk-0-0"]); // wrapped
    press("p");
    expect(activeIds()).toEqual(["diff-hunk-1-0"]); // wrapped backwards
  });

  it("DiffPane_should_stepViaButtons", () => {
    render(<DiffPane content={DIFF} added={3} removed={3} />);
    fireEvent.click(screen.getByTestId("diff-next-hunk"));
    fireEvent.click(screen.getByTestId("diff-next-hunk"));
    expect(activeIds()).toEqual(["diff-hunk-0-1"]);
    fireEvent.click(screen.getByTestId("diff-prev-hunk"));
    expect(activeIds()).toEqual(["diff-hunk-0-0"]);
  });

  it("DiffPane_should_toggleSplit_When_SPressed", () => {
    render(<DiffPane content={DIFF} added={3} removed={3} />);
    press("s");
    expect(screen.getAllByTestId("diff-split-grid").length).toBeGreaterThan(0);
    press("s");
    expect(screen.queryAllByTestId("diff-split-grid")).toHaveLength(0);
  });

  it("DiffPane_should_ignoreKeys_When_FocusIsOutsideThePane", () => {
    render(
      <>
        <button data-testid="outside">x</button>
        <DiffPane content={DIFF} added={3} removed={3} />
      </>,
    );
    act(() => screen.getByTestId("outside").focus());
    press("n");
    expect(activeIds()).toEqual([]);
  });

  it("DiffPane_should_registerShortcutsUnderDiffContextForTheHelpOverlay", () => {
    render(<DiffPane content={DIFF} added={3} removed={3} />);
    const labels = registry.getAll().diff.map((s) => s.label);
    expect(labels).toEqual(expect.arrayContaining(["Next change", "Previous change", "Toggle split / unified view"]));
  });

  it("DiffPane_should_notThrowOrNavigate_When_NoHunks", () => {
    render(<DiffPane content="" added={0} removed={0} />);
    press("n");
    expect(activeIds()).toEqual([]);
  });
});
