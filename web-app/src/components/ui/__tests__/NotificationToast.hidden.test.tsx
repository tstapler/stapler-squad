import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { NotificationToast } from "@/components/ui/NotificationToast";
import type { NotificationData } from "@/lib/types/notification";
import { useSessionHidden } from "@/lib/hooks/useSessionHidden";

jest.mock("@/lib/hooks/useAuditLog", () => ({
  useAuditLog: () => ({ logNotificationSessionViewed: jest.fn() }),
}));
jest.mock("@/lib/hooks/useSessionHidden", () => ({ useSessionHidden: jest.fn() }));
let mockReplyFlag = true;
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: (name: string) => (name === "hidden_session_reply" ? mockReplyFlag : false),
}));

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

  describe("Reply action (Story 5.6, RP-8)", () => {
    const replyable = {
      notificationType: "question" as const,
      title: "Claude has a question",
      metadata: { question_id: "q1", question_shape: "single", question_options: '["A","B"]' },
    };

    beforeEach(() => {
      mockReplyFlag = true;
    });

    it("rp8_hidden_question_toast_should_offer_reply_beside_view_output", () => {
      (useSessionHidden as jest.Mock).mockReturnValue(true);
      const notification = make(replyable);
      render(<NotificationToast notification={notification} onClose={jest.fn()} stacked />);
      expect(screen.getByTestId("notification-view-output")).toBeInTheDocument();
      fireEvent.click(screen.getByTestId("notification-reply"));
      expect(notification.onView).toHaveBeenCalledTimes(1);
    });

    it.each([
      ["a visible session", () => (useSessionHidden as jest.Mock).mockReturnValue(false), replyable],
      ["a multi-select question", () => (useSessionHidden as jest.Mock).mockReturnValue(true), { ...replyable, metadata: { question_shape: "multi" } }],
      ["a question with no proof", () => (useSessionHidden as jest.Mock).mockReturnValue(true), { ...replyable, metadata: { reply_unavailable: "no_proof" } }],
      ["the kill switch off", () => { (useSessionHidden as jest.Mock).mockReturnValue(true); mockReplyFlag = false; }, replyable],
    ])("rp8_should_not_offer_reply_for_%s", (_name, arrange, over) => {
      arrange();
      render(<NotificationToast notification={make(over as Partial<NotificationData>)} onClose={jest.fn()} stacked />);
      expect(screen.queryByTestId("notification-reply")).toBeNull();
    });
  });
});
