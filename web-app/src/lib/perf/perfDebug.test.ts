import { traceCall } from "./perfDebug";

describe("traceCall", () => {
  afterEach(() => {
    delete window.__ssqPerf;
    jest.restoreAllMocks();
  });

  it("traceCall_should_LogStackPerCall_When_SsqPerfTraceEnabled_And_NoOpOtherwise", () => {
    const trace = jest.spyOn(console, "trace").mockImplementation(() => {});
    traceCall("getFeatureFlags");
    expect(trace).not.toHaveBeenCalled();
    window.__ssqPerf = { trace: false };
    traceCall("getFeatureFlags");
    expect(trace).not.toHaveBeenCalled();
    window.__ssqPerf = { trace: true };
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
