/**
 * Tests for useLogViewer — focused on the invariant that display entries
 * (`logs`) and their raw proto counterparts (`rawEntries`) stay paired
 * through filtering and live-tail appends, plus that `timeRange`/`limit`
 * options are threaded into the GetLogs request.
 */

import { renderHook, act, waitFor } from "@testing-library/react";
import { useLogViewer } from "../useLogViewer";

type FetchLogs = () => Promise<void>;
let capturedFetchNewLogs: FetchLogs | null = null;

const mockGetLogs = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    getLogs: mockGetLogs,
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: jest.fn(() => "http://localhost:8543"),
}));

// Live-tail polling itself (interval scheduling) is not what this suite is
// about — capture its fetch callback so tests can trigger a poll on demand
// instead of racing real timers.
jest.mock("../useLiveTail", () => ({
  useLiveTail: jest.fn((fetchLogs: FetchLogs) => {
    capturedFetchNewLogs = fetchLogs;
    return [{ isActive: true, isPaused: false, newLogCount: 0, lastFetch: null, error: null }, {}];
  }),
}));

function protoEntry(message: string, level = "INFO") {
  return { message, level, timestamp: undefined };
}

describe("useLogViewer", () => {
  beforeEach(() => {
    mockGetLogs.mockReset();
    capturedFetchNewLogs = null;
  });

  it("pairs each display log with its raw proto entry on initial fetch", async () => {
    mockGetLogs.mockResolvedValueOnce({
      entries: [protoEntry("first"), protoEntry("second", "WARN")],
      totalCount: 2,
    });

    const { result } = renderHook(() => useLogViewer("app"));

    await waitFor(() => expect(result.current.logs).toHaveLength(2));

    expect(result.current.logs.map((l) => l.message)).toEqual(["first", "second"]);
    expect(result.current.rawEntries.map((r) => r.message)).toEqual(["first", "second"]);
    expect(result.current.rawEntries.map((r) => r.level)).toEqual(["INFO", "WARN"]);
  });

  it("keeps logs and rawEntries aligned when a search filter drops entries", async () => {
    mockGetLogs.mockResolvedValueOnce({
      entries: [protoEntry("alpha task done"), protoEntry("beta failure"), protoEntry("gamma task done")],
      totalCount: 3,
    });

    const { result } = renderHook(() => useLogViewer("app"));
    await waitFor(() => expect(result.current.logs).toHaveLength(3));

    act(() => {
      result.current.setSearchQuery("task done");
    });

    await waitFor(() => expect(result.current.logs).toHaveLength(2));
    expect(result.current.rawEntries).toHaveLength(2);
    result.current.logs.forEach((entry, i) => {
      expect(result.current.rawEntries[i].message).toBe(entry.message);
    });
  });

  it("keeps logs and rawEntries aligned when a level filter drops entries", async () => {
    mockGetLogs.mockResolvedValueOnce({
      entries: [protoEntry("info line", "INFO"), protoEntry("warn line", "WARN"), protoEntry("error line", "ERROR")],
      totalCount: 3,
    });

    const { result } = renderHook(() => useLogViewer("app"));
    await waitFor(() => expect(result.current.logs).toHaveLength(3));

    act(() => {
      result.current.setLevelFilters(["ERROR"]);
    });

    await waitFor(() => expect(result.current.logs).toHaveLength(1));
    expect(result.current.rawEntries).toHaveLength(1);
    expect(result.current.rawEntries[0].message).toBe("error line");
    expect(result.current.logs[0].message).toBe("error line");
  });

  it("threads active level filters into a live-tail poll request", async () => {
    mockGetLogs.mockResolvedValueOnce({ entries: [protoEntry("x")], totalCount: 1 });

    const { result } = renderHook(() => useLogViewer("app"));
    await waitFor(() => expect(result.current.logs).toHaveLength(1));

    act(() => {
      result.current.setLevelFilters(["ERROR"]);
    });

    mockGetLogs.mockResolvedValueOnce({ entries: [protoEntry("x")], totalCount: 1 });
    const fetchNewLogs = capturedFetchNewLogs;
    expect(fetchNewLogs).not.toBeNull();
    await act(async () => {
      await fetchNewLogs?.();
    });

    const lastCall = mockGetLogs.mock.calls.at(-1)?.[0];
    expect(lastCall.levels).toEqual(["ERROR"]);
  });

  it("keeps logs and rawEntries aligned after a live-tail poll appends new entries", async () => {
    mockGetLogs.mockResolvedValueOnce({
      entries: [protoEntry("existing")],
      totalCount: 1,
    });

    const { result } = renderHook(() => useLogViewer("app"));
    await waitFor(() => expect(result.current.logs).toHaveLength(1));

    mockGetLogs.mockResolvedValueOnce({
      entries: [protoEntry("newest"), protoEntry("existing")],
      totalCount: 2,
    });

    const fetchNewLogs = capturedFetchNewLogs;
    expect(fetchNewLogs).not.toBeNull();
    await act(async () => {
      await fetchNewLogs?.();
    });

    await waitFor(() => expect(result.current.logs).toHaveLength(2));
    expect(result.current.logs.map((l) => l.message)).toEqual(["existing", "newest"]);
    expect(result.current.rawEntries.map((r) => r.message)).toEqual(["existing", "newest"]);
  });

  it("threads timeRange into the initial GetLogs request", async () => {
    mockGetLogs.mockResolvedValueOnce({ entries: [], totalCount: 0 });

    const start = new Date("2026-01-01T00:00:00.000Z");
    const end = new Date("2026-01-02T00:00:00.000Z");

    renderHook(() => useLogViewer("app", undefined, { timeRange: { start, end } }));

    await waitFor(() => expect(mockGetLogs).toHaveBeenCalledTimes(1));
    const requestArg = mockGetLogs.mock.calls[0][0];
    expect(requestArg.startTime).toBeDefined();
    expect(requestArg.endTime).toBeDefined();
    expect(Number(requestArg.startTime.seconds) * 1000).toBe(start.getTime());
    expect(Number(requestArg.endTime.seconds) * 1000).toBe(end.getTime());
  });

  it("threads a custom limit into the initial GetLogs request", async () => {
    mockGetLogs.mockResolvedValueOnce({ entries: [], totalCount: 0 });

    renderHook(() => useLogViewer("app", undefined, { limit: 50 }));

    await waitFor(() => expect(mockGetLogs).toHaveBeenCalledTimes(1));
    expect(mockGetLogs.mock.calls[0][0].limit).toBe(50);
  });

  it("makes live-tail polling a no-op while a historical timeRange is active", async () => {
    const start = new Date("2026-01-01T00:00:00.000Z");
    const end = new Date("2026-01-02T00:00:00.000Z");
    mockGetLogs.mockResolvedValueOnce({ entries: [protoEntry("one")], totalCount: 1 });

    renderHook(() => useLogViewer("app", undefined, { timeRange: { start, end } }));
    await waitFor(() => expect(mockGetLogs).toHaveBeenCalledTimes(1));

    const fetchNewLogs = capturedFetchNewLogs;
    expect(fetchNewLogs).not.toBeNull();
    await act(async () => {
      await fetchNewLogs?.();
    });

    expect(mockGetLogs).toHaveBeenCalledTimes(1);
  });

  it("re-runs the initial fetch when refresh() is called", async () => {
    mockGetLogs.mockResolvedValueOnce({ entries: [protoEntry("one")], totalCount: 1 });

    const { result } = renderHook(() => useLogViewer("app"));
    await waitFor(() => expect(result.current.logs).toHaveLength(1));

    mockGetLogs.mockResolvedValueOnce({ entries: [protoEntry("replaced")], totalCount: 1 });

    act(() => {
      result.current.refresh();
    });

    await waitFor(() => expect(result.current.logs.map((l) => l.message)).toEqual(["replaced"]));
    expect(result.current.rawEntries.map((r) => r.message)).toEqual(["replaced"]);
  });
});
