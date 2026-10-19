import { act, renderHook, waitFor } from "@testing-library/react";
import { resetFeatureFlagValueCache, resolveFlagValue, useFeatureFlagValue, flagOffMessage } from "./useFeatureFlagValue";

const mockGetFeatureFlags = jest.fn();

jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getFeatureFlags: (...a: unknown[]) => mockGetFeatureFlags(...a) }),
}));

const INLINE = "notification_inline_actions";

function flags(enabled: boolean, scopes: Array<{ scope: string; enabled: boolean }> = []) {
  return { flags: [{ name: INLINE, enabled, description: "", statusDetail: "", scopes }] };
}

beforeEach(() => {
  resetFeatureFlagValueCache();
  mockGetFeatureFlags.mockReset();
});

describe("useFeatureFlagValue", () => {
  it("useFeatureFlagValue_should_returnScopedValueAndCache_When_flagReadTwice", async () => {
    mockGetFeatureFlags.mockResolvedValue(flags(true, [{ scope: "action:DENY", enabled: false }]));
    const deny = renderHook(() => useFeatureFlagValue(INLINE, "action:DENY"));
    const approve = renderHook(() => useFeatureFlagValue(INLINE, "action:APPROVE"));
    await waitFor(() => expect(approve.result.current).toBe(true));
    expect(deny.result.current).toBe(false);
    const again = renderHook(() => useFeatureFlagValue(INLINE));
    await waitFor(() => expect(again.result.current).toBe(true));
    expect(mockGetFeatureFlags).toHaveBeenCalledTimes(1);
  });

  it("useFeatureFlagValue_should_fallBackToDefaultOff_When_getFeatureFlagsRejects", async () => {
    mockGetFeatureFlags.mockRejectedValue(new Error("down"));
    const { result } = renderHook(() => useFeatureFlagValue(INLINE));
    await waitFor(() => expect(mockGetFeatureFlags).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("useFeatureFlagValue_should_pickUpAKillSwitchFlipAndKeepItThroughAFailedReread_When_30sPass", async () => {
    jest.useFakeTimers();
    try {
      mockGetFeatureFlags.mockResolvedValue(flags(true));
      const { result } = renderHook(() => useFeatureFlagValue(INLINE));
      await act(async () => {});
      expect(result.current).toBe(true);

      mockGetFeatureFlags.mockResolvedValue(flags(false));
      await act(async () => void jest.advanceTimersByTime(30_001));
      expect(result.current).toBe(false);

      mockGetFeatureFlags.mockRejectedValue(new Error("down"));
      await act(async () => void jest.advanceTimersByTime(30_001));
      expect(result.current).toBe(false);
    } finally {
      jest.useRealTimers();
    }
  });

  it("useFeatureFlagValue_should_skipTheRead_When_notEnabled", async () => {
    mockGetFeatureFlags.mockResolvedValue(flags(true));
    const { result } = renderHook(() => useFeatureFlagValue(INLINE, undefined, false, false));
    await Promise.resolve();
    expect(mockGetFeatureFlags).not.toHaveBeenCalled();
    expect(result.current).toBe(false);
  });

  it("useFeatureFlagValue_should_returnFallback_When_flagUnknown", async () => {
    mockGetFeatureFlags.mockResolvedValue(flags(true));
    const { result } = renderHook(() => useFeatureFlagValue("nope", undefined, true));
    await waitFor(() => expect(mockGetFeatureFlags).toHaveBeenCalled());
    expect(result.current).toBe(true);
  });
});

describe("resolveFlagValue", () => {
  it("resolveFlagValue_should_preferScopeOverGlobal_When_overridePresent", () => {
    const list = flags(true, [{ scope: "action:DENY", enabled: false }]).flags as never;
    expect(resolveFlagValue(list, INLINE, "action:DENY")).toBe(false);
    expect(resolveFlagValue(list, INLINE, "action:APPROVE")).toBe(true);
    expect(resolveFlagValue(list, "nope")).toBeUndefined();
  });
  it("exposes the FLAG_OFF copy", () => {
    expect(flagOffMessage).toBe("This action is turned off. Open session.");
  });
});
