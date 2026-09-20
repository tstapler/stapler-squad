import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { InsightsDashboard } from "./InsightsDashboard";
import {
  GetInsightsSummaryResponseSchema,
  SessionTokenSummarySchema,
  RoleCostBreakdownSchema,
} from "@/gen/session/v1/insights_pb";

// InsightsDashboard pulls in a lot of hooks unrelated to this integration
// test (Task 5.2.2d: clicking a StageCostChart bar cross-filters
// SessionsTable) — mock every data-fetching hook so this test exercises only
// the roleFilter wiring between StageCostChart and SessionsTable.
jest.mock("@/lib/hooks/useInsightsService", () => ({
  useInsightsSummary: jest.fn(),
  useDismissFinding: () => ({ dismissFinding: jest.fn() }),
}));
jest.mock("@/lib/hooks/useBudgetThreshold", () => ({
  useBudgetThreshold: () => ({ threshold: null, setThreshold: jest.fn(), isHydrated: true }),
}));
jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogSessionIndex: () => ({ index: new Map(), loading: false }),
}));
// FindingsPanel/SessionDetailDrawer pull in several more data-fetching hooks
// (dismissed findings, per-session turn timelines) unrelated to this test —
// stub both components entirely, matching this test's narrow scope.
jest.mock("./FindingsPanel", () => ({ FindingsPanel: () => null }));
jest.mock("./SessionDetailDrawer", () => ({ SessionDetailDrawer: () => null }));

import { useInsightsSummary } from "@/lib/hooks/useInsightsService";

function makeSession(sessionId: string, sessionRole: string) {
  return create(SessionTokenSummarySchema, {
    sessionId,
    conversationId: sessionId,
    sessionRole,
    projectPath: `/repo/${sessionId}`,
  });
}

function makeSummary() {
  return create(GetInsightsSummaryResponseSchema, {
    sessions: [
      makeSession("sess-work-1", "work"),
      makeSession("sess-triage-1", "triage"),
      makeSession("sess-review-1", "review"),
    ],
    roleBreakdown: [
      create(RoleCostBreakdownSchema, { sessionRole: "work", estimatedCostUsd: 14.3, sessionCount: 1 }),
      create(RoleCostBreakdownSchema, { sessionRole: "triage", estimatedCostUsd: 0.5, sessionCount: 1 }),
      create(RoleCostBreakdownSchema, { sessionRole: "review", estimatedCostUsd: 1.2, sessionCount: 1 }),
    ],
  });
}

describe("InsightsDashboard bar-click cross-filter (Story 5.2.2)", () => {
  beforeEach(() => {
    (useInsightsSummary as jest.Mock).mockReturnValue({
      summary: makeSummary(),
      loading: false,
      isLiveUpdating: false,
      error: null,
      refetch: jest.fn(),
    });
  });

  it("InsightsDashboard_should_FilterSessionsTableToWorkRole_When_WorkBarClicked", async () => {
    render(<InsightsDashboard />);

    const workLegendEntry = await screen.findByTestId("stage-cost-legend-work");
    fireEvent.click(workLegendEntry);

    await waitFor(() => {
      expect(screen.getByTestId("role-filter-chip")).toHaveTextContent("Filtered to: work");
    });

    expect(screen.getByText("sess-work-1")).toBeInTheDocument();
    expect(screen.queryByText("sess-triage-1")).not.toBeInTheDocument();
    expect(screen.queryByText("sess-review-1")).not.toBeInTheDocument();
  });

  it("InsightsDashboard_should_ClearRoleFilter_When_ActiveBarClickedAgain", async () => {
    render(<InsightsDashboard />);

    const workLegendEntry = await screen.findByTestId("stage-cost-legend-work");
    fireEvent.click(workLegendEntry);
    await waitFor(() => screen.getByTestId("role-filter-chip"));

    fireEvent.click(screen.getByTestId("stage-cost-legend-work"));

    await waitFor(() => {
      expect(screen.queryByTestId("role-filter-chip")).not.toBeInTheDocument();
    });
    expect(screen.getByText("sess-triage-1")).toBeInTheDocument();
  });
});
