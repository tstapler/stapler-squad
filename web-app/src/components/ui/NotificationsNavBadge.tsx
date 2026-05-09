"use client";

import { useNotifications } from "@/lib/contexts/NotificationContext";
import { badge, inline as inlineClass } from "./NavBadge.css";

interface NotificationsNavBadgeProps {
  inline?: boolean;
}

export function NotificationsNavBadge({ inline = false }: NotificationsNavBadgeProps) {
  const { getUnreadCount } = useNotifications();
  const count = getUnreadCount();

  if (count === 0) return null;

  const className = inline ? `${badge} ${inlineClass}` : badge;

  return (
    <span
      className={className}
      data-testid="notifications-nav-badge"
      aria-label={`${count} unread notification${count !== 1 ? "s" : ""}`}
    >
      {count > 99 ? "99+" : count}
    </span>
  );
}
