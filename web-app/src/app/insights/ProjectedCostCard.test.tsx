import React from "react";
import { render, screen } from "@testing-library/react";
import { ProjectedCostCard } from "./ProjectedCostCard";
import type { ProjectedCostResult } from "@/lib/hooks/useProjectedCost";

function makeProjection(overrides: Partial<ProjectedCostResult> = {}): ProjectedCostResult {
  return {
    projectedMonthly: 10,
    daysData: 10,
    daysInMonth: 30,
    hasUnpricedUsage: false,
    ...overrides,
  };
}

describe("ProjectedCostCard", () => {
  it("ProjectedCostCard_should_showUnpricedCaveat_When_hasUnpricedUsageTrue", () => {
    render(
      <ProjectedCostCard
        projection={makeProjection({ hasUnpricedUsage: true })}
        threshold={null}
        isHydrated={true}
        onThresholdChange={() => {}}
      />
    );

    expect(screen.getByText(/excludes unpriced usage/i)).toBeInTheDocument();
  });

  it("ProjectedCostCard_should_omitCaveat_When_hasUnpricedUsageFalse", () => {
    render(
      <ProjectedCostCard
        projection={makeProjection({ hasUnpricedUsage: false })}
        threshold={null}
        isHydrated={true}
        onThresholdChange={() => {}}
      />
    );

    expect(screen.queryByText(/excludes unpriced usage/i)).toBeNull();
  });
});
