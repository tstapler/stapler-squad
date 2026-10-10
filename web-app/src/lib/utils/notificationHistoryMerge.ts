import type { NotificationHistoryRecord } from "@/gen/session/v1/session_pb";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import { mapNotificationType, mapPriority } from "@/lib/utils/notificationMapping";

export function recordToHistoryItem(record: NotificationHistoryRecord): NotificationHistoryItem {
  return {
    id: record.id,
    sessionId: record.sessionId,
    sessionName: record.sessionName,
    title: record.title,
    message: record.message,
    timestamp: record.createdAt ? Number(record.createdAt.seconds) * 1000 : Date.now(),
    priority: mapPriority(record.priority),
    notificationType: mapNotificationType(record.notificationType),
    metadata: record.metadata ? Object.fromEntries(Object.entries(record.metadata)) : undefined,
    isRead: record.isRead,
    occurrenceCount: record.occurrenceCount,
  };
}

const dedupKeyOf = (n: NotificationHistoryItem) => `${n.sessionId ?? ""}:${n.notificationType ?? ""}`;

/**
 * Merges the server's history into the local list. The backend is authoritative:
 * a local item is replaced by its server version (matched by id, or by
 * session+type for stream-added items that still carry a client-generated id) so
 * isRead and metadata always reflect server truth. Local-only callbacks (onView,
 * onApprove, ...) are carried over because they are never persisted.
 */
export function mergeBackendHistory(
  prev: NotificationHistoryItem[],
  backendItems: NotificationHistoryItem[],
): NotificationHistoryItem[] {
  const backendById = new Map(backendItems.map((n) => [n.id, n]));
  const backendByDedupKey = new Map(backendItems.map((n) => [dedupKeyOf(n), n]));

  const updated: NotificationHistoryItem[] = [];
  const consumedDedupKeys = new Set<string>();
  for (const n of prev) {
    const dk = dedupKeyOf(n);
    if (consumedDedupKeys.has(dk)) continue;
    const serverVersion = backendById.get(n.id) ?? backendByDedupKey.get(dk);
    updated.push(
      serverVersion
        ? { ...serverVersion, onView: n.onView, onApprove: n.onApprove, onDeny: n.onDeny, onFocusWindow: n.onFocusWindow }
        : n,
    );
    consumedDedupKeys.add(dk);
  }

  // Mutating the key set as we go keeps duplicate-type backend records (several
  // auto_approved entries for one session) from all slipping through.
  const existingIds = new Set(updated.map((n) => n.id));
  const existingDedupKeys = new Set(updated.map(dedupKeyOf));
  const fromBackend: NotificationHistoryItem[] = [];
  for (const n of backendItems) {
    if (existingIds.has(n.id)) continue;
    const dk = dedupKeyOf(n);
    if (existingDedupKeys.has(dk)) continue;
    fromBackend.push(n);
    existingDedupKeys.add(dk);
  }

  return [...fromBackend, ...updated];
}
