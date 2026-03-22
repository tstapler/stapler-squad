/**
 * notificationStorage.ts
 *
 * Manages localStorage persistence for notification deduplication and acknowledgment tracking.
 * Prevents duplicate notifications on WebSocket reconnection and page refresh.
 *
 * Key features:
 * - Track notified sessions to prevent re-notification on snapshot events
 * - Track acknowledged sessions with grace period to prevent immediate re-notification
 * - Automatic cleanup of expired records to prevent localStorage quota issues
 */

/** Record of a notification shown to the user */
export interface NotificationRecord {
  sessionId: string;
  /** Unix timestamp (ms) when notification was first shown */
  notifiedAt: number;
  /** Unix timestamp (ms) when user acknowledged/dismissed the notification */
  acknowledgedAt?: number;
}

const STORAGE_KEY = "stapler-squad-notifications";

/** Time-to-live for notification records (1 hour) */
const NOTIFICATION_TTL_MS = 60 * 60 * 1000;

/** Grace period after acknowledgment before re-notifying (5 minutes) */
const GRACE_PERIOD_MS = 5 * 60 * 1000;

/**
 * Safely retrieve notification records from localStorage.
 * Returns empty map on parse errors or missing data.
 */
export function getNotifiedSessions(): Map<string, NotificationRecord> {
  try {
    const data = localStorage.getItem(STORAGE_KEY);
    if (!data) return new Map();
    const records: NotificationRecord[] = JSON.parse(data);
    return new Map(records.map((r) => [r.sessionId, r]));
  } catch {
    // Parse error or localStorage unavailable
    return new Map();
  }
}

/**
 * Save notification records to localStorage.
 * Automatically cleans up expired records before saving.
 */
function saveRecords(records: Map<string, NotificationRecord>): void {
  try {
    // Clean up expired records before saving
    const now = Date.now();
    for (const [sessionId, record] of records) {
      if (now - record.notifiedAt > NOTIFICATION_TTL_MS) {
        records.delete(sessionId);
      }
    }
    const data = Array.from(records.values());
    localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
  } catch {
    // localStorage unavailable or quota exceeded
    console.warn("[notificationStorage] Failed to save notification records");
  }
}

/**
 * Mark a session as notified.
 * Called when a notification is shown to the user.
 */
export function markNotified(sessionId: string): void {
  const records = getNotifiedSessions();
  const existing = records.get(sessionId);
  records.set(sessionId, {
    sessionId,
    notifiedAt: Date.now(),
    // Preserve acknowledgment if it exists
    acknowledgedAt: existing?.acknowledgedAt,
  });
  saveRecords(records);
}

/**
 * Mark a session as acknowledged (dismissed/snoozed).
 * The grace period prevents re-notification for GRACE_PERIOD_MS.
 */
export function markAcknowledged(sessionId: string): void {
  const records = getNotifiedSessions();
  const existing = records.get(sessionId);
  records.set(sessionId, {
    sessionId,
    notifiedAt: existing?.notifiedAt ?? Date.now(),
    acknowledgedAt: Date.now(),
  });
  saveRecords(records);
}

/**
 * Check if a notification should be shown for a session.
 *
 * Returns false if:
 * - Session was already notified and is still in TTL window
 * - Session was acknowledged within grace period
 *
 * Returns true if:
 * - Never notified before
 * - Previous notification expired (> TTL)
 * - Acknowledgment expired (> grace period)
 */
export function shouldNotify(sessionId: string): boolean {
  const records = getNotifiedSessions();
  const record = records.get(sessionId);

  if (!record) {
    // Never notified - should notify
    return true;
  }

  const now = Date.now();

  // Check acknowledgment grace period first
  if (record.acknowledgedAt) {
    const timeSinceAck = now - record.acknowledgedAt;
    if (timeSinceAck < GRACE_PERIOD_MS) {
      // Still within grace period - don't notify
      return false;
    }
  }

  // Check if previous notification has expired
  const timeSinceNotified = now - record.notifiedAt;
  if (timeSinceNotified > NOTIFICATION_TTL_MS) {
    // Notification expired - can notify again
    return true;
  }

  // Already notified within TTL - don't duplicate
  return false;
}

/**
 * Check if a session is currently within the acknowledgment grace period.
 */
export function isInGracePeriod(sessionId: string): boolean {
  const records = getNotifiedSessions();
  const record = records.get(sessionId);

  if (!record?.acknowledgedAt) {
    return false;
  }

  const timeSinceAck = Date.now() - record.acknowledgedAt;
  return timeSinceAck < GRACE_PERIOD_MS;
}

/**
 * Get all sessions that are currently acknowledged (within grace period).
 */
export function getAcknowledgedSessions(): Set<string> {
  const records = getNotifiedSessions();
  const acknowledged = new Set<string>();
  const now = Date.now();

  for (const [sessionId, record] of records) {
    if (record.acknowledgedAt) {
      const timeSinceAck = now - record.acknowledgedAt;
      if (timeSinceAck < GRACE_PERIOD_MS) {
        acknowledged.add(sessionId);
      }
    }
  }

  return acknowledged;
}

/**
 * Remove all expired notification records.
 * Called automatically by saveRecords, but can be called manually.
 */
export function cleanupExpired(): void {
  const records = getNotifiedSessions();
  const now = Date.now();

  for (const [sessionId, record] of records) {
    if (now - record.notifiedAt > NOTIFICATION_TTL_MS) {
      records.delete(sessionId);
    }
  }

  saveRecords(records);
}

/**
 * Clear all notification records.
 * Useful for testing or resetting state.
 */
export function clearAll(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // localStorage unavailable
  }
}

/**
 * Mark multiple sessions as notified (batch operation).
 * More efficient than calling markNotified multiple times.
 */
export function markNotifiedBatch(sessionIds: string[]): void {
  if (sessionIds.length === 0) return;

  const records = getNotifiedSessions();
  const now = Date.now();

  for (const sessionId of sessionIds) {
    const existing = records.get(sessionId);
    records.set(sessionId, {
      sessionId,
      notifiedAt: now,
      acknowledgedAt: existing?.acknowledgedAt,
    });
  }

  saveRecords(records);
}
