/** Task 4.1.3c: useSessionVcs polls by pollMode (off hidden, slow unfocused, fast focused). */
import { renderHook, act } from "@testing-library/react";

const mockGetVCSStatus = jest.fn();
const mockGetSessionDiff = jest.fn().mockResolvedValue({ diffStats: null });

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

const advance = (ms: number) => act(async () => { await jest.advanceTimersByTimeAsync(ms); });

describe("useSessionVcs pollMode", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    resetVcsCacheForTests();
    mockGetVCSStatus.mockReset();
    mockGetVCSStatus.mockResolvedValue({ vcsStatus: { branch: "main" }, error: "" });
  });
  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  it("useSessionVcs_should_IssueZeroGetVCSStatus_When_HiddenWindowFor120s", async () => {
    renderHook(() => useSessionVcs("h1", "", "off"));
    await advance(120_000);
    expect(mockGetVCSStatus).not.toHaveBeenCalled();
  });

  it("useSessionVcs_should_PollAtSlowCadence_When_VisibleButUnfocused", async () => {
    jest.spyOn(Math, "random").mockReturnValue(0.99); // pin the poll phase near the end of the cadence
    renderHook(() => useSessionVcs("u1", "", "slow"));
    await advance(120_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1); // mount only; the slow cadence is 600 s
    await advance(480_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(2);
  });

  it("useSessionVcs_should_SpreadSlowPolls_When_PanesMountTogether", async () => {
    const random = jest.spyOn(Math, "random");
    random.mockReturnValueOnce(0.1).mockReturnValueOnce(0.6);
    renderHook(() => useSessionVcs("j1", "", "slow"));
    renderHook(() => useSessionVcs("j2", "", "slow"));
    await advance(1_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(2); // the two mount fetches
    await advance(60_000); // j1's phase is 60 s, j2's is 360 s
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(3);
    await advance(300_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(4);
  });

  it("useSessionVcs_should_NotAddExtraPolls_When_SessionUpdatesKeepArriving", async () => {
    jest.spyOn(Math, "random").mockReturnValue(0.01); // a 6 s phase
    const { rerender } = renderHook(({ n }: { n: number }) => { void n; return useSessionVcs("k1", "", "slow"); }, { initialProps: { n: 0 } });
    await advance(1_000);
    const afterMount = mockGetVCSStatus.mock.calls.length;
    expect(afterMount).toBe(1);
    await advance(10_000); // the one jittered first tick
    const afterPhase = mockGetVCSStatus.mock.calls.length;
    expect(afterPhase).toBe(2);
    for (let i = 1; i <= 10; i++) {
      rerender({ n: i });
      await advance(30_000);
    }
    expect(mockGetVCSStatus.mock.calls.length).toBe(afterPhase); // full periods restart on re-run, as before
  });

  it("useSessionVcs_should_PollFast_When_Focused", async () => {
    renderHook(() => useSessionVcs("f1", "", "fast"));
    await advance(120_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(3); // mount + 60 s + 120 s
  });

  it("useSessionVcs_should_RefetchOnce_When_WindowBecomesActive", async () => {
    const { rerender } = renderHook(({ mode }: { mode: PollMode }) => useSessionVcs("a1", "", mode), {
      initialProps: { mode: "off" as PollMode },
    });
    await advance(1_000);
    expect(mockGetVCSStatus).not.toHaveBeenCalled();
    rerender({ mode: "fast" });
    await advance(1_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1);
  });
});
