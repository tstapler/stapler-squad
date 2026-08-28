/**
 * Regression test for the 2026-07-21 finding: item.gateVerdict/gateCriteria are
 * derived from the most recent review session regardless of the item's current
 * status, but GateVerdictBox only ever rendered while status === "review" — so
 * the verdict that caused a rework round (e.g. a FAIL that triggered
 * auto-reopen) silently disappeared the moment the item bounced back to
 * in_progress. Covers:
 *  1. status "in_progress" + a resolved gateVerdict → shows "Last Review
 *     Result" with the verdict, in read-only mode (no action buttons)
 *  2. status "in_progress" + no gateVerdict (nothing reviewed yet) → no
 *     "Last Review Result" section at all
 */

import React from "react";
import { render, screen, act } from "@testing-library/react";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

jest.mock("./SessionMonitor", () => require("./backlogItemDetailTestFixtures").sessionMonitorMock());
jest.mock("./TriageReviewPanel", () => require("./backlogItemDetailTestFixtures").triageReviewPanelMock());
jest.mock("./TriageLoadingIndicator", () => require("./backlogItemDetailTestFixtures").triageLoadingIndicatorMock());
jest.mock("./ReviewChangesModal", () => ({ ReviewChangesModal: () => null }));
jest.mock("./BacklogFileBrowserModal", () => ({ BacklogFileBrowserModal: () => null }));

// Real GateVerdictBox behavior matters here (readOnly must actually suppress
// actions), so unlike the other BacklogItemDetail test files, this one does
// NOT mock it away.

const useVcsStatusMock = jest.fn();
jest.mock("@/lib/hooks/useVcsStatus", () => ({
  useVcsStatus: (...args: unknown[]) => useVcsStatusMock(...args),
}));

const useBacklogItemShipStatusMock = jest.fn();
jest.mock("@/lib/hooks/useBacklogItemShipStatus", () => ({
  useBacklogItemShipStatus: (...args: unknown[]) => useBacklogItemShipStatusMock(...args),
}));

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
jest.mock("@connectrpc/connect", () => require("./backlogItemDetailTestFixtures").connectMock());
jest.mock("@connectrpc/connect-web", () => require("./backlogItemDetailTestFixtures").connectWebMock());

const getBacklogItem = jest.fn();
const listPipelineModes = jest.fn().mockResolvedValue([]);

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
    triggerShipPR: jest.fn(),
    submitManualReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

// Same benign vanilla-extract jest-mock className warning as the sibling
// BacklogItemDetail test files — see BacklogItemDetail.test.tsx's comment.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
  useBacklogItemShipStatusMock.mockReturnValue({ data: null, loading: false, refetch: jest.fn() });
});

function makeWorkSession(): LinkedSession {
  return {
    entityId: "work-session-entity-1",
    sessionId: "work-session-1",
    role: "work",
    estimatedCostUsd: 0,
    pipelineModeSnapshot: "",
    pipelineModeSnapshotHash: "",
  };
}

function makeItem(overrides: Partial<BacklogItem>): BacklogItem {
  return {
    id: "item-1",
    title: "Fix the thing",
    description: "desc",
    status: "in_progress",
    priority: 3,
    repoPath: "/tmp/repo",
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [makeWorkSession()],
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

async function renderItem(item: BacklogItem) {
  getBacklogItem.mockReset().mockResolvedValue(item);
  render(<BacklogItemDetail itemId={item.id} />);
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("BacklogItemDetail — Last Review Result (in_progress)", () => {
  it("shows the most recent review's verdict, read-only, once the item is back in_progress", async () => {
    await renderItem(
      makeItem({
        status: "in_progress",
        gateVerdict: "FAIL",
        gateVerdictSummary: "2 criteria failed",
        gateCriteria: [{ label: "Unit tests pass", passed: false }],
      }),
    );

    expect(screen.getByText("Last Review Result")).toBeInTheDocument();
    expect(screen.getByText("FAILED")).toBeInTheDocument();
    expect(screen.getByText("2 criteria failed")).toBeInTheDocument();

    // readOnly: the interactive review-flow actions must not appear.
    expect(screen.queryByRole("button", { name: /Reopen for Revision/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/Override: Mark done anyway/i)).not.toBeInTheDocument();
  });

  it("renders nothing when the item has never been reviewed yet", async () => {
    await renderItem(makeItem({ status: "in_progress", gateVerdict: undefined }));

    expect(screen.queryByText("Last Review Result")).not.toBeInTheDocument();
  });
});
