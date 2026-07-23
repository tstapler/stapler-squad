/**
 * Tests for useWorktreeSuggestions hook.
 *
 * Mirrors usePathCompletions.test.ts's debounce/timeout patterns. Covers AC1:
 * a hung ListWorktrees backend call must surface a bounded timeout/error state
 * instead of leaving the "existing worktree" dropdown loading forever.
 */

import { renderHook, act } from "@testing-library/react";
import { useWorktreeSuggestions } from "../useWorktreeSuggestions";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

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
}));

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useWorktreeSuggestions", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockListWorktrees.mockResolvedValue({ worktrees: [] });
  });

  afterEach(() => {
    act(() => { jest.runOnlyPendingTimers(); });
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("does not fetch for an empty repoPath", () => {
    const { result } = renderHook(() => useWorktreeSuggestions(""));
    expect(result.current.worktrees).toEqual([]);
    expect(result.current.isLoading).toBe(false);
    expect(mockListWorktrees).not.toHaveBeenCalled();
  });

  it("does not fetch when enabled=false", () => {
    renderHook(() => useWorktreeSuggestions("/repo", { enabled: false }));
    act(() => { jest.advanceTimersByTime(500); });
    expect(mockListWorktrees).not.toHaveBeenCalled();
  });

  it("fetches after the debounce and passes an AbortSignal", async () => {
    renderHook(() => useWorktreeSuggestions("/repo"));
    act(() => { jest.advanceTimersByTime(149); });
    expect(mockListWorktrees).not.toHaveBeenCalled();

    await act(async () => { jest.advanceTimersByTime(1); });
    expect(mockListWorktrees).toHaveBeenCalledWith(
      { repoPath: "/repo" },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
  });

  it("surfaces worktrees from a successful response", async () => {
    const worktrees = [{ path: "/repo", branch: "main", isMain: true }];
    mockListWorktrees.mockResolvedValue({ worktrees });

    const { result } = renderHook(() => useWorktreeSuggestions("/repo"));
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(result.current.worktrees).toEqual(worktrees);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("sets error and clears isLoading on RPC rejection", async () => {
    mockListWorktrees.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() => useWorktreeSuggestions("/repo"));
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(result.current.error).toBe("boom");
    expect(result.current.isLoading).toBe(false);
    expect(result.current.worktrees).toEqual([]);
  });

  // AC1: a request that never resolves must not leave isLoading stuck forever —
  // the hook has to abort and surface a timeout error within a bounded time.
  it("times out and clears isLoading when the backend request hangs forever", async () => {
    let rejectFn: (err: unknown) => void = () => {};
    mockListWorktrees.mockImplementation(
      (_req: unknown, opts: { signal: AbortSignal }) =>
        new Promise((_resolve, reject) => {
          rejectFn = reject;
          opts.signal.addEventListener("abort", () => {
            const err = new Error("aborted");
            (err as Error & { name: string }).name = "AbortError";
            reject(err);
          });
        })
    );

    const { result } = renderHook(() => useWorktreeSuggestions("/repo"));

    // Debounce fires; request is now in flight and loading.
    await act(async () => { jest.advanceTimersByTime(150); });
    expect(result.current.isLoading).toBe(true);

    // Still hanging well past a reasonable bound — must not be stuck.
    await act(async () => { jest.advanceTimersByTime(10_000); });

    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBeTruthy();
    expect(result.current.worktrees).toEqual([]);

    // Avoid an unhandled rejection warning if the mock promise is ever settled late.
    rejectFn(new Error("cleanup"));
  });

  it("aborts the in-flight request when repoPath changes before it resolves", async () => {
    const { rerender } = renderHook(
      ({ repoPath }: { repoPath: string }) => useWorktreeSuggestions(repoPath),
      { initialProps: { repoPath: "/repo-a" } }
    );
    await act(async () => { jest.advanceTimersByTime(150); });
    expect(mockListWorktrees).toHaveBeenCalledTimes(1);
    const firstCallSignal = mockListWorktrees.mock.calls[0][1].signal as AbortSignal;

    rerender({ repoPath: "/repo-b" });
    await act(async () => { jest.advanceTimersByTime(150); });

    expect(firstCallSignal.aborted).toBe(true);
    expect(mockListWorktrees).toHaveBeenLastCalledWith(
      { repoPath: "/repo-b" },
      expect.any(Object)
    );
  });
});
