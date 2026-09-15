import { getErrorMessage } from "@/lib/utils/connectError";
import type { BacklogItemStatus } from "@/lib/hooks/useBacklogService";

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
    public readonly itemStatusAfterFailure: BacklogItemStatus,
    cause: unknown
  ) {
    super(cause instanceof Error ? cause.message : String(cause), { cause });
    this.name = "SendBackError";
  }
}

export interface SendBackErrorCopy {
  headline: string;
  message: string;
  retryable?: boolean;
}

/**
 * Maps a handleSubmit failure to the recovery copy SendBackFeedbackBox
 * renders (ux.md Surface 6/7). A plain rejection with no SendBackError
 * wrapper falls back to the generic Surface 6 copy. `retryable` is true only
 * for the "landed at idea" case: idea→ready is itself a valid transition
 * (session/domain/backlog.go's validTransitions), so unlike the other two
 * branches — where resubmitting would replay transitionStatus against an
 * already-"ready" item and fail — retrying from "idea" is a genuine recovery
 * path, not a misleading affordance.
 */
const RETRIAGE_HEADLINE = "Sent back, but retriage didn't start";

export function deriveSendBackErrorCopy(err: unknown): SendBackErrorCopy {
  if (!(err instanceof SendBackError) || err.failedAt === "transition") {
    return { headline: "Failed to send back", message: getErrorMessage(err, "Failed to send back.") };
  }

  // triggerTriage's own internal ready→idea CAS can commit before a later
  // step in that same call fails (pre-mortem P1 #1), landing the item at
  // "idea" rather than "ready".
  if (err.itemStatusAfterFailure === "idea") {
    return {
      headline: RETRIAGE_HEADLINE,
      message:
        'The item moved back to "idea" before retriage could start. Your ' +
        "feedback is still in this box — click Retry to send it back to " +
        '"ready" and start retriage again.',
      retryable: true,
    };
  }
  if (err.failedAt === "reject") {
    return {
      headline: RETRIAGE_HEADLINE,
      message:
        'The item already moved to "ready." Close this form and use the ' +
        '"Request Changes" button below to record your feedback and retry ' +
        "— do not resubmit this box.",
    };
  }

  // rejectPlan succeeded, so point at "Regenerate Plan with This Feedback"
  // (PlanVerdictBox), not the plain "Trigger Triage" button — that one sends
  // no feedback argument and the backend never reads the already-persisted
  // PlanRejectionReason on its own, so it would silently discard it.
  return {
    headline: RETRIAGE_HEADLINE,
    message:
      'The item already moved to "ready" and your feedback was recorded. ' +
      'Close this form and use the "Regenerate Plan with This Feedback" ' +
      "button below to retry — do not resubmit this box.",
  };
}
