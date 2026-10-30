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

type Row = ReturnType<typeof makeGuidanceRow>;
const respond = (row: Row) => getGuidanceRequest.mockResolvedValue({ request: row });
const setHidden = (hidden: boolean) => {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event("visibilitychange"));
};
const totalCalls = () => getGuidanceRequest.mock.calls.length + listAllPendingGuidanceRequests.mock.calls.length;
const tickOnce = () =>
  act(async () => {
    jest.advanceTimersByTime(GUIDANCE_POLL_MS);
  });
const ids = ["q1", "q2", "q3", "q4", "q5"];
const mountMany = () => ids.map((id) => renderHook(() => useGuidanceRow(id, true)));

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
    await tickOnce();
    expect(getGuidanceRequest).toHaveBeenCalledTimes(2);

    respond(makeGuidanceRow({ status: "answered", deliveryState: "acknowledged" }));
    await tickOnce();
    expect(result.current?.deliveryState).toBe("acknowledged");
    const settled = totalCalls();
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS * 5);
    });
    expect(totalCalls()).toBe(settled);
  });

  it("reads one or two rows individually and never calls the global list", async () => {
    respond(makeGuidanceRow());
    renderHook(() => useGuidanceRow("q1", true));
    await act(async () => {});
    await tickOnce();
    expect(listAllPendingGuidanceRequests).not.toHaveBeenCalled();
    expect(getGuidanceRequest).toHaveBeenCalledWith(expect.anything(), { timeoutMs: 10_000 });
  });

  it("keeps the last row when a read fails", async () => {
    respond(makeGuidanceRow());
    const { result } = renderHook(() => useGuidanceRow("q1", true));
    await act(async () => {});
    getGuidanceRequest.mockRejectedValue(new Error("offline"));
    await tickOnce();
    expect(result.current?.id).toBe("q1");
  });

  it("serves many open rows with one list call per tick instead of one read per row", async () => {
    listAllPendingGuidanceRequests.mockResolvedValue({ requests: ids.map((id) => makeGuidanceRow({ id })) });
    const hooks = mountMany();
    await act(async () => {});
    for (let i = 0; i < 10; i++) await tickOnce();
    // 10 ticks plus the initial reads (one more for rows mounted mid-flight), never one per row
    // (before: 5 rows x 11 reads = 55 Get calls)
    expect(getGuidanceRequest).not.toHaveBeenCalled();
    expect(listAllPendingGuidanceRequests.mock.calls.length).toBeLessThanOrEqual(12);
    hooks.forEach((h, i) => expect(h.result.current?.id).toBe(ids[i]));
  });

  it("does not fall back to one read per row when the list call fails", async () => {
    listAllPendingGuidanceRequests.mockRejectedValue(new Error("busy"));
    mountMany();
    await act(async () => {});
    for (let i = 0; i < 3; i++) await tickOnce();
    expect(getGuidanceRequest).not.toHaveBeenCalled();
  });

  it("re-reads a settled row that is only held fresh every 15s, not every 3s", async () => {
    respond(makeGuidanceRow({ status: "answered", deliveryState: "acknowledged" }));
    renderHook(() => useGuidanceRow("q1", true));
    await act(async () => {});
    expect(getGuidanceRequest).toHaveBeenCalledTimes(1);
    for (let i = 0; i < 4; i++) await tickOnce(); // 12 s
    expect(getGuidanceRequest).toHaveBeenCalledTimes(1);
    await tickOnce(); // 15 s
    expect(getGuidanceRequest).toHaveBeenCalledTimes(2);
  });

  it("keeps the cached row when keepFresh flips on a mounted row", async () => {
    respond(makeGuidanceRow());
    const { result, rerender } = renderHook(({ keep }) => useGuidanceRow("q1", keep), { initialProps: { keep: true } });
    await act(async () => {});
    expect(result.current?.id).toBe("q1");
    rerender({ keep: false });
    expect(result.current?.id).toBe("q1");
    rerender({ keep: true });
    expect(result.current?.id).toBe("q1");
  });

  it("reads a row that mounts while another read is in flight as soon as that read returns", async () => {
    let release: (v: { request: Row }) => void = () => {};
    getGuidanceRequest.mockImplementationOnce(() => new Promise((r) => (release = r)));
    renderHook(() => useGuidanceRow("q1", true));
    await act(async () => {});
    respond(makeGuidanceRow({ id: "q2" }));
    const second = renderHook(() => useGuidanceRow("q2", true));
    await act(async () => {});
    expect(second.result.current).toBeUndefined();
    await act(async () => release({ request: makeGuidanceRow() }));
    await act(async () => {});
    expect(second.result.current?.id).toBe("q2");
  });

  it("makes no calls while the tab is hidden and catches up when it is shown", async () => {
    respond(makeGuidanceRow());
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
    getGuidanceRequest.mockImplementation(() => new Promise(() => {}));
    renderHook(() => useGuidanceRow("q1", true));
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS * 5);
    });
    expect(getGuidanceRequest).toHaveBeenCalledTimes(1);
  });
});
