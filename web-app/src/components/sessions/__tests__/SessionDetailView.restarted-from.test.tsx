/**
 * context-compression — "Restarted from:" lineage row on SessionDetailView's Info tab.
 *
 * Covers UX acceptance criterion #10 (project_plans/context-compression/design/ux.md,
 * "Lineage is inspectable or gracefully absent, never a broken link"): a session created
 * via "Restart with summary" renders a `Restarted from:` row only when
 * `session.restartedFromSessionId` is set — a clickable same-tab link to the source
 * session's title when it still resolves in the live session list, or plain
 * non-clickable text with "(no longer available)" when it doesn't.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionDetailView } from "../SessionDetailView";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

jest.mock("next/dynamic", () => () => {
  return function DynamicStub() {
    return <div data-testid="terminal-output" />;
  };
});

jest.mock("../DiffViewer", () => ({ DiffViewer: () => <div data-testid="diff-viewer" /> }));
jest.mock("../VcsPanel", () => ({ VcsPanel: () => <div data-testid="vcs-panel" /> }));
jest.mock("../SessionLogsTab", () => ({ SessionLogsTab: () => <div data-testid="logs-tab" /> }));
jest.mock("../FilesTab", () => ({ FilesTab: () => <div data-testid="files-tab" /> }));
jest.mock("../ArtifactsTab", () => ({ ArtifactsTab: () => <div data-testid="artifacts-tab" /> }));
jest.mock("../WorkspaceSwitchModal", () => ({ WorkspaceSwitchModal: () => null }));
jest.mock("../TagEditor", () => ({ TagEditor: () => null }));
jest.mock("../ResumeSessionModal", () => ({ ResumeSessionModal: () => null }));
jest.mock("../BrowserTab", () => ({ BrowserTab: () => <div data-testid="browser-tab-stub" /> }));
jest.mock("../SessionSummaryPanel", () => ({ SessionSummaryPanel: () => null }));
jest.mock("@/components/ui/ActionBar", () => ({
  ActionBar: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
}));
jest.mock("@/components/ui/Modal", () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost:8543" }));
jest.mock("@/lib/constants/programs", () => ({
  getProgramDisplay: (p: string) => p,
  isKnownProgram: () => true,
  PROGRAMS: [],
}));
jest.mock("@/lib/store", () => ({ useAppSelector: jest.fn(() => []) }));
jest.mock("@/lib/store/sessionsSlice", () => ({ selectAllSessions: jest.fn() }));
jest.mock("@/lib/hooks/useShells", () => ({
  useShells: () => ({
    shells: [], isLoading: false, spawnShell: jest.fn(), stopShell: jest.fn(),
    restartShell: jest.fn(), deleteShell: jest.fn(), updateShellStatus: jest.fn(), refetch: jest.fn(),
  }),
}));
jest.mock("@/lib/hooks/useWorkflows", () => ({
  useWorkflows: () => ({
    workflows: [],
    loading: false,
    error: null,
    createWorkflow: jest.fn(),
    updateWorkflow: jest.fn(),
    deleteWorkflow: jest.fn(),
    archiveWorkflowSessions: jest.fn(),
    deleteWorkflowFailedSessions: jest.fn(),
    refresh: jest.fn(),
  }),
}));

const makeSession = (overrides: Partial<Session>): Session =>
  ({
    id: "sess-new",
    title: "New Session",
    status: SessionStatus.RUNNING,
    instanceType: InstanceType.MANAGED,
    sessionType: SessionType.DIRECTORY,
    path: "/tmp/test",
    branch: "main",
    program: "claude",
    workingDir: "",
    category: "",
    tags: [],
    externalMetadata: undefined,
    workflowId: "",
    restartedFromSessionId: "",
    ...overrides,
  }) as unknown as Session;

function renderView(session: Session, allSessions: Session[] = []) {
  const actions = {} as ReturnType<typeof useSessionActions>;
  return render(
    <SessionDetailView
      session={session}
      allSessions={allSessions}
      actions={actions}
      onClose={jest.fn()}
      initialTab="info"
    />
  );
}

beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

describe("SessionDetailView — restart lineage row (context-compression UX AC#10)", () => {
  it("does not render the row when the session was not created via restart", () => {
    renderView(makeSession({ restartedFromSessionId: "" }));
    expect(screen.queryByTestId("restarted-from-row")).not.toBeInTheDocument();
  });

  it("renders a clickable same-tab link to the source session's title when it still resolves", () => {
    const source = makeSession({ id: "sess-source", title: "Fix flaky auth test" });
    const restarted = makeSession({ id: "sess-new", restartedFromSessionId: "sess-source" });
    renderView(restarted, [source]);

    const row = screen.getByTestId("restarted-from-row");
    expect(row).toHaveTextContent("Restarted from:");

    const link = screen.getByTestId("restarted-from-link");
    expect(link).toHaveTextContent("Fix flaky auth test");
    expect(link.tagName).toBe("A");
    expect(link).toHaveAttribute("href", "/?session=sess-source");
    expect(link).not.toHaveAttribute("target");

    expect(screen.queryByTestId("restarted-from-unavailable")).not.toBeInTheDocument();
  });

  it("renders plain, non-clickable text when the source session can no longer be resolved", () => {
    const restarted = makeSession({ id: "sess-new", restartedFromSessionId: "sess-gone" });
    renderView(restarted, []);

    const row = screen.getByTestId("restarted-from-row");
    expect(row).toHaveTextContent("Restarted from:");

    const unavailable = screen.getByTestId("restarted-from-unavailable");
    expect(unavailable).toHaveTextContent("sess-gone (no longer available)");
    expect(unavailable.tagName).not.toBe("A");

    expect(screen.queryByTestId("restarted-from-link")).not.toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
