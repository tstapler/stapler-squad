"use client";

import { useNotifications } from "@/lib/contexts/NotificationContext";
import { NavBadge } from "./NavBadge";

interface NotificationsNavBadgeProps {
  inline?: boolean;
}

export function NotificationsNavBadge({ inline = false }: NotificationsNavBadgeProps) {
  const { getUnreadCount } = useNotifications();
  const count = getUnreadCount();

  return (
    <NavBadge
      count={count}
      element="span"
      inline={inline}
      data-testid="notifications-nav-badge"
      aria-label={`${count} unread notification${count !== 1 ? "s" : ""}`}
    />
  );
}
