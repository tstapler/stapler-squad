/**
 * Epic 3.2 / Story 3.2.1 (Task 3.2.1d) — Summary tab in SessionDetailView.
 *
 * Covers:
 *  - Summary tab is `aria-disabled="true"` (and shows the "generated after the
 *    session ends" tooltip) for a non-terminal (Active) session.
 *  - Summary tab is enabled and clickable for a terminal (Stopped) session.
 *  - Clicking the tab renders `SessionSummaryPanel` with the session's id.
 *
 * Renders `SessionDetailView` directly (not the `SessionDetail` wrapper) so
 * that mocking `next/dynamic` here only affects this component's own
 * dynamic-imported `TerminalOutput`, not a second, outer dynamic-imported
 * layer — see `SessionDetail.embedded.test.tsx`, where mocking `next/dynamic`
 * at that level also stubs out the dynamically-imported `SessionDetailView`
 * itself and breaks several of that file's own assertions (pre-existing,
 * unrelated to this change).
 */
import React from "react";
import { render, screen, within, fireEvent, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SessionDetailView } from "../SessionDetailView";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import { installConsoleErrorSilencer } from "./sessionDetailViewTestFixtures";

// --- Component mocks (mirrors SessionDetail.embedded.test.tsx conventions) ---

jest.mock("next/dynamic", () => require("./sessionDetailViewTestFixtures").mockNextDynamic());

jest.mock("../DiffViewer", () => require("./sessionDetailViewTestFixtures").mockDiffViewer());
jest.mock("../VcsPanel", () => require("./sessionDetailViewTestFixtures").mockVcsPanel());
jest.mock("../SessionLogsTab", () => require("./sessionDetailViewTestFixtures").mockSessionLogsTab());
jest.mock("../FilesTab", () => require("./sessionDetailViewTestFixtures").mockFilesTab());
jest.mock("../ArtifactsTab", () => require("./sessionDetailViewTestFixtures").mockArtifactsTab());
jest.mock("../WorkspaceSwitchModal", () => require("./sessionDetailViewTestFixtures").mockWorkspaceSwitchModal());
jest.mock("../TagEditor", () => require("./sessionDetailViewTestFixtures").mockTagEditor());
jest.mock("../ResumeSessionModal", () => require("./sessionDetailViewTestFixtures").mockResumeSessionModal());
jest.mock("../BrowserTab", () => require("./sessionDetailViewTestFixtures").mockBrowserTabWithSessionId());

// Mock SessionSummaryPanel to assert exactly which sessionId it was given,
// without needing the full useSessionSummary RPC/hook chain to resolve. Kept inline
// (not in the shared fixture) because it needs to expose this spy for assertions.
const sessionSummaryPanelSpy = jest.fn();
jest.mock("../SessionSummaryPanel", () => ({
  SessionSummaryPanel: (props: { sessionId: string }) => {
    sessionSummaryPanelSpy(props);
    return <div data-testid="session-summary-panel-stub">{props.sessionId}</div>;
  },
}));
jest.mock("../HandoffSummarySection", () => require("./sessionDetailViewTestFixtures").mockHandoffSummarySection());

jest.mock("@/components/ui/ActionBar", () => require("./sessionDetailViewTestFixtures").mockActionBar());
jest.mock("@/components/ui/Modal", () => require("./sessionDetailViewTestFixtures").mockModal());
jest.mock("@/lib/config", () => require("./sessionDetailViewTestFixtures").mockLibConfig());
jest.mock("@/lib/constants/programs", () => require("./sessionDetailViewTestFixtures").mockConstantsPrograms());
jest.mock("@/lib/store", () => require("./sessionDetailViewTestFixtures").mockStore());
jest.mock("@/lib/store/sessionsSlice", () => require("./sessionDetailViewTestFixtures").mockSessionsSlice());
jest.mock("@/lib/hooks/useShells", () => require("./sessionDetailViewTestFixtures").mockUseShells());

// --- Session fixture ---

const makeSession = (status: SessionStatus): Session =>
  ({
    id: "sess-summary-1",
    title: "Test Session",
    status,
    instanceType: InstanceType.MANAGED,
    sessionType: SessionType.DIRECTORY,
    path: "/tmp/test",
    branch: "main",
    program: "claude",
    workingDir: "",
    category: "",
    tags: [],
    externalMetadata: undefined,
  }) as unknown as Session;

function renderView(status: SessionStatus, initialTab: "info" | "summary" = "info") {
  const actions = {} as ReturnType<typeof useSessionActions>;
  return render(
    <SessionDetailView
      session={makeSession(status)}
      allSessions={[]}
      actions={actions}
      onClose={jest.fn()}
      initialTab={initialTab}
    />
  );
}

installConsoleErrorSilencer();

beforeEach(() => {
  sessionSummaryPanelSpy.mockClear();
});

describe("SessionDetailView — Summary tab (Epic 3.2, Story 3.2.1)", () => {
  it("is disabled with the terminal-only tooltip for a non-terminal (Active) session", () => {
    renderView(SessionStatus.ACTIVE);

    const summaryTab = screen.getByRole("tab", { name: /summary/i });
    expect(summaryTab).toHaveAttribute("aria-disabled", "true");
    expect(summaryTab).toHaveAttribute("title", "Summary is generated after the session ends.");
  });

  it("is enabled and clickable for a terminal (Stopped) session", async () => {
    const user = userEvent.setup();
    renderView(SessionStatus.STOPPED);

    const summaryTab = screen.getByRole("tab", { name: /summary/i });
    expect(summaryTab).toHaveAttribute("aria-disabled", "false");
    expect(summaryTab).not.toHaveAttribute("title");

    await user.click(summaryTab);
    expect(summaryTab).toHaveAttribute("aria-selected", "true");
  });

  it("does not switch tabs when clicked while disabled (Active session)", async () => {
    const user = userEvent.setup();
    renderView(SessionStatus.ACTIVE);

    const summaryTab = screen.getByRole("tab", { name: /summary/i });
    await user.click(summaryTab);
    expect(summaryTab).toHaveAttribute("aria-selected", "false");
    expect(screen.queryByTestId("session-summary-panel-stub")).not.toBeInTheDocument();
  });

  it("renders SessionSummaryPanel with the session's id once the tab is active", async () => {
    const user = userEvent.setup();
    renderView(SessionStatus.STOPPED);

    await user.click(screen.getByRole("tab", { name: /summary/i }));

    const panel = screen.getByTestId("session-summary-panel-stub");
    expect(within(panel).getByText("sess-summary-1")).toBeInTheDocument();
    expect(sessionSummaryPanelSpy).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "sess-summary-1" })
    );
  });

  it("renders SessionSummaryPanel immediately when starting on the summary tab (terminal session)", () => {
    renderView(SessionStatus.STOPPED, "summary");

    expect(screen.getByTestId("session-summary-panel-stub")).toBeInTheDocument();
    expect(sessionSummaryPanelSpy).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "sess-summary-1" })
    );
  });

  // Touch devices have no `:hover`, so the `title` tooltip asserted above is
  // unreachable there — tapping a disabled tab must surface the same reason
  // via a visible, dismissible hint instead.
  it("shows a dismissible hint with the disabled reason on tap (touch fallback for title)", async () => {
    const user = userEvent.setup();
    renderView(SessionStatus.ACTIVE);

    const summaryTab = screen.getByRole("tab", { name: /summary/i });
    await user.click(summaryTab);

    const hint = screen.getByTestId("disabled-tab-hint");
    expect(hint).toHaveTextContent("Summary is generated after the session ends.");
    // Tapping a disabled tab must not activate it.
    expect(summaryTab).toHaveAttribute("aria-selected", "false");

    await user.click(hint);
    expect(screen.queryByTestId("disabled-tab-hint")).not.toBeInTheDocument();
  });

  it("auto-dismisses the disabled-tab hint after the timeout", () => {
    jest.useFakeTimers();
    renderView(SessionStatus.ACTIVE);

    fireEvent.click(screen.getByRole("tab", { name: /summary/i }));
    expect(screen.getByTestId("disabled-tab-hint")).toBeInTheDocument();

    act(() => {
      jest.advanceTimersByTime(3000);
    });
    expect(screen.queryByTestId("disabled-tab-hint")).not.toBeInTheDocument();

    jest.useRealTimers();
  });
});
