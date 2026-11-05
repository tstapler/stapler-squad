import { render } from "@testing-library/react";
import { WebVitalsReporter } from "../WebVitalsReporter";

const track = jest.fn();
jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({ track }),
}));
jest.mock("@/lib/telemetry/active-session", () => ({
  getActiveSessionUUID: () => "sess-1",
}));

const handlers: Record<string, (m: unknown) => void> = {};
jest.mock("web-vitals", () => {
  const reg = (name: string) => (cb: (m: unknown) => void) => {
    handlers[name] = cb;
  };
  return {
    onCLS: reg("CLS"),
    onFCP: reg("FCP"),
    onINP: reg("INP"),
    onLCP: reg("LCP"),
    onTTFB: reg("TTFB"),
  };
});

describe("WebVitalsReporter", () => {
  beforeEach(() => {
    track.mockClear();
    performance.mark = jest.fn();
  });


  it("registers all five metrics and emits the LCP payload and performance mark", () => {
    render(<WebVitalsReporter />);
    expect(Object.keys(handlers).sort()).toEqual([
      "CLS",
      "FCP",
      "INP",
      "LCP",
      "TTFB",
    ]);

    handlers.LCP({
      name: "LCP",
      value: 1200,
      rating: "good",
      delta: 1200,
      id: "v1",
    });

    expect(performance.mark).toHaveBeenCalledWith("web-vital:LCP", {
      detail: { value: 1200, rating: "good", delta: 1200, id: "v1" },
    });
    expect(track).toHaveBeenCalledWith({
      name: "web_vital.lcp",
      category: "performance",
      durationMs: 1200,
      sessionId: "sess-1",
      page: "home",
      labels: { rating: "good", id: "v1" },
    });
  });

  it("scales CLS to micro-units and labels the unit", () => {
    render(<WebVitalsReporter />);
    handlers.CLS({
      name: "CLS",
      value: 0.1234,
      rating: "good",
      delta: 0.1234,
      id: "c1",
    });
    expect(track).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "web_vital.cls",
        durationMs: 1234,
        labels: { rating: "good", id: "c1", cls_unit: "x10000" },
      }),
    );
  });
});
