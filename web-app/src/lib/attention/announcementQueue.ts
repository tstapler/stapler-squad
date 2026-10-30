export type AnnouncementKind = "needs-you" | "action-result" | "connection";

/** needs-you arrival (1) > action result (2) > connection state (3). */
export const ANNOUNCEMENT_PRIORITY: Record<AnnouncementKind, number> = {
  "needs-you": 1,
  "action-result": 2,
  connection: 3,
};

export const COALESCE_WINDOW_MS = 2500;
export const MAX_QUEUED = 3;
export const MAX_AGE_MS = 10_000;

interface Announcement {
  kind: AnnouncementKind;
  count: number;
  format: (count: number) => string;
  enqueuedAt: number;
  lastAt: number;
}

/** Pure priority queue; the clock is a parameter so tests need no timers. */
export class AnnouncementQueue {
  private entries: Announcement[] = [];

  enqueue(kind: AnnouncementKind, format: (count: number) => string, now: number, count = 1): void {
    const same = this.entries.find((e) => e.kind === kind && now - e.lastAt <= COALESCE_WINDOW_MS);
    if (same) {
      same.count += count;
      same.format = format;
      same.lastAt = now;
      return;
    }
    this.entries.push({ kind, count, format, enqueuedAt: now, lastAt: now });
    this.entries.sort(
      (a, b) => ANNOUNCEMENT_PRIORITY[a.kind] - ANNOUNCEMENT_PRIORITY[b.kind] || a.enqueuedAt - b.enqueuedAt,
    );
    // Overflow drops the lowest-priority entry, oldest first within a priority.
    while (this.entries.length > MAX_QUEUED) this.entries.pop();
  }

  /** Returns the highest-priority fresh message, or null; entries older than MAX_AGE_MS are discarded. */
  take(now: number): string | null {
    this.entries = this.entries.filter((e) => now - e.enqueuedAt <= MAX_AGE_MS);
    const next = this.entries.shift();
    return next ? next.format(next.count) : null;
  }

  size(): number {
    return this.entries.length;
  }
}
