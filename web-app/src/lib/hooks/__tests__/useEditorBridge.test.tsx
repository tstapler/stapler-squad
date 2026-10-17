import { act, renderHook, waitFor } from "@testing-library/react";
import { createFakeTransport, createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import type { CodeEditorHandle } from "@/components/sessions/CodeEditor";
import { useOpenBuffers } from "../useOpenBuffers";
import { useEditorBridge } from "../useEditorBridge";

jest.mock("../useFileService", () => ({ invalidateFileContent: jest.fn() }));
import { invalidateFileContent } from "../useFileService";

const FILES = { "a.go": "x", "b.go": "y", "c.go": "z" };

function fakeHandle(text: string, cursor = { line: 1, col: 0 }) {
  const snapshot = { kind: "scroll-snapshot" } as const;
  return {
    getText: () => text, getCursor: jest.fn(() => cursor), gotoLine: jest.fn(), scrollSnapshot: jest.fn(() => snapshot),
    restoreScroll: jest.fn(), snapshot,
  };
}
const asHandle = (h: ReturnType<typeof fakeHandle>) => h as unknown as CodeEditorHandle;

function setup() {
  const h = createFilesTestHarness({ sessionId: "s1", transport: createFakeTransport({ files: FILES }) });
  const view = renderHook(() => {
    const buffers = useOpenBuffers("s1");
    return { buffers, bridge: useEditorBridge(buffers) };
  }, { wrapper: h.wrapper });
  return { h, ...view };
}

const lines = (n: number) => Array.from({ length: n }, (_, i) => `l${i + 1}`).join("\n");

describe("useEditorBridge", () => {
  beforeEach(() => { window.localStorage.clear(); jest.clearAllMocks(); });

  it("editorBridge_should_RevealPendingCursorOnce_When_EditorForThatPathIsReady", async () => {
    const { result } = setup();
    act(() => result.current.bridge.jumpTo("a.go", 42, { pinned: true, source: "deeplink" }));
    expect(result.current.buffers.state.pendingReveal).toMatchObject({ path: "a.go", line: 42 });
    const other = fakeHandle(lines(100));
    act(() => result.current.bridge.onEditorReady(asHandle(other), "b.go"));
    expect(other.gotoLine).not.toHaveBeenCalled();
    const editor = fakeHandle(lines(100));
    act(() => result.current.bridge.onEditorReady(asHandle(editor), "a.go"));
    expect(editor.gotoLine).toHaveBeenCalledTimes(1);
    expect(editor.gotoLine).toHaveBeenCalledWith(42);
    expect(result.current.buffers.state.pendingReveal).toBeNull();
  });

  it("editorBridge_should_ClampGoToLine_And_EmitJumpLineWithoutLineNumbers_When_9999On300Lines", async () => {
    const { result, h } = setup();
    act(() => result.current.buffers.open("a.go", { pinned: true }));
    const editor = fakeHandle(lines(300));
    act(() => result.current.bridge.onEditorReady(asHandle(editor), "a.go"));
    act(() => result.current.bridge.goToLine(250, "palette"));
    expect(editor.gotoLine).toHaveBeenLastCalledWith(250);
    act(() => result.current.bridge.goToLine(9999, "palette"));
    expect(editor.gotoLine).toHaveBeenLastCalledWith(300);
    const jumps = h.analytics.events.filter((e) => e.name === "files.nav.jump_line").map((e) => e.labels);
    expect(jumps).toHaveLength(2);
    expect(jumps[0]).toMatchObject({ via: "palette", clamped: "false" });
    expect(jumps[1]).toMatchObject({ via: "palette", clamped: "true" });
    expect(JSON.stringify(jumps)).not.toMatch(/250|9999|300/);
  });

  it("editorBridge_should_RecordCursorBeforeLeaving_And_EmitBackForward_When_JumplistKeys", async () => {
    const { result, h } = setup();
    act(() => { result.current.buffers.open("a.go", { pinned: true }); });
    const a = fakeHandle(lines(100), { line: 10, col: 0 });
    act(() => result.current.bridge.onEditorReady(asHandle(a), "a.go"));
    act(() => result.current.bridge.open("b.go", { pinned: true }));
    await waitFor(() => expect(result.current.buffers.state.buffers.map((b) => b.status)).toEqual(["loaded", "loaded"]));
    expect(result.current.buffers.state.buffers[0].viewState.line).toBe(10);
    act(() => result.current.bridge.back("keyboard"));
    expect(result.current.buffers.selectedPath).toBe("a.go");
    expect(a.gotoLine).toHaveBeenLastCalledWith(10);
    act(() => result.current.bridge.forward("button"));
    expect(result.current.buffers.selectedPath).toBe("b.go");
    const nav = h.analytics.events.filter((e) => e.name.startsWith("files.nav.")).map((e) => [e.name, e.labels?.via, e.labels?.depth]);
    expect(nav).toEqual([["files.nav.back", "keyboard", "1"], ["files.nav.forward", "button", "1"]]);
  });

  it("editorBridge_should_ReloadKeepingLineAndScroll_When_BannerReloadChosen", async () => {
    const { result } = setup();
    act(() => result.current.buffers.open("a.go", { pinned: true }));
    await waitFor(() => expect(result.current.buffers.active?.status).toBe("loaded"));
    const before = fakeHandle(lines(100), { line: 33, col: 4 });
    act(() => result.current.bridge.onEditorReady(asHandle(before), "a.go"));
    const key = result.current.bridge.reloadKey;
    act(() => result.current.bridge.reload("v1:9:9:new"));
    expect(invalidateFileContent).toHaveBeenCalledWith("s1", "a.go");
    expect(result.current.bridge.reloadKey).toBe(key + 1);
    expect(result.current.buffers.active).toMatchObject({ token: "v1:9:9:new" });
    expect(result.current.buffers.state.nav).toHaveLength(1);
    const after = fakeHandle(lines(100));
    act(() => result.current.bridge.onEditorReady(asHandle(after), "a.go"));
    expect(after.gotoLine).toHaveBeenCalledWith(33);
    expect(after.restoreScroll).toHaveBeenCalledWith(before.snapshot);
  });

  it("editorBridge_should_BuildOutlineFromEditorText_And_ToggleOverlays", async () => {
    const { result } = setup();
    act(() => result.current.buffers.open("a.go", { pinned: true }));
    act(() => result.current.bridge.onEditorReady(asHandle(fakeHandle("package p\n\nfunc A() {}\n")), "a.go"));
    expect(result.current.bridge.outline()).toEqual({ kind: "symbols", symbols: [{ name: "A", line: 3, kind: "function", depth: 0 }] });
    expect(result.current.bridge.overlay).toBeNull();
    act(() => result.current.bridge.setOverlay("outline"));
    expect(result.current.bridge.overlay).toBe("outline");
  });
});
