// Load and evict as non-history, non-user transactions: undo depth and dirty state never see them.
import { Transaction } from "@codemirror/state";
import type { EditorView } from "@codemirror/view";
import { MAX_GAP_LINES, PLACEHOLDER, gapTableOf, lazyLoad, setSegs, type Seg } from "./gapTable";

const shift = (segs: Seg[], delta: number): Seg[] => segs.map((s) => ({ ...s, from: s.from + delta, to: s.to + delta }));

function dispatchLoad(view: EditorView, change: { from: number; to: number; insert: string }, segs: Seg[]): void {
  view.dispatch({
    changes: change,
    effects: [setSegs.of(segs), lazyLoad.of(null)],
    annotations: Transaction.addToHistory.of(false),
  });
}

/** Replaces `count` lines of gap `index`, starting `offset` lines into it, with `text` (exactly `count` lines). */
export function loadIntoGap(view: EditorView, index: number, offset: number, text: string, count: number): boolean {
  if (view.composing) return false;
  const table = gapTableOf(view.state);
  const gap = table.segs[index];
  if (gap?.kind !== "gap" || count < 1 || offset + count > gap.lines) return false;
  const pieces: { text: string; kind: Seg["kind"]; lines: number }[] = [];
  if (offset > 0) pieces.push({ text: PLACEHOLDER, kind: "gap", lines: offset });
  pieces.push({ text, kind: "loaded", lines: count });
  const rest = gap.lines - offset - count;
  if (rest > 0) pieces.push({ text: PLACEHOLDER, kind: "gap", lines: rest });
  let pos = gap.from;
  const fresh = pieces.map((p): Seg => {
    const seg: Seg = { kind: p.kind, from: pos, to: pos + p.text.length, lines: p.lines };
    pos += p.text.length + 1;
    return seg;
  });
  const insert = pieces.map((p) => p.text).join("\n");
  const delta = insert.length - (gap.to - gap.from);
  dispatchLoad(view, { from: gap.from, to: gap.to, insert }, [
    ...table.segs.slice(0, index), ...fresh, ...shift(table.segs.slice(index + 1), delta),
  ]);
  return true;
}

/**
 * Turns `count` lines of loaded segment `index`, from `offset`, back into a placeholder, merging with a
 * neighboring gap while it stays under the cap. The caller has already checked the lines are clean.
 */
export function evictFromLoaded(view: EditorView, index: number, offset: number, count: number): boolean {
  if (view.composing) return false;
  const { segs } = gapTableOf(view.state);
  const seg = segs[index];
  if (seg?.kind !== "loaded" || count < 1 || offset + count > seg.lines) return false;
  const doc = view.state.doc;
  const first = doc.lineAt(seg.from).number;
  const start = doc.line(first + offset).from;
  const end = doc.line(first + offset + count - 1).to;
  const keepLeft = offset;
  const keepRight = seg.lines - offset - count;
  let lines = count;
  let lo = index;
  let hi = index;
  const prev = segs[index - 1];
  if (keepLeft === 0 && prev?.kind === "gap" && lines + prev.lines <= MAX_GAP_LINES) {
    lo = index - 1;
    lines += prev.lines;
  }
  const next = segs[index + 1];
  if (keepRight === 0 && next?.kind === "gap" && lines + next.lines <= MAX_GAP_LINES) {
    hi = index + 1;
    lines += next.lines;
  }
  const changeFrom = lo < index ? segs[lo].from : start;
  const changeTo = hi > index ? segs[hi].to : end;
  const delta = PLACEHOLDER.length - (changeTo - changeFrom);
  const fresh: Seg[] = [];
  if (keepLeft > 0) fresh.push({ kind: "loaded", from: seg.from, to: start - 1, lines: keepLeft });
  fresh.push({ kind: "gap", from: changeFrom, to: changeFrom + PLACEHOLDER.length, lines });
  if (keepRight > 0) fresh.push({ kind: "loaded", from: end + 1 + delta, to: seg.to + delta, lines: keepRight });
  dispatchLoad(view, { from: changeFrom, to: changeTo, insert: PLACEHOLDER }, [
    ...segs.slice(0, lo), ...fresh, ...shift(segs.slice(hi + 1), delta),
  ]);
  return true;
}
