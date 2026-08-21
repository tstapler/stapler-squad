/**
 * Tests for useStuckBacklogItems hook (Story 4.1.1).
 *
 * Covers: fetch on mount, error is populated (not swallowed) without
 * blanking the last-known list, and refetch()/snooze().
 */
import { renderHook, act, waitFor } from "@testing-library/react";
import { useStuckBacklogItems, StuckBacklogItemsProvider } from "./useStuckBacklogItems";
import { StuckReason } from "@/gen/session/v1/backlog_pb";

const mockListStuckBacklogItems = jest.fn();
const mockSnoozeStuckItem = jest.fn();
const mockResetStuckRemediation = jest.fn();
const mockBulkResetStuckRemediation = jest.fn();
const mockTriggerRemediationNow = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    listStuckBacklogItems: mockListStuckBacklogItems,
    snoozeStuckItem: mockSnoozeStuckItem,
    resetStuckRemediation: mockResetStuckRemediation,
    bulkResetStuckRemediation: mockBulkResetStuckRemediation,
    triggerRemediationNow: mockTriggerRemediationNow,
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

  it("resetRemediation() calls ResetStuckRemediation and refetches on applied=true", async () => {
    mockListStuckBacklogItems.mockResolvedValue({ items: [makeItem("a")] });
    mockResetStuckRemediation.mockResolvedValue({ applied: true });

    const { result } = renderHook(() => useStuckBacklogItems(60_000));
    await waitFor(() => expect(result.current.items).toHaveLength(1));
    mockListStuckBacklogItems.mockClear();
    mockListStuckBacklogItems.mockResolvedValueOnce({ items: [] });

    let applied = false;
    await act(async () => {
      applied = await result.current.resetRemediation("a", StuckReason.BOUNCING);
    });

    expect(applied).toBe(true);
    expect(mockResetStuckRemediation).toHaveBeenCalledWith(
      expect.objectContaining({ itemId: "a", reason: StuckReason.BOUNCING })
    );
    expect(mockListStuckBacklogItems).toHaveBeenCalledTimes(1);
  });

  it("bulkResetParkedRemediation() calls BulkResetStuckRemediation with only_parked=true and returns the reset count", async () => {
    mockListStuckBacklogItems.mockResolvedValue({ items: [] });
    mockBulkResetStuckRemediation.mockResolvedValue({ resetCount: 3 });

    const { result } = renderHook(() => useStuckBacklogItems(60_000));
    await waitFor(() => expect(result.current.lastFetched).not.toBeNull());
    mockListStuckBacklogItems.mockClear();

    let count = 0;
    await act(async () => {
      count = await result.current.bulkResetParkedRemediation();
    });

    expect(count).toBe(3);
    expect(mockBulkResetStuckRemediation).toHaveBeenCalledWith(
      expect.objectContaining({ onlyParked: true, onlyParkedExplicitlySet: true })
    );
    expect(mockListStuckBacklogItems).toHaveBeenCalledTimes(1);
  });

  it("triggerRemediationNow() calls TriggerRemediationNow, refetches, and rethrows on failure", async () => {
    mockListStuckBacklogItems.mockResolvedValue({ items: [] });
    mockTriggerRemediationNow.mockRejectedValue(new Error("already parked"));

    const { result } = renderHook(() => useStuckBacklogItems(60_000));
    await waitFor(() => expect(result.current.lastFetched).not.toBeNull());

    await expect(
      act(async () => {
        await result.current.triggerRemediationNow("a", StuckReason.BOUNCING);
      })
    ).rejects.toThrow("already parked");
    expect(mockTriggerRemediationNow).toHaveBeenCalledWith(
      expect.objectContaining({ itemId: "a", reason: StuckReason.BOUNCING })
    );
  });
});

describe("StuckBacklogItemsProvider", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shares a single poll across multiple consumers instead of each polling independently", async () => {
    mockListStuckBacklogItems.mockResolvedValue({ items: [makeItem("a")] });

    const { result } = renderHook(
      () => ({ a: useStuckBacklogItems(), b: useStuckBacklogItems() }),
      { wrapper: StuckBacklogItemsProvider }
    );

    await waitFor(() => expect(result.current.a.items).toHaveLength(1));
    expect(result.current.b.items).toHaveLength(1);
    expect(mockListStuckBacklogItems).toHaveBeenCalledTimes(1);
  });

  it("propagates a refetch from one consumer to every other consumer without waiting for the poll interval", async () => {
    mockListStuckBacklogItems.mockResolvedValueOnce({ items: [] });
    const { result } = renderHook(
      () => ({ a: useStuckBacklogItems(), b: useStuckBacklogItems() }),
      { wrapper: StuckBacklogItemsProvider }
    );
    await waitFor(() => expect(result.current.a.lastFetched).not.toBeNull());

    mockListStuckBacklogItems.mockResolvedValueOnce({ items: [makeItem("a")] });
    await act(async () => {
      await result.current.a.refetch();
    });

    expect(result.current.b.items).toHaveLength(1);
  });

  it("falls back to an independent standalone poll per instance when there is no provider ancestor", async () => {
    mockListStuckBacklogItems.mockResolvedValue({ items: [makeItem("a")] });

    renderHook(() => useStuckBacklogItems());
    renderHook(() => useStuckBacklogItems());

    await waitFor(() => expect(mockListStuckBacklogItems).toHaveBeenCalledTimes(2));
  });
});
