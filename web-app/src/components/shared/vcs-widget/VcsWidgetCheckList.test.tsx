import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { VcsWidgetCheckList } from "./VcsWidgetCheckList";
import type { CheckItemSummary } from "@/lib/vcs/types";

function makeCheck(overrides: Partial<CheckItemSummary> = {}): CheckItemSummary {
  return {
    name: "build",
    context: "ci/build",
    state: "completed",
    status: "completed",
    conclusion: "success",
    ...overrides,
  };
}

describe("VcsWidgetCheckList", () => {
  it("VcsWidgetCheckList_should_RenderNothing_When_ChecksEmpty", () => {
    const { container } = render(<VcsWidgetCheckList checks={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("VcsWidgetCheckList_should_RenderOneRowPerCheck_When_MultipleChecksProvided", () => {
    const checks = [
      makeCheck({ name: "build", context: "ci/build", conclusion: "success" }),
      makeCheck({ name: "lint", context: "ci/lint", conclusion: "failure" }),
      makeCheck({ name: "test", context: "ci/test", conclusion: "pending" }),
    ];
    render(<VcsWidgetCheckList checks={checks} />);

    // Header shows the count.
    expect(screen.getByTestId("collapsible-header-ci-checks")).toHaveTextContent("Checks (3)");

    // All rows render even while collapsed (content is expanded via click below,
    // but Radix Accordion keeps content in DOM as long as we assert post-expand).
    fireEvent.click(screen.getByTestId("collapsible-header-ci-checks"));

    expect(screen.getByText("build")).toBeInTheDocument();
    expect(screen.getByText("ci/build")).toBeInTheDocument();
    expect(screen.getByText("lint")).toBeInTheDocument();
    expect(screen.getByText("ci/lint")).toBeInTheDocument();
    expect(screen.getByText("test")).toBeInTheDocument();
    expect(screen.getByText("ci/test")).toBeInTheDocument();
  });

  it("VcsWidgetCheckList_should_RenderConclusionSpecificGlyph_When_ChecksHaveDifferentConclusions", () => {
    // Regression guard for checkClassName/checkIcon's switch statement — a prior
    // review found the row-content test above never inspected the glyph itself,
    // so a success/failure icon swap would have gone uncaught.
    const checks = [
      makeCheck({ name: "build", conclusion: "success" }),
      makeCheck({ name: "lint", conclusion: "failure" }),
      makeCheck({ name: "test", conclusion: "pending" }),
    ];
    const { container } = render(<VcsWidgetCheckList checks={checks} />);
    fireEvent.click(screen.getByTestId("collapsible-header-ci-checks"));

    const rows = container.querySelectorAll("li");
    expect(rows).toHaveLength(3);
    expect(rows[0].querySelector("svg")).toHaveClass("checkSuccess");
    expect(rows[1].querySelector("svg")).toHaveClass("checkFailure");
    expect(rows[2].querySelector("svg")).toHaveClass("checkPending");
    // Distinct lucide icons per conclusion (not the same glyph reused for all three).
    expect(rows[0].querySelector("svg")).toHaveClass("lucide-circle-check");
    expect(rows[1].querySelector("svg")).toHaveClass("lucide-circle-x");
    expect(rows[2].querySelector("svg")).toHaveClass("lucide-clock");
  });

  it("VcsWidgetCheckList_should_RenderTextConclusionForScreenReaders_When_IconIsAriaHidden", () => {
    // The status icon is aria-hidden (decorative); without a text alternative
    // a screen reader announces only the check name, never pass/fail/pending.
    const checks = [
      makeCheck({ name: "build", context: "ci/build", conclusion: "success" }),
      makeCheck({ name: "lint", context: "ci/lint", conclusion: "failure" }),
      makeCheck({ name: "test", context: "ci/test", conclusion: "pending" }),
    ];
    render(<VcsWidgetCheckList checks={checks} />);
    fireEvent.click(screen.getByTestId("collapsible-header-ci-checks"));

    expect(screen.getByText("Passed:")).toBeInTheDocument();
    expect(screen.getByText("Failed:")).toBeInTheDocument();
    expect(screen.getByText("Pending:")).toBeInTheDocument();
  });

  it("VcsWidgetCheckList_should_FallBackToState_When_ConclusionEmpty", () => {
    // Regression test: `conclusion` is Checks-API-only and empty for a
    // legacy Commit-Status-API item. Without falling back to `state`
    // (mirroring github.getCheckConclusion server-side), a passed/failed
    // legacy-API check rendered with a blank conclusion and the gray
    // "Pending" clock icon regardless of its real outcome.
    const checks = [
      makeCheck({ name: "coverage-bot", context: "coverage", conclusion: "", state: "success" }),
      makeCheck({ name: "legacy-ci", context: "legacy", conclusion: "", state: "failure" }),
    ];
    render(<VcsWidgetCheckList checks={checks} />);
    fireEvent.click(screen.getByTestId("collapsible-header-ci-checks"));

    const rows = screen.getAllByRole("listitem");
    expect(rows[0].querySelector("svg")).toHaveClass("lucide-circle-check");
    expect(rows[1].querySelector("svg")).toHaveClass("lucide-circle-x");
    expect(screen.getByText("Passed:")).toBeInTheDocument();
    expect(screen.getByText("Failed:")).toBeInTheDocument();
  });

  it("VcsWidgetCheckList_should_RenderSkippedGlyphAndLabel_When_ConclusionIsNeutralSkippedOrCancelled", () => {
    const checks = [
      makeCheck({ name: "a", context: "a", conclusion: "neutral" }),
      makeCheck({ name: "b", context: "b", conclusion: "skipped" }),
      makeCheck({ name: "c", context: "c", conclusion: "cancelled" }),
    ];
    render(<VcsWidgetCheckList checks={checks} />);
    fireEvent.click(screen.getByTestId("collapsible-header-ci-checks"));

    const rows = screen.getAllByRole("listitem");
    rows.forEach((row) => expect(row.querySelector("svg")).toHaveClass("lucide-circle-minus"));
    expect(screen.getAllByText("Skipped:")).toHaveLength(3);
  });

  it("VcsWidgetCheckList_should_StartCollapsed_When_Rendered", () => {
    render(<VcsWidgetCheckList checks={[makeCheck()]} />);

    expect(screen.getByTestId("collapsible-header-ci-checks")).toHaveAttribute(
      "aria-expanded",
      "false"
    );
  });

  it("VcsWidgetCheckList_should_ExpandOnClick_When_HeaderClicked", () => {
    render(<VcsWidgetCheckList checks={[makeCheck()]} />);

    const header = screen.getByTestId("collapsible-header-ci-checks");
    expect(header).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(header);

    expect(header).toHaveAttribute("aria-expanded", "true");
  });
});
