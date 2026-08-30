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

const mockUpdateSession = jest.fn();
const mockAddNotification = jest.fn();

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

// Stand-in for the real pane tree: reads the same CockpitActionsContext the
// real PaneTilingContainer's descendants (e.g. SessionActionsOverflow) do,
// and exposes a button that calls onSteerAutonomousSession exactly like a
// real steer-dialog submit would.
jest.mock("@/components/pane/PaneTilingContainer", () => ({
  PaneTilingContainer: () => {
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

describe("HomeContent handleSteerAutonomousSession", () => {
  beforeEach(() => {
    mockUpdateSession.mockReset();
    mockAddNotification.mockReset();
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
