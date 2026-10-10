import React from "react";
import fs from "fs";
import path from "path";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { NotificationProvider, useNotifications } from "@/lib/contexts/NotificationContext";
import { DeckViewportContext } from "@/lib/contexts/deckViewportContext";
import { markSessionViewed } from "@/lib/utils/viewedSessions";
import { setStackTopOffset } from "@/lib/utils/toastDock";
import { registerTerminalCursorSource } from "@/lib/terminal/cursorRect";
import { useSessionNotifications } from "@/lib/hooks/useSessionNotifications";
import { NotificationType, NotificationPriority } from "@/gen/session/v1/types_pb";
import type { NotificationData } from "@/lib/types/notification";

const mockViewport = { isInnerScreen: true, isVirtualKeyboardOpen: false };

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

/** Re-reads the mutable viewport on every render, like the bridge does from ViewportProvider. */
function Harness({ children }: { children: React.ReactNode }) {
  return (
    <DeckViewportContext.Provider value={{ ...mockViewport }}>
      <NotificationProvider>{children}</NotificationProvider>
    </DeckViewportContext.Provider>
  );
}

function mount() {
  return render(
    <Harness>
      <Driver />
    </Harness>,
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
  Object.assign(mockViewport, { isInnerScreen: true, isVirtualKeyboardOpen: false });
}
function setPhone(keyboardOpen = false) {
  Object.assign(mockViewport, { isInnerScreen: false, isVirtualKeyboardOpen: keyboardOpen });
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
      <Harness>
        <Driver />
      </Harness>,
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
      <Harness>
        <Driver />
        <Feed />
      </Harness>,
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
      <Harness>
        <Driver />
      </Harness>,
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
    expect(within(card).getByRole("button", { name: "Move to tray" })).not.toHaveAttribute("aria-disabled");
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

describe("Mobile and keyboard positioning (Story 3.7)", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => {
    setStackTopOffset(null);
    jest.useRealTimers();
  });

  it("docks at the top under the session tab row on a phone session page, and at the bottom on a page with no terminal", () => {
    setPhone();
    mount();
    addMany(3);
    expect(screen.getByTestId("toast-stack")).toHaveAttribute("data-placement", "mobileBottom");

    act(() => setStackTopOffset(120));
    expect(screen.getByTestId("toast-stack")).toHaveAttribute("data-placement", "mobileTop");
    expect(document.documentElement.style.getPropertyValue("--mobile-stack-top-offset")).toBe("120px");
  });

  it("shows one line chip '3 notifications - 1 needs you' with the keyboard open and no card", () => {
    setPhone(true);
    mount();
    act(() => {
      notifications.addNotification(toast(0, { notificationType: "error", isPendingDecision: true }));
      notifications.addNotification(toast(1));
      notifications.addNotification(toast(2));
    });
    expect(screen.queryAllByTestId("toast")).toHaveLength(0);
    const chip = screen.getByTestId("toast-overflow-chip");
    expect(chip).toHaveTextContent("3 notifications - 1 needs you");
  });

  it("keeps terminal focus when a deck control is pressed", () => {
    setPhone(true);
    mount();
    addMany(2);
    const chip = screen.getByTestId("toast-overflow-chip");
    const notPrevented = fireEvent.mouseDown(chip);
    expect(notPrevented).toBe(false); // preventDefault was called, so the textarea is not blurred
  });

  it("moves the desktop deck to the top-right when it would cover the cursor cell, and back when it moves", () => {
    mount();
    let cursor: { left: number; top: number; width: number; height: number } | null = {
      left: window.innerWidth - 100,
      top: window.innerHeight - 40,
      width: 8,
      height: 16,
    };
    const unregister = registerTerminalCursorSource(() => cursor);
    addMany(2);
    act(() => {
      jest.advanceTimersByTime(300);
    });
    expect(screen.getByTestId("toast-stack")).toHaveAttribute("data-placement", "desktopTopRight");

    cursor = { left: 20, top: 20, width: 8, height: 16 };
    act(() => {
      jest.advanceTimersByTime(300);
    });
    expect(screen.getByTestId("toast-stack")).toHaveAttribute("data-placement", "desktop");
    unregister();
  });

  it("stays bottom-right on a page with no terminal", () => {
    mount();
    addMany(2);
    act(() => {
      jest.advanceTimersByTime(600);
    });
    expect(screen.getByTestId("toast-stack")).toHaveAttribute("data-placement", "desktop");
  });
});

describe("Timers pause and pinned collapse (Story 3.5)", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    window.localStorage.clear();
  });
  afterEach(() => jest.useRealTimers());

  const advance = (ms: number) =>
    act(() => {
      jest.advanceTimersByTime(ms);
    });

  it("toast_timers_should_preserve_remaining_time_on_hover_pause_and_pause_when_tray_open_and_never_expire_pinned_10min", () => {
    mount();
    act(() => notifications.addNotification(toast(0, { notificationType: "info" })));
    const card = screen.getByTestId("toast");

    advance(3_000);
    fireEvent.pointerEnter(card);
    advance(7_000); // pointer leaves at t=10s; the 8s close would long since have fired
    expect(notifications.notifications).toHaveLength(1);

    fireEvent.pointerLeave(card);
    advance(4_999);
    expect(notifications.notifications).toHaveLength(1);
    advance(1 + 300); // remaining 5s, then the 300ms exit animation
    expect(notifications.notifications).toHaveLength(0);
  });

  it("holds the timer while focus is within the card", () => {
    mount();
    act(() => notifications.addNotification(toast(0, { notificationType: "info" })));
    const close = within(screen.getByTestId("toast")).getByRole("button", { name: "Dismiss notification" });

    fireEvent.focus(close);
    advance(60_000);
    expect(notifications.notifications).toHaveLength(1);
    fireEvent.blur(close);
    advance(8_000 + 300);
    expect(notifications.notifications).toHaveLength(0);
  });

  it("pauses every timer while the tray is open and resumes them when it closes", () => {
    mount();
    act(() => notifications.addNotification(toast(0, { notificationType: "info" })));
    act(() => notifications.togglePanel());
    advance(60_000);
    expect(notifications.notifications).toHaveLength(1);

    act(() => notifications.togglePanel());
    advance(8_000 + 300);
    expect(notifications.notifications).toHaveLength(0);
  });

  it("never expires a pinned decision in the deck", () => {
    mount();
    act(() =>
      notifications.addNotification(
        toast(0, { notificationType: "approval_needed", isPendingDecision: true, onApprove: jest.fn(), onDeny: jest.fn() }),
      ),
    );
    advance(10 * 60 * 1000);
    expect(notifications.notifications).toHaveLength(1);
  });

  describe("pinned card collapse on a phone (TD-14)", () => {
    const pinnedError = () =>
      act(() => notifications.addNotification(toast(0, { notificationType: "error", isPendingDecision: true })));

    it("collapses to a one-line chip after the default 8s and stays pinned in the tray", () => {
      setPhone();
      mount();
      pinnedError();
      advance(7_999);
      expect(screen.queryByTestId("toast-collapsed-chip")).toBeNull();
      advance(1);
      expect(screen.getByTestId("toast-collapsed-chip")).toHaveTextContent("1 needs you - Title 0");
      expect(notifications.notifications).toHaveLength(1);
      expect(notifications.notificationHistory[0].isPendingDecision).toBe(true);
    });

    it("expands on tap and is not re-collapsed", () => {
      setPhone();
      mount();
      pinnedError();
      advance(8_000);
      fireEvent.click(screen.getByTestId("toast-collapsed-chip"));
      expect(screen.queryByTestId("toast-collapsed-chip")).toBeNull();
      advance(60_000);
      expect(screen.queryByTestId("toast-collapsed-chip")).toBeNull();
    });

    it("honours the per-device delay and Never, and does not collapse on desktop", () => {
      window.localStorage.setItem("ssq.notifications.pinnedCollapse", "15000");
      setPhone();
      const first = mount();
      pinnedError();
      advance(14_999);
      expect(screen.queryByTestId("toast-collapsed-chip")).toBeNull();
      advance(1);
      expect(screen.getByTestId("toast-collapsed-chip")).toBeInTheDocument();
      first.unmount();

      window.localStorage.setItem("ssq.notifications.pinnedCollapse", "never");
      mount();
      pinnedError();
      advance(5 * 60 * 1000);
      expect(screen.queryByTestId("toast-collapsed-chip")).toBeNull();

      window.localStorage.clear();
      setDesktop();
      mount();
      pinnedError();
      advance(60_000);
      expect(screen.queryByTestId("toast-collapsed-chip")).toBeNull();
    });

    it("holds the collapse timer while the card is hovered", () => {
      setPhone();
      mount();
      pinnedError();
      const card = screen.getByTestId("toast");
      fireEvent.pointerEnter(card);
      advance(60_000);
      expect(screen.queryByTestId("toast-collapsed-chip")).toBeNull();
      fireEvent.pointerLeave(card);
      advance(8_000);
      expect(screen.getByTestId("toast-collapsed-chip")).toBeInTheDocument();
    });
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

describe("single phone tray entry (Task 4.2i, TH-7..TH-9)", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    setPhone();
    act(() => setStackTopOffset(120));
  });
  afterEach(() => {
    setStackTopOffset(null);
    jest.useRealTimers();
  });

  const entries = () => screen.queryAllByTestId("tray-entry");

  it("shows exactly one entry as a bell with 0 toasts, then with 1 toast and 0 overflow (never zero)", () => {
    mount();
    expect(entries()).toHaveLength(1);
    expect(entries()[0]).toHaveAttribute("data-content", "bell");

    addMany(1);
    expect(entries()).toHaveLength(1);
    expect(entries()[0]).toHaveAttribute("data-content", "bell");
    expect(screen.getByTestId("tray-entry-open")).toHaveAccessibleName(/^Notifications, \d+ unread/);
  });

  it("keeps one mounted node while its content changes bell -> more-row -> keyboard-chip -> bell", () => {
    const { rerender } = mount();
    const node = entries()[0];
    addMany(3);
    expect(entries()[0]).toBe(node);
    expect(node).toHaveAttribute("data-content", "more-row");
    expect(screen.getByTestId("toast-overflow-chip")).toHaveAccessibleName("2 more notifications, open tray");

    setPhone(true);
    rerender(
      <Harness>
        <Driver />
      </Harness>,
    );
    expect(entries()).toHaveLength(1);
    expect(entries()[0]).toBe(node);
    expect(node).toHaveAttribute("data-content", "keyboard-chip");

    act(() => notifications.moveAllToTray());
    act(() => notifications.undoMoveToTray());
    act(() => notifications.clearAll());
    expect(entries()[0]).toBe(node);
  });

  it("opens the same tray from every content", () => {
    mount();
    fireEvent.click(screen.getByTestId("tray-entry-open"));
    expect(notifications.isPanelOpen).toBe(true);
  });

  it("renders the undo inside the same entry with a trailing bell", () => {
    mount();
    const node = entries()[0];
    addMany(3);
    fireEvent.click(screen.getByTestId("toast-move-all-to-tray"));
    expect(entries()).toHaveLength(1);
    expect(entries()[0]).toBe(node);
    expect(node).toHaveAttribute("data-undo", "true");
    expect(within(node).getByTestId("toast-undo-bar")).toBeInTheDocument();
    expect(within(node).getByTestId("tray-entry-open")).toBeInTheDocument();
  });

  it("shows no entry on a phone page with no terminal and nothing to say", () => {
    act(() => setStackTopOffset(null));
    mount();
    expect(entries()).toHaveLength(0);
  });
});

describe("deck rules from the UX criteria (TD-5, TD-9, TB-2, T-TS-32)", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  const advance = (ms: number) =>
    act(() => {
      jest.advanceTimersByTime(ms);
    });

  // T-AR-03: the auto-remediating warning (isPendingDecision=false) does not pin and auto-closes.
  it("td5_should_keep_a_pending_toast_for_10_minutes_and_remove_an_auto_remediating_warning_at_its_timeout", () => {
    mount();
    act(() => notifications.addNotification(toast(0, { notificationType: "warning", isPendingDecision: true })));
    // The producer stamped auto_remediating, so the server sent isPendingDecision=false.
    act(() => notifications.addNotification(toast(1, { notificationType: "warning", isPendingDecision: false })));
    expect(screen.getAllByTestId("toast")).toHaveLength(2);

    advance(8_000 + 300);
    expect(notifications.notifications.map((n) => n.sessionId)).toEqual(["s0"]);

    advance(10 * 60 * 1000);
    expect(notifications.notifications.map((n) => n.sessionId)).toEqual(["s0"]);
    expect(screen.getAllByTestId("toast")).toHaveLength(1);
  });

  it("td9_should_leave_document_active_element_alone_when_a_toast_arrives", () => {
    mount();
    const field = document.createElement("textarea");
    document.body.appendChild(field);
    field.focus();
    expect(document.activeElement).toBe(field);

    act(() => notifications.addNotification(toast(0, { notificationType: "error", isPendingDecision: true })));
    act(() => notifications.addNotification(toast(1)));
    advance(500);
    expect(screen.getAllByTestId("toast").length).toBeGreaterThan(0);
    expect(document.activeElement).toBe(field);
    field.remove();
  });

  it("tb2_should_label_the_bulk_control_move_all_to_tray_n_for_every_deck_content_and_never_offer_dismiss_all", () => {
    const { unmount } = mount();
    act(() => {
      notifications.addNotification(toast(0));
      notifications.addNotification(toast(1));
    });
    expect(screen.getByTestId("toast-move-all-to-tray")).toHaveTextContent("Move all to tray (2)");
    expect(screen.queryByText(/dismiss all/i)).toBeNull();
    unmount();

    mount();
    addMany(2);
    expect(screen.getByTestId("toast-move-all-to-tray")).toHaveTextContent("Move all to tray (2)");
    expect(screen.queryByText(/dismiss all/i)).toBeNull();
  });

  it("toast_action_should_show_inline_could_not_verb_retry_and_pin_toast_when_rpc_fails", async () => {
    mount();
    const onApprove = jest.fn().mockRejectedValue(new Error("boom"));
    act(() => {
      notifications.addNotification(
        toast(1, {
          notificationType: "error",
          isPendingDecision: false,
          onApprove,
          onDeny: jest.fn(),
          metadata: { risk_level: "low" },
        }),
      );
      notifications.addNotification(toast(2, { notificationType: "error", isPendingDecision: false }));
    });
    const failing = screen.getByText("Title 1").closest('[data-testid="toast"]') as HTMLElement;

    await act(async () => {
      fireEvent.click(within(failing).getByRole("button", { name: /Approve/ }));
    });
    expect(onApprove).toHaveBeenCalledTimes(1);
    expect(within(failing).getByTestId("toast-action-error")).toHaveTextContent("Could not approve - Retry");

    // The untouched twin times out; the toast whose action failed stays until it is resolved.
    advance(60_000);
    expect(notifications.notifications.map((n) => n.sessionId)).toEqual(["s1"]);
    expect(screen.getByTestId("toast-action-error")).toBeInTheDocument();
  });

  it("td12_should_shift_the_desktop_deck_clear_of_the_open_tray_and_step_a_phone_deck_aside", () => {
    mount();
    addMany(2);
    expect(screen.getByTestId("toast-stack").className).not.toMatch(/deckBesideTray|deckBehindTray/);

    act(() => notifications.togglePanel());
    // Desktop: still visible, with its right edge moved left of the min(400px, 40vw) tray plus 16px.
    expect(screen.getByTestId("toast-stack").className).toMatch(/deckBesideTray/);
    expect(screen.getByTestId("toast-stack").className).not.toMatch(/deckBehindTray/);
    const css = fs.readFileSync(path.join(process.cwd(), "src/components/ui/NotificationToast.css.ts"), "utf8");
    expect(css).toMatch(/deckBesideTray = style\(\{\s*right: "calc\(min\(400px, 40vw\) \+ 16px\)"/);
  });

  it("td12_phone_deck_should_stay_mounted_but_hidden_while_the_tray_is_open", () => {
    setPhone();
    mount();
    addMany(2);
    act(() => notifications.togglePanel());
    expect(screen.getByTestId("toast-stack").className).toMatch(/deckBehindTray/);
  });

  it("pinned_collapse_should_pause_on_focus_and_touch_hold_and_not_run_on_desktop (T-PC-02)", () => {
    setPhone();
    mount();
    act(() => notifications.addNotification(toast(0, { notificationType: "error", isPendingDecision: true })));
    const card = screen.getByTestId("toast");
    fireEvent.focus(within(card).getAllByRole("button")[0]);
    advance(60_000);
    expect(screen.queryByTestId("toast-collapsed-chip")).toBeNull();
    fireEvent.blur(within(card).getAllByRole("button")[0]);
    advance(8_000);
    expect(screen.getByTestId("toast-collapsed-chip")).toBeInTheDocument();
  });
});
