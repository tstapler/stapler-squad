import { act, renderHook } from "@testing-library/react";
import { useBackgroundActivity } from "./useBackgroundActivity";
import { makeElevenStopped, makeHiddenSession } from "@/components/ui/__fixtures__/notificationFixtures";
import type { HiddenSessionSummary } from "@/lib/utils/backgroundActivity";

let mockPoll: { sessions: HiddenSessionSummary[]; lastUpdatedAt: number | null };
jest.mock("./useBackgroundSessions", () => ({
  useBackgroundSessions: () => ({
    sessions: mockPoll.sessions,
    departed: [],
    loading: false,
    failed: false,
    lastUpdatedAt: mockPoll.lastUpdatedAt,
    refresh: () => {},
  }),
}));

describe("useBackgroundActivity", () => {
  it("useBackgroundActivity_should_deriveRowsAndRollupFromPollState_When_elevenSessions", () => {
    mockPoll = { sessions: makeElevenStopped("completed"), lastUpdatedAt: 1 };
    const { result } = renderHook(() => useBackgroundActivity(true, []));
    expect(result.current.rows).toHaveLength(11);
    expect(result.current.rollup).toMatchObject({ variant: "no_failures_reported", total: 11 });
    expect(result.current.exceptionCount).toBe(0);
    expect(result.current.hasHiddenSessions).toBe(true);
  });

  it("useBackgroundActivity_should_keepDepartedRowForOnePollCycle_When_sessionDisappears", () => {
    const keep = makeHiddenSession({ id: "review:keep" });
    mockPoll = { sessions: [keep, makeHiddenSession({ id: "review:old" })], lastUpdatedAt: 1 };
    const history: never[] = [];
    const { result, rerender } = renderHook(() => useBackgroundActivity(true, history));

    mockPoll = { sessions: [keep], lastUpdatedAt: 2 };
    rerender();
    expect(result.current.rows.map((r) => r.state)).toEqual(["stopped_unknown", "departed"]);

    mockPoll = { sessions: [keep], lastUpdatedAt: 3 };
    act(() => rerender());
    expect(result.current.rows.map((r) => r.sessionId)).toEqual(["review:keep"]);
  });
});
