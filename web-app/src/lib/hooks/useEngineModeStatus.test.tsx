/**
 * useEngineModeStatus (Story 1.3.2): per-class mapping, fail-closed states,
 * stale handling and focus/visibility refetch debouncing.
 */

import { renderHook, act, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import { useEngineModeStatus, readinessFromResponse, bannerText, differencesText, ENGINE_STATUS_REFETCH_WINDOW_MS } from "./useEngineModeStatus";
import {
  ClassReadinessSchema,
  GetWorkflowEngineStatusResponseSchema,
  WorkflowEngineSoakVerdict,
  WorkflowEngineMode,
  WorkflowEngineReviewState,
} from "@/gen/session/v1/backlog_pb";

const mockGetStatus = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getWorkflowEngineStatus: (...a: unknown[]) => mockGetStatus(...a) }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

const M = WorkflowEngineMode;

function evResponse(
  modes: { operator?: WorkflowEngineMode; agent?: WorkflowEngineMode; system?: WorkflowEngineMode },
  extra: Record<string, unknown> = {}
) {
  return create(GetWorkflowEngineStatusResponseSchema, {
    classModes: modes,
    divergence24h: 7n,
    gateWired: true,
    invalidatorWired: true,
    ...extra,
  });
}

beforeEach(() => {
  mockGetStatus.mockReset();
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => jest.restoreAllMocks());

describe("useEngineModeStatus", () => {
  it("starts in loading, then maps per-class modes", async () => {
    mockGetStatus.mockResolvedValue(evResponse({ operator: M.ENFORCE, agent: M.SHADOW, system: M.SHADOW }));
    const { result } = renderHook(() => useEngineModeStatus());
    expect(result.current.view.kind).toBe("loading");
    await waitFor(() => expect(result.current.view.kind).toBe("partial"));
    expect(result.current.view.classes).toEqual({ operator: "enforce", agent: "shadow", system: "shadow" });
    expect(result.current.view.divergence24h).toBe(7);
    expect(result.current.view.flagRowId).toBe("flag-row-backlog-workflow-engine-enforce-agent");
  });

  it.each([
    [{ operator: M.ENFORCE, agent: M.ENFORCE, system: M.ENFORCE }, "enforced"],
    [{ operator: M.SHADOW, agent: M.SHADOW, system: M.SHADOW }, "shadow"],
    [{ operator: M.OFF, agent: M.OFF, system: M.OFF }, "off"],
    [{ operator: M.OFF, agent: M.SHADOW, system: M.OFF }, "shadow"],
    // A class the server omitted cannot be assumed: fail closed.
    [{ operator: M.ENFORCE, agent: M.ENFORCE }, "unknown"],
  ])("classifies %j as %s", async (modes, kind) => {
    mockGetStatus.mockResolvedValue(evResponse(modes));
    const { result } = renderHook(() => useEngineModeStatus());
    await waitFor(() => expect(result.current.view.kind).toBe(kind));
  });

  it("states the 200 divergence cap as a floor in the shadow banner", async () => {
    mockGetStatus.mockResolvedValue(evResponse({ operator: M.SHADOW, agent: M.SHADOW, system: M.SHADOW }, { divergence24h: 200n }));
    const { result } = renderHook(() => useEngineModeStatus());
    await waitFor(() => expect(result.current.view.kind).toBe("shadow"));
    expect(bannerText(result.current.view)).toContain("at least 200 differences in the last 24h");
    expect(differencesText(199)).toBe("199");
  });

  it("treats an unwired gate as unknown even when every class says enforce", async () => {
    mockGetStatus.mockResolvedValue(
      evResponse({ operator: M.ENFORCE, agent: M.ENFORCE, system: M.ENFORCE }, { gateWired: false })
    );
    const { result } = renderHook(() => useEngineModeStatus());
    await waitFor(() => expect(result.current.view.kind).toBe("unknown"));
  });

  it("RPC failure with no prior value is unknown", async () => {
    mockGetStatus.mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() => useEngineModeStatus());
    await waitFor(() => expect(result.current.view.kind).toBe("unknown"));
  });

  it("a failed refetch after a good value keeps it and marks it stale; recovery clears stale", async () => {
    mockGetStatus.mockResolvedValueOnce(evResponse({ operator: M.SHADOW, agent: M.SHADOW, system: M.SHADOW }));
    const { result } = renderHook(() => useEngineModeStatus());
    await waitFor(() => expect(result.current.view.kind).toBe("shadow"));

    mockGetStatus.mockRejectedValueOnce(new Error("down"));
    await act(() => result.current.refetch());
    expect(result.current.view.kind).toBe("shadow");
    expect(result.current.view.stale).toBe(true);

    mockGetStatus.mockResolvedValueOnce(evResponse({ operator: M.ENFORCE, agent: M.ENFORCE, system: M.ENFORCE }));
    await act(() => result.current.refetch());
    expect(result.current.view.kind).toBe("enforced");
    expect(result.current.view.stale).toBe(false);
  });

  it("maps the review fields and omits the line when the ledger is unavailable", async () => {
    const due = create(TimestampSchema, { seconds: BigInt(Date.UTC(2026, 10, 1) / 1000) });
    mockGetStatus.mockResolvedValueOnce(
      evResponse({ operator: M.SHADOW, agent: M.SHADOW, system: M.SHADOW }, {
        reviewDueAt: due,
        reviewOverdue: true,
        reviewState: WorkflowEngineReviewState.OVERDUE,
      })
    );
    const { result } = renderHook(() => useEngineModeStatus());
    await waitFor(() => expect(result.current.view.review).toEqual({ state: "overdue", date: "2026-11-01" }));

    mockGetStatus.mockResolvedValueOnce(
      evResponse({ operator: M.SHADOW, agent: M.SHADOW, system: M.SHADOW }, {
        reviewState: WorkflowEngineReviewState.UNAVAILABLE,
      })
    );
    await act(() => result.current.refetch());
    expect(result.current.view.review).toBeNull();
  });

  describe("focus and visibility refetch", () => {
    beforeEach(() => jest.useFakeTimers({ doNotFake: ["nextTick", "setImmediate"] }));
    afterEach(() => jest.useRealTimers());

    it("refetches once after the window, coalescing focus and visibilitychange", async () => {
      mockGetStatus.mockResolvedValue(evResponse({ operator: M.SHADOW, agent: M.SHADOW, system: M.SHADOW }));
      renderHook(() => useEngineModeStatus());
      await waitFor(() => expect(mockGetStatus).toHaveBeenCalledTimes(1));

      // Inside the 5 s window: ignored.
      act(() => {
        window.dispatchEvent(new Event("focus"));
      });
      expect(mockGetStatus).toHaveBeenCalledTimes(1);

      jest.advanceTimersByTime(ENGINE_STATUS_REFETCH_WINDOW_MS + 1);
      await act(async () => {
        window.dispatchEvent(new Event("focus"));
        document.dispatchEvent(new Event("visibilitychange"));
      });
      expect(mockGetStatus).toHaveBeenCalledTimes(2);
    });
  });
});

describe("readinessFromResponse", () => {
  it("maps one class's evidence and returns null for a class the server did not report", () => {
    const resp = evResponse(
      {},
      {
        classReadiness: {
          agent: create(ClassReadinessSchema, {
            divergence24h: 3n,
            soakVerdict: WorkflowEngineSoakVerdict.NEEDS_OWNER_ACK,
            strandedItemCount: 2n,
          }),
        },
      }
    );
    expect(readinessFromResponse(resp, "agent")).toEqual({ divergence24h: 3, verdict: "needs_ack", strandedItemCount: 2 });
    expect(readinessFromResponse(resp, "system")).toBeNull();
  });

  it("maps an unspecified verdict to unavailable rather than ready", () => {
    const resp = evResponse({}, { classReadiness: { operator: create(ClassReadinessSchema, {}) } });
    expect(readinessFromResponse(resp, "operator")?.verdict).toBe("unavailable");
  });
});
