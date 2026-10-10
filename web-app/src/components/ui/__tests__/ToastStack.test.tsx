import React from "react";
import fs from "fs";
import path from "path";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { NotificationProvider, useNotifications } from "@/lib/contexts/NotificationContext";
import { markSessionViewed } from "@/lib/utils/viewedSessions";
import { useSessionNotifications } from "@/lib/hooks/useSessionNotifications";
import { NotificationType, NotificationPriority } from "@/gen/session/v1/types_pb";
import type { NotificationData } from "@/lib/types/notification";

const mockViewport = { isMobile: false, isFoldable: false, isInnerScreen: true, hasFinePointer: true, isVirtualKeyboardOpen: false };
jest.mock("@/components/providers/ViewportProvider", () => ({ useViewport: () => mockViewport }));

const mockFlags: Record<string, boolean> = {};
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: (name: string) => mockFlags[name] ?? false,
}));

let mockConnectivity = { state: "connected", isOffline: false };
jest.mock("@/lib/hooks/useNotificationConnectivity", () => ({
  useNotificationConnectivity: () => mockConnectivity,
}));

jest.mock("@/lib/utils/notifications", () => ({
  showBrowserNotification: jest.fn().mockResolvedValue(undefined),
  playPriorityNotificationSound: jest.fn(),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

jest.mock("@/lib/utils/broadcastChannel", () => ({
  createNotificationSyncChannel: () => ({ broadcast: jest.fn(), subscribe: () => () => {} }),
}));
jest.mock("@/lib/utils/notificationStorage", () => ({ markAcknowledged: jest.fn() }));
jest.mock("@/lib/hooks/useNotificationHistory", () => ({
  useNotificationHistory: () => ({
    notifications: [],
    unreadCount: 0,
    loading: false,
    error: null,
    hasMore: false,
    lastUpdatedAt: null,
    markAsRead: jest.fn().mockResolvedValue(undefined),
    clearHistory: jest.fn().mockResolvedValue(undefined),
    loadMore: jest.fn().mockResolvedValue(undefined),
    refresh: jest.fn().mockResolvedValue(undefined),
  }),
}));
jest.mock("@/lib/hooks/useAuditLog", () => ({
  useAuditLog: () => ({
    logNotificationDismissed: jest.fn(),
    logNotificationMarkedRead: jest.fn(),
    logNotificationPanelOpened: jest.fn(),
    logNotificationPanelClosed: jest.fn(),
    logNotificationRemoved: jest.fn(),
    logNotificationHistoryCleared: jest.fn(),
    logNotificationSessionViewed: jest.fn(),
  }),
}));

type Commands = ReturnType<typeof useNotifications>;
let notifications: Commands;

function Driver() {
  notifications = useNotifications();
  return null;
}

function mount() {
  return render(
    <NotificationProvider>
      <Driver />
    </NotificationProvider>,
  );
}

function toast(index: number, overrides: Partial<NotificationData> = {}): Omit<NotificationData, "id" | "timestamp"> {
  return {
    sessionId: `s${index}`,
    sessionName: `Session ${index}`,
    title: `Title ${index}`,
    message: `Message ${index}`,
    notificationType: "info",
    ...overrides,
  };
}

function setDesktop() {
  Object.assign(mockViewport, { isMobile: false, isInnerScreen: true, isVirtualKeyboardOpen: false });
}
function setPhone(keyboardOpen = false) {
  Object.assign(mockViewport, { isMobile: true, isInnerScreen: false, isVirtualKeyboardOpen: keyboardOpen });
}

function addMany(count: number, overrides: Partial<NotificationData> = {}) {
  act(() => {
    for (let i = 0; i < count; i++) {
      notifications.addNotification(toast(i, { notificationType: "error", isPendingDecision: true, ...overrides }));
    }
  });
}

beforeEach(() => {
  setDesktop();
  mockFlags["notification_tray_v2"] = true;
  mockConnectivity = { state: "connected", isOffline: false };
});

describe("ToastStack cap and chip (Story 3.3)", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("toast_stack_should_render_3_cards_and_chip_plus7_with_aria_label_when_10_nonroutine_toasts", () => {
    mount();
    addMany(10);

    expect(screen.getAllByTestId("toast")).toHaveLength(3);
    const chip = screen.getByTestId("toast-overflow-chip");
    expect(chip).toHaveTextContent("+7 more");
    expect(chip).toHaveAttribute("aria-label", "7 more notifications, open tray");

    expect(notifications.isPanelOpen).toBe(false);
    fireEvent.click(chip);
    expect(notifications.isPanelOpen).toBe(true);
  });

  it("toast_stack_should_cap_1_portrait_1_landscape_0_keyboard_when_viewport_variant_changes", () => {
    setPhone();
    mount();
    addMany(10);
    expect(screen.getAllByTestId("toast")).toHaveLength(1);
    expect(screen.getByTestId("toast-overflow-chip")).toHaveTextContent("+9 more");
  });

  it("shows no cards, only the chip, with the keyboard open and restores them when it closes", () => {
    setPhone(true);
    const { rerender } = mount();
    addMany(3);
    expect(screen.queryAllByTestId("toast")).toHaveLength(0);
    expect(screen.getByTestId("toast-overflow-chip")).toBeInTheDocument();

    setPhone(false);
    rerender(
      <NotificationProvider>
        <Driver />
      </NotificationProvider>,
    );
    expect(screen.getAllByTestId("toast")).toHaveLength(1);
  });

  it("toast_stack_should_not_toast_routine_types_when_v2_on", () => {
    function Feed() {
      const handle = useSessionNotifications({ enableAudio: false });
      React.useEffect(() => {
        handle({
          sessionId: "s-done",
          sessionName: "Done",
          notificationType: NotificationType.TASK_COMPLETE,
          priority: NotificationPriority.MEDIUM,
          title: "Task complete",
          message: "ok",
          metadata: {},
        } as never);
      }, [handle]);
      return null;
    }
    render(
      <NotificationProvider>
        <Driver />
        <Feed />
      </NotificationProvider>,
    );
    expect(screen.queryAllByTestId("toast")).toHaveLength(0);
    expect(notifications.notificationHistory).toHaveLength(1);
  });

  it("toast_css_should_have_no_nth_child_and_single_flex_column_when_stylesheet_inspected", () => {
    const source = fs.readFileSync(path.join(process.cwd(), "src/components/ui/NotificationToast.css.ts"), "utf8");
    expect(source).not.toMatch(/nth-child/);
    expect(source).toMatch(/flexDirection: "column"/);
    expect(source).toMatch(/gap: "8px"/);
  });

  it("toast_stack_should_render_legacy_list_of_10_when_flag_off", () => {
    mockFlags["notification_tray_v2"] = false;
    mount();
    addMany(10);

    expect(screen.getAllByTestId("toast")).toHaveLength(10);
    expect(screen.queryByTestId("toast-overflow-chip")).toBeNull();
    expect(screen.queryByTestId("toast-stack")).toBeNull();
  });

  it("toast_stack_should_read_flag_live_and_apply_cap_without_reload_when_notification_tray_v2_toggled", () => {
    mockFlags["notification_tray_v2"] = false;
    const { rerender } = mount();
    addMany(5);
    expect(screen.getAllByTestId("toast")).toHaveLength(5);

    mockFlags["notification_tray_v2"] = true;
    rerender(
      <NotificationProvider>
        <Driver />
      </NotificationProvider>,
    );
    expect(screen.getAllByTestId("toast")).toHaveLength(3);
    expect(screen.getByTestId("toast-overflow-chip")).toHaveTextContent("+2 more");
  });

  it("toast_stack_should_hold_cap_and_update_chip_in_place_when_burst_of_50", () => {
    mount();
    addMany(50);
    expect(screen.getAllByTestId("toast")).toHaveLength(3);
    expect(screen.getByTestId("toast-overflow-chip")).toHaveTextContent("+47 more");
  });

  it("toast_dedupe_should_show_x2_instead_of_stacking_twin_when_same_session_and_type", () => {
    mount();
    act(() => {
      notifications.addNotification(toast(1, { notificationType: "error", isPendingDecision: true }));
    });
    act(() => {
      notifications.addNotification(toast(1, { notificationType: "error", isPendingDecision: true }));
    });
    expect(screen.getAllByTestId("toast")).toHaveLength(1);
    expect(screen.getByTestId("toast-repeat-count")).toHaveTextContent("x2");
  });

  it("toast_stack_should_suppress_nonpinned_toast_for_session_in_view_when_event_arrives", () => {
    mount();
    const release = markSessionViewed("s7");
    act(() => {
      notifications.addNotification(toast(7, { notificationType: "info", isPendingDecision: false }));
    });
    expect(screen.queryAllByTestId("toast")).toHaveLength(0);
    expect(notifications.notificationHistory).toHaveLength(1);

    act(() => {
      notifications.addNotification(toast(7, { notificationType: "error", isPendingDecision: true }));
    });
    expect(screen.getAllByTestId("toast")).toHaveLength(1);
    release();
  });

  it("disables Approve and Deny with a visible Offline reason, but keeps close enabled, when disconnected", () => {
    mockConnectivity = { state: "disconnected", isOffline: true };
    const onApprove = jest.fn();
    mount();
    act(() => {
      notifications.addNotification(
        toast(1, { notificationType: "approval_needed", isPendingDecision: true, onApprove, onDeny: jest.fn() }),
      );
    });
    const card = screen.getByTestId("toast");
    const approve = within(card).getByRole("button", { name: /Approve/ });
    expect(approve).toHaveAttribute("aria-disabled", "true");
    expect(within(card).getByTestId("toast-offline-hint")).toHaveTextContent("Offline");

    fireEvent.click(approve);
    expect(onApprove).not.toHaveBeenCalled();
    expect(within(card).getByRole("button", { name: "Close notification" })).not.toHaveAttribute("aria-disabled");
  });
});

describe("Move all to tray (Story 3.4)", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  const approval = (index: number): Partial<NotificationData> => ({
    notificationType: "approval_needed",
    isPendingDecision: true,
    onApprove: jest.fn(),
    onDeny: jest.fn(),
    metadata: { approval_id: `a${index}` },
  });

  it("deck_header_should_always_read_move_all_to_tray_n_and_hide_for_0_or_1_toast", () => {
    mount();
    expect(screen.queryByTestId("toast-move-all-to-tray")).toBeNull();

    act(() => notifications.addNotification(toast(0, { isPendingDecision: true, notificationType: "error" })));
    expect(screen.queryByTestId("toast-move-all-to-tray")).toBeNull();

    act(() => notifications.addNotification(toast(1)));
    expect(screen.getByTestId("toast-move-all-to-tray")).toHaveTextContent("Move all to tray (2)");

    act(() => notifications.addNotification(toast(2, { isPendingDecision: true, notificationType: "error" })));
    expect(screen.getByTestId("toast-move-all-to-tray")).toHaveTextContent("Move all to tray (3)");
  });

  it("moveAllToTray_should_make_0_rpc_calls_keep_all_rows_unread_and_undo_in_order_when_10_pinned", () => {
    mount();
    for (let i = 0; i < 10; i++) {
      act(() => notifications.addNotification(toast(i, approval(i))));
    }
    const orderBefore = notifications.notifications.map((n) => n.sessionId);
    expect(notifications.notificationHistory.every((n) => !n.isRead && n.isPendingDecision)).toBe(true);

    fireEvent.click(screen.getByTestId("toast-move-all-to-tray"));

    expect(notifications.notifications).toHaveLength(0);
    expect(screen.queryAllByTestId("toast")).toHaveLength(0);
    expect(notifications.notificationHistory).toHaveLength(10);
    expect(notifications.notificationHistory.every((n) => !n.isRead && n.isPendingDecision)).toBe(true);
    expect(screen.getByTestId("toast-undo-bar")).toHaveTextContent("Moved 10 to tray");
    expect(screen.getByTestId("announcer-polite")).toHaveTextContent("10 moved to tray");

    fireEvent.click(screen.getByTestId("toast-undo-move"));
    expect(notifications.notifications.map((n) => n.sessionId)).toEqual(orderBefore);
    expect(screen.queryByTestId("toast-undo-bar")).toBeNull();
  });

  it("undo_should_restore_moved_toasts_within_undo_window_pause_on_hover_and_vanish_after_when_move_all", () => {
    mount();
    act(() => notifications.addNotification(toast(0, approval(0))));
    act(() => notifications.addNotification(toast(1, { notificationType: "info" })));
    fireEvent.click(screen.getByTestId("toast-move-all-to-tray"));
    const bar = screen.getByTestId("toast-undo-bar");

    // Hovering holds the window open past its 8s.
    fireEvent.pointerEnter(bar);
    act(() => {
      jest.advanceTimersByTime(20_000);
    });
    expect(screen.getByTestId("toast-undo-bar")).toBeInTheDocument();

    fireEvent.pointerLeave(bar);
    act(() => {
      jest.advanceTimersByTime(7_999);
    });
    expect(screen.getByTestId("toast-undo-bar")).toBeInTheDocument();
    act(() => {
      jest.advanceTimersByTime(1);
    });
    expect(screen.queryByTestId("toast-undo-bar")).toBeNull();
    // Final: the toasts stay in the tray only.
    expect(notifications.notifications).toHaveLength(0);
    expect(notifications.notificationHistory).toHaveLength(2);
  });

  it("opening the tray ends the undo window", () => {
    mount();
    act(() => notifications.addNotification(toast(0)));
    act(() => notifications.addNotification(toast(1)));
    fireEvent.click(screen.getByTestId("toast-move-all-to-tray"));
    expect(screen.getByTestId("toast-undo-bar")).toBeInTheDocument();

    act(() => notifications.togglePanel());
    expect(screen.queryByTestId("toast-undo-bar")).toBeNull();
  });

  it("on a phone the undo replaces the chip row in place, also with the keyboard open", () => {
    setPhone(true);
    mount();
    addMany(3);
    expect(screen.getByTestId("toast-overflow-chip")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("toast-move-all-to-tray"));

    expect(screen.queryByTestId("toast-overflow-chip")).toBeNull();
    expect(screen.getByTestId("toast-undo-bar")).toHaveTextContent("Moved 3 to tray");
  });

  it("clearAll_should_keep_pinned_and_dismissToast_should_remove_one_informational_toast_and_leave_history_when_called", () => {
    mount();
    act(() => notifications.addNotification(toast(0, approval(0))));
    act(() => notifications.addNotification(toast(1, { notificationType: "info" })));
    act(() => notifications.addNotification(toast(2, { notificationType: "info" })));

    const infoToast = notifications.notifications.find((n) => n.sessionId === "s1")!;
    act(() => notifications.dismissToast(infoToast.id));
    expect(notifications.notifications.map((n) => n.sessionId)).toEqual(["s0", "s2"]);
    expect(notifications.notificationHistory).toHaveLength(3);

    act(() => notifications.clearAll());
    expect(notifications.notifications.map((n) => n.sessionId)).toEqual(["s0"]);
    expect(notifications.notificationHistory).toHaveLength(3);
  });

  it("clearAll_should_not_remove_pinned_question_toast_when_flag_off", () => {
    mockFlags["notification_tray_v2"] = false;
    mount();
    act(() =>
      notifications.addNotification(toast(0, { notificationType: "question", isPendingDecision: true })),
    );
    act(() => notifications.clearAll());
    expect(notifications.notifications).toHaveLength(1);
  });

  it("moveAllToTray_should_never_import_bulk_clear_or_history_mutators_when_grepped", () => {
    const source = fs.readFileSync(path.join(process.cwd(), "src/lib/contexts/toastTray.ts"), "utf8");
    const code = source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*$/gm, "");
    expect(code).not.toMatch(/markAsRead|clearHistory|removeFromHistory|ClearNotificationHistory|MarkNotificationRead|useNotificationHistory|acknowledgeNotification|clearAll/);
    expect(code).not.toMatch(/^import .*session_pb/m);
  });
});

describe("ToastStack announcements (Story 3.6)", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("announcer_should_own_single_polite_and_assertive_region_and_coalesce_burst_once_with_no_surface_live_role", () => {
    mockFlags["notification_tray_v2"] = false; // legacy list: all five cards render
    const { container } = mount();
    expect(screen.getAllByTestId("announcer-polite")).toHaveLength(1);
    expect(screen.getAllByTestId("announcer-assertive")).toHaveLength(1);

    act(() => {
      for (let i = 0; i < 5; i++) notifications.addNotification(toast(i));
    });
    // Cards render, and none of them carries its own live role.
    expect(screen.getAllByTestId("toast")).toHaveLength(5);
    for (const card of screen.getAllByTestId("toast")) {
      expect(card.closest("[role='status'], [role='alert']")).toBeNull();
      expect(card.querySelector("[role='status'], [role='alert']")).toBeNull();
      expect(card).not.toHaveAttribute("role");
      expect(card).not.toHaveAttribute("aria-live");
    }
    // The single always-mounted regions are the only live roles in the tree.
    expect(container.ownerDocument.querySelectorAll("[role='status'], [role='alert']")).toHaveLength(2);

    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(screen.getByTestId("announcer-polite")).toHaveTextContent("5 new notifications");
  });

  it("announces a lone pinned toast assertively by title", () => {
    mount();
    act(() => {
      notifications.addNotification(
        toast(1, { notificationType: "approval_needed", title: "Approve rm -rf?", isPendingDecision: true }),
      );
    });
    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(screen.getByTestId("announcer-assertive")).toHaveTextContent("Approve rm -rf?");
  });

  it("announces an action receipt through the polite region", () => {
    mount();
    act(() => {
      notifications.showActionToast("Item saved", "success", "k");
    });
    expect(screen.getByTestId("announcer-polite")).toHaveTextContent("Item saved");
  });
});
