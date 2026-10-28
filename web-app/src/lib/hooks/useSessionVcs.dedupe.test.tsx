/**
 * Story 2.1.6a: a SessionDetail mount (prefetchVcsStatus + useSessionVcs) issues
 * exactly one getVCSStatus per session.
 */
import { renderHook, waitFor } from "@testing-library/react";

const mockGetVCSStatus = jest.fn();
const mockGetSessionDiff = jest.fn().mockResolvedValue({ diffStats: null });

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getVCSStatus: mockGetVCSStatus, getSessionDiff: mockGetSessionDiff }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@/lib/store", () => ({ useAppSelector: () => undefined }));
jest.mock("@/lib/store/sessionsSlice", () => ({ selectAllSessions: () => [] }));

import { prefetchVcsStatus, resetVcsCacheForTests } from "@/lib/hooks/useVcsStatus";
import { useSessionVcs } from "@/lib/hooks/useSessionVcs";
import { priorityKey } from "@/lib/api/requestPriority";

describe("useSessionVcs prefetch dedupe", () => {
  beforeEach(() => {
    resetVcsCacheForTests();
    mockGetVCSStatus.mockReset();
  });

  it("useSessionVcs_should_ShareInFlightPrefetch_When_MountedDuringPrefetch", async () => {
    let resolve!: (v: unknown) => void;
    mockGetVCSStatus.mockReturnValue(new Promise((r) => { resolve = r; }));
    void prefetchVcsStatus("s1", "");
    const { result } = renderHook(() => useSessionVcs("s1", ""));
    resolve({ vcsStatus: { branch: "main" }, error: "" });
    await waitFor(() => expect(result.current.statusLoading).toBe(false));
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1);
    expect(result.current.status).toEqual({ branch: "main" });
  });

  it("useSessionVcs_should_ReadPrefetchCache_When_PrefetchAlreadyResolved", async () => {
    mockGetVCSStatus.mockResolvedValue({ vcsStatus: { branch: "dev" }, error: "" });
    await prefetchVcsStatus("s2", "");
    const { result } = renderHook(() => useSessionVcs("s2", ""));
    await waitFor(() => expect(result.current.statusLoading).toBe(false));
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1);
    expect(result.current.status).toEqual({ branch: "dev" });
  });

  it("useSessionVcs_should_TagGetVCSStatusDeferred_When_PaneNotActive", async () => {
    mockGetVCSStatus.mockResolvedValue({ vcsStatus: { branch: "x" }, error: "" });
    const { result } = renderHook(() => useSessionVcs("s3", "", "slow"));
    await waitFor(() => expect(result.current.statusLoading).toBe(false));
    expect(mockGetVCSStatus.mock.calls[0][1].contextValues.get(priorityKey)).toBe("deferred");
  });

  it("useSessionVcs_should_LeaveGetVCSStatusCritical_When_PaneActive", async () => {
    mockGetVCSStatus.mockResolvedValue({ vcsStatus: { branch: "x" }, error: "" });
    const { result } = renderHook(() => useSessionVcs("s4", "", "fast"));
    await waitFor(() => expect(result.current.statusLoading).toBe(false));
    expect(mockGetVCSStatus.mock.calls[0][1]).toBeUndefined();
  });
});
