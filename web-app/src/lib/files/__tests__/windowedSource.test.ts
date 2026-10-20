import { bufferReducer, initialBufferState, type BufferState } from "@/lib/files/bufferReducer";
import { writePersisted, storageKey } from "@/lib/hooks/useOpenBuffers";
import { installJsdomLayoutShims } from "./fixtures/jsdomLayout";
import { mountLazyEngine, type MountedLazy } from "./fixtures/lazyEngineAdapter";
import { createVirtualRangeTransport } from "./fixtures/virtualRangeTransport";
import { WINDOW_LINES, WindowedFile, type WindowedEngine } from "../windowedFile";
import { approxLines, classifyTier, decideOpen, lineCountOf, T0_MAX_BYTES, type OpenInputs } from "../windowedSource";

const MIB = 1024 * 1024;
const REF = { sessionId: "s1", path: "big.log" };
const base: OpenInputs = { size: 5 * MIB, readOnly: "none", analysis: "pending", ops: ["stat", "read_all", "read_lines"], hashCapBytes: 64 * MIB, rangeRead: true };

function fakeEngine() {
  const calls: string[] = [];
  const engine: WindowedEngine = {
    createBufferState: async (init) => { calls.push(`buffer:${init.lazy?.lineCount}`); return {} as never; },
    setBufferState: () => undefined,
    onNeedRange: () => undefined,
    provideRange: (r) => calls.push(`provide:${r.startLine}-${r.endLine}`),
    gotoLine: (n) => calls.push(`goto:${n}`),
    getCursor: () => ({ line: 1, col: 0 }),
    highlightRange: () => undefined,
    getViewport: () => ({ startLine: 1, endLine: 40 }),
  };
  return { engine, calls };
}

describe("windowedSource", () => {
  it("windowedSource_should_ClassifyT0T1T2_And_OpenAtAnchorLine_And_PersistOnlyAnchor", async () => {
    // T0 opens whole, T1 windowed, T2 windowed; windowed buffers are view-only with the TooLarge reason.
    expect(classifyTier(300 * 1024, 64 * MIB)).toBe("t0");
    expect(classifyTier(T0_MAX_BYTES, 64 * MIB)).toBe("t0");
    expect(classifyTier(5 * MIB, 64 * MIB)).toBe("t1");
    expect(classifyTier(200 * MIB, 64 * MIB)).toBe("t2");
    expect(decideOpen({ ...base, size: 300 * 1024 })).toEqual({ source: { kind: "full" }, readOnly: "none" });
    expect(decideOpen(base)).toMatchObject({ source: { kind: "windowed", tier: "t1" }, readOnly: "too_large" });
    expect(decideOpen({ ...base, size: 200 * MIB })).toMatchObject({ source: { kind: "windowed", tier: "t2" }, readOnly: "too_large" });
    // Flag off, or a server without READ_LINES, or a binary file: the base-plan state, no error.
    expect(decideOpen({ ...base, rangeRead: false })).toEqual({ source: { kind: "full" }, readOnly: "none" });
    expect(decideOpen({ ...base, ops: ["stat", "read_all"] })).toEqual({ source: { kind: "full" }, readOnly: "none" });
    expect(decideOpen({ ...base, readOnly: "binary" }).source).toEqual({ kind: "full" });

    // A windowed open at line 812,345 is one range read around it, painted and landed on that line.
    const t = createVirtualRangeTransport({ lines: 4_200_000 });
    const { engine, calls } = fakeEngine();
    const file = new WindowedFile(t, REF, t.size);
    await file.attach(engine, "text", 812_345);
    expect(t.reads).toHaveLength(1);
    expect(t.reads[0]).toMatchObject({ anchorLine: 812_345, dir: "around" });
    expect(calls).toEqual(["buffer:4200000", `provide:${812_345 - WINDOW_LINES / 2}-${812_345 + WINDOW_LINES / 2 - 1}`, "goto:812345"]);
    expect(file.getSnapshot()).toMatchObject({ lineCount: 4_200_000, exact: true, opened: true });

    // Persisted tab state holds the path and the anchor line, never chunk text or the source.
    let s: BufferState = bufferReducer(initialBufferState(), { type: "open", path: "big.log", sessionId: "s1" });
    s = bufferReducer(s, { type: "recordViewState", path: "big.log", viewState: { line: 812_345, col: 0, scrollTop: 0 } });
    s = bufferReducer(s, { type: "markLoaded", path: "big.log", token: "-", readOnly: "too_large", source: decideOpen(base).source });
    expect(s.buffers[0]).toMatchObject({ source: { kind: "windowed", windowStartLine: 812_345 } });
    expect(writePersisted("s1", s)).toBe(true);
    const stored = window.localStorage.getItem(storageKey("s1", "desktop")) ?? window.localStorage.getItem(storageKey("s1", "mobile")) ?? "";
    expect(JSON.parse(stored).tabs).toEqual([{ path: "big.log", line: 812_345, col: 0, scrollTop: 0 }]);
    expect(stored).not.toMatch(/line \d|windowed|source/);
  });

  it("lineCountOf_should_PreferExact_ThenEndOfFile_ThenEstimateFromBytesPerLine", () => {
    const r = (over: object) => ({ text: "", token: "-", size: 0, lineEnding: "lf", hasBom: false, readOnly: "none", isBinary: false, truncated: true, range: { startLine: 1, endLine: 100, startByte: 0, endByte: 1000, bof: true, eof: false }, ...over }) as never;
    expect(lineCountOf(1e6, r({ lineCount: 42 }))).toEqual({ count: 42, exact: true });
    expect(lineCountOf(1e6, r({ range: { startLine: 1, endLine: 100, startByte: 0, endByte: 1000, bof: true, eof: true } }))).toEqual({ count: 100, exact: true });
    expect(lineCountOf(1e6, r({}))).toEqual({ count: 100_000, exact: false });
    expect(approxLines(4_200_000)).toBe("4.2 million");
    expect(approxLines(12_345)).toBe("12,345");
  });
});

describe("WindowedFile on the production engine", () => {
  let host: HTMLElement;
  let mounted: MountedLazy;
  beforeAll(installJsdomLayoutShims);
  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
  });
  afterEach(() => {
    mounted?.handle.destroy();
    host.remove();
    jest.useRealTimers();
  });

  it("windowedGoTo_should_LandOnTheExactLine_AfterLoadingTheTargetWindow_And_ClampPastTheEnd", async () => {
    const t = createVirtualRangeTransport({ lines: 1_000_000 });
    mounted = await mountLazyEngine(host);
    const file = new WindowedFile(t, REF, t.size);
    await file.attach(mounted.adapter, "text", 1);
    expect(mounted.adapter.getCursor().line).toBe(1);

    for (const target of [500_000, 123_457, 999_999, 2]) {
      const before = t.reads.length;
      await expect(file.goTo(target)).resolves.toMatchObject({ line: target, clamped: false, cancelled: false });
      // The target window is read once up front; the cursor sits on the requested absolute line, not near it.
      expect(t.reads.length - before).toBeLessThanOrEqual(2);
      expect(mounted.adapter.getCursor().line).toBe(target);
    }

    const past = await file.goTo(5_000_000);
    expect(past).toMatchObject({ line: 1_000_000, clamped: true });
    expect(mounted.adapter.getCursor().line).toBe(1_000_000);
    expect(file.getSnapshot().notice).toContain("past the end");
    expect(file.getSnapshot().jump).toBeNull();
  });

  it("windowedOpen_should_QueueTheJumpBehindTheIndex_And_RebuildWhenTheExactCountDiffers", async () => {
    jest.useFakeTimers();
    let ready = false;
    const t = createVirtualRangeTransport({ lines: 900_000, size: 10_800_000, ready: () => ready });
    mounted = await mountLazyEngine(host);
    const file = new WindowedFile(t, REF, t.size);
    await file.attach(mounted.adapter, "text", 600_000);

    // Index pending: line 1 paints at once, the chip says so, and the jump waits instead of failing.
    expect(file.getSnapshot()).toMatchObject({ opened: true, exact: false, lineCount: 1_080_000 });
    expect(mounted.adapter.getCursor().line).toBe(1);
    ready = true;
    await jest.advanceTimersByTimeAsync(1600);
    await jest.advanceTimersByTimeAsync(10);
    expect(mounted.adapter.getCursor().line).toBe(600_000);
    expect(file.getSnapshot()).toMatchObject({ indexing: false, exact: true, lineCount: 900_000 });
  });
});
