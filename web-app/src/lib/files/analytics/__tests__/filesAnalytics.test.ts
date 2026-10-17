import { renderHook, waitFor, act } from "@testing-library/react";
import { createFilesTestHarness } from "../../__tests__/fixtures";
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import { useFilesAnalytics } from "../filesAnalytics";

function setViewport(width: number, coarse: boolean) {
  (window as unknown as { innerWidth: number }).innerWidth = width;
  window.matchMedia = ((query: string) => ({
    matches: coarse && query.includes("coarse"), media: query, addEventListener: jest.fn(), removeEventListener: jest.fn(),
  })) as unknown as typeof window.matchMedia;
}

describe("useFilesAnalytics", () => {
  afterEach(() => setViewport(1024, false));

  it("filesAnalytics_should_StampFormInputEditingV2AndUpdateWithoutRemount_When_ViewportProfileOrFlagChanges", async () => {
    let flagList = [{ name: "files:editing", enabled: false }];
    const h = createFilesTestHarness({ handlers: { getFeatureFlags: () => ({ flags: flagList }) } });
    setViewport(1280, false);
    const { result } = renderHook(() => ({ fa: useFilesAnalytics(), ff: useFeatureFlags() }), { wrapper: h.wrapper });
    await waitFor(() => expect(result.current.ff.isLoading).toBe(false));
    const first = result.current.fa;

    first.emit("files.tab.evict_undo");
    expect(h.analytics.events[0].labels).toEqual({ form: "desktop", input: "hardware", editing: "off", v2: "on" });

    setViewport(500, true);
    flagList = [{ name: "files:editing", enabled: true }];
    await act(async () => { await result.current.ff.refresh?.(); });
    result.current.fa.emit("files.tab.evict_undo");

    expect(h.analytics.events[1].labels).toEqual({ form: "mobile", input: "touch", editing: "on", v2: "on" });
    expect(result.current.fa).toBe(first);
    expect(h.analytics.events[1]).toMatchObject({ name: "files.tab.evict_undo", category: "user_action", component: "FilesTab" });
  });

  it("filesAnalytics_should_NotThrowOrBlock_When_ProviderThrowsOrContextMissing", () => {
    const h = createFilesTestHarness();
    h.analytics.track = () => { throw new Error("boom"); };
    const { result } = renderHook(() => useFilesAnalytics(), { wrapper: h.wrapper });
    expect(() => result.current.emit("files.tab.evict_undo")).not.toThrow();
    expect(() => result.current.time("files.perf.save_ms")({ outcome: "saved", size: "lt10k" })).not.toThrow();

    const bare = renderHook(() => useFilesAnalytics());
    expect(() => bare.result.current.emit("files.tab.evict_undo")).not.toThrow();
    expect(() => bare.result.current.emitExtraKey("esc")).not.toThrow();
  });

  it("filesAnalytics_should_TimePerformanceEventsAndCarrySessionAndComponent", () => {
    const h = createFilesTestHarness();
    const { result } = renderHook(() => useFilesAnalytics({ component: "CodeEditor", sessionId: "s1" }), { wrapper: h.wrapper });
    result.current.time("files.perf.open_ms")({ size: "lt10k", degraded: "false", tier: "t0" });
    expect(h.analytics.events[0]).toMatchObject({ category: "performance", sessionId: "s1", component: "CodeEditor" });
    expect(typeof h.analytics.events[0].durationMs).toBe("number");
  });

  it("filesAnalytics_should_FlushAggregatedKeysOnPagehide", () => {
    const h = createFilesTestHarness();
    const { result } = renderHook(() => useFilesAnalytics(), { wrapper: h.wrapper });
    for (let i = 0; i < 7; i++) result.current.emitExtraKey("arrow");
    expect(h.analytics.events).toHaveLength(0);
    window.dispatchEvent(new Event("pagehide"));
    expect(h.analytics.events).toHaveLength(1);
    expect(h.analytics.events[0]).toMatchObject({ name: "files.mobile.extra_key_used", labels: { key_class: "arrow", count: "6-20" } });
  });
});
