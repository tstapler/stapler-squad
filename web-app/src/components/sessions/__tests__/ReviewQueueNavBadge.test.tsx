import React from "react";
import { render, screen } from "@testing-library/react";
import { AttentionReason } from "@/gen/session/v1/types_pb";
import { ReviewQueueNavBadge } from "../ReviewQueueNavBadge";
import { itemsForReasons } from "@/test-utils/attentionFixtures";

let mockItems = itemsForReasons([]);
let mockFlag = true;

jest.mock("@/lib/nav", () => ({
  ...jest.requireActual("@/lib/nav/impl.memory"),
  useRouter: () => ({ push: jest.fn() }),
}));
jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: () => ({ items: mockItems }),
}));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: () => mockFlag,
}));
jest.mock("@/lib/hooks/useReviewQueueNotifications", () => ({
  useReviewQueueNotifications: jest.fn(),
}));

const MIXED = itemsForReasons([
  AttentionReason.PROSE_QUESTION,
  AttentionReason.APPROVAL_PENDING,
  AttentionReason.AWAITING_DIRECTION,
  AttentionReason.AWAITING_DIRECTION,
  AttentionReason.AWAITING_DIRECTION,
]);

describe("ReviewQueueNavBadge count rule", () => {
  it("flag on: shows the needs-human count and names it", () => {
    mockFlag = true;
    mockItems = MIXED;
    render(<ReviewQueueNavBadge />);
    const badge = screen.getByTestId("review-queue-badge");
    expect(badge).toHaveTextContent("2");
    expect(badge).toHaveAttribute("aria-label", "Attention, 2 need you");
  });

  it("flag off: legacy count and label are unchanged", () => {
    mockFlag = false;
    mockItems = MIXED;
    render(<ReviewQueueNavBadge />);
    const badge = screen.getByTestId("review-queue-badge");
    expect(badge).toHaveTextContent("5");
    expect(badge).toHaveAttribute("aria-label", "5 items in review queue");
  });
});
