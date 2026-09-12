/**
 * Tests for useRepoPathSuggestions.
 *
 * Covers AC1 (resolves via SessionService.ListWorktrees, no other RPC), AC2
 * (a root discovered via the resolved family is returned even when it wasn't
 * itself a candidate path), and the RPC-failure/timeout degrade path (AC3)
 * that RepoPathInput relies on to fall back to plain history.
 */

import { renderHook, act } from "@testing-library/react";
import { useRepoPathSuggestions } from "../useRepoPathSuggestions";

const mockListWorktrees = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    listWorktrees: mockListWorktrees,
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
  createAuthInterceptor: jest.fn(() => jest.fn()),
}));

describe("useRepoPathSuggestions", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockListWorktrees.mockResolvedValue({ worktrees: [] });
  });

  afterEach(() => {
    act(() => { jest.runOnlyPendingTimers(); });
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("does not fetch for an empty candidate list", () => {
    const { result } = renderHook(() => useRepoPathSuggestions([]));
    act(() => { jest.advanceTimersByTime(500); });
    expect(result.current.resolutions.size).toBe(0);
    expect(mockListWorktrees).not.toHaveBeenCalled();
  });

  it("calls SessionService.listWorktrees for each candidate path after the debounce", async () => {
    renderHook(() => useRepoPathSuggestions(["/repo/root", "/repo/worktrees/wt-a"]));

    act(() => { jest.advanceTimersByTime(149); });
    expect(mockListWorktrees).not.toHaveBeenCalled();

    await act(async () => { jest.advanceTimersByTime(1); });
    expect(mockListWorktrees).toHaveBeenCalledWith(
      { repoPath: "/repo/root" },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(mockListWorktrees).toHaveBeenCalledWith(
      { repoPath: "/repo/worktrees/wt-a" },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(mockListWorktrees).toHaveBeenCalledTimes(2);
  });

  // AC1: resolves via ListWorktrees only — no other RPC in the mocked client.
  it("associates a worktree path with its resolved root", async () => {
    mockListWorktrees.mockResolvedValue({
      worktrees: [
        { path: "/repo/root", branch: "main", isMain: true },
        { path: "/repo/worktrees/wt-a", branch: "feature-a", isMain: false },
      ],
    });

    const { result } = renderHook(() => useRepoPathSuggestions(["/repo/worktrees/wt-a"]));
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(result.current.resolutions.get("/repo/worktrees/wt-a")).toEqual({
      rootPath: "/repo/root",
      isMain: false,
      branch: "feature-a",
    });
    expect(result.current.resolutions.get("/repo/root")).toEqual({
      rootPath: "/repo/root",
      isMain: true,
      branch: "main",
    });
  });

  // AC2: the root is present in the returned map even though only a worktree
  // path (never the root itself) was passed in as a candidate.
  it("surfaces the root discovered via a worktree candidate, even though the root itself was never a candidate", async () => {
    mockListWorktrees.mockResolvedValue({
      worktrees: [
        { path: "/repo/root", branch: "main", isMain: true },
        { path: "/repo/worktrees/wt-a", branch: "feature-a", isMain: false },
      ],
    });

    const { result } = renderHook(() => useRepoPathSuggestions(["/repo/worktrees/wt-a"]));
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(result.current.resolutions.has("/repo/root")).toBe(true);
    expect(result.current.resolutions.get("/repo/root")?.isMain).toBe(true);
  });

  // AC4: a trailing slash must not prevent matching the canonical (slash-free)
  // path ListWorktrees returns.
  it("normalizes a trailing slash so the candidate still matches its WorktreeEntry", async () => {
    mockListWorktrees.mockResolvedValue({
      worktrees: [{ path: "/repo/root", branch: "main", isMain: true }],
    });

    const { result } = renderHook(() => useRepoPathSuggestions(["/repo/root/"]));
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(result.current.resolutions.get("/repo/root")).toEqual({
      rootPath: "/repo/root",
      isMain: true,
      branch: "main",
    });
  });

  // AC3: an RPC failure must not surface as a thrown error or crash — the
  // path simply stays absent from the map so callers fall back to plain
  // (ungrouped) history behavior.
  it("leaves a path out of the map when its ListWorktrees call rejects", async () => {
    mockListWorktrees.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() => useRepoPathSuggestions(["/repo/worktrees/wt-a"]));
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(result.current.resolutions.size).toBe(0);
  });

  // AC3: a hung request must not stall subsequent resolutions or leave the
  // hook implicitly loading forever.
  it("does not throw when a request hangs and is aborted on unmount", async () => {
    mockListWorktrees.mockImplementation(
      (_req: unknown, opts: { signal: AbortSignal }) =>
        new Promise((_resolve, reject) => {
          opts.signal.addEventListener("abort", () => {
            const err = new Error("aborted");
            (err as Error & { name: string }).name = "AbortError";
            reject(err);
          });
        })
    );

    const { result, unmount } = renderHook(() => useRepoPathSuggestions(["/repo/worktrees/wt-a"]));
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(() => unmount()).not.toThrow();
    expect(result.current.resolutions.size).toBe(0);
  });

  it("does not re-fetch a path already resolved (cached for the component's lifetime)", async () => {
    mockListWorktrees.mockResolvedValue({
      worktrees: [{ path: "/repo/root", branch: "main", isMain: true }],
    });

    const { rerender } = renderHook(
      ({ paths }: { paths: string[] }) => useRepoPathSuggestions(paths),
      { initialProps: { paths: ["/repo/root"] } }
    );
    await act(async () => { jest.advanceTimersByTime(150); });
    expect(mockListWorktrees).toHaveBeenCalledTimes(1);

    rerender({ paths: ["/repo/root"] });
    await act(async () => { jest.advanceTimersByTime(150); });
    expect(mockListWorktrees).toHaveBeenCalledTimes(1);
  });
});
