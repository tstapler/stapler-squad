/**
 * Focus-restoration regression test for the resume-session modal reached
 * from SessionDetailView's action sheet (WCAG 2.4.3).
 *
 * Deliberately does NOT mock ResumeSessionModal or useFocusTrap (unlike
 * SessionDetailView.trigger-attribution.test.tsx) so the real trap-and-restore
 * behavior runs end to end. The action sheet's own "Resume" button unmounts
 * when the sheet closes, so handlePauseResume falls back to the persistent
 * "More actions" button (moreActionsButtonRef) as the resume trigger — this
 * verifies that fallback actually restores focus there rather than to
 * document.body.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SessionDetailView } from "../SessionDetailView";
import type { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionStatus, InstanceType } from "@/gen/session/v1/types_pb";

jest.mock("../SessionDetailView.css", () =>
  new Proxy({}, { get: (_target, key) => (typeof key === "string" ? key : "") })
);

jest.mock("next/dynamic", () => () => {
  const DynamicComponent = () => null;
  DynamicComponent.displayName = "LoadableComponent";
  return DynamicComponent;
});

jest.mock("../DiffViewer", () => ({ DiffViewer: () => null }));
jest.mock("../VcsPanel", () => ({ VcsPanel: () => null }));
jest.mock("../SessionLogsTab", () => ({ SessionLogsTab: () => null }));
jest.mock("../FilesTab", () => ({ FilesTab: () => null }));
jest.mock("../ArtifactsTab", () => ({ ArtifactsTab: () => null }));
jest.mock("../BrowserTab", () => ({ BrowserTab: () => null }));
jest.mock("../SessionSummaryPanel", () => ({ SessionSummaryPanel: () => null }));
// HandoffSummarySection (Info tab) embeds RestartWithSummaryButton, which
// calls useSessionService -> useAnalytics -- unavailable without an
// AnalyticsContextProvider wrapper, which this file's render tree doesn't
// set up (it isn't relevant to focus restoration, this file's own concern).
jest.mock("../HandoffSummarySection", () => ({ HandoffSummarySection: () => null }));
jest.mock("@/components/ui/ActionBar", () => ({
  ActionBar: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
jest.mock("../WorkspaceSwitchModal", () => ({ WorkspaceSwitchModal: () => null }));
jest.mock("../TagEditor", () => ({ TagEditor: () => null }));

jest.mock("@/components/ui/Modal", () => ({
  Modal: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <>{children}</> : null,
  ModalContent: ({ children, ...rest }: { children: React.ReactNode }) => (
    <div {...rest}>{children}</div>
  ),
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
jest.mock("@/lib/hooks/useShells", () => ({ useShells: () => ({ shells: [] }) }));
jest.mock("@/lib/hooks/useWorkflows", () => ({ useWorkflows: () => ({ workflows: [] }) }));

const mockResume = jest.fn().mockResolvedValue(undefined);
const mockPause = jest.fn().mockResolvedValue(undefined);

function makeSession() {
  return {
    id: "s1",
    title: "Session One",
    status: SessionStatus.PAUSED,
    instanceType: InstanceType.MANAGED,
    path: "/workspace/s1",
    workingDir: "/workspace/s1",
    branch: "main",
    program: "claude",
    tags: [],
  } as unknown as import("@/gen/session/v1/types_pb").Session;
}

describe("SessionDetailView resume-modal focus restoration", () => {
  beforeEach(() => {
    mockResume.mockClear();
    mockPause.mockClear();
  });

  it("SessionDetailView_should_restoreFocusToMoreActionsButton_When_resumeModalClosedAfterOpeningFromActionSheet", async () => {
    const session = makeSession();
    const actions = {
      resume: mockResume,
      pause: mockPause,
    } as unknown as ReturnType<typeof useSessionActions>;
    render(
      <SessionDetailView
        session={session}
        allSessions={[session]}
        actions={actions}
        onClose={() => {}}
        initialTab="info"
      />
    );

    const moreActions = screen.getByTestId("more-actions-button");
    fireEvent.click(moreActions);

    const resumeButton = await screen.findByTestId("action-pause");
    fireEvent.click(resumeButton);

    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(moreActions));
  });
});
