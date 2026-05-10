import { renderHook } from "@testing-library/react";
import { useApprovals } from "@/lib/hooks/useApprovals";
import { useApprovalsContext } from "@/lib/contexts/ApprovalsContext";
import type { PlainApproval } from "@/lib/api/approvalsApi";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

jest.mock("@/lib/contexts/ApprovalsContext", () => ({
  useApprovalsContext: jest.fn(),
}));

const mockUseApprovalsContext = useApprovalsContext as jest.MockedFunction<
  typeof useApprovalsContext
>;

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const mockApprove = jest.fn().mockResolvedValue(undefined);
const mockDeny = jest.fn().mockResolvedValue(undefined);
const mockRefresh = jest.fn().mockResolvedValue(undefined);

function makeApproval(overrides: Partial<PlainApproval> = {}): PlainApproval {
  return {
    id: "a1",
    sessionId: "s1",
    secondsRemaining: 30,
    toolName: "bash",
    toolInput: {},
    cwd: "/tmp",
    permissionMode: "default",
    createdAt: undefined,
    expiresAt: undefined,
    ...overrides,
  };
}

const baseContextValue = {
  approvals: [
    makeApproval({ id: "a1", sessionId: "s1" }),
    makeApproval({ id: "a2", sessionId: "s2" }),
    makeApproval({ id: "a3", sessionId: "s1" }),
  ],
  pendingCount: 3,
  loading: false,
  error: null,
  approve: mockApprove,
  deny: mockDeny,
  refresh: mockRefresh,
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useApprovals", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseApprovalsContext.mockReturnValue(baseContextValue);
  });

  describe("sessionId filtering", () => {
    it("returns all approvals when no sessionId is provided", () => {
      const { result } = renderHook(() => useApprovals({}));

      expect(result.current.approvals).toHaveLength(3);
    });

    it("returns all approvals when options object is empty", () => {
      const { result } = renderHook(() => useApprovals());

      expect(result.current.approvals).toHaveLength(3);
    });

    it("returns only matching approvals when sessionId is provided", () => {
      const { result } = renderHook(() => useApprovals({ sessionId: "s1" }));

      expect(result.current.approvals).toHaveLength(2);
      expect(result.current.approvals.every((a) => a.sessionId === "s1")).toBe(true);
    });

    it("returns only the single matching approval for sessionId with one match", () => {
      const { result } = renderHook(() => useApprovals({ sessionId: "s2" }));

      expect(result.current.approvals).toHaveLength(1);
      expect(result.current.approvals[0].id).toBe("a2");
    });

    it("returns empty array when sessionId matches no approvals", () => {
      const { result } = renderHook(() => useApprovals({ sessionId: "unknown" }));

      expect(result.current.approvals).toHaveLength(0);
    });
  });

  describe("passthrough from context", () => {
    it("passes loading through from context", () => {
      mockUseApprovalsContext.mockReturnValue({ ...baseContextValue, loading: true });

      const { result } = renderHook(() => useApprovals({}));

      expect(result.current.loading).toBe(true);
    });

    it("passes error through from context", () => {
      const testError = new Error("fetch failed");
      mockUseApprovalsContext.mockReturnValue({ ...baseContextValue, error: testError });

      const { result } = renderHook(() => useApprovals({}));

      expect(result.current.error).toBe(testError);
    });

    it("passes error=null through from context when no error", () => {
      const { result } = renderHook(() => useApprovals({}));

      expect(result.current.error).toBeNull();
    });

    it("passes approve function through", () => {
      const { result } = renderHook(() => useApprovals({}));

      expect(result.current.approve).toBe(mockApprove);
    });

    it("passes deny function through", () => {
      const { result } = renderHook(() => useApprovals({}));

      expect(result.current.deny).toBe(mockDeny);
    });

    it("passes refresh function through", () => {
      const { result } = renderHook(() => useApprovals({}));

      expect(result.current.refresh).toBe(mockRefresh);
    });
  });

  describe("deprecated options", () => {
    it("accepts pollInterval without effect", () => {
      const { result } = renderHook(() => useApprovals({ pollInterval: 1000 }));

      // Still returns all approvals; pollInterval is silently ignored
      expect(result.current.approvals).toHaveLength(3);
    });

    it("accepts notificationTrigger without effect", () => {
      const { result } = renderHook(() => useApprovals({ notificationTrigger: 42 }));

      expect(result.current.approvals).toHaveLength(3);
    });
  });
});
