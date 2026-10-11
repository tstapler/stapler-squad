/**
 * Which sessions the operator is looking at right now. SessionDetailView reports
 * its session here while mounted and the tab is visible; the notification
 * context uses it to keep a non-pinned toast for that session off the deck (the
 * history row is still recorded). A multiset, because two panes can show one session.
 */
const counts = new Map<string, number>();

export function markSessionViewed(sessionId: string): () => void {
  if (!sessionId) return () => {};
  counts.set(sessionId, (counts.get(sessionId) ?? 0) + 1);
  let released = false;
  return () => {
    if (released) return;
    released = true;
    const next = (counts.get(sessionId) ?? 1) - 1;
    if (next <= 0) counts.delete(sessionId);
    else counts.set(sessionId, next);
  };
}

export function isSessionViewed(sessionId: string | undefined): boolean {
  if (!sessionId || !counts.has(sessionId)) return false;
  return typeof document === "undefined" || document.visibilityState !== "hidden";
}
