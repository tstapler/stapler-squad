/**
 * Tests for the "Ship PR" self-service action on the backlog item detail page
 * (docs/tasks/backlog-feature-improvement.md, 2026-07-18 update).
 *
 * Before this, an item sitting in review with all acceptance criteria complete
 * and a PASS gate verdict had no way to ask the agent to ship a PR from the
 * item detail page itself — the only paths were a fully-automated pipeline
 * (opt-in AutoCreatePR, default off) or a manual "Create PR" click on the
 * unrelated Review Queue page. This covers the button's visibility rules and
 * that clicking it calls triggerShipPR for the current item.
 */

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
const triggerShipPR = jest.fn().mockResolvedValue({ prUrl: "https://github.com/example/repo/pull/42" });

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus: jest.fn().mockResolvedValue(true),
    triggerTriage: jest.fn(),
    cancelTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    triggerShipPR,
    submitManualReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

// Pre-existing jest/vanilla-extract mock limitation — see BacklogItemDetail.test.tsx.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: "session-entity-1",
    sessionId: "session-1",
    role: "work",
    estimatedCostUsd: 0,
    pipelineModeSnapshot: "",
    pipelineModeSnapshotHash: "",
    ...overrides,
  };
}

function makeReviewItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Dedent shortcut broken in edit mode",
    description: "desc",
    status: "review",
    priority: 3,
    repoPath: "/tmp/repo",
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [
      { index: 0, text: "AC 1", status: "done" },
      { index: 1, text: "AC 2", status: "done" },
    ],
    linkedSessions: [makeSession()],
    notes: "",
    createdAt: "2026-07-12T14:02:00Z",
    updatedAt: "2026-07-12T14:02:00Z",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    gateVerdict: "PASS",
    gateVerdictSummary: "All criteria verified",
    ...overrides,
  };
}

async function renderItem(item: BacklogItem) {
  getBacklogItem.mockReset().mockResolvedValue(item);
  triggerShipPR.mockClear();
  // Story 3.1.4's useSectionExpandState persists collapse state to
  // localStorage keyed by itemId — clear so one test's expand/collapse
  // interactions never leak into the next test reusing the same itemId.
  localStorage.clear();

  render(<BacklogItemDetail itemId={item.id} />);

  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("BacklogItemDetail — Ship PR action", () => {
  it("shows an enabled Ship PR button for a review-status item with all AC complete and no PR yet", async () => {
    await renderItem(makeReviewItem());

    const button = screen.getByTestId("backlog-action-ship-pr");
    expect(button).toBeInTheDocument();
    expect(button).toBeEnabled();
  });

  it("does not show the Ship PR button once the item already has a PR", async () => {
    await renderItem(makeReviewItem({ prUrl: "https://github.com/example/repo/pull/1", prNumber: 1 }));

    expect(screen.queryByTestId("backlog-action-ship-pr")).not.toBeInTheDocument();
  });

  it("does not show the Ship PR button outside review status", async () => {
    await renderItem(makeReviewItem({ status: "in_progress" }));

    expect(screen.queryByTestId("backlog-action-ship-pr")).not.toBeInTheDocument();
  });

  it("disables Ship PR (with an explanatory title) when acceptance criteria are incomplete", async () => {
    await renderItem(
      makeReviewItem({
        acCriteria: [
          { index: 0, text: "AC 1", status: "done" },
          { index: 1, text: "AC 2", status: "pending" },
        ],
      })
    );

    const button = screen.getByTestId("backlog-action-ship-pr");
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("title", expect.stringContaining("acceptance criteria"));
  });

  it("calls triggerShipPR with the item id when clicked", async () => {
    await renderItem(makeReviewItem());

    fireEvent.click(screen.getByTestId("backlog-action-ship-pr"));

    await waitFor(() => expect(triggerShipPR).toHaveBeenCalledWith("item-1"));
  });
});
