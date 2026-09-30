import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { SessionDetailPageClient } from "./SessionDetailPageClient";
import { SessionTokenSummarySchema, type SessionTokenSummary } from "@/gen/session/v1/insights_pb";

const mockUseSessionDetail = jest.fn();
const mockUseSessionTurnTimeline = jest.fn();

jest.mock("@/lib/hooks/useInsightsService", () => ({
  useSessionDetail: (sessionId: string) => mockUseSessionDetail(sessionId),
  useSessionTurnTimeline: (conversationId: string | undefined) =>
    mockUseSessionTurnTimeline(conversationId),
}));

const mockUseBacklogSessionIndex = jest.fn();
jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogSessionIndex: () => mockUseBacklogSessionIndex(),
}));

jest.mock("../SessionDetailDrawer.css", () => ({
  overlay: "overlay",
  drawer: "drawer",
  drawerHeader: "drawerHeader",
  drawerTitle: "drawerTitle",
  sessionIdChip: "sessionIdChip",
  closeButton: "closeButton",
  section: "section",
  sectionTitle: "sectionTitle",
  metaGrid: "metaGrid",
  metaLabel: "metaLabel",
  metaValue: "metaValue",
  toolsTable: "toolsTable",
  toolsTh: "toolsTh",
  toolsThRight: "toolsThRight",
  toolsTd: "toolsTd",
  toolsTdRight: "toolsTdRight",
  outlierCell: "outlierCell",
  skillList: "skillList",
  skillBadge: "skillBadge",
  emptyState: "emptyState",
  srOnly: "srOnly",
  backlogLink: "backlogLink",
}));

function makeSession(
  overrides: Partial<Omit<SessionTokenSummary, "$typeName" | "$unknown">> = {}
): SessionTokenSummary {
  return create(SessionTokenSummarySchema, {
    sessionId: "session-1",
    conversationId: "conversation-1",
    projectPath: "/test/project",
    primaryModel: "claude-sonnet-4",
    estimatedCostUsd: 0.02,
    messageCount: 3,
    topTools: [],
    skillActivations: [],
    unpricedModels: [],
    ...overrides,
  });
}

describe("SessionDetailPageClient", () => {
  beforeEach(() => {
    mockUseSessionDetail.mockReset();
    mockUseSessionTurnTimeline.mockReset();
    mockUseSessionTurnTimeline.mockReturnValue({ turns: [], loading: false, error: null });
    mockUseBacklogSessionIndex.mockReset();
    mockUseBacklogSessionIndex.mockReturnValue({ index: new Map(), loading: false });
  });

  it("SessionDetailPageClient_should_fetchByBothFiltersAndRenderContent_when_mountedWithNoParentState", () => {
    const session = makeSession({ sessionId: "abc123", conversationId: "abc123" });
    mockUseSessionDetail.mockReturnValue({ summary: session, loading: false, error: null });

    render(<SessionDetailPageClient sessionId="abc123" />);

    // The hook itself owns the actual RPC filter construction (tested in
    // useInsightsService's own suite) — this asserts the page client wires
    // the route's sessionId straight through with no dependency on any
    // parent/dashboard state.
    expect(mockUseSessionDetail).toHaveBeenCalledWith("abc123");
    expect(mockUseSessionTurnTimeline).toHaveBeenCalledWith("abc123");
    expect(screen.getByText("Metadata")).toBeInTheDocument();
  });

  it("SessionDetailPageClient_should_showSessionNotFoundMessage_when_sessionIdMatchesNoSession", async () => {
    mockUseSessionDetail.mockReturnValue({ summary: null, loading: false, error: null });

    render(<SessionDetailPageClient sessionId="does-not-exist" />);

    await waitFor(() => {
      expect(screen.getByTestId("session-not-found")).toBeInTheDocument();
    });
    expect(screen.getByText(/Session not found/i)).toBeInTheDocument();
  });

  it("SessionDetailPageClient_should_showLoadingState_when_stillFetching", () => {
    mockUseSessionDetail.mockReturnValue({ summary: null, loading: true, error: null });

    render(<SessionDetailPageClient sessionId="abc123" />);

    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.queryByTestId("session-not-found")).not.toBeInTheDocument();
  });

  it("SessionDetailPageClient_should_showErrorMessage_when_fetchFails", () => {
    mockUseSessionDetail.mockReturnValue({ summary: null, loading: false, error: "boom" });

    render(<SessionDetailPageClient sessionId="abc123" />);

    expect(screen.getByRole("alert")).toHaveTextContent("boom");
    expect(screen.queryByTestId("session-not-found")).not.toBeInTheDocument();
  });

  it("SessionDetailPageClient_should_offerBackToDashboardLink_when_sessionNotFound", () => {
    mockUseSessionDetail.mockReturnValue({ summary: null, loading: false, error: null });

    render(<SessionDetailPageClient sessionId="does-not-exist" />);

    expect(screen.getByRole("link", { name: /Back to dashboard/i })).toHaveAttribute(
      "href",
      "/insights"
    );
  });

  it("SessionDetailPageClient_should_moveFocusToHeading_when_routeMounts", () => {
    const session = makeSession();
    mockUseSessionDetail.mockReturnValue({ summary: session, loading: false, error: null });

    render(<SessionDetailPageClient sessionId="abc123" />);

    expect(document.activeElement).toBe(screen.getByRole("heading", { level: 1 }));
  });
});
