/**
 * SessionDetailView no longer writes the `#file=` hash: useFilesDeepLink (inside FilesTab) is
 * the single writer. SessionDetailView only seeds the initial selection from the hash and
 * forgets it when the session changes. The hash encoding itself is covered by the
 * useFilesDeepLink tests.
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
  FilesTab: (props: { initialSelectedPath: string | null; onSelectedPathChange: (p: string | null) => void; tabActive?: boolean }) => (
    <div data-tab-active={String(props.tabActive)}>
      <span data-testid="initial-path">{props.initialSelectedPath}</span>
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

const renderView = (props: { isActive?: boolean; initialTab?: "files" } = {}) =>
  render(
    <SessionDetailView
      session={session}
      allSessions={[]}
      actions={{} as ReturnType<typeof useSessionActions>}
      onClose={jest.fn()}
      {...props}
    />,
  );

describe("SessionDetailView file-selection hash", () => {
  afterEach(() => window.history.replaceState(null, "", "/"));

  it("seeds FilesTab from #file= and never writes the hash itself", () => {
    window.history.replaceState(null, "", "/#file=a%2Fb.go");
    const replaceState = jest.spyOn(window.history, "replaceState");
    renderView({ initialTab: "files" });
    expect(screen.getByTestId("initial-path").textContent).toBe("a/b.go");

    fireEvent.click(screen.getByText("select-file"));
    fireEvent.click(screen.getByText("clear-file"));
    expect(replaceState).not.toHaveBeenCalled();
    expect(window.location.hash).toBe("#file=a%2Fb.go");
    replaceState.mockRestore();
  });

  it("sessionDetailView_should_NotMountFilesTab_When_FilesTabNeverOpened", () => {
    renderView();
    expect(screen.queryByTestId("initial-path")).toBeNull();
  });

  it("sessionDetailView_should_MountFilesTabOnceAndRetainIt_When_FilesTabOpenedThenLeft", () => {
    window.history.replaceState(null, "", "/#file=a%2Fb.go");
    renderView();
    fireEvent.click(screen.getByRole("tab", { name: /files/i }));
    expect(screen.getByTestId("initial-path").textContent).toBe("a/b.go");
    fireEvent.click(screen.getByRole("tab", { name: /info/i }));
    expect(screen.getByTestId("initial-path")).toBeTruthy();
  });

  it("sessionDetailView_should_PassTabActiveFalse_When_FilesTabRetainedButHidden", () => {
    const { container } = renderView({ initialTab: "files" });
    expect(container.querySelector("[data-tab-active]")?.getAttribute("data-tab-active")).toBe("true");
    fireEvent.click(screen.getByRole("tab", { name: /info/i }));
    expect(container.querySelector("[data-tab-active]")?.getAttribute("data-tab-active")).toBe("false");
  });

  it("sessionDetailView_should_NotSeedFilesSelectionFromHash_When_PaneNotFocused", () => {
    window.history.replaceState(null, "", "/#file=a%2Fb.go");
    renderView({ isActive: false, initialTab: "files" });
    expect(screen.getByTestId("initial-path").textContent).toBe("");
  });
});
