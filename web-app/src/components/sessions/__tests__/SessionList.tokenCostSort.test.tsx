// @feature session-list
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { create } from "@bufbuild/protobuf";
import { SessionList } from "../SessionList";
import type { Session } from "@/gen/session/v1/types_pb";
import { GetInsightsSummaryResponseSchema, SessionTokenSummarySchema } from "@/gen/session/v1/insights_pb";

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

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

jest.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({ index, key: index, start: index * 50, size: 50 })),
    getTotalSize: () => count * 50,
    measureElement: () => {},
  }),
}));

jest.mock("react-virtuoso", () => ({
  GroupedVirtuoso: ({
    groupCounts,
    groupContent,
    itemContent,
  }: {
    groupCounts: number[];
    groupContent: (groupIndex: number) => React.ReactNode;
    itemContent: (index: number) => React.ReactNode;
  }) => {
    const nodes: React.ReactNode[] = [];
    let flatIndex = 0;
    groupCounts.forEach((count, groupIndex) => {
      nodes.push(<React.Fragment key={`g-${groupIndex}`}>{groupContent(groupIndex)}</React.Fragment>);
      for (let i = 0; i < count; i++) {
        nodes.push(<React.Fragment key={`i-${flatIndex}`}>{itemContent(flatIndex)}</React.Fragment>);
        flatIndex++;
      }
    });
    return <div data-testid="grouped-virtuoso">{nodes}</div>;
  },
}));

jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: () => ({ items: [] }),
}));

jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    showUndoToast: jest.fn(() => "toast-id"),
    removeNotification: jest.fn(),
    addNotification: jest.fn(),
  }),
}));

jest.mock("@/lib/store", () => ({
  useAppSelector: jest.fn(() => ({})),
}));

jest.mock("@/lib/store/sessionsSlice", () => ({
  selectDetectedStatusMap: jest.fn(),
}));

jest.mock("../SessionCard", () => ({
  SessionCard: ({ session }: { session: { title: string } }) => (
    <div data-testid="session-card">{session.title}</div>
  ),
}));

jest.mock("../SessionRow", () => ({
  SessionRow: ({ session }: { session: { title: string } }) => (
    <div data-testid="session-row">{session.title}</div>
  ),
}));

jest.mock("../BulkActions", () => ({
  BulkActions: () => null,
}));

jest.mock("../TagEditor", () => ({
  TagEditor: () => null,
}));

jest.mock("@/components/ui/ActionBar", () => ({
  ActionBar: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

jest.mock("@/components/ui/Modal", () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

jest.mock("@/components/ui/AppLink", () => ({
  AppLink: ({ href, children, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...rest}>{children}</a>
  ),
}));

jest.mock("@/lib/contexts/ApprovalsContext", () => ({
  useApprovalsContext: () => ({ clearedSessions: new Set() }),
}));

const makeSession = (id: string, title: string): Partial<Session> => ({
  id,
  title,
  status: 1 as Session["status"],
  tags: [],
  category: "Working",
  path: "/tmp/session",
  branch: "",
  program: "claude",
});

const sessions = [
  makeSession("cheap", "Cheap Session") as Session,
  makeSession("expensive", "Expensive Session") as Session,
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
