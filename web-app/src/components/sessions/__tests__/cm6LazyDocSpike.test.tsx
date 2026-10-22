/**
 * Task 0.1.4c (ADR-036 S-T3): jsdom half of the lazy-document spike. Both prototypes (a2 gap-line,
 * a1 window-swap) are checked against a full-replace oracle: every random edit is converted to
 * absolute file line/col through the prototype and applied to a plain array of lines; the
 * prototype's materialized file must then equal that array. Layout-dependent criteria (scroll
 * anchoring, heap) are measured in tests/e2e/files-lazydoc-spike.spec.ts.
 */
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { history, isolateHistory, undo, undoDepth } from "@codemirror/commands";
import { VirtualDoc, prng } from "@/app/spike-lazydoc/virtualDoc";
import {
  CHUNK_LINES, absLineOfDocLine, docPosToAbs, evictSegment, gapLineExtension, gapTable, loadChunk, loadedChars, materialize, splitGap,
} from "@/app/spike-lazydoc/gapLine";
import { WindowController, WINDOW_LINES } from "@/app/spike-lazydoc/windowSwap";

const ORACLE_LINES = 20_000;
const EDITS = 1000;
const PHONE_BUDGET_CHARS = 8 * 1024 * 1024;

interface AbsEdit { sl: number; sc: number; el: number; ec: number; insert: string }

function applyToOracle(full: string[], e: AbsEdit): void {
  const merged = full[e.sl].slice(0, e.sc) + e.insert + full[e.el].slice(e.ec);
  full.splice(e.sl, e.el - e.sl + 1, ...merged.split("\n"));
}

function randomText(rand: () => number): string {
  const alphabet = "abc xyz\n\n01_é\u{1F600}";
  const chars = [...alphabet];
  const n = Math.floor(rand() * 8);
  return Array.from({ length: n }, () => chars[Math.floor(rand() * chars.length)]).join("");
}

function mountGap(vdoc: VirtualDoc): EditorView {
  const parent = document.createElement("div");
  document.body.appendChild(parent);
  const { extension, initialDoc } = gapLineExtension(vdoc, splitGap(0, vdoc.lineCount));
  return new EditorView({ state: EditorState.create({ doc: initialDoc, extensions: [history(), extension] }), parent });
}

describe("lazy document spike (Task 0.1.4c)", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("lazyDocA2_should_EqualFullReplaceOracle_On1000RandomEditsAcrossGaps", () => {
    const vdoc = new VirtualDoc(ORACLE_LINES, 7);
    const view = mountGap(vdoc);
    const rand = prng(20261016);
    const full = vdoc.fetch(0, ORACLE_LINES);
    for (const start of [3000, 9000, 15000]) loadChunk(view, vdoc, start);

    for (let step = 1; step <= EDITS; step++) {
      const table = gapTable(view.state);
      const r = rand();
      if (r < 0.08) {
        loadChunk(view, vdoc, Math.floor(rand() * ORACLE_LINES));
      } else if (r < 0.12) {
        const clean = table.segs.map((s, i) => i).filter((i) => table.segs[i].kind === "loaded" && !table.segs[i].dirty);
        if (clean.length) evictSegment(view, clean[Math.floor(rand() * clean.length)]);
      } else {
        const loaded = table.segs.filter((s) => s.kind === "loaded");
        const seg = loaded[Math.floor(rand() * loaded.length)];
        const a = seg.from + Math.floor(rand() * (seg.to - seg.from + 1));
        const b = Math.min(seg.to, a + Math.floor(rand() * 12));
        const insert = randomText(rand);
        const s = docPosToAbs(view.state, a);
        const e = docPosToAbs(view.state, b);
        const before = view.state.doc.length;
        view.dispatch({ changes: { from: a, to: b, insert } });
        expect(view.state.doc.length).toBe(before - (b - a) + insert.length);
        applyToOracle(full, { sl: s.line, sc: s.col, el: e.line, ec: e.col, insert });
      }
      if (step % 100 === 0) expect(materialize(view.state, vdoc)).toEqual(full);
    }
    expect(materialize(view.state, vdoc)).toEqual(full);
    expect(gapTable(view.state).segs.some((s) => s.kind === "gap")).toBe(true);
  });

  it("lazyDocA2_should_RefuseEditsIntoPlaceholders_And_ShowAbsoluteLineNumbers", () => {
    const vdoc = new VirtualDoc(ORACLE_LINES, 7);
    const view = mountGap(vdoc);
    loadChunk(view, vdoc, 5000);
    const table = gapTable(view.state);
    const gap = table.segs.find((s) => s.kind === "gap")!;
    const docBefore = view.state.doc.toString();
    view.dispatch({ changes: { from: gap.from, to: gap.to, insert: "x" } });
    view.dispatch({ changes: { from: gap.to, insert: "y" } });
    expect(view.state.doc.toString()).toBe(docBefore);

    const loadedIdx = table.segs.findIndex((s) => s.kind === "loaded");
    const first = table.segs[loadedIdx];
    expect(absLineOfDocLine(view.state, view.state.doc.lineAt(first.from).number)).toBe(table.prefix[loadedIdx] + 1);
    expect(table.prefix[loadedIdx]).toBe(Math.floor(5000 / CHUNK_LINES) * CHUNK_LINES);
  });

  it("lazyDocA2_should_KeepUndoSelectionAndCleanState_Across100LoadEvictCycles", () => {
    const vdoc = new VirtualDoc(ORACLE_LINES, 7);
    const view = mountGap(vdoc);
    loadChunk(view, vdoc, 1000);
    const seg = gapTable(view.state).segs.find((s) => s.kind === "loaded")!;
    view.dispatch({ changes: { from: seg.from, insert: "edit1 " }, selection: { anchor: seg.from + 3 } });
    view.dispatch({ changes: { from: seg.from, insert: "edit2 " }, annotations: isolateHistory.of("full") });
    // isolate so the two edits are separate undo events
    const depth = undoDepth(view.state);
    const selRel = () => view.state.selection.main.head - gapTable(view.state).segs.find((x) => x.dirty)!.from;
    const selBefore = selRel();
    expect(depth).toBe(2);

    for (let c = 0; c < 100; c++) {
      const target = 8000 + (c % 20) * CHUNK_LINES;
      loadChunk(view, vdoc, target);
      const t = gapTable(view.state);
      const idx = t.segs.findIndex((s, i) => s.kind === "loaded" && !s.dirty && t.prefix[i] === target - (target % CHUNK_LINES));
      if (idx >= 0) evictSegment(view, idx);
    }
    expect(undoDepth(view.state)).toBe(depth);
    const dirty = gapTable(view.state).segs.filter((s) => s.dirty);
    expect(dirty).toHaveLength(1);
    expect(view.state.sliceDoc(dirty[0].from, dirty[0].from + 6)).toBe("edit2 ");
    expect(selRel()).toBe(selBefore);
    undo(view);
    expect(undoDepth(view.state)).toBe(depth - 1);
    expect(view.state.sliceDoc(dirty[0].from, dirty[0].from + 6)).toBe("edit1 ");
  });

  it("lazyDocA2_should_NotDispatchLoadOrEvict_While_ViewComposing_And_StayUnderPhoneBudget", () => {
    const vdoc = new VirtualDoc(1_000_000, 1);
    const view = mountGap(vdoc);
    let composing = true;
    Object.defineProperty(view, "composing", { get: () => composing, configurable: true });
    const spy = jest.spyOn(view, "dispatch");
    expect(loadChunk(view, vdoc, 400_000)).toBe(false);
    expect(spy).not.toHaveBeenCalled();
    composing = false;
    expect(loadChunk(view, vdoc, 400_000)).toBe(true);

    // Loading 600 distinct chunks while evicting the oldest keeps loaded text under the 8 MiB phone budget.
    const order: number[] = [];
    for (let c = 0; c < 600; c++) {
      const abs = c * 1500;
      loadChunk(view, vdoc, abs);
      order.push(abs);
      while (loadedChars(view.state) > PHONE_BUDGET_CHARS) {
        const t = gapTable(view.state);
        const idx = t.segs.findIndex((s) => s.kind === "loaded" && !s.dirty);
        evictSegment(view, idx);
      }
    }
    expect(loadedChars(view.state)).toBeLessThanOrEqual(PHONE_BUDGET_CHARS);
    expect(gapTable(view.state).segs.reduce((n, s) => n + s.lines, 0)).toBe(1_000_000);
  });

  it("lazyDocA1_should_EqualFullReplaceOracle_On1000RandomEdits_And_RefuseSwapWhileDirtyOrComposing", () => {
    const vdoc = new VirtualDoc(ORACLE_LINES, 7);
    const parent = document.createElement("div");
    document.body.appendChild(parent);
    const view = new EditorView({ state: EditorState.create({ doc: "" }), parent });
    const ctl = new WindowController(view, vdoc);
    const rand = prng(99);
    const full = vdoc.fetch(0, ORACLE_LINES);
    expect(ctl.swapTo(10_000)).toBe("swapped");

    for (let i = 0; i < EDITS; i++) {
      const a = Math.floor(rand() * (view.state.doc.length + 1));
      const b = Math.min(view.state.doc.length, a + Math.floor(rand() * 12));
      const insert = randomText(rand);
      const s = ctl.posToAbs(a);
      const e = ctl.posToAbs(b);
      view.dispatch({ changes: { from: a, to: b, insert } });
      applyToOracle(full, { sl: s.line, sc: s.col, el: e.line, ec: e.col, insert });
    }
    const window = view.state.doc.toString().split("\n");
    const edited = full.slice(ctl.start, ctl.start + window.length);
    expect(window).toEqual(edited);

    expect(ctl.swapTo(500)).toBe("refused-dirty");
    expect(ctl.start).toBe(10_000 - 1 - WINDOW_LINES / 2);
    Object.defineProperty(view, "composing", { get: () => true, configurable: true });
    expect(ctl.swapTo(500)).toBe("refused-composing");
  });
});
