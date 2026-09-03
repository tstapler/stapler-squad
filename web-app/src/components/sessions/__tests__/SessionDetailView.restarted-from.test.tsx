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
import { installConsoleErrorSilencer } from "./sessionDetailViewTestFixtures";

jest.mock("next/dynamic", () => require("./sessionDetailViewTestFixtures").mockNextDynamic());

jest.mock("../DiffViewer", () => require("./sessionDetailViewTestFixtures").mockDiffViewer());
jest.mock("../VcsPanel", () => require("./sessionDetailViewTestFixtures").mockVcsPanel());
jest.mock("../SessionLogsTab", () => require("./sessionDetailViewTestFixtures").mockSessionLogsTab());
jest.mock("../FilesTab", () => require("./sessionDetailViewTestFixtures").mockFilesTab());
jest.mock("../ArtifactsTab", () => require("./sessionDetailViewTestFixtures").mockArtifactsTab());
jest.mock("../WorkspaceSwitchModal", () => require("./sessionDetailViewTestFixtures").mockWorkspaceSwitchModal());
jest.mock("../TagEditor", () => require("./sessionDetailViewTestFixtures").mockTagEditor());
jest.mock("../ResumeSessionModal", () => require("./sessionDetailViewTestFixtures").mockResumeSessionModal());
jest.mock("../BrowserTab", () => require("./sessionDetailViewTestFixtures").mockBrowserTabSimple());
jest.mock("../SessionSummaryPanel", () => require("./sessionDetailViewTestFixtures").mockSessionSummaryPanelNull());
jest.mock("../HandoffSummarySection", () => require("./sessionDetailViewTestFixtures").mockHandoffSummarySection());
jest.mock("@/components/ui/ActionBar", () => require("./sessionDetailViewTestFixtures").mockActionBar());
jest.mock("@/components/ui/Modal", () => require("./sessionDetailViewTestFixtures").mockModal());
jest.mock("@/lib/config", () => require("./sessionDetailViewTestFixtures").mockLibConfig());
jest.mock("@/lib/constants/programs", () => require("./sessionDetailViewTestFixtures").mockConstantsPrograms());
jest.mock("@/lib/store", () => require("./sessionDetailViewTestFixtures").mockStore());
jest.mock("@/lib/store/sessionsSlice", () => require("./sessionDetailViewTestFixtures").mockSessionsSlice());
jest.mock("@/lib/hooks/useShells", () => require("./sessionDetailViewTestFixtures").mockUseShells());
jest.mock("@/lib/hooks/useWorkflows", () => require("./sessionDetailViewTestFixtures").mockUseWorkflowsEmpty());

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

installConsoleErrorSilencer();

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
