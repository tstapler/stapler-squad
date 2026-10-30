import {
  loadFlags,
  peekFlags,
  preloadFlagValue,
  recordFlagWrite,
  resetFlagStoreForTest,
  WINDOW_KEEP_ALIVE_DEFAULT,
} from "./flagStore";

const mockGetFeatureFlags = jest.fn();

jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ getFeatureFlags: (...a: unknown[]) => mockGetFeatureFlags(...a) }),
}));

function flag(name: string, enabled: boolean) {
  return { name, enabled, description: "", statusDetail: "", scopes: [] };
}

beforeEach(() => {
  resetFlagStoreForTest();
  mockGetFeatureFlags.mockReset();
  localStorage.clear();
});

describe("flagStore", () => {
  it("flagStore_should_IssueOneGetFeatureFlags_When_ProviderAnd5HookConsumersMountTogether", async () => {
    mockGetFeatureFlags.mockResolvedValue({ flags: [flag("x", true)] });
    const results = await Promise.all(Array.from({ length: 6 }, () => loadFlags()));
    expect(mockGetFeatureFlags).toHaveBeenCalledTimes(1);
    results.forEach((r) => expect(r).toBe(results[0]));
  });

  it("flagStore_should_KeepLastKnownValues_When_RefreshFails", async () => {
    let t = 1_000;
    const now = () => t;
    expect(peekFlags()).toBeNull();
    mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", true)] });
    await loadFlags(now);
    t += 30_001;
    mockGetFeatureFlags.mockRejectedValueOnce(new Error("down"));
    await expect(loadFlags(now)).rejects.toThrow("down");
    expect(peekFlags()?.[0]).toMatchObject({ name: "x", enabled: true });
    expect(mockGetFeatureFlags).toHaveBeenCalledTimes(2);
  });

  it("setFlag_should_ReturnNewValueImmediately_When_ReadBackAfterSet", async () => {
    mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", false)] });
    await loadFlags();
    recordFlagWrite("x", true);
    expect(peekFlags()?.[0].enabled).toBe(true);
    mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", true)] });
    const next = await loadFlags();
    expect(next[0].enabled).toBe(true);
    expect(mockGetFeatureFlags).toHaveBeenCalledTimes(2); // write invalidated the 30 s cache
  });

  it.each(["unary_over_ws", "unary_over_ws_remote"])(
    "flagStore_should_RemovePersistedWsKey_When_ResponseOmitsFlagOrReportsOff (%s)",
    async (name) => {
      const key = `ssq:flag:${name}`;
      let t = 0;
      const now = () => t;
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag(name, true)] });
      await loadFlags(now);
      expect(localStorage.getItem(key)).toBe("1");
      expect(preloadFlagValue(name)).toBe(true);

      t += 30_001;
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag(name, false)] });
      await loadFlags(now);
      expect(localStorage.getItem(key)).toBeNull();

      localStorage.setItem(key, "1");
      t += 30_001;
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [] });
      await loadFlags(now);
      expect(localStorage.getItem(key)).toBeNull();
      expect(preloadFlagValue(name)).toBe(false);
    },
  );

  it("flagStore_should_ExposePersistedKeepAliveBeforeFirstFetch_And_FallBackToShippedDefault_When_NoKey", async () => {
    expect(preloadFlagValue("window_keep_alive")).toBe(WINDOW_KEEP_ALIVE_DEFAULT);
    localStorage.setItem("ssq:flag:window_keep_alive", "1");
    expect(preloadFlagValue("window_keep_alive")).toBe(true);
    localStorage.setItem("ssq:flag:window_keep_alive", "0");
    expect(preloadFlagValue("window_keep_alive")).toBe(false);

    mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("window_keep_alive", true)] });
    await loadFlags();
    expect(localStorage.getItem("ssq:flag:window_keep_alive")).toBe("1");
  });

  it("flagStore_should_RevertGhosttyRendererToOff_When_FlagDisabledOrOmitted_WithoutRebuild", async () => {
    expect(preloadFlagValue("ghostty_renderer")).toBe(false);

    let t = 0;
    const now = () => t;
    mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("ghostty_renderer", true)] });
    await loadFlags(now);
    expect(localStorage.getItem("ssq:flag:ghostty_renderer")).toBe("1");
    expect(preloadFlagValue("ghostty_renderer")).toBe(true);

    t += 30_001;
    mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("ghostty_renderer", false)] });
    await loadFlags(now);
    expect(localStorage.getItem("ssq:flag:ghostty_renderer")).toBeNull();
    expect(preloadFlagValue("ghostty_renderer")).toBe(false);
  });

  it("flagStore_should_UseFixedOnConstantForRequestCoalescing_And_UndefinedForUnlistedFlags", () => {
    expect(preloadFlagValue("request_coalescing")).toBe(true);
    expect(localStorage.getItem("ssq:flag:request_coalescing")).toBeNull();
    expect(preloadFlagValue("notification_inline_actions")).toBeUndefined();
  });
});
