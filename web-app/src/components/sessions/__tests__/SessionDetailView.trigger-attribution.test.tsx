/**
 * webhook-triggers Epic 7.4 — trigger attribution badge on SessionDetailView.
 *
 * Covers AC6: a session created by an automated trigger (cron/github_push/webhook)
 * shows a "Triggered by: {slug} ({trigger_type})" badge linking back to /triggers;
 * a session tied to a plain "manual" (@slug) Workflow does not.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionDetailView } from "../SessionDetailView";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import type { WorkflowProto } from "@/gen/session/v1/session_pb";

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

let mockWorkflows: Partial<WorkflowProto>[] = [];
jest.mock("@/lib/hooks/useWorkflows", () => ({
  useWorkflows: () => ({
    workflows: mockWorkflows,
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

const makeSession = (workflowId: string): Session =>
  ({
    id: "sess-1",
    title: "Test Session",
    status: SessionStatus.STOPPED,
    instanceType: InstanceType.MANAGED,
    sessionType: SessionType.DIRECTORY,
    path: "/tmp/test",
    branch: "main",
    program: "claude",
    workingDir: "",
    category: "",
    tags: [],
    externalMetadata: undefined,
    workflowId,
    workflowName: "Triage tickets",
  }) as unknown as Session;

function renderView(session: Session) {
  const actions = {} as ReturnType<typeof useSessionActions>;
  return render(
    <SessionDetailView session={session} allSessions={[]} actions={actions} onClose={jest.fn()} initialTab="info" />
  );
}

beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});
beforeEach(() => {
  mockWorkflows = [];
});

describe("SessionDetailView — trigger attribution badge (Epic 7.4)", () => {
  it("shows the attribution badge for a session created by an automated (webhook) trigger", () => {
    mockWorkflows = [
      { id: "wf-1", slug: "jira-ticket", triggerType: "webhook" } as WorkflowProto,
    ];
    renderView(makeSession("wf-1"));

    const badge = screen.getByTestId("trigger-attribution-badge");
    expect(badge).toHaveTextContent("Triggered by: jira-ticket (webhook)");
    expect(badge).toHaveAttribute("href", "/triggers");
  });

  it("does not show the attribution badge for a plain manual (@slug) workflow session", () => {
    mockWorkflows = [
      { id: "wf-2", slug: "my-workflow", triggerType: "manual" } as WorkflowProto,
    ];
    renderView(makeSession("wf-2"));

    expect(screen.queryByTestId("trigger-attribution-badge")).not.toBeInTheDocument();
  });

  it("does not show the attribution badge for a session with no workflowId", () => {
    renderView(makeSession(""));
    expect(screen.queryByTestId("trigger-attribution-badge")).not.toBeInTheDocument();
  });
});
