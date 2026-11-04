/**
 * Only event-driven and user-requested refreshes set waitForFresh; the interval poll must not, or the
 * server's stale-while-revalidate answer is never used for it.
 */
import { renderHook, act } from "@testing-library/react";

const mockGetVCSStatus = jest.fn();
const mockGetSessionDiff = jest.fn().mockResolvedValue({ diffStats: null });
let mockSessionUpdatedAt: number | undefined = 1;

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getVCSStatus: mockGetVCSStatus, getSessionDiff: mockGetSessionDiff }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@/lib/store", () => ({ useAppSelector: () => mockSessionUpdatedAt }));
jest.mock("@/lib/store/sessionsSlice", () => ({ selectAllSessions: () => [] }));

import { resetVcsCacheForTests } from "@/lib/hooks/useVcsStatus";
import { useSessionVcs } from "@/lib/hooks/useSessionVcs";

const advance = (ms: number) => act(async () => { await jest.advanceTimersByTimeAsync(ms); });
const waitForFreshOfCall = (i: number) => mockGetVCSStatus.mock.calls[i][0].waitForFresh;

describe("useSessionVcs waitForFresh", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    resetVcsCacheForTests();
    mockGetVCSStatus.mockReset();
    mockGetVCSStatus.mockResolvedValue({ vcsStatus: { branch: "main" }, error: "" });
    mockSessionUpdatedAt = 1;
  });
  afterEach(() => jest.useRealTimers());

  it("useSessionVcs_should_NotWaitForFresh_When_IntervalPollFires", async () => {
    renderHook(() => useSessionVcs("w1", "", "fast"));
    await advance(60_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(2); // mount + interval
    expect(waitForFreshOfCall(0)).toBe(false);
    expect(waitForFreshOfCall(1)).toBe(false);
  });

  it("useSessionVcs_should_WaitForFresh_When_SessionUpdatedEventRefetches", async () => {
    const { rerender } = renderHook(() => useSessionVcs("w2", "", "fast"));
    await advance(1_000);
    mockSessionUpdatedAt = 2;
    rerender();
    await advance(1_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(2);
    expect(waitForFreshOfCall(1)).toBe(true);
  });

  it("useSessionVcs_should_WaitForFresh_When_UserRefreshes", async () => {
    const { result } = renderHook(() => useSessionVcs("w3", "", "fast"));
    await advance(1_000);
    act(() => result.current.refreshStatus());
    await advance(1_000);
    expect(waitForFreshOfCall(mockGetVCSStatus.mock.calls.length - 1)).toBe(true);
  });
});
