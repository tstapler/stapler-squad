import React from "react";
import { render, screen, act, fireEvent, waitFor } from "@testing-library/react";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

// Heavy children pull their own hooks/timers; stub them out so this test is
// focused on BacklogItemDetail's own Actions-panel render behavior.
jest.mock("./SessionMonitor", () => require("./backlogItemDetailTestFixtures").sessionMonitorMock());
jest.mock("./GateVerdictBox", () => require("./backlogItemDetailTestFixtures").gateVerdictBoxMock());
jest.mock("./TriageReviewPanel", () => require("./backlogItemDetailTestFixtures").triageReviewPanelMock());
jest.mock("./TriageLoadingIndicator", () => require("./backlogItemDetailTestFixtures").triageLoadingIndicatorMock());

jest.mock("@/lib/hooks/useSessionRepoPaths", () => require("./backlogItemDetailTestFixtures").useSessionRepoPathsMock());
jest.mock("@/lib/hooks/usePathCompletions", () => require("./backlogItemDetailTestFixtures").usePathCompletionsMock());
jest.mock("@/lib/hooks/useSessionService", () => require("./backlogItemDetailTestFixtures").useSessionServiceMock());
jest.mock("@/lib/analytics", () => require("./backlogItemDetailTestFixtures").analyticsMock());

// Epic 5.3 (backlog-event-driven-updates): BacklogItemDetail now also
// subscribes via useWatchBacklogItems + a Redux selector, and opens its own
// lightweight raw watch stream for archive/removal terminal-state detection
// (Task 5.3.1b/5.3.1c). None of these tests exercise that live-update path,
// so everything is stubbed inert: no live item ever arrives, and the raw
// terminal stream yields no events.
jest.mock("@/lib/hooks/useWatchBacklogItems", () => require("./backlogItemDetailTestFixtures").useWatchBacklogItemsMock());
jest.mock("@/lib/store", () => require("./backlogItemDetailTestFixtures").storeMock());
jest.mock("@connectrpc/connect", () => require("./backlogItemDetailTestFixtures").connectMockWithActual());
jest.mock("@connectrpc/connect-web", () => require("./backlogItemDetailTestFixtures").connectWebMock());

// BacklogItemDetail calls useStuckBacklogItems() once and passes the
// resolved StuckBacklogItem down to LifecycleSummary as a prop — stub it so
// this suite never attempts a real ConnectRPC call.
jest.mock("@/lib/hooks/useStuckBacklogItems", () => require("./backlogItemDetailTestFixtures").useStuckBacklogItemsMock());


const getBacklogItem = jest.fn();
const listPipelineModes = jest.fn().mockResolvedValue([]);
const spawnSessionFromItem = jest.fn().mockResolvedValue(undefined);

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus: jest.fn().mockResolvedValue(true),
    triggerTriage: jest.fn(),
    cancelTriage: jest.fn(),
    spawnSessionFromItem,
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    triggerShipPR: jest.fn(),
    submitManualReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Restart confirm item",
    description: "desc",
    status: "pr_pending",
    priority: 3,
    repoPath: "/tmp/repo",
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    autoApprovePlan: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    notes: "",
    createdAt: "2026-07-12T14:02:00Z",
    updatedAt: "2026-07-12T14:02:00Z",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

function liveSession(): LinkedSession {
  return {
    entityId: "e1",
    sessionId: "session-1",
    role: "work",
    estimatedCostUsd: 0,
    pipelineModeSnapshot: "",
    pipelineModeSnapshotHash: "",
  };
}

async function renderItem(item: BacklogItem) {
  getBacklogItem.mockReset().mockResolvedValue(item);
  spawnSessionFromItem.mockClear();
  localStorage.clear();
  render(<BacklogItemDetail itemId={item.id} />);
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("BacklogItemDetail restart_session confirm", () => {
  let confirmSpy: jest.SpyInstance;
  beforeEach(() => {
    confirmSpy = jest.spyOn(window, "confirm").mockReturnValue(true);
  });
  afterEach(() => confirmSpy.mockRestore());

  it("does not spawn when the operator cancels the confirm for a pr_pending item with a live session", async () => {
    confirmSpy.mockReturnValue(false);
    await renderItem(makeItem({ linkedSessions: [liveSession()] }));

    fireEvent.click(screen.getByTestId("backlog-action-restart-session"));

    await waitFor(() => expect(confirmSpy).toHaveBeenCalledTimes(1));
    expect(spawnSessionFromItem).not.toHaveBeenCalled();
  });

  it("spawns with force:true when the operator accepts the confirm", async () => {
    await renderItem(makeItem({ linkedSessions: [liveSession()] }));

    fireEvent.click(screen.getByTestId("backlog-action-restart-session"));

    await waitFor(() => expect(spawnSessionFromItem).toHaveBeenCalledWith("item-1", { force: true }));
    expect(confirmSpy).toHaveBeenCalledTimes(1);
  });

  it("skips the confirm for a pr_pending item whose sessions have all ended", async () => {
    await renderItem(makeItem({ linkedSessions: [{ ...liveSession(), endedAt: "2026-07-12T15:00:00Z" }] }));

    fireEvent.click(screen.getByTestId("backlog-action-restart-session"));

    await waitFor(() => expect(spawnSessionFromItem).toHaveBeenCalledWith("item-1", { force: true }));
    expect(confirmSpy).not.toHaveBeenCalled();
  });

  it("skips the confirm for a non-pr_pending item with a live session", async () => {
    await renderItem(makeItem({ status: "in_progress", linkedSessions: [liveSession()] }));

    fireEvent.click(screen.getByTestId("backlog-action-restart-session"));

    await waitFor(() => expect(spawnSessionFromItem).toHaveBeenCalledWith("item-1", { force: true }));
    expect(confirmSpy).not.toHaveBeenCalled();
  });
});
