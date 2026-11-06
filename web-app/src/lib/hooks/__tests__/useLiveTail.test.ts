import { act, renderHook } from "@testing-library/react";
import { useLiveTail } from "../useLiveTail";

const setHidden = (hidden: boolean) => {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event("visibilitychange"));
};

const advance = (ms: number) =>
  act(async () => {
    jest.advanceTimersByTime(ms);
  });

beforeEach(() => {
  jest.useFakeTimers();
  setHidden(false);
});
afterEach(() => jest.useRealTimers());

describe("useLiveTail polling", () => {
  it("fetches immediately and then every interval while new entries keep arriving", async () => {
    const fetchLogs = jest.fn().mockResolvedValue(true);
    renderHook(() => useLiveTail(fetchLogs, { enabled: true, interval: 2000 }));
    await advance(0);
    for (let i = 0; i < 5; i++) await advance(2000);
    expect(fetchLogs).toHaveBeenCalledTimes(6);
  });

  it("backs off while polls find nothing: 30s costs at most 5 calls instead of 16", async () => {
    const fetchLogs = jest.fn().mockResolvedValue(false);
    renderHook(() => useLiveTail(fetchLogs, { enabled: true, interval: 2000 }));
    await advance(0);
    for (let i = 0; i < 15; i++) await advance(2000);
    // waits 4s, 8s, then 10s: calls at t=0,4,12,22 (before: 16 calls in 30s)
    expect(fetchLogs.mock.calls.length).toBeLessThanOrEqual(5);
  });

  it("returns to the base interval once new entries arrive", async () => {
    const fetchLogs = jest.fn().mockResolvedValue(false);
    renderHook(() => useLiveTail(fetchLogs, { enabled: true, interval: 2000 }));
    await advance(0);
    await advance(4000);
    await advance(8000);
    fetchLogs.mockResolvedValue(true);
    await advance(10000);
    const afterHit = fetchLogs.mock.calls.length;
    await advance(2000);
    await advance(2000);
    expect(fetchLogs.mock.calls.length).toBe(afterHit + 2);
  });

  it("never starts a second fetch while the previous one is still in flight", async () => {
    let release: () => void = () => {};
    const fetchLogs = jest.fn().mockImplementation(() => new Promise<void>((r) => (release = r)));
    renderHook(() => useLiveTail(fetchLogs, { enabled: true, interval: 2000 }));
    await advance(20000);
    expect(fetchLogs).toHaveBeenCalledTimes(1);
    await act(async () => release());
  });

  it("makes no calls while the tab is hidden and fetches at once when shown", async () => {
    const fetchLogs = jest.fn().mockResolvedValue(true);
    renderHook(() => useLiveTail(fetchLogs, { enabled: true, interval: 2000 }));
    await advance(0);
    await act(async () => setHidden(true));
    const before = fetchLogs.mock.calls.length;
    await advance(60000);
    expect(fetchLogs).toHaveBeenCalledTimes(before);
    await act(async () => setHidden(false));
    expect(fetchLogs).toHaveBeenCalledTimes(before + 1);
  });

  it("does not overlap a fetch when pause then resume lands while one is still pending", async () => {
    let release: () => void = () => {};
    const fetchLogs = jest.fn().mockImplementation(() => new Promise<void>((r) => (release = r)));
    const { result } = renderHook(() => useLiveTail(fetchLogs, { enabled: true, interval: 2000 }));
    await advance(0);
    expect(fetchLogs).toHaveBeenCalledTimes(1);
    await act(async () => result.current[1].pause());
    await act(async () => result.current[1].resume());
    await advance(1000);
    expect(fetchLogs).toHaveBeenCalledTimes(1);
    await act(async () => release());
    await advance(2000);
    expect(fetchLogs.mock.calls.length).toBeLessThanOrEqual(2);
  });

  it("backs off after errors instead of retrying at the base interval", async () => {
    const fetchLogs = jest.fn().mockRejectedValue(new Error("down"));
    renderHook(() => useLiveTail(fetchLogs, { enabled: true, interval: 2000 }));
    await advance(0);
    for (let i = 0; i < 15; i++) await advance(2000);
    expect(fetchLogs.mock.calls.length).toBeLessThanOrEqual(5);
  });
});

