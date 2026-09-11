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
import { render, screen, fireEvent, within } from "@testing-library/react";
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
let mockHasLoadedSessionsOnce = true;
jest.mock("@/lib/store", () => ({
  // selectAllSessions/selectSessionsHasLoadedOnce (mocked below) ignore the state argument entirely.
  useAppSelector: (selector: unknown) => (selector as (s: unknown) => unknown)(undefined),
}));
jest.mock("@/lib/store/sessionsSlice", () => ({
  selectAllSessions: () => mockLiveSessions,
  selectSessionsHasLoadedOnce: () => mockHasLoadedSessionsOnce,
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
let mockHistoryError: Error | null = null;
let mockHistoryLastUpdatedAt: number | null = null;
let mockUnreadCountOverride: number | null = null;
const mockLoadMoreHistory = jest.fn();
const mockMarkAsRead = jest.fn();
const mockClearHistory = jest.fn();
const mockRefreshHistory = jest.fn();
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    notificationHistory: mockHistory,
    markAsRead: mockMarkAsRead,
    removeFromHistory: jest.fn(),
    acknowledgeNotification: jest.fn(),
    clearHistory: mockClearHistory,
    getUnreadCount: () => mockUnreadCountOverride ?? mockHistory.filter((n) => !n.isRead).length,
    historyLoading: false,
    historyHasMore: mockHistoryHasMore,
    historyError: mockHistoryError,
    historyLastUpdatedAt: mockHistoryLastUpdatedAt,
    loadMoreHistory: mockLoadMoreHistory,
    refreshHistory: mockRefreshHistory,
  }),
}));

/** Recent Activity is a CollapsibleSection, collapsed by default — expand it
 * before asserting on items rendered inside it. */
function expandRecentActivity() {
  fireEvent.click(screen.getByTestId("collapsible-header-recent-activity"));
}

describe("NotificationsPage — session link fallback (Task 3.3.2b)", () => {
  beforeEach(() => {
    mockLiveSessions = [];
    mockHasLoadedSessionsOnce = true;
    mockHistory = [];
  });

  it("navigates to the durable summary route when the notification's session has no live match", () => {
    mockHistory = [makeNotification({ id: "notif-deleted", sessionId: "sess-deleted" })];
    mockLiveSessions = []; // sess-deleted is not in the live list

    render(<NotificationsPage />);
    expandRecentActivity();

    const link = screen.getByRole("link", { name: "View Session" });
    expect(link).toHaveAttribute("href", "/sessions/summary?sessionId=sess-deleted");
  });

  it("keeps the existing live-list route unaffected when the session is still live", () => {
    mockHistory = [makeNotification({ id: "notif-live", sessionId: "sess-live" })];
    mockLiveSessions = [{ id: "sess-live" }];

    render(<NotificationsPage />);
    expandRecentActivity();

    const link = screen.getByRole("link", { name: "View Session" });
    expect(link).toHaveAttribute("href", "/?session=sess-live");
  });

  it("defaults to the live route before the sessions store has ever loaded, even though the id isn't in the (still-empty) live list", () => {
    // Regression: a fresh page load whose WatchSessions snapshot hasn't landed yet must
    // not be mistaken for "session confirmed gone" — that raced a real, active session
    // into the summary route instead of the terminal view.
    mockHistory = [makeNotification({ id: "notif-racing", sessionId: "sess-still-loading" })];
    mockLiveSessions = [];
    mockHasLoadedSessionsOnce = false;

    render(<NotificationsPage />);
    expandRecentActivity();

    const link = screen.getByRole("link", { name: "View Session" });
    expect(link).toHaveAttribute("href", "/?session=sess-still-loading");
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
    expandRecentActivity();

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
    expandRecentActivity();

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
    expandRecentActivity();

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
    expandRecentActivity();

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
    expandRecentActivity();

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
    expandRecentActivity();

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
    expandRecentActivity();
    expect(screen.getByText("Only Item")).toBeInTheDocument();
    // The clear button itself only renders while a query is present.
    expect(screen.queryByRole("button", { name: "Clear search" })).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Story 3.1.2: "Needs a decision" section always on top
// ---------------------------------------------------------------------------

function resetSharedMocks() {
  mockLiveSessions = [];
  mockHistory = [];
  mockHistoryHasMore = false;
  mockHistoryError = null;
  mockHistoryLastUpdatedAt = null;
  mockUnreadCountOverride = null;
  mockMarkAsRead.mockClear();
  mockClearHistory.mockClear();
  mockRefreshHistory.mockClear();
}

describe("NotificationsPage — NeedsDecisionSection tiering (Task 3.1.2c/3.1.2d)", () => {
  beforeEach(resetSharedMocks);

  it("unread actionable notifications render in NeedsDecisionSection above informational list", () => {
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        sessionId: "sess-a1b2c3",
        sessionName: "Approval Session",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
      }),
      makeNotification({ id: "notif-1", sessionId: "sess-x", sessionName: "Complete 1", notificationType: "task_complete", isRead: true }),
      makeNotification({ id: "notif-2", sessionId: "sess-y", sessionName: "Complete 2", notificationType: "task_complete", isRead: true }),
      makeNotification({ id: "notif-3", sessionId: "sess-z", sessionName: "Complete 3", notificationType: "task_complete", isRead: true }),
    ];

    render(<NotificationsPage />);

    const section = screen.getByTestId("needs-decision-section");
    expect(section).toHaveTextContent("Approval Session");
    // task_complete items are not in NeedsDecisionSection — only visible once expanded below.
    expect(section).not.toHaveTextContent("Complete 1");
    expandRecentActivity();
    expect(screen.getByText("Complete 1")).toBeInTheDocument();
    expect(screen.getByText("Complete 2")).toBeInTheDocument();
    expect(screen.getByText("Complete 3")).toBeInTheDocument();
  });

  it("shows 'All caught up' when there is nothing to decide, and Recent Activity stays visible collapsed", () => {
    mockHistory = [
      makeNotification({ id: "notif-1", sessionName: "Complete 1", notificationType: "task_complete", isRead: true }),
    ];

    render(<NotificationsPage />);

    const emptyState = screen.getByTestId("needs-decision-empty");
    expect(within(emptyState).getByText("All caught up")).toBeInTheDocument();
    expect(screen.queryByText("No items found")).not.toBeInTheDocument();
    // Recent Activity is present (collapsed) rather than hidden or expanded to fill the space.
    expect(screen.getByTestId("collapsible-header-recent-activity")).toBeInTheDocument();
    expect(screen.queryByText("Complete 1")).not.toBeInTheDocument();
  });

  it("shows the 'hidden by filter' state (not calm empty) when a filter hides a real actionable item, with Recent Activity non-empty", () => {
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        sessionName: "Approval Session",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
      }),
      makeNotification({ id: "notif-1", sessionName: "Complete 1", notificationType: "task_complete", isRead: true }),
    ];

    render(<NotificationsPage />);
    fireEvent.click(screen.getByRole("button", { name: "Error" }));

    expect(within(screen.getByTestId("needs-decision-hidden-by-filter")).getByText("1 item needs a decision, but is hidden by your filter")).toBeInTheDocument();
    expect(screen.queryByText("All caught up")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Clear filter" }));
    expect(screen.getByTestId("needs-decision-section")).toHaveTextContent("Approval Session");
  });

  it("shows the 'hidden by filter' state, not the legacy 'No matching notifications' state, when the filter empties Recent Activity too (round-5 fix)", () => {
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        sessionName: "Approval Session",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
      }),
    ];

    render(<NotificationsPage />);
    fireEvent.click(screen.getByRole("button", { name: "Error" }));

    expect(within(screen.getByTestId("needs-decision-hidden-by-filter")).getByText("1 item needs a decision, but is hidden by your filter")).toBeInTheDocument();
    expect(screen.queryByText("No matching notifications")).not.toBeInTheDocument();
  });
});

describe("NotificationsPage — scoped bulk-read button (Task 3.1.2e/3.1.2f)", () => {
  beforeEach(resetSharedMocks);

  it("marks only non-actionable unread IDs read, leaving the approval_needed item unread", () => {
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        sessionName: "Approval Session",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
      }),
      makeNotification({ id: "notif-complete", sessionId: "sess-c", sessionName: "Complete", notificationType: "task_complete" }),
    ];

    render(<NotificationsPage />);

    const button = screen.getByRole("button", { name: "Mark activity as read" });
    fireEvent.click(button);

    expect(mockMarkAsRead).toHaveBeenCalledWith(["notif-complete"]);
  });

  it("is absent when NeedsDecisionSection has unread items but Recent Activity/Auto-handled do not", () => {
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        sessionName: "Approval Session",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
      }),
    ];

    render(<NotificationsPage />);

    expect(screen.queryByRole("button", { name: "Mark activity as read" })).not.toBeInTheDocument();
  });

  it("renders the visible label 'Mark activity read'", () => {
    mockHistory = [
      makeNotification({ id: "notif-complete", sessionName: "Complete", notificationType: "task_complete" }),
    ];
    render(<NotificationsPage />);
    expect(screen.getByText("Mark activity read")).toBeInTheDocument();
  });
});

describe("NotificationsPage — no ✕ control in NeedsDecisionSection (Task 3.1.2g)", () => {
  beforeEach(resetSharedMocks);

  it("renders no 'Remove notification' control for an unread approval_needed item in NeedsDecisionSection, but does for the same item once read in Recent Activity", () => {
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        sessionName: "Approval Session",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
      }),
    ];

    const { rerender } = render(<NotificationsPage />);
    expect(screen.getByTestId("needs-decision-section")).toBeInTheDocument();
    expect(screen.queryByLabelText("Remove notification")).not.toBeInTheDocument();

    // Once read, the same underlying item renders in Recent Activity, where the
    // ✕ control is expected to work exactly as before (scoping fix, not removal).
    mockHistory = [
      makeNotification({
        id: "notif-approval",
        sessionName: "Approval Session",
        notificationType: "approval_needed",
        metadata: { approval_id: "appr-1" },
        isRead: true,
      }),
    ];
    rerender(<NotificationsPage />);
    expandRecentActivity();
    expect(screen.getByLabelText("Remove notification")).toBeInTheDocument();
  });
});

describe("NotificationsPage — staleness indicator (Task 3.1.2h/3.1.2i)", () => {
  beforeEach(resetSharedMocks);

  it("shows 'Last updated <Xm ago> · Retry' when historyError is set, without replacing the last-known list", () => {
    mockHistory = [makeNotification({ id: "notif-1", sessionName: "Only Item", notificationType: "task_complete", isRead: true })];
    mockHistoryError = new Error("network error");
    mockHistoryLastUpdatedAt = Date.now() - 3 * 60_000;

    render(<NotificationsPage />);

    expect(screen.getByTestId("needs-decision-staleness")).toHaveTextContent("Last updated 3m ago");
    expandRecentActivity();
    expect(screen.getByText("Only Item")).toBeInTheDocument();
  });

  it("calls refreshHistory when Retry is clicked", () => {
    mockHistory = [makeNotification({ id: "notif-1", notificationType: "task_complete", isRead: true })];
    mockHistoryError = new Error("network error");
    mockHistoryLastUpdatedAt = Date.now();

    render(<NotificationsPage />);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(mockRefreshHistory).toHaveBeenCalled();
  });

  it("does not render the staleness indicator when there has been no fetch failure", () => {
    mockHistory = [makeNotification({ id: "notif-1", notificationType: "task_complete", isRead: true })];
    render(<NotificationsPage />);
    expect(screen.queryByTestId("needs-decision-staleness")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Story 3.1.3: Auto-handled section extended to reconciled items
// ---------------------------------------------------------------------------

describe("NotificationsPage — reconciled items in AutoHandledSection (Task 3.1.3b)", () => {
  beforeEach(resetSharedMocks);

  it("a reconciled approval_needed record lands in AutoHandledSection, not the main feed", () => {
    mockHistory = [
      makeNotification({
        id: "notif-reconciled",
        sessionName: "Reconciled Session",
        notificationType: "approval_needed",
        metadata: {
          approval_id: "appr-1",
          approval_decision: "allow",
          reconciled: "true",
          tool_name: "Bash: git status",
        },
      }),
    ];

    render(<NotificationsPage />);

    // Excluded from the main feed entirely — with no other items, the page reads
    // as calm "All caught up," not showing the reconciled item's content.
    expect(screen.queryByText("Bash: git status")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Auto-handled/ }));
    expect(screen.getByText("Bash: git status")).toBeInTheDocument();
  });
});

describe("NotificationsPage — Info filter excludes auto_approved (review finding #3)", () => {
  beforeEach(resetSharedMocks);

  it("never renders an auto_approved notification under the Info filter, in Recent Activity or elsewhere", () => {
    mockHistory = [
      makeNotification({
        id: "notif-auto",
        sessionName: "Auto Approved Session",
        notificationType: "auto_approved",
      }),
      makeNotification({
        id: "notif-info",
        sessionName: "Informational Session",
        notificationType: "info",
        isRead: true,
      }),
    ];

    render(<NotificationsPage />);
    fireEvent.click(screen.getByRole("button", { name: "Info" }));
    expandRecentActivity();

    expect(screen.queryByText("Auto Approved Session")).not.toBeInTheDocument();
    expect(screen.getByText("Informational Session")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Story 3.1.4: Cap the headline unread count
// ---------------------------------------------------------------------------

describe("NotificationsPage — header unread count cap (Task 3.1.4b)", () => {
  beforeEach(resetSharedMocks);

  it("caps at 99+ above 99, and shows the literal number at/below 99", () => {
    mockHistory = [makeNotification({ id: "notif-1" })];

    mockUnreadCountOverride = 152;
    const { rerender } = render(<NotificationsPage />);
    expect(screen.getByTestId("notifications-unread-badge")).toHaveTextContent("99+");

    mockUnreadCountOverride = 99;
    rerender(<NotificationsPage />);
    expect(screen.getByTestId("notifications-unread-badge")).toHaveTextContent("99");

    mockUnreadCountOverride = 7;
    rerender(<NotificationsPage />);
    expect(screen.getByTestId("notifications-unread-badge")).toHaveTextContent("7");
  });
});

// ---------------------------------------------------------------------------
// Story 3.1.5: "Clear history" confirm gate (Task 3.1.5d/3.1.5e)
// ---------------------------------------------------------------------------

describe("NotificationsPage — 'Clear history' confirm gate (Task 3.1.5d)", () => {
  beforeEach(() => {
    resetSharedMocks();
    mockHistory = [makeNotification({ id: "notif-1", notificationType: "task_complete", isRead: true })];
  });

  it("is a no-op on any item when the confirm dialog is dismissed", () => {
    jest.spyOn(window, "confirm").mockReturnValue(false);
    render(<NotificationsPage />);
    fireEvent.click(screen.getByRole("button", { name: "Clear notification history" }));
    expect(mockClearHistory).not.toHaveBeenCalled();
    (window.confirm as jest.Mock).mockRestore();
  });

  it("calls clearHistory once the confirm dialog is accepted", () => {
    jest.spyOn(window, "confirm").mockReturnValue(true);
    render(<NotificationsPage />);
    fireEvent.click(screen.getByRole("button", { name: "Clear notification history" }));
    expect(mockClearHistory).toHaveBeenCalled();
    (window.confirm as jest.Mock).mockRestore();
  });

  it("renders the renamed 'Clear history' label", () => {
    render(<NotificationsPage />);
    expect(screen.getByText("Clear history")).toBeInTheDocument();
  });
});
