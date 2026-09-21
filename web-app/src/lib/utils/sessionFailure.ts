/**
 * Canonical FailureReason taxonomy set server-side by the Background
 * Resolution Pipeline (session.Instance.FailureReason, see ADR-001) and
 * carried on the wire via Session.failure_reason (types_pb.ts). Both the
 * persistent card/row copy (getFailureMessage below) and the transient toast
 * copy (NotificationContext.tsx's getFailureReasonToastMessage) key off this
 * same set of reasons — importing it there guarantees the two switches never
 * silently drift apart on *which* reasons are handled, even though the two
 * contexts intentionally use different wording.
 */
export type FailureReason =
  | "GitHubResolutionError"
  | "StartupError"
  | "Stale"
  | "Cancelled";

/**
 * Reason-specific copy for the persistent Failed-state card/row message
 * (async-session-creation Epic 5.2/plan.md Story 5.2.2 / UX research §2/§4's
 * "three different messages, not one generic Failed card"). Shared by
 * SessionCard.tsx and SessionRow.tsx so the two views never show conflicting
 * copy for the same FailureReason.
 */
export function getFailureMessage(failureReason: string): string {
  switch (failureReason as FailureReason) {
    case "GitHubResolutionError":
      return "Failed to resolve GitHub URL.";
    case "StartupError":
      return "Failed to start session.";
    case "Stale":
      return "This session creation appears to have stalled.";
    default:
      return "Session creation failed.";
  }
}
