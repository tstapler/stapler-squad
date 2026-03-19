import { NotificationHistoryItem } from "@/lib/contexts/NotificationContext";

export interface GroupedNotification {
  /** Representative notification (most recent in the group) */
  notification: NotificationHistoryItem;
  /** Total occurrences -- from server occurrence_count if available, else client-side count */
  count: number;
  /** IDs of all notifications in this group (for batch mark-read) */
  allIds: string[];
}

/**
 * Groups notifications by (sessionId, notificationType).
 *
 * - Uses server-provided occurrenceCount when a record has one (>= 1)
 * - Falls back to client-side grouping for backward compatibility
 * - Groups ordered by most recent timestamp
 * - Representative notification is the most recent one (latest metadata/approval_id)
 *
 * Count precedence (see ADR in notification-deduplication.md):
 *   count = max(representative.occurrenceCount ?? 0, group.length)
 * This ensures the badge is correct both for server-deduplicated single records
 * (occurrenceCount = N, group.length = 1) and for pre-dedup stale data where
 * multiple records exist for the same key (occurrenceCount = 0, group.length = N).
 */
export function groupNotifications(
  notifications: NotificationHistoryItem[]
): GroupedNotification[] {
  // 1. Build a Map keyed by "sessionId:notificationType"
  const groups = new Map<string, NotificationHistoryItem[]>();

  for (const notification of notifications) {
    const key = `${notification.sessionId ?? ""}:${notification.notificationType ?? ""}`;
    const existing = groups.get(key);
    if (existing) {
      existing.push(notification);
    } else {
      groups.set(key, [notification]);
    }
  }

  // 2. For each group, sort by timestamp descending and build the result
  const result: GroupedNotification[] = [];

  for (const members of groups.values()) {
    // Sort members by timestamp descending (most recent first)
    members.sort((a, b) => b.timestamp - a.timestamp);

    const representative = members[0];

    // The server's occurrenceCount field (proto field 12) is carried through
    // from NotificationHistoryRecord during hydration in NotificationContext.
    // When the record was server-deduplicated it will be >= 1.
    // For old records without the field it defaults to 0.
    const serverCount = representative.occurrenceCount ?? 0;

    // Prefer server count when available, fall back to client-side group size
    const count = Math.max(serverCount, members.length);

    const allIds = members.map((m) => m.id);

    result.push({ notification: representative, count, allIds });
  }

  // 3. Sort groups by representative's timestamp descending (most recent first)
  result.sort((a, b) => b.notification.timestamp - a.notification.timestamp);

  return result;
}
