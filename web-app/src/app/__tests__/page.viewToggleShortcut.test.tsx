/**
 * 'b' keyboard shortcut toggles the List/Board view mode (AC1), guarded against
 * firing while a text input is focused (useKeyboard's default ignoreElements).
 */

import React from "react";
import { render, fireEvent, screen } from "@testing-library/react";
import type { Session } from "@/gen/session/v1/types_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";

if (typeof performance.mark !== "function") {
  performance.mark = jest.fn();
}

jest.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: () => null }),
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
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

const stableSessions: Session[] = [makeSession("s1")];

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
    deleteSession: jest.fn(),
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

// Stub the tiling container with the real SessionViewModeContext consumer plus
// a text input, so the guard-against-typing case has somewhere to type "b" into.
jest.mock("@/components/pane/PaneTilingContainer", () => ({
  PaneTilingContainer: () => {
    const { useSessionViewModeContext } = jest.requireActual("@/lib/contexts/SessionViewModeContext");
    const { viewMode } = useSessionViewModeContext();
    return (
      <>
        <div data-testid="current-view-mode">{viewMode}</div>
        <input data-testid="search-input" aria-label="search" />
      </>
    );
  },
}));

import HomePage from "../page";

describe("'b' keyboard shortcut toggles List/Board view", () => {
  it("toggles view mode from list to board when no input is focused", async () => {
    render(<HomePage />);
    expect(await screen.findByTestId("current-view-mode")).toHaveTextContent("list");

    fireEvent.keyDown(document.body, { key: "b" });

    expect(await screen.findByTestId("current-view-mode")).toHaveTextContent("board");
  });

  it("toggles back to list on a second press", async () => {
    render(<HomePage />);
    fireEvent.keyDown(document.body, { key: "b" });
    expect(await screen.findByTestId("current-view-mode")).toHaveTextContent("board");

    fireEvent.keyDown(document.body, { key: "b" });
    expect(await screen.findByTestId("current-view-mode")).toHaveTextContent("list");
  });

  it("does not toggle while a text input is focused", async () => {
    render(<HomePage />);
    const input = await screen.findByTestId("search-input");
    input.focus();

    fireEvent.keyDown(input, { key: "b" });

    expect(screen.getByTestId("current-view-mode")).toHaveTextContent("list");
  });
});
