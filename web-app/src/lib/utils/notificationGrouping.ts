import { NotificationHistoryItem } from "@/lib/types/notification";

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

// ---------------------------------------------------------------------------
// Tray model: per-session groups, the pinned "Needs attention" group and the
// flattened row list the virtualized tray renders.
// ---------------------------------------------------------------------------

export const NEEDS_ATTENTION_KEY = "__needs_attention__";

/** Groups whose representative row is a pending decision (the server-sent field, never a type list). */
export function selectPinnedDecisions(groups: GroupedNotification[]): GroupedNotification[] {
  return groups.filter((g) => g.notification.isPendingDecision === true);
}

/** Count behind "N need attention" and the handle's dot: one per pinned group. */
export function countNeedsAttention(notifications: NotificationHistoryItem[]): number {
  return selectPinnedDecisions(groupNotifications(notifications)).length;
}

export interface SessionGroup {
  /** Stable collapse key. */
  key: string;
  sessionName: string;
  groups: GroupedNotification[];
  /** Notification records in this session group (sum of group counts is NOT used; rows are records). */
  rowCount: number;
}

/** Splits non-pinned groups by session, newest session first. */
export function groupBySession(groups: GroupedNotification[]): SessionGroup[] {
  const bySession = new Map<string, SessionGroup>();
  for (const group of groups) {
    const key = group.notification.sessionId || "__no_session__";
    let bucket = bySession.get(key);
    if (!bucket) {
      bucket = {
        key,
        sessionName: group.notification.sessionName || group.notification.sessionId || "No session",
        groups: [],
        rowCount: 0,
      };
      bySession.set(key, bucket);
    }
    bucket.groups.push(group);
    bucket.rowCount += group.allIds.length;
  }
  // `groups` arrive newest first, so insertion order is already newest-session first.
  return [...bySession.values()];
}

export type TrayRow =
  | { kind: "header"; key: string; label: string; count: number; collapsed: boolean; pinned: boolean }
  | { kind: "note"; key: string; text: string }
  | {
      kind: "group";
      key: string;
      group: GroupedNotification;
      pinned: boolean;
      /** Position among the selectable rows (headers excluded) for aria-posinset. */
      posInSet: number;
    };

/**
 * Flattens the tray into the single ordered list the virtualizer windows over:
 * the pinned group first (never collapsible), then one header and its rows per
 * session, skipping the rows of a collapsed session.
 */
export function flattenGroups(
  notifications: NotificationHistoryItem[],
  collapsed: ReadonlySet<string>,
): { rows: TrayRow[]; needsAttention: number; setSize: number } {
  const all = groupNotifications(notifications);
  const pinned = selectPinnedDecisions(all);
  const pinnedSet = new Set(pinned);
  const rest = all.filter((g) => !pinnedSet.has(g));
  const rows: TrayRow[] = [];
  let pos = 0;

  if (pinned.length > 0) {
    rows.push({ kind: "header", key: NEEDS_ATTENTION_KEY, label: "NEEDS ATTENTION", count: pinned.length, collapsed: false, pinned: true });
    for (const group of pinned) {
      rows.push({ kind: "group", key: group.notification.id, group, pinned: true, posInSet: ++pos });
    }
  } else if (rest.length > 0) {
    rows.push({ kind: "note", key: "nothing-needs-attention", text: "Nothing needs attention" });
  }

  for (const session of groupBySession(rest)) {
    const isCollapsed = collapsed.has(session.key);
    rows.push({ kind: "header", key: session.key, label: session.sessionName, count: session.rowCount, collapsed: isCollapsed, pinned: false });
    if (isCollapsed) continue;
    for (const group of session.groups) {
      rows.push({ kind: "group", key: group.notification.id, group, pinned: false, posInSet: ++pos });
    }
  }
  return { rows, needsAttention: pinned.length, setSize: pos };
}
