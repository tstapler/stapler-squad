import { renderHook } from "@testing-library/react";
import React from "react";
import {
  SessionServiceContext,
  useSessionServiceContext,
} from "./SessionServiceContext";
import type { SessionServiceContextValue } from "./SessionServiceContext";

// The context pulls in next/navigation and many heavy dependencies via
// GlobalSessionServiceProvider.  We only need to test the context plumbing
// itself (Provider + useSessionServiceContext hook), so we mock the heavy
// modules that the file imports at the top level.

jest.mock("next/navigation", () => ({ useRouter: () => ({ push: jest.fn() }) }));
jest.mock("@/lib/hooks/useSessionService", () => ({ useSessionService: jest.fn() }));
jest.mock("@/lib/hooks/useSessionNotifications", () => ({ useSessionNotifications: jest.fn(() => jest.fn()) }));
jest.mock("@/lib/contexts/AuthContext", () => ({ useAuth: () => ({ authEnabled: false, authenticated: true, loading: false }) }));
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    refreshHistory: jest.fn(),
    markAsReadBySessionId: jest.fn(),
    removeToastByApprovalId: jest.fn(),
    removeToastBySessionId: jest.fn(),
  }),
}));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost:8543" }));
jest.mock("@/lib/utils/notifications", () => ({
  closeNativeNotification: jest.fn(),
  notificationTag: { approval: jest.fn(), tier1Review: jest.fn() },
}));

describe("SessionServiceContext", () => {
  it("useSessionServiceContext_should_exposeReconnectAttemptCount_When_providerSetsIt", () => {
    const mockValue: SessionServiceContextValue = {
      sessions: [],
      loading: false,
      error: null,
      connectionState: "connected",
      systemMemoryPct: 0,
      reconnectAttemptCount: 5,
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
      retrySession: jest.fn(),
      clearConversationState: jest.fn(),
      acknowledgeSession: jest.fn(),
      createCheckpoint: jest.fn(),
      listCheckpoints: jest.fn(),
      forkSession: jest.fn(),
      runOneShot: jest.fn(),
      listPromptHistory: jest.fn(),
      watchSessions: jest.fn(),
      stopWatching: jest.fn(),
    };

    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(SessionServiceContext.Provider, { value: mockValue }, children);

    const { result } = renderHook(() => useSessionServiceContext(), { wrapper });
    expect(result.current.reconnectAttemptCount).toBe(5);
  });

  it("useSessionServiceContext_should_throwError_When_usedOutsideProvider", () => {
    // Suppress the expected console.error from React
    const spy = jest.spyOn(console, "error").mockImplementation(() => {});
    expect(() => renderHook(() => useSessionServiceContext())).toThrow(
      "useSessionServiceContext must be used within GlobalSessionServiceProvider"
    );
    spy.mockRestore();
  });
});
