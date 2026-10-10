import React from "react";
import { act, render, screen } from "@testing-library/react";
import { GateKindOverrides } from "./GateKindOverrides";
import { GateStatusLine } from "./GateStatusLine";

const mockGetStats = jest.fn();

jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getDeliveryGateStats: (...a: unknown[]) => mockGetStats(...a) }),
}));

describe("shared gate stats poller", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockGetStats.mockReset();
    mockGetStats.mockResolvedValue({ sinceProcessStart: [], buckets: [], eventsByKind24h: { review: 5n } });
  });
  afterEach(() => jest.useRealTimers());

  it("should_send_one_request_per_tick_when_status_line_and_overrides_are_both_mounted", async () => {
    const { unmount } = render(
      <>
        <GateStatusLine />
        <GateKindOverrides onChange={jest.fn()} onReset={jest.fn()} />
      </>,
    );
    await act(async () => {});
    expect(mockGetStats).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("gate-status-line")).toBeInTheDocument();
    expect(screen.getByTestId("gate-override-count-review").textContent).toBe(" (5 events in the last 24h)");

    await act(async () => {
      jest.advanceTimersByTime(30_000);
    });
    expect(mockGetStats).toHaveBeenCalledTimes(2);

    unmount();
    await act(async () => {
      jest.advanceTimersByTime(90_000);
    });
    expect(mockGetStats).toHaveBeenCalledTimes(2);
  });
});
