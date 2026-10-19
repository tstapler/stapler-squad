/**
 * Characterises SessionDetailView's raw history.replaceState file-selection hash
 * before it moves behind the nav facade: exact `#file=` encoding, and clearing it.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SessionDetailView } from "../SessionDetailView";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import type { useSessionActions } from "@/lib/hooks/useSessionActions";
import { installConsoleErrorSilencer } from "./sessionDetailViewTestFixtures";

jest.mock("next/dynamic", () => require("./sessionDetailViewTestFixtures").mockNextDynamic());
jest.mock("../DiffViewer", () => require("./sessionDetailViewTestFixtures").mockDiffViewer());
jest.mock("../VcsPanel", () => require("./sessionDetailViewTestFixtures").mockVcsPanel());
jest.mock("../SessionLogsTab", () => require("./sessionDetailViewTestFixtures").mockSessionLogsTab());
jest.mock("../ArtifactsTab", () => require("./sessionDetailViewTestFixtures").mockArtifactsTab());
jest.mock("../WorkspaceSwitchModal", () => require("./sessionDetailViewTestFixtures").mockWorkspaceSwitchModal());
jest.mock("../TagEditor", () => require("./sessionDetailViewTestFixtures").mockTagEditor());
jest.mock("../ResumeSessionModal", () => require("./sessionDetailViewTestFixtures").mockResumeSessionModal());
jest.mock("../BrowserTab", () => require("./sessionDetailViewTestFixtures").mockBrowserTabWithSessionId());
jest.mock("../HandoffSummarySection", () => require("./sessionDetailViewTestFixtures").mockHandoffSummarySection());
jest.mock("@/components/ui/ActionBar", () => require("./sessionDetailViewTestFixtures").mockActionBar());
jest.mock("@/components/ui/Modal", () => require("./sessionDetailViewTestFixtures").mockModal());
jest.mock("@/lib/config", () => require("./sessionDetailViewTestFixtures").mockLibConfig());
jest.mock("@/lib/constants/programs", () => require("./sessionDetailViewTestFixtures").mockConstantsPrograms());
jest.mock("@/lib/store", () => require("./sessionDetailViewTestFixtures").mockStore());
jest.mock("@/lib/store/sessionsSlice", () => require("./sessionDetailViewTestFixtures").mockSessionsSlice());
jest.mock("@/lib/hooks/useShells", () => require("./sessionDetailViewTestFixtures").mockUseShells());

// Exposes the selection callbacks so the test can drive the file selection.
jest.mock("../FilesTab", () => ({
  FilesTab: (props: { onSelectedPathChange: (p: string | null) => void }) => (
    <div>
      <button onClick={() => props.onSelectedPathChange("a/b.go")}>select-file</button>
      <button onClick={() => props.onSelectedPathChange(null)}>clear-file</button>
    </div>
  ),
}));

const session = {
  id: "sess-hash-1",
  title: "Hash Session",
  status: SessionStatus.ACTIVE,
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

installConsoleErrorSilencer();

describe("SessionDetailView file-selection hash", () => {
  it("writes #file=<encoded path> via replaceState and clears it again", () => {
    const replaceState = jest.spyOn(window.history, "replaceState");
    render(
      <SessionDetailView
        session={session}
        allSessions={[]}
        actions={{} as ReturnType<typeof useSessionActions>}
        onClose={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByText("select-file"));
    expect(window.location.hash).toBe("#file=a%2Fb.go");
    expect(replaceState).toHaveBeenLastCalledWith(null, "", "#file=a%2Fb.go");

    fireEvent.click(screen.getByText("clear-file"));
    expect(window.location.hash).toBe("");
    replaceState.mockRestore();
  });
});
