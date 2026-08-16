/**
 * Focus-restoration regression test for the main session-list page's delete
 * confirmation dialog (WCAG 2.4.3).
 *
 * Deliberately does NOT mock useFocusTrap so the real trap-and-restore
 * behavior runs end to end. The dialog is only reachable via the "d"
 * keyboard shortcut (page.tsx's useKeyboard handler), which requires a
 * session to already be selected — production reaches that state through
 * PaneTilingContainer's descendants calling `useCockpitActions().onSessionClick`.
 * This harness stubs PaneTilingContainer with a real button that consumes the
 * same context and calls the real onSessionClick, mirroring exactly how a
 * session row click behaves, then fires the real "d" keydown (useKeyboard is
 * NOT mocked here since it's the mechanism under test) to open the dialog.
 */

import React from "react";
import { render, fireEvent, waitFor, screen } from "@testing-library/react";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";

// jsdom does not implement performance.mark; page.tsx's handleSessionClick
// calls it unconditionally behind a `typeof performance !== "undefined"`
// guard that doesn't check for the method itself.
if (typeof performance.mark !== "function") {
  performance.mark = jest.fn();
}

const mockPush = jest.fn();
const mockReplace = jest.fn();

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: () => null }),
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
}));

jest.mock("@/lib/analytics/usePageView", () => ({ usePageView: jest.fn() }));
jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));
jest.mock("@/lib/contexts/OmnibarContext", () => ({
  useOmnibar: () => ({ openInCreationMode: jest.fn(), openOmnibar: jest.fn() }),
}));

function makeSession(id: string): Session {
  return {
    id,
    title: `Session ${id}`,
    status: SessionStatus.UNSPECIFIED,
    path: `/workspace/${id}`,
    workingDir: `/workspace/${id}`,
    branch: "main",
    program: "claude",
    tags: [],
  } as unknown as Session;
}

const sessionOne = makeSession("s1");
const sessionTwo = makeSession("s2");

// Stable reference — page.tsx effects key off `sessions` by identity.
const stableSessions: Session[] = [sessionOne, sessionTwo];
const mockDeleteSession = jest.fn().mockResolvedValue(undefined);

jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: jest.fn(() => ({
    sessions: stableSessions,
    loading: false,
    error: null,
    connectionState: "connected",
    systemMemoryPct: 0,
    listSessions: jest.fn(),
    getSession: jest.fn(),
    createSession: jest.fn(),
    updateSession: jest.fn(),
    deleteSession: mockDeleteSession,
    pauseSession: jest.fn(),
    resumeSession: jest.fn(),
    hibernateSession: jest.fn(),
    resumeHibernatedSession: jest.fn(),
    renameSession: jest.fn(),
    restartSession: jest.fn(),
    clearConversationState: jest.fn(),
    acknowledgeSession: jest.fn(),
    createCheckpoint: jest.fn(),
    listCheckpoints: jest.fn(),
    forkSession: jest.fn(),
    runOneShot: jest.fn().mockResolvedValue(null),
    listPromptHistory: jest.fn(),
    watchSessions: jest.fn(),
    stopWatching: jest.fn(),
  })),
}));

// Stub out the heavy tiling container with a real button per session that
// invokes the real onSessionClick from CockpitActionsContext — mirroring how
// a real session row click sets `selectedSession` via the shared funnel.
jest.mock("@/components/pane/PaneTilingContainer", () => ({
  PaneTilingContainer: () => {
    const { useCockpitActions } = jest.requireActual("@/lib/contexts/CockpitActionsContext");
    const { onSessionClick } = useCockpitActions();
    return (
      <>
        <button data-testid="open-session-1" onClick={() => onSessionClick(sessionOne)}>
          Session One
        </button>
        <button data-testid="open-session-2" onClick={() => onSessionClick(sessionTwo)}>
          Session Two
        </button>
      </>
    );
  },
}));

import HomePage from "../page";

describe("Delete confirmation dialog focus restoration", () => {
  beforeEach(() => {
    mockPush.mockClear();
    mockDeleteSession.mockClear();
  });

  it("DeleteConfirmDialog_should_restoreFocusToSelectedSessionRow_When_cancelledViaCancelButton", async () => {
    render(<HomePage />);
    const opener = await screen.findByTestId("open-session-1");
    opener.focus();
    fireEvent.click(opener);

    fireEvent.keyDown(document, { key: "d" });
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("DeleteConfirmDialog_should_restoreFocusToSelectedSessionRow_When_cancelledViaCloseButton", async () => {
    render(<HomePage />);
    const opener = await screen.findByTestId("open-session-2");
    opener.focus();
    fireEvent.click(opener);

    fireEvent.keyDown(document, { key: "d" });
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    fireEvent.click(screen.getByLabelText("Close"));

    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(document.activeElement).not.toBe(screen.getByTestId("open-session-1"));
  });

  it("DeleteConfirmDialog_should_restoreFocus_When_closedViaEscape", async () => {
    render(<HomePage />);
    const opener = await screen.findByTestId("open-session-1");
    opener.focus();
    fireEvent.click(opener);

    fireEvent.keyDown(document, { key: "d" });
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("DeleteConfirmDialog_should_callDeleteSession_When_confirmed", async () => {
    render(<HomePage />);
    const opener = await screen.findByTestId("open-session-1");
    opener.focus();
    fireEvent.click(opener);

    fireEvent.keyDown(document, { key: "d" });
    await waitFor(() => expect(screen.getByRole("dialog")).not.toBeNull());

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(mockDeleteSession).toHaveBeenCalledWith(sessionOne.id));
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });
});
