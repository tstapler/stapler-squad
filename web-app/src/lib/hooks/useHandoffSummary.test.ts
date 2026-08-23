/**
 * Tests for useHandoffSummary (plan.md Story 3.1.1).
 *
 * Mirrors useSessionSummary.test.ts's shape: fetch on mount, 2s poll loop
 * while GENERATING/PENDING or `nil`, stopping at READY/ERROR, and trigger()
 * resuming polling.
 */

import { renderHook, act, waitFor } from "@testing-library/react";
import type { HandoffSummaryProto } from "@/gen/session/v1/handoff_summary_pb";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";

const mockGetHandoffSummary = jest.fn();
const mockTriggerHandoffSummary = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getHandoffSummary: (...args: unknown[]) => mockGetHandoffSummary(...args),
    triggerHandoffSummary: (...args: unknown[]) => mockTriggerHandoffSummary(...args),
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

// jest.setup.js globally mocks this module (RestartWithSummaryButton /
// HandoffSummarySection guard, see its comment) so components that embed
// them don't fire real network calls in unrelated tests. This file tests
// the real hook implementation, so it must opt back out of that mock.
jest.unmock("@/lib/hooks/useHandoffSummary");

import { isGenerating, useHandoffSummary } from "@/lib/hooks/useHandoffSummary";

function makeSummary(
  status: HandoffSummaryStatus,
  overrides: Partial<HandoffSummaryProto> = {}
): HandoffSummaryProto {
  return {
    sessionId: "sess-123",
    sessionTitle: "fix-login-redirect",
    status,
    activeTask: "",
    summaryText: "",
    middleMessagesSummarized: 0,
    errorMessage: "",
    errorStage: "",
    ...overrides,
  } as HandoffSummaryProto;
}

describe("useHandoffSummary", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockGetHandoffSummary.mockReset();
    mockTriggerHandoffSummary.mockReset();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("useHandoffSummary_should_fetchOnMount_When_sessionIdProvided", async () => {
    mockGetHandoffSummary.mockResolvedValue({ summary: makeSummary(HandoffSummaryStatus.READY) });

    const { result } = renderHook(() => useHandoffSummary("sess-123"));

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(mockGetHandoffSummary).toHaveBeenCalledTimes(1);
    expect(mockGetHandoffSummary).toHaveBeenCalledWith({ sessionId: "sess-123" });
    expect(result.current.data?.status).toBe(HandoffSummaryStatus.READY);
  });

  it("useHandoffSummary_should_StopPolling_When_StatusBecomesReady", async () => {
    mockGetHandoffSummary
      .mockResolvedValueOnce({ summary: makeSummary(HandoffSummaryStatus.GENERATING) })
      .mockResolvedValueOnce({ summary: makeSummary(HandoffSummaryStatus.READY) });

    const { result } = renderHook(() => useHandoffSummary("sess-123"));

    await waitFor(() => expect(result.current.data?.status).toBe(HandoffSummaryStatus.GENERATING));
    expect(mockGetHandoffSummary).toHaveBeenCalledTimes(1);

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });

    expect(mockGetHandoffSummary).toHaveBeenCalledTimes(2);
    expect(result.current.data?.status).toBe(HandoffSummaryStatus.READY);

    // Polling has stopped — no further calls on additional ticks.
    await act(async () => {
      jest.advanceTimersByTime(4000);
      await Promise.resolve();
    });
    expect(mockGetHandoffSummary).toHaveBeenCalledTimes(2);
  });

  it("useHandoffSummary_should_poll_When_dataIsNull", async () => {
    mockGetHandoffSummary
      .mockResolvedValueOnce({ summary: undefined })
      .mockResolvedValueOnce({ summary: makeSummary(HandoffSummaryStatus.READY) });

    const { result } = renderHook(() => useHandoffSummary("sess-123"));

    await waitFor(() => expect(mockGetHandoffSummary).toHaveBeenCalledTimes(1));
    expect(result.current.data).toBeNull();

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });

    expect(mockGetHandoffSummary).toHaveBeenCalledTimes(2);
    expect(result.current.data?.status).toBe(HandoffSummaryStatus.READY);
  });

  it("useHandoffSummary_should_ResumePollingAfterTrigger_When_NoExistingRow", async () => {
    mockGetHandoffSummary.mockResolvedValue({ summary: undefined });

    const { result } = renderHook(() => useHandoffSummary("sess-123"));

    await waitFor(() => expect(mockGetHandoffSummary).toHaveBeenCalledTimes(1));
    expect(result.current.data).toBeNull();

    mockTriggerHandoffSummary.mockResolvedValue({
      summary: makeSummary(HandoffSummaryStatus.GENERATING),
    });

    await act(async () => {
      await result.current.trigger();
    });

    expect(mockTriggerHandoffSummary).toHaveBeenCalledWith({ sessionId: "sess-123" });
    // trigger() immediately reflects the returned row synchronously after
    // the call resolves.
    expect(result.current.data?.status).toBe(HandoffSummaryStatus.GENERATING);

    mockGetHandoffSummary.mockResolvedValue({ summary: makeSummary(HandoffSummaryStatus.READY) });

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });

    // Polling resumed after trigger() — a fresh GetHandoffSummary call fired.
    expect(mockGetHandoffSummary.mock.calls.length).toBeGreaterThan(1);
    expect(result.current.data?.status).toBe(HandoffSummaryStatus.READY);
  });

  it("useHandoffSummary_should_resumePolling_When_triggerCalledAfterStoppedAtError", async () => {
    mockGetHandoffSummary.mockResolvedValue({
      summary: makeSummary(HandoffSummaryStatus.ERROR, { errorStage: "decisions" }),
    });

    const { result } = renderHook(() => useHandoffSummary("sess-123"));

    await waitFor(() => expect(result.current.data?.status).toBe(HandoffSummaryStatus.ERROR));

    // Polling should have stopped at ERROR — advancing time makes no new calls.
    await act(async () => {
      jest.advanceTimersByTime(4000);
      await Promise.resolve();
    });
    expect(mockGetHandoffSummary).toHaveBeenCalledTimes(1);

    mockTriggerHandoffSummary.mockResolvedValue({
      summary: makeSummary(HandoffSummaryStatus.GENERATING),
    });

    await act(async () => {
      await result.current.trigger();
    });

    expect(result.current.data?.status).toBe(HandoffSummaryStatus.GENERATING);

    mockGetHandoffSummary.mockResolvedValue({ summary: makeSummary(HandoffSummaryStatus.READY) });

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });

    expect(mockGetHandoffSummary.mock.calls.length).toBeGreaterThan(1);
    expect(result.current.data?.status).toBe(HandoffSummaryStatus.READY);
  });

  it("useHandoffSummary_should_setErrorAndStopLoading_When_initialFetchThrows", async () => {
    mockGetHandoffSummary.mockRejectedValue(new Error("Network request failed"));

    const { result } = renderHook(() => useHandoffSummary("sess-123"));

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toBe("Network request failed");
    expect(result.current.data).toBeNull();
  });

  it("useHandoffSummary_should_recoverViaRefetch_When_retryingAfterAFailedInitialFetch", async () => {
    mockGetHandoffSummary.mockRejectedValueOnce(new Error("Network request failed"));

    const { result } = renderHook(() => useHandoffSummary("sess-123"));

    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.data).toBeNull();

    mockGetHandoffSummary.mockResolvedValueOnce({ summary: makeSummary(HandoffSummaryStatus.READY) });

    await act(async () => {
      await result.current.refetch();
    });

    expect(result.current.error).toBeNull();
    expect(result.current.data?.status).toBe(HandoffSummaryStatus.READY);
    expect(mockGetHandoffSummary).toHaveBeenCalledTimes(2);
  });

  it("useHandoffSummary_should_rejectAndSetError_When_triggerFails", async () => {
    mockGetHandoffSummary.mockResolvedValue({
      summary: makeSummary(HandoffSummaryStatus.ERROR, { errorStage: "decisions" }),
    });

    const { result } = renderHook(() => useHandoffSummary("sess-123"));
    await waitFor(() => expect(result.current.data?.status).toBe(HandoffSummaryStatus.ERROR));

    mockTriggerHandoffSummary.mockRejectedValue(new Error("trigger boom"));

    let caughtError: unknown;
    await act(async () => {
      try {
        await result.current.trigger();
      } catch (err) {
        caughtError = err;
      }
    });

    expect(caughtError).toBeInstanceOf(Error);
    expect((caughtError as Error).message).toBe("trigger boom");
    expect(result.current.error?.message).toBe("trigger boom");
  });

  it("useHandoffSummary_should_setNeverResolvedTrue_When_maxPollAttemptsOfNullReadsExceeded", async () => {
    mockGetHandoffSummary.mockResolvedValue({ summary: undefined });

    const { result } = renderHook(() => useHandoffSummary("sess-123"));

    await waitFor(() => expect(mockGetHandoffSummary).toHaveBeenCalledTimes(1));
    expect(result.current.neverResolved).toBe(false);

    for (let i = 0; i < 9; i++) {
      await act(async () => {
        jest.advanceTimersByTime(2000);
        await Promise.resolve();
      });
    }
    expect(mockGetHandoffSummary).toHaveBeenCalledTimes(10);
    expect(result.current.neverResolved).toBe(true);

    // Polling has stopped — no further calls on additional ticks.
    await act(async () => {
      jest.advanceTimersByTime(4000);
      await Promise.resolve();
    });
    expect(mockGetHandoffSummary).toHaveBeenCalledTimes(10);
  });

  it("isGenerating_should_TreatUnspecifiedPendingAndGenerating_AsInFlight", () => {
    expect(isGenerating(HandoffSummaryStatus.UNSPECIFIED)).toBe(true);
    expect(isGenerating(HandoffSummaryStatus.PENDING)).toBe(true);
    expect(isGenerating(HandoffSummaryStatus.GENERATING)).toBe(true);
    expect(isGenerating(HandoffSummaryStatus.READY)).toBe(false);
    expect(isGenerating(HandoffSummaryStatus.ERROR)).toBe(false);
  });
});
