import { type Session, SessionStatus } from "@/gen/session/v1/types_pb";
import type { BoardColumnKey } from "./columns";

export const legalBoardTransitions: Record<BoardColumnKey, BoardColumnKey[]> = {
  running: ["paused", "complete"],
  paused: ["running", "complete"],
  needs_review: [], // handled via ApprovalResolution, not a raw column-to-column drag
  complete: [], // terminal board column; no outbound drag wired in this plan
};

export function isLegalBoardDrag(from: BoardColumnKey, to: BoardColumnKey): boolean {
  return legalBoardTransitions[from]?.includes(to) ?? false;
}

/**
 * Status-aware wrapper around isLegalBoardDrag: CREATING/RESTORING sessions have narrower
 * real backend edges than the "running" column they render in, so any drag out of that
 * column is illegal while transient regardless of column-level legality.
 */
export function isLegalBoardDragForSession(
  session: Session,
  fromColumn: BoardColumnKey,
  toColumn: BoardColumnKey
): boolean {
  if (session.status === SessionStatus.CREATING || session.status === SessionStatus.RESTORING) {
    return false;
  }
  return isLegalBoardDrag(fromColumn, toColumn);
}
