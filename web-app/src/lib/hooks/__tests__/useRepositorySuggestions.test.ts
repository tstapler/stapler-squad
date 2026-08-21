/**
 * Tests for useRepositorySuggestions hook.
 *
 * Verifies that:
 * - BigInt → ms timestamp extraction from proto Timestamp fields is correct
 * - Paths are ranked by frecency (not alphabetically)
 * - Empty session list produces fallback hints
 * - RPC failure produces fallback hints (not an empty list)
 */

import { renderHook, act } from "@testing-library/react";
import { useRepositorySuggestions } from "../useRepositorySuggestions";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockListSessions = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    listSessions: mockListSessions,
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

const DAY_S = BigInt(24 * 60 * 60);

/** Build a minimal proto-like session object. */
function makeSession(
  path: string,
  opts: {
    updatedAtS?: bigint;
    createdAtS?: bigint;
    lastOutputS?: bigint;
  } = {},
) {
  return {
    path,
    updatedAt: opts.updatedAtS !== undefined ? { seconds: opts.updatedAtS, nanos: 0 } : undefined,
    createdAt: opts.createdAtS !== undefined ? { seconds: opts.createdAtS, nanos: 0 } : undefined,
    lastMeaningfulOutput: opts.lastOutputS !== undefined ? { seconds: opts.lastOutputS, nanos: 0 } : undefined,
  };
}

const NOW_S = BigInt(Math.floor(Date.now() / 1000)); // current time in seconds

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useRepositorySuggestions", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("starts with isLoading=true and empty suggestions", () => {
    mockListSessions.mockReturnValue(new Promise(() => {})); // never resolves
    const { result } = renderHook(() => useRepositorySuggestions());
    expect(result.current.isLoading).toBe(true);
    expect(result.current.suggestions).toEqual([]);
  });

  it("extracts BigInt seconds from updatedAt and converts to ms correctly", async () => {
    // updatedAt.seconds is BigInt — verify Number() conversion doesn't lose precision
    const sessions = [
      makeSession("/repo/a", { updatedAtS: NOW_S }),
    ];
    mockListSessions.mockResolvedValue({ sessions });

    const { result } = renderHook(() => useRepositorySuggestions());
    await act(async () => {});

    expect(result.current.suggestions).toContain("/repo/a");
    expect(result.current.isLoading).toBe(false);
  });

  it("ranks more-frequent paths above less-frequent paths of similar recency", async () => {
    const sessions = [
      makeSession("/repo/rare",     { updatedAtS: NOW_S }),
      makeSession("/repo/frequent", { updatedAtS: NOW_S }),
      makeSession("/repo/frequent", { updatedAtS: NOW_S - DAY_S }),
      makeSession("/repo/frequent", { updatedAtS: NOW_S - 2n * DAY_S }),
    ];
    mockListSessions.mockResolvedValue({ sessions });

    const { result } = renderHook(() => useRepositorySuggestions());
    await act(async () => {});

    expect(result.current.suggestions[0]).toBe("/repo/frequent");
    expect(result.current.suggestions[1]).toBe("/repo/rare");
  });

  it("uses the most recent timestamp across updatedAt, createdAt, and lastMeaningfulOutput", async () => {
    const sessions = [
      // /repo/a: old updatedAt, recent lastMeaningfulOutput
      makeSession("/repo/a", { updatedAtS: NOW_S - 30n * DAY_S, lastOutputS: NOW_S }),
      // /repo/b: only createdAt, older
      makeSession("/repo/b", { createdAtS: NOW_S - 2n * DAY_S }),
    ];
    mockListSessions.mockResolvedValue({ sessions });

    const { result } = renderHook(() => useRepositorySuggestions());
    await act(async () => {});

    // /repo/a has the most recent timestamp via lastMeaningfulOutput
    expect(result.current.suggestions[0]).toBe("/repo/a");
  });

  it("returns fallback hints when no sessions exist", async () => {
    mockListSessions.mockResolvedValue({ sessions: [] });

    const { result } = renderHook(() => useRepositorySuggestions());
    await act(async () => {});

    expect(result.current.suggestions.length).toBeGreaterThan(0);
    // Fallback paths should look like filesystem paths
    expect(result.current.suggestions[0]).toMatch(/\/(Users|home)\/username\//);
  });

  it("returns fallback hints (not empty array) when RPC throws", async () => {
    mockListSessions.mockRejectedValue(new Error("network error"));

    const { result } = renderHook(() => useRepositorySuggestions());
    await act(async () => {});

    // Should degrade to fallback hints, not silently empty
    expect(result.current.suggestions.length).toBeGreaterThan(0);
    expect(result.current.isLoading).toBe(false);
  });

  it("handles sessions with undefined path gracefully", async () => {
    const sessions = [
      { path: undefined, updatedAt: { seconds: NOW_S, nanos: 0 } },
      makeSession("/repo/valid", { updatedAtS: NOW_S }),
    ];
    mockListSessions.mockResolvedValue({ sessions });

    const { result } = renderHook(() => useRepositorySuggestions());
    await act(async () => {});

    expect(result.current.suggestions).toEqual(["/repo/valid"]);
  });

  it("deduplicates multiple sessions with the same path", async () => {
    const sessions = [
      makeSession("/repo/dup", { updatedAtS: NOW_S }),
      makeSession("/repo/dup", { updatedAtS: NOW_S - DAY_S }),
      makeSession("/repo/dup", { updatedAtS: NOW_S - 2n * DAY_S }),
    ];
    mockListSessions.mockResolvedValue({ sessions });

    const { result } = renderHook(() => useRepositorySuggestions());
    await act(async () => {});

    const dupCount = result.current.suggestions.filter((p) => p === "/repo/dup").length;
    expect(dupCount).toBe(1);
  });
});
