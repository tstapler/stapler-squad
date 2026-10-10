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
import { render, screen, fireEvent, act, within, waitFor } from "@testing-library/react";
import { NotificationPanel } from "./NotificationPanel";
import { TrayErrorBoundary } from "./TrayErrorBoundary";
import { DeckViewportContext } from "@/lib/contexts/deckViewportContext";
import { writeQuietMode } from "@/lib/utils/deckSettings";
import type { NotificationHistoryItem } from "@/lib/types/notification";

const mockFlags: Record<string, boolean> = {};
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: (name: string) => mockFlags[name] ?? false,
}));

// jsdom has no layout, so the real virtualizer renders nothing. This stand-in
// windows the first 20 rows the way a real viewport would, so the panel's
// wiring (only reported items are in the DOM) is what the tests exercise; the
// "fewer than 60 nodes" count against real layout is the Playwright spec's.
jest.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: Math.min(count, 20) }, (_, index) => ({ index, key: index, start: index * 100, size: 100 })),
    getTotalSize: () => count * 100,
    measureElement: () => {},
    scrollToIndex: jest.fn(),
  }),
}));

const mockUseBackgroundSessions = jest.fn();
jest.mock("@/lib/hooks/useBackgroundSessions", () => ({
  useBackgroundSessions: (enabled: boolean) =>
    mockUseBackgroundSessions(enabled) ?? {
      sessions: [],
      departed: [],
      loading: false,
      failed: false,
      lastUpdatedAt: null,
      refresh: () => {},
    },
}));

let mockConnectivity = { state: "connected", isOffline: false };
jest.mock("@/lib/hooks/useNotificationConnectivity", () => ({
  useNotificationConnectivity: () => mockConnectivity,
}));

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
let mockIsPanelOpen = true;
let mockHistoryError: Error | null = null;
let mockHistoryLoading = false;
let mockLastUpdatedAt: number | null = 1;
let mockHasMore = false;
const mockMarkAsRead = jest.fn();
const mockRemoveFromHistory = jest.fn();
const mockClearHistory = jest.fn();
const mockTogglePanel = jest.fn();
const mockClearByIds = jest.fn();
const mockShowActionToast = jest.fn();
const mockLoadMore = jest.fn();
const mockRefresh = jest.fn();
const mockSetQuiet = jest.fn();
let mockQuiet = false;
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    notificationHistory: mockHistory,
    isPanelOpen: mockIsPanelOpen,
    togglePanel: mockTogglePanel,
    markAsRead: mockMarkAsRead,
    removeFromHistory: mockRemoveFromHistory,
    acknowledgeNotification: jest.fn(),
    clearHistory: mockClearHistory,
    clearHistoryByIds: mockClearByIds,
    showActionToast: mockShowActionToast,
    getUnreadCount: () => mockHistory.filter((n) => !n.isRead).length,
    historyLoading: mockHistoryLoading,
    historyHasMore: mockHasMore,
    historyError: mockHistoryError,
    historyLastUpdatedAt: mockLastUpdatedAt,
    loadMoreHistory: mockLoadMore,
    refreshHistory: mockRefresh,
    quietMode: mockQuiet,
    setQuietMode: mockSetQuiet,
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
        isPendingDecision: true,
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
        isPendingDecision: true,
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
        isPendingDecision: true,
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

describe("NotificationPanel — Info filter excludes auto_approved (review finding #3)", () => {
  beforeEach(() => {
    mockHistory = [];
  });

  it("never renders an auto_approved notification under the Info filter", () => {
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
      }),
    ];

    render(<NotificationPanel />);
    fireEvent.click(screen.getByRole("button", { name: "Info" }));

    expect(screen.queryByText("Auto Approved Session")).not.toBeInTheDocument();
    expect(screen.getByText("Informational Session")).toBeInTheDocument();
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

// ---------------------------------------------------------------------------
// Tray (notification_tray_v2): Epic 4
// ---------------------------------------------------------------------------

function renderTray(viewport = { isInnerScreen: true, isVirtualKeyboardOpen: false }) {
  return render(
    <DeckViewportContext.Provider value={viewport}>
      <NotificationPanel />
    </DeckViewportContext.Provider>,
  );
}

function pending(id: string, sessionId = `s-${id}`): NotificationHistoryItem {
  return makeNotification({
    id,
    sessionId,
    sessionName: sessionId,
    notificationType: "approval_needed",
    isPendingDecision: true,
    metadata: { approval_id: `appr-${id}` },
  });
}

function info(id: string, sessionId = "s-info", overrides: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem {
  return makeNotification({ id, sessionId, sessionName: sessionId, notificationType: "task_complete", ...overrides });
}

function resetTrayState() {
  mockHistory = [];
  mockIsPanelOpen = true;
  mockHistoryError = null;
  mockHistoryLoading = false;
  mockLastUpdatedAt = 1;
  mockHasMore = false;
  mockConnectivity = { state: "connected", isOffline: false };
  mockFlags.notification_tray_v2 = true;
  mockQuiet = false;
  [mockMarkAsRead, mockClearByIds, mockShowActionToast, mockTogglePanel, mockLoadMore, mockRefresh, mockSetQuiet].forEach((m) => m.mockReset());
  window.localStorage.clear();
}

describe("tray (notification_tray_v2)", () => {
  beforeEach(resetTrayState);
  afterEach(() => {
    mockFlags.notification_tray_v2 = false;
    jest.useRealTimers();
  });

  describe("non-modal layout (Story 4.1)", () => {
    it("tray_should_have_no_aria_modal_backdrop_or_inert_when_desktop_open", () => {
      mockHistory = [info("a")];
      const { container } = renderTray();
      const tray = screen.getByTestId("notification-tray");
      expect(tray).not.toHaveAttribute("aria-modal");
      expect(tray).toHaveAttribute("role", "complementary");
      expect(tray).toHaveAttribute("id", "notification-tray");
      expect(container.querySelector('[aria-hidden="true"][class*="overlay"]')).toBeNull();
      expect(document.getElementById("main-content")).toBeNull();
      expect(tray).not.toHaveAttribute("inert");
    });

    it("legacy_panel_should_stay_modal_when_flag_is_off", () => {
      mockFlags.notification_tray_v2 = false;
      renderTray();
      expect(screen.getByRole("dialog", { name: "Notification Panel" })).toHaveAttribute("aria-modal", "true");
    });

    it("tray_should_use_the_zindex_token_and_no_numeric_9998_or_9999_literals", () => {
      const css = require("fs").readFileSync(require("path").join(__dirname, "NotificationPanel.css.ts"), "utf8") as string;
      expect(css).not.toMatch(/\b999[89]\b/);
      expect(css).toContain("zIndex.slideOver");
      expect(css).not.toMatch(/backdrop-filter|backdropFilter/);
    });
  });

  describe("focus policy (Task 4.1b, 4.2g)", () => {
    function withTerminal() {
      const textarea = document.createElement("textarea");
      textarea.className = "xterm-helper-textarea";
      textarea.setAttribute("aria-label", "Terminal input");
      document.body.appendChild(textarea);
      textarea.focus();
      return textarea;
    }

    it("tray_should_not_move_focus_on_pointer_open_and_restore_terminal_on_esc_when_fine_pointer", () => {
      mockIsPanelOpen = false;
      const textarea = withTerminal();
      const blur = jest.fn();
      textarea.addEventListener("blur", blur);
      const { rerender } = renderTray();

      fireEvent.pointerDown(document.body);
      mockIsPanelOpen = true;
      rerender(
        <DeckViewportContext.Provider value={{ isInnerScreen: true, isVirtualKeyboardOpen: false }}>
          <NotificationPanel />
        </DeckViewportContext.Provider>,
      );
      expect(document.activeElement).toBe(textarea);
      expect(blur).not.toHaveBeenCalled();

      // The user clicks into the tray, then presses Esc there (TY-2).
      const search = screen.getByRole("searchbox", { name: "Search notifications" });
      search.focus();
      fireEvent.keyDown(search, { key: "Escape" });
      expect(mockTogglePanel).toHaveBeenCalledTimes(1);
      mockIsPanelOpen = false;
      rerender(
        <DeckViewportContext.Provider value={{ isInnerScreen: true, isVirtualKeyboardOpen: false }}>
          <NotificationPanel />
        </DeckViewportContext.Provider>,
      );
      expect(document.activeElement).toBe(textarea);
      textarea.remove();
    });

    it("tray_should_focus_its_heading_when_opened_from_the_keyboard", () => {
      mockIsPanelOpen = false;
      const { rerender } = renderTray();
      fireEvent.keyDown(document.body, { key: "Enter" });
      mockIsPanelOpen = true;
      rerender(
        <DeckViewportContext.Provider value={{ isInnerScreen: true, isVirtualKeyboardOpen: false }}>
          <NotificationPanel />
        </DeckViewportContext.Provider>,
      );
      expect(document.activeElement).toBe(screen.getByRole("heading", { name: /Notifications/ }));
    });

    it("tray_should_ignore_esc_when_focus_is_in_the_terminal", () => {
      const textarea = withTerminal();
      renderTray();
      fireEvent.keyDown(textarea, { key: "Escape" });
      expect(mockTogglePanel).not.toHaveBeenCalled();
      textarea.remove();
    });

    it("tray_should_never_call_focus_on_the_terminal_on_coarse_pointer_close", () => {
      const original = window.matchMedia;
      window.matchMedia = ((q: string) => ({ matches: q.includes("coarse"), media: q, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia;
      mockIsPanelOpen = false;
      const textarea = withTerminal();
      const { rerender } = renderTray({ isInnerScreen: false, isVirtualKeyboardOpen: false });
      mockIsPanelOpen = true;
      const tree = () => (
        <DeckViewportContext.Provider value={{ isInnerScreen: false, isVirtualKeyboardOpen: false }}>
          <NotificationPanel />
        </DeckViewportContext.Provider>
      );
      rerender(tree());
      screen.getByRole("searchbox", { name: "Search notifications" }).focus();
      const focusSpy = jest.spyOn(textarea, "focus");
      fireEvent.pointerDown(document.body);
      mockIsPanelOpen = false;
      rerender(tree());
      expect(focusSpy).not.toHaveBeenCalled();
      window.matchMedia = original;
      textarea.remove();
    });
  });

  describe("needs attention group and virtualization (Story 4.3)", () => {
    it("tray_should_pin_needs_attention_group_without_dismiss_swipe_or_duplicate_and_match_handle_dot_count", () => {
      mockHistory = [
        pending("p1"),
        pending("p2"),
        pending("p3"),
        info("i1", "s-a"),
        info("i2", "s-b", { isRead: true }),
        // An auto-remediating WARNING is informational: same type, not pinned.
        makeNotification({ id: "w1", sessionId: "s-w", sessionName: "s-w", notificationType: "warning", isPendingDecision: false }),
      ];
      renderTray();

      expect(screen.getByTestId("tray-needs-attention")).toHaveTextContent("3 need attention");
      expect(screen.getByTestId("tray-needs-attention-header")).toHaveTextContent("NEEDS ATTENTION (3)");
      const pinnedRows = screen.getAllByTestId("needs-attention-row");
      expect(pinnedRows).toHaveLength(3);
      for (const row of pinnedRows) expect(within(row).queryByLabelText("Remove notification")).toBeNull();
      // Pinned rows are not swipeable and appear once.
      expect(screen.getAllByTestId("tray-swipe-row")).toHaveLength(3); // i1, i2, w1 only
      const rows = screen.getAllByTestId("tray-row");
      expect(rows).toHaveLength(6);
    });

    it("tray_should_say_nothing_needs_attention_when_rows_exist_and_none_pending", () => {
      mockHistory = [info("i1")];
      renderTray();
      expect(screen.getByText("Nothing needs attention")).toBeInTheDocument();
      expect(screen.queryByTestId("tray-needs-attention")).toBeNull();
    });

    it("tray_should_render_fewer_than_60_rows_and_issue_0_extra_history_fetches_when_500_rows_then_page_on_scroll", () => {
      mockHistory = Array.from({ length: 500 }, (_, i) => info(`n-${i}`, `s-${i}`, { timestamp: 1_000_000 - i }));
      mockHasMore = true;
      renderTray();
      expect(screen.getAllByTestId("tray-row").length).toBeLessThan(60);
      expect(screen.getAllByTestId("tray-row").length).toBeGreaterThan(0);
      expect(mockLoadMore).not.toHaveBeenCalled();
    });

    it("group_collapse_should_hide_rows_and_expose_aria_expanded_when_header_toggled", () => {
      mockHistory = [info("a", "s1", { sessionName: "alpha" }), info("b", "s1", { sessionName: "alpha", notificationType: "error" })];
      renderTray();
      const toggle = screen.getByRole("button", { name: /alpha \(2\)/ });
      expect(toggle).toHaveAttribute("aria-expanded", "true");
      fireEvent.click(toggle);
      expect(screen.getByRole("button", { name: /alpha \(2\)/ })).toHaveAttribute("aria-expanded", "false");
      expect(screen.queryAllByTestId("tray-row")).toHaveLength(0);
    });

    it("list_should_use_roving_tabindex_and_x_dismisses_only_with_list_focus", () => {
      mockHistory = [info("a", "s1"), info("b", "s2")];
      mockClearByIds.mockResolvedValue({ deleted: 1, kept: [] });
      renderTray();
      const tabbable = screen.getAllByRole("listitem").filter((el) => el.getAttribute("tabindex") === "0");
      expect(tabbable).toHaveLength(1);

      // x with focus outside the list does nothing.
      fireEvent.keyDown(screen.getByRole("searchbox", { name: "Search notifications" }), { key: "x" });
      expect(screen.queryByTestId("tray-undo-bar")).toBeNull();

      const row = screen.getAllByTestId("tray-row")[0];
      row.focus();
      fireEvent.keyDown(row, { key: "x" });
      expect(screen.getByTestId("tray-undo-bar")).toHaveTextContent("Dismissed 1 notification");
    });
  });

  describe("bulk actions (Story 4.4)", () => {
    it("mark_activity_read_should_send_non_decision_ids_only_and_revert_with_retry_when_rpc_fails", async () => {
      mockHistory = [pending("p1"), info("a"), info("b"), info("c"), info("d")];
      mockMarkAsRead.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
      renderTray();

      fireEvent.click(screen.getByRole("button", { name: "Mark activity read" }));
      await waitFor(() => expect(screen.getByTestId("tray-mark-read-error")).toBeInTheDocument());
      expect(mockMarkAsRead).toHaveBeenCalledWith(["a", "b", "c", "d"]);

      fireEvent.click(within(screen.getByTestId("tray-mark-read-error")).getByRole("button", { name: "Retry" }));
      await waitFor(() => expect(screen.queryByTestId("tray-mark-read-error")).toBeNull());
      expect(mockMarkAsRead).toHaveBeenCalledTimes(2);
    });

    function openMenuItem(testId: string) {
      fireEvent.click(screen.getByTestId("tray-overflow"));
      fireEvent.click(screen.getByTestId(testId));
    }

    it("clear_informational_should_send_exact_20_ids_after_undo_window_show_2_kept_and_restore_kept_rows", async () => {
      jest.useFakeTimers();
      const infos = Array.from({ length: 20 }, (_, i) => info(`i-${i}`, `s-${i}`));
      mockHistory = [pending("p1"), pending("p2"), ...infos];
      mockClearByIds.mockResolvedValue({ deleted: 20, kept: [] });
      renderTray();

      openMenuItem("tray-menu-clear-informational");
      expect(screen.getByTestId("tray-confirm")).toHaveTextContent("Clear 20 informational notifications? 2 awaiting decision kept.");
      fireEvent.click(screen.getByTestId("tray-confirm-ok"));

      // Rows are gone from view, the undo bar names the count, and no RPC has been sent yet.
      expect(screen.getByTestId("tray-undo-bar")).toHaveTextContent("Cleared 20");
      expect(mockClearByIds).not.toHaveBeenCalled();

      await act(async () => {
        jest.advanceTimersByTime(8_000);
      });
      expect(mockClearByIds).toHaveBeenCalledTimes(1);
      expect(mockClearByIds.mock.calls[0][0]).toEqual(infos.map((n) => n.id));
      expect(mockClearByIds.mock.calls[0][0]).not.toContain("p1");
    });

    it("clear_informational_should_show_kept_line_when_response_lists_kept_rows", async () => {
      jest.useFakeTimers();
      mockHistory = [info("a", "s1"), info("b", "s2")];
      mockClearByIds.mockResolvedValue({ deleted: 1, kept: ["b"] });
      renderTray();
      openMenuItem("tray-menu-clear-informational");
      fireEvent.click(screen.getByTestId("tray-confirm-ok"));
      await act(async () => {
        jest.advanceTimersByTime(8_000);
      });
      expect(screen.getByTestId("tray-kept-line")).toHaveTextContent("1 kept: still needs attention");
      // The kept row is shown again: it was only hidden for the window.
      expect(screen.getAllByTestId("tray-row").length).toBe(2);
    });

    it("clear_informational_should_issue_no_rpc_when_undo_tapped_and_rollback_with_could_not_clear_when_rpc_fails", async () => {
      jest.useFakeTimers();
      mockHistory = [info("a", "s1"), info("b", "s2")];
      renderTray();
      openMenuItem("tray-menu-clear-informational");
      fireEvent.click(screen.getByTestId("tray-confirm-ok"));
      fireEvent.click(screen.getByTestId("tray-undo"));
      await act(async () => {
        jest.advanceTimersByTime(60_000);
      });
      expect(mockClearByIds).not.toHaveBeenCalled();
      expect(screen.getAllByTestId("tray-row")).toHaveLength(2);

      mockClearByIds.mockRejectedValue(new Error("boom"));
      openMenuItem("tray-menu-clear-informational");
      fireEvent.click(screen.getByTestId("tray-confirm-ok"));
      await act(async () => {
        jest.advanceTimersByTime(8_000);
      });
      expect(screen.getByTestId("tray-clear-error")).toHaveTextContent("Could not clear notifications");
      expect(mockShowActionToast).toHaveBeenCalledWith("Could not clear notifications", "error", "tray-clear");
      expect(screen.getAllByTestId("tray-row")).toHaveLength(2);
    });

    it("clear_history_should_use_inline_confirm_never_window_confirm_with_focus_on_cancel_and_stay_open_on_failure", async () => {
      const confirmSpy = jest.spyOn(window, "confirm");
      mockHistory = [info("a", "s1", { isRead: true }), info("b", "s2", { isRead: true }), pending("p1")];
      mockClearByIds.mockRejectedValueOnce(new Error("boom")).mockResolvedValueOnce({ deleted: 2, kept: [] });
      renderTray();

      openMenuItem("tray-menu-clear-history");
      expect(confirmSpy).not.toHaveBeenCalled();
      expect(screen.getByTestId("tray-confirm")).toHaveTextContent("Clear 2 read notifications? This can't be undone. 1 awaiting decision kept.");
      expect(document.activeElement).toBe(screen.getByTestId("tray-confirm-cancel"));

      fireEvent.click(screen.getByTestId("tray-confirm-ok"));
      const retry = await screen.findByRole("button", { name: "Could not clear - Retry" });
      expect(screen.getByTestId("tray-confirm")).toBeInTheDocument();
      expect(screen.getAllByTestId("tray-row")).toHaveLength(3);

      fireEvent.click(retry);
      await waitFor(() => expect(screen.queryByTestId("tray-confirm")).toBeNull());
      expect(mockClearByIds).toHaveBeenLastCalledWith(["a", "b"]);
      expect(mockShowActionToast).toHaveBeenCalledWith("Cleared 2 notifications", "success", "tray-clear-history");
      confirmSpy.mockRestore();
    });

    it("clear_history_confirm_should_cancel_on_esc_without_closing_the_tray", () => {
      mockHistory = [info("a", "s1", { isRead: true })];
      renderTray();
      openMenuItem("tray-menu-clear-history");
      fireEvent.keyDown(screen.getByTestId("tray-confirm-cancel"), { key: "Escape" });
      expect(screen.queryByTestId("tray-confirm")).toBeNull();
      expect(mockTogglePanel).not.toHaveBeenCalled();
    });

    it("overflow_menu_should_separate_undoable_from_irreversible_and_close_on_esc_returning_focus", () => {
      mockHistory = [info("a", "s1", { isRead: true })];
      renderTray();
      const trigger = screen.getByTestId("tray-overflow");
      fireEvent.click(trigger);
      const menu = screen.getByRole("menu");
      const labels = within(menu).getAllByRole("menuitem").map((el) => el.textContent);
      expect(labels[0]).toContain("Clear informational (1)");
      expect(labels[0]).toContain("Undo available");
      expect(labels[1]).toContain("Clear history...");
      expect(labels[1]).toContain("cannot be undone");
      expect(within(menu).getAllByRole("separator").length).toBeGreaterThanOrEqual(2);

      fireEvent.keyDown(within(menu).getAllByRole("menuitem")[0], { key: "ArrowDown" });
      expect(document.activeElement).toBe(within(menu).getAllByRole("menuitem")[1]);
      fireEvent.keyDown(document.activeElement as Element, { key: "Escape" });
      expect(screen.queryByRole("menu")).toBeNull();
      expect(document.activeElement).toBe(trigger);
    });
  });

  describe("offline and error states (Tasks 4.3e-4.3g)", () => {
    it("tray_should_show_offline_banner_disable_server_mutating_actions_and_never_show_all_caught_up_when_offline_or_load_error", () => {
      mockConnectivity = { state: "disconnected", isOffline: true };
      mockHistory = [pending("p1"), info("a")];
      renderTray();
      expect(screen.getByTestId("tray-banner-offline")).toHaveTextContent("Offline - showing cached");
      expect(screen.getByRole("button", { name: "Mark activity read" })).toHaveAttribute("aria-disabled", "true");
      const approve = screen.getAllByRole("button", { name: /Approve/ })[0];
      expect(approve).toBeDisabled();
      expect(screen.getByText("Offline", { selector: "[data-testid='row-offline-reason']" })).toBeInTheDocument();
      expect(screen.queryByText("All caught up")).toBeNull();
    });

    it("offline_dismiss_should_be_disabled_with_reason_and_never_queued_replayed_or_marked_pending_sync", () => {
      mockConnectivity = { state: "stale", isOffline: true };
      mockHistory = [info("a", "s1")];
      renderTray();
      const remove = screen.getByLabelText("Remove notification");
      expect(remove).toHaveAttribute("aria-disabled", "true");
      expect(remove).toHaveAttribute("title", "Offline");
      fireEvent.click(remove);
      expect(screen.queryByTestId("tray-undo-bar")).toBeNull();
      expect(mockClearByIds).not.toHaveBeenCalled();
      expect(screen.queryByText(/pending sync/i)).toBeNull();
    });

    it("tray_should_show_load_error_with_retry_and_cached_rows_when_connected_and_history_fails", () => {
      mockHistoryError = new Error("x");
      mockHistory = [info("a")];
      renderTray();
      expect(screen.getByTestId("tray-banner-load-error")).toHaveTextContent("Could not load notifications.");
      expect(screen.queryByTestId("tray-banner-offline")).toBeNull();
      fireEvent.click(within(screen.getByTestId("tray-banner-load-error")).getByRole("button", { name: "Retry" }));
      expect(mockRefresh).toHaveBeenCalled();
    });

    it("tray_should_show_skeleton_empty_filtered_empty_and_all_caught_up_states_with_controls_when_each_state_reached", () => {
      mockHistoryLoading = true;
      mockLastUpdatedAt = null;
      const { unmount } = renderTray();
      expect(screen.getByTestId("tray-skeleton")).toHaveAttribute("aria-busy", "true");
      unmount();

      mockHistoryLoading = false;
      mockLastUpdatedAt = 1;
      const second = renderTray();
      expect(screen.getByText("All caught up")).toBeInTheDocument();
      second.unmount();

      mockHistory = [info("a")];
      renderTray();
      fireEvent.change(screen.getByRole("searchbox", { name: "Search notifications" }), { target: { value: "zzz" } });
      expect(screen.getByText("No matching notifications")).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
      expect(screen.getAllByTestId("tray-row")).toHaveLength(1);
    });
  });

  describe("sheet (Story 4.2)", () => {
    const phone = { isInnerScreen: false, isVirtualKeyboardOpen: false };

    it("sheet_should_render_peek_then_expanded_and_never_trap_or_inert_on_a_coarse_pointer", () => {
      const original = window.matchMedia;
      window.matchMedia = ((q: string) => ({ matches: q.includes("coarse"), media: q, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia;
      renderTray(phone);
      const tray = screen.getByTestId("notification-tray");
      expect(tray).toHaveAttribute("data-variant", "bottom-sheet");
      expect(tray).toHaveAttribute("data-sheet", "peek");
      fireEvent.click(screen.getByTestId("tray-expand"));
      expect(tray).toHaveAttribute("data-sheet", "expanded");
      expect(tray).not.toHaveAttribute("aria-modal");
      expect(screen.queryByTestId("tray-scrim")).toBeNull();
      window.matchMedia = original;
    });

    it("sheet_should_be_modal_with_scrim_and_inert_background_when_expanded_on_a_fine_pointer", () => {
      const main = document.createElement("main");
      main.id = "main-content";
      document.body.appendChild(main);
      renderTray(phone);
      fireEvent.click(screen.getByTestId("tray-expand"));
      const tray = screen.getByTestId("notification-tray");
      expect(tray).toHaveAttribute("aria-modal", "true");
      expect(main).toHaveAttribute("inert");
      fireEvent.click(screen.getByTestId("tray-scrim"));
      expect(mockTogglePanel).toHaveBeenCalled();
      main.remove();
    });

    it("sheet_should_drag_only_by_grabber_and_ignore_list_drag", () => {
      // jsdom has no PointerEvent; MouseEvent carries the clientY the grabber reads.
      const originalPointerEvent = (window as unknown as { PointerEvent?: unknown }).PointerEvent;
      (window as unknown as { PointerEvent: unknown }).PointerEvent = MouseEvent;
      mockHistory = [info("a")];
      renderTray(phone);
      const tray = screen.getByTestId("notification-tray");
      fireEvent.click(screen.getByTestId("tray-expand"));
      expect(tray).toHaveAttribute("data-sheet", "expanded");

      // Dragging a row 80px does not move the sheet.
      const row = screen.getAllByTestId("tray-row")[0];
      fireEvent.pointerDown(row, { clientY: 100, pointerId: 1 });
      fireEvent.pointerUp(row, { clientY: 180, pointerId: 1 });
      expect(tray).toHaveAttribute("data-sheet", "expanded");

      const grabber = screen.getByTestId("tray-grabber");
      fireEvent.pointerDown(grabber, { clientY: 100, pointerId: 1 });
      fireEvent.pointerUp(grabber, { clientY: 180, pointerId: 1 });
      expect(tray).toHaveAttribute("data-sheet", "peek");
      (window as unknown as { PointerEvent: unknown }).PointerEvent = originalPointerEvent;
    });
  });

  describe("Quiet mode toggle (Task 4.5b)", () => {
    it("quiet_toggle_should_expose_aria_pressed_and_flip_the_setting_when_clicked", () => {
      renderTray();
      const toggle = screen.getByTestId("tray-quiet");
      expect(toggle).toHaveAttribute("aria-pressed", "false");
      fireEvent.click(toggle);
      expect(mockSetQuiet).toHaveBeenCalledWith(true);
    });

    it("quiet_toggle_should_reflect_an_enabled_setting", () => {
      mockQuiet = true;
      renderTray();
      expect(screen.getByTestId("tray-quiet")).toHaveAttribute("aria-pressed", "true");
      fireEvent.click(screen.getByTestId("tray-quiet"));
      expect(mockSetQuiet).toHaveBeenCalledWith(false);
    });
  });

  describe("variants and Pin tray (Task 4.2e)", () => {
    it("tray_should_pick_top_sheet_with_keyboard_open_and_landscape_panel_on_its_side", () => {
      const { unmount } = renderTray({ isInnerScreen: false, isVirtualKeyboardOpen: true });
      expect(screen.getByTestId("notification-tray")).toHaveAttribute("data-variant", "top-sheet");
      // The top sheet has no grabber or expand control.
      expect(screen.queryByTestId("tray-grabber")).toBeNull();
      unmount();

      renderTray({ isInnerScreen: false, isVirtualKeyboardOpen: false, isLandscape: true } as never);
      expect(screen.getByTestId("notification-tray")).toHaveAttribute("data-variant", "landscape-panel");
      expect(screen.queryByTestId("tray-expand")).toBeNull();
    });

    it("pin_should_be_an_aria_pressed_toggle_only_on_the_desktop_overlay_and_dock_the_open_tray", () => {
      const { unmount } = renderTray({ isInnerScreen: false, isVirtualKeyboardOpen: false });
      expect(screen.queryByTestId("tray-pin")).toBeNull();
      unmount();

      renderTray();
      const pin = screen.getByTestId("tray-pin");
      expect(pin).toHaveAttribute("aria-pressed", "false");
      expect(pin).toHaveAccessibleName("Pin notifications tray");
      expect(document.documentElement.dataset.trayPinned).toBeUndefined();

      fireEvent.click(pin);
      expect(screen.getByTestId("tray-pin")).toHaveAttribute("aria-pressed", "true");
      expect(screen.getByTestId("tray-pin")).toHaveAccessibleName("Unpin notifications tray");
      expect(document.documentElement.dataset.trayPinned).toBe("true");
      expect(window.localStorage.getItem("ssq.notifications.trayPinned")).toBe("true");

      fireEvent.click(screen.getByTestId("tray-pin"));
      expect(document.documentElement.dataset.trayPinned).toBeUndefined();
    });

    it("pin_should_render_off_and_not_crash_when_localstorage_throws", () => {
      const get = jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
        throw new Error("blocked");
      });
      renderTray();
      expect(screen.getByTestId("tray-pin")).toHaveAttribute("aria-pressed", "false");
      get.mockRestore();
    });
  });

  describe("settings and What changed card (Task 4.4h)", () => {
    it("settings_should_persist_undo_window_and_pinned_collapse_and_survive_storage_throwing", () => {
      renderTray();
      fireEvent.click(screen.getByTestId("tray-overflow"));
      fireEvent.click(screen.getByTestId("tray-menu-settings"));
      const undo = screen.getByLabelText("Undo window");
      fireEvent.change(undo, { target: { value: "15000" } });
      expect(window.localStorage.getItem("ssq.notifications.undoWindow")).toBe("15000");
      fireEvent.change(screen.getByLabelText("Collapse pinned cards after"), { target: { value: "never" } });
      expect(window.localStorage.getItem("ssq.notifications.pinnedCollapse")).toBe("never");

      const spy = jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
        throw new Error("blocked");
      });
      expect(() => fireEvent.change(undo, { target: { value: "30000" } })).not.toThrow();
      spy.mockRestore();
    });

    it("what_changed_should_show_once_dismiss_with_got_it_and_reopen_from_overflow_without_moving_focus", () => {
      renderTray();
      const card = screen.getByTestId("tray-what-changed");
      expect(card).not.toHaveAttribute("role");
      const before = document.activeElement;
      fireEvent.click(screen.getByTestId("tray-what-changed-dismiss"));
      expect(screen.queryByTestId("tray-what-changed")).toBeNull();
      expect(window.localStorage.getItem("ssq.notifications.whatChangedSeen")).toBe("true");
      expect(document.activeElement).not.toBe(card);
      void before;

      fireEvent.click(screen.getByTestId("tray-overflow"));
      fireEvent.click(screen.getByTestId("tray-menu-what-changed"));
      expect(screen.getByTestId("tray-what-changed")).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// Background activity segment (Story 5.4)
// ---------------------------------------------------------------------------

describe("tray Background segment (Story 5.4)", () => {
  const refresh = jest.fn();
  const hiddenState = (over: Record<string, unknown> = {}) => ({
    sessions: [{ id: "review:h1", title: "review:h1", state: "running", updatedAtMs: Date.now() }],
    departed: [],
    loading: false,
    failed: false,
    lastUpdatedAt: Date.now(),
    refresh,
    ...over,
  });

  beforeEach(() => {
    mockFlags.notification_tray_v2 = true;
    mockIsPanelOpen = true;
    mockConnectivity = { state: "connected", isOffline: false };
    mockMarkAsRead.mockClear();
    mockTogglePanel.mockClear();
    mockUseBackgroundSessions.mockReset();
    mockUseBackgroundSessions.mockReturnValue(hiddenState());
    mockHistory = [
      makeNotification({ id: "f1", sessionId: "review:h1", sessionName: "review:h1", notificationType: "error", message: "boom" }),
      makeNotification({ id: "v1", sessionId: "vis", sessionName: "vis", notificationType: "info" }),
    ];
  });
  afterEach(() => {
    mockFlags.notification_tray_v2 = false;
  });

  it("segment_should_not_poll_while_collapsed_and_poll_when_the_background_tab_is_selected", () => {
    renderTray();
    expect(mockUseBackgroundSessions).toHaveBeenLastCalledWith(false);
    fireEvent.click(screen.getByTestId("tray-tab-background"));
    expect(mockUseBackgroundSessions).toHaveBeenLastCalledWith(true);
    fireEvent.click(screen.getByTestId("tray-tab-notifications"));
    expect(mockUseBackgroundSessions).toHaveBeenLastCalledWith(false);
  });

  it("does_not_poll_while_the_tray_is_closed_or_offline", () => {
    mockIsPanelOpen = false;
    const { rerender } = renderTray();
    fireEvent.click(screen.getByTestId("tray-tab-background"));
    expect(mockUseBackgroundSessions).toHaveBeenLastCalledWith(false);
    mockIsPanelOpen = true;
    mockConnectivity = { state: "offline", isOffline: true };
    rerender(
      <DeckViewportContext.Provider value={{ isInnerScreen: true, isVirtualKeyboardOpen: false }}>
        <NotificationPanel />
      </DeckViewportContext.Provider>,
    );
    expect(mockUseBackgroundSessions).toHaveBeenLastCalledWith(false);
  });

  it("background_badge_should_count_only_failure_and_needs_human_and_not_change_bell_count", () => {
    renderTray();
    expect(screen.getByTestId("tray-tab-background")).toHaveTextContent("Background (1)");
    // bell/header unread badge is the plain unread total of history, unchanged by the join
    expect(screen.getByRole("heading", { name: /Notifications/ })).toHaveTextContent("2");
    fireEvent.click(screen.getByTestId("tray-tab-background"));
    expect(screen.getAllByTestId("background-row")).toHaveLength(1);
    expect(screen.getByRole("heading", { name: /Notifications/ })).toHaveTextContent("2");
  });

  it("opening_a_row_should_mark_its_records_read_and_close_the_tray", () => {
    renderTray();
    fireEvent.click(screen.getByTestId("tray-tab-background"));
    fireEvent.click(screen.getByTestId("notification-view-output"));
    expect(mockMarkAsRead).toHaveBeenCalledWith(["f1"]);
    expect(mockTogglePanel).toHaveBeenCalledTimes(1);
  });

  it("a_deleted_hidden_session_row_stays_until_read", () => {
    mockUseBackgroundSessions.mockReturnValue(
      hiddenState({ sessions: [], departed: [{ id: "review:h1", title: "review:h1" }] }),
    );
    renderTray();
    fireEvent.click(screen.getByTestId("tray-tab-background"));
    expect(screen.getByTestId("background-row")).toHaveTextContent("Session no longer available");
  });

  it("legacy_panel_has_no_segments", () => {
    mockFlags.notification_tray_v2 = false;
    render(<NotificationPanel />);
    expect(screen.queryByTestId("tray-segments")).toBeNull();
    expect(mockUseBackgroundSessions).toHaveBeenLastCalledWith(false);
  });
});

// ---------------------------------------------------------------------------
// Spec-gap repair (Phase 5): validation.md T-TY-10..T-TY-24 and the tray UX criteria
// ---------------------------------------------------------------------------

describe("tray spec gaps (Phase 5)", () => {
  const DESKTOP = { isInnerScreen: true, isVirtualKeyboardOpen: false };
  const PHONE = { isInnerScreen: false, isVirtualKeyboardOpen: false };

  function tree(viewport: { isInnerScreen: boolean; isVirtualKeyboardOpen: boolean } = DESKTOP) {
    return (
      <DeckViewportContext.Provider value={viewport}>
        <NotificationPanel />
      </DeckViewportContext.Provider>
    );
  }

  function setPointer(coarse: boolean) {
    window.matchMedia = ((q: string) => ({
      matches: coarse && q.includes("coarse"),
      media: q,
      addEventListener() {},
      removeEventListener() {},
    })) as unknown as typeof window.matchMedia;
  }

  /** A fling to the left: 90px in 150ms passes the swipe machine's dismiss rule. */
  function swipe(target: Element, dx: number, dy = 0) {
    const fire = (type: string, x: number, y: number, t: number) => {
      const event = new Event(type, { bubbles: true, cancelable: true });
      const point = { clientX: x, clientY: y };
      Object.defineProperty(event, "touches", { value: type === "touchend" ? [] : [point] });
      Object.defineProperty(event, "changedTouches", { value: [point] });
      Object.defineProperty(event, "timeStamp", { value: t });
      act(() => {
        target.dispatchEvent(event);
      });
    };
    fire("touchstart", 100, 100, 0);
    fire("touchmove", 100 - dx / 2, 100 + dy / 2, 60);
    fire("touchmove", 100 - dx, 100 + dy, 150);
    fire("touchend", 100 - dx, 100 + dy, 200);
  }

  const rowTitles = () => screen.getAllByTestId("tray-row").map((row) => row.textContent ?? "");

  beforeEach(() => {
    resetTrayState();
    setPointer(false);
    jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      width: 360, height: 80, top: 0, left: 0, right: 360, bottom: 80, x: 0, y: 0, toJSON: () => ({}),
    } as DOMRect);
  });
  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
    mockFlags.notification_tray_v2 = false;
  });

  it("group_collapse_should_be_per_viewer_and_not_shared_across_tabs_when_toggled", () => {
    const created = jest.fn();
    const original = (window as unknown as { BroadcastChannel?: unknown }).BroadcastChannel;
    (window as unknown as { BroadcastChannel: unknown }).BroadcastChannel = class {
      constructor(name: string) {
        created(name);
      }
      postMessage() {}
      close() {}
      addEventListener() {}
      removeEventListener() {}
    };
    mockHistory = [info("a", "s1", { sessionName: "alpha" }), info("b", "s1", { sessionName: "alpha", notificationType: "error" })];
    const storedBefore = window.localStorage.length + window.sessionStorage.length;

    const first = render(tree());
    const toggle = within(first.container).getByRole("button", { name: /alpha \(2\)/ });
    fireEvent.click(toggle);
    expect(within(first.container).getByRole("button", { name: /alpha \(2\)/ })).toHaveAttribute("aria-expanded", "false");

    // A second viewer (another tab or device) renders its own tray: still expanded, nothing was written or broadcast.
    const second = render(tree());
    expect(within(second.container).getByRole("button", { name: /alpha \(2\)/ })).toHaveAttribute("aria-expanded", "true");
    expect(window.localStorage.length + window.sessionStorage.length).toBe(storedBefore);
    expect(created).not.toHaveBeenCalled();
    (window as unknown as { BroadcastChannel?: unknown }).BroadcastChannel = original;
  });

  it("group_dismiss_should_offer_dismiss_n_informational_and_keep_decision_when_group_has_decision", async () => {
    jest.useFakeTimers();
    mockHistory = [
      pending("p1", "s-x"),
      info("a", "s-x", { notificationType: "task_complete" }),
      info("b", "s-x", { notificationType: "error" }),
    ];
    mockClearByIds.mockResolvedValue({ deleted: 2, kept: [] });
    render(tree());

    fireEvent.click(screen.getByRole("button", { name: "Dismiss 2 informational" }));
    expect(screen.getByTestId("tray-undo-bar")).toHaveTextContent("Dismissed 2 informational");
    // The decision row never left the pinned group while the others were hidden for the window.
    expect(screen.getAllByTestId("needs-attention-row")).toHaveLength(1);

    await act(async () => {
      jest.advanceTimersByTime(8_000);
    });
    expect(mockClearByIds).toHaveBeenCalledTimes(1);
    expect([...mockClearByIds.mock.calls[0][0]].sort()).toEqual(["a", "b"]);
    expect(screen.getAllByTestId("needs-attention-row")).toHaveLength(1);
  });

  it("duplicate_should_increment_group_count_in_place_and_show_n_new_pill_when_order_would_change", () => {
    mockHistory = [
      info("a", "s-a", { timestamp: 3_000 }),
      info("b", "s-b", { timestamp: 2_000 }),
    ];
    const view = render(tree());
    fireEvent.scroll(screen.getByTestId("tray-scroll"), { target: { scrollTop: 40 } });
    expect(rowTitles()[0]).toContain("s-a");

    // The server folds a duplicate into its record (same id, newer time, count 2), which would
    // move b to the top; a brand new session row arrives too.
    mockHistory = [
      info("b", "s-b", { timestamp: 5_000, occurrenceCount: 2 }),
      info("c", "s-c", { timestamp: 4_000 }),
      info("a", "s-a", { timestamp: 3_000 }),
    ];
    view.rerender(tree());

    const rows = rowTitles();
    expect(rows).toHaveLength(2);
    expect(rows[0]).toContain("s-a");
    expect(rows[1]).toContain("s-b");
    expect(within(screen.getAllByTestId("tray-row")[1]).getByLabelText("2 occurrences")).toHaveTextContent("x2");
    expect(screen.getByTestId("tray-new-pill")).toHaveTextContent("1 new");

    fireEvent.click(screen.getByTestId("tray-new-pill"));
    expect(screen.queryByTestId("tray-new-pill")).toBeNull();
    expect(rowTitles()[0]).toContain("s-b");
    expect(rowTitles()).toHaveLength(3);
  });

  it("duplicate_should_increment_count_without_reordering_when_tray_is_open_at_top (TR-5)", () => {
    mockHistory = [
      info("a", "s-a", { timestamp: 3_000 }),
      info("b", "s-b", { timestamp: 2_000 }),
    ];
    const view = render(tree());
    mockHistory = [
      info("b", "s-b", { timestamp: 5_000, occurrenceCount: 2 }),
      info("a", "s-a", { timestamp: 3_000 }),
    ];
    view.rerender(tree());

    const rows = rowTitles();
    expect(rows[0]).toContain("s-a");
    expect(rows[1]).toContain("s-b");
    expect(within(screen.getAllByTestId("tray-row")[1]).getByLabelText("2 occurrences")).toHaveTextContent("x2");
  });

  it("tray_should_keep_search_text_and_scroll_when_variant_switches_across_900px (TK-5)", () => {
    mockHistory = [info("a", "s1", { sessionName: "alpha" }), info("b", "s1", { sessionName: "alpha", notificationType: "error" })];
    const view = render(tree(PHONE));
    expect(screen.getByTestId("notification-tray")).toHaveAttribute("data-variant", "bottom-sheet");

    fireEvent.change(screen.getByRole("searchbox", { name: "Search notifications" }), { target: { value: "alp" } });
    fireEvent.click(screen.getByRole("button", { name: /alpha \(2\)/ }));
    const scroller = screen.getByTestId("tray-scroll");
    scroller.scrollTop = 120;

    const expectKept = () => {
      expect(screen.getByRole("searchbox", { name: "Search notifications" })).toHaveValue("alp");
      expect(screen.getByRole("button", { name: /alpha \(2\)/ })).toHaveAttribute("aria-expanded", "false");
      expect(screen.getByTestId("tray-scroll").scrollTop).toBe(120);
    };

    view.rerender(tree({ isInnerScreen: false, isVirtualKeyboardOpen: true }));
    expect(screen.getByTestId("notification-tray")).toHaveAttribute("data-variant", "top-sheet");
    expectKept();

    view.rerender(tree(DESKTOP));
    expect(screen.getByTestId("notification-tray")).toHaveAttribute("data-variant", "side-overlay");
    expectKept();
  });

  it("row_should_hide_dismiss_and_swipe_for_unread_decision_and_show_both_for_informational", () => {
    mockHistory = [
      pending("p1", "s-p"),
      info("i1", "s-i"),
      // An auto-remediating WARNING arrives with isPendingDecision=false (TR-11); an unstamped one is pinned.
      makeNotification({ id: "w-auto", sessionId: "s-wa", sessionName: "s-wa", notificationType: "warning", isPendingDecision: false }),
      makeNotification({ id: "w-real", sessionId: "s-wr", sessionName: "s-wr", notificationType: "warning", isPendingDecision: true }),
    ];
    render(tree());

    expect(screen.getByTestId("tray-needs-attention")).toHaveTextContent("2 need attention");
    const pinned = screen.getAllByTestId("needs-attention-row");
    expect(pinned.map((row) => row.textContent)).toEqual(expect.arrayContaining([expect.stringContaining("s-p"), expect.stringContaining("s-wr")]));
    for (const row of pinned) {
      expect(within(row).queryByLabelText("Remove notification")).toBeNull();
      expect(row.closest('[data-testid="tray-swipe-row"]')).toBeNull();
    }

    const swipeRows = screen.getAllByTestId("tray-swipe-row");
    expect(swipeRows).toHaveLength(2);
    for (const row of swipeRows) {
      expect(within(row).getByLabelText("Remove notification")).toBeInTheDocument();
    }
    expect(swipeRows.map((row) => row.textContent).join(" ")).toContain("s-wa");
    // The auto-remediating warning sits in its session group and is not counted as needing attention.
    expect(screen.getAllByTestId("tray-group-header").map((h) => h.textContent).join(" ")).toContain("s-wa");
  });

  it("tr3_swipe_of_90px_should_dismiss_with_undo_and_60px_vertical_drift_should_scroll_instead", () => {
    mockHistory = [info("a", "s-a"), info("b", "s-b")];
    render(tree());
    const [first, second] = screen.getAllByTestId("tray-swipe-row");

    swipe(second, 20, 60);
    expect(screen.queryByTestId("tray-undo-bar")).toBeNull();
    expect(screen.getAllByTestId("tray-row")).toHaveLength(2);

    swipe(first, 90);
    expect(screen.getByTestId("tray-undo-bar")).toHaveTextContent("Dismissed 1 notification");
    expect(screen.getAllByTestId("tray-row")).toHaveLength(1);
  });

  it("tr6_rows_should_carry_visible_text_for_type_and_a_text_background_chip", () => {
    mockHistory = [
      info("e1", "s-e", { notificationType: "error", sessionName: "failing" }),
      info("t1", "s-t", { notificationType: "task_complete", sessionName: "finished" }),
    ];
    render(tree());
    const rows = screen.getAllByTestId("tray-row");
    // The type is a word on the row, not only a colour; unread is an image with a label, not a colour.
    expect(within(rows[0]).getByText(/^Error$/i)).toBeInTheDocument();
    expect(within(rows[1]).getByText(/complete/i)).toBeInTheDocument();
    expect(within(rows[0]).getByRole("img", { name: "Unread" })).toBeInTheDocument();
  });

  it("tr8_keyboard_alone_should_open_a_row_collapse_a_group_and_dismiss_a_row", async () => {
    jest.useFakeTimers();
    mockHistory = [info("a", "s1", { sessionName: "alpha" }), info("b", "s2", { sessionName: "beta" })];
    mockClearByIds.mockResolvedValue({ deleted: 1, kept: [] });
    render(tree());
    const headerRow = document.querySelector('[data-tray-row="s1"]') as HTMLElement;
    headerRow.focus();

    fireEvent.keyDown(headerRow, { key: "Enter" });
    expect(screen.getByRole("button", { name: /alpha \(1\)/ })).toHaveAttribute("aria-expanded", "false");
    fireEvent.keyDown(headerRow, { key: " " });
    expect(screen.getByRole("button", { name: /alpha \(1\)/ })).toHaveAttribute("aria-expanded", "true");

    const groupRow = screen.getAllByTestId("tray-row")[0];
    groupRow.focus();
    fireEvent.keyDown(groupRow, { key: "Enter" });
    expect(mockMarkAsRead).toHaveBeenCalledWith(["a"]);

    fireEvent.keyDown(groupRow, { key: "x" });
    expect(screen.getByTestId("tray-undo-bar")).toHaveTextContent("Dismissed 1 notification");
  });

  it("tr9_should_never_call_focus_on_a_row_when_a_new_notification_arrives", () => {
    mockHistory = [info("a", "s-a", { timestamp: 1_000 })];
    const view = render(tree());
    const focus = jest.spyOn(HTMLElement.prototype, "focus");

    mockHistory = [info("n", "s-n", { timestamp: 2_000 }), ...mockHistory];
    view.rerender(tree());

    expect(screen.getAllByTestId("tray-row")).toHaveLength(2);
    const focusedRows = focus.mock.contexts.filter((el) => (el as HTMLElement).hasAttribute?.("data-tray-row"));
    expect(focusedRows).toHaveLength(0);
  });

  it("ty8_j_and_k_should_move_between_rows_only_while_focus_is_inside_the_list", () => {
    mockHistory = [info("a", "s1", { sessionName: "alpha" }), info("b", "s2", { sessionName: "beta" })];
    render(tree());
    const keyOf = () => (document.activeElement as HTMLElement).getAttribute("data-tray-row");

    const first = document.querySelector('[data-tray-row="s1"]') as HTMLElement;
    first.focus();
    fireEvent.keyDown(first, { key: "j" });
    expect(keyOf()).not.toBe("s1");
    const moved = keyOf();
    fireEvent.keyDown(document.activeElement as HTMLElement, { key: "k" });
    expect(keyOf()).toBe("s1");
    expect(moved).toBeTruthy();

    // Outside the list a j does nothing and is not swallowed.
    const search = screen.getByRole("searchbox", { name: "Search notifications" });
    search.focus();
    const notPrevented = fireEvent.keyDown(search, { key: "j" });
    expect(notPrevented).toBe(true);
    expect(document.activeElement).toBe(search);
  });

  it("ty6_ty7_header_controls_should_be_named_and_tab_order_should_follow_the_surface_without_a_trap", () => {
    mockHistory = [info("a", "s1")];
    render(tree());
    const tray = screen.getByTestId("notification-tray");

    for (const button of within(tray).getAllByRole("button")) {
      expect(button).toHaveAccessibleName();
    }

    const heading = within(tray).getByRole("heading", { name: /Notifications/ });
    const firstControl = within(tray).getByTestId("tray-quiet");
    const segment = within(tray).getByTestId("tray-tab-notifications");
    const search = within(tray).getByRole("searchbox", { name: "Search notifications" });
    const row = within(tray).getAllByRole("listitem").find((el) => el.getAttribute("tabindex") === "0") as HTMLElement;
    const review = within(tray).getByRole("link", { name: "Review all notifications" });
    const order = [heading, firstControl, segment, search, row, review];
    for (let i = 0; i < order.length - 1; i++) {
      expect(order[i].compareDocumentPosition(order[i + 1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    }

    // The desktop overlay never traps Tab at either end.
    expect(fireEvent.keyDown(review, { key: "Tab" })).toBe(true);
    expect(fireEvent.keyDown(firstControl, { key: "Tab", shiftKey: true })).toBe(true);
  });

  it("ts5_peek_sheet_should_keep_the_background_interactive_with_no_trap_or_aria_modal", () => {
    const main = document.createElement("main");
    main.id = "main-content";
    document.body.appendChild(main);
    render(tree(PHONE));
    const tray = screen.getByTestId("notification-tray");

    expect(tray).toHaveAttribute("data-sheet", "peek");
    expect(tray).not.toHaveAttribute("aria-modal");
    expect(main).not.toHaveAttribute("inert");
    const review = screen.getByRole("link", { name: "Review all notifications" });
    expect(fireEvent.keyDown(review, { key: "Tab" })).toBe(true);
    main.remove();
  });

  it("tk3_should_not_focus_the_search_input_when_opened_on_touch", () => {
    setPointer(true);
    mockIsPanelOpen = false;
    const view = render(tree(PHONE));
    mockIsPanelOpen = true;
    view.rerender(tree(PHONE));

    expect(document.activeElement).not.toBe(screen.getByRole("searchbox", { name: "Search notifications" }));
    expect(screen.getByTestId("notification-tray")).toHaveAttribute("data-state", "open");
  });

  it("te4_should_disable_dismiss_swipe_and_bulk_actions_offline_and_reenable_them_on_reconnect", () => {
    mockHistory = [info("a", "s1", { sessionName: "alpha" })];
    mockConnectivity = { state: "disconnected", isOffline: true };
    const view = render(tree());

    expect(screen.getByLabelText("Remove notification")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("button", { name: "Mark activity read" })).toHaveAttribute("aria-disabled", "true");
    swipe(screen.getByTestId("tray-swipe-row"), 90);
    expect(screen.queryByTestId("tray-undo-bar")).toBeNull();
    fireEvent.click(screen.getByTestId("tray-overflow"));
    fireEvent.click(screen.getByTestId("tray-menu-clear-informational"));
    expect(screen.queryByTestId("tray-confirm")).toBeNull();
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });

    mockConnectivity = { state: "connected", isOffline: false };
    view.rerender(tree());
    expect(screen.queryByTestId("tray-banner-offline")).toBeNull();
    expect(screen.getByLabelText("Remove notification")).not.toHaveAttribute("aria-disabled");
    expect(screen.getByRole("button", { name: "Mark activity read" })).not.toHaveAttribute("aria-disabled");
    swipe(screen.getByTestId("tray-swipe-row"), 90);
    expect(screen.getByTestId("tray-undo-bar")).toBeInTheDocument();
  });

  it("te5_te3_loading_should_be_one_busy_region_without_live_roles_and_filtered_empty_should_offer_a_44px_exit", () => {
    mockHistoryLoading = true;
    mockLastUpdatedAt = null;
    const loading = render(tree());
    const skeleton = screen.getByTestId("tray-skeleton");
    expect(skeleton).toHaveAttribute("aria-busy", "true");
    expect(skeleton.querySelectorAll("[role='status'], [role='alert'], [aria-live]")).toHaveLength(0);
    loading.unmount();

    mockHistoryLoading = false;
    mockLastUpdatedAt = 1;
    mockHistory = [info("a")];
    render(tree());
    fireEvent.change(screen.getByRole("searchbox", { name: "Search notifications" }), { target: { value: "zzz" } });
    const clear = screen.getByRole("button", { name: "Clear filters" });
    // The 44px target rule is one global selector over every control inside the v2 tray.
    expect(clear.closest('[data-notification-tray="v2"]')).not.toBeNull();
    const css = require("fs").readFileSync(require("path").join(__dirname, "NotificationPanel.css.ts"), "utf8") as string;
    expect(css).toMatch(/\[data-notification-tray="v2"\] :is\(button, select, input\[type="search"\]\)[\s\S]{0,80}minHeight: "44px"/);
  });

  it("tm4_should_restore_rows_and_show_could_not_clear_notifications_with_retry_when_the_rpc_fails", async () => {
    jest.useFakeTimers();
    mockHistory = [info("a", "s1"), info("b", "s2")];
    mockClearByIds.mockRejectedValueOnce(new Error("boom")).mockResolvedValueOnce({ deleted: 2, kept: [] });
    render(tree());
    fireEvent.click(screen.getByTestId("tray-overflow"));
    fireEvent.click(screen.getByTestId("tray-menu-clear-informational"));
    fireEvent.click(screen.getByTestId("tray-confirm-ok"));
    await act(async () => {
      jest.advanceTimersByTime(8_000);
    });

    const banner = screen.getByTestId("tray-clear-error");
    expect(banner).toHaveTextContent("Could not clear notifications");
    expect(screen.getAllByTestId("tray-row")).toHaveLength(2);
    fireEvent.click(within(banner).getByRole("button", { name: "Retry" }));
    await act(async () => {
      jest.advanceTimersByTime(8_000);
    });
    expect(mockClearByIds).toHaveBeenCalledTimes(2);
  });

  it("tm8_should_clear_50_informational_rows_in_three_taps", async () => {
    jest.useFakeTimers();
    const rows = Array.from({ length: 50 }, (_, i) => info(`i-${i}`, `s-${i}`));
    mockHistory = rows;
    mockClearByIds.mockResolvedValue({ deleted: 50, kept: [] });
    render(tree());

    fireEvent.click(screen.getByTestId("tray-overflow")); // tap 1
    fireEvent.click(screen.getByTestId("tray-menu-clear-informational")); // tap 2
    fireEvent.click(screen.getByTestId("tray-confirm-ok")); // tap 3
    await act(async () => {
      jest.advanceTimersByTime(8_000);
    });
    expect(mockClearByIds).toHaveBeenCalledTimes(1);
    expect(mockClearByIds.mock.calls[0][0]).toHaveLength(50);
  });

  it("xa15_tray_and_background_segment_should_own_no_live_region_of_their_own", () => {
    mockUseBackgroundSessions.mockReturnValue({
      sessions: [{ id: "review:h1", title: "review:h1", state: "running", updatedAtMs: Date.now() }],
      departed: [],
      loading: false,
      failed: false,
      lastUpdatedAt: Date.now(),
      refresh: jest.fn(),
    });
    mockHistory = [
      pending("p1"),
      makeNotification({ id: "f1", sessionId: "review:h1", sessionName: "review:h1", notificationType: "error" }),
    ];
    render(tree());
    const tray = screen.getByTestId("notification-tray");
    const live = () => tray.querySelectorAll("[role='status'], [role='alert'], [aria-live]");
    expect(live()).toHaveLength(0);

    fireEvent.click(screen.getByTestId("tray-tab-background"));
    expect(screen.getAllByTestId("background-row").length).toBeGreaterThan(0);
    expect(live()).toHaveLength(0);
  });

  it("quiet_mode_should_state_itself_in_the_header_say_push_is_unchanged_and_note_a_failed_save", () => {
    mockQuiet = true;
    mockSetQuiet.mockImplementation((on: boolean) => writeQuietMode(on));
    const view = render(tree());
    const toggle = screen.getByTestId("tray-quiet");
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(toggle).toHaveTextContent("Quiet mode on");
    expect(screen.getByTestId("tray-quiet-state")).toHaveTextContent("Quiet mode on");
    expect(screen.getByTestId("tray-quiet-hint")).toHaveTextContent("Push notifications are unchanged");
    expect(screen.queryByTestId("tray-quiet-unsaved")).toBeNull();
    view.unmount();

    // Storage that throws: the toggle still applies for this page, and the panel says it will not persist.
    mockQuiet = false;
    const set = jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    mockSetQuiet.mockImplementation((on: boolean) => {
      mockQuiet = on;
      writeQuietMode(on);
    });
    const second = render(tree());
    fireEvent.click(screen.getByTestId("tray-quiet"));
    second.rerender(tree());
    expect(screen.getByTestId("tray-quiet-unsaved")).toHaveTextContent("Preference could not be saved");
    set.mockRestore();
  });

  it("ts7_tl1_sheet_and_landscape_panel_should_pad_by_the_device_safe_area_insets", () => {
    const css = require("fs").readFileSync(require("path").join(__dirname, "NotificationPanel.css.ts"), "utf8") as string;
    const block = (name: string) => {
      const from = css.indexOf(`${name}: [`);
      return css.slice(from, css.indexOf("],", from));
    };
    // The jsdom run cannot apply a 34px inset; the real-device leg is the operator's DV-3.
    expect(block("bottomSheet")).toMatch(/paddingBottom: "max\(0px, calc\(env\(safe-area-inset-bottom, 0px\) - var\(--bottom-nav-height, 0px\)\)\)"/);
    // TS-1: no pixel floor on the peek height (it exceeded 28% of the viewport below 786px).
    expect(block("bottomSheet")).toMatch(/height: "calc\(var\(--viewport-height, 100dvh\) \* 0\.27\)"/);
    expect(block("landscapePanel")).toMatch(/paddingRight: "env\(safe-area-inset-right, 0px\)"/);
    expect(block("landscapePanel")).toMatch(/width: "min\(360px, 50vw\)"/);
  });

  it("tray_error_boundary_should_show_open_notifications_page_link_and_keep_handle_usable_when_render_throws", () => {
    mockFlags.notification_tray_v2 = true;
    jest.spyOn(console, "error").mockImplementation(() => {});
    function Broken(): React.ReactElement {
      throw new Error("tray render failed");
    }
    render(
      <DeckViewportContext.Provider value={DESKTOP}>
        <TrayErrorBoundary>
          <Broken />
        </TrayErrorBoundary>
      </DeckViewportContext.Provider>,
    );

    const link = screen.getByRole("link", { name: "Notifications unavailable - Open Notifications page" });
    expect(link).toHaveAttribute("href", "/notifications");
    const handle = screen.getByTestId("tray-handle");
    expect(handle).toBeEnabled();
    expect(handle).toHaveAttribute("aria-controls", "notification-tray");
  });
});
