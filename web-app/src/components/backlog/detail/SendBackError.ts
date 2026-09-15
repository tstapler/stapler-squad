export type SendBackFailedAt = "transition" | "reject" | "triage";

/**
 * Thrown by handleSendBackWithFeedback (Task 2.2.1a) so SendBackFeedbackBox
 * can render distinct recovery copy per which of the 3 chained calls failed
 * (ux.md Surface 7) instead of one generic message for every failure.
 * itemStatusAfterFailure is the item's real status re-fetched after the
 * failure — needed because triggerTriage's own internal ready→idea CAS
 * (backlog_service_trigger_triage.go:249-260) can commit before a LATER
 * step in that same call fails, leaving the item at "idea" rather than
 * "ready" (pre-mortem P1 #1).
 */
export class SendBackError extends Error {
  constructor(
    public readonly failedAt: SendBackFailedAt,
    public readonly itemStatusAfterFailure: string,
    public readonly cause: unknown
  ) {
    super(cause instanceof Error ? cause.message : String(cause));
    this.name = "SendBackError";
  }
}
