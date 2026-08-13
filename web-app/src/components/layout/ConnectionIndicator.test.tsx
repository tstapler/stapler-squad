import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import { ConnectionIndicator } from "./ConnectionIndicator";
import sessionsReducer from "@/lib/store/sessionsSlice";
import * as SessionServiceContextModule from "@/lib/contexts/SessionServiceContext";

// Mock the context module
jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  ...jest.requireActual("@/lib/contexts/SessionServiceContext"),
  useSessionServiceContext: jest.fn(),
}));

const mockUseSessionServiceContext = SessionServiceContextModule.useSessionServiceContext as jest.Mock;

function createTestStore(connectionState: "connected" | "stale" | "disconnected") {
  return configureStore({
    reducer: {
      sessions: sessionsReducer,
    },
    preloadedState: {
      sessions: {
        ids: [],
        entities: {},
        loading: false,
        error: null,
        connectionState,
        detectedStatusMap: {},
        deletedIds: {},
      },
    },
  });
}

function makeMockContext(watchSessionsMock: jest.Mock, connectionState: "connected" | "stale" | "disconnected", reconnectAttemptCount = 0) {
  return {
    watchSessions: watchSessionsMock,
    reconnectAttemptCount,
    sessions: [],
    loading: false,
    error: null,
    connectionState,
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
    runOneShot: jest.fn(),
    listPromptHistory: jest.fn(),
    stopWatching: jest.fn(),
    archiveSession: jest.fn(),
    unarchiveSession: jest.fn(),
    listSessionsByWorkflow: jest.fn(),
    runWorkflow: jest.fn(),
    spawnShell: jest.fn(),
    stopShell: jest.fn(),
    restartShell: jest.fn(),
    listShells: jest.fn(),
    deleteShell: jest.fn(),
    getTerminalSnapshot: jest.fn(),
    writeToSession: jest.fn(),
    getConversationMessages: jest.fn(),
  };
}

function renderWithStore(
  connectionState: "connected" | "stale" | "disconnected",
  reconnectAttemptCount = 0
) {
  const watchSessionsMock = jest.fn();
  mockUseSessionServiceContext.mockReturnValue(makeMockContext(watchSessionsMock, connectionState, reconnectAttemptCount));

  const store = createTestStore(connectionState);
  const result = render(
    <Provider store={store}>
      <ConnectionIndicator />
    </Provider>
  );
  return { watchSessionsMock, store, ...result };
}

describe("ConnectionIndicator", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("ConnectionIndicator_should_callWatchSessions_When_clickedInStaleState", () => {
    const { watchSessionsMock } = renderWithStore("stale");
    const btn = screen.getByRole("button");
    fireEvent.click(btn);
    expect(watchSessionsMock).toHaveBeenCalledTimes(1);
  });

  it("ConnectionIndicator_should_callWatchSessions_When_clickedInDisconnectedState", () => {
    const { watchSessionsMock } = renderWithStore("disconnected");
    const btn = screen.getByRole("button");
    fireEvent.click(btn);
    expect(watchSessionsMock).toHaveBeenCalledTimes(1);
  });

  it("ConnectionIndicator_should_renderLiveLabel_When_connectionStateIsConnected", () => {
    const { watchSessionsMock } = renderWithStore("connected");
    expect(screen.getByRole("button")).toBeDisabled();
    expect(screen.getByText("Live")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button"));
    expect(watchSessionsMock).not.toHaveBeenCalled();
  });

  it("ConnectionIndicator_should_showAttemptCountInTooltip_When_reconnectAttemptCountIsThree", () => {
    renderWithStore("stale", 3);
    const btn = screen.getByRole("button");
    expect(btn).toHaveAttribute("title", expect.stringContaining("attempt 3"));
  });

  it("ConnectionIndicator_should_showReconnectingLabel_When_stateIsStaleOrDisconnected", () => {
    renderWithStore("stale");
    expect(screen.getByText("Reconnecting…")).toBeInTheDocument();
    expect(screen.queryByText("Stale")).not.toBeInTheDocument();
    expect(screen.queryByText("Offline")).not.toBeInTheDocument();
  });

  it("ConnectionIndicator_should_haveAriaLiveOnSeparateDiv_When_rendered", () => {
    renderWithStore("connected");
    const btn = screen.getByRole("button");
    expect(btn).not.toHaveAttribute("aria-live");
    const liveRegion = document.querySelector('div[aria-live="polite"]');
    expect(liveRegion).not.toBeNull();
  });

  it("ConnectionIndicator_should_announceReconnecting_When_connectionStateChangesToStale", () => {
    const watchSessionsMock = jest.fn();
    mockUseSessionServiceContext.mockReturnValue(makeMockContext(watchSessionsMock, "connected", 0));
    const store = createTestStore("connected");
    const { rerender } = render(
      <Provider store={store}>
        <ConnectionIndicator />
      </Provider>
    );

    const staleStore = createTestStore("stale");
    mockUseSessionServiceContext.mockReturnValue(makeMockContext(watchSessionsMock, "stale", 1));

    act(() => {
      rerender(
        <Provider store={staleStore}>
          <ConnectionIndicator />
        </Provider>
      );
    });

    const liveRegion = document.querySelector('div[aria-live="polite"]');
    expect(liveRegion?.textContent).toContain("Reconnecting");
  });

  it("ConnectionIndicator_should_announceConnectionRestored_When_connectionStateChangesToConnected", () => {
    const watchSessionsMock = jest.fn();
    mockUseSessionServiceContext.mockReturnValue(makeMockContext(watchSessionsMock, "stale", 1));
    const staleStore = createTestStore("stale");
    const { rerender } = render(
      <Provider store={staleStore}>
        <ConnectionIndicator />
      </Provider>
    );

    const connectedStore = createTestStore("connected");
    mockUseSessionServiceContext.mockReturnValue(makeMockContext(watchSessionsMock, "connected", 0));

    act(() => {
      rerender(
        <Provider store={connectedStore}>
          <ConnectionIndicator />
        </Provider>
      );
    });

    const liveRegion = document.querySelector('div[aria-live="polite"]');
    expect(liveRegion?.textContent).toContain("Connection restored");
  });

  it("ConnectionIndicator_should_showReloadEscapeHatch_When_tooltipIsOpen", () => {
    renderWithStore("stale");
    expect(screen.getByText("Reload page (resets state)")).toBeInTheDocument();
  });
});
