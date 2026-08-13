/**
 * Tests for useDestinationPathPreview hook.
 *
 * Mirrors useWorktreeSuggestions.test.ts's debounce/abort/timeout patterns, plus
 * coverage for this hook's mode-specific short-circuits and the unresolved_reason
 * "no preview, not an error" contract.
 */

import { renderHook, act } from "@testing-library/react";
import { useDestinationPathPreview } from "../useDestinationPathPreview";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockPreviewDestinationPath = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    previewDestinationPath: mockPreviewDestinationPath,
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));

jest.mock("@/gen/session/v1/session_pb", () => ({
  SessionService: {},
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useDestinationPathPreview", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockPreviewDestinationPath.mockResolvedValue({
      path: "",
      isExact: false,
      unresolvedReason: "",
    });
  });

  afterEach(() => {
    act(() => { jest.runOnlyPendingTimers(); });
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("does not fetch when params is null", () => {
    const { result } = renderHook(() => useDestinationPathPreview(null));
    act(() => { jest.advanceTimersByTime(500); });

    expect(result.current.path).toBeNull();
    expect(result.current.isLoading).toBe(false);
    expect(mockPreviewDestinationPath).not.toHaveBeenCalled();
  });

  it("does not fetch when enabled=false", () => {
    renderHook(() =>
      useDestinationPathPreview(
        { mode: "github_url", input: "owner/repo" },
        { enabled: false }
      )
    );
    act(() => { jest.advanceTimersByTime(500); });

    expect(mockPreviewDestinationPath).not.toHaveBeenCalled();
  });

  it("does not fetch github_url mode with empty input", () => {
    renderHook(() => useDestinationPathPreview({ mode: "github_url", input: "" }));
    act(() => { jest.advanceTimersByTime(500); });

    expect(mockPreviewDestinationPath).not.toHaveBeenCalled();
  });

  it("does not fetch new_worktree mode missing repoPath/sessionName", () => {
    renderHook(() =>
      useDestinationPathPreview({ mode: "new_worktree", input: "", sessionName: "foo" })
    );
    act(() => { jest.advanceTimersByTime(500); });

    expect(mockPreviewDestinationPath).not.toHaveBeenCalled();
  });

  it("fetches after the debounce and passes an AbortSignal", async () => {
    renderHook(() => useDestinationPathPreview({ mode: "github_url", input: "owner/repo" }));

    act(() => { jest.advanceTimersByTime(299); });
    expect(mockPreviewDestinationPath).not.toHaveBeenCalled();

    await act(async () => { jest.advanceTimersByTime(1); });
    expect(mockPreviewDestinationPath).toHaveBeenCalledWith(
      { input: "owner/repo", mode: "github_url", repoPath: "", sessionName: "" },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
  });

  it("surfaces an exact path from a successful github_url response", async () => {
    mockPreviewDestinationPath.mockResolvedValue({
      path: "/home/user/.stapler-squad/repos/github.com/owner/repo",
      isExact: true,
      unresolvedReason: "",
    });

    const { result } = renderHook(() =>
      useDestinationPathPreview({ mode: "github_url", input: "owner/repo" })
    );
    await act(async () => { jest.advanceTimersByTime(300); });

    expect(result.current.path).toBe("/home/user/.stapler-squad/repos/github.com/owner/repo");
    expect(result.current.isExact).toBe(true);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("surfaces an approximate prefix from a successful new_worktree response", async () => {
    mockPreviewDestinationPath.mockResolvedValue({
      path: "/home/user/.stapler-squad/worktrees/my-feature",
      isExact: false,
      unresolvedReason: "",
    });

    const { result } = renderHook(() =>
      useDestinationPathPreview({
        mode: "new_worktree",
        input: "",
        repoPath: "/repo",
        sessionName: "My Feature",
      })
    );
    await act(async () => { jest.advanceTimersByTime(300); });

    expect(result.current.path).toBe("/home/user/.stapler-squad/worktrees/my-feature");
    expect(result.current.isExact).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("treats unresolvedReason as no preview, not an error", async () => {
    mockPreviewDestinationPath.mockResolvedValue({
      path: "",
      isExact: false,
      unresolvedReason: "input does not look like a github url",
    });

    const { result } = renderHook(() =>
      useDestinationPathPreview({ mode: "github_url", input: "not a url" })
    );
    await act(async () => { jest.advanceTimersByTime(300); });

    expect(result.current.path).toBeNull();
    expect(result.current.isExact).toBe(false);
    expect(result.current.error).toBeNull();
    expect(result.current.isLoading).toBe(false);
  });

  it("sets error and clears isLoading on RPC rejection", async () => {
    mockPreviewDestinationPath.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() =>
      useDestinationPathPreview({ mode: "github_url", input: "owner/repo" })
    );
    await act(async () => { jest.advanceTimersByTime(300); });

    expect(result.current.error).toBe("boom");
    expect(result.current.isLoading).toBe(false);
    expect(result.current.path).toBeNull();
  });

  // A request that never resolves must not leave isLoading stuck forever — the hook
  // has to abort and surface a timeout error within a bounded time.
  it("times out and clears isLoading when the backend request hangs forever", async () => {
    let rejectFn: (err: unknown) => void = () => {};
    mockPreviewDestinationPath.mockImplementation(
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

    const { result } = renderHook(() =>
      useDestinationPathPreview({ mode: "github_url", input: "owner/repo" })
    );

    await act(async () => { jest.advanceTimersByTime(300); });
    expect(result.current.isLoading).toBe(true);

    await act(async () => { jest.advanceTimersByTime(3_000); });

    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBe("Destination path preview timed out");
    expect(result.current.path).toBeNull();

    // Avoid an unhandled rejection warning if the mock promise is ever settled late.
    rejectFn(new Error("cleanup"));
  });

  it("aborts the in-flight request when input changes before it resolves", async () => {
    const { rerender } = renderHook(
      ({ input }: { input: string }) =>
        useDestinationPathPreview({ mode: "github_url", input }),
      { initialProps: { input: "owner/repo-a" } }
    );
    await act(async () => { jest.advanceTimersByTime(300); });
    expect(mockPreviewDestinationPath).toHaveBeenCalledTimes(1);
    const firstCallSignal = mockPreviewDestinationPath.mock.calls[0][1].signal as AbortSignal;

    rerender({ input: "owner/repo-b" });
    await act(async () => { jest.advanceTimersByTime(300); });

    expect(firstCallSignal.aborted).toBe(true);
    expect(mockPreviewDestinationPath).toHaveBeenLastCalledWith(
      { input: "owner/repo-b", mode: "github_url", repoPath: "", sessionName: "" },
      expect.any(Object)
    );
  });

  it("drops a stale response that resolves after a newer request has started", async () => {
    let resolveFirst: (value: unknown) => void = () => {};
    mockPreviewDestinationPath.mockImplementationOnce(
      () => new Promise((resolve) => { resolveFirst = resolve; })
    );

    const { result, rerender } = renderHook(
      ({ input }: { input: string }) =>
        useDestinationPathPreview({ mode: "github_url", input }),
      { initialProps: { input: "owner/repo-a" } }
    );
    await act(async () => { jest.advanceTimersByTime(300); });

    mockPreviewDestinationPath.mockResolvedValueOnce({
      path: "/repos/owner/repo-b",
      isExact: true,
      unresolvedReason: "",
    });
    rerender({ input: "owner/repo-b" });
    await act(async () => { jest.advanceTimersByTime(300); });

    expect(result.current.path).toBe("/repos/owner/repo-b");

    // The stale first response resolves late — must not clobber the newer result.
    await act(async () => {
      resolveFirst({ path: "/repos/owner/repo-a", isExact: true, unresolvedReason: "" });
    });

    expect(result.current.path).toBe("/repos/owner/repo-b");
  });
});
