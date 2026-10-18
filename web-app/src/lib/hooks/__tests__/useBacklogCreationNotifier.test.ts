import { renderHook, act } from "@testing-library/react";
import { useBacklogCreationNotifier } from "../useBacklogCreationNotifier";

const mockPush = jest.fn();
jest.mock("next/navigation", () => ({ useRouter: () => ({ push: mockPush }) }));

const mockAddNotification = jest.fn();
jest.mock("@/lib/contexts/notificationContexts", () => ({
  useNotificationCommands: () => ({ addNotification: mockAddNotification }),
}));

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getBacklogItem: jest.fn().mockResolvedValue({ item: undefined }) }),
}));
jest.mock("@connectrpc/connect-web", () => ({ createConnectTransport: jest.fn() }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "", createAuthInterceptor: () => ({}) }));

const mockGetBacklogItem = jest.fn();
jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({ getBacklogItem: mockGetBacklogItem }),
}));

async function tick() {
  await act(async () => {
    jest.advanceTimersByTime(4000);
  });
}

beforeEach(() => {
  jest.useFakeTimers();
});
afterEach(() => {
  jest.useRealTimers();
  jest.clearAllMocks();
});

describe("useBacklogCreationNotifier", () => {
  it("notifies once when triage completes and navigates to the item on view", async () => {
    mockGetBacklogItem.mockResolvedValue({ id: "a", triageStatus: "completed" });
    const { result } = renderHook(() => useBacklogCreationNotifier());
    act(() => result.current.trackItem("a", "Dark mode"));

    await tick();
    await tick();

    expect(mockAddNotification).toHaveBeenCalledTimes(1);
    const n = mockAddNotification.mock.calls[0][0];
    expect(n.title).toBe("Triage complete — Dark mode");
    n.onView();
    expect(mockPush).toHaveBeenCalledWith("/backlog?item=a");
  });

  it("notifies when the AI needs input, and again for a later request", async () => {
    mockGetBacklogItem.mockResolvedValue({ id: "a", triageStatus: "running", hasPendingGuidanceRequests: true });
    const { result } = renderHook(() => useBacklogCreationNotifier());
    act(() => result.current.trackItem("a", "Dark mode"));

    await tick();
    await tick();
    expect(mockAddNotification).toHaveBeenCalledTimes(1);
    expect(mockAddNotification.mock.calls[0][0].title).toBe("AI needs your input — Dark mode");

    mockGetBacklogItem.mockResolvedValue({ id: "a", triageStatus: "running", hasPendingGuidanceRequests: false });
    await tick();
    mockGetBacklogItem.mockResolvedValue({ id: "a", triageStatus: "running", hasPendingGuidanceRequests: true });
    await tick();
    expect(mockAddNotification).toHaveBeenCalledTimes(2);
  });

  it("never polls or notifies for items that were not tracked", async () => {
    mockGetBacklogItem.mockResolvedValue({ id: "other", triageStatus: "completed" });
    renderHook(() => useBacklogCreationNotifier());

    await tick();

    expect(mockGetBacklogItem).not.toHaveBeenCalled();
    expect(mockAddNotification).not.toHaveBeenCalled();
  });

  it("notifies when triage fails", async () => {
    mockGetBacklogItem.mockResolvedValue({ id: "a", triageStatus: "failed" });
    const { result } = renderHook(() => useBacklogCreationNotifier());
    act(() => result.current.trackItem("a", "Dark mode"));

    await tick();

    expect(mockAddNotification).toHaveBeenCalledTimes(1);
    expect(mockAddNotification.mock.calls[0][0].title).toBe("Triage failed — Dark mode");
  });

  it("keeps polling after a failed fetch", async () => {
    mockGetBacklogItem.mockRejectedValueOnce(new Error("boom"));
    mockGetBacklogItem.mockResolvedValue({ id: "a", triageStatus: "completed" });
    const { result } = renderHook(() => useBacklogCreationNotifier());
    act(() => result.current.trackItem("a", "Dark mode"));

    await tick();
    expect(mockAddNotification).not.toHaveBeenCalled();
    await tick();
    expect(mockAddNotification).toHaveBeenCalledTimes(1);
  });

  it("does not start a second poll while one is in flight", async () => {
    mockGetBacklogItem.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useBacklogCreationNotifier());
    act(() => result.current.trackItem("a", "Dark mode"));

    await tick();
    await tick();

    expect(mockGetBacklogItem).toHaveBeenCalledTimes(1);
  });

  it("stops polling after unmount", async () => {
    mockGetBacklogItem.mockResolvedValue({ id: "a", triageStatus: "running" });
    const { result, unmount } = renderHook(() => useBacklogCreationNotifier());
    act(() => result.current.trackItem("a", "Dark mode"));
    unmount();

    await tick();

    expect(mockGetBacklogItem).not.toHaveBeenCalled();
  });
});
