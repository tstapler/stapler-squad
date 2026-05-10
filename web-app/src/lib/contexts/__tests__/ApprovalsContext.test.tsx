import React from "react";
import { renderHook } from "@testing-library/react";
import { ApprovalsProvider, useApprovalsContext } from "../ApprovalsContext";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockRefetch = jest.fn().mockResolvedValue(undefined);
const mockResolveApproval = jest.fn().mockResolvedValue({ data: undefined });

jest.mock("@/lib/api/approvalsApi", () => ({
  useGetApprovalsQuery: () => ({
    data: {
      approvals: [
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
      ],
    },
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
});
