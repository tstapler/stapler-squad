import { AttentionReason } from "@/gen/session/v1/types_pb";
import { isReviewQueueVisible } from "../reviewQueueVisibility";

describe("isReviewQueueVisible", () => {
  it.each([
    AttentionReason.PROSE_QUESTION,
    AttentionReason.AWAITING_DIRECTION,
    AttentionReason.APPROVAL_PENDING,
    AttentionReason.INPUT_REQUIRED,
    AttentionReason.TASK_COMPLETE,
  ])("shows reason %s", (reason) => {
    expect(isReviewQueueVisible({ reason })).toBe(true);
  });

  it("keeps IDLE hidden", () => {
    expect(isReviewQueueVisible({ reason: AttentionReason.IDLE })).toBe(false);
  });
});
