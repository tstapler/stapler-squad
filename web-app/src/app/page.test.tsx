/**
 * PR #645 Gate 2 review finding #3: handleSteerAutonomousSession's
 * result === null failure branch (page.tsx) had zero direct test —
 * SessionActionsOverflow.test.tsx only mocks the handler, never exercises
 * its real body. This exercises the real handler via useCockpitActions(),
 * the way PaneTilingContainer's descendants actually invoke it.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import Home from "./page";
import { useCockpitActions } from "@/lib/contexts/CockpitActionsContext";
import { initialPaneState } from "@/lib/pane/paneReducer";
import { useWindowManager } from "@/lib/window/useWindowManager";
import { useWindowUrlSync } from "@/lib/window/useWindowUrlSync";

const mockUpdateSession = jest.fn();
const mockAddNotification = jest.fn();
const mockPaneTilingContainer = jest.fn();
const mockDispatchPane = jest.fn();
const mockSwitchToWindow = jest.fn();

jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: () => ({
    sessions: [],
    loading: false,
    error: null,
    deleteSession: jest.fn(),
    pauseSession: jest.fn(),
    resumeSession: jest.fn(),
    renameSession: jest.fn(),
    restartSession: jest.fn(),
    clearConversationState: jest.fn(),
    createCheckpoint: jest.fn(),
    listCheckpoints: jest.fn(),
    forkSession: jest.fn(),
    listSessions: jest.fn(),
    updateSession: mockUpdateSession,
    getSession: jest.fn(),
  }),
}));

jest.mock("@/lib/hooks/useKeyboard", () => ({ useKeyboard: jest.fn() }));
jest.mock("@/lib/hooks/useFocusTrap", () => ({ useFocusTrap: jest.fn() }));
jest.mock("@/lib/analytics/usePageView", () => ({ usePageView: jest.fn() }));
jest.mock("@/lib/contexts/AnalyticsContext", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ addNotification: mockAddNotification }),
}));
jest.mock("@/lib/contexts/OmnibarContext", () => ({
  useOmnibar: () => ({ openInCreationMode: jest.fn(), openOmnibar: jest.fn() }),
}));
jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
}));
jest.mock("@/components/sessions/ResumeSessionModal", () => ({
  ResumeSessionModal: () => null,
}));

jest.mock("@/lib/window/useWindowManager", () => ({
  useWindowManager: jest.fn(),
}));
jest.mock("@/lib/window/useWindowUrlSync", () => ({
  useWindowUrlSync: jest.fn(),
}));

// Stand-in for the real pane tree: reads the same CockpitActionsContext the
// real PaneTilingContainer's descendants (e.g. SessionActionsOverflow) do,
// and exposes a button that calls onSteerAutonomousSession exactly like a
// real steer-dialog submit would. Also records every render's props so tests
// can assert what paneState/dispatch HomeContent actually wired in.
jest.mock("@/components/pane/PaneTilingContainer", () => ({
  PaneTilingContainer: (props: { paneState: unknown; dispatch: (action: unknown) => void }) => {
    mockPaneTilingContainer(props);
    const { onSteerAutonomousSession } = useCockpitActions();
    const [result, setResult] = React.useState<string>("pending");
    return (
      <button
        onClick={async () => {
          const ok = await onSteerAutonomousSession("session-1", "do the thing");
          setResult(String(ok));
        }}
      >
        steer:{result}
      </button>
    );
  },
}));

// Default single-window fixture so every test that doesn't care about
// multi-window wiring still gets a valid currentWindow (page.tsx crashes
// on `currentWindow.paneState` if `windows` is empty or the id doesn't match).
function defaultWindowManagerMock() {
  (useWindowManager as jest.Mock).mockReturnValue({
    windows: [{ id: "win-default", name: "Window 1", paneState: initialPaneState() }],
    dispatchPane: mockDispatchPane,
    createWindow: jest.fn(),
    closeWindow: jest.fn(),
    renameWindow: jest.fn(),
  });
}
function defaultWindowUrlSyncMock() {
  (useWindowUrlSync as jest.Mock).mockReturnValue({
    currentWindowId: "win-default",
    switchToWindow: mockSwitchToWindow,
  });
}

describe("HomeContent handleSteerAutonomousSession", () => {
  beforeEach(() => {
    mockUpdateSession.mockReset();
    mockAddNotification.mockReset();
    mockPaneTilingContainer.mockClear();
    mockDispatchPane.mockClear();
    mockSwitchToWindow.mockClear();
    defaultWindowManagerMock();
    defaultWindowUrlSyncMock();
  });

  it("notifies and resolves false when updateSession resolves null (steer failed)", async () => {
    mockUpdateSession.mockResolvedValue(null);
    render(<Home />);

    fireEvent.click(screen.getByRole("button"));

    await waitFor(() => expect(screen.getByRole("button")).toHaveTextContent("steer:false"));
    expect(mockAddNotification).toHaveBeenCalledWith(
      expect.objectContaining({
        notificationType: "error",
        sessionId: "session-1",
        message: expect.stringMatching(/failed to send steering message/i),
      })
    );
  });

  it("resolves true and does not notify when updateSession succeeds", async () => {
    mockUpdateSession.mockResolvedValue({ id: "session-1" });
    render(<Home />);

    fireEvent.click(screen.getByRole("button"));

    await waitFor(() => expect(screen.getByRole("button")).toHaveTextContent("steer:true"));
    expect(mockAddNotification).not.toHaveBeenCalled();
  });
});

function mockWindowManagerReturning(windows: { id: string; name: string; paneState: unknown }[]) {
  (useWindowManager as jest.Mock).mockReturnValue({
    windows,
    dispatchPane: mockDispatchPane,
    createWindow: jest.fn(),
    closeWindow: jest.fn(),
    renameWindow: jest.fn(),
  });
}

function mockUrlSyncResolvingTo(currentWindowId: string, switchToWindow = mockSwitchToWindow) {
  (useWindowUrlSync as jest.Mock).mockReturnValue({ currentWindowId, switchToWindow });
}

function lastPaneTilingContainerProps() {
  const calls = mockPaneTilingContainer.mock.calls;
  return calls[calls.length - 1][0];
}

describe("HomeContent multi-window wiring", () => {
  beforeEach(() => {
    mockPaneTilingContainer.mockClear();
    mockDispatchPane.mockClear();
    mockSwitchToWindow.mockClear();
  });

  it("HomeContent_should_renderPaneTilingContainerWithActiveWindowPaneState_When_currentWindowIdChanges", () => {
    const paneStateA = initialPaneState();
    const paneStateB = initialPaneState();
    const windows = [
      { id: "win-1", name: "Window 1", paneState: paneStateA },
      { id: "win-2", name: "Window 2", paneState: paneStateB },
    ];
    mockWindowManagerReturning(windows);
    mockUrlSyncResolvingTo("win-2");

    const { rerender } = render(<Home />);

    // PaneTilingContainer receives windows[currentWindowId]'s paneState, not windows[0]'s,
    // and its dispatch prop routes through dispatchPane(currentWindowId, action).
    const someAction = { type: "ZOOM_PANE", paneId: "pane-1" };
    expect(lastPaneTilingContainerProps().paneState).toBe(paneStateB);
    lastPaneTilingContainerProps().dispatch(someAction);
    expect(mockDispatchPane).toHaveBeenCalledWith("win-2", someAction);
    expect(mockDispatchPane).not.toHaveBeenCalledWith("win-1", expect.anything());

    // Simulate currentWindowId changing (e.g. the URL's ?window= param changed).
    mockUrlSyncResolvingTo("win-1");
    rerender(<Home />);

    expect(lastPaneTilingContainerProps().paneState).toBe(paneStateA);
    mockDispatchPane.mockClear();
    lastPaneTilingContainerProps().dispatch(someAction);
    expect(mockDispatchPane).toHaveBeenCalledWith("win-1", someAction);
  });

  it("HomeContent_should_keepEachTabIndependentlyOnItsOwnWindow_When_twoInstancesResolveDifferentUrlParams", () => {
    const paneStateA = initialPaneState();
    const paneStateB = initialPaneState();
    const windows = [
      { id: "win-A", name: "Window A", paneState: paneStateA },
      { id: "win-B", name: "Window B", paneState: paneStateB },
    ];
    mockWindowManagerReturning(windows);

    // "Tab" 1 resolves ?window=win-A via its own useWindowUrlSync instance.
    mockUrlSyncResolvingTo("win-A");
    render(<Home />);
    const tab1Props = lastPaneTilingContainerProps();
    expect(tab1Props.paneState).toBe(paneStateA);

    // "Tab" 2 is a fully independent mount (its own useWindowUrlSync instance) that
    // resolves a different ?window= param against the same shared windows list.
    const mockSwitchToWindowTab2 = jest.fn();
    mockUrlSyncResolvingTo("win-B", mockSwitchToWindowTab2);
    render(<Home />);
    const tab2Props = lastPaneTilingContainerProps();
    expect(tab2Props.paneState).toBe(paneStateB);

    // Tab 1's already-rendered props are untouched by tab 2 mounting/resolving a
    // different window — no shared `activeWindowId` leaked between instances (ADR-001).
    expect(tab1Props.paneState).toBe(paneStateA);

    // Switching windows via tab 2's own switchToWindow never touches tab 1's.
    tab2Props.dispatch({ type: "ZOOM_PANE", paneId: "pane-1" });
    expect(mockDispatchPane).toHaveBeenCalledWith("win-B", { type: "ZOOM_PANE", paneId: "pane-1" });
    expect(mockSwitchToWindow).not.toHaveBeenCalled();
    expect(mockSwitchToWindowTab2).not.toHaveBeenCalled();
  });
});
