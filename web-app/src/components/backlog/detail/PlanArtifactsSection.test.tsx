import React from "react";
import { render, screen } from "@testing-library/react";
import { PlanArtifactsSection } from "./PlanArtifactsSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Item",
    status: "ready",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    notes: "",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

describe("PlanArtifactsSection", () => {
  it("renders nothing when there is no plan artifacts path", () => {
    const { container } = render(<PlanArtifactsSection item={makeItem()} defaultExpanded={false} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("is collapsed by default and reveals the path once expanded", () => {
    render(
      <PlanArtifactsSection
        item={makeItem({ planArtifactsPath: "/tmp/plans/item-1.md" })}
        defaultExpanded={false}
      />
    );

    const header = screen.getByTestId("collapsible-header-plan-artifacts");
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("/tmp/plans/item-1.md")).not.toBeInTheDocument();
  });
});
