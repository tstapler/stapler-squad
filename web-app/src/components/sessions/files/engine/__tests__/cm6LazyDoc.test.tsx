import { isolateHistory, undo, undoDepth } from "@codemirror/commands";
import { installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures/jsdomLayout";
import { prng } from "@/lib/__tests__/virtualFile";
import { CHUNK_LINES, absLineOfDocLine, gapTableOf, loadedChars } from "../cm6LazyDoc/gapTable";
import { placeholderField } from "../cm6LazyDoc/placeholderWidget";
import { RESIDENT_BUDGET_CHARS } from "../cm6LazyDoc/gapLineController";
import { applyEdits, openLazy, provide, type OpenLazy } from "./lazyDocFixtures";

const ORACLE_LINES = 20_000;
const EDIT_SCRIPTS = 1000;

describe("cm6LazyDoc gap-line model (Story 1.6.4)", () => {
  let host: HTMLElement;
  let o: OpenLazy;

  beforeAll(installJsdomLayoutShims);
  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
  });
  afterEach(() => {
    o?.handle.destroy();
    host.remove();
  });

  const segs = () => gapTableOf(o.view.state).segs;
  const firstLoaded = () => segs().findIndex((s) => s.kind === "loaded");

  it("lazyDoc_should_ShowAbsoluteLineNumbersAndRefuseEditsIntoPlaceholders_And_KeepUndoDepthAcross100LoadEvictCycles", async () => {
    o = await openLazy(host, 1_000_000);
    provide(o, 1, 400);
    provide(o, 500_000, 400);

    // One document: loaded lines plus one placeholder line per gap; the gutter reads true file lines.
    const table = gapTableOf(o.view.state);
    const secondWindow = table.segs.findIndex((s, i) => s.kind === "loaded" && table.prefix[i] === 499_999);
    expect(secondWindow).toBeGreaterThan(0);
    const docLine = (i: number) => o.view.state.doc.lineAt(table.segs[i].from).number;
    expect(absLineOfDocLine(o.view.state, docLine(secondWindow))).toBe(500_000);
    expect(absLineOfDocLine(o.view.state, docLine(secondWindow) - 1)).toBe(table.prefix[secondWindow - 1] + 1);
    expect(absLineOfDocLine(o.view.state, docLine(1))).toBe(401);
    expect(o.view.state.field(placeholderField).size).toBe(table.segs.filter((s) => s.kind === "gap").length);

    // Placeholders cannot be typed into, deleted into or partly selected for deletion.
    const gap = segs().find((s) => s.kind === "gap")!;
    const before = o.view.state.doc.toString();
    o.view.dispatch({ changes: { from: gap.from, to: gap.to, insert: "x" } });
    o.view.dispatch({ changes: { from: gap.to, insert: "y" } });
    o.view.dispatch({ changes: { from: gap.from - 1, to: gap.to + 1 } });
    expect(o.view.state.doc.toString()).toBe(before);

    // 100 load/evict cycles interleaved with 20 user edits: undo depth is the number of edits.
    const target = segs()[firstLoaded()];
    for (let edit = 0; edit < 20; edit++) {
      o.view.dispatch({
        changes: { from: target.from, insert: `e${edit} ` }, annotations: isolateHistory.of("full"),
      });
      for (let c = 0; c < 5; c++) {
        const start = 100_000 + ((edit * 5 + c) % 40) * CHUNK_LINES;
        provide(o, start, CHUNK_LINES);
        o.handle.evictRange({ startLine: start, endLine: start + CHUNK_LINES - 1 });
      }
    }
    expect(undoDepth(o.view.state)).toBe(20);
    const userText = o.view.state.sliceDoc(segs()[firstLoaded()].from, segs()[firstLoaded()].from + 7);
    expect(userText).toBe("e19 e18");
    undo(o.view);
    expect(undoDepth(o.view.state)).toBe(19);
    expect(o.view.state.sliceDoc(segs()[firstLoaded()].from, segs()[firstLoaded()].from + 7)).toBe("e18 e17");
  });

  it("eviction_should_KeepViewportSelectionAndUndo_And_NeverEvictDirtyRegion", async () => {
    o = await openLazy(host, 5_000);
    provide(o, 1, 100);
    provide(o, 2_001, 100);
    const edited = segs()[firstLoaded()];
    o.view.dispatch({ changes: { from: edited.from, insert: "dirty " }, selection: { anchor: edited.from + 3 } });
    const depth = undoDepth(o.view.state);
    const selection = o.view.state.selection.main.head;
    const dirtyText = o.view.state.sliceDoc(edited.from, edited.from + 7);

    o.handle.evictRange({ startLine: 1, endLine: 100 });
    expect(segs()[firstLoaded()].from).toBe(edited.from);
    expect(o.view.state.sliceDoc(edited.from, edited.from + 7)).toBe(dirtyText);

    o.handle.evictRange({ startLine: 2_001, endLine: 2_100 });
    expect(segs().filter((s) => s.kind === "loaded")).toHaveLength(1);
    expect(undoDepth(o.view.state)).toBe(depth);
    expect(o.view.state.selection.main.head).toBe(selection);
    o.handle.ack("t1");
    o.handle.takePending();
    o.handle.ack("t2");
    o.handle.evictRange({ startLine: 1, endLine: 100 });
    expect(segs().every((s) => s.kind === "gap")).toBe(true);
  });

  it("resident_should_StayUnderTheBudget_When_TheViewportSweepsAFiftyMegabyteFile", async () => {
    o = await openLazy(host, 1_000_000, "gap-line", 1);
    for (let start = 1; start < 1_000_000; start += 20_000) {
      provide(o, start, 5_000);
      expect(loadedChars(o.view.state)).toBeLessThanOrEqual(RESIDENT_BUDGET_CHARS);
    }
    expect(segs().reduce((n, s) => n + s.lines, 0)).toBe(1_000_000);
    expect(loadedChars(o.view.state)).toBeGreaterThan(0);
  });

  it("pendingToAbsolute_should_EqualFullReplaceOracle_For1000SeededEditScriptsAcrossGaps", async () => {
    o = await openLazy(host, ORACLE_LINES);
    const rand = prng(20261019);
    let acked = o.file.fetch(0, ORACLE_LINES);
    const oracle = [...acked];
    const randomText = () => {
      const chars = [..."abc xyz\n\n01_é\u{1F600}"];
      return Array.from({ length: Math.floor(rand() * 8) }, () => chars[Math.floor(rand() * chars.length)]).join("");
    };
    const file = o.file;
    const spy = jest.spyOn(file, "fetch").mockImplementation((start, count) => acked.slice(start, start + count));
    for (const start of [3000, 9000, 15000]) provide(o, start, 200);

    for (let step = 1; step <= EDIT_SCRIPTS; step++) {
      const r = rand();
      if (r < 0.08) {
        provide(o, 1 + Math.floor(rand() * (ORACLE_LINES - 300)), 100 + Math.floor(rand() * 200));
      } else if (r < 0.12) {
        const from = 1 + Math.floor(rand() * ORACLE_LINES);
        o.handle.evictRange({ startLine: from, endLine: from + 300 });
      } else {
        const loaded = segs().filter((s) => s.kind === "loaded");
        const seg = loaded[Math.floor(rand() * loaded.length)];
        const a = seg.from + Math.floor(rand() * (seg.to - seg.from + 1));
        const b = Math.min(seg.to, a + Math.floor(rand() * 12));
        const insert = randomText();
        const pos = (p: number) => {
          const line = o.view.state.doc.lineAt(p);
          return { line: absLineOfDocLine(o.view.state, line.number) - 1, col: p - line.from };
        };
        const [s, e] = [pos(a), pos(b)];
        o.view.dispatch({ changes: { from: a, to: b, insert } });
        const merged = oracle[s.line].slice(0, s.col) + insert + oracle[e.line].slice(e.col);
        oracle.splice(s.line, e.line - s.line + 1, ...merged.split("\n"));
      }
      if (step % 100 === 0) {
        expect(applyEdits(acked, o.handle.takePending().edits)).toEqual(oracle);
      }
      if (step === 500) {
        o.handle.takePending();
        o.handle.ack("saved");
        acked = [...oracle];
      }
    }
    expect(applyEdits(acked, o.handle.takePending().edits)).toEqual(oracle);
    expect(segs().some((s) => s.kind === "gap")).toBe(true);
    spy.mockRestore();
  });
});
