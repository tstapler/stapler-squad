import { DetectedStatus, SubStatus, WorkingState } from "@/gen/session/v1/types_pb";

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
    case undefined:
      // Fall through to detectedStatus-based fallback intentionally. `undefined`
      // is handled defensively alongside UNSPECIFIED even though subStatus is
      // typed as required — some callers (e.g. partially-typed test fixtures)
      // may omit it, and it should behave identically to an unset/UNSPECIFIED
      // sub-status rather than throwing.
      break;
    default: {
      // Proto enums are forward-compatible: a newer server can send a SubStatus
      // value this deployed client bundle doesn't know about yet. Fall through
      // to the detectedStatus-based fallback (same as UNSPECIFIED) instead of
      // throwing, so one unrecognized wire value can't crash session rendering.
      // `_exhaustive: never` still gives a compile error if a new case is added
      // to the switch above without also being handled here.
      const _exhaustive: never = session.subStatus;
      console.warn("deriveWorkingState: unrecognized SubStatus value", _exhaustive);
      break;
    }
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
    default: {
      // See the comment on the subStatus switch above: don't throw on a
      // forward-compatible enum value the deployed client doesn't recognize.
      const _exhaustive: never = session.detectedStatus;
      console.warn("deriveWorkingState: unrecognized DetectedStatus value", _exhaustive);
      return WorkingState.UNSPECIFIED;
    }
  }
}
