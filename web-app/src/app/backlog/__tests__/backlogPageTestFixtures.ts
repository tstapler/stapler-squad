/**
 * Shared mock-factory bodies and fixtures for BacklogPage tests.
 *
 * babel-jest hoists `jest.mock(...)` calls above imports, so each test file
 * still needs its own thin `jest.mock('x', () => require('./backlogPageTestFixtures').y())`
 * call -- only the factory *bodies* live here. See the individual test files
 * for the inline `jest.mock` wiring.
 */

export function itemFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: "item-1",
    title: "Fix retry loop in triage",
    status: "in_progress",
    priority: 3,
    acCriteria: [],
    liveVersion: 1,
    repoPath: "owner/repo",
    ...overrides,
  } as any;
}

export function nextNavigationMock() {
  return {
    useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
    useSearchParams: () => new URLSearchParams(),
  };
}

export function analyticsMock() {
  return { useAnalytics: () => ({ track: jest.fn() }) };
}

export function usePageViewMock() {
  return { usePageView: () => {} };
}

export function backlogItemDetailMock() {
  return { BacklogItemDetail: () => null };
}

export function backlogItemFormMock() {
  return { BacklogItemForm: () => null };
}

export function backlogEmptyStateMock() {
  return {
    BacklogEmptyState: () => null,
    FilterZeroState: () => null,
    FooterNudge: () => null,
  };
}

export function vaguenessPromptModalMock() {
  return { VaguenessPromptModal: () => null };
}

export function backlogTourModalMock() {
  return { BacklogTourModal: () => null };
}

export function gitHubIssuePickerMock() {
  return { GitHubIssuePicker: () => null };
}

export function connectMock() {
  return { createClient: () => ({ getBacklogItem: jest.fn() }) };
}

export function connectWebMock() {
  return { createConnectTransport: jest.fn().mockReturnValue({}) };
}

export function configMock() {
  return {
    getApiBaseUrl: () => "http://localhost:8543",
    createAuthInterceptor: () => jest.fn(),
  };
}

export function storeMock() {
  return { useAppDispatch: () => jest.fn() };
}

export function useBacklogServiceMockFactory() {
  const actual = jest.requireActual("@/lib/hooks/useBacklogService");
  return {
    ...actual,
    useBacklogService: () => ({
      createBacklogItem: jest.fn(),
      importGitHubIssue: jest.fn(),
      triggerTriage: jest.fn(),
    }),
  };
}

// Shared `jest.fn()` instance -- the per-file `jest.mock('@/lib/hooks/useWatchBacklogItems', ...)`
// call requires this same module, so both the mock factory and the test
// bodies (via a normal `import`) resolve to the identical mock instance.
export const mockUseWatchBacklogItems = jest.fn();

export function useWatchBacklogItemsMock() {
  return { useWatchBacklogItems: () => mockUseWatchBacklogItems() };
}
