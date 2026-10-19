import { installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures/jsdomLayout";
import { prng } from "@/lib/__tests__/virtualFile";
import { CmEngine } from "../CmEngine";
import { WINDOW_LINES } from "../cm6LazyDoc/windowSwap";
import type { LineRange } from "../EditorEngine";
import { applyEdits, openLazy, provide, type OpenLazy } from "./lazyDocFixtures";

const LINES = 20_000;

describe("cm6LazyDoc window-swap fallback (Task 1.6.4d)", () => {
  let host: HTMLElement;
  let o: OpenLazy;
  let needs: LineRange[];

  beforeAll(installJsdomLayoutShims);
  beforeEach(async () => {
    host = document.createElement("div");
    document.body.appendChild(host);
    needs = [];
    o = await openLazy(host, LINES, "window-swap");
    o.handle.onNeedRange((r) => needs.push(r));
    await new Promise((r) => setTimeout(r, 5));
  });
  afterEach(() => {
    o.handle.destroy();
    host.remove();
  });

  const gutterStart = () => o.view.state.doc.lines && Number(host.querySelector(".cm-lineNumbers .cm-gutterElement:nth-child(2)")?.textContent);

  it("windowSwap_should_StayOffByDefault_And_AskForTheFirstWindowThroughOnNeedRange", () => {
    expect(new CmEngine().lazyMode).toBe("gap-line");
    expect(needs).toEqual([{ startLine: 1, endLine: WINDOW_LINES }]);
  });

  it("windowSwap_should_EqualFullReplaceOracle_On1000RandomEdits_And_RefuseSwapWhileDirtyOrComposing", async () => {
    provide(o, 1, WINDOW_LINES);
    o.handle.reveal(10_000);
    const ask = needs[needs.length - 1];
    expect(ask.startLine).toBeLessThanOrEqual(10_000);
    expect(ask.endLine).toBeGreaterThanOrEqual(10_000);
    provide(o, ask.startLine, ask.endLine - ask.startLine + 1);
    expect(o.handle.getCursor().line).toBe(10_000);
    expect(gutterStart()).toBeGreaterThan(1);

    const rand = prng(99);
    const acked = o.file.fetch(0, LINES);
    const oracle = [...acked];
    const start = ask.startLine - 1;
    for (let i = 0; i < 1000; i++) {
      const doc = o.view.state.doc;
      const a = Math.floor(rand() * (doc.length + 1));
      const b = Math.min(doc.length, a + Math.floor(rand() * 12));
      const insert = ["", "x", "é\n", "😀", "ab\ncd"][Math.floor(rand() * 5)];
      const [sl, el] = [doc.lineAt(a), doc.lineAt(b)];
      const merged = oracle[start + sl.number - 1].slice(0, a - sl.from) + insert + oracle[start + el.number - 1].slice(b - el.from);
      o.view.dispatch({ changes: { from: a, to: b, insert } });
      oracle.splice(start + sl.number - 1, el.number - sl.number + 1, ...merged.split("\n"));
    }
    expect(applyEdits(acked, o.handle.takePending().edits)).toEqual(oracle);

    // Pinned while dirty: a swap request is dropped and the window stays.
    o.handle.reveal(500);
    provide(o, 1, WINDOW_LINES);
    expect(o.handle.getViewport().startLine).toBeGreaterThan(WINDOW_LINES);

    // Composing: the swap waits for compositionend, and only then (clean again) applies.
    o.handle.ack("saved");
    const textbox = host.querySelector('[role="textbox"]')!;
    textbox.dispatchEvent(new Event("compositionstart", { bubbles: true }));
    provide(o, 1, WINDOW_LINES);
    expect(o.handle.getViewport().startLine).toBeGreaterThan(WINDOW_LINES);
    textbox.dispatchEvent(new Event("compositionend", { bubbles: true }));
    await new Promise((r) => setTimeout(r, 5));
    expect(o.handle.getText().startsWith("L1 ")).toBe(true);
  }, 60_000); // ~1.8s idle but 1000 CM6 dispatches over 20k lines took 6.4s on a loaded CI worker; this is a work bound, not a perf assertion
});
