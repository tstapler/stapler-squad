import React from "react";
import { renderHook } from "@testing-library/react";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";
import { buildActivityRows } from "@/lib/utils/backgroundActivity";
import { makeHiddenSession } from "@/components/ui/__fixtures__/notificationFixtures";
import { useBackgroundTelemetry } from "./useBackgroundTelemetry";
import { usePaneOpenTiming } from "./usePaneOpenTiming";

const track = jest.fn();
const wrapper = ({ children }: { children: React.ReactNode }) => (
  <AnalyticsContext.Provider value={{ provider: {} as never, track }}>{children}</AnalyticsContext.Provider>
);

const stopped = (n: number) => Array.from({ length: n }, (_, i) => makeHiddenSession({ id: `diagnose:d${i}` }));

describe("useBackgroundTelemetry", () => {
  beforeEach(() => {
    track.mockClear();
    window.localStorage.clear();
  });

  it("useBackgroundTelemetry_should_postCountsByStateOncePerPoll_When_10StoppedUnknownAnd1Failure", () => {
    const rows = buildActivityRows([], [...stopped(10), makeHiddenSession({ id: "review:bad", backgroundOutcome: "failed" })]);
    const { rerender } = renderHook(({ at }) => useBackgroundTelemetry(rows, at, true), {
      wrapper,
      initialProps: { at: 1 },
    });
    rerender({ at: 1 });
    expect(track).toHaveBeenCalledTimes(1);
    expect(track).toHaveBeenCalledWith(
      expect.objectContaining({ name: "background_activity_rows", labels: { stopped_unknown: "10", exception_failure: "1" } }),
    );
    rerender({ at: 2 });
    expect(track).toHaveBeenCalledTimes(2);
  });

  it("useBackgroundTelemetry_should_raiseEvidenceAlertOnce_When_24hOfPollsShowNoFinished", () => {
    jest.useFakeTimers().setSystemTime(0);
    const rows = buildActivityRows([], stopped(11));
    const onAlert = jest.fn();
    const { rerender } = renderHook(({ at }) => useBackgroundTelemetry(rows, at, true, onAlert), {
      wrapper,
      initialProps: { at: 1 },
    });
    for (let poll = 2; poll <= 30; poll += 1) {
      jest.setSystemTime((poll - 1) * 3_600_000);
      rerender({ at: poll });
    }
    expect(onAlert).toHaveBeenCalledTimes(1);
    jest.useRealTimers();
  });

  it("useBackgroundTelemetry_should_stayInert_When_disabledOrNoProvider", () => {
    const rows = buildActivityRows([], stopped(2));
    renderHook(() => useBackgroundTelemetry(rows, 1, false), { wrapper });
    renderHook(() => useBackgroundTelemetry(rows, 1, true));
    expect(track).not.toHaveBeenCalled();
  });
});

describe("usePaneOpenTiming", () => {
  beforeEach(() => track.mockClear());

  it("usePaneOpenTiming_should_postOneSample_When_firstActionRowCommitsAfterOpen", () => {
    const { rerender } = renderHook(({ open, ready }) => usePaneOpenTiming(open, ready), {
      wrapper,
      initialProps: { open: false, ready: true },
    });
    expect(track).not.toHaveBeenCalled();
    rerender({ open: true, ready: true });
    rerender({ open: true, ready: false });
    rerender({ open: true, ready: true });
    expect(track).toHaveBeenCalledTimes(1);
    expect(track).toHaveBeenCalledWith(
      expect.objectContaining({ name: "notification_pane_open_ms", category: "performance", durationMs: expect.any(Number) }),
    );
    rerender({ open: false, ready: true });
    rerender({ open: true, ready: true });
    expect(track).toHaveBeenCalledTimes(2);
  });
});
