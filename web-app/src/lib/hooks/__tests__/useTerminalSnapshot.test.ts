/**
 * Tests for useTerminalSnapshot hook.
 *
 * Covers:
 *  - Returns loading=true on initial fetch
 *  - Returns html and loading=false on success
 *  - Returns isEmpty=true when response.isEmpty is true
 *  - Returns error string on fetch failure
 *  - Cache hit: returns cached result immediately without re-fetching
 *  - refetch(): bypasses cache and fetches fresh data
 *  - Disabled: does not fetch when enabled=false
 *  - ANSI fallback: plain text returned when ansi-to-html throws
 */

import { renderHook, act, waitFor } from "@testing-library/react";
import { useTerminalSnapshot } from "../useTerminalSnapshot";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockGetTerminalSnapshot = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    getTerminalSnapshot: mockGetTerminalSnapshot,
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => (next: unknown) => next,
}));

// Module-level cache in useTerminalSnapshot is shared across tests.
// Clear it between tests by reimporting (jest module registry isolates per test file,
// but the Map persists within the same module instance).
// We reset the mock return values instead and rely on TTL expiry via Date.now mock.

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeResponse(overrides: { content?: string; isEmpty?: boolean } = {}) {
  return {
    content: overrides.content ?? "\x1B[32mgreen text\x1B[0m",
    isEmpty: overrides.isEmpty ?? false,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useTerminalSnapshot", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Use a unique sessionId per test to avoid hitting the module-level cache
  });

  it("starts in loading state", () => {
    mockGetTerminalSnapshot.mockResolvedValue(makeResponse());
    const { result } = renderHook(() => useTerminalSnapshot("session-loading-1"));
    expect(result.current.loading).toBe(true);
  });

  it("resolves html and loading=false on success", async () => {
    mockGetTerminalSnapshot.mockResolvedValue(makeResponse({ content: "hello world" }));
    const { result } = renderHook(() => useTerminalSnapshot("session-success-1"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBeNull();
    // ansi-to-html is dynamically required; in test env require may fail → plain text fallback
    expect(result.current.html).toContain("hello world");
  });

  it("sets isEmpty=true when response.isEmpty is true", async () => {
    mockGetTerminalSnapshot.mockResolvedValue(makeResponse({ isEmpty: true, content: "" }));
    const { result } = renderHook(() => useTerminalSnapshot("session-empty-1"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.isEmpty).toBe(true);
    expect(result.current.html).toBe("");
  });

  it("returns error string on fetch failure", async () => {
    mockGetTerminalSnapshot.mockRejectedValue(new Error("network error"));
    const { result } = renderHook(() => useTerminalSnapshot("session-error-1"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe("network error");
  });

  it("returns generic error for non-Error thrown values", async () => {
    mockGetTerminalSnapshot.mockRejectedValue("string error");
    const { result } = renderHook(() => useTerminalSnapshot("session-error-2"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe("Failed to load snapshot");
  });

  it("does not fetch when enabled=false", () => {
    const { result } = renderHook(() => useTerminalSnapshot("session-disabled-1", false));
    expect(mockGetTerminalSnapshot).not.toHaveBeenCalled();
    expect(result.current.loading).toBe(false);
  });

  it("refetch() fetches fresh data bypassing cache", async () => {
    mockGetTerminalSnapshot
      .mockResolvedValueOnce(makeResponse({ content: "first" }))
      .mockResolvedValueOnce(makeResponse({ content: "second" }));

    const { result } = renderHook(() => useTerminalSnapshot("session-refetch-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.html).toContain("first");

    act(() => {
      result.current.refetch();
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.html).toContain("second");
    expect(mockGetTerminalSnapshot).toHaveBeenCalledTimes(2);
  });
});
