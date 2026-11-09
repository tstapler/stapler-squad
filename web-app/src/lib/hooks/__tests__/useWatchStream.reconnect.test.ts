import { renderHook, act } from "@testing-library/react";
import { backoffDelayMs, useWatchStream } from "../useWatchStream";

describe("backoffDelayMs", () => {
  it("grows exponentially, is capped at 30s, and jitters within [base/2, base]", () => {
    expect(backoffDelayMs(0, () => 1)).toBe(1000);
    expect(backoffDelayMs(0, () => 0)).toBe(500);
    expect(backoffDelayMs(3, () => 1)).toBe(8000);
    expect(backoffDelayMs(3, () => 1)).toBeGreaterThan(backoffDelayMs(1, () => 1));
    expect(backoffDelayMs(20, () => 1)).toBe(30_000);
    expect(backoffDelayMs(20, () => 0)).toBe(15_000);
  });
});

describe("useWatchStream reconnect", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("a forced reconnect cancels the pending backoff timer instead of opening a second stream", async () => {
    const signals: AbortSignal[] = [];
    const subscribe = jest.fn((_seq: bigint, signal: AbortSignal) => {
      signals.push(signal);
      // eslint-disable-next-line require-yield
      return (async function* () {
        throw new Error("boom");
      })();
    });
    jest.spyOn(console, "error").mockImplementation(() => {});

    const { result, unmount } = renderHook(() =>
      useWatchStream<{ hb: boolean }>({
        subscribe,
        onEvent: () => {},
        isHeartbeat: (e) => e.hb,
        enableVisibilityWatchdog: false,
      })
    );
    await act(async () => {
      await jest.advanceTimersByTimeAsync(0);
    });
    expect(subscribe).toHaveBeenCalledTimes(1); // failed; backoff timer now pending

    await act(async () => {
      result.current.reconnect();
      await jest.advanceTimersByTimeAsync(0);
    });
    expect(subscribe).toHaveBeenCalledTimes(2);
    expect(signals[0].aborted).toBe(true); // close-before-reopen

    // Past the first attempt's backoff window; its timer must not fire a third attempt.
    // (The second attempt also fails, so allow its own, single, scheduled retry.)
    await act(async () => {
      await jest.advanceTimersByTimeAsync(1000);
    });
    expect(subscribe).toHaveBeenCalledTimes(3);
    unmount();
  });

  it("stops retrying after unmount", async () => {
    const subscribe = jest.fn(() =>
      // eslint-disable-next-line require-yield
      (async function* () {
        throw new Error("boom");
      })()
    );
    jest.spyOn(console, "error").mockImplementation(() => {});
    const { unmount } = renderHook(() =>
      useWatchStream<{ hb: boolean }>({
        subscribe,
        onEvent: () => {},
        isHeartbeat: (e) => e.hb,
        enableVisibilityWatchdog: false,
      })
    );
    await act(async () => {
      await jest.advanceTimersByTimeAsync(0);
    });
    unmount();
    await act(async () => {
      await jest.advanceTimersByTimeAsync(60_000);
    });
    expect(subscribe).toHaveBeenCalledTimes(1);
  });
});
