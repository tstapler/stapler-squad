/**
 * Tests for useGenerateRule hook.
 *
 * Covers:
 *  - useGenerateRule_should_setSuggestion_When_RPCSucceeds
 *  - useGenerateRule_should_setError_When_RPCFails
 *  - useGenerateRule_should_clearError_On_SecondGenerate
 *  - useGenerateRule_should_notSetError_When_Cancelled
 *  - useGenerateRule_should_resetLoading_After_Cancel
 *  - loading=true during call, false after
 */

import { renderHook, act, waitFor } from "@testing-library/react";
import { useGenerateRule } from "@/lib/hooks/useGenerateRule";
import { SuggestionSource } from "@/gen/session/v1/types_pb";
import type { GenerateRuleRequest } from "@/lib/hooks/useGenerateRule";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockGenerateSuggestedRule = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    generateSuggestedRule: mockGenerateSuggestedRule,
  }),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({ unary: jest.fn(), stream: jest.fn() }),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
}));

jest.mock("@bufbuild/protobuf", () => ({
  create: (_schema: unknown, fields: Record<string, unknown> = {}) => ({ ...fields }),
}));

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeSuggestion(overrides: Record<string, unknown> = {}) {
  return {
    name: "Allow git status",
    toolName: "Bash",
    toolPattern: "",
    commandPattern: "^git status",
    filePattern: "",
    decision: 1,
    riskLevel: "low",
    reason: "Read-only git command",
    alternative: "",
    priority: 100,
    confidence: 0.9,
    explanation: "Safe read-only git operation",
    sourceCommands: ["git status"],
    shadowedByRuleIds: [],
    shadowsRuleIds: [],
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useGenerateRule", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe("initial state", () => {
    it("starts with empty suggestions, loading=false, error=null", () => {
      const { result } = renderHook(() => useGenerateRule());

      expect(result.current.suggestions).toEqual([]);
      expect(result.current.loading).toBe(false);
      expect(result.current.error).toBeNull();
    });
  });

  describe("successful generate call", () => {
    it("useGenerateRule_should_setSuggestion_When_RPCSucceeds", async () => {
      const suggestion = makeSuggestion();
      mockGenerateSuggestedRule.mockResolvedValue({ suggestions: [suggestion] });

      const { result } = renderHook(() => useGenerateRule());

      await act(async () => {
        await result.current.generate({ source: SuggestionSource.ANALYTICS_GAPS });
      });

      expect(result.current.suggestions).toHaveLength(1);
      expect(result.current.suggestions[0].name).toBe("Allow git status");
      expect(result.current.loading).toBe(false);
      expect(result.current.error).toBeNull();
    });

    it("sets loading=true during call and false after", async () => {
      let resolveRpc!: (value: { suggestions: ReturnType<typeof makeSuggestion>[] }) => void;
      const rpcPromise = new Promise<{ suggestions: ReturnType<typeof makeSuggestion>[] }>((resolve) => {
        resolveRpc = resolve;
      });
      mockGenerateSuggestedRule.mockReturnValue(rpcPromise);

      const { result } = renderHook(() => useGenerateRule());

      let generatePromise!: Promise<void>;
      act(() => {
        generatePromise = result.current.generate({ source: SuggestionSource.ANALYTICS_GAPS });
      });

      // Loading should be true while RPC is in-flight.
      await waitFor(() => expect(result.current.loading).toBe(true));

      // Resolve the RPC.
      act(() => {
        resolveRpc({ suggestions: [makeSuggestion()] });
      });

      await act(async () => {
        await generatePromise;
      });

      expect(result.current.loading).toBe(false);
    });
  });

  describe("error handling", () => {
    it("useGenerateRule_should_setError_When_RPCFails", async () => {
      const rpcError = new Error("RPC timeout");
      mockGenerateSuggestedRule.mockRejectedValue(rpcError);

      const { result } = renderHook(() => useGenerateRule());

      await act(async () => {
        await result.current.generate({ source: SuggestionSource.ANALYTICS_GAPS });
      });

      expect(result.current.error).toBe(rpcError);
      expect(result.current.loading).toBe(false);
      expect(result.current.suggestions).toEqual([]);
    });

    it("useGenerateRule_should_clearError_On_SecondGenerate", async () => {
      // First call fails.
      const rpcError = new Error("first failure");
      mockGenerateSuggestedRule.mockRejectedValueOnce(rpcError);
      // Second call succeeds.
      mockGenerateSuggestedRule.mockResolvedValueOnce({ suggestions: [] });

      const { result } = renderHook(() => useGenerateRule());

      await act(async () => {
        await result.current.generate({ source: SuggestionSource.ANALYTICS_GAPS });
      });

      expect(result.current.error).toBe(rpcError);

      await act(async () => {
        await result.current.generate({ source: SuggestionSource.ANALYTICS_GAPS });
      });

      // Error cleared on second attempt.
      expect(result.current.error).toBeNull();
    });
  });

  describe("cancellation", () => {
    it("useGenerateRule_should_notSetError_When_Cancelled", async () => {
      // Simulate an AbortError (what fetch/connectrpc throws on abort).
      const abortError = new Error("The operation was aborted");
      abortError.name = "AbortError";
      mockGenerateSuggestedRule.mockRejectedValue(abortError);

      const { result } = renderHook(() => useGenerateRule());

      await act(async () => {
        await result.current.generate({ source: SuggestionSource.ANALYTICS_GAPS });
      });

      // AbortError must NOT set error state.
      expect(result.current.error).toBeNull();
    });

    it("useGenerateRule_should_resetLoading_After_Cancel", async () => {
      let rejectRpc!: (err: Error) => void;
      const rpcPromise = new Promise<{ suggestions: unknown[] }>((_, reject) => {
        rejectRpc = reject;
      });
      mockGenerateSuggestedRule.mockReturnValue(rpcPromise);

      const { result } = renderHook(() => useGenerateRule());

      act(() => {
        void result.current.generate({ source: SuggestionSource.ANALYTICS_GAPS });
      });

      await waitFor(() => expect(result.current.loading).toBe(true));

      // cancel() sets loading=false immediately (before RPC resolves).
      act(() => {
        result.current.cancel();
      });

      expect(result.current.loading).toBe(false);

      // Settle the promise to avoid unhandled-rejection warnings.
      const abortError = new Error("aborted");
      abortError.name = "AbortError";
      act(() => {
        rejectRpc(abortError);
      });
    });
  });

  describe("clear()", () => {
    it("resets suggestions and error to initial state", async () => {
      mockGenerateSuggestedRule.mockResolvedValue({ suggestions: [makeSuggestion()] });

      const { result } = renderHook(() => useGenerateRule());

      await act(async () => {
        await result.current.generate({ source: SuggestionSource.ANALYTICS_GAPS });
      });

      expect(result.current.suggestions).toHaveLength(1);

      act(() => {
        result.current.clear();
      });

      expect(result.current.suggestions).toEqual([]);
      expect(result.current.error).toBeNull();
    });
  });
});
