/**
 * Tests for RevivedContextBadge (session-revive-uuid-loss AC3).
 *
 * Covers:
 *  - Renders the badge with the expected aria/role shape for FRESH_LOST_HISTORY
 *  - Renders nothing for every other ReviveOutcome value
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { RevivedContextBadge } from "../RevivedContextBadge";
import { ReviveOutcome, SessionSchema } from "@/gen/session/v1/types_pb";

function renderBadge(reviveOutcome: ReviveOutcome) {
  const session = create(SessionSchema, { reviveOutcome });
  return render(<RevivedContextBadge session={session} />);
}

describe("RevivedContextBadge", () => {
  it("renders the badge for FRESH_LOST_HISTORY", () => {
    renderBadge(ReviveOutcome.FRESH_LOST_HISTORY);
    const badge = screen.getByRole("status");
    expect(badge).toHaveAttribute(
      "aria-label",
      "This session lost its previous conversation and started fresh",
    );
    expect(badge).toHaveAttribute("aria-live", "polite");
    expect(screen.getByText(/Context lost/)).toBeInTheDocument();
    expect(screen.getByText("⚠")).toHaveAttribute("aria-hidden", "true");
  });

  it.each([
    ["UNSPECIFIED", ReviveOutcome.UNSPECIFIED],
    ["RESUME_LIVE", ReviveOutcome.RESUME_LIVE],
    ["RESUME_RECOVERED", ReviveOutcome.RESUME_RECOVERED],
    ["FRESH_EXPECTED", ReviveOutcome.FRESH_EXPECTED],
  ])("renders nothing for %s", (_label, outcome) => {
    const { container } = renderBadge(outcome);
    expect(container).toBeEmptyDOMElement();
  });
});
