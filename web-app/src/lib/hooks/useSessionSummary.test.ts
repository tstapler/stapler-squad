/**
 * Tests for useSessionSummary (plan.md Story 3.1.1, Task 3.1.1b).
 *
 * Covers: fetch on mount, 2s poll loop while GENERATING/PENDING or `nil`,
 * stopping at READY/ERROR, regenerate() resuming polling, copy() delegating
 * to copyToClipboard, and the maxPollAttempts/neverResolved terminal empty
 * state from design/ux.md surface (g).
 */

import { renderHook, act, waitFor } from "@testing-library/react";
import type { SessionSummaryProto } from "@/gen/session/v1/session_summary_pb";
import { SessionSummaryStatus } from "@/gen/session/v1/types_pb";

const mockGetSessionSummary = jest.fn();
const mockRegenerateSessionSummary = jest.fn();
const mockCopyToClipboard = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getSessionSummary: (...args: unknown[]) => mockGetSessionSummary(...args),
    regenerateSessionSummary: (...args: unknown[]) => mockRegenerateSessionSummary(...args),
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

jest.mock("@/lib/clipboard", () => ({
  copyToClipboard: (...args: unknown[]) => mockCopyToClipboard(...args),
}));

import { isGenerating, useSessionSummary } from "@/lib/hooks/useSessionSummary";

function makeSummary(
  status: SessionSummaryStatus,
  overrides: Partial<SessionSummaryProto> = {}
): SessionSummaryProto {
  return {
    sessionId: "sess-123",
    sessionTitle: "fix-login-redirect",
    status,
    narrative: "",
    narrativeFallbackUsed: false,
    markdown: "# Session Summary",
    errorMessage: "",
    errorStage: "",
    ...overrides,
  } as SessionSummaryProto;
}

describe("useSessionSummary", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockGetSessionSummary.mockReset();
    mockRegenerateSessionSummary.mockReset();
    mockCopyToClipboard.mockReset();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("useSessionSummary_should_fetchOnMount_When_sessionIdProvided", async () => {
    mockGetSessionSummary.mockResolvedValue({ summary: makeSummary(SessionSummaryStatus.READY) });

    const { result } = renderHook(() => useSessionSummary("sess-123"));

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(mockGetSessionSummary).toHaveBeenCalledTimes(1);
    expect(mockGetSessionSummary).toHaveBeenCalledWith({ sessionId: "sess-123" });
    expect(result.current.data?.status).toBe(SessionSummaryStatus.READY);
  });

  it("useSessionSummary_should_pollEvery2s_When_statusGenerating_And_stopAtReady", async () => {
    mockGetSessionSummary
      .mockResolvedValueOnce({ summary: makeSummary(SessionSummaryStatus.GENERATING) })
      .mockResolvedValueOnce({ summary: makeSummary(SessionSummaryStatus.GENERATING) })
      .mockResolvedValueOnce({ summary: makeSummary(SessionSummaryStatus.READY) });

    const { result } = renderHook(() => useSessionSummary("sess-123"));

    await waitFor(() => expect(mockGetSessionSummary).toHaveBeenCalledTimes(1));

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });
    expect(mockGetSessionSummary).toHaveBeenCalledTimes(2);

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });
    expect(mockGetSessionSummary).toHaveBeenCalledTimes(3);
    expect(result.current.data?.status).toBe(SessionSummaryStatus.READY);

    // Polling has stopped — no further calls on additional ticks.
    await act(async () => {
      jest.advanceTimersByTime(4000);
      await Promise.resolve();
    });
    expect(mockGetSessionSummary).toHaveBeenCalledTimes(3);
  });

  it("useSessionSummary_should_poll_When_dataIsNull", async () => {
    mockGetSessionSummary
      .mockResolvedValueOnce({ summary: undefined })
      .mockResolvedValueOnce({ summary: makeSummary(SessionSummaryStatus.READY) });

    const { result } = renderHook(() => useSessionSummary("sess-123"));

    await waitFor(() => expect(mockGetSessionSummary).toHaveBeenCalledTimes(1));
    expect(result.current.data).toBeNull();

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });

    expect(mockGetSessionSummary).toHaveBeenCalledTimes(2);
    expect(result.current.data?.status).toBe(SessionSummaryStatus.READY);
  });

  it("useSessionSummary_should_resumePolling_When_regenerateCalled", async () => {
    mockGetSessionSummary.mockResolvedValue({
      summary: makeSummary(SessionSummaryStatus.ERROR, { errorStage: "decisions" }),
    });

    const { result } = renderHook(() => useSessionSummary("sess-123"));

    await waitFor(() => expect(result.current.data?.status).toBe(SessionSummaryStatus.ERROR));

    // Polling should have stopped at ERROR — advancing time makes no new calls.
    await act(async () => {
      jest.advanceTimersByTime(4000);
      await Promise.resolve();
    });
    expect(mockGetSessionSummary).toHaveBeenCalledTimes(1);

    mockRegenerateSessionSummary.mockResolvedValue({
      summary: makeSummary(SessionSummaryStatus.GENERATING),
    });

    await act(async () => {
      await result.current.regenerate();
    });

    expect(mockRegenerateSessionSummary).toHaveBeenCalledWith({ sessionId: "sess-123" });
    expect(result.current.data?.status).toBe(SessionSummaryStatus.GENERATING);

    mockGetSessionSummary.mockResolvedValue({ summary: makeSummary(SessionSummaryStatus.READY) });

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });

    // Polling resumed after regenerate() — a fresh GetSessionSummary call fired.
    expect(mockGetSessionSummary.mock.calls.length).toBeGreaterThan(1);
    expect(result.current.data?.status).toBe(SessionSummaryStatus.READY);
  });

  it("useSessionSummary_should_delegateToCopyToClipboard_When_copyCalled", async () => {
    mockGetSessionSummary.mockResolvedValue({
      summary: makeSummary(SessionSummaryStatus.READY, { markdown: "# Hello" }),
    });
    mockCopyToClipboard.mockResolvedValue(true);

    const { result } = renderHook(() => useSessionSummary("sess-123"));
    await waitFor(() => expect(result.current.data?.status).toBe(SessionSummaryStatus.READY));

    let copyResult: boolean | undefined;
    await act(async () => {
      copyResult = await result.current.copy();
    });

    expect(mockCopyToClipboard).toHaveBeenCalledWith("# Hello");
    expect(copyResult).toBe(true);
  });

  it("useSessionSummary_should_returnFalse_When_copyToClipboardFails", async () => {
    mockGetSessionSummary.mockResolvedValue({
      summary: makeSummary(SessionSummaryStatus.READY, { markdown: "# Hello" }),
    });
    mockCopyToClipboard.mockResolvedValue(false);

    const { result } = renderHook(() => useSessionSummary("sess-123"));
    await waitFor(() => expect(result.current.data?.status).toBe(SessionSummaryStatus.READY));

    let copyResult: boolean | undefined;
    await act(async () => {
      copyResult = await result.current.copy();
    });

    expect(copyResult).toBe(false);
  });

  it("useSessionSummary_should_setNeverResolvedTrue_When_maxPollAttemptsOfNullReadsExceeded", async () => {
    mockGetSessionSummary.mockResolvedValue({ summary: undefined });

    const { result } = renderHook(() => useSessionSummary("sess-123"));

    // Initial fetch is attempt #1; 9 more ticks reach the 10-attempt cap.
    await waitFor(() => expect(mockGetSessionSummary).toHaveBeenCalledTimes(1));
    expect(result.current.neverResolved).toBe(false);

    for (let i = 0; i < 8; i++) {
      await act(async () => {
        jest.advanceTimersByTime(2000);
        await Promise.resolve();
      });
    }
    expect(mockGetSessionSummary).toHaveBeenCalledTimes(9);
    expect(result.current.neverResolved).toBe(false);

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });
    expect(mockGetSessionSummary).toHaveBeenCalledTimes(10);
    expect(result.current.neverResolved).toBe(true);
    expect(result.current.data).toBeNull();

    // Polling has stopped — no further calls on additional ticks.
    await act(async () => {
      jest.advanceTimersByTime(4000);
      await Promise.resolve();
    });
    expect(mockGetSessionSummary).toHaveBeenCalledTimes(10);
  });

  it("useSessionSummary_should_resetNullAttemptCounter_When_nonNullSummaryObserved", async () => {
    mockGetSessionSummary
      .mockResolvedValueOnce({ summary: undefined })
      .mockResolvedValueOnce({ summary: undefined })
      .mockResolvedValueOnce({ summary: makeSummary(SessionSummaryStatus.GENERATING) })
      .mockResolvedValue({ summary: undefined });

    const { result } = renderHook(() => useSessionSummary("sess-123"));
    await waitFor(() => expect(mockGetSessionSummary).toHaveBeenCalledTimes(1));

    // Two null reads, then a non-null GENERATING read resets the counter.
    for (let i = 0; i < 2; i++) {
      await act(async () => {
        jest.advanceTimersByTime(2000);
        await Promise.resolve();
      });
    }
    expect(result.current.data?.status).toBe(SessionSummaryStatus.GENERATING);
    expect(result.current.neverResolved).toBe(false);

    // Nine more null reads would exceed the cap if the counter hadn't reset —
    // confirm it takes another full 10 to trip neverResolved.
    for (let i = 0; i < 9; i++) {
      await act(async () => {
        jest.advanceTimersByTime(2000);
        await Promise.resolve();
      });
    }
    expect(result.current.neverResolved).toBe(false);

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });
    expect(result.current.neverResolved).toBe(true);
  });

  it("useSessionSummary_should_setErrorAndStopLoading_When_initialFetchThrows", async () => {
    mockGetSessionSummary.mockRejectedValue(new Error("Network request failed"));

    const { result } = renderHook(() => useSessionSummary("sess-123"));

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toBe("Network request failed");
    expect(result.current.data).toBeNull();
  });

  it("useSessionSummary_should_recoverViaRefetch_When_retryingAfterAFailedInitialFetch", async () => {
    mockGetSessionSummary.mockRejectedValueOnce(new Error("Network request failed"));

    const { result } = renderHook(() => useSessionSummary("sess-123"));

    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.data).toBeNull();

    mockGetSessionSummary.mockResolvedValueOnce({ summary: makeSummary(SessionSummaryStatus.READY) });

    await act(async () => {
      await result.current.refetch();
    });

    expect(result.current.error).toBeNull();
    expect(result.current.data?.status).toBe(SessionSummaryStatus.READY);
    expect(mockGetSessionSummary).toHaveBeenCalledTimes(2);
  });

  it("useSessionSummary_should_treatUnspecifiedStatusAsGenerating_When_polling", async () => {
    // Bug 3 regression guard: UNSPECIFIED is proto3's zero value, not a
    // terminal state — a summary row read back with this status before
    // isGenerating() is set for the first time must keep polling rather
    // than being (wrongly) treated as "done."
    mockGetSessionSummary
      .mockResolvedValueOnce({ summary: makeSummary(SessionSummaryStatus.UNSPECIFIED) })
      .mockResolvedValueOnce({ summary: makeSummary(SessionSummaryStatus.READY) });

    const { result } = renderHook(() => useSessionSummary("sess-123"));

    await waitFor(() => expect(result.current.data?.status).toBe(SessionSummaryStatus.UNSPECIFIED));
    expect(mockGetSessionSummary).toHaveBeenCalledTimes(1);

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });

    expect(mockGetSessionSummary).toHaveBeenCalledTimes(2);
    expect(result.current.data?.status).toBe(SessionSummaryStatus.READY);
  });

  it("isGenerating_should_beExportedAsCanonicalDefinition_treatingUnspecifiedPendingAndGeneratingAsInFlight", () => {
    expect(isGenerating(SessionSummaryStatus.UNSPECIFIED)).toBe(true);
    expect(isGenerating(SessionSummaryStatus.PENDING)).toBe(true);
    expect(isGenerating(SessionSummaryStatus.GENERATING)).toBe(true);
    expect(isGenerating(SessionSummaryStatus.READY)).toBe(false);
    expect(isGenerating(SessionSummaryStatus.ERROR)).toBe(false);
  });
});
