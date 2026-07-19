/**
 * Tests for useStuckBacklogItems hook (Story 4.1.1).
 *
 * Covers: fetch on mount, error is populated (not swallowed) without
 * blanking the last-known list, and refetch()/snooze().
 */
import { renderHook, act, waitFor } from "@testing-library/react";
import { useStuckBacklogItems } from "./useStuckBacklogItems";
import { StuckReason } from "@/gen/session/v1/backlog_pb";

const mockListStuckBacklogItems = jest.fn();
const mockSnoozeStuckItem = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    listStuckBacklogItems: mockListStuckBacklogItems,
    snoozeStuckItem: mockSnoozeStuckItem,
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({ unary: jest.fn(), stream: jest.fn() }),
}));

function makeItem(id: string) {
  return {
    itemId: id,
    title: `item ${id}`,
    status: "pr_pending",
    reason: StuckReason.PR_READY_UNMERGED,
    prNumber: 148,
    prUrl: "https://github.com/x/y/pull/148",
    context: "",
  };
}

describe("useStuckBacklogItems", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("fetches on mount and populates items + lastFetched", async () => {
    mockListStuckBacklogItems.mockResolvedValue({
      items: [makeItem("a"), makeItem("b"), makeItem("c"), makeItem("d"), makeItem("e")],
    });

    const { result } = renderHook(() => useStuckBacklogItems(60_000));

    await waitFor(() => expect(result.current.items).toHaveLength(5));
    expect(result.current.lastFetched).not.toBeNull();
    expect(result.current.error).toBeNull();
    expect(result.current.isLoading).toBe(false);
  });

  it("populates error (not silently swallowed) on RPC failure, retaining prior items", async () => {
    mockListStuckBacklogItems.mockResolvedValueOnce({ items: [makeItem("a")] });
    const { result } = renderHook(() => useStuckBacklogItems(60_000));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    mockListStuckBacklogItems.mockRejectedValueOnce(new Error("network down"));
    await act(async () => {
      await result.current.refetch();
    });

    expect(result.current.error).not.toBeNull();
    expect(result.current.error?.message).toBe("network down");
    // Prior items retained — never blanked on a failed poll.
    expect(result.current.items).toHaveLength(1);
  });

  it("refetch() re-invokes the RPC and updates items", async () => {
    mockListStuckBacklogItems.mockResolvedValueOnce({ items: [] });
    const { result } = renderHook(() => useStuckBacklogItems(60_000));
    await waitFor(() => expect(result.current.lastFetched).not.toBeNull());

    mockListStuckBacklogItems.mockResolvedValueOnce({ items: [makeItem("a")] });
    await act(async () => {
      await result.current.refetch();
    });

    expect(result.current.items).toHaveLength(1);
  });

  it("snooze() calls SnoozeStuckItem and refetches on success", async () => {
    mockListStuckBacklogItems.mockResolvedValue({ items: [makeItem("a")] });
    mockSnoozeStuckItem.mockResolvedValue({ applied: true });

    const { result } = renderHook(() => useStuckBacklogItems(60_000));
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    mockListStuckBacklogItems.mockClear();
    mockListStuckBacklogItems.mockResolvedValueOnce({ items: [] });

    let applied = false;
    await act(async () => {
      applied = await result.current.snooze("a", StuckReason.REWORK_CAP, new Date());
    });

    expect(applied).toBe(true);
    expect(mockSnoozeStuckItem).toHaveBeenCalledTimes(1);
    expect(mockListStuckBacklogItems).toHaveBeenCalledTimes(1);
  });

  it("snooze() does not refetch when the server reports applied=false", async () => {
    mockListStuckBacklogItems.mockResolvedValue({ items: [] });
    mockSnoozeStuckItem.mockResolvedValue({ applied: false });

    const { result } = renderHook(() => useStuckBacklogItems(60_000));
    await waitFor(() => expect(result.current.lastFetched).not.toBeNull());
    mockListStuckBacklogItems.mockClear();

    await act(async () => {
      await result.current.snooze("missing", StuckReason.REWORK_CAP, new Date());
    });

    expect(mockListStuckBacklogItems).not.toHaveBeenCalled();
  });
});
