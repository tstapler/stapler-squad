import type { AnnouncementKind } from "./announcementQueue";

export interface AttentionAnnouncement {
  kind: AnnouncementKind;
  format: (count: number) => string;
  count: number;
}

type Listener = (announcement: AttentionAnnouncement) => void;

const listeners = new Set<Listener>();

/** Queues `text` on the single AttentionAnnouncer; a no-op when none is mounted. Never include a time or age. */
export function announceAttention(kind: AnnouncementKind, text: string): void {
  emit({ kind, format: () => text, count: 1 });
}

/** Burst-coalescing arrival message: "3 sessions need you". */
export function announceNeedsYou(count: number): void {
  if (count <= 0) return;
  emit({
    kind: "needs-you",
    count,
    format: (n) => (n === 1 ? "1 session needs you" : `${n} sessions need you`),
  });
}

function emit(announcement: AttentionAnnouncement): void {
  for (const listener of listeners) listener(announcement);
}

/** Used by AttentionAnnouncer only. */
export function subscribeAttentionAnnouncements(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
