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
jest.mock("../BrowserTab", () => require("./sessionDetailViewTestFixtures").mockBrowserTabWithSessionId());
jest.mock("../SessionSummaryPanel", () => require("./sessionDetailViewTestFixtures").mockSessionSummaryPanelStub());
jest.mock("../HandoffSummarySection", () => require("./sessionDetailViewTestFixtures").mockHandoffSummarySection());
jest.mock("@/components/ui/ActionBar", () => require("./sessionDetailViewTestFixtures").mockActionBar());
jest.mock("@/components/ui/Modal", () => require("./sessionDetailViewTestFixtures").mockModal());
jest.mock("@/lib/config", () => require("./sessionDetailViewTestFixtures").mockLibConfig());
jest.mock("@/lib/constants/programs", () => require("./sessionDetailViewTestFixtures").mockConstantsPrograms());
jest.mock("@/lib/store", () => require("./sessionDetailViewTestFixtures").mockStore());
jest.mock("@/lib/store/sessionsSlice", () => require("./sessionDetailViewTestFixtures").mockSessionsSlice());
jest.mock("@/lib/hooks/useShells", () => require("./sessionDetailViewTestFixtures").mockUseShells());

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

installConsoleErrorSilencer();

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
