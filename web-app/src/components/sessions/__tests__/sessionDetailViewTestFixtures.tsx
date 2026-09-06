import React from "react";
import type { WorkflowProto } from "@/gen/session/v1/session_pb";

// Shared mock-factory bodies and fixture helpers for SessionDetailView.*.test.tsx.
//
// jest.mock(...) factories are hoisted by babel-jest above imports, so each test file
// keeps its own thin `jest.mock("path", () => require("./sessionDetailViewTestFixtures").xyz())`
// call — only the factory *bodies* live here. Every export below is a function (not a
// pre-built object) so each test file gets its own fresh jest.fn() instances, matching
// the semantics the inline factories had before extraction.

export const mockNextDynamic = () => (_loader?: () => Promise<{ default: React.ComponentType }>) => {
  return function DynamicStub() {
    return <div data-testid="terminal-output" />;
  };
};

export const mockDiffViewer = () => ({ DiffViewer: () => <div data-testid="diff-viewer" /> });
export const mockVcsPanel = () => ({ VcsPanel: () => <div data-testid="vcs-panel" /> });
export const mockSessionLogsTab = () => ({ SessionLogsTab: () => <div data-testid="logs-tab" /> });
export const mockFilesTab = () => ({ FilesTab: () => <div data-testid="files-tab" /> });
export const mockArtifactsTab = () => ({ ArtifactsTab: () => <div data-testid="artifacts-tab" /> });
export const mockWorkspaceSwitchModal = () => ({ WorkspaceSwitchModal: () => null });
export const mockTagEditor = () => ({ TagEditor: () => null });
export const mockResumeSessionModal = () => ({ ResumeSessionModal: () => null });

// BrowserTab has two variants in use: a plain stub, and one that reflects the sessionId
// prop into the stub's data-testid (needed wherever a test distinguishes multiple
// browser-tab instances by session).
export const mockBrowserTabSimple = () => ({
  BrowserTab: () => <div data-testid="browser-tab-stub" />,
});

export const mockBrowserTabWithSessionId = () => ({
  BrowserTab: ({ sessionId }: { sessionId: string }) => (
    <div data-testid={`browser-tab-stub-${sessionId}`} />
  ),
});

// SessionSummaryPanel has two shared variants: a null stub (Summary tab's content isn't
// under test), and a stub div (Summary tab's content is rendered but not asserted on in
// detail). A third, spy-based variant lives inline in SessionDetailView.summary-tab.test.tsx
// itself, since that file asserts directly on the spy — not shareable without threading
// the spy instance back out of the mock factory.
export const mockSessionSummaryPanelNull = () => ({ SessionSummaryPanel: () => null });
export const mockSessionSummaryPanelStub = () => ({
  SessionSummaryPanel: () => <div data-testid="session-summary-panel-stub" />,
});

// HandoffSummarySection (Info tab) embeds RestartWithSummaryButton, which calls
// useSessionService -> useAnalytics -- unavailable without an AnalyticsContextProvider
// wrapper, which none of these files' render trees set up (not relevant to what any of
// them test).
export const mockHandoffSummarySection = () => ({ HandoffSummarySection: () => null });

export const mockActionBar = () => ({
  ActionBar: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
});

export const mockModal = () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
});

export const mockLibConfig = () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => jest.fn(),
});

export const mockConstantsPrograms = () => ({
  getProgramDisplay: (p: string) => p,
  isKnownProgram: () => true,
  PROGRAMS: [],
});

export const mockStore = () => ({ useAppSelector: jest.fn(() => []) });
export const mockSessionsSlice = () => ({ selectAllSessions: jest.fn() });

// useShells otherwise fires a real ConnectRPC listShells call on mount (silently
// swallowed by the hook, but its async setState lands outside a test's `act()` scope and
// produces noisy console warnings). Stub it to keep runs deterministic.
export const mockUseShells = () => ({
  useShells: () => ({
    shells: [],
    isLoading: false,
    spawnShell: jest.fn(),
    stopShell: jest.fn(),
    restartShell: jest.fn(),
    deleteShell: jest.fn(),
    updateShellStatus: jest.fn(),
    refetch: jest.fn(),
  }),
});

const workflowsHookShape = (workflows: Partial<WorkflowProto>[]) => ({
  workflows,
  loading: false,
  error: null,
  createWorkflow: jest.fn(),
  updateWorkflow: jest.fn(),
  deleteWorkflow: jest.fn(),
  archiveWorkflowSessions: jest.fn(),
  deleteWorkflowFailedSessions: jest.fn(),
  refresh: jest.fn(),
});

export const mockUseWorkflowsEmpty = () => ({
  useWorkflows: () => workflowsHookShape([]),
});

// For tests that mutate the workflow list per-test (e.g. via a `let mockWorkflows = [...]`
// module-level variable), pass a getter so the mock always reads the current value.
export const makeUseWorkflowsFromGetter = (getWorkflows: () => Partial<WorkflowProto>[]) => ({
  useWorkflows: () => workflowsHookShape(getWorkflows()),
});

// The jest styleMock for `.css.ts` files wraps every export (including plain `style()`
// string exports) in a callable proxy, which triggers a benign "Invalid value for prop
// className" React warning — see BacklogItemPanel.test.tsx and RadioGroup.test.tsx, which
// silence it the same way. Call this at the top level of a test file (not inside
// describe/it) so beforeAll/afterAll register in the right scope.
export function installConsoleErrorSilencer(): void {
  beforeAll(() => {
    jest.spyOn(console, "error").mockImplementation(() => {});
  });
  afterAll(() => {
    jest.restoreAllMocks();
  });
}
