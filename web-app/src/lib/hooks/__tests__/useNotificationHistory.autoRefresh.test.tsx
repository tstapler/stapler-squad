import { act, renderHook } from "@testing-library/react";
import { useNotificationHistory } from "@/lib/hooks/useNotificationHistory";

const getNotificationHistory = jest.fn();
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getNotificationHistory }),
}));
jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
  getKeepaliveConnectTransport: () => ({}),
}));

const FALLBACK_POLL_MS = 30_000;

async function flush() {
  await act(async () => {
    await Promise.resolve();
  });
}

describe("useNotificationHistory auto-refresh", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    getNotificationHistory.mockReset().mockResolvedValue({ notifications: [], unreadCount: 0, hasMore: false });
    Object.defineProperty(document, "visibilityState", { value: "visible", configurable: true });
  });
  afterEach(() => jest.useRealTimers());

  it("does not poll while the live stream is up", async () => {
    renderHook(() => useNotificationHistory({ streamLive: true }));
    await flush();
    expect(getNotificationHistory).toHaveBeenCalledTimes(1);
    await act(async () => jest.advanceTimersByTime(FALLBACK_POLL_MS * 3));
    expect(getNotificationHistory).toHaveBeenCalledTimes(1);
  });

  it("polls by RPC while the stream is down so the list stays current", async () => {
    renderHook(() => useNotificationHistory({ streamLive: false }));
    await flush();
    await act(async () => jest.advanceTimersByTime(FALLBACK_POLL_MS));
    await flush();
    expect(getNotificationHistory).toHaveBeenCalledTimes(2);
  });

  it("refreshes when the network comes back, once per burst", async () => {
    renderHook(() => useNotificationHistory({ streamLive: true }));
    await flush();
    await act(async () => {
      window.dispatchEvent(new Event("online"));
      window.dispatchEvent(new Event("online"));
    });
    await flush();
    expect(getNotificationHistory).toHaveBeenCalledTimes(2);
  });
});
