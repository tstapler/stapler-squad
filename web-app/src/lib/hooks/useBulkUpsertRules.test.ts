/**
 * Tests for useBulkUpsertRules hook.
 * Covers UT-FE-08 through UT-FE-10.
 */

import { renderHook, act } from "@testing-library/react";
import { useBulkUpsertRules } from "@/lib/hooks/useBulkUpsertRules";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockBulkUpsertRules = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    bulkUpsertRules: mockBulkUpsertRules,
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: jest.fn().mockReturnValue({}),
}));

jest.mock("@bufbuild/protobuf", () => ({
  create: (_schema: unknown, fields: Record<string, unknown> = {}) => ({ ...fields }),
}));

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useBulkUpsertRules", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("UT-FE-08: useBulkUpsertRules_returns_counts_on_success", async () => {
    mockBulkUpsertRules.mockResolvedValueOnce({
      created: 3,
      updated: 1,
      skipped: 1,
      errors: [],
    });

    const { result } = renderHook(() => useBulkUpsertRules());

    await act(async () => {
      await result.current.applyRules([], false);
    });

    expect(result.current.result?.created).toBe(3);
    expect(result.current.result?.updated).toBe(1);
    expect(result.current.result?.skipped).toBe(1);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("UT-FE-09: useBulkUpsertRules_passes_overwriteDuplicates_flag", async () => {
    mockBulkUpsertRules.mockResolvedValueOnce({ created: 0, updated: 2, skipped: 0, errors: [] });

    const { result } = renderHook(() => useBulkUpsertRules());
    const rules = [{ name: "Rule 1" } as never, { name: "Rule 2" } as never];

    await act(async () => {
      await result.current.applyRules(rules, true);
    });

    const callArg = mockBulkUpsertRules.mock.calls[0][0];
    expect(callArg.overwriteDuplicates).toBe(true);
  });

  it("UT-FE-10: useBulkUpsertRules_sets_error_on_rpc_failure", async () => {
    mockBulkUpsertRules.mockRejectedValueOnce(new Error("Apply rules failed"));

    const { result } = renderHook(() => useBulkUpsertRules());

    await act(async () => {
      await result.current.applyRules([], false);
    });

    expect(result.current.error?.message).toBe("Apply rules failed");
    expect(result.current.result).toBeNull();
  });
});
