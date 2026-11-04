import {
  loadFlags,
  peekFlags,
  preloadFlagValue,
  recordFlagWrite,
  resetFlagStoreForTest,
  subscribeFlags,
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
  describe("conditional reads", () => {
    it("flagStore_should_SendLastEtagAsIfNoneMatch_When_Refreshing", async () => {
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", true)], etag: '"e1"' });
      await loadFlags();
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [], etag: '"e1"', notModified: true });
      await loadFlags(Date.now, { force: true });
      expect(mockGetFeatureFlags.mock.calls[0][0]).toEqual({ ifNoneMatch: "" });
      expect(mockGetFeatureFlags.mock.calls[1][0]).toEqual({ ifNoneMatch: '"e1"' });
    });

    it("flagStore_should_KeepPreviousBodyAndNotNotify_When_NotModified", async () => {
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", true)], etag: '"e1"' });
      const first = await loadFlags();
      const listener = jest.fn();
      const unsubscribe = subscribeFlags(listener);
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [], etag: '"e1"', notModified: true });
      const second = await loadFlags(Date.now, { force: true });
      unsubscribe();
      expect(second).toBe(first);
      expect(peekFlags()).toBe(first);
      expect(listener).not.toHaveBeenCalled();
    });

    it("flagStore_should_SendNoValidator_When_ServerNeverReturnedAnEtag", async () => {
      mockGetFeatureFlags.mockResolvedValue({ flags: [flag("x", true)] });
      await loadFlags();
      await loadFlags(Date.now, { force: true });
      expect(mockGetFeatureFlags.mock.calls[1][0]).toEqual({ ifNoneMatch: "" });
    });

    it("flagStore_should_DropValidator_When_AFlagWriteIsRecorded", async () => {
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", false)], etag: '"e1"' });
      await loadFlags();
      recordFlagWrite("x", true);
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", true)], etag: '"e2"' });
      await loadFlags();
      expect(mockGetFeatureFlags.mock.calls[1][0]).toEqual({ ifNoneMatch: "" });
    });

    it("flagStore_should_KeepWrittenValue_When_NotModifiedAnswersAPreWriteRequest", async () => {
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", false)], etag: '"e1"' });
      await loadFlags();
      let resolve!: (v: unknown) => void;
      mockGetFeatureFlags.mockReturnValueOnce(new Promise((r) => (resolve = r)));
      const pending = loadFlags(Date.now, { force: true });
      recordFlagWrite("x", true);
      resolve({ flags: [], etag: '"e1"', notModified: true });
      await pending;
      expect(peekFlags()?.[0].enabled).toBe(true);
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", true)], etag: '"e2"' });
      await loadFlags();
      expect(mockGetFeatureFlags.mock.calls[2][0]).toEqual({ ifNoneMatch: "" });
    });

    it("flagStore_should_ReturnFreshFlags_When_StaleNotModifiedFindsTheCacheCleared", async () => {
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", true)], etag: '"e1"' });
      await loadFlags();
      let resolve!: (v: unknown) => void;
      mockGetFeatureFlags.mockReturnValueOnce(new Promise((r) => (resolve = r)));
      const pending = loadFlags(Date.now, { force: true });
      resetFlagStoreForTest();
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", true)], etag: '"e2"' });
      resolve({ flags: [], etag: '"e1"', notModified: true });
      const flags = await pending;
      expect(flags).toHaveLength(1);
      expect(mockGetFeatureFlags.mock.calls[2][0]).toEqual({ ifNoneMatch: "" });
    });

    it("flagStore_should_NotCacheOrWipePersistedHints_When_ServerAnswersNotModifiedToAnEmptyValidator", async () => {
      localStorage.setItem("ssq:flag:unary_over_ws", "1");
      localStorage.setItem("ssq:flag:window_keep_alive", "1");
      mockGetFeatureFlags.mockResolvedValue({ flags: [], etag: '"e1"', notModified: true });
      const flags = await loadFlags();
      expect(flags).toEqual([]);
      expect(peekFlags()).toBeNull();
      expect(localStorage.getItem("ssq:flag:unary_over_ws")).toBe("1");
      expect(localStorage.getItem("ssq:flag:window_keep_alive")).toBe("1");
      expect(mockGetFeatureFlags).toHaveBeenCalledTimes(2);
    });

    it("flagStore_should_RefetchInFull_When_NotModifiedArrivesWithNothingToKeep", async () => {
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [], etag: '"e1"', notModified: true });
      mockGetFeatureFlags.mockResolvedValueOnce({ flags: [flag("x", true)], etag: '"e2"' });
      const flags = await loadFlags();
      expect(flags[0]).toMatchObject({ name: "x", enabled: true });
      expect(mockGetFeatureFlags.mock.calls[1][0]).toEqual({ ifNoneMatch: "" });
    });
  });
});
