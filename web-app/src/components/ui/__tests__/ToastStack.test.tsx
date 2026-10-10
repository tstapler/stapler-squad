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
