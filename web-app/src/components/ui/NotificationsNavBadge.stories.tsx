// +feature: ui-notifications-nav-badge
import { useEffect } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { NotificationProvider, useNotifications } from "@/lib/contexts/NotificationContext";
import { NotificationsNavBadge } from "./NotificationsNavBadge";

const meta: Meta<typeof NotificationsNavBadge> = {
  component: NotificationsNavBadge,
  title: "UI/NotificationsNavBadge",
};
export default meta;
type Story = StoryObj<typeof NotificationsNavBadge>;

// The no-provider fallback lacks getUnreadCount, so a real provider is required.
// Zero unread renders no badge; the label keeps the canvas non-empty.
export const NoUnread: Story = {
  render: () => (
    <NotificationProvider>
      <span>
        Notifications <NotificationsNavBadge inline />
      </span>
    </NotificationProvider>
  ),
};

function SeededBadge({ count, inline }: { count: number; inline?: boolean }) {
  const { addToHistoryOnly } = useNotifications();
  useEffect(() => {
    for (let i = 0; i < count; i++) {
      addToHistoryOnly({
        sessionId: `session-${i}`,
        sessionName: `Session ${i}`,
        message: `Needs attention ${i}`,
        notificationType: "info",
      });
    }
    // seed once on mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return (
    <span>
      Notifications <NotificationsNavBadge inline={inline} />
    </span>
  );
}

export const WithUnread: Story = {
  render: () => (
    <NotificationProvider>
      <SeededBadge count={3} inline />
    </NotificationProvider>
  ),
};
