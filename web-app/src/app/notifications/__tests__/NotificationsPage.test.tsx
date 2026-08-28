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
import { render, screen, fireEvent } from "@testing-library/react";
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
let mockHistoryHasMore = false;
const mockLoadMoreHistory = jest.fn();
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
    historyHasMore: mockHistoryHasMore,
    loadMoreHistory: mockLoadMoreHistory,
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

describe("NotificationsPage — hide backlog items toggle", () => {
  beforeEach(() => {
    mockLiveSessions = [];
    mockHistory = [];
    mockHistoryHasMore = false;
  });

  it("hides backlog-item notifications (those with metadata.item_id) when toggled on, and restores them when toggled off", () => {
    mockHistory = [
      makeNotification({ id: "notif-backlog", sessionId: "sess-backlog", sessionName: "Backlog Item", metadata: { item_id: "item-1" } }),
      makeNotification({ id: "notif-regular", sessionId: "sess-regular", sessionName: "Regular Item" }),
    ];

    render(<NotificationsPage />);

    expect(screen.getByText("Backlog Item")).toBeInTheDocument();
    expect(screen.getByText("Regular Item")).toBeInTheDocument();

    const toggle = screen.getByRole("button", { name: "Exclude backlog notifications" });
    fireEvent.click(toggle);

    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByText("Backlog Item")).not.toBeInTheDocument();
    expect(screen.getByText("Regular Item")).toBeInTheDocument();

    fireEvent.click(toggle);

    expect(toggle).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByText("Backlog Item")).toBeInTheDocument();
    expect(screen.getByText("Regular Item")).toBeInTheDocument();
  });
});

describe("NotificationsPage — search across extended fields", () => {
  beforeEach(() => {
    mockLiveSessions = [];
    mockHistory = [];
    mockHistoryHasMore = false;
  });

  function search(query: string) {
    fireEvent.change(screen.getByLabelText("Search notifications"), { target: { value: query } });
  }

  it("matches on sourceProject", () => {
    mockHistory = [
      makeNotification({ id: "notif-1", sessionId: "sess-a", sessionName: "First", sourceProject: "stapler-squad" }),
      makeNotification({ id: "notif-2", sessionId: "sess-b", sessionName: "Second", sourceProject: "other-repo" }),
    ];
    render(<NotificationsPage />);

    search("stapler-squad");

    expect(screen.getByText("First")).toBeInTheDocument();
    expect(screen.queryByText("Second")).not.toBeInTheDocument();
  });

  it("matches on sourceWorkingDir", () => {
    mockHistory = [
      makeNotification({ id: "notif-1", sessionId: "sess-a", sessionName: "First", sourceWorkingDir: "/home/user/worktrees/feature-x" }),
      makeNotification({ id: "notif-2", sessionId: "sess-b", sessionName: "Second", sourceWorkingDir: "/home/user/worktrees/feature-y" }),
    ];
    render(<NotificationsPage />);

    search("feature-x");

    expect(screen.getByText("First")).toBeInTheDocument();
    expect(screen.queryByText("Second")).not.toBeInTheDocument();
  });

  it("matches on metadata.tool_name", () => {
    mockHistory = [
      makeNotification({ id: "notif-1", sessionId: "sess-a", sessionName: "First", metadata: { tool_name: "Bash" } }),
      makeNotification({ id: "notif-2", sessionId: "sess-b", sessionName: "Second", metadata: { tool_name: "Edit" } }),
    ];
    render(<NotificationsPage />);

    search("bash");

    expect(screen.getByText("First")).toBeInTheDocument();
    expect(screen.queryByText("Second")).not.toBeInTheDocument();
  });

  it("matches on metadata.tool_input_command", () => {
    mockHistory = [
      makeNotification({ id: "notif-1", sessionId: "sess-a", sessionName: "First", metadata: { tool_input_command: "npm run build" } }),
      makeNotification({ id: "notif-2", sessionId: "sess-b", sessionName: "Second", metadata: { tool_input_command: "go test ./..." } }),
    ];
    render(<NotificationsPage />);

    search("npm run");

    expect(screen.getByText("First")).toBeInTheDocument();
    expect(screen.queryByText("Second")).not.toBeInTheDocument();
  });

  it("matches on metadata.tool_input_file", () => {
    mockHistory = [
      makeNotification({ id: "notif-1", sessionId: "sess-a", sessionName: "First", metadata: { tool_input_file: "src/index.ts" } }),
      makeNotification({ id: "notif-2", sessionId: "sess-b", sessionName: "Second", metadata: { tool_input_file: "src/other.ts" } }),
    ];
    render(<NotificationsPage />);

    search("index.ts");

    expect(screen.getByText("First")).toBeInTheDocument();
    expect(screen.queryByText("Second")).not.toBeInTheDocument();
  });
});

describe("NotificationsPage — clear search button", () => {
  beforeEach(() => {
    mockLiveSessions = [];
    mockHistory = [];
    mockHistoryHasMore = false;
  });

  it("resets the search query and clears the active-filter empty state when clicked", () => {
    mockHistory = [makeNotification({ id: "notif-1", sessionName: "Only Item" })];
    render(<NotificationsPage />);

    const input = screen.getByLabelText("Search notifications") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "no-match-at-all" } });

    expect(input.value).toBe("no-match-at-all");
    expect(screen.getByText("No matching notifications")).toBeInTheDocument();

    const clearButton = screen.getByRole("button", { name: "Clear search" });
    fireEvent.click(clearButton);

    expect(input.value).toBe("");
    expect(screen.queryByText("No matching notifications")).not.toBeInTheDocument();
    expect(screen.getByText("Only Item")).toBeInTheDocument();
    // The clear button itself only renders while a query is present.
    expect(screen.queryByRole("button", { name: "Clear search" })).not.toBeInTheDocument();
  });
});
