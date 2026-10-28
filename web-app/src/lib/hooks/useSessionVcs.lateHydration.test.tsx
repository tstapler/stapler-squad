/**
 * HAR perf-har-2: 7 of 8 sessions issued GetVCSStatus twice, overlapping in time. The
 * second call came from the session arriving in Redux (updatedAt null -> value) while the
 * mount fetch was still in flight; that re-run forced a fetch that, by design, never
 * joins an in-flight one.
 */
import { renderHook, waitFor, act } from "@testing-library/react";

const mockGetVCSStatus = jest.fn();
const mockState: { sessions: { id: string; status: number; updatedAt?: { seconds: number } }[] } = { sessions: [] };

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getVCSStatus: mockGetVCSStatus, getSessionDiff: jest.fn().mockResolvedValue({ diffStats: null }) }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@/lib/store", () => ({ useAppSelector: (sel: (s: unknown) => unknown) => sel(mockState) }));
jest.mock("@/lib/store/sessionsSlice", () => ({
  selectAllSessions: (s: typeof mockState) => s.sessions,
}));

import { resetVcsCacheForTests } from "@/lib/hooks/useVcsStatus";
import { useSessionVcs } from "@/lib/hooks/useSessionVcs";

describe("useSessionVcs late session hydration", () => {
  beforeEach(() => {
    resetVcsCacheForTests();
    mockGetVCSStatus.mockReset();
    mockState.sessions = [];
  });

  it("useSessionVcs_should_IssueOneGetVCSStatus_When_SessionHydratesInReduxDuringMountFetch", async () => {
    let resolve!: (v: unknown) => void;
    mockGetVCSStatus.mockReturnValue(new Promise((r) => { resolve = r; }));
    const { result, rerender } = renderHook(() => useSessionVcs("s1", ""));
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1);

    mockState.sessions = [{ id: "s1", status: 1, updatedAt: { seconds: 100 } }];
    rerender();
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1);

    resolve({ vcsStatus: { branch: "main" }, error: "" });
    await waitFor(() => expect(result.current.statusLoading).toBe(false));
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1);
  });

  it("useSessionVcs_should_ForceFreshFetch_When_KnownSessionIsUpdated", async () => {
    mockState.sessions = [{ id: "s2", status: 1, updatedAt: { seconds: 100 } }];
    mockGetVCSStatus.mockResolvedValue({ vcsStatus: { branch: "a" }, error: "" });
    const { result, rerender } = renderHook(() => useSessionVcs("s2", ""));
    await waitFor(() => expect(result.current.statusLoading).toBe(false));
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1);

    mockState.sessions = [{ id: "s2", status: 1, updatedAt: { seconds: 101 } }];
    mockGetVCSStatus.mockResolvedValue({ vcsStatus: { branch: "b" }, error: "" });
    await act(async () => rerender());
    await waitFor(() => expect(result.current.status).toEqual({ branch: "b" }));
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(2);
  });
});
