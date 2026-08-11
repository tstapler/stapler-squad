/**
 * Tests for usePathCompletions hook.
 *
 * Mocks ConnectRPC to return controlled responses.
 * Uses fake timers to test 150ms debounce deterministically.
 */

import { renderHook, act } from "@testing-library/react";
import { usePathCompletions, clearCompletionCache } from "../usePathCompletions";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockListPathCompletions = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    listPathCompletions: mockListPathCompletions,
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));

jest.mock("@/gen/session/v1/session_pb", () => ({
  SessionService: {},
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const makeResponse = (overrides = {}) => ({
  entries: [],
  baseDir: "/home/user",
  baseDirExists: true,
  pathExists: false,
  truncated: false,
  ...overrides,
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("usePathCompletions", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockListPathCompletions.mockResolvedValue(makeResponse());
  });

  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
    jest.clearAllMocks();
    // The module-level LRU cache persists across tests; clear it so prior results
    // don't cause false cache hits in subsequent tests.
    clearCompletionCache();
  });

  it("returns empty state for empty pathPrefix", () => {
    const { result } = renderHook(() => usePathCompletions(""));
    expect(result.current.entries).toEqual([]);
    expect(result.current.isLoading).toBe(false);
    expect(mockListPathCompletions).not.toHaveBeenCalled();
  });

  it("does not fetch when enabled=false", async () => {
    renderHook(() =>
      usePathCompletions("/home/user/proj", { enabled: false })
    );
    act(() => { jest.advanceTimersByTime(300); });
    expect(mockListPathCompletions).not.toHaveBeenCalled();
  });

  it("fetches after 150ms debounce", async () => {
    renderHook(() => usePathCompletions("/home/user/proj"));

    // Not called before debounce fires.
    act(() => { jest.advanceTimersByTime(149); });
    expect(mockListPathCompletions).not.toHaveBeenCalled();

    // Called after debounce.
    await act(async () => { jest.advanceTimersByTime(1); });
    expect(mockListPathCompletions).toHaveBeenCalledTimes(1);
    expect(mockListPathCompletions).toHaveBeenCalledWith(
      expect.objectContaining({ pathPrefix: "/home/user/proj" }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
  });

  it("passes directoriesOnly and maxResults to the RPC", async () => {
    renderHook(() =>
      usePathCompletions("/home/user/", {
        directoriesOnly: true,
        maxResults: 25,
      })
    );
    await act(async () => { jest.advanceTimersByTime(150); });
    expect(mockListPathCompletions).toHaveBeenCalledWith(
      expect.objectContaining({
        directoriesOnly: true,
        maxResults: 25,
      }),
      expect.any(Object)
    );
  });

  it("surfaces RPC results in state", async () => {
    const entries = [
      { path: "/home/user/projects", name: "projects", isDirectory: true },
    ];
    mockListPathCompletions.mockResolvedValue(
      makeResponse({ entries, baseDirExists: true, pathExists: true, baseDir: "/home/user" })
    );

    const { result } = renderHook(() => usePathCompletions("/home/user/proj"));
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(result.current.entries).toEqual(entries);
    expect(result.current.baseDirExists).toBe(true);
    expect(result.current.pathExists).toBe(true);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("sets error on RPC failure", async () => {
    mockListPathCompletions.mockRejectedValue(new Error("network error"));

    const { result } = renderHook(() => usePathCompletions("/home/user/proj"));
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(result.current.error).toBe("network error");
    expect(result.current.isLoading).toBe(false);
  });

  it("cancels debounce timer when prefix changes quickly", async () => {
    const { rerender } = renderHook(
      ({ prefix }: { prefix: string }) => usePathCompletions(prefix),
      { initialProps: { prefix: "/home/user/a" } }
    );

    act(() => { jest.advanceTimersByTime(100); });

    // Change prefix before debounce fires.
    rerender({ prefix: "/home/user/b" });

    await act(async () => { jest.advanceTimersByTime(150); });

    // Only one call with the latest prefix.
    expect(mockListPathCompletions).toHaveBeenCalledTimes(1);
    expect(mockListPathCompletions).toHaveBeenCalledWith(
      expect.objectContaining({ pathPrefix: "/home/user/b" }),
      expect.any(Object)
    );
  });

  it("serves from cache on repeated identical prefix", async () => {
    mockListPathCompletions.mockResolvedValue(makeResponse({ entries: [] }));

    const { rerender } = renderHook(
      ({ prefix }: { prefix: string }) => usePathCompletions(prefix),
      { initialProps: { prefix: "/home/user/cached" } }
    );
    await act(async () => { jest.advanceTimersByTime(150); });
    expect(mockListPathCompletions).toHaveBeenCalledTimes(1);

    // Change to a different prefix and back.
    rerender({ prefix: "/home/user/other" });
    await act(async () => { jest.advanceTimersByTime(150); });
    rerender({ prefix: "/home/user/cached" });

    // Should NOT re-fetch for the cached prefix (no timer advance needed).
    expect(mockListPathCompletions).toHaveBeenCalledTimes(2); // only /other triggered a second call
  });

  it("resets entries when pathPrefix becomes empty", async () => {
    mockListPathCompletions.mockResolvedValue(
      makeResponse({ entries: [{ path: "/home/user/a", name: "a", isDirectory: true }] })
    );
    const { rerender, result } = renderHook(
      ({ prefix }: { prefix: string }) => usePathCompletions(prefix),
      { initialProps: { prefix: "/home/user/a" } }
    );
    await act(async () => { jest.advanceTimersByTime(150); });
    expect(result.current.entries.length).toBe(1);

    rerender({ prefix: "" });
    expect(result.current.entries).toEqual([]);
  });
});
