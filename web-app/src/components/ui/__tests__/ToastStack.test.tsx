import React from "react";
import { act, render, screen } from "@testing-library/react";
import { NotificationProvider, useNotifications } from "@/lib/contexts/NotificationContext";
import type { NotificationData } from "@/lib/types/notification";

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

describe("ToastStack announcements (Story 3.6)", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("announcer_should_own_single_polite_and_assertive_region_and_coalesce_burst_once_with_no_surface_live_role", () => {
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
