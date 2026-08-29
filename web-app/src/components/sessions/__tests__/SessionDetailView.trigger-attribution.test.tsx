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

let mockWorkflows: Partial<WorkflowProto>[] = [];
jest.mock("@/lib/hooks/useWorkflows", () =>
  require("./sessionDetailViewTestFixtures").makeUseWorkflowsFromGetter(() => mockWorkflows)
);

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

installConsoleErrorSilencer();
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
