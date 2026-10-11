import React from "react";
import { act, render, screen } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { GateStatusLine, summarizeGateStats, type GateStatsLike } from "./GateStatusLine";

jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({ createClient: () => ({}) }));

const NOW = new Date("2026-10-09T12:30:00Z");

function stats(): GateStatsLike {
  const hoursAgo = (h: number) => timestampFromDate(new Date(NOW.getTime() - h * 3600_000));
  return {
    sinceProcessStart: [
      { counter: "rpc_unversioned", count: 4n },
      { counter: "would_suppress{channel=bus}", count: 99n },
    ],
    buckets: [
      { hourStart: hoursAgo(30), counters: [{ counter: "would_suppress", count: 1000n }] },
      {
        hourStart: hoursAgo(5),
        counters: [
          { counter: "suppressed", count: 5n },
          { counter: "would_suppress", count: 7n },
          { counter: "unresolved", count: 2n },
        ],
      },
      { hourStart: hoursAgo(1), counters: [{ counter: "would_suppress", count: 3 }] },
    ],
  };
}

describe("summarizeGateStats", () => {
  it("summarizeGateStats_should_CountOnlyBucketsWithinTwentyFourHoursAndUnversionedSinceStart", () => {
    expect(summarizeGateStats(stats(), NOW)).toBe(
      "Last 24h: 5 hidden events suppressed; 10 would have been suppressed (gate off); 2 unresolved fail-open; 4 unversioned ssq-notify",
    );
  });

  it("summarizeGateStats_should_ReadZeros_When_NothingRecorded", () => {
    expect(summarizeGateStats({ sinceProcessStart: [], buckets: [] }, NOW)).toBe(
      "Last 24h: 0 hidden events suppressed; 0 would have been suppressed (gate off); 0 unresolved fail-open; 0 unversioned ssq-notify",
    );
  });
});

describe("GateStatusLine", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("GateStatusLine_should_RenderStatusAndPollWhileMounted_ThenStopPolling_When_Unmounted", async () => {
    const fetchStats = jest.fn().mockResolvedValue(stats());
    const { unmount } = render(<GateStatusLine fetchStats={fetchStats} pollMs={1000} />);
    await act(async () => {});
    expect(screen.getByTestId("gate-status-line").textContent).toMatch(/^Last 24h: /);
    expect(fetchStats).toHaveBeenCalledTimes(1);

    await act(async () => {
      jest.advanceTimersByTime(1000);
    });
    expect(fetchStats).toHaveBeenCalledTimes(2);

    unmount();
    await act(async () => {
      jest.advanceTimersByTime(5000);
    });
    expect(fetchStats).toHaveBeenCalledTimes(2);
  });

  it("GateStatusLine_should_RenderNothing_When_RpcFails", async () => {
    const fetchStats = jest.fn().mockRejectedValue(new Error("unavailable"));
    render(<GateStatusLine fetchStats={fetchStats} pollMs={1000} />);
    await act(async () => {});
    expect(screen.queryByTestId("gate-status-line")).toBeNull();
  });
});
