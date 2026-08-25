/**
 * Shared mock-factory bodies and fixtures for BacklogItemDetail tests.
 *
 * babel-jest hoists `jest.mock(...)` calls above imports, so each test file
 * still needs its own thin
 * `jest.mock('x', () => require('./backlogItemDetailTestFixtures').y())`
 * call -- only the factory *bodies* live here. See the individual test files
 * for the inline `jest.mock` wiring.
 */

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
