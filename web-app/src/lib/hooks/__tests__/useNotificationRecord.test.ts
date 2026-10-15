import { renderHook, waitFor } from "@testing-library/react";
import { useNotificationRecord } from "../useNotificationRecord";
import type { NotificationHistoryItem } from "@/lib/types/notification";

const mockGetNotificationHistory = jest.fn();
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getNotificationHistory: (...args: unknown[]) => mockGetNotificationHistory(...args) }),
}));

const row = (id: string): NotificationHistoryItem => ({
  id,
  sessionId: "h1",
  sessionName: "review:h1",
  title: "Review failed",
  message: "tests red",
  timestamp: 1_700_000_000_000,
  isRead: false,
  notificationType: "error",
});

describe("useNotificationRecord (deleted-session card text)", () => {
  beforeEach(() => mockGetNotificationHistory.mockReset());

  it("reads the hydrated slice first and makes no request", () => {
    const { result } = renderHook(() => useNotificationRecord([row("n1")], "n1", "h1", true));
    expect(result.current).toEqual({ title: "Review failed", message: "tests red", timestampMs: 1_700_000_000_000 });
    expect(mockGetNotificationHistory).not.toHaveBeenCalled();
  });

  it("fetches the session's history when the id is not in the slice and returns the record", async () => {
    mockGetNotificationHistory.mockResolvedValue({
      notifications: [{ id: "n2", title: "Old failure", message: "boom", createdAt: { seconds: 1_600_000_000n } }],
    });
    const { result } = renderHook(() => useNotificationRecord([], "n2", "h1", true));
    expect(result.current).toBeUndefined();
    await waitFor(() => expect(result.current).toEqual({ title: "Old failure", message: "boom", timestampMs: 1_600_000_000_000 }));
    expect(mockGetNotificationHistory.mock.calls[0][0]).toMatchObject({ sessionId: "h1" });
  });

  it("falls back to null when the record was pruned, the call fails, or the link carried no id", async () => {
    mockGetNotificationHistory.mockResolvedValueOnce({ notifications: [] });
    const pruned = renderHook(() => useNotificationRecord([], "gone", "h1", true));
    await waitFor(() => expect(pruned.result.current).toBeNull());

    mockGetNotificationHistory.mockRejectedValueOnce(new Error("offline"));
    const failed = renderHook(() => useNotificationRecord([], "n3", "h1", true));
    await waitFor(() => expect(failed.result.current).toBeNull());

    const noId = renderHook(() => useNotificationRecord([row("n1")], null, "h1", true));
    expect(noId.result.current).toBeNull();
  });

  it("does not ask the server until enabled", () => {
    renderHook(() => useNotificationRecord([], "n4", "h1", false));
    expect(mockGetNotificationHistory).not.toHaveBeenCalled();
  });
});
