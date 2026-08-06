/**
 * Epic 3.3 (session-completion-summary), Task 3.3.2b: "View Session" link
 * fallback for notifications whose session is no longer in the live list.
 *
 * A notification's sessionId may reference a session that's since been
 * deleted (e.g. after DeleteSession) and so no longer appears in the
 * Redux `selectAllSessions` list. In that case the link must fall back to
 * the durable standalone `/sessions/summary?sessionId=<id>` route instead of the
 * live-list-dependent `/?session=<id>` route, which would be a dead link.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { NotificationsPage } from "../NotificationsPage";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import type { Session } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Heavy dependency mocks
// ---------------------------------------------------------------------------

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({})),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
}));

jest.mock("@/lib/hooks/useAuditLog", () => ({
  useAuditLog: () => ({
    logNotificationSessionViewed: jest.fn(),
    logNotificationViewed: jest.fn(),
  }),
}));

let mockLiveSessions: Pick<Session, "id">[] = [];
jest.mock("@/lib/store", () => ({
  // selectAllSessions (mocked below) ignores the state argument entirely.
  useAppSelector: (selector: unknown) => (selector as (s: unknown) => unknown)(undefined),
}));
jest.mock("@/lib/store/sessionsSlice", () => ({
  selectAllSessions: () => mockLiveSessions,
}));

function makeNotification(overrides: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem {
  return {
    id: overrides.id ?? "notif-1",
    sessionId: "sess-1",
    sessionName: "my-session",
    message: "Something happened",
    timestamp: Date.now(),
    isRead: false,
    notificationType: "info",
    ...overrides,
  };
}

let mockHistory: NotificationHistoryItem[] = [];
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    notificationHistory: mockHistory,
    markAsRead: jest.fn(),
    markAllAsRead: jest.fn(),
    removeFromHistory: jest.fn(),
    acknowledgeNotification: jest.fn(),
    clearHistory: jest.fn(),
    getUnreadCount: () => mockHistory.filter((n) => !n.isRead).length,
    historyLoading: false,
    historyHasMore: false,
    loadMoreHistory: jest.fn(),
  }),
}));

describe("NotificationsPage — session link fallback (Task 3.3.2b)", () => {
  beforeEach(() => {
    mockLiveSessions = [];
    mockHistory = [];
  });

  it("navigates to the durable summary route when the notification's session has no live match", () => {
    mockHistory = [makeNotification({ id: "notif-deleted", sessionId: "sess-deleted" })];
    mockLiveSessions = []; // sess-deleted is not in the live list

    render(<NotificationsPage />);

    const link = screen.getByRole("link", { name: "View Session" });
    expect(link).toHaveAttribute("href", "/sessions/summary?sessionId=sess-deleted");
  });

  it("keeps the existing live-list route unaffected when the session is still live", () => {
    mockHistory = [makeNotification({ id: "notif-live", sessionId: "sess-live" })];
    mockLiveSessions = [{ id: "sess-live" }];

    render(<NotificationsPage />);

    const link = screen.getByRole("link", { name: "View Session" });
    expect(link).toHaveAttribute("href", "/?session=sess-live");
  });
});
