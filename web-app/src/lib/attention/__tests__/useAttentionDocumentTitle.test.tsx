import React from "react";
import { render, act } from "@testing-library/react";
import { AttentionReason } from "@/gen/session/v1/types_pb";
import { AttentionDocumentTitle } from "@/components/sessions/AttentionDocumentTitle";
import { itemsForReasons } from "@/test-utils/attentionFixtures";

let mockItems = itemsForReasons([]);
let mockFlag = true;

jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: () => ({ items: mockItems }),
}));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: () => mockFlag,
}));

const MIXED = [
  AttentionReason.PROSE_QUESTION,
  AttentionReason.APPROVAL_PENDING,
  AttentionReason.AWAITING_DIRECTION,
  AttentionReason.AWAITING_DIRECTION,
  AttentionReason.AWAITING_DIRECTION,
];

describe("AttentionDocumentTitle", () => {
  beforeEach(() => {
    document.head.innerHTML = "<title>Stapler Squad</title>";
    document.title = "Stapler Squad";
    mockFlag = true;
  });

  it("prefixes the needs-human count, ignoring awaiting_direction", () => {
    mockItems = itemsForReasons(MIXED);
    render(<AttentionDocumentTitle />);
    expect(document.title).toBe("(2) Stapler Squad");
  });

  it("re-applies the prefix after the title is rewritten", async () => {
    mockItems = itemsForReasons(MIXED);
    render(<AttentionDocumentTitle />);
    await act(async () => {
      document.title = "Attention";
      await Promise.resolve();
    });
    expect(document.title).toBe("(2) Attention");
  });

  it("leaves the title alone with the flag off", () => {
    mockFlag = false;
    mockItems = itemsForReasons(MIXED);
    render(<AttentionDocumentTitle />);
    expect(document.title).toBe("Stapler Squad");
  });

  it("shows no prefix at zero", () => {
    mockItems = itemsForReasons([AttentionReason.AWAITING_DIRECTION]);
    render(<AttentionDocumentTitle />);
    expect(document.title).toBe("Stapler Squad");
  });
});
