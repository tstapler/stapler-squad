import { act, renderHook } from "@testing-library/react";
import { Code, ConnectError } from "@connectrpc/connect";
import { NudgeOutcome } from "@/gen/session/v1/github_user_pb";
import { failingPR, fakeNudgeClient } from "@/components/unfinished/prs/prTestFixtures";
import { NUDGE_STATE_TTL_MS, useNudgePR } from "./useNudgePR";

describe("useNudgePR", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("useNudgePR_should_CallRPCWithPRKeyAndSessionIdThenClearNudgedState_When_60sFakeTimerElapses", async () => {
    const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED });
    const { result } = renderHook(() => useNudgePR({ client }));
    expect(result.current.state.status).toBe("idle");

    await act(async () => {
      await result.current.nudge(failingPR(), "fix-ci");
    });
    expect(client.calls).toHaveLength(1);
    expect(client.calls[0].sessionId).toBe("fix-ci");
    expect(client.calls[0].pr).toMatchObject({ host: "github.com", owner: "acme", repo: "api", number: 42 });
    expect(result.current.state).toMatchObject({ status: "done", outcome: NudgeOutcome.DELIVERED });

    act(() => void jest.advanceTimersByTime(NUDGE_STATE_TTL_MS - 1));
    expect(result.current.state.status).toBe("done");
    act(() => void jest.advanceTimersByTime(1));
    expect(result.current.state.status).toBe("idle");
  });

  it("useNudgePR_should_SurfaceTransportError_When_RPCRejects", async () => {
    const client = fakeNudgeClient(
      new ConnectError("GitHub rate limit reached until 10:15", Code.ResourceExhausted),
      new ConnectError("no controller", Code.FailedPrecondition),
      new Error("socket hang up")
    );
    const { result } = renderHook(() => useNudgePR({ client }));
    const kinds: string[] = [];
    for (let i = 0; i < 3; i++) {
      await act(async () => {
        await result.current.nudge(failingPR(), "fix-ci");
      });
      const s = result.current.state;
      kinds.push(s.status === "error" ? `${s.kind}:${s.message}` : s.status);
    }
    expect(kinds).toEqual([
      "rate_limit:GitHub rate limit reached until 10:15",
      "failed_precondition:no controller",
      expect.stringMatching(/^network:Could not send request: .*socket hang up/),
    ]);
  });

  it("useNudgePR_should_KeepMessageUntilNextActionOr60sAndNeverAutoClearAlerts_When_FakeTimersAdvanceAndRefreshHappens", async () => {
    const client = fakeNudgeClient(new ConnectError("limit", Code.ResourceExhausted), { outcome: NudgeOutcome.BUSY });
    const { result, rerender } = renderHook(() => useNudgePR({ client }));

    await act(async () => {
      await result.current.nudge(failingPR(), "fix-ci");
    });
    act(() => void jest.advanceTimersByTime(10 * 60_000));
    rerender(); // a data refresh alone never clears it
    expect(result.current.state.status).toBe("error");

    await act(async () => {
      await result.current.nudge(failingPR(), "fix-ci"); // next action replaces it
    });
    expect(result.current.state).toMatchObject({ status: "done", outcome: NudgeOutcome.BUSY });
    act(() => void jest.advanceTimersByTime(NUDGE_STATE_TTL_MS));
    expect(result.current.state.status).toBe("idle");
  });

  it("useNudgePR_should_StartIdleAfterRemountAndReturnServerDuplicate_When_ReloadedWithin60s", async () => {
    const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED }, { outcome: NudgeOutcome.DUPLICATE });
    const first = renderHook(() => useNudgePR({ client }));
    await act(async () => {
      await first.result.current.nudge(failingPR(), "fix-ci");
    });
    first.unmount();

    const second = renderHook(() => useNudgePR({ client }));
    expect(second.result.current.state.status).toBe("idle");
    await act(async () => {
      await second.result.current.nudge(failingPR(), "fix-ci");
    });
    expect(second.result.current.state).toMatchObject({ status: "done", outcome: NudgeOutcome.DUPLICATE });
  });
});
