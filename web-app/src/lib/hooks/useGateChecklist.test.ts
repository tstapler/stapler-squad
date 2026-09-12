/**
 * Tests for useGateChecklist/useGateApproval (ADR-005 Decision point 2).
 *
 * Covers:
 *  1. `archived` is excluded from the candidate loop — GetPendingGates is
 *     never called for it, even when it's in allowedTransitions.
 *  2. A target whose GetPendingGates response has zero gates is dropped
 *     entirely, not rendered as an empty candidate.
 *  3. Multiple non-empty candidates are all kept, each mapped to
 *     GateChecklistItem shape.
 *  4. useGateApproval.recordApproval calls RecordGateApproval with the right
 *     args and surfaces a friendly error on failure.
 */

import { renderHook, waitFor } from "@testing-library/react";
import { useGateChecklist, useGateApproval } from "./useGateChecklist";

const mockGetPendingGates = jest.fn();
const mockRecordGateApproval = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getPendingGates: mockGetPendingGates,
    recordGateApproval: mockRecordGateApproval,
  }),
  ConnectError: class ConnectError extends Error {
    code: number;
    rawMessage: string;
    constructor(message: string, code = 0) {
      super(message);
      this.name = "ConnectError";
      this.code = code;
      this.rawMessage = message;
    }
  },
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

beforeEach(() => {
  jest.clearAllMocks();
});

describe("useGateChecklist — excludes archived", () => {
  it("never calls GetPendingGates for 'archived', even when it's in allowedTransitions", async () => {
    mockGetPendingGates.mockResolvedValue({ gates: [] });

    const { result } = renderHook(() => useGateChecklist("item-1", ["review", "archived"]));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(mockGetPendingGates).toHaveBeenCalledTimes(1);
    expect(mockGetPendingGates).toHaveBeenCalledWith({ itemId: "item-1", toStatus: "review" });
  });
});

describe("useGateChecklist — drops empty-gate targets", () => {
  it("drops a candidate whose response has zero gates", async () => {
    mockGetPendingGates.mockImplementation(({ toStatus }: { toStatus: string }) =>
      Promise.resolve(
        toStatus === "review"
          ? { gates: [{ gateId: "g1", kind: "structural", satisfied: false, description: "blocked", actionHint: "" }] }
          : { gates: [] }
      )
    );

    const { result } = renderHook(() => useGateChecklist("item-1", ["review", "design_review"]));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.candidates).toHaveLength(1);
    expect(result.current.candidates[0].toStatus).toBe("review");
  });
});

describe("useGateChecklist — multiple non-empty candidates", () => {
  it("keeps every non-empty candidate, mapped to GateChecklistItem shape", async () => {
    mockGetPendingGates.mockImplementation(({ toStatus }: { toStatus: string }) =>
      Promise.resolve({
        gates: [
          {
            gateId: `gate-${toStatus}`,
            kind: "human_approval",
            satisfied: false,
            description: "",
            actionHint: "approve it",
          },
        ],
      })
    );

    const { result } = renderHook(() => useGateChecklist("item-1", ["review", "design_review"]));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.candidates).toHaveLength(2);
    const byTo = Object.fromEntries(result.current.candidates.map((c) => [c.toStatus, c]));
    expect(byTo["review"].gates[0]).toEqual({
      gateId: "gate-review",
      kind: "human_approval",
      satisfied: false,
      description: undefined,
      actionHint: "approve it",
    });
    expect(byTo["design_review"].gates[0].gateId).toBe("gate-design_review");
  });
});

describe("useGateChecklist — RPC failure", () => {
  it("surfaces an RPC failure via `error` and never fabricates candidates", async () => {
    mockGetPendingGates.mockRejectedValue(new Error("network error"));

    const { result } = renderHook(() => useGateChecklist("item-1", ["review"]));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.error).toBe("network error");
    expect(result.current.candidates).toEqual([]);
  });
});

describe("useGateApproval — recordApproval", () => {
  it("calls RecordGateApproval with itemId/gateId", async () => {
    mockRecordGateApproval.mockResolvedValue({ record: {} });

    const { result } = renderHook(() => useGateApproval());
    await result.current.recordApproval("item-1", "gate-1");

    expect(mockRecordGateApproval).toHaveBeenCalledWith({
      itemId: "item-1",
      gateId: "gate-1",
      satisfiedBy: "",
    });
  });

  it("throws a friendly message when the RPC fails", async () => {
    mockRecordGateApproval.mockRejectedValue(new Error("already recorded"));

    const { result } = renderHook(() => useGateApproval());

    await expect(result.current.recordApproval("item-1", "gate-1")).rejects.toThrow("already recorded");
  });
});
