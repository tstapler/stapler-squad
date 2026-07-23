/**
 * Tests for BacklogItemBadge — the non-board `/backlog` list row.
 *
 * Story 5.1.2 decision: DEFERRED. `BacklogItemBadge`'s container caps at
 * 260px, single-line (`whiteSpace: "nowrap"`), and already renders 3 inline
 * elements (status chip, AC count, truncated title) — no width budget for a
 * 4th compact BlockerChip without truncating the title further. See the code
 * comment above the status chip in BacklogItemBadge.tsx for the full
 * reasoning. This test is the regression guard proving that decision didn't
 * silently turn into scope creep: the badge renders exactly as it did before
 * this project, with no BlockerChip anywhere in its output.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { BacklogItemBadge } from "./BacklogItemBadge";

describe("BacklogItemBadge — Story 5.1.2 list-view decision", () => {
  it("BacklogItemBadge_should_RemainVisuallyUnchanged_When_ImplementationDecisionIsDefer", () => {
    render(
      <BacklogItemBadge itemTitle="Fix flaky WIP-cap test" status="review" acTotal={5} acDone={3} />
    );

    // Existing 3-element layout is untouched: status chip, AC count, title.
    expect(screen.getByLabelText("Status: Review")).toHaveTextContent("Review");
    expect(screen.getByLabelText("3 of 5 criteria done")).toHaveTextContent("3/5 ✓");
    expect(screen.getByText("Fix flaky WIP-cap test")).toBeInTheDocument();

    // No BlockerChip anywhere — the deferred decision means the badge stays
    // exactly as it was, no 4th inline element added.
    expect(screen.queryByTestId("blocker-chip")).not.toBeInTheDocument();
  });

  it("renders without an AC count when acTotal is 0, matching pre-existing behavior", () => {
    render(<BacklogItemBadge itemTitle="Idea with no criteria yet" status="idea" acTotal={0} acDone={0} />);

    expect(screen.getByLabelText("Status: Idea")).toBeInTheDocument();
    expect(screen.queryByText(/criteria done/)).not.toBeInTheDocument();
  });
});
