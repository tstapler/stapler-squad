import React from "react";
import { render, screen } from "@testing-library/react";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { ActivityCostBreakdownSchema, ActivityType } from "@/gen/session/v1/insights_pb";
import { ActivityBreakdownTable } from "./ActivityBreakdownTable";

function makeRow(overrides: MessageInitShape<typeof ActivityCostBreakdownSchema> = {}) {
  return create(ActivityCostBreakdownSchema, {
    activityType: ActivityType.FEATURE_DEV,
    estimatedCostUsd: 1,
    sessionCount: 1,
    ...overrides,
  });
}

describe("ActivityBreakdownTable", () => {
  it("ActivityBreakdownTable_should_renderNoDataMessage_when_rowsEmpty", () => {
    render(<ActivityBreakdownTable rows={[]} />);
    expect(screen.getByText(/no data/i)).toBeInTheDocument();
  });

  it("ActivityBreakdownTable_should_preserveGivenOrder_when_rowsRendered", () => {
    // ActivityBreakdownTable does no sorting of its own (see ActivityBreakdownTable.tsx's
    // plain rows.map) — this asserts render order matches input order, not any cost-based
    // sort. Rows are deliberately given in a non-cost-sorted order to prove that.
    const rows = [
      makeRow({ activityType: ActivityType.DEBUGGING, estimatedCostUsd: 2, sessionCount: 1 }),
      makeRow({ activityType: ActivityType.FEATURE_DEV, estimatedCostUsd: 7, sessionCount: 2 }),
    ];
    render(<ActivityBreakdownTable rows={rows} />);

    const table = screen.getByTestId("activity-breakdown-table");
    const rowLabels = Array.from(table.querySelectorAll("tbody tr")).map((tr) => tr.textContent);

    expect(rowLabels[0]).toMatch(/Debugging/);
    expect(rowLabels[1]).toMatch(/Feature Dev/);
  });

  it("ActivityBreakdownTable_should_showHumanLabel_when_activityTypeIsFeatureDev", () => {
    render(<ActivityBreakdownTable rows={[makeRow({ activityType: ActivityType.FEATURE_DEV })]} />);
    expect(screen.getByText("Feature Dev")).toBeInTheDocument();
    expect(screen.queryByText(/ACTIVITY_TYPE/)).not.toBeInTheDocument();
  });

  it("ActivityBreakdownTable_should_showEstimatedMarker_when_rowRendered", () => {
    render(<ActivityBreakdownTable rows={[makeRow({ estimatedCostUsd: 5 })]} />);
    expect(screen.getByText(/~\$5\.00/)).toBeInTheDocument();
  });
});
