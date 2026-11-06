// Gap table of the cm6LazyDoc gap-line model (ADR-036 decision 19): the document holds loaded lines plus one
// placeholder line per unloaded gap; this StateField maps document positions to file lines.
import { Facet, StateEffect, StateField, type EditorState, type Transaction } from "@codemirror/state";

export const LINE_HEIGHT = 20;
export const CHUNK_LINES = 500;
/** One placeholder never spans more than this many lines: a 250k-line (5M px) block defeated CM6's height scaler. */
export const MAX_GAP_LINES = 5000;
/** Not a "special char": basicSetup's highlightSpecialChars would otherwise wrap it in a plugin decoration that clashes with the block widget. */
export const PLACEHOLDER = " ";

export interface Seg {
  kind: "loaded" | "gap";
  from: number;
  to: number;
  /** Lines this segment stands for in the current file (gap: unloaded lines; loaded: lines in the document). */
  lines: number;
}

export interface GapTable {
  segs: Seg[];
  /** prefix[i] = file lines before segs[i] in the current (edited) file. */
  prefix: number[];
}

export const setSegs = StateEffect.define<Seg[]>();

/** Marks a load/evict transaction: guards let it through and the buffer treats it as non-user. */
export const lazyLoad = StateEffect.define<null>();
export const isLazyLoad = (tr: Transaction): boolean => tr.effects.some((e) => e.is(lazyLoad));

/** Segments the field starts from; a buffer supplies it once, when its state is created. */
export const initialSegs = Facet.define<Seg[], Seg[]>({ combine: (v) => v[0] ?? [] });

export function withPrefix(segs: Seg[]): GapTable {
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
  return withPrefix(
    table.segs.map((s): Seg => {
      const from = tr.changes.mapPos(s.from, -1);
      const to = tr.changes.mapPos(s.to, 1);
      const lines = s.kind === "gap" ? s.lines : doc.lineAt(to).number - doc.lineAt(from).number + 1;
      return { ...s, from, to, lines };
    }),
  );
}

export const gapTableField = StateField.define<GapTable>({
  create: (state) => withPrefix(state.facet(initialSegs)),
  update(table, tr) {
    for (const e of tr.effects) if (e.is(setSegs)) return withPrefix(e.value);
    return tr.docChanged ? remap(table, tr) : table;
  },
});

export const gapTableOf = (state: EditorState): GapTable => state.field(gapTableField);
export const isLazyState = (state: EditorState): boolean => state.field(gapTableField, false) !== undefined;

/** Initial gap pieces covering `total` lines, each at most MAX_GAP_LINES, one placeholder line apiece. */
export function splitGap(total: number): Seg[] {
  const segs: Seg[] = [];
  let pos = 0;
  for (let done = 0; done < total; done += MAX_GAP_LINES) {
    segs.push({ kind: "gap", from: pos, to: pos + 1, lines: Math.min(MAX_GAP_LINES, total - done) });
    pos += 2;
  }
  return segs;
}

export const initialDocFor = (segs: Seg[]): string => segs.map(() => PLACEHOLDER).join("\n");

export function segIndexAtPos(table: GapTable, pos: number): number {
  let lo = 0;
  let hi = table.segs.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (table.segs[mid].from <= pos) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

/** 1-based file line shown in the gutter for a 1-based document line. */
export function absLineOfDocLine(state: EditorState, docLine: number): number {
  // The gutter may format a line number from a document one step behind the state it is handed.
  if (docLine > state.doc.lines) return docLine;
  const table = gapTableOf(state);
  const i = segIndexAtPos(table, state.doc.line(docLine).from);
  const seg = table.segs[i];
  if (seg.kind === "gap") return table.prefix[i] + 1;
  return table.prefix[i] + (docLine - state.doc.lineAt(seg.from).number) + 1;
}

export function loadedChars(state: EditorState): number {
  return gapTableOf(state).segs.reduce((n, s) => (s.kind === "loaded" ? n + (s.to - s.from) : n), 0);
}

/** Index of the segment holding 0-based current-file line `abs`, and the line's offset inside it. */
export function segAtFileLine(table: GapTable, abs: number): { index: number; offset: number } | null {
  for (let i = 0; i < table.segs.length; i++) {
    const start = table.prefix[i];
    if (abs >= start && abs < start + table.segs[i].lines) return { index: i, offset: abs - start };
  }
  return null;
}
