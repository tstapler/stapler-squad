import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { DescriptionSection } from "./DescriptionSection";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Item",
    description: "Some description",
    status: "idea",
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
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

describe("DescriptionSection", () => {
  it("DescriptionSection_should_RenderCollapsed_When_defaultExpandedFalse", () => {
    render(<DescriptionSection item={makeItem()} defaultExpanded={false} />);

    const header = screen.getByTestId("collapsible-header-description");
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByTestId("backlog-description-rendered")).not.toBeInTheDocument();
  });

  it("DescriptionSection_should_RenderExpanded_When_defaultExpandedTrue", () => {
    render(<DescriptionSection item={makeItem({ description: "**bold**" })} defaultExpanded={true} />);

    const header = screen.getByTestId("collapsible-header-description");
    expect(header).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByTestId("backlog-description-rendered")).toBeInTheDocument();
  });

  it("reveals the markdown description once expanded", () => {
    render(<DescriptionSection item={makeItem({ description: "**bold**" })} defaultExpanded={false} />);

    fireEvent.click(screen.getByTestId("collapsible-header-description"));

    expect(screen.getByTestId("backlog-description-rendered")).toBeInTheDocument();
  });

  it("shows an empty-state message when there is no description", () => {
    render(<DescriptionSection item={makeItem({ description: "" })} defaultExpanded={false} />);

    fireEvent.click(screen.getByTestId("collapsible-header-description"));

    expect(screen.getByText("No description.")).toBeInTheDocument();
  });
});
