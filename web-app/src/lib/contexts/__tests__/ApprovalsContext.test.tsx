import React from "react";
import { renderHook, act } from "@testing-library/react";
import { ApprovalsProvider, useApprovalsContext } from "../ApprovalsContext";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockRefetch = jest.fn().mockResolvedValue(undefined);
const mockResolveApproval = jest.fn().mockResolvedValue({ data: undefined });

// Mutable approvals array so individual tests can override it
let mockApprovalData = [
  {
    id: "a1",
    sessionId: "s1",
    secondsRemaining: 30,
    toolName: "bash",
    toolInput: {},
    cwd: "/tmp",
    permissionMode: "default",
    createdAt: undefined,
    expiresAt: undefined,
  },
];

jest.mock("@/lib/api/approvalsApi", () => ({
  useGetApprovalsQuery: () => ({
    data: { approvals: mockApprovalData },
    isLoading: false,
    error: null,
    refetch: mockRefetch,
  }),
  useResolveApprovalMutation: () => [mockResolveApproval, {}],
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function wrapper({ children }: { children: React.ReactNode }) {
  return <ApprovalsProvider>{children}</ApprovalsProvider>;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ApprovalsContext", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Reset to default single-approval data
    mockApprovalData = [
      {
        id: "a1",
        sessionId: "s1",
        secondsRemaining: 30,
        toolName: "bash",
        toolInput: {},
        cwd: "/tmp",
        permissionMode: "default",
        createdAt: undefined,
        expiresAt: undefined,
      },
    ];
  });

  describe("useApprovalsContext outside ApprovalsProvider", () => {
    it("returns FALLBACK_CONTEXT when called outside ApprovalsProvider (no throw)", () => {
      const { result } = renderHook(() => useApprovalsContext());

      expect(result.current.approvals).toEqual([]);
      expect(result.current.pendingCount).toBe(0);
      expect(result.current.loading).toBe(false);
      expect(result.current.error).toBeNull();
      expect(typeof result.current.approve).toBe("function");
      expect(typeof result.current.deny).toBe("function");
      expect(typeof result.current.refresh).toBe("function");
    });

    it("fallback approve is a no-op async function", async () => {
      const { result } = renderHook(() => useApprovalsContext());
      await expect(result.current.approve("any-id")).resolves.toBeUndefined();
    });

    it("fallback deny is a no-op async function", async () => {
      const { result } = renderHook(() => useApprovalsContext());
      await expect(result.current.deny("any-id")).resolves.toBeUndefined();
    });

    it("fallback refresh is a no-op async function", async () => {
      const { result } = renderHook(() => useApprovalsContext());
      await expect(result.current.refresh()).resolves.toBeUndefined();
    });
  });

  describe("useApprovalsContext inside ApprovalsProvider", () => {
    it("returns approvals from provider", () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      expect(result.current.approvals).toHaveLength(1);
      expect(result.current.approvals[0].id).toBe("a1");
      expect(result.current.approvals[0].sessionId).toBe("s1");
    });

    it("pendingCount matches approvals.length", () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      expect(result.current.pendingCount).toBe(result.current.approvals.length);
      expect(result.current.pendingCount).toBe(1);
    });

    it("loading is false when isLoading is false", () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      expect(result.current.loading).toBe(false);
    });

    it("error is null when query succeeds", () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      expect(result.current.error).toBeNull();
    });

    it("approve calls resolveApproval with allow decision", async () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      await result.current.approve("a1");

      expect(mockResolveApproval).toHaveBeenCalledWith({
        approvalId: "a1",
        decision: "allow",
      });
    });

    it("deny calls resolveApproval with deny decision", async () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      await result.current.deny("a1", "not safe");

      expect(mockResolveApproval).toHaveBeenCalledWith({
        approvalId: "a1",
        decision: "deny",
        message: "not safe",
      });
    });

    it("refresh calls refetch", async () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      await result.current.refresh();

      expect(mockRefetch).toHaveBeenCalledTimes(1);
    });
  });

  describe("clearForSession", () => {
    it("T-UNIT-TS-001: clearForSession_should_filterApprovals_When_sessionIdIsCleared", async () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      // Initially s1's approval is present
      expect(result.current.approvals).toHaveLength(1);
      expect(result.current.approvals[0].sessionId).toBe("s1");

      // clearForSession is called synchronously for the optimistic clear
      act(() => {
        result.current.clearForSession("s1");
      });

      // Approval for s1 should be filtered out immediately
      expect(result.current.approvals.filter(a => a.sessionId === "s1")).toHaveLength(0);
    });

    it("T-UNIT-TS-002: clearForSession_should_decrementPendingCount_When_sessionIsCleared", async () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      expect(result.current.pendingCount).toBe(1);

      act(() => {
        result.current.clearForSession("s1");
      });

      expect(result.current.pendingCount).toBe(0);
    });

    it("T-UNIT-TS-003: clearForSession_should_notAffectOtherSessions_When_differentSessionIdCleared", async () => {
      mockApprovalData = [
        { id: "a1", sessionId: "s1", secondsRemaining: 30, toolName: "bash", toolInput: {}, cwd: "/tmp", permissionMode: "default", createdAt: undefined, expiresAt: undefined },
        { id: "a2", sessionId: "s2", secondsRemaining: 60, toolName: "bash", toolInput: {}, cwd: "/tmp", permissionMode: "default", createdAt: undefined, expiresAt: undefined },
      ];

      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      expect(result.current.approvals).toHaveLength(2);

      act(() => {
        result.current.clearForSession("s1");
      });

      // s2's approval must still be present
      expect(result.current.approvals.filter(a => a.sessionId === "s2")).toHaveLength(1);
      expect(result.current.pendingCount).toBe(1);
    });

    it("T-UNIT-TS-007: clearForSession_should_beNoOp_When_sessionHasNoApprovals", async () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      const initialCount = result.current.pendingCount;

      act(() => {
        result.current.clearForSession("no-approvals-session");
      });

      // Count unchanged — clearing an empty session is a no-op
      expect(result.current.pendingCount).toBe(initialCount);
    });

    it("T-UNIT-TS-008: clearForSession_should_exposeClearedSessions_When_sessionInClearedSet", async () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      expect(result.current.clearedSessions.has("s1")).toBe(false);

      act(() => {
        result.current.clearForSession("s1");
      });

      expect(result.current.clearedSessions.has("s1")).toBe(true);
    });

    it("T-UNIT-TS-010: FALLBACK_CONTEXT_should_includeNoop_When_usedOutsideProvider", () => {
      const { result } = renderHook(() => useApprovalsContext());

      expect(typeof result.current.clearForSession).toBe("function");
      expect(result.current.clearedSessions).toBeDefined();
      expect(result.current.clearedSessions.has("anything")).toBe(false);

      // Should not throw
      expect(() => result.current.clearForSession("any-session")).not.toThrow();
    });

    it("T-UNIT-TS-009: clearForSession_should_callRefetch_When_invoked", async () => {
      const { result } = renderHook(() => useApprovalsContext(), { wrapper });

      act(() => {
        result.current.clearForSession("s1");
      });

      // Wait for the async refetch to be called
      await act(async () => {
        await Promise.resolve();
      });

      expect(mockRefetch).toHaveBeenCalled();
    });
  });
});
