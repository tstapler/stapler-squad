import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { NotificationToast } from "@/components/ui/NotificationToast";
import type { NotificationData } from "@/lib/types/notification";
import { useSessionHidden } from "@/lib/hooks/useSessionHidden";

jest.mock("@/lib/hooks/useAuditLog", () => ({
  useAuditLog: () => ({ logNotificationSessionViewed: jest.fn() }),
}));
jest.mock("@/lib/hooks/useSessionHidden", () => ({ useSessionHidden: jest.fn() }));

const make = (overrides: Partial<NotificationData> = {}): NotificationData => ({
  id: "t1",
  sessionId: "h1",
  sessionName: "review:ee1b4be0",
  title: "Review failed",
  message: "tests failing",
  timestamp: Date.now(),
  notificationType: "error",
  sourceApp: "Terminal",
  onView: jest.fn(),
  onFocusWindow: jest.fn(),
  ...overrides,
});

describe("hidden-session toast variant (Story 5.3, C6/C15)", () => {
  beforeEach(() => {
    window.matchMedia = jest.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
    })) as unknown as typeof window.matchMedia;
  });

  it("hidden_toast_should_show_background_chip_and_view_output_by_testid_and_no_focus_window", () => {
    (useSessionHidden as jest.Mock).mockReturnValue(true);
    const notification = make();
    render(<NotificationToast notification={notification} onClose={jest.fn()} stacked />);

    expect(screen.getByTestId("notification-background-chip")).toHaveTextContent("Background");
    const view = screen.getByTestId("notification-view-output");
    expect(view).toHaveTextContent("View output");
    expect(screen.queryByTestId("notification-view-session")).toBeNull();
    expect(screen.queryByText(/Focus Window/)).toBeNull();

    fireEvent.click(view);
    expect(notification.onView).toHaveBeenCalledTimes(1);
  });

  it("visible_toast_should_keep_view_session_label_and_testid", () => {
    (useSessionHidden as jest.Mock).mockReturnValue(false);
    render(<NotificationToast notification={make()} onClose={jest.fn()} stacked />);

    expect(screen.getByTestId("notification-view-session")).toHaveTextContent("View Session");
    expect(screen.queryByTestId("notification-background-chip")).toBeNull();
    expect(screen.queryByTestId("notification-view-output")).toBeNull();
  });

  it("unknown_hidden_state_should_render_the_visible_variant", () => {
    (useSessionHidden as jest.Mock).mockReturnValue(undefined);
    render(<NotificationToast notification={make()} onClose={jest.fn()} stacked />);
    expect(screen.getByTestId("notification-view-session")).toBeInTheDocument();
  });
});
