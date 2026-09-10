import { render, screen } from "@testing-library/react";
import { SessionsSection } from "./SessionsSection";
import type { SessionsSectionProps } from "./SessionsSection";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

const sessionMonitorSpy = jest.fn((_props: unknown) => null);
jest.mock("../SessionMonitor", () => ({ SessionMonitor: (props: unknown) => sessionMonitorSpy(props) }));

beforeEach(() => {
  localStorage.clear();
  sessionMonitorSpy.mockClear();
});

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: `entity-${Math.random()}`,
    sessionId: `jules-sessions/${Math.random()}`,
    role: "jules_work",
    estimatedCostUsd: 0,
    ...overrides,
  };
}

function makeItem(linkedSessions: LinkedSession[], overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "itm_jules_row",
    title: "Jules-dispatched item",
    status: "in_progress",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions,
    notes: "",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

function renderSection(linkedSessions: LinkedSession[], overrides: Partial<SessionsSectionProps> = {}) {
  const props: SessionsSectionProps = {
    item: makeItem(linkedSessions),
    pipelineModes: [],
    latestWorkSession: undefined,
    deletingSessionId: null,
    defaultExpanded: true,
    onDeleteSession: jest.fn(),
    onSteerSession: jest.fn(),
    steeringSessionId: null,
    ...overrides,
  };
  return render(<SessionsSection {...props} />);
}

describe("SessionsSection jules_work rows (Story 3.3.2)", () => {
  it("renders JulesStatusBadge with no branch badge and no SessionMonitor for a jules_work row", () => {
    const session = makeSession({
      sessionId: "jules-sessions/abc123",
      worktreeBranch: "backlog/some-fix",
    });
    renderSection([session]);

    expect(screen.getByRole("img", { name: "Jules: Running" })).toBeInTheDocument();
    expect(screen.queryByText("backlog/some-fix")).not.toBeInTheDocument();
    expect(sessionMonitorSpy).not.toHaveBeenCalled();
  });

  it("contains a link whose accessible name is 'View this session on jules.google.com'", () => {
    const session = makeSession({ sessionId: "jules-sessions/xyz" });
    renderSection([session]);

    const link = screen.getByRole("link", { name: "View this session on jules.google.com" });
    expect(link).toHaveAttribute("href", "https://jules.google.com/session/xyz");
  });

  it("shows the failed badge without the generic orphan-ended treatment for an ended jules_work row", () => {
    const session = makeSession({
      sessionId: "jules-sessions/failed-1",
      endedAt: new Date().toISOString(),
      endReason: "jules_failed",
    });
    renderSection([session], { item: makeItem([session], { status: "ready" }) });

    expect(screen.getByRole("img", { name: "Jules: Failed" })).toBeInTheDocument();
    expect(screen.queryByText("ended")).not.toBeInTheDocument();
  });

  it("overrides an open row's phase to reconnect-required when GetJulesConfig reports auth_reconnect_required", () => {
    const session = makeSession({ sessionId: "jules-sessions/reconnect-1" });
    renderSection([session], { authReconnectRequired: true });

    expect(screen.getByRole("img", { name: "Jules: Reconnect required" })).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "Jules: Running" })).not.toBeInTheDocument();
  });

  it("does not override a closed row's phase when auth_reconnect_required is true", () => {
    const session = makeSession({
      sessionId: "jules-sessions/done-1",
      endedAt: new Date().toISOString(),
      endReason: "jules_completed",
    });
    renderSection([session], { authReconnectRequired: true, item: makeItem([session], { status: "review" }) });

    expect(screen.getByRole("img", { name: "Jules: Done" })).toBeInTheDocument();
  });
});
