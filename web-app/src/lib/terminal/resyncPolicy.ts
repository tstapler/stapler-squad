/** A size-unchanged terminal that synced this recently is not resynced on tab/window focus. */
export const VISIBILITY_RESYNC_FRESH_MS = 60_000;

export interface SyncStamp {
  cols: number;
  rows: number;
  at: number;
}

export type VisibilityResyncDecision = "send" | "skipped_fresh" | "skipped_unavailable";

/**
 * Whether a focus/visibility event should cost a full resync (a CapturePane/full-state redraw on
 * the server, seconds under load). The stream is ordered and a resize always resyncs by itself,
 * so a resync is only needed when the size differs from the last synced one, nothing has synced
 * yet, or the last sync is old enough that a throttled background tab may have missed output.
 */
export function decideVisibilityResync(
  terminal: { cols: number; rows: number } | null,
  lastSync: SyncStamp | null,
  now: number,
  freshMs: number = VISIBILITY_RESYNC_FRESH_MS,
): VisibilityResyncDecision {
  if (!terminal) return "skipped_unavailable";
  if (!lastSync) return "send";
  const sameSize = terminal.cols === lastSync.cols && terminal.rows === lastSync.rows;
  return sameSize && now - lastSync.at < freshMs ? "skipped_fresh" : "send";
}
