// Edit bookkeeping for a lazy buffer. The pending/inflight change sets address `base`, the document at the
// last ack. Loads and evictions rewrite placeholders in the live document, so they are mapped into `base`
// and over both change sets; edits then leave in absolute file coordinates by adding the hidden gap lines.
import type { ChangeSet, EditorState, Text } from "@codemirror/state";
import type { Edit, EditPos } from "../EditorEngine";
import { gapTableOf, type Seg } from "./gapTable";

/** The slice of a buffer this module reads and rewrites; CmBuffer satisfies it structurally. */
export interface EditTrack {
  base: Text;
  pending: ChangeSet | null;
  inflight: ChangeSet | null;
}

export function unacked(t: EditTrack): ChangeSet | null {
  return t.inflight && t.pending ? t.inflight.compose(t.pending) : (t.inflight ?? t.pending);
}

/** `load` was applied to the live document (base plus all unacked edits); carry it into base and the edit sets. */
export function rebaseOverLoad(t: EditTrack, load: ChangeSet): void {
  const mid = t.inflight ? t.inflight.apply(t.base) : t.base;
  const loadMid = t.pending ? load.map(t.pending.invert(mid)) : load;
  const loadBase = t.inflight ? loadMid.map(t.inflight.invert(t.base)) : loadMid;
  const { inflight, pending } = t;
  t.base = loadBase.apply(t.base);
  t.inflight = inflight ? inflight.map(loadBase) : null;
  t.pending = pending ? pending.map(loadMid) : null;
}

/** Lines hidden behind placeholders that sit before live-document offset `pos`, minus the placeholder line itself. */
function hiddenBefore(segs: Seg[], pos: number): number {
  let hidden = 0;
  for (const s of segs) if (s.kind === "gap" && s.from < pos) hidden += s.lines - 1;
  return hidden;
}

/** Unacked edits as absolute (whole-file) line and UTF-16 column in the coordinates of the last-acked file. */
export function absoluteEdits(t: EditTrack, state: EditorState): Edit[] {
  const changes = unacked(t);
  if (!changes) return [];
  const { segs } = gapTableOf(state);
  const posAt = (offset: number): EditPos => {
    const line = t.base.lineAt(offset);
    const live = changes.mapPos(offset, -1);
    return { line: line.number - 1 + hiddenBefore(segs, live), colUtf16: offset - line.from };
  };
  const edits: Edit[] = [];
  changes.iterChanges((fromA, toA, _fromB, _toB, inserted) => {
    edits.push({ start: posAt(fromA), end: posAt(toA), insert: inserted.sliceString(0, undefined, "\n") });
  });
  return edits;
}

/** Whether any unacked edit touches the live-document range [from, to]; such a range must not be evicted. */
export function isDirtyRange(t: EditTrack, from: number, to: number): boolean {
  const changes = unacked(t);
  let dirty = false;
  changes?.iterChangedRanges((_a, _b, fromB, toB) => {
    if (fromB <= to && toB >= from) dirty = true;
  });
  return dirty;
}

/** Start line (0-based) and line count of every segment in the last-acked file, derived from `base`. */
export function baseLayout(t: EditTrack, state: EditorState): { start: number; count: number }[] {
  const inv = unacked(t)?.invert(t.base);
  let start = 0;
  return gapTableOf(state).segs.map((s) => {
    let count = s.lines;
    if (s.kind === "loaded") {
      const from = inv ? inv.mapPos(s.from, -1) : s.from;
      const to = inv ? inv.mapPos(s.to, 1) : s.to;
      count = t.base.lineAt(to).number - t.base.lineAt(from).number + 1;
    }
    const entry = { start, count };
    start += count;
    return entry;
  });
}
