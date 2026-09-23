/**
 * Covers the SessionDetailView header's BacklogOriginBadge: a session dispatched by
 * backlog automation (work/review/triage) shows the badge; one with no backlogEntry
 * does not.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionDetailView } from "../SessionDetailView";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import type { BacklogIndexEntry } from "@/lib/hooks/useBacklogService";
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
jest.mock("@/lib/hooks/useWorkflows", () => require("./sessionDetailViewTestFixtures").makeUseWorkflowsFromGetter(() => []));

const session: Session = {
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
} as unknown as Session;

function renderView(backlogEntry?: BacklogIndexEntry) {
  const actions = {} as ReturnType<typeof useSessionActions>;
  return render(
    <SessionDetailView
      session={session}
      allSessions={[]}
      actions={actions}
      onClose={jest.fn()}
      initialTab="info"
      backlogEntry={backlogEntry}
    />
  );
}

installConsoleErrorSilencer();

describe("SessionDetailView — backlog-origin badge", () => {
  it("shows the badge when backlogEntry is set", () => {
    renderView({ itemId: "item-1", itemTitle: "Fix the thing", itemStatus: "in_progress", sessionRole: "work" });
    expect(screen.getByTestId("backlog-origin-badge")).toBeInTheDocument();
  });

  it("does not show the badge when there is no backlogEntry", () => {
    renderView(undefined);
    expect(screen.queryByTestId("backlog-origin-badge")).not.toBeInTheDocument();
  });
});
