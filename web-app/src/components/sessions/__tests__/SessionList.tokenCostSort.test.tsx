// @feature session-list
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { create } from "@bufbuild/protobuf";
import { SessionList } from "../SessionList";
import type { Session } from "@/gen/session/v1/types_pb";
import { GetInsightsSummaryResponseSchema, SessionTokenSummarySchema } from "@/gen/session/v1/insights_pb";
import { makeSession } from "./sessionListTestFixtures";

// Heavy dependency mocks for SessionList (mirrors SessionList.collapse.test.tsx),
// with getInsightsSummary/watchInsights added for AC-2's cost join.

const mockGetInsightsSummary = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    listProjects: jest.fn(async () => ({ projects: [] })),
    getInsightsSummary: (...args: unknown[]) => mockGetInsightsSummary(...args),
    watchInsights: async function* () {
      // Immediately-completing stream — no live updates needed for this test.
    },
  })),
}));

jest.mock("@connectrpc/connect-web", () => require("./sessionListTestFixtures").mockConnectWeb());

jest.mock("@tanstack/react-virtual", () => require("./sessionListTestFixtures").mockReactVirtual());

jest.mock("react-virtuoso", () => require("./sessionListTestFixtures").mockReactVirtuoso());

jest.mock("@/lib/contexts/ReviewQueueContext", () => require("./sessionListTestFixtures").mockReviewQueueContext());

jest.mock("@/lib/contexts/NotificationContext", () => require("./sessionListTestFixtures").mockNotificationContext());

jest.mock("@/lib/store", () => require("./sessionListTestFixtures").mockStore());

jest.mock("@/lib/store/sessionsSlice", () => require("./sessionListTestFixtures").mockSessionsSlice());

jest.mock("../SessionCard", () => require("./sessionListTestFixtures").mockSessionCard());

jest.mock("../SessionRow", () => require("./sessionListTestFixtures").mockSessionRow());

jest.mock("../BulkActions", () => require("./sessionListTestFixtures").mockBulkActions());

jest.mock("../TagEditor", () => require("./sessionListTestFixtures").mockTagEditor());

jest.mock("@/components/ui/ActionBar", () => require("./sessionListTestFixtures").mockActionBarPassthrough());

jest.mock("@/components/ui/Modal", () => require("./sessionListTestFixtures").mockModal());

jest.mock("@/components/ui/AppLink", () => require("./sessionListTestFixtures").mockAppLink());

jest.mock("@/lib/contexts/ApprovalsContext", () => require("./sessionListTestFixtures").mockApprovalsContext());

const sessions = [
  makeSession("cheap", "Cheap Session", { category: "Working" }) as Session,
  makeSession("expensive", "Expensive Session", { category: "Working" }) as Session,
];

describe("SessionList — token cost sort (AC-2)", () => {
  beforeEach(() => {
    window.localStorage.clear();
    mockGetInsightsSummary.mockReset();
    mockGetInsightsSummary.mockResolvedValue(
      create(GetInsightsSummaryResponseSchema, {
        sessions: [
          create(SessionTokenSummarySchema, { sessionId: "cheap", estimatedCostUsd: 0.1 }),
          create(SessionTokenSummarySchema, { sessionId: "expensive", estimatedCostUsd: 5.0 }),
        ],
      })
    );
  });

  it("SessionList_should_populateCostByIdAndSortByCost_When_insightsSummaryResolves", async () => {
    const user = userEvent.setup();
    render(<SessionList sessions={sessions} />);

    await waitFor(() => expect(mockGetInsightsSummary).toHaveBeenCalled());

    const sortSelect = screen.getByLabelText("Sort sessions by");
    await user.selectOptions(sortSelect, "tokenCost");

    await waitFor(() => {
      const rows = screen.getAllByTestId("session-row").map((el) => el.textContent);
      expect(rows[0]).toBe("Expensive Session");
      expect(rows[1]).toBe("Cheap Session");
    });
  });

  it("SessionList_should_offerSortByCostOption_When_dropdownRendered", () => {
    render(<SessionList sessions={sessions} />);
    expect(screen.getByRole("option", { name: "Sort: Cost" })).toBeInTheDocument();
  });
});
