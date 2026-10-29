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
  afterEach(() => jest.useRealTimers());

  it("useSessionVcs_should_IssueZeroGetVCSStatus_When_HiddenWindowFor120s", async () => {
    renderHook(() => useSessionVcs("h1", "", "off"));
    await advance(120_000);
    expect(mockGetVCSStatus).not.toHaveBeenCalled();
  });

  it("useSessionVcs_should_PollAtSlowCadence_When_VisibleButUnfocused", async () => {
    renderHook(() => useSessionVcs("u1", "", "slow"));
    await advance(120_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1); // mount only; the slow cadence is 600 s
    await advance(480_000);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(2);
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
