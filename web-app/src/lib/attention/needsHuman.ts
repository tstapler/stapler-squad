import { AttentionReason, type ReviewItem } from "@/gen/session/v1/types_pb";
import { assertNever } from "@/lib/utils/assertNever";

export type ReasonClass = "needs-human" | "informational" | "hidden" | "unspecified";

/** plan.md "Attention Reason Classification": the single source for the badge, tab badge and title. */
export function classifyReason(reason: AttentionReason): ReasonClass {
  switch (reason) {
    case AttentionReason.APPROVAL_PENDING:
    case AttentionReason.INPUT_REQUIRED:
    case AttentionReason.ERROR_STATE:
    case AttentionReason.PROSE_QUESTION:
    case AttentionReason.TESTS_FAILING:
    case AttentionReason.WAITING_FOR_USER:
      return "needs-human";
    case AttentionReason.AWAITING_DIRECTION:
    case AttentionReason.TASK_COMPLETE:
    case AttentionReason.UNCOMMITTED_CHANGES:
    case AttentionReason.STALE:
      return "informational";
    case AttentionReason.IDLE:
    case AttentionReason.IDLE_TIMEOUT:
      return "hidden";
    case AttentionReason.UNSPECIFIED:
      return "unspecified";
    default:
      return assertNever(reason);
  }
}

export function isNeedsHuman(reason: AttentionReason): boolean {
  return classifyReason(reason) === "needs-human";
}

/**
 * The one count behind the nav badge, Sessions-tab badge and document title. With
 * `needsHumanOnly` off (the `attention_prose_question_detection` flag) it is the legacy item count.
 */
export function attentionCount(items: readonly Pick<ReviewItem, "reason">[], needsHumanOnly: boolean): number {
  return needsHumanOnly ? items.filter((i) => isNeedsHuman(i.reason)).length : items.length;
}
