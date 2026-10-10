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
import { DeckViewportContext } from "@/lib/contexts/deckViewportContext";
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
