import { DetectedStatus, SubStatus, WorkingState } from "@/gen/session/v1/types_pb";
import { assertNever } from "@/lib/utils/assertNever";

/**
 * Derives the effective WorkingState for a session from its SubStatus and DetectedStatus.
 *
 * This is the frontend equivalent of the removed server-side MapDetectedStatusToWorkingState.
 * SubStatus is the primary signal; DetectedStatus is a fallback for cases where SubStatus
 * is UNSPECIFIED (e.g. non-Active sessions or legacy items).
 *
 * Mapping:
 *   SubStatus.PROCESSING, NEEDS_APPROVAL, INPUT_REQUIRED, ERROR, TESTS_FAILING, RATE_LIMITED, WAITING_FOR_AGENT
 *     → WorkingState.PROCESSING  (actively doing something or blocked on user)
 *   SubStatus.IDLE, READY
 *     → WorkingState.IDLE        (at prompt, ready for next instruction)
 *   SubStatus.SUCCESS
 *     → WorkingState.IDLE        (task finished, prompt available)
 *   SubStatus.UNSPECIFIED → fall through to detectedStatus
 *     DetectedStatus.EXECUTING, WAITING_FOR_AGENT → WorkingState.ACTIVE
 *     DetectedStatus.PROCESSING                   → WorkingState.PROCESSING
 *     default                                     → WorkingState.UNSPECIFIED
 */
export function deriveWorkingState(session: {
  subStatus: SubStatus;
  detectedStatus?: DetectedStatus;
}): WorkingState {
  switch (session.subStatus) {
    case SubStatus.PROCESSING:
    case SubStatus.NEEDS_APPROVAL:
    case SubStatus.INPUT_REQUIRED:
    case SubStatus.ERROR:
    case SubStatus.TESTS_FAILING:
    case SubStatus.RATE_LIMITED:
    case SubStatus.WAITING_FOR_AGENT:
      return WorkingState.PROCESSING;
    case SubStatus.IDLE:
    case SubStatus.READY:
    case SubStatus.SUCCESS:
      return WorkingState.IDLE;
    case SubStatus.UNSPECIFIED:
      // fall through to detectedStatus-based fallback
      break;
  }

  // detectedStatus-based fallback (used when subStatus is UNSPECIFIED)
  switch (session.detectedStatus) {
    case DetectedStatus.EXECUTING:
    case DetectedStatus.WAITING_FOR_AGENT:
      return WorkingState.ACTIVE;
    case DetectedStatus.PROCESSING:
    case DetectedStatus.NEEDS_APPROVAL:
    case DetectedStatus.INPUT_REQUIRED:
    case DetectedStatus.ERROR:
    case DetectedStatus.TESTS_FAILING:
      return WorkingState.PROCESSING;
    case DetectedStatus.IDLE:
    case DetectedStatus.READY:
      return WorkingState.IDLE;
    case DetectedStatus.SUCCESS:
    case DetectedStatus.UNKNOWN:
    case DetectedStatus.UNSPECIFIED:
    case undefined:
      return WorkingState.UNSPECIFIED;
    default:
      return assertNever(session.detectedStatus);
  }
}
