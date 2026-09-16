/**
 * Shared mock-factory bodies and fixtures for BacklogItemDetail tests.
 *
 * babel-jest hoists `jest.mock(...)` calls above imports, so each test file
 * still needs its own thin
 * `jest.mock('x', () => require('./backlogItemDetailTestFixtures').y())`
 * call -- only the factory *bodies* live here. See the individual test files
 * for the inline `jest.mock` wiring.
 */

import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

export function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
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

export function makeReviewItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-42",
    title: "Fix mobile layout",
    description: "desc",
    status: "review",
    priority: 3,
    repoPath: "/tmp/repo",
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    autoApprovePlan: false,
    planApproved: false,
    acCriteria: [{ index: 0, text: "AC 1", status: "done" }],
    linkedSessions: [makeSession()],
    notes: "",
    createdAt: "2026-07-12T14:02:00.000Z",
    updatedAt: "2026-07-12T14:02:00.000Z",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

/**
 * Shared body for the `jest.mock("@/lib/hooks/useBacklogService", ...)` block
 * every send-back test file needs. `getMocks` is a closure (not a plain
 * object) so it's read lazily on each `useBacklogService()` call — matching
 * the existing direct-closure pattern in this file, since the module-scope
 * `jest.fn()` consts a test file passes in aren't assigned until after
 * babel-jest's hoisted `jest.mock(...)` calls have already registered this
 * factory.
 */
export function useBacklogServiceMock(
  getMocks: () => {
    getBacklogItem: jest.Mock;
    transitionStatus: jest.Mock;
    rejectPlan: jest.Mock;
    triggerTriage: jest.Mock;
    listPipelineModes?: jest.Mock;
  }
) {
  return {
    useBacklogService: () => {
      const m = getMocks();
      return {
        getBacklogItem: m.getBacklogItem,
        transitionStatus: m.transitionStatus,
        triggerTriage: m.triggerTriage,
        rejectPlan: m.rejectPlan,
        cancelTriage: jest.fn(),
        spawnSessionFromItem: jest.fn(),
        approvePlan: jest.fn(),
        overrideVerdict: jest.fn(),
        triggerReReview: jest.fn(),
        triggerShipPR: jest.fn(),
        submitManualReview: jest.fn(),
        archiveBacklogItem: jest.fn(),
        unarchiveBacklogItem: jest.fn(),
        deleteBacklogItem: jest.fn(),
        updateBacklogItem: jest.fn().mockResolvedValue(null),
        listPipelineModes: m.listPipelineModes ?? jest.fn().mockResolvedValue([]),
        lastError: null,
      };
    },
  };
}

export function sessionMonitorMock() {
  return { SessionMonitor: () => null };
}

export function gateVerdictBoxMock() {
  return { GateVerdictBox: () => null };
}

export function triageReviewPanelMock() {
  return { TriageReviewPanel: () => null };
}

export function triageLoadingIndicatorMock() {
  return { TriageLoadingIndicator: () => null };
}

export function useSessionRepoPathsMock() {
  return { useSessionRepoPaths: () => [] };
}

export function usePathCompletionsMock() {
  return { usePathCompletions: () => ({ entries: [], isLoading: false }) };
}

export function useSessionServiceMock() {
  return { useSessionService: () => ({ deleteSession: jest.fn() }) };
}

export function analyticsMock() {
  return { useAnalytics: () => ({ track: jest.fn() }) };
}

// Epic 5.3 (backlog-event-driven-updates): BacklogItemDetail now also
// subscribes via useWatchBacklogItems + a Redux selector, and opens its own
// lightweight raw watch stream for archive/removal terminal-state detection
// (Task 5.3.1b/5.3.1c). None of these tests exercise that live-update path,
// so everything is stubbed inert: no live item ever arrives, and the raw
// terminal stream yields no events.
export function useWatchBacklogItemsMock() {
  return { useWatchBacklogItems: () => ({ items: [], connectionState: "live" }) };
}

export function storeMock() {
  return { useAppSelector: () => undefined };
}

export function connectMock() {
  return { createClient: () => ({ watchBacklogItems: () => (async function* () {})() }) };
}

// shipPR.test.tsx additionally spreads the real module's other exports
// before overriding createClient -- kept as its own variant so behavior
// stays identical to before the extraction.
export function connectMockWithActual() {
  return {
    ...jest.requireActual("@connectrpc/connect"),
    createClient: () => ({ watchBacklogItems: () => (async function* () {})() }),
  };
}

export function connectWebMock() {
  return { createConnectTransport: jest.fn().mockReturnValue({}) };
}

// BacklogItemDetail calls useStuckBacklogItems() once and passes the
// resolved StuckBacklogItem down to LifecycleSummary as a prop -- stub it so
// suites that exercise it never attempt a real ConnectRPC call.
export function useStuckBacklogItemsMock() {
  return { useStuckBacklogItems: () => ({ items: [], isLoading: false, error: null }) };
}
