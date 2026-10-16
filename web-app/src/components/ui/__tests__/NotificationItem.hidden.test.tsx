import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { NotificationItem } from "../NotificationItem";
import type { GroupedNotification } from "@/lib/utils/notificationGrouping";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import { useSessionHidden } from "@/lib/hooks/useSessionHidden";

jest.mock("@/lib/hooks/useSessionHidden", () => ({ useSessionHidden: jest.fn() }));
let mockReplyFlag = true;
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: (name: string) => (name === "hidden_session_reply" ? mockReplyFlag : false),
}));
jest.mock("next/link", () => ({
  __esModule: true,
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [k: string]: unknown }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

function group(over: Partial<NotificationHistoryItem> = {}): GroupedNotification {
  const notification: NotificationHistoryItem = {
    id: "n-9",
    sessionId: "review:h1",
    sessionName: "review:h1",
    message: "tests failing",
    timestamp: Date.now(),
    notificationType: "error",
    isRead: false,
    sourceApp: "Terminal",
    onFocusWindow: jest.fn(),
    ...over,
  };
  return { notification, count: 1, allIds: [notification.id] };
}

function renderItem(g: GroupedNotification, extra: Record<string, unknown> = {}) {
  return render(
    <NotificationItem
      group={g}
      resolvedApprovals={{}}
      pendingApprovals={{}}
      blockedApprovals={{}}
      failedApprovals={{}}
      resolveApproval={jest.fn()}
      removeFromHistory={jest.fn()}
      handleNotificationClick={jest.fn()}
      {...extra}
    />,
  );
}

describe("hidden-session tray row (Task 5.3f, C6)", () => {
  it("hidden_row_should_show_background_chip_and_view_output_testid_linking_to_the_read_only_view", () => {
    (useSessionHidden as jest.Mock).mockReturnValue(true);
    const handle = jest.fn();
    renderItem(group(), {
      handleNotificationClick: handle,
      getSessionHref: () => "/sessions/summary?sessionId=review%3Ah1",
    });

    expect(screen.getByTestId("notification-background-chip")).toHaveTextContent("Background");
    const view = screen.getByTestId("notification-view-output");
    expect(view).toHaveTextContent("View output");
    expect(view).toHaveAttribute("href", "/?session=review%3Ah1&tab=terminal&notification=n-9");
    expect(screen.queryByTestId("notification-view-session")).toBeNull();
    expect(screen.queryByText(/Focus/)).toBeNull();

    fireEvent.click(view);
    expect(handle).toHaveBeenCalledWith(["n-9"], undefined, "review:h1");
  });

  it("visible_row_should_keep_view_session_label_and_testid", () => {
    (useSessionHidden as jest.Mock).mockReturnValue(false);
    renderItem(group());
    const view = screen.getByTestId("notification-view-session");
    expect(view).toHaveTextContent("View Session");
    expect(screen.queryByTestId("notification-background-chip")).toBeNull();
    expect(screen.queryByTestId("notification-view-output")).toBeNull();
  });

  it("lookup_should_follow_the_lookupHidden_prop", () => {
    (useSessionHidden as jest.Mock).mockReturnValue(undefined);
    renderItem(group(), { lookupHidden: false });
    expect(useSessionHidden).toHaveBeenLastCalledWith("review:h1", false);
    renderItem(group());
    expect(useSessionHidden).toHaveBeenLastCalledWith("review:h1", true);
  });

  it("unknown_hidden_state_should_render_the_visible_variant", () => {
    (useSessionHidden as jest.Mock).mockReturnValue(undefined);
    renderItem(group());
    expect(screen.getByTestId("notification-view-session")).toBeInTheDocument();
  });

  describe("Reply action (Story 5.6, RP-8)", () => {
    const question = {
      notificationType: "question" as const,
      metadata: { question_id: "q1", question_shape: "single", question_options: '["A","B"]' },
    };

    beforeEach(() => {
      mockReplyFlag = true;
      (useSessionHidden as jest.Mock).mockReturnValue(true);
    });

    it("rp8_hidden_question_row_should_link_reply_with_the_reply_deep_link_beside_view_output", () => {
      renderItem(group(question));
      expect(screen.getByTestId("notification-reply")).toHaveAttribute(
        "href",
        "/?session=review%3Ah1&tab=terminal&notification=n-9&reply=1",
      );
      expect(screen.getByTestId("notification-view-output")).toBeInTheDocument();
    });

    it("rp8_should_not_show_reply_for_a_non_replyable_question_or_with_the_kill_switch_off", () => {
      renderItem(group({ ...question, metadata: { question_shape: "multi" } }));
      expect(screen.queryByTestId("notification-reply")).toBeNull();
      mockReplyFlag = false;
      renderItem(group(question));
      expect(screen.queryByTestId("notification-reply")).toBeNull();
    });

    it("rp8_should_not_show_reply_on_a_visible_session", () => {
      (useSessionHidden as jest.Mock).mockReturnValue(false);
      renderItem(group(question));
      expect(screen.queryByTestId("notification-reply")).toBeNull();
    });
  });
});
