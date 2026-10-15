import { renderHook } from "@testing-library/react";
import { createToastTimerRegistry, useToastTimers } from "@/lib/hooks/useToastTimers";

describe("createToastTimerRegistry", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("toast_timers_should_register_pause_resume_cancel_and_cancelAll_when_driven", () => {
    const registry = createToastTimerRegistry();
    const closed = jest.fn();
    const minimized = jest.fn();

    registry.register("a", "close", 5_000, closed);
    registry.register("a", "minimize", 3_000, minimized);
    expect(registry.pendingCount()).toBe(2);

    jest.advanceTimersByTime(3_000);
    expect(minimized).toHaveBeenCalledTimes(1);
    expect(closed).not.toHaveBeenCalled();
    expect(registry.has("a", "minimize")).toBe(false);

    jest.advanceTimersByTime(2_000);
    expect(closed).toHaveBeenCalledTimes(1);
    expect(registry.pendingCount()).toBe(0);

    registry.register("b", "close", 1_000, closed);
    registry.cancel("b");
    jest.advanceTimersByTime(5_000);
    expect(closed).toHaveBeenCalledTimes(1);

    registry.register("c", "close", 1_000, closed);
    registry.register("d", "close", 1_000, closed);
    registry.cancelAll();
    jest.advanceTimersByTime(5_000);
    expect(closed).toHaveBeenCalledTimes(1);
    expect(registry.pendingCount()).toBe(0);
  });

  it("preserves the remaining time across a pause", () => {
    const registry = createToastTimerRegistry();
    const fire = jest.fn();
    registry.register("a", "close", 5_000, fire);

    jest.advanceTimersByTime(3_000);
    registry.pause("a", "hover");
    jest.advanceTimersByTime(7_000); // pointer leaves at t=10s
    expect(fire).not.toHaveBeenCalled();

    registry.resume("a", "hover");
    jest.advanceTimersByTime(1_999);
    expect(fire).not.toHaveBeenCalled();
    jest.advanceTimersByTime(1);
    expect(fire).toHaveBeenCalledTimes(1);
  });

  it("stays paused until every reason for the toast is released", () => {
    const registry = createToastTimerRegistry();
    const fire = jest.fn();
    registry.register("a", "close", 1_000, fire);

    registry.pause("a", "hover");
    registry.pause("a", "focus");
    registry.resume("a", "hover");
    jest.advanceTimersByTime(5_000);
    expect(fire).not.toHaveBeenCalled();

    registry.resume("a", "focus");
    jest.advanceTimersByTime(1_000);
    expect(fire).toHaveBeenCalledTimes(1);
  });

  it("holds every timer, including ones registered later, while a global hold is active", () => {
    const registry = createToastTimerRegistry();
    const first = jest.fn();
    const second = jest.fn();

    registry.register("a", "close", 1_000, first);
    registry.hold("tray-open");
    registry.register("b", "close", 1_000, second);
    jest.advanceTimersByTime(60_000);
    expect(first).not.toHaveBeenCalled();
    expect(second).not.toHaveBeenCalled();

    registry.release("tray-open");
    jest.advanceTimersByTime(1_000);
    expect(first).toHaveBeenCalledTimes(1);
    expect(second).toHaveBeenCalledTimes(1);
  });

  it("replaces a timer registered again with the same id and kind", () => {
    const registry = createToastTimerRegistry();
    const stale = jest.fn();
    const fresh = jest.fn();

    registry.register("a", "close", 1_000, stale);
    registry.register("a", "close", 2_000, fresh);
    jest.advanceTimersByTime(2_000);

    expect(stale).not.toHaveBeenCalled();
    expect(fresh).toHaveBeenCalledTimes(1);
  });
});

describe("useToastTimers", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("returns one stable registry and cancels everything on unmount", () => {
    const fire = jest.fn();
    const { result, rerender, unmount } = renderHook(() => useToastTimers());
    const first = result.current;

    first.register("a", "close", 1_000, fire);
    rerender();
    expect(result.current).toBe(first);

    unmount();
    expect(first.pendingCount()).toBe(0);
    jest.advanceTimersByTime(5_000);
    expect(fire).not.toHaveBeenCalled();
  });
});
