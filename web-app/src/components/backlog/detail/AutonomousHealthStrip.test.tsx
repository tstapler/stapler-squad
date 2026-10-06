import React from "react";
import { render, screen } from "@testing-library/react";
import {
  AutonomousHealthStrip,
  reworkAttemptCount,
  type AutonomousHealthStripItem,
} from "./AutonomousHealthStrip";

function makeItem(overrides: Partial<AutonomousHealthStripItem> = {}): AutonomousHealthStripItem {
  return {
    totalEstimatedCostUsd: 0,
    linkedSessions: [],
    ...overrides,
  };
}

describe("reworkAttemptCount", () => {
  it("reworkAttemptCount_should_CountOnlyWorkRoleSessions_When_MixedRolesPresent", () => {
    const item = makeItem({
      linkedSessions: [{ role: "work" }, { role: "review" }, { role: "work" }, { role: "triage" }],
    });
    expect(reworkAttemptCount(item)).toBe(2);
  });

  it("reworkAttemptCount_should_ReturnZero_When_NoLinkedSessionsExist", () => {
    expect(reworkAttemptCount(makeItem())).toBe(0);
  });
});

describe("AutonomousHealthStrip", () => {
  it("AutonomousHealthStrip_should_ShowCostAndAttemptCountWithCap_When_CapOverrideIsSet", () => {
    const item = makeItem({
      totalEstimatedCostUsd: 1.2345,
      reworkCapOverride: 3,
      linkedSessions: [{ role: "work" }, { role: "work" }],
    });
    render(<AutonomousHealthStrip item={item} />);

    const strip = screen.getByTestId("autonomous-health-strip");
    expect(strip).toHaveTextContent("$1.2345");
    expect(strip).toHaveTextContent("2");
    expect(strip).toHaveTextContent("3");
  });

  it("AutonomousHealthStrip_should_ShowDefaultCapLabel_When_ReworkCapOverrideIsUndefined", () => {
    const item = makeItem({
      totalEstimatedCostUsd: 0.5,
      linkedSessions: [{ role: "work" }],
    });
    render(<AutonomousHealthStrip item={item} />);

    expect(screen.getByTestId("autonomous-health-strip")).toHaveTextContent("default");
  });

  it("AutonomousHealthStrip_should_ShowUnlimitedLabel_When_ReworkCapOverrideIsZero", () => {
    const item = makeItem({
      totalEstimatedCostUsd: 0.5,
      reworkCapOverride: 0,
      linkedSessions: [{ role: "work" }],
    });
    render(<AutonomousHealthStrip item={item} />);

    expect(screen.getByTestId("autonomous-health-strip")).toHaveTextContent("unlimited");
  });

  it("AutonomousHealthStrip_should_HideCost_When_TotalEstimatedCostUsdIsZero", () => {
    const item = makeItem({
      totalEstimatedCostUsd: 0,
      reworkCapOverride: 2,
      linkedSessions: [{ role: "work" }],
    });
    render(<AutonomousHealthStrip item={item} />);

    expect(screen.queryByText(/Estimated cost/)).not.toBeInTheDocument();
  });

  it("AutonomousHealthStrip_should_RenderNothing_When_NoCostNoAttemptsNoCapOverride", () => {
    const { container } = render(<AutonomousHealthStrip item={makeItem()} />);
    expect(container).toBeEmptyDOMElement();
  });
});
