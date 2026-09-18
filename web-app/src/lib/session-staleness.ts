import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

/**
 * Returns the more recent of `lastMeaningfulOutput` and `lastTerminalUpdate`,
 * or `undefined` if neither is set. Extracted verbatim from the inline IIFE
 * that used to live in SessionCard.tsx's "Active Xm ago" header row.
 */
export function getLastActivityTimestamp(session: Session): Timestamp | undefined {
  const moSecs = session.lastMeaningfulOutput?.seconds ?? BigInt(0);
  const tuSecs = session.lastTerminalUpdate?.seconds ?? BigInt(0);
  if (moSecs === BigInt(0) && tuSecs === BigInt(0)) {
    return undefined;
  }
  return moSecs >= tuSecs ? session.lastMeaningfulOutput : session.lastTerminalUpdate;
}

/**
 * A session is stale if it's ACTIVE but hasn't produced any recorded
 * activity within `thresholdMinutes`. Non-ACTIVE sessions (PAUSED, STOPPED,
 * HIBERNATED, CREATING, RESTORING, etc.) are never considered stale, and a
 * session with no recorded activity at all (e.g. brand new) is never
 * flagged either.
 */
export function isSessionStale(session: Session, thresholdMinutes: number): boolean {
  if (session.status !== SessionStatus.ACTIVE) {
    return false;
  }
  const lastActivity = getLastActivityTimestamp(session);
  if (!lastActivity) {
    return false;
  }
  const lastActivityMs = Number(lastActivity.seconds) * 1000;
  return Date.now() - lastActivityMs > thresholdMinutes * 60_000;
}
