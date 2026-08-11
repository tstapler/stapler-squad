/**
 * Tests for SeverityBadge component and getRiskLevelInfo helper (plan.md Task 4.3.2).
 *
 * Covers:
 *  - Each of the 4 known levels renders correct label/icon/aria-label
 *  - "" / unrecognized renders the distinct "not recorded" state, never "Low"
 *  - compact renders the abbreviation instead of the full label
 *  - Severity is never colour-only (icon + text alongside the colour class)
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { SeverityBadge, getRiskLevelInfo } from "../SeverityBadge";

describe("SeverityBadge", () => {
  it("SeverityBadge_should_RenderCriticalBadgeWithIconAndLabel_When_RiskLevelIsCritical", () => {
    render(<SeverityBadge riskLevel="critical" />);
    const el = screen.getByRole("status");
    expect(el).toHaveAttribute("aria-label", "Critical risk");
    expect(screen.getByText("Critical")).toBeInTheDocument();
  });

  it("renders High/Medium/Low with correct label and aria-label", () => {
    const { rerender } = render(<SeverityBadge riskLevel="high" />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "High risk");
    expect(screen.getByText("High")).toBeInTheDocument();

    rerender(<SeverityBadge riskLevel="medium" />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Medium risk");
    expect(screen.getByText("Medium")).toBeInTheDocument();

    rerender(<SeverityBadge riskLevel="low" />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Low risk");
    expect(screen.getByText("Low")).toBeInTheDocument();
  });

  it("SeverityBadge_should_RenderNotRecordedState_When_RiskLevelIsEmptyString", () => {
    render(<SeverityBadge riskLevel="" />);
    expect(screen.getByText("Severity not recorded")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", "Severity not recorded");
    expect(screen.queryByText("Low")).not.toBeInTheDocument();
  });

  it("SeverityBadge_should_RenderVisuallyDistinctFromLow_When_RiskLevelIsEmptyVsLow", () => {
    // The jest CSS mock (src/__mocks__/styleMock.js) collapses recipe() calls to an
    // arg-independent string, so class names can't distinguish variants here — assert via
    // the two other observable, variant-driven outputs instead: aria-label text and icon glyph.
    const unrecorded = getRiskLevelInfo("");
    const low = getRiskLevelInfo("low");

    expect(unrecorded.variant).not.toBe(low.variant);
    expect(unrecorded.label).not.toBe(low.label);
    expect(unrecorded.icon).not.toBe(low.icon);

    const { container: unrecordedContainer } = render(<SeverityBadge riskLevel="" />);
    const { container: lowContainer } = render(<SeverityBadge riskLevel="low" />);
    expect(unrecordedContainer.querySelector('[role="status"]')!.getAttribute("aria-label"))
      .not.toBe(lowContainer.querySelector('[role="status"]')!.getAttribute("aria-label"));
  });

  it("renders the compact abbreviation instead of the full label", () => {
    render(<SeverityBadge riskLevel="critical" compact />);
    expect(screen.getByText("CRIT")).toBeInTheDocument();
    expect(screen.queryByText("Critical")).not.toBeInTheDocument();
  });

  it("SeverityBadge_should_PairIconAndTextWithColour_When_AnyOfFiveStatesRenders", () => {
    const levels = ["critical", "high", "medium", "low", ""];
    for (const level of levels) {
      const { container, unmount } = render(<SeverityBadge riskLevel={level} />);
      const icon = container.querySelector('[aria-hidden="true"]');
      expect(icon).not.toBeNull();
      expect(icon!.textContent).not.toBe("");
      unmount();
    }
  });

  it("getRiskLevelInfo maps every known level and falls back for unrecognized input", () => {
    expect(getRiskLevelInfo("critical").variant).toBe("critical");
    expect(getRiskLevelInfo("high").variant).toBe("high");
    expect(getRiskLevelInfo("medium").variant).toBe("medium");
    expect(getRiskLevelInfo("low").variant).toBe("low");
    expect(getRiskLevelInfo("garbage").variant).toBe("unknown");
    expect(getRiskLevelInfo("garbage").label).toBe("Severity not recorded");
  });
});
