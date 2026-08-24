/**
 * Regression test for a bug found by sdd:6-verify's React idiom review pass on the
 * session-notes feature: `useSessionService.updateSession` never rejects on RPC
 * failure — it catches the error internally, dispatches a global `setError`, and
 * resolves to `null`. NotePanel's save-error UI (aria-live assertive message,
 * preserved textarea) only fires on a *rejected* promise, so the original
 * `onSave={async (v) => { await actions.update({ note: v }); }}` wiring in
 * SessionDetailView could never surface a real save failure to the user — the
 * panel would silently exit edit mode as if the save succeeded. Fixed by checking
 * the resolved value and throwing when it's null/falsy.
 *
 * Mirrors SessionDetailView.summary-tab.test.tsx's mock harness (same component
 * dependencies), scoped to only what's needed to reach the Info tab's NotePanel.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SessionDetailView } from "../SessionDetailView";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

jest.mock("next/dynamic", () => (loader: () => Promise<{ default: React.ComponentType }>) => {
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
jest.mock("../BrowserTab", () => ({
  BrowserTab: ({ sessionId }: { sessionId: string }) => (
    <div data-testid={`browser-tab-stub-${sessionId}`} />
  ),
}));
jest.mock("../SessionSummaryPanel", () => ({
  SessionSummaryPanel: () => <div data-testid="session-summary-panel-stub" />,
}));
// HandoffSummarySection (Info tab) embeds RestartWithSummaryButton, which
// calls useSessionService -> useAnalytics -- unavailable without an
// AnalyticsContextProvider wrapper, which this file's render tree doesn't
// set up (it isn't relevant to note save-error wiring, this file's own
// concern).
jest.mock("../HandoffSummarySection", () => ({ HandoffSummarySection: () => null }));
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
    shells: [],
    isLoading: false,
    spawnShell: jest.fn(),
    stopShell: jest.fn(),
    restartShell: jest.fn(),
    deleteShell: jest.fn(),
    updateShellStatus: jest.fn(),
    refetch: jest.fn(),
  }),
}));

const makeSession = (note: string): Session =>
  ({
    id: "sess-note-1",
    title: "Test Session",
    status: SessionStatus.ACTIVE,
    instanceType: InstanceType.MANAGED,
    sessionType: SessionType.DIRECTORY,
    path: "/tmp/test",
    branch: "main",
    program: "claude",
    workingDir: "",
    category: "",
    tags: [],
    note,
    externalMetadata: undefined,
  }) as unknown as Session;

function renderView(updateMock: jest.Mock) {
  const actions = { update: updateMock } as unknown as ReturnType<typeof useSessionActions>;
  return render(
    <SessionDetailView
      session={makeSession("")}
      allSessions={[]}
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

describe("SessionDetailView — NotePanel save-error wiring", () => {
  it("surfaces an assertive error and preserves the draft when actions.update resolves null (RPC failure that never rejects)", async () => {
    const user = userEvent.setup();
    const updateMock = jest.fn().mockResolvedValue(null);
    renderView(updateMock);

    await user.click(screen.getByRole("button", { name: /add note/i }));
    await user.type(screen.getByTestId("session-note-textarea"), "left this waiting on CI");
    await user.click(screen.getByTestId("session-note-save-button"));

    expect(updateMock).toHaveBeenCalledWith({ note: "left this waiting on CI" });
    expect(await screen.findByRole("alert")).toHaveTextContent(/failed to save note/i);
    expect(screen.getByTestId("session-note-textarea")).toHaveValue("left this waiting on CI");
  });

  it("exits edit mode with no error when actions.update resolves a session (success)", async () => {
    const user = userEvent.setup();
    const updateMock = jest.fn().mockResolvedValue({ id: "sess-note-1" } as unknown as Session);
    renderView(updateMock);

    await user.click(screen.getByRole("button", { name: /add note/i }));
    await user.type(screen.getByTestId("session-note-textarea"), "left this waiting on CI");
    await user.click(screen.getByTestId("session-note-save-button"));

    expect(updateMock).toHaveBeenCalledWith({ note: "left this waiting on CI" });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
