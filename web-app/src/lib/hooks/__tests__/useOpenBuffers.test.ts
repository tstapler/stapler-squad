import { renderHook, act, waitFor } from "@testing-library/react";
import { createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import { BufferLoadError, storageKey, useOpenBuffers, type StatFn } from "../useOpenBuffers";

const okStat: StatFn = async () => ({ token: "v1:1:1:x", readOnly: "none" });
const KEY = storageKey("s1", "desktop");
const vs = (line: number) => ({ line, col: 2, scrollTop: line * 5 });

function setup(stat: StatFn = okStat, extra: { initialSelectedPath?: string | null; onSelectedPathChange?: (p: string | null) => void } = {}) {
  const h = createFilesTestHarness({ sessionId: "s1" });
  const view = renderHook(() => useOpenBuffers("s1", { stat, ...extra }), { wrapper: h.wrapper });
  return { h, ...view };
}

describe("useOpenBuffers", () => {
  beforeEach(() => window.localStorage.clear());
  afterEach(() => jest.restoreAllMocks());

  it("useOpenBuffers_should_RestorePathsActiveIndexViewState_When_RemountedSameSessionDevice", async () => {
    const first = setup();
    act(() => { first.result.current.open("a.go", { pinned: true }); first.result.current.open("b.go", { pinned: true }); first.result.current.open("c.go", { pinned: true }); });
    act(() => first.result.current.recordViewState("b.go", vs(40)));
    act(() => first.result.current.activate("b.go"));
    await waitFor(() => expect(first.result.current.state.buffers.every((b) => b.status === "loaded")).toBe(true));
    first.unmount();

    const stored = JSON.parse(window.localStorage.getItem(KEY) as string);
    expect(stored.tabs.map((t: { path: string }) => t.path)).toEqual(["a.go", "b.go", "c.go"]);
    expect(JSON.stringify(stored)).not.toMatch(/content|token|text/);

    const second = setup();
    expect(second.result.current.state.buffers.map((b) => b.path)).toEqual(["a.go", "b.go", "c.go"]);
    expect(second.result.current.selectedPath).toBe("b.go");
    expect(second.result.current.state.buffers[1].viewState).toEqual(vs(40));
    expect(second.h.analytics.events.filter((e) => e.name === "files.tab.restore").map((e) => e.labels)).toEqual([
      expect.objectContaining({ outcome: "ok", tabs: "2-3" }),
    ]);
    await waitFor(() => expect(second.result.current.state.buffers.every((b) => b.status === "loaded")).toBe(true));
  });

  it("useOpenBuffers_should_WorkInMemory_When_LocalStorageThrows", async () => {
    jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("denied"); });
    jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("denied"); });
    const { result, h } = setup();
    expect(result.current.storageUnavailable).toBe(true);
    act(() => result.current.open("a.go"));
    await waitFor(() => expect(result.current.active?.status).toBe("loaded"));
    expect(result.current.selectedPath).toBe("a.go");
    expect(h.analytics.events.find((e) => e.name === "files.tab.restore")?.labels).toMatchObject({ outcome: "storage_unavailable" });
  });

  it("openBuffers_should_ShowNotFoundTabsAndOneTimeNotice_When_FileMissingOrStorageThrows", async () => {
    window.localStorage.setItem(KEY, JSON.stringify({ v: 1, active: 0, tabs: [{ path: "gone.go", line: 1, col: 0, scrollTop: 0 }, { path: "ok.go", line: 3, col: 0, scrollTop: 0 }] }));
    const stat: StatFn = async ({ path }) => {
      if (path === "gone.go") throw new BufferLoadError("not_found");
      return { token: "t", readOnly: "none" };
    };
    const { result } = setup(stat);
    await waitFor(() => expect(result.current.state.buffers.map((b) => b.status)).toEqual(["failed", "loaded"]));
    expect(result.current.state.buffers[0]).toMatchObject({ status: "failed", reason: "not_found" });
    expect(result.current.storageUnavailable).toBe(false);
  });

  it("useOpenBuffers_should_KeepBackCompat_When_InitialSelectedPathProvided", async () => {
    const onChange = jest.fn();
    const { result, rerender } = setup(okStat, { initialSelectedPath: "src/main.go", onSelectedPathChange: onChange });
    expect(result.current.selectedPath).toBe("src/main.go");
    expect(onChange).toHaveBeenCalledWith("src/main.go");
    act(() => result.current.open("other.go", { pinned: true }));
    expect(onChange).toHaveBeenLastCalledWith("other.go");
    rerender();
    await waitFor(() => expect(result.current.state.buffers.every((b) => b.status === "loaded")).toBe(true));
  });

  it("useOpenBuffers_should_StatWithTheBufferSession_And_CarryReadOnlyKind", async () => {
    const stat = jest.fn<ReturnType<StatFn>, Parameters<StatFn>>(async () => ({ token: "t", readOnly: "binary" }));
    const { result } = setup(stat);
    act(() => result.current.open("img.png"));
    await waitFor(() => expect(result.current.active).toMatchObject({ status: "loaded", readOnly: "binary" }));
    expect(stat.mock.calls[0][0]).toEqual({ sessionId: "s1", path: "img.png" });
  });
});
