/** GetSessionDiff is the heaviest reload-path read: it must fire only when a consumer is visible. */
import { renderHook, act } from "@testing-library/react";

const mockGetVCSStatus = jest.fn();
const mockGetSessionDiff = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getVCSStatus: mockGetVCSStatus, getSessionDiff: mockGetSessionDiff }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@/lib/store", () => ({ useAppSelector: () => undefined }));
jest.mock("@/lib/store/sessionsSlice", () => ({ selectAllSessions: () => [] }));

import { resetVcsCacheForTests } from "@/lib/hooks/useVcsStatus";
import { useSessionVcs } from "@/lib/hooks/useSessionVcs";
import type { PollMode } from "@/lib/hooks/pollMode";

const flush = () => act(async () => { await Promise.resolve(); });

describe("useSessionVcs lazy diff", () => {
  beforeEach(() => {
    resetVcsCacheForTests();
    mockGetVCSStatus.mockReset().mockResolvedValue({ vcsStatus: { branch: "main" }, error: "" });
    mockGetSessionDiff.mockReset().mockResolvedValue({ diffStats: { content: "d", added: 1, removed: 0 } });
  });

  it("useSessionVcs_should_NotCallGetSessionDiff_When_NoConsumerAsks", async () => {
    renderHook(() => useSessionVcs("s1", "", "fast"));
    await flush();
    expect(mockGetVCSStatus).toHaveBeenCalled();
    expect(mockGetSessionDiff).not.toHaveBeenCalled();
  });

  it("useSessionVcs_should_NotCallGetSessionDiff_When_PaneIsOff", async () => {
    const { result } = renderHook(() => useSessionVcs("s2", "", "off"));
    act(() => result.current.ensureDiff());
    await flush();
    expect(mockGetSessionDiff).not.toHaveBeenCalled();
  });

  it("useSessionVcs_should_CallGetSessionDiffOnce_When_EnsureDiffRepeated", async () => {
    const { result } = renderHook(() => useSessionVcs("s3", "", "fast"));
    act(() => { result.current.ensureDiff(); result.current.ensureDiff(); });
    await flush();
    act(() => result.current.ensureDiff());
    await flush();
    expect(mockGetSessionDiff).toHaveBeenCalledTimes(1);
    expect(result.current.diff?.content).toBe("d");
  });

  it("useSessionVcs_should_FetchOnceWhenActivated_After_Off", async () => {
    const { result, rerender } = renderHook(({ mode }: { mode: PollMode }) => useSessionVcs("s4", "", mode), {
      initialProps: { mode: "off" as PollMode },
    });
    act(() => result.current.ensureDiff());
    await flush();
    expect(mockGetSessionDiff).not.toHaveBeenCalled();
    rerender({ mode: "fast" });
    act(() => result.current.ensureDiff());
    await flush();
    expect(mockGetSessionDiff).toHaveBeenCalledTimes(1);
  });

  it("useSessionVcs_should_NotRefetchDiffOnRefresh_When_NeverRequested", async () => {
    const { result } = renderHook(() => useSessionVcs("s5", "", "fast"));
    act(() => result.current.refresh());
    await flush();
    expect(mockGetSessionDiff).not.toHaveBeenCalled();
  });

  it("useSessionVcs_should_AbortInFlightDiff_When_Unmounted", async () => {
    let signal: AbortSignal | undefined;
    mockGetSessionDiff.mockImplementation((_req: unknown, opts: { signal: AbortSignal }) => {
      signal = opts.signal;
      return new Promise(() => {});
    });
    const { result, unmount } = renderHook(() => useSessionVcs("s6", "", "fast"));
    act(() => result.current.ensureDiff());
    unmount();
    expect(signal?.aborted).toBe(true);
  });

  it("useSessionVcs_should_RefetchDiff_When_ContextLinesChangeAfterRequest", async () => {
    const { result } = renderHook(() => useSessionVcs("s7", "", "fast"));
    act(() => result.current.ensureDiff());
    await flush();
    act(() => result.current.setDiffContextLines(-1));
    act(() => result.current.ensureDiff());
    await flush();
    expect(mockGetSessionDiff).toHaveBeenCalledTimes(2);
    expect(mockGetSessionDiff).toHaveBeenLastCalledWith({ id: "s7", contextLines: -1 }, expect.anything());
  });
});
