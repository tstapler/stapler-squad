import { renderHook, act } from "@testing-library/react";
import { ScrollBlockedReason } from "@/gen/session/v1/events_pb";
import { DEFAULT_TOAST_MS } from "@/lib/notification-policy";
import { useScrollLoadingPill } from "../useScrollLoadingPill";
import { useBlockedToast } from "../useBlockedToast";

beforeEach(() => jest.useFakeTimers());
afterEach(() => jest.useRealTimers());

describe("useScrollLoadingPill", () => {
  it("shows after 150ms, then flags stalled after 8s, carrying the program", () => {
    const { result } = renderHook(() => useScrollLoadingPill());
    act(() => result.current.startScrollLoadingTimers("claude"));

    expect(result.current.pillState.visible).toBe(false);
    act(() => { jest.advanceTimersByTime(150); });
    expect(result.current.pillState).toEqual({ visible: true, stalled: false, program: "claude" });
    act(() => { jest.advanceTimersByTime(7850); });
    expect(result.current.pillState).toEqual({ visible: true, stalled: true, program: "claude" });
  });

  it("clearScrollLoadingState hides the pill and cancels pending timers", () => {
    const { result } = renderHook(() => useScrollLoadingPill());
    act(() => result.current.startScrollLoadingTimers());
    act(() => result.current.clearScrollLoadingState());
    act(() => { jest.advanceTimersByTime(10000); });

    expect(result.current.pillState).toEqual({ visible: false, stalled: false });
  });

  it("restarting replaces the previous timers rather than stacking them", () => {
    const { result } = renderHook(() => useScrollLoadingPill());
    act(() => result.current.startScrollLoadingTimers("a"));
    act(() => { jest.advanceTimersByTime(100); });
    act(() => result.current.startScrollLoadingTimers("b"));
    act(() => { jest.advanceTimersByTime(150); });

    expect(result.current.pillState.program).toBe("b");
  });

  it("leaves no timers behind on unmount", () => {
    const { result, unmount } = renderHook(() => useScrollLoadingPill());
    act(() => result.current.startScrollLoadingTimers());
    unmount();
    expect(jest.getTimerCount()).toBe(0);
  });
});

describe("useBlockedToast", () => {
  const blocked = (reason: ScrollBlockedReason) => ({ blockedReason: reason, program: "claude" });

  it("shows a toast with an increasing seq and auto-dismisses", () => {
    const { result } = renderHook(() => useBlockedToast());
    act(() => result.current.showBlockedOutcome(blocked(ScrollBlockedReason.LEASE_CONTENTION)));
    expect(result.current.blockedToast).toMatchObject({ reason: ScrollBlockedReason.LEASE_CONTENTION, seq: 1 });

    act(() => result.current.showBlockedOutcome(blocked(ScrollBlockedReason.LEASE_CONTENTION)));
    expect(result.current.blockedToast?.seq).toBe(2);

    act(() => { jest.advanceTimersByTime(DEFAULT_TOAST_MS); });
    expect(result.current.blockedToast).toBeNull();
  });

  it("pulses the connection indicator only for viewer-related reasons", () => {
    const { result } = renderHook(() => useBlockedToast());

    act(() => result.current.showBlockedOutcome(blocked(ScrollBlockedReason.LEASE_CONTENTION)));
    expect(result.current.connectionPulse).toBe(false);

    act(() => result.current.showBlockedOutcome(blocked(ScrollBlockedReason.MULTIPLE_VIEWERS)));
    expect(result.current.connectionPulse).toBe(true);
    act(() => { jest.advanceTimersByTime(600); });
    expect(result.current.connectionPulse).toBe(false);
  });

  it("dismissBlockedToast clears the toast immediately", () => {
    const { result } = renderHook(() => useBlockedToast());
    act(() => result.current.showBlockedOutcome(blocked(ScrollBlockedReason.MULTIPLE_VIEWERS)));
    act(() => result.current.dismissBlockedToast());
    expect(result.current.blockedToast).toBeNull();
  });

  it("leaves no timers behind on unmount", () => {
    const { result, unmount } = renderHook(() => useBlockedToast());
    act(() => result.current.showBlockedOutcome(blocked(ScrollBlockedReason.MULTIPLE_VIEWERS)));
    unmount();
    expect(jest.getTimerCount()).toBe(0);
  });
});
