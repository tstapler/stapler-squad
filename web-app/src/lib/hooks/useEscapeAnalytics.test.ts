/**
 * Tests for useEscapeAnalyticsSummary (plan.md Story 2.1 / Story 2.6, Task 2.6.1).
 *
 * Covers the `cancelled`-flag guard backported from useEscapeEvents (no setState
 * after the effect re-runs on a session change before the in-flight fetch
 * resolves) and the new `enabled: boolean = true` gating parameter.
 */

import { renderHook, act, waitFor } from "@testing-library/react";

const mockGetEscapeAnalyticsSummary = jest.fn();
const mockGetEscapeAnalyticsGlobalSummary = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getEscapeAnalyticsSummary: (...args: unknown[]) => mockGetEscapeAnalyticsSummary(...args),
    getEscapeAnalyticsGlobalSummary: (...args: unknown[]) => mockGetEscapeAnalyticsGlobalSummary(...args),
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

import { useEscapeAnalyticsSummary, useEscapeAnalyticsGlobalSummary } from "@/lib/hooks/useEscapeAnalytics";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function makeSummaryResponse(overrides: Record<string, unknown> = {}) {
  return {
    histogram: [],
    totalSequences: 5n,
    totalMangled: 1n,
    mangleRate: 0.2,
    ...overrides,
  };
}

describe("useEscapeAnalyticsSummary", () => {
  beforeEach(() => {
    mockGetEscapeAnalyticsSummary.mockReset();
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it("useEscapeAnalyticsSummary_should_IgnoreStaleResponse_When_SessionChangesBeforeFetchResolves", async () => {
    const first = deferred<ReturnType<typeof makeSummaryResponse>>();
    const second = deferred<ReturnType<typeof makeSummaryResponse>>();

    mockGetEscapeAnalyticsSummary
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);

    const { result, rerender } = renderHook(
      ({ sessionId }) => useEscapeAnalyticsSummary(sessionId),
      { initialProps: { sessionId: "sess-1" } }
    );

    await waitFor(() => expect(mockGetEscapeAnalyticsSummary).toHaveBeenCalledTimes(1), { timeout: 5000 });
    expect(result.current.loading).toBe(true);

    // Switch sessions before the first fetch resolves — this unmounts the
    // first effect (setting its `cancelled` flag) and starts a second fetch.
    rerender({ sessionId: "sess-2" });
    await waitFor(() => expect(mockGetEscapeAnalyticsSummary).toHaveBeenCalledTimes(2), { timeout: 5000 });

    // Resolve the stale (session "sess-1") request now. If the cancellation
    // guard is missing, this would overwrite state with sess-1's data.
    await act(async () => {
      first.resolve(makeSummaryResponse({ totalSequences: 999n, totalMangled: 999n }));
      await Promise.resolve();
    });

    expect(result.current.totalSequences).not.toBe(999n);
    expect(result.current.loading).toBe(true);

    // Now resolve the current (sess-2) request — its data should land.
    await act(async () => {
      second.resolve(makeSummaryResponse({ totalSequences: 7n, totalMangled: 2n, mangleRate: 0.28 }));
      await Promise.resolve();
    });

    await waitFor(() => expect(result.current.loading).toBe(false), { timeout: 5000 });
    expect(result.current.totalSequences).toBe(7n);
    expect(result.current.totalMangled).toBe(2n);
  });

  it("useEscapeAnalyticsSummary_should_NotFetch_When_Disabled", async () => {
    mockGetEscapeAnalyticsSummary.mockResolvedValue(makeSummaryResponse());

    const { result } = renderHook(() => useEscapeAnalyticsSummary("sess-1", false));

    // Give any (incorrect) effect a chance to fire.
    await act(async () => {
      await Promise.resolve();
    });

    expect(mockGetEscapeAnalyticsSummary).not.toHaveBeenCalled();
    expect(result.current.loading).toBe(false);

    // refresh() should also be a no-op while disabled.
    await act(async () => {
      await result.current.refresh();
    });

    expect(mockGetEscapeAnalyticsSummary).not.toHaveBeenCalled();
  });

  it("useEscapeAnalyticsSummary_should_Fetch_When_Enabled", async () => {
    mockGetEscapeAnalyticsSummary.mockResolvedValue(makeSummaryResponse());

    const { result } = renderHook(() => useEscapeAnalyticsSummary("sess-1", true));

    await waitFor(() => expect(result.current.loading).toBe(false), { timeout: 5000 });

    expect(mockGetEscapeAnalyticsSummary).toHaveBeenCalledTimes(1);
    expect(mockGetEscapeAnalyticsSummary).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "sess-1" })
    );
    expect(result.current.totalSequences).toBe(5n);
  });
});

function makeGlobalSummaryResponse(overrides: Record<string, unknown> = {}) {
  return {
    histogram: [],
    totalSequences: 5n,
    totalMangled: 1n,
    mangleRate: 0.2,
    perSession: [],
    ...overrides,
  };
}

describe("useEscapeAnalyticsGlobalSummary", () => {
  beforeEach(() => {
    mockGetEscapeAnalyticsGlobalSummary.mockReset();
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it("useEscapeAnalyticsGlobalSummary_should_Fetch_When_Enabled", async () => {
    mockGetEscapeAnalyticsGlobalSummary.mockResolvedValue(
      makeGlobalSummaryResponse({
        perSession: [{ sessionId: "sess-1", totalSequences: 5n, totalMangled: 1n, mangleRate: 0.2 }],
      })
    );

    const { result } = renderHook(() => useEscapeAnalyticsGlobalSummary(true));

    await waitFor(() => expect(result.current.loading).toBe(false), { timeout: 5000 });

    expect(mockGetEscapeAnalyticsGlobalSummary).toHaveBeenCalledTimes(1);
    expect(result.current.totalSequences).toBe(5n);
    expect(result.current.totalMangled).toBe(1n);
    expect(result.current.mangleRate).toBe(0.2);
    expect(result.current.perSession).toHaveLength(1);
    expect(result.current.perSession[0]).toEqual(
      expect.objectContaining({ sessionId: "sess-1" })
    );
  });

  it("useEscapeAnalyticsGlobalSummary_should_NotFetch_When_Disabled", async () => {
    mockGetEscapeAnalyticsGlobalSummary.mockResolvedValue(makeGlobalSummaryResponse());

    const { result } = renderHook(() => useEscapeAnalyticsGlobalSummary(false));

    // Give any (incorrect) effect a chance to fire.
    await act(async () => {
      await Promise.resolve();
    });

    expect(mockGetEscapeAnalyticsGlobalSummary).not.toHaveBeenCalled();
    expect(result.current.loading).toBe(false);

    // refresh() should also be a no-op while disabled.
    await act(async () => {
      await result.current.refresh();
    });

    expect(mockGetEscapeAnalyticsGlobalSummary).not.toHaveBeenCalled();
  });

  it("useEscapeAnalyticsGlobalSummary_should_IgnoreStaleResponse_When_DisabledBeforeFetchResolves", async () => {
    const first = deferred<ReturnType<typeof makeGlobalSummaryResponse>>();
    mockGetEscapeAnalyticsGlobalSummary.mockImplementationOnce(() => first.promise);

    const { result, rerender } = renderHook(
      ({ enabled }) => useEscapeAnalyticsGlobalSummary(enabled),
      { initialProps: { enabled: true } }
    );

    await waitFor(() => expect(mockGetEscapeAnalyticsGlobalSummary).toHaveBeenCalledTimes(1), { timeout: 5000 });
    expect(result.current.loading).toBe(true);

    // Disable the hook before the in-flight fetch resolves — this unmounts
    // the effect (setting its `cancelled` flag).
    rerender({ enabled: false });

    // Resolve the now-stale request. If the cancellation guard is missing,
    // this would overwrite state after the hook was disabled.
    await act(async () => {
      first.resolve(makeGlobalSummaryResponse({ totalSequences: 999n, totalMangled: 999n }));
      await Promise.resolve();
    });

    expect(result.current.totalSequences).not.toBe(999n);
    expect(result.current.totalMangled).not.toBe(999n);
    expect(mockGetEscapeAnalyticsGlobalSummary).toHaveBeenCalledTimes(1);
  });
});
