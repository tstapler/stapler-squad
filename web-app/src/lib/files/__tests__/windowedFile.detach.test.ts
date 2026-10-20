import { createVirtualRangeTransport } from "./fixtures/virtualRangeTransport";
import { WindowedFile, type WindowedEngine } from "../windowedFile";
import type { LineRange } from "@/components/sessions/files/engine/EditorEngine";

const REF = { sessionId: "s1", path: "big.log" };

/** An engine whose buffer build can be held open, so the editor can go away mid-await. */
function gatedEngine() {
  const calls: string[] = [];
  let needRange: (r: LineRange) => void = () => undefined;
  let release: () => void = () => undefined;
  let gate: Promise<void> | null = null;
  const engine: WindowedEngine = {
    createBufferState: async () => { await gate; return {} as never; },
    setBufferState: () => calls.push("setBuffer"),
    onNeedRange: (cb) => { needRange = cb; },
    provideRange: (r) => calls.push(`provide:${r.startLine}-${r.endLine}`),
    gotoLine: (n) => calls.push(`goto:${n}`),
    getCursor: () => ({ line: 1, col: 0 }),
    highlightRange: () => undefined,
    getViewport: () => ({ startLine: 1, endLine: 40 }),
  };
  return {
    engine, calls, demand: (r: LineRange) => needRange(r),
    hold: () => { gate = new Promise<void>((res) => { release = res; }); },
    release: () => release(),
  };
}

describe("WindowedFile detach during an await", () => {
  it("windowedFile_should_NotTouchTheEngine_When_DetachedWhileMountBuildsTheBuffer", async () => {
    const t = createVirtualRangeTransport({ lines: 1000 });
    const g = gatedEngine();
    const file = new WindowedFile(t, REF, t.size);
    g.hold();
    const attached = file.attach(g.engine, "text", 1);
    await Promise.resolve();
    file.detach();
    g.release();
    await attached;

    expect(g.calls).toEqual([]);
    expect(file.getSnapshot().opened).toBe(false);
  });

  it("windowedFile_should_NotTouchTheEngine_When_DetachedWhileTheCountIsSettling", async () => {
    let ready = false;
    // Skewed size: the open path estimates 10,000 lines; the exact count (1,000) arrives with the next window.
    const t = createVirtualRangeTransport({ lines: 1000, size: 100_000, ready: () => ready });
    const g = gatedEngine();
    const file = new WindowedFile(t, REF, t.size);
    await file.attach(g.engine, "text", 1);
    g.calls.length = 0;

    ready = true;
    g.hold();
    g.demand({ startLine: 200, endLine: 260 });
    await new Promise((r) => setTimeout(r, 0));
    file.detach();
    g.release();
    await new Promise((r) => setTimeout(r, 0));

    expect(g.calls.filter((c) => c.startsWith("goto:") || c.startsWith("provide:"))).toEqual([]);
  });
});
