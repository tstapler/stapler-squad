/**
 * Tests for useValidateRules hook.
 * Covers UT-FE-01 through UT-FE-04.
 */

import { renderHook, act, waitFor } from "@testing-library/react";
import { useValidateRules } from "@/lib/hooks/useValidateRules";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockValidateRules = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    validateRules: mockValidateRules,
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn().mockReturnValue({}),
}));

jest.mock("@bufbuild/protobuf", () => ({
  create: (_schema: unknown, fields: Record<string, unknown> = {}) => ({ ...fields }),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeValidResult(name: string) {
  return { originalName: name, valid: true, errors: [], rule: { name, toolName: "Bash", decision: 1, priority: 10, enabled: true } };
}

function makeErrorResult(name: string) {
  return { originalName: name, valid: false, errors: ["name is required"], rule: undefined };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useValidateRules", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("UT-FE-01: useValidateRules_returns_results_after_debounce", async () => {
    mockValidateRules.mockResolvedValueOnce({
      results: [makeValidResult("Rule 1"), makeValidResult("Rule 2"), makeErrorResult("Rule 3")],
      validCount: 2,
      errorCount: 1,
    });

    const { result } = renderHook(() => useValidateRules("rules:\n- name: test\n", 400));

    // Before debounce fires: loading is still false, results empty
    expect(result.current.results).toEqual([]);
    expect(result.current.loading).toBe(false);

    // Advance past debounce
    act(() => {
      jest.advanceTimersByTime(400);
    });

    // After debounce, loading should be true then resolve
    await waitFor(() => {
      expect(mockValidateRules).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(result.current.validCount).toBe(2);
      expect(result.current.errorCount).toBe(1);
      expect(result.current.results).toHaveLength(3);
    });
  });

  it("UT-FE-02: useValidateRules_clears_results_on_empty_input", async () => {
    mockValidateRules.mockResolvedValueOnce({
      results: [makeValidResult("Rule 1")],
      validCount: 1,
      errorCount: 0,
    });

    const { result, rerender } = renderHook(({ yaml }) => useValidateRules(yaml, 400), {
      initialProps: { yaml: "rules:\n- name: test\n" },
    });

    act(() => { jest.advanceTimersByTime(400); });
    await waitFor(() => { expect(result.current.validCount).toBe(1); });

    // Now clear the input
    rerender({ yaml: "" });

    await waitFor(() => {
      expect(result.current.results).toEqual([]);
      expect(result.current.validCount).toBe(0);
      expect(result.current.errorCount).toBe(0);
    });
    // RPC should not be called again for empty input
    expect(mockValidateRules).toHaveBeenCalledTimes(1);
  });

  it("UT-FE-03: useValidateRules_cancels_inflight_on_new_input", async () => {
    // First call blocks, second resolves immediately
    let firstAborted = false;
    mockValidateRules.mockImplementationOnce((_req: unknown, opts: { signal?: AbortSignal }) => {
      return new Promise((_resolve, reject) => {
        opts?.signal?.addEventListener("abort", () => {
          firstAborted = true;
          reject(Object.assign(new Error("AbortError"), { name: "AbortError" }));
        });
      });
    });
    mockValidateRules.mockResolvedValueOnce({
      results: [makeValidResult("Rule X")],
      validCount: 1,
      errorCount: 0,
    });

    const { result, rerender } = renderHook(({ yaml }) => useValidateRules(yaml, 100), {
      initialProps: { yaml: "yaml1" },
    });

    act(() => { jest.advanceTimersByTime(100); });
    // Immediately update input to trigger abort
    rerender({ yaml: "yaml2" });
    act(() => { jest.advanceTimersByTime(100); });

    await waitFor(() => {
      expect(firstAborted).toBe(true);
    });

    await waitFor(() => {
      expect(result.current.validCount).toBe(1);
    });
  });

  it("UT-FE-04: useValidateRules_sets_error_on_rpc_failure", async () => {
    mockValidateRules.mockRejectedValueOnce(new Error("Server error"));

    const { result } = renderHook(() => useValidateRules("some yaml content", 100));

    act(() => { jest.advanceTimersByTime(100); });

    await waitFor(() => {
      expect(result.current.error).not.toBeNull();
      expect(result.current.error?.message).toBe("Server error");
      expect(result.current.results).toEqual([]);
    });
  });
});
