import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { TrayHandle, trayEntryLabel } from "../TrayHandle";
import { DeckViewportContext } from "@/lib/contexts/deckViewportContext";
import type { NotificationHistoryItem } from "@/lib/types/notification";

const mockFlags: Record<string, boolean> = {};
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: (name: string) => mockFlags[name] ?? false,
}));

let mockHistory: NotificationHistoryItem[] = [];
let mockOpen = false;
const mockToggle = jest.fn();
jest.mock("@/lib/contexts/notificationContexts", () => ({
  useNotificationState: () => ({
    notificationHistory: mockHistory,
    unreadCount: mockHistory.filter((n) => !n.isRead).length,
    isPanelOpen: mockOpen,
  }),
  useNotificationCommands: () => ({ togglePanel: mockToggle }),
}));

const row = (i: number, overrides: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem => ({
  id: `n${i}`,
  sessionId: `s${i}`,
  sessionName: `s${i}`,
  message: "m",
  timestamp: i,
  isRead: false,
  notificationType: "info",
  ...overrides,
});

function renderHandle(isInnerScreen = true) {
  return render(
    <DeckViewportContext.Provider value={{ isInnerScreen, isVirtualKeyboardOpen: false }}>
      <TrayHandle />
    </DeckViewportContext.Provider>,
  );
}

describe("TrayHandle", () => {
  beforeEach(() => {
    mockFlags.notification_tray_v2 = true;
    mockHistory = [];
    mockOpen = false;
    mockToggle.mockReset();
  });

  it("handle_should_cap_99plus_and_expose_aria_expanded_controls_and_exact_count_name_when_120_unread", () => {
    mockHistory = Array.from({ length: 120 }, (_, i) => row(i));
    renderHandle();
    const handle = screen.getByTestId("tray-handle");
    expect(screen.getByTestId("tray-handle-count")).toHaveTextContent("99+");
    expect(handle).toHaveAttribute("aria-expanded", "false");
    expect(handle).toHaveAttribute("aria-controls", "notification-tray");
    expect(handle).toHaveAccessibleName("Notifications, 120 unread");
  });

  it("handle_should_show_dot_and_decision_count_in_name_when_a_decision_is_pending", () => {
    mockHistory = [row(1, { isPendingDecision: true, notificationType: "approval_needed" }), row(2)];
    renderHandle();
    expect(screen.getByTestId("tray-handle-dot")).toBeInTheDocument();
    expect(screen.getByTestId("tray-handle")).toHaveAccessibleName("Notifications, 2 unread, 1 need attention");
  });

  it("handle_should_toggle_the_tray_on_click_and_reflect_open_state", () => {
    mockOpen = true;
    renderHandle();
    const handle = screen.getByTestId("tray-handle");
    expect(handle).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(handle);
    expect(mockToggle).toHaveBeenCalledTimes(1);
  });

  it("handle_should_not_blur_the_terminal_on_mousedown", () => {
    renderHandle();
    const notPrevented = fireEvent.mouseDown(screen.getByTestId("tray-handle"));
    expect(notPrevented).toBe(false);
  });

  it("handle_should_hide_when_flag_off_or_on_a_phone", () => {
    mockFlags.notification_tray_v2 = false;
    const { unmount } = renderHandle();
    expect(screen.queryByTestId("tray-handle")).toBeNull();
    unmount();
    mockFlags.notification_tray_v2 = true;
    renderHandle(false);
    expect(screen.queryByTestId("tray-handle")).toBeNull();
  });

  it("entry label names unread and decision counts", () => {
    expect(trayEntryLabel(7, 0)).toBe("Notifications, 7 unread");
    expect(trayEntryLabel(7, 2)).toBe("Notifications, 7 unread, 2 need attention");
  });
});
