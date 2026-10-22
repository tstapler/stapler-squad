// THROWAWAY (Task 0.1.4c, ADR-036 S-T3) prototype a2: gap-line model. Loaded lines plus one block-replace
// placeholder line per unloaded gap; the gap table lives in a StateField; load/evict are non-history
// transactions; changeFilter and atomicRanges protect the placeholders. Promoted to cm6LazyDoc in Story 1.6.4.
import {
  Annotation, EditorState, StateEffect, StateField, Transaction, type Extension, type Text,
} from "@codemirror/state";
import { Decoration, EditorView, WidgetType, lineNumbers, type DecorationSet } from "@codemirror/view";
import type { VirtualDoc } from "./virtualDoc";

export const LINE_HEIGHT = 20;
export const CHUNK_LINES = 500;
/** One placeholder never spans more than this many lines: a 250k-line (5M px) block defeated CM6's height scaler (jump landed at line 247,375 instead of 500,000). */
export const MAX_GAP_LINES = 5000;
const PLACEHOLDER = "​";

export interface Seg {
  kind: "loaded" | "gap";
  from: number;
  to: number;
  /** Lines this segment stands for in the current file (placeholder: unloaded lines). */
  lines: number;
  /** First line index of this segment in the base (unedited) file. */
  baseStart: number;
  dirty: boolean;
}

export interface GapTable {
  segs: Seg[];
  /** prefix[i] = file lines before segs[i]. */
  prefix: number[];
}

export const setSegs = StateEffect.define<Seg[]>();
export const lazyLoad = Annotation.define<boolean>();

function withPrefix(segs: Seg[]): GapTable {
  const prefix: number[] = [];
  let acc = 0;
  for (const s of segs) {
    prefix.push(acc);
    acc += s.lines;
  }
  return { segs, prefix };
}

function remap(table: GapTable, tr: Transaction): GapTable {
  const doc = tr.newDoc;
  const segs = table.segs.map((s): Seg => {
    const from = tr.changes.mapPos(s.from, -1);
    const to = tr.changes.mapPos(s.to, 1);
    let touched = false;
    tr.changes.iterChangedRanges((fa, ta) => {
      if (s.kind === "loaded" && fa <= s.to && ta >= s.from) touched = true;
    });
    const lines = s.kind === "gap" ? s.lines : doc.lineAt(to).number - doc.lineAt(from).number + 1;
    return { ...s, from, to, lines, dirty: s.dirty || touched };
  });
  return withPrefix(segs);
}

export function createGapTableField(initial: Seg[]): StateField<GapTable> {
  return StateField.define<GapTable>({
    create: () => withPrefix(initial),
    update(table, tr) {
      for (const e of tr.effects) if (e.is(setSegs)) return withPrefix(e.value);
      return tr.docChanged ? remap(table, tr) : table;
    },
  });
}

let tableField: StateField<GapTable>;
export const gapTable = (state: EditorState): GapTable => state.field(tableField);

function segIndexAtPos(table: GapTable, pos: number): number {
  let lo = 0;
  let hi = table.segs.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (table.segs[mid].from <= pos) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

/** 1-based file line number shown in the gutter for a 1-based document line. */
export function absLineOfDocLine(state: EditorState, docLine: number): number {
  // The gutter may format a line number from a document one step behind the state it is handed.
  if (docLine > state.doc.lines) return docLine;
  const table = gapTable(state);
  const line = state.doc.line(docLine);
  const i = segIndexAtPos(table, line.from);
  const seg = table.segs[i];
  if (seg.kind === "gap") return table.prefix[i] + 1;
  return table.prefix[i] + (docLine - state.doc.lineAt(seg.from).number) + 1;
}

/** Converts a document position to 0-based file line and UTF-16 column (the oracle's coordinates). */
export function docPosToAbs(state: EditorState, pos: number): { line: number; col: number } {
  const dl = state.doc.lineAt(pos);
  return { line: absLineOfDocLine(state, dl.number) - 1, col: pos - dl.from };
}

class GapWidget extends WidgetType {
  constructor(readonly lines: number) {
    super();
  }
  eq(o: GapWidget) {
    return o.lines === this.lines;
  }
  get estimatedHeight() {
    return this.lines * LINE_HEIGHT;
  }
  toDOM() {
    const el = document.createElement("div");
    el.className = "cm-lazy-gap";
    el.setAttribute("aria-hidden", "true");
    el.style.height = `${this.lines * LINE_HEIGHT}px`;
    el.textContent = `… ${this.lines} lines not loaded`;
    return el;
  }
  ignoreEvent() {
    return true;
  }
}

function placeholders(state: EditorState): DecorationSet {
  const ranges = gapTable(state)
    .segs.filter((s) => s.kind === "gap")
    .map((s) => Decoration.replace({ widget: new GapWidget(s.lines), block: true }).range(s.from, s.to));
  return Decoration.set(ranges, true);
}

const placeholderField = StateField.define<DecorationSet>({
  create: placeholders,
  update: (_v, tr) => placeholders(tr.state),
  provide: (f) => [EditorView.decorations.from(f), EditorView.atomicRanges.of((v) => v.state.field(f))],
});

function touchesPlaceholder(table: GapTable, tr: Transaction): boolean {
  let bad = false;
  tr.changes.iterChangedRanges((fa, ta) => {
    for (const s of table.segs) {
      if (s.kind !== "gap") continue;
      const ps = Math.max(0, s.from - 1);
      const pe = s.to + 1;
      const hit = ta > fa ? fa < pe && ta > ps : fa > ps && fa < pe;
      if (hit) bad = true;
    }
  });
  return bad;
}

export function gapLineExtension(vdoc: VirtualDoc, initial?: Seg[]): { extension: Extension; initialDoc: string } {
  const segs = initial ?? splitGap(0, vdoc.lineCount);
  tableField = createGapTableField(segs);
  const initialDoc = segs.map((s) => PLACEHOLDER).join("\n");
  const extension: Extension = [
    tableField,
    placeholderField,
    lineNumbers({ formatNumber: (n, state) => String(absLineOfDocLine(state, n)) }),
    EditorState.changeFilter.of((tr) => tr.annotation(lazyLoad) || !touchesPlaceholder(gapTable(tr.startState), tr)),
    EditorView.theme({ ".cm-line": { height: `${LINE_HEIGHT}px`, lineHeight: `${LINE_HEIGHT}px` } }),
  ];
  return { extension, initialDoc };
}

/** Initial gap pieces covering [start, start + total), each at most MAX_GAP_LINES, with doc offsets. */
export function splitGap(start: number, total: number): Seg[] {
  const segs: Seg[] = [];
  let pos = 0;
  for (let done = 0; done < total; done += MAX_GAP_LINES) {
    const lines = Math.min(MAX_GAP_LINES, total - done);
    segs.push({ kind: "gap", from: pos, to: pos + 1, lines, baseStart: start + done, dirty: false });
    pos += 2;
  }
  return segs;
}

function shift(segs: Seg[], delta: number): Seg[] {
  return segs.map((s) => ({ ...s, from: s.from + delta, to: s.to + delta }));
}

/** Loads the CHUNK_LINES-aligned chunk containing file line `abs` (0-based). Returns false if nothing to do or deferred. */
export function loadChunk(view: EditorView, vdoc: VirtualDoc, abs: number): boolean {
  if (view.composing) return false;
  const table = gapTable(view.state);
  const i = table.segs.findIndex((s, k) => s.kind === "gap" && abs >= table.prefix[k] && abs < table.prefix[k] + s.lines);
  if (i < 0) return false;
  const g = table.segs[i];
  const gs = table.prefix[i];
  const s = Math.max(gs, Math.floor(abs / CHUNK_LINES) * CHUNK_LINES);
  const e = Math.min(gs + g.lines, s + CHUNK_LINES);
  const loadedText = vdoc.fetch(g.baseStart + (s - gs), e - s).join("\n");
  const pieces: { text: string; seg: Omit<Seg, "from" | "to"> }[] = [];
  if (s > gs) pieces.push({ text: PLACEHOLDER, seg: { kind: "gap", lines: s - gs, baseStart: g.baseStart, dirty: false } });
  pieces.push({ text: loadedText, seg: { kind: "loaded", lines: e - s, baseStart: g.baseStart + (s - gs), dirty: false } });
  if (e < gs + g.lines) pieces.push({ text: PLACEHOLDER, seg: { kind: "gap", lines: gs + g.lines - e, baseStart: g.baseStart + (e - gs), dirty: false } });
  const insert = pieces.map((p) => p.text).join("\n");
  let pos = g.from;
  const fresh: Seg[] = pieces.map((p) => {
    const seg = { ...p.seg, from: pos, to: pos + p.text.length } as Seg;
    pos += p.text.length + 1;
    return seg;
  });
  const delta = insert.length - (g.to - g.from);
  const next = [...table.segs.slice(0, i), ...fresh, ...shift(table.segs.slice(i + 1), delta)];
  view.dispatch({
    changes: { from: g.from, to: g.to, insert },
    effects: setSegs.of(next),
    annotations: [Transaction.addToHistory.of(false), lazyLoad.of(true)],
  });
  return true;
}

/** Turns clean loaded segment `i` back into a placeholder, merging with adjacent gaps while under the cap. */
export function evictSegment(view: EditorView, i: number): boolean {
  if (view.composing) return false;
  const table = gapTable(view.state);
  const seg = table.segs[i];
  if (!seg || seg.kind !== "loaded" || seg.dirty) return false;
  let lo = i;
  let hi = i;
  let lines = seg.lines;
  const prev = table.segs[i - 1];
  if (prev?.kind === "gap" && prev.baseStart + prev.lines === seg.baseStart && lines + prev.lines <= MAX_GAP_LINES) {
    lo = i - 1;
    lines += prev.lines;
  }
  const nextSeg = table.segs[i + 1];
  if (nextSeg?.kind === "gap" && seg.baseStart + seg.lines === nextSeg.baseStart && lines + nextSeg.lines <= MAX_GAP_LINES) {
    hi = i + 1;
    lines += nextSeg.lines;
  }
  const from = table.segs[lo].from;
  const to = table.segs[hi].to;
  const merged: Seg = { kind: "gap", from, to: from + 1, lines, baseStart: table.segs[lo].baseStart, dirty: false };
  const delta = merged.to - to;
  const next = [...table.segs.slice(0, lo), merged, ...shift(table.segs.slice(hi + 1), delta)];
  view.dispatch({
    changes: { from, to, insert: PLACEHOLDER },
    effects: setSegs.of(next),
    annotations: [Transaction.addToHistory.of(false), lazyLoad.of(true)],
  });
  return true;
}

/** Current file as lines: loaded text from the document, gaps regenerated from the source. */
export function materialize(state: EditorState, vdoc: VirtualDoc): string[] {
  const out: string[] = [];
  const doc: Text = state.doc;
  for (const s of gapTable(state).segs) {
    const lines = s.kind === "gap" ? vdoc.fetch(s.baseStart, s.lines) : doc.sliceString(s.from, s.to).split("\n");
    for (const l of lines) out.push(l);
  }
  return out;
}

export function loadedChars(state: EditorState): number {
  return gapTable(state).segs.reduce((n, s) => (s.kind === "loaded" ? n + (s.to - s.from) : n), 0);
}

