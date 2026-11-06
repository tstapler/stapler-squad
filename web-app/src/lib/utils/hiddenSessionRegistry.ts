import type { HiddenSessionRef } from "@/lib/utils/backgroundActivity";

// Notification history records carry no hidden flag, so once a hidden session is deleted
// nothing server-side says it was hidden. The ids seen hidden in this tab are what keeps a
// failure row alive as "Session no longer available" (C7).
const known = new Map<string, HiddenSessionRef>();

export function rememberHiddenSession(ref: HiddenSessionRef): void {
  known.set(ref.id, { id: ref.id, title: ref.title });
}

export function knownHiddenSessions(): HiddenSessionRef[] {
  return Array.from(known.values());
}

/** Test seam. */
export function resetHiddenSessionRegistry(): void {
  known.clear();
}
