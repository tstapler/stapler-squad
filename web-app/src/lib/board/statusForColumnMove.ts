import { type Session, SessionStatus } from "@/gen/session/v1/types_pb";
import type { BoardColumnKey } from "./columns";

/**
 * Maps a drop target column to the concrete SessionStatus to send via updateSession, branching
 * on the session's *current* status (not just its column) — "paused" column members can be
 * either PAUSED or HIBERNATED and need different handling on the way out.
 *
 * Returns null when the caller must use a different RPC entirely (resumeHibernatedSession for
 * a HIBERNATED session moving to "running" — hibernation has its own dedicated RPC, it isn't a
 * plain status write).
 */
export function statusForColumnMove(session: Session, targetColumn: BoardColumnKey): SessionStatus | null {
  switch (targetColumn) {
    case "paused":
      return SessionStatus.PAUSED;
    case "complete":
      return SessionStatus.STOPPED;
    case "running":
      if (session.status === SessionStatus.HIBERNATED) {
        return null;
      }
      return SessionStatus.ACTIVE;
    case "needs_review":
      // No board-drag ever targets "needs_review" directly (legalBoardTransitions has no
      // inbound edges to it) — callers should never reach this branch via a legal drag.
      return null;
    default:
      return null;
  }
}
