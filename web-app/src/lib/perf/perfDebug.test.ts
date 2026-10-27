import { installSsqPerf, traceCall, withInFlightGauge } from "./perfDebug";
import { defined } from "@/lib/utils/defined";

describe("traceCall", () => {
  afterEach(() => {
    delete window.__ssqPerf;
    window.localStorage.removeItem("ssq:perf");
    jest.restoreAllMocks();
  });

  it("traceCall_should_LogStackPerCall_When_SsqPerfTraceEnabled_And_NoOpOtherwise", () => {
    const trace = jest.spyOn(console, "trace").mockImplementation(() => {});
    traceCall("getFeatureFlags");
    expect(trace).not.toHaveBeenCalled();
    window.__ssqPerf = { trace: false } as never;
    traceCall("getFeatureFlags");
    expect(trace).not.toHaveBeenCalled();
    window.__ssqPerf = { trace: true } as never;
    traceCall("getFeatureFlags");
    traceCall("loadFlags");
    expect(trace).toHaveBeenCalledTimes(2);
    expect(trace.mock.calls[1][0]).toContain("loadFlags");
  });

  it("traceCall_should_NotThrow_When_WindowSsqPerfUndefined", () => {
    delete window.__ssqPerf;
    expect(() => traceCall("x")).not.toThrow();
  });
});

describe("InFlightGauge", () => {
  afterEach(() => {
    delete window.__ssqPerf;
    window.localStorage.removeItem("ssq:perf");
    jest.restoreAllMocks();
  });

  function deferredFetch() {
    const resolvers: Array<() => void> = [];
    const impl = jest.fn(
      () => new Promise<Response>((resolve) => resolvers.push(() => resolve({} as Response))),
    ) as unknown as typeof fetch;
    return { impl, settleAll: () => resolvers.splice(0).forEach((r) => r()) };
  }

  it("ssqPerf_should_BeUndefined_When_NoRuntimeOptIn_And_ObjectWhenOptedIn", () => {
    expect(installSsqPerf()).toBeUndefined();
    expect(typeof window.__ssqPerf).toBe("undefined");
    window.localStorage.setItem("ssq:perf", "1");
    expect(installSsqPerf()).toBeDefined();
    expect(typeof window.__ssqPerf).toBe("object");
  });

  it("gauge_should_ReportPeak10AndDupCount_When_10ConcurrentListFilesAgainst100msServer", async () => {
    window.localStorage.setItem("ssq:perf", "1");
    const perf = defined(installSsqPerf());
    const { impl, settleAll } = deferredFetch();
    const gauged = withInFlightGauge(impl);
    const body = new Uint8Array([1, 2, 3]);
    const calls = Array.from({ length: 10 }, () =>
      gauged("http://x/session.v1.SessionService/ListFiles", { method: "POST", body }),
    );
    expect(perf.inFlight).toBe(10);
    settleAll();
    await Promise.all(calls);
    expect(perf.inFlight).toBe(0);
    expect(perf.peakInFlight).toBe(10);
    expect(perf.dupCount["ListFiles"]).toBe(9);
  });

  it("gauge_should_CountStatus0_When_FetchRejects_And_PassThroughWhenNotOptedIn", async () => {
    const plain = jest.fn(async () => ({}) as Response) as unknown as typeof fetch;
    await withInFlightGauge(plain)("http://x/a/B");
    expect(window.__ssqPerf).toBeUndefined();
    window.localStorage.setItem("ssq:perf", "1");
    const perf = defined(installSsqPerf());
    const failing = jest.fn(async () => {
      throw new TypeError("net");
    }) as unknown as typeof fetch;
    await expect(withInFlightGauge(failing)("http://x/a/B")).rejects.toThrow("net");
    expect(perf.status0).toBe(1);
    expect(perf.inFlight).toBe(0);
  });

  it("queuedOver1s_should_EqualDirectResourceTimingRead_When_12CallsUnder6ConnectionCap", () => {
    const entries = Array.from({ length: 12 }, (_, i) => ({
      initiatorType: "fetch",
      fetchStart: 0,
      requestStart: i < 6 ? 5 : 1500 + i,
    })) as unknown as PerformanceResourceTiming[];
    let cb: PerformanceObserverCallback = () => {};
    const original = global.PerformanceObserver;
    global.PerformanceObserver = class {
      constructor(c: PerformanceObserverCallback) {
        cb = c;
      }
      observe() {}
      disconnect() {}
    } as never;
    (performance as unknown as Record<string, unknown>).getEntriesByType = () => entries;
    try {
      window.localStorage.setItem("ssq:perf", "1");
      const perf = defined(installSsqPerf());
      cb({ getEntries: () => entries } as never, {} as never);
      const direct = (performance.getEntriesByType("resource") as PerformanceResourceTiming[]).filter(
        (e) => e.requestStart - e.fetchStart > 1000,
      ).length;
      expect(direct).toBe(6);
      expect(perf.queuedOver1s).toBe(direct);
    } finally {
      delete (performance as unknown as Record<string, unknown>).getEntriesByType;
      global.PerformanceObserver = original;
    }
  });

  it("queuedOver1sByClass_should_AttributeEntriesToLastSeenPriority_When_QueuedOver1s", () => {
    const { createPriorityInterceptor, resetRequestPriorityForTest } = jest.requireActual("@/lib/api/requestPriority");
    resetRequestPriorityForTest();
    const { SessionService } = jest.requireActual("@/gen/session/v1/session_pb");
    const run = createPriorityInterceptor({ gate: () => Promise.resolve() })((async () => ({})) as never) as (r: unknown) => Promise<unknown>;
    const mk = (name: string) => ({ stream: false, service: SessionService, method: SessionService.method[name], contextValues: { get: () => undefined }, signal: new AbortController().signal });
    void run(mk("getFeatureFlags"));
    void run(mk("listShells"));
    const entries = ["GetFeatureFlags", "ListShells", "ListShells"].map((n) => ({
      name: `http://x/api/session.v1.SessionService/${n}`, initiatorType: "fetch", fetchStart: 0, requestStart: 2000,
    })) as unknown as PerformanceResourceTiming[];
    let cb: PerformanceObserverCallback = () => {};
    const original = global.PerformanceObserver;
    global.PerformanceObserver = class {
      constructor(c: PerformanceObserverCallback) { cb = c; }
      observe() {}
      disconnect() {}
    } as never;
    try {
      window.localStorage.setItem("ssq:perf", "1");
      const perf = defined(installSsqPerf());
      cb({ getEntries: () => entries } as never, {} as never);
      expect(perf.queuedOver1sByClass).toEqual({ critical: 1, deferred: 2 });
      expect(perf.queuedOver1s).toBe(3);
    } finally {
      global.PerformanceObserver = original;
    }
  });
});
