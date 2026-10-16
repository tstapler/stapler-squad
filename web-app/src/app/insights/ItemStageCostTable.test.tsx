import React from "react";
import { render, screen } from "@testing-library/react";
import { ItemStageCostTable, type ItemStageCostRow } from "./ItemStageCostTable";

function makeRow(overrides: Partial<ItemStageCostRow> = {}): ItemStageCostRow {
  return {
    role: "triage",
    costUsd: 0.02,
    sessionCount: 1,
    unpricedSessionCount: 0,
    ...overrides,
  };
}

describe("ItemStageCostTable", () => {
  it("ItemStageCostTable_should_RenderMultiRoleTable_When_MultipleRolesProvided", () => {
    render(
      <ItemStageCostTable
        rows={[
          makeRow({ role: "triage", costUsd: 0.02 }),
          makeRow({ role: "review", costUsd: 0.15 }),
        ]}
      />
    );

    expect(screen.getByText("Triage")).toBeInTheDocument();
    expect(screen.getByText("Review")).toBeInTheDocument();
    expect(screen.getByText("$0.020")).toBeInTheDocument();
    expect(screen.getByText("$0.150")).toBeInTheDocument();
    expect(screen.queryByText("Work")).not.toBeInTheDocument();
  });

  it("ItemStageCostTable_should_RenderSingleRoleTable_When_OneRoleProvided", () => {
    render(<ItemStageCostTable rows={[makeRow({ role: "work", costUsd: 14.3 })]} />);

    expect(screen.getByText("Work")).toBeInTheDocument();
    expect(screen.getByText("$14.30")).toBeInTheDocument();
  });

  it("ItemStageCostTable_should_OmitTableEntirely_When_RowsIsEmpty", () => {
    render(<ItemStageCostTable rows={[]} />);

    expect(screen.queryByTestId("item-stage-cost-table")).not.toBeInTheDocument();
  });

  it("ItemStageCostTable_should_RenderEmphasizedReworkIndicator_When_SessionCountGreaterThanOne", () => {
    render(<ItemStageCostTable rows={[makeRow({ role: "triage", sessionCount: 3 })]} />);

    const runsCell = screen.getByLabelText("3 runs, rework indicator");
    expect(runsCell).toHaveTextContent("3 runs");
  });

  it("ItemStageCostTable_should_RenderPlainRunsLabel_When_SessionCountIsOne", () => {
    render(<ItemStageCostTable rows={[makeRow({ role: "triage", sessionCount: 1 })]} />);

    expect(screen.getByText("1 run")).toBeInTheDocument();
    expect(screen.queryByLabelText(/rework indicator/)).not.toBeInTheDocument();
  });

  it("ItemStageCostTable_should_ShowUnpricedIndicator_When_UnpricedSessionCountGreaterThanZero", () => {
    render(<ItemStageCostTable rows={[makeRow({ role: "triage", unpricedSessionCount: 2 })]} />);

    expect(screen.getByLabelText("2 unpriced sessions")).toBeInTheDocument();
  });
});
