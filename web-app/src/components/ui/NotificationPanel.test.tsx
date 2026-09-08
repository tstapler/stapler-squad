/**
 * Task 3.1.5e: NotificationPanel is a third live exit path for the "an
 * unresolved decision silently leaves the needs-a-decision view" failure
 * mode already fixed once for the Notifications page (Story 3.1.2) — this
 * suite pins the same three guarantees for the header bell dropdown:
 *   - "Mark activity read" never marks an unread actionable item as read.
 *   - No ✕ control renders for an unread actionable item.
 *   - "Clear history" requires confirmation before it runs.
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { NotificationPanel } from "./NotificationPanel";
import type { NotificationHistoryItem } from "@/lib/types/notification";

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
    logNotificationPanelOpened: jest.fn(),
    logNotificationPanelClosed: jest.fn(),
  }),
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
const mockMarkAsRead = jest.fn();
const mockRemoveFromHistory = jest.fn();
const mockClearHistory = jest.fn();
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    notificationHistory: mockHistory,
    isPanelOpen: true,
    togglePanel: jest.fn(),
    markAsRead: mockMarkAsRead,
    markAllAsRead: jest.fn(),
    removeFromHistory: mockRemoveFromHistory,
    acknowledgeNotification: jest.fn(),
    clearHistory: mockClearHistory,
    getUnreadCount: () => mockHistory.filter((n) => !n.isRead).length,
    historyLoading: false,
    historyHasMore: false,
    loadMoreHistory: jest.fn(),
  }),
}));

describe("NotificationPanel — scoped bulk-read (Task 3.1.5a)", () => {
  beforeEach(() => {
    mockHistory = [];
    mockMarkAsRead.mockClear();
  });

  it("never marks an unread approval_needed item read, only non-actionable unread items", () => {
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
      }),
      makeNotification({ id: "notif-complete", notificationType: "task_complete" }),
    ];

    render(<NotificationPanel />);
    fireEvent.click(screen.getByRole("button", { name: "Mark activity as read" }));

    expect(mockMarkAsRead).toHaveBeenCalledWith(["notif-complete"]);
  });

  it("is absent when the only unread items are actionable", () => {
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
      }),
    ];

    render(<NotificationPanel />);
    expect(screen.queryByRole("button", { name: "Mark activity as read" })).not.toBeInTheDocument();
  });
});

describe("NotificationPanel — no ✕ control for an unread actionable item (Task 3.1.5b)", () => {
  beforeEach(() => {
    mockHistory = [];
  });

  it("renders no 'Remove notification' control for an unread approval_needed item, but does for a read/informational item", () => {
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
      }),
      makeNotification({ id: "notif-complete", notificationType: "task_complete", isRead: true }),
    ];

    render(<NotificationPanel />);

    const removeButtons = screen.getAllByLabelText("Remove notification");
    // Only the read/informational item gets a ✕ — the unread actionable one doesn't.
    expect(removeButtons).toHaveLength(1);
  });
});

describe("NotificationPanel — 'Clear history' confirm gate (Task 3.1.5d)", () => {
  beforeEach(() => {
    mockHistory = [makeNotification({ id: "notif-1", isRead: true })];
    mockClearHistory.mockClear();
  });

  it("is a no-op when the confirm dialog is dismissed", () => {
    jest.spyOn(window, "confirm").mockReturnValue(false);
    render(<NotificationPanel />);
    fireEvent.click(screen.getByRole("button", { name: "Clear notification history" }));
    expect(mockClearHistory).not.toHaveBeenCalled();
    (window.confirm as jest.Mock).mockRestore();
  });

  it("calls clearHistory once the confirm dialog is accepted", () => {
    jest.spyOn(window, "confirm").mockReturnValue(true);
    render(<NotificationPanel />);
    fireEvent.click(screen.getByRole("button", { name: "Clear notification history" }));
    expect(mockClearHistory).toHaveBeenCalled();
    (window.confirm as jest.Mock).mockRestore();
  });

  it("renders the renamed 'Clear history' label", () => {
    render(<NotificationPanel />);
    expect(screen.getByText("Clear history")).toBeInTheDocument();
  });
});
