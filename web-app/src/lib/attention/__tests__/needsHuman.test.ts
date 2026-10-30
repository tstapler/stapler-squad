import { AttentionReason, AttentionReasonSchema } from "@/gen/session/v1/types_pb";
import { attentionCount, classifyReason, isNeedsHuman, type ReasonClass } from "../needsHuman";
import { itemsForReasons } from "@/test-utils/attentionFixtures";

// Mirrors plan.md "Attention Reason Classification"; a new enum value fails here until classified.
const EXPECTED: Record<string, ReasonClass> = {
  UNSPECIFIED: "unspecified",
  APPROVAL_PENDING: "needs-human",
  INPUT_REQUIRED: "needs-human",
  ERROR_STATE: "needs-human",
  IDLE_TIMEOUT: "hidden",
  TASK_COMPLETE: "informational",
  UNCOMMITTED_CHANGES: "informational",
  IDLE: "hidden",
  STALE: "informational",
  WAITING_FOR_USER: "needs-human",
  TESTS_FAILING: "needs-human",
  PROSE_QUESTION: "needs-human",
  AWAITING_DIRECTION: "informational",
};

describe("attention reason classification", () => {
  it("classifies every AttentionReason value", () => {
    const names = AttentionReasonSchema.values.map((v) => v.localName);
    expect(names.sort()).toEqual(Object.keys(EXPECTED).sort());
    for (const v of AttentionReasonSchema.values) {
      expect(classifyReason(v.number as AttentionReason)).toBe(EXPECTED[v.localName]);
    }
  });

  it("counts prose_question but not awaiting_direction", () => {
    expect(isNeedsHuman(AttentionReason.PROSE_QUESTION)).toBe(true);
    expect(isNeedsHuman(AttentionReason.AWAITING_DIRECTION)).toBe(false);
  });
});

describe("attentionCount", () => {
  const items = itemsForReasons([
    AttentionReason.PROSE_QUESTION,
    AttentionReason.APPROVAL_PENDING,
    AttentionReason.AWAITING_DIRECTION,
    AttentionReason.AWAITING_DIRECTION,
    AttentionReason.AWAITING_DIRECTION,
  ]);

  it("flag on: counts only needs-human reasons", () => {
    expect(attentionCount(items, true)).toBe(2);
  });

  it("flag off: legacy count is every item", () => {
    expect(attentionCount(items, false)).toBe(5);
  });
});
