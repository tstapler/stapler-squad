/**
 * Task 2.1.0b pin: the window keydown moved into useReviewNavKeys with no behavior change.
 * Task 4.1.3b inverted the unfocused-pane case: keysEnabled = windowActive && isFocused.
 */
import React from "react";
import { render, fireEvent } from "@testing-library/react";
import { SessionDetailView } from "../SessionDetailView";
import { useReviewNavKeys } from "../useReviewNavKeys";
import type { useSessionActions } from "@/lib/hooks/useSessionActions";
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

installConsoleErrorSilencer();

const session = {
  id: "sess-keys-1",
  title: "Keys",
  status: SessionStatus.ACTIVE,
  instanceType: InstanceType.MANAGED,
  sessionType: SessionType.DIRECTORY,
  path: "/tmp/test",
  branch: "main",
  program: "claude",
  workingDir: "",
  category: "",
  tags: [],
} as unknown as Session;

function Probe(props: { enabled: boolean; onNext: () => void }) {
  useReviewNavKeys(props.enabled, {
    isFullscreen: false,
    setIsFullscreen: () => {},
    showNavigation: true,
    onNext: props.onNext,
  });
  return null;
}

describe("useReviewNavKeys / SessionDetailView keydown", () => {
  it("reviewNavKeys_should_CallOnNextOnce_When_ShiftArrowRightWithKeysEnabled", () => {
    const onNext = jest.fn();
    render(<Probe enabled onNext={onNext} />);
    fireEvent.keyDown(window, { key: "ArrowRight", shiftKey: true });
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  it("reviewNavKeys_should_NotCallOnNext_When_KeysDisabled", () => {
    const onNext = jest.fn();
    render(<Probe enabled={false} onNext={onNext} />);
    fireEvent.keyDown(window, { key: "ArrowRight", shiftKey: true });
    expect(onNext).not.toHaveBeenCalled();
  });

  const renderViews = (views: Array<{ onNext: () => void; isActive: boolean; windowActive?: boolean }>) => {
    const actions = {} as ReturnType<typeof useSessionActions>;
    render(
      <>
        {views.map((v, i) => (
          <SessionDetailView
            key={i}
            session={session}
            allSessions={[]}
            actions={actions}
            onClose={jest.fn()}
            onNext={v.onNext}
            showNavigation
            isActive={v.isActive}
            windowActive={v.windowActive}
          />
        ))}
      </>,
    );
    fireEvent.keyDown(window, { key: "ArrowRight", shiftKey: true });
  };

  it("reviewNavKeys_should_NotFire_When_UnfocusedPaneOrHiddenWindowAfterTask4_1_3b", () => {
    const focusedNext = jest.fn();
    const unfocusedNext = jest.fn();
    const hiddenWindowNext = jest.fn();
    renderViews([
      { onNext: focusedNext, isActive: true },
      { onNext: unfocusedNext, isActive: false },
      { onNext: hiddenWindowNext, isActive: true, windowActive: false },
    ]);
    expect(focusedNext).toHaveBeenCalledTimes(1);
    expect(unfocusedNext).not.toHaveBeenCalled();
    expect(hiddenWindowNext).not.toHaveBeenCalled();
  });

  it("keypress_should_BeHandledOnce_When_TwoWindowsMounted", () => {
    const activeWindowNext = jest.fn();
    const hiddenWindowNext = jest.fn();
    renderViews([
      { onNext: hiddenWindowNext, isActive: true, windowActive: false },
      { onNext: activeWindowNext, isActive: true, windowActive: true },
    ]);
    expect(activeWindowNext).toHaveBeenCalledTimes(1);
    expect(hiddenWindowNext).not.toHaveBeenCalled();
  });
});
