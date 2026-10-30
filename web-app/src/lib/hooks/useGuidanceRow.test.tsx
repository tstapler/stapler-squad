import { act, renderHook } from "@testing-library/react";
import { GUIDANCE_POLL_MS, isMoving, useGuidanceRow } from "./useGuidanceRow";
import { resetGuidanceRowStore } from "./guidanceRowStore";
import { makeGuidanceRow } from "@/components/ui/__fixtures__/notificationFixtures";

const getGuidanceRequest = jest.fn();
const listAllPendingGuidanceRequests = jest.fn();
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getGuidanceRequest, listAllPendingGuidanceRequests }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

const respond = (row: ReturnType<typeof makeGuidanceRow>) => getGuidanceRequest.mockResolvedValue({ request: row });
const setHidden = (hidden: boolean) => {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event("visibilitychange"));
};
const totalCalls = () => getGuidanceRequest.mock.calls.length + listAllPendingGuidanceRequests.mock.calls.length;

beforeEach(() => {
  jest.useFakeTimers();
  getGuidanceRequest.mockReset();
  listAllPendingGuidanceRequests.mockReset();
  listAllPendingGuidanceRequests.mockResolvedValue({ requests: [] });
  setHidden(false);
});
afterEach(() => {
  resetGuidanceRowStore();
  jest.useRealTimers();
});

describe("isMoving", () => {
  it("is true for an open or sent row and false once settled", () => {
    expect(isMoving(undefined)).toBe(true);
    expect(isMoving(makeGuidanceRow())).toBe(true);
    expect(isMoving(makeGuidanceRow({ status: "answered", deliveryState: "sent" }))).toBe(true);
    expect(isMoving(makeGuidanceRow({ status: "answered", deliveryState: "acknowledged" }))).toBe(false);
    expect(isMoving(makeGuidanceRow({ status: "cancelled" }))).toBe(false);
    expect(isMoving(makeGuidanceRow({ status: "answered", deliveryState: "delivery_failed" }))).toBe(false);
  });
});

describe("useGuidanceRow", () => {
  it("reads nothing without an id", async () => {
    renderHook(() => useGuidanceRow(undefined, true));
    await act(async () => {});
    expect(totalCalls()).toBe(0);
  });

  it("polls while the row can still change and stops once it settles", async () => {
    respond(makeGuidanceRow({ status: "answered", deliveryState: "sent" }));
    const { result } = renderHook(() => useGuidanceRow("q1", false));
    await act(async () => {});
    expect(result.current?.deliveryState).toBe("sent");
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS);
    });
    expect(getGuidanceRequest).toHaveBeenCalledTimes(2);

    respond(makeGuidanceRow({ status: "answered", deliveryState: "acknowledged" }));
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS);
    });
    expect(result.current?.deliveryState).toBe("acknowledged");
    const settled = totalCalls();
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS * 5);
    });
    expect(totalCalls()).toBe(settled);
  });

  it("keeps the last row when a read fails", async () => {
    listAllPendingGuidanceRequests.mockResolvedValue({ requests: [makeGuidanceRow()] });
    const { result } = renderHook(() => useGuidanceRow("q1", true));
    await act(async () => {});
    listAllPendingGuidanceRequests.mockRejectedValue(new Error("offline"));
    getGuidanceRequest.mockRejectedValue(new Error("offline"));
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS);
    });
    expect(result.current?.id).toBe("q1");
  });

  it("serves many open rows with one list call per tick instead of one read per row", async () => {
    const ids = ["q1", "q2", "q3", "q4", "q5"];
    listAllPendingGuidanceRequests.mockResolvedValue({ requests: ids.map((id) => makeGuidanceRow({ id })) });
    const hooks = ids.map((id) => renderHook(() => useGuidanceRow(id, true)));
    await act(async () => {});
    for (let i = 0; i < 10; i++) {
      await act(async () => {
        jest.advanceTimersByTime(GUIDANCE_POLL_MS);
      });
    }
    // 1 initial shared read + 10 ticks, never one-per-row (before: 5 rows x 11 reads = 55 Get calls)
    expect(getGuidanceRequest).not.toHaveBeenCalled();
    expect(listAllPendingGuidanceRequests).toHaveBeenCalledTimes(11);
    hooks.forEach((h, i) => expect(h.result.current?.id).toBe(ids[i]));
  });

  it("makes no calls while the tab is hidden and catches up when it is shown", async () => {
    listAllPendingGuidanceRequests.mockResolvedValue({ requests: [makeGuidanceRow()] });
    renderHook(() => useGuidanceRow("q1", true));
    await act(async () => {});
    const before = totalCalls();
    act(() => setHidden(true));
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS * 20);
    });
    expect(totalCalls()).toBe(before);
    await act(async () => setHidden(false));
    expect(totalCalls()).toBe(before + 1);
  });

  it("never overlaps ticks while a read is still in flight", async () => {
    let release: (v: { requests: never[] }) => void = () => {};
    listAllPendingGuidanceRequests.mockImplementation(() => new Promise((r) => (release = r)));
    renderHook(() => useGuidanceRow("q1", true));
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS * 5);
    });
    expect(listAllPendingGuidanceRequests).toHaveBeenCalledTimes(1);
    await act(async () => release({ requests: [] }));
  });
});
