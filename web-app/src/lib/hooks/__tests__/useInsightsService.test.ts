/**
 * Tests for useInsightsService utilities.
 *
 * The useInsightsSummary hook depends on ConnectRPC which requires a live
 * transport, so we test the pure utility function (useTopSessions) which
 * has no side-effects.
 *
 * Covers:
 *  - useTopSessions returns sessions sorted by cost descending
 *  - useTopSessions respects the limit parameter
 *  - useTopSessions handles empty array
 *  - useTopSessions default limit of 10
 *  - useTopSessions does not mutate the input array
 */

import { useTopSessions } from "@/lib/hooks/useInsightsService";
import type { SessionTokenSummary } from "@/gen/session/v1/insights_pb";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSession(
  costUsd: number,
  id = Math.random().toString(36).slice(2)
): SessionTokenSummary {
  return {
    sessionId: id,
    conversationId: id,
    projectPath: "/test",
    primaryModel: "claude-sonnet",
    totalInputTokens: BigInt(1000),
    totalOutputTokens: BigInt(500),
    cacheCreationTokens: BigInt(0),
    cacheReadTokens: BigInt(0),
    estimatedCostUsd: costUsd,
    cacheHitRate: 0,
    messageCount: 5,
    firstMessageAt: undefined,
    lastMessageAt: undefined,
    isOrphan: false,
    skillActivations: [],
    topTools: [],
  } as unknown as SessionTokenSummary;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useTopSessions", () => {
  it("useTopSessions_should_sortByCostDesc_When_calledWithSessions", () => {
    const sessions = [
      makeSession(0.01, "a"),
      makeSession(0.05, "b"),
      makeSession(0.02, "c"),
    ];
    const result = useTopSessions(sessions, 10);
    expect(result[0].estimatedCostUsd).toBe(0.05);
    expect(result[1].estimatedCostUsd).toBe(0.02);
    expect(result[2].estimatedCostUsd).toBe(0.01);
  });

  it("useTopSessions_should_respectLimit_When_limitIsSmaller", () => {
    const sessions = [
      makeSession(0.10),
      makeSession(0.05),
      makeSession(0.03),
      makeSession(0.01),
    ];
    const result = useTopSessions(sessions, 2);
    expect(result).toHaveLength(2);
    expect(result[0].estimatedCostUsd).toBe(0.10);
    expect(result[1].estimatedCostUsd).toBe(0.05);
  });

  it("useTopSessions_should_returnEmpty_When_inputIsEmpty", () => {
    const result = useTopSessions([], 10);
    expect(result).toHaveLength(0);
  });

  it("useTopSessions_should_returnAll_When_limitExceedsCount", () => {
    const sessions = [makeSession(0.01), makeSession(0.02)];
    const result = useTopSessions(sessions, 100);
    expect(result).toHaveLength(2);
  });

  it("useTopSessions_should_notMutateInput_When_sorting", () => {
    const sessions = [
      makeSession(0.01, "a"),
      makeSession(0.10, "b"),
      makeSession(0.05, "c"),
    ];
    const originalOrder = sessions.map((s) => s.sessionId);
    useTopSessions(sessions, 10);
    // Input array unchanged
    expect(sessions.map((s) => s.sessionId)).toEqual(originalOrder);
  });

  it("useTopSessions_should_defaultTo10_When_noLimitGiven", () => {
    const sessions = Array.from({ length: 15 }, (_, i) => makeSession(i * 0.01));
    const result = useTopSessions(sessions);
    expect(result).toHaveLength(10);
  });
});
