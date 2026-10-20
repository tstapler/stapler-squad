/**
 * useEnforceReadiness (Story 1.3.3c): every evidence transition, with a recorded
 * status-RPC counter. The hook never writes the flag; its caller does, only when
 * confirm() resolves true.
 */

import { act, renderHook, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import {
  ClassReadinessSchema,
  GetWorkflowEngineStatusResponseSchema,
  WorkflowEngineSoakVerdict,
} from "@/gen/session/v1/backlog_pb";
import type { ClassReadinessView } from "@/lib/hooks/useEngineModeStatus";
import { isClean, useEnforceReadiness, worseningOf } from "./useEnforceReadiness";

const mockGetStatus = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getWorkflowEngineStatus: (...a: unknown[]) => mockGetStatus(...a) }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

const V = WorkflowEngineSoakVerdict;

function status(divergence: bigint, verdict: WorkflowEngineSoakVerdict, stranded = 0n) {
  const r = create(ClassReadinessSchema, { divergence24h: divergence, soakVerdict: verdict, strandedItemCount: stranded });
  return create(GetWorkflowEngineStatusResponseSchema, { classReadiness: { operator: r, agent: r, system: r } });
}

const CLEAN = status(0n, V.READY);

beforeEach(() => {
  mockGetStatus.mockReset();
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => jest.restoreAllMocks());

describe("worseningOf / isClean", () => {
  const clean: ClassReadinessView = { divergence24h: 0, verdict: "ready", strandedItemCount: 0 };

  it("treats only zero differences, READY and zero stranded items as clean", () => {
    expect(isClean(clean)).toBe(true);
    expect(isClean({ ...clean, divergence24h: 1 })).toBe(false);
    expect(isClean({ ...clean, verdict: "needs_ack" })).toBe(false);
    expect(isClean({ ...clean, strandedItemCount: 1 })).toBe(false);
  });

  it("names each way the evidence got worse and ignores improvements", () => {
    expect(worseningOf(clean, { ...clean, divergence24h: 2 })).toContain("rose from 0 to 2");
    expect(worseningOf(clean, { ...clean, verdict: "not_ready" })).toContain("moved from READY to NOT READY");
    expect(worseningOf(clean, { ...clean, strandedItemCount: 3 })).toContain("would lose an exit rose from 0 to 3");
    expect(worseningOf({ ...clean, divergence24h: 5 }, { ...clean, divergence24h: 2 })).toBeNull();
    expect(worseningOf(clean, clean)).toBeNull();
  });

  it("counts evidence the operator never saw as a change", () => {
    expect(worseningOf(null, clean)).not.toBeNull();
  });
});

describe("useEnforceReadiness", () => {
  it("is loading with Enforce unavailable, then clean and enabled with no acknowledgement", async () => {
    mockGetStatus.mockResolvedValue(CLEAN);
    const { result } = renderHook(() => useEnforceReadiness("operator"));
    expect(result.current.evidence.phase).toBe("loading");
    expect(result.current.canEnforce).toBe(false);
    expect(result.current.needsAck).toBe(false);

    await waitFor(() => expect(result.current.evidence.phase).toBe("loaded"));
    expect(result.current.clean).toBe(true);
    expect(result.current.needsAck).toBe(false);
    expect(result.current.canEnforce).toBe(true);
  });

  it("needs the acknowledgement when the evidence is not clean", async () => {
    mockGetStatus.mockResolvedValue(status(3n, V.NEEDS_OWNER_ACK, 1n));
    const { result } = renderHook(() => useEnforceReadiness("agent"));
    await waitFor(() => expect(result.current.evidence.phase).toBe("loaded"));
    expect(result.current.needsAck).toBe(true);
    expect(result.current.canEnforce).toBe(false);

    act(() => result.current.setAcknowledged(true));
    expect(result.current.canEnforce).toBe(true);
  });

  it("reports a class the status did not carry as a failure, not as zeros", async () => {
    mockGetStatus.mockResolvedValue(create(GetWorkflowEngineStatusResponseSchema, {}));
    const { result } = renderHook(() => useEnforceReadiness("system"));
    await waitFor(() => expect(result.current.evidence.phase).toBe("failed"));
    expect(result.current.needsAck).toBe(true);
    expect(result.current.canEnforce).toBe(false);
  });

  it("recovers from a failed load through retryEvidence", async () => {
    mockGetStatus.mockRejectedValueOnce(new Error("down"));
    const { result } = renderHook(() => useEnforceReadiness("operator"));
    await waitFor(() => expect(result.current.evidence).toEqual({ phase: "failed", error: "down" }));

    mockGetStatus.mockResolvedValue(CLEAN);
    let ok = false;
    await act(async () => {
      ok = await result.current.retryEvidence();
    });
    expect(ok).toBe(true);
    expect(result.current.evidence.phase).toBe("loaded");
    expect(result.current.retrying).toBe(false);
  });

  it("allows the write when the re-check is unchanged, with exactly one more RPC", async () => {
    mockGetStatus.mockResolvedValue(CLEAN);
    const { result } = renderHook(() => useEnforceReadiness("operator"));
    await waitFor(() => expect(result.current.evidence.phase).toBe("loaded"));
    expect(mockGetStatus).toHaveBeenCalledTimes(1);

    let allowed = false;
    await act(async () => {
      allowed = await result.current.confirm();
    });
    expect(allowed).toBe(true);
    expect(mockGetStatus).toHaveBeenCalledTimes(2);
    expect(result.current.recheck.phase).toBe("idle");
  });

  it("allows the write when the re-check is better than what was shown", async () => {
    mockGetStatus.mockResolvedValue(status(2n, V.READY));
    const { result } = renderHook(() => useEnforceReadiness("operator"));
    await waitFor(() => expect(result.current.evidence.phase).toBe("loaded"));
    act(() => result.current.setAcknowledged(true));

    mockGetStatus.mockResolvedValue(CLEAN);
    let allowed = false;
    await act(async () => {
      allowed = await result.current.confirm();
    });
    expect(allowed).toBe(true);
  });

  it.each([
    ["divergence up", status(5n, V.READY), "shadow differences rose from 0 to 5"],
    ["verdict away from READY", status(0n, V.NOT_READY), "soak verdict moved from READY to NOT READY"],
    ["stranded count up", status(0n, V.READY, 2n), "items that would lose an exit rose from 0 to 2"],
  ])("refuses the write and re-asks when the re-check is worse (%s)", async (_name, worse, note) => {
    mockGetStatus.mockResolvedValue(CLEAN);
    const { result } = renderHook(() => useEnforceReadiness("operator"));
    await waitFor(() => expect(result.current.evidence.phase).toBe("loaded"));

    mockGetStatus.mockResolvedValue(worse);
    let allowed = true;
    await act(async () => {
      allowed = await result.current.confirm();
    });
    expect(allowed).toBe(false);
    expect(result.current.changeNote).toContain("Readiness evidence changed since you opened this dialog");
    expect(result.current.changeNote).toContain(note);
    expect(result.current.acknowledged).toBe(false);
    expect(result.current.canEnforce).toBe(false);
    expect(result.current.needsAck).toBe(true);
    expect(result.current.focusRequest).toBe("ack");
  });

  it("clears a ticked acknowledgement when the evidence got worse", async () => {
    mockGetStatus.mockResolvedValue(status(1n, V.READY));
    const { result } = renderHook(() => useEnforceReadiness("operator"));
    await waitFor(() => expect(result.current.evidence.phase).toBe("loaded"));
    act(() => result.current.setAcknowledged(true));

    mockGetStatus.mockResolvedValue(status(9n, V.READY));
    await act(async () => {
      await result.current.confirm();
    });
    expect(result.current.acknowledged).toBe(false);
    expect(result.current.canEnforce).toBe(false);
  });

  it("refuses the write and surfaces the reason when the re-check fails", async () => {
    mockGetStatus.mockResolvedValue(CLEAN);
    const { result } = renderHook(() => useEnforceReadiness("operator"));
    await waitFor(() => expect(result.current.evidence.phase).toBe("loaded"));

    mockGetStatus.mockRejectedValue(new Error("offline"));
    let allowed = true;
    await act(async () => {
      allowed = await result.current.confirm();
    });
    expect(allowed).toBe(false);
    expect(result.current.recheck).toEqual({ phase: "failed", error: "offline" });
  });

  it("treats evidence that failed at open and then loads on re-check as a change to confirm", async () => {
    mockGetStatus.mockRejectedValueOnce(new Error("down"));
    const { result } = renderHook(() => useEnforceReadiness("operator"));
    await waitFor(() => expect(result.current.evidence.phase).toBe("failed"));
    act(() => result.current.setAcknowledged(true));

    mockGetStatus.mockResolvedValue(CLEAN);
    let allowed = true;
    await act(async () => {
      allowed = await result.current.confirm();
    });
    expect(allowed).toBe(false);
    expect(result.current.evidence.phase).toBe("loaded");
    expect(result.current.acknowledged).toBe(false);
  });
});
