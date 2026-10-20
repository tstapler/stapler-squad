import { act, renderHook } from "@testing-library/react";
import { GUIDANCE_POLL_MS, isMoving, useGuidanceRow } from "./useGuidanceRow";
import { makeGuidanceRow } from "@/components/ui/__fixtures__/notificationFixtures";

const getGuidanceRequest = jest.fn();
jest.mock("@connectrpc/connect", () => ({ ...jest.requireActual("@connectrpc/connect"), createClient: () => ({ getGuidanceRequest }) }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

const respond = (row: ReturnType<typeof makeGuidanceRow>) => getGuidanceRequest.mockResolvedValue({ request: row });

beforeEach(() => {
  jest.useFakeTimers();
  getGuidanceRequest.mockReset();
});
afterEach(() => jest.useRealTimers());

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
    expect(getGuidanceRequest).not.toHaveBeenCalled();
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
    const settled = getGuidanceRequest.mock.calls.length;
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS * 5);
    });
    expect(getGuidanceRequest.mock.calls.length).toBeLessThanOrEqual(settled + 1);
  });

  it("keeps the last row when a read fails", async () => {
    respond(makeGuidanceRow());
    const { result } = renderHook(() => useGuidanceRow("q1", true));
    await act(async () => {});
    getGuidanceRequest.mockRejectedValue(new Error("offline"));
    await act(async () => {
      jest.advanceTimersByTime(GUIDANCE_POLL_MS);
    });
    expect(result.current?.id).toBe("q1");
  });
});
