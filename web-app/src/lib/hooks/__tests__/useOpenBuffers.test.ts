import { renderHook, act, waitFor } from "@testing-library/react";
import { createFakeTransport, createFilesTestHarness, type FakeTransport } from "@/lib/files/__tests__/fixtures";
import { readFileSync } from "fs";
import { resolve } from "path";
import { Rejected } from "@/lib/files/transport/fileTransport";
import { storageKey, useOpenBuffers } from "../useOpenBuffers";

const KEY = storageKey("s1", "desktop");
const vs = (line: number) => ({ line, col: 2, scrollTop: line * 5 });

const FILES = { "a.go": "a", "b.go": "b", "c.go": "c", "ok.go": "ok", "src/main.go": "m", "other.go": "o", "img.png": "p" };

function setup(extra: { initialSelectedPath?: string | null; onSelectedPathChange?: (p: string | null) => void; transport?: FakeTransport } = {}) {
  const transport = extra.transport ?? createFakeTransport({ files: FILES });
  const h = createFilesTestHarness({ sessionId: "s1", transport });
  const view = renderHook(() => useOpenBuffers("s1", extra), { wrapper: h.wrapper });
  return { h, transport, ...view };
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
    const { result } = setup();
    await waitFor(() => expect(result.current.state.buffers.map((b) => b.status)).toEqual(["failed", "loaded"]));
    expect(result.current.state.buffers[0]).toMatchObject({ status: "failed", reason: "not_found" });
    expect(result.current.storageUnavailable).toBe(false);
  });

  it("useOpenBuffers_should_KeepBackCompat_When_InitialSelectedPathProvided", async () => {
    const onChange = jest.fn();
    const { result, rerender } = setup({ initialSelectedPath: "src/main.go", onSelectedPathChange: onChange });
    expect(result.current.selectedPath).toBe("src/main.go");
    expect(onChange).toHaveBeenCalledWith("src/main.go");
    act(() => result.current.open("other.go", { pinned: true }));
    expect(onChange).toHaveBeenLastCalledWith("other.go");
    rerender();
    await waitFor(() => expect(result.current.state.buffers.every((b) => b.status === "loaded")).toBe(true));
  });

  it("useOpenBuffers_should_StatWithTheBufferSession_And_CarryReadOnlyKind", async () => {
    const transport = createFakeTransport({ files: FILES });
    const original = transport.stat;
    transport.stat = async (ref, o, signal) => ({ ...(await original(ref, o, signal)), readOnly: "binary" });
    const { result } = setup({ transport });
    act(() => result.current.open("img.png"));
    await waitFor(() => expect(result.current.active).toMatchObject({ status: "loaded", readOnly: "binary" }));
    expect(transport.calls).toEqual(["stat:img.png"]);
  });

  it("hooks_should_ObtainTransportFromContext_And_ImportNoGeneratedClient", async () => {
    const transport = createFakeTransport({ files: FILES });
    const { result } = setup({ transport });
    act(() => result.current.open("a.go"));
    await waitFor(() => expect(result.current.active?.status).toBe("loaded"));
    expect(transport.calls).toContain("stat:a.go");

    const hooks = ["useOpenBuffers.ts", "useBufferFreshness.ts", "useSaveBuffer.ts"];
    for (const file of hooks) {
      let src = "";
      try { src = readFileSync(resolve(__dirname, "..", file), "utf8"); } catch { continue; }
      expect([file, src.match(/createClient|session_connect|getConnectTransport|useFileService|SessionService/)]).toEqual([file, null]);
    }
  });

  it("useOpenBuffers_should_MarkFailed_When_TransportRejectsNotFound", async () => {
    const transport = createFakeTransport({ files: FILES });
    transport.failures.stat = [new Rejected("not_found")];
    const { result } = setup({ transport });
    act(() => result.current.open("a.go"));
    await waitFor(() => expect(result.current.active).toMatchObject({ status: "failed", reason: "not_found" }));
  });
});
