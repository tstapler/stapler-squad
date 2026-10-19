import type { NotificationHistoryItem } from "@/lib/types/notification";
import type { BackgroundRow, BackgroundOutcome, HiddenSessionSummary } from "@/lib/utils/backgroundActivity";

export function makeNotification(overrides: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem {
  return {
    id: overrides.id ?? "notif-1",
    sessionId: "sess-1",
    sessionName: "my-session",
    message: "Something happened",
    timestamp: Date.now(),
    isRead: false,
    notificationType: "info",
    ...overrides,
  };
}

export function makeHiddenSession(over: Partial<HiddenSessionSummary> = {}): HiddenSessionSummary {
  const id = over.id ?? "review:h1";
  return { id, title: id, state: "stopped", updatedAtMs: Date.now() - 3 * 60_000, backgroundOutcome: "unknown", ...over };
}

/** The live-shaped list: 10 stopped `review:*` sessions and 1 stopped `diagnose:*`, newest first. */
export function makeElevenStopped(reviewOutcome: BackgroundOutcome = "completed", now = Date.now()): HiddenSessionSummary[] {
  const reviews = Array.from({ length: 10 }, (_, i) =>
    makeHiddenSession({ id: `review:r${i}`, backgroundOutcome: reviewOutcome, updatedAtMs: now - (3 + i) * 60_000 }),
  );
  return [...reviews, makeHiddenSession({ id: "diagnose:d1", backgroundOutcome: "unknown", updatedAtMs: now - 60 * 60_000 })];
}

export function makeBackgroundRow(over: Partial<BackgroundRow> = {}): BackgroundRow {
  return {
    key: "bg:review:h1",
    sessionId: "review:h1",
    sessionTitle: "review:h1",
    kind: "failure",
    statusLabel: "FAILED",
    title: "Review failed",
    message: "3 tests failing",
    timestampMs: Date.now() - 4 * 60_000,
    recordIds: ["n1"],
    primaryRecordId: "n1",
    sessionAvailable: true,
    pendingQuestion: false,
    ...over,
  };
}

/** A `useBackgroundSessions` state, as the NotificationPanel suite mocks it. */
export function makeHiddenState(over: Record<string, unknown> = {}) {
  return {
    sessions: [{ id: "review:h1", title: "review:h1", state: "running", updatedAtMs: Date.now() }],
    departed: [],
    loading: false,
    failed: false,
    lastUpdatedAt: Date.now(),
    refresh: () => {},
    ...over,
  };
}
