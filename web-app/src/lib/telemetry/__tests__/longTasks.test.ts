import { MAX_LONG_TASKS_PER_MINUTE, observeLongTasks, routeLabel } from "../longTasks";

describe("routeLabel", () => {
  it("keeps only the first path segment so ids never become labels", () => {
    expect(routeLabel("/sessions/8f3a-uuid/files")).toBe("sessions");
    expect(routeLabel("/")).toBe("home");
    expect(routeLabel("/Back_Log!?x")).toBe("backlogx");
    expect(routeLabel("/" + "a".repeat(80))).toHaveLength(24);
  });
});

describe("observeLongTasks", () => {
  const original = global.PerformanceObserver;
  let callback: (list: { getEntries: () => Array<{ duration: number }> }) => void;
  const disconnect = jest.fn();

  beforeEach(() => {
    class FakeObserver {
      static supportedEntryTypes = ["longtask"];
      constructor(cb: typeof callback) {
        callback = cb;
      }
      observe = jest.fn();
      disconnect = disconnect;
    }
    (global as unknown as { PerformanceObserver: unknown }).PerformanceObserver = FakeObserver;
  });
  afterEach(() => {
    (global as unknown as { PerformanceObserver: unknown }).PerformanceObserver = original;
  });

  it("reports each long task with the rounded duration and page label", () => {
    const track = jest.fn();
    observeLongTasks(track, () => "/backlog/42", () => 0);
    callback({ getEntries: () => [{ duration: 123.6 }] });
    expect(track).toHaveBeenCalledWith({ name: "cm.longtask", category: "performance", durationMs: 124, page: "backlog" });
  });

  it("caps events per minute and resets the window", () => {
    const track = jest.fn();
    let t = 0;
    observeLongTasks(track, () => "/", () => t);
    const burst = Array.from({ length: MAX_LONG_TASKS_PER_MINUTE + 10 }, () => ({ duration: 60 }));
    callback({ getEntries: () => burst });
    expect(track).toHaveBeenCalledTimes(MAX_LONG_TASKS_PER_MINUTE);
    t = 61_000;
    callback({ getEntries: () => [{ duration: 60 }] });
    expect(track).toHaveBeenCalledTimes(MAX_LONG_TASKS_PER_MINUTE + 1);
  });

  it("is a no-op where longtask is unsupported", () => {
    (global.PerformanceObserver as unknown as { supportedEntryTypes: string[] }).supportedEntryTypes = [];
    const stop = observeLongTasks(jest.fn());
    expect(() => stop()).not.toThrow();
  });
});
