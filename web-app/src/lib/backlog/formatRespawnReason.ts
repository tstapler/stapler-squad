/**
 * Maps a RespawnEvent.reason value to a human-readable label. Mirrors
 * session/backlog.go's RespawnReason* constants. An unrecognized reason
 * (future call site added server-side before the frontend catches up) falls
 * back to the raw string rather than an empty/misleading label.
 */
export function formatRespawnReason(reason: string): string {
  switch (reason) {
    case "autonomous_turn_respawn":
      return "Autonomous turn respawn";
    case "stale_work_remediation":
      return "Stale work session remediation";
    case "review_respawn":
      return "Abandoned review respawn";
    case "triage_respawn":
      return "Orphaned triage respawn";
    default:
      return reason;
  }
}
