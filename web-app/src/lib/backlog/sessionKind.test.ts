import { classifySessionKind } from "./sessionKind";
import type { LinkedSession } from "@/lib/hooks/useBacklogService";

function makeSession(overrides: Partial<LinkedSession> & Pick<LinkedSession, "role" | "sessionId">): LinkedSession {
  return {
    entityId: "entity-1",
    estimatedCostUsd: 0,
    ...overrides,
  };
}

describe("classifySessionKind", () => {
  it("classifySessionKind_should_ReturnHeadlessDiagnostic_When_SessionIdStartsWithHeadlessTriage", () => {
    const session = makeSession({ role: "triage", sessionId: "headless-triage-a1b2c3d4" });
    expect(classifySessionKind(session)).toBe("headless_diagnostic");
  });

  it("classifies a headless re-review session (role review, headless- prefix) as headless_diagnostic", () => {
    const session = makeSession({ role: "review", sessionId: "headless-re-review-a1b2c3d4" });
    expect(classifySessionKind(session)).toBe("headless_diagnostic");
  });

  it("classifies a role-triage session with no synthetic prefix as headless_diagnostic (role wins)", () => {
    const session = makeSession({ role: "triage", sessionId: "some-uuid-without-prefix" });
    expect(classifySessionKind(session)).toBe("headless_diagnostic");
  });

  it("classifies a review-blocked- prefixed session as blocked_guardrail", () => {
    const session = makeSession({ role: "review", sessionId: "review-blocked-a1b2c3d4" });
    expect(classifySessionKind(session)).toBe("blocked_guardrail");
  });

  it("classifies a diff-error- prefixed session as blocked_guardrail", () => {
    const session = makeSession({ role: "review", sessionId: "diff-error-a1b2c3d4" });
    expect(classifySessionKind(session)).toBe("blocked_guardrail");
  });

  it("classifySessionKind_should_ReturnManualReviewMarker_NotReview_When_SessionIdStartsWithManualReview", () => {
    const session = makeSession({
      role: "review",
      sessionId: "manual-review-a1b2c3d4-1721577600000000000",
    });
    expect(classifySessionKind(session)).toBe("manual_review_marker");
    expect(classifySessionKind(session)).not.toBe("review");
  });

  it("classifies a role-review session with no synthetic prefix as review", () => {
    const session = makeSession({ role: "review", sessionId: "a1b2c3d4-e5f6-7890" });
    expect(classifySessionKind(session)).toBe("review");
  });

  it("classifySessionKind_should_ReturnWork_When_SessionIdIsNormalUuidAndRoleIsWork", () => {
    const session = makeSession({ role: "work", sessionId: "a1b2c3d4-e5f6-7890-abcd-1234567890ab" });
    expect(classifySessionKind(session)).toBe("work");
  });
});
