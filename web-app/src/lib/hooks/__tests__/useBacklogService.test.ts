/**
 * Regression tests for useBacklogService stability.
 *
 * These tests exist to prevent the infinite-reload bug caused by the hook
 * returning a new plain object on every render. Any call site that puts the
 * hook's return value into a useCallback/useEffect dep array will re-run on
 * every render if the reference is unstable — turning "load on mount" into
 * an infinite loop and causing focus loss in modals.
 *
 * The tests below MUST fail if useMemo is removed from useBacklogService's
 * return statement and the plain object literal is restored.
 */

import { renderHook } from "@testing-library/react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { useBacklogService } from "@/lib/hooks/useBacklogService";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({})),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => () => ({}),
}));

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useBacklogService", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (createClient as jest.Mock).mockReturnValue({});
    (createConnectTransport as jest.Mock).mockReturnValue({});
  });

  // T-STAB-001
  it("returns the same object reference across re-renders when lastError has not changed", () => {
    const { result, rerender } = renderHook(() => useBacklogService());

    const firstRender = result.current;
    rerender();
    const secondRender = result.current;

    // This assertion fails against pre-fix code (plain object literal returned each render).
    expect(secondRender).toBe(firstRender);
  });

  // T-STAB-002
  it("returns stable method references across re-renders", () => {
    const { result, rerender } = renderHook(() => useBacklogService());

    const methodsBefore = {
      listBacklogItems: result.current.listBacklogItems,
      getBacklogItem: result.current.getBacklogItem,
      createBacklogItem: result.current.createBacklogItem,
      transitionStatus: result.current.transitionStatus,
    };

    rerender();

    expect(result.current.listBacklogItems).toBe(methodsBefore.listBacklogItems);
    expect(result.current.getBacklogItem).toBe(methodsBefore.getBacklogItem);
    expect(result.current.createBacklogItem).toBe(methodsBefore.createBacklogItem);
    expect(result.current.transitionStatus).toBe(methodsBefore.transitionStatus);
  });
});
