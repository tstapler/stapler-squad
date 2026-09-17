import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { StageCostChart } from "./StageCostChart";
import { RoleCostBreakdownSchema, type RoleCostBreakdown } from "@/gen/session/v1/insights_pb";

function makeRole(fields: {
  sessionRole: string;
  estimatedCostUsd?: number;
  sessionCount?: number;
  unpricedSessionCount?: number;
}): RoleCostBreakdown {
  return create(RoleCostBreakdownSchema, {
    sessionRole: fields.sessionRole,
    estimatedCostUsd: fields.estimatedCostUsd ?? 0,
    sessionCount: fields.sessionCount ?? 1,
    unpricedSessionCount: fields.unpricedSessionCount ?? 0,
  });
}

describe("StageCostChart", () => {
  it("StageCostChart_should_RenderBarsSortedDescendingByCost_When_ThreeRoleBreakdownEntriesProvided", () => {
    render(
      <StageCostChart
        roles={[
          makeRole({ sessionRole: "triage", estimatedCostUsd: 0.5 }),
          makeRole({ sessionRole: "review", estimatedCostUsd: 1.2 }),
          makeRole({ sessionRole: "work", estimatedCostUsd: 14.3 }),
        ]}
      />
    );

    const legendEntries = screen.getAllByTestId(/^stage-cost-legend-/);
    expect(legendEntries.map((el) => el.getAttribute("data-testid"))).toEqual([
      "stage-cost-legend-work",
      "stage-cost-legend-review",
      "stage-cost-legend-triage",
    ]);
  });

  it("StageCostChart_should_OmitZeroSessionRole_When_RoleHasNoEntryInBreakdown", () => {
    render(
      <StageCostChart
        roles={[
          makeRole({ sessionRole: "triage", estimatedCostUsd: 0.5 }),
          makeRole({ sessionRole: "work", estimatedCostUsd: 14.3 }),
        ]}
      />
    );

    expect(screen.queryByTestId("stage-cost-legend-review")).not.toBeInTheDocument();
  });

  it("StageCostChart_should_ShowNoDataCard_When_RoleBreakdownIsEmpty", () => {
    render(<StageCostChart roles={[]} />);

    expect(screen.getByText("No data")).toBeInTheDocument();
    expect(screen.queryByTestId(/^stage-cost-legend-/)).not.toBeInTheDocument();
  });

  it("StageCostChart_should_ShowUnpricedIndicator_When_UnpricedSessionCountGreaterThanZero", () => {
    render(
      <StageCostChart
        roles={[makeRole({ sessionRole: "triage", estimatedCostUsd: 0.5, unpricedSessionCount: 2 })]}
      />
    );

    expect(screen.getByText(/2 unpriced/)).toBeInTheDocument();
    expect(screen.getByTestId("stage-cost-legend-triage")).toHaveAccessibleName(
      "triage, 2 unpriced sessions"
    );
  });

  it("StageCostChart_should_IncludeUnpricedCountInChartAriaLabel_When_RoleHasUnpricedSessions", () => {
    render(
      <StageCostChart
        roles={[
          makeRole({ sessionRole: "review", estimatedCostUsd: 1.2, unpricedSessionCount: 1 }),
          makeRole({ sessionRole: "triage", estimatedCostUsd: 0.5 }),
        ]}
      />
    );

    expect(screen.getByRole("img")).toHaveAccessibleName(
      "Cost by stage: review $1.20 (1 unpriced), triage $0.500"
    );
  });

  it("StageCostChart_should_CallOnRoleClick_When_LegendEntryClicked", () => {
    const onRoleClick = jest.fn();
    render(
      <StageCostChart
        roles={[makeRole({ sessionRole: "work", estimatedCostUsd: 2 })]}
        onRoleClick={onRoleClick}
      />
    );

    fireEvent.click(screen.getByTestId("stage-cost-legend-work"));
    expect(onRoleClick).toHaveBeenCalledWith("work");
  });

  it("StageCostChart_should_MarkLegendEntryPressed_When_ActiveRoleMatches", () => {
    render(
      <StageCostChart
        roles={[makeRole({ sessionRole: "work", estimatedCostUsd: 2 })]}
        activeRole="work"
        onRoleClick={jest.fn()}
      />
    );

    expect(screen.getByTestId("stage-cost-legend-work")).toHaveAttribute("aria-pressed", "true");
  });
});
