import { type Session, SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";

export type BoardColumnKey = "running" | "needs_review" | "paused" | "complete";

export const BOARD_COLUMNS: { key: BoardColumnKey; label: string }[] = [
  { key: "running", label: "Running" },
  { key: "needs_review", label: "Needs Review" },
  { key: "paused", label: "Paused" },
  { key: "complete", label: "Complete" },
];

/**
 * Maps a session's backend status/subStatus to the board column it renders in.
 * UNSPECIFIED falls back to "running" defensively — it should not occur in practice.
 */
export function getBoardColumnKey(session: Session): BoardColumnKey {
  if (
    session.status === SessionStatus.ACTIVE &&
    (session.subStatus === SubStatus.NEEDS_APPROVAL || session.subStatus === SubStatus.INPUT_REQUIRED)
  ) {
    return "needs_review";
  }
  if (
    session.status === SessionStatus.ACTIVE ||
    session.status === SessionStatus.CREATING ||
    session.status === SessionStatus.RESTORING
  ) {
    return "running";
  }
  if (session.status === SessionStatus.PAUSED || session.status === SessionStatus.HIBERNATED) {
    return "paused";
  }
  if (session.status === SessionStatus.STOPPED) {
    return "complete";
  }
  return "running";
}
