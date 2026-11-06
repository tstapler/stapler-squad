import type { Transaction } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import type { Edit, LineRange } from "../EditorEngine";
import type { LazyBufferInfo, LazyController } from "./controller";
import {
  CHUNK_LINES, LINE_HEIGHT, absLineOfDocLine, gapTableOf, isLazyLoad, loadedChars, segAtFileLine,
} from "./gapTable";
import { evictFromLoaded, loadIntoGap } from "./loadEvict";
import { absoluteEdits, baseLayout, isDirtyRange, rebaseOverLoad, type EditTrack } from "./lazyPending";
import type { DemandSink, GapDemand } from "./scrollAnchor";

/** Resident chunk text above this many UTF-16 units is evicted back to placeholders (the phone budget). */
export const RESIDENT_BUDGET_CHARS = 8 * 1024 * 1024;
const RETRY_AFTER_MS = 1000;

const keyOf = (r: LineRange) => `${r.startLine}-${r.endLine}`;

/** Gap-line model (ADR-036 a2): host-facing ranges are 1-based inclusive lines of the last-acked file. */
export class GapLineController implements LazyController {
  readonly mode = "gap-line" as const;
  private track: EditTrack | null = null;
  private sink: DemandSink | null = null;
  private needCb: ((range: LineRange) => void) | null = null;
  private fetcher: ((range: LineRange) => Promise<string>) | null = null;
  private asked = new Map<string, number>();
  private revealTo: { line: number; col: number } | null = null;

  constructor(private readonly view: EditorView) {}

  attach(track: EditTrack | null, info: LazyBufferInfo | null): void {
    if (this.sink) this.sink.target = null;
    const sink = info?.sink ?? null;
    this.track = track;
    this.sink = sink;
    this.asked.clear();
    this.revealTo = null;
    if (sink) sink.target = (demand) => this.onDemand(demand);
  }

  onNeedRange(cb: (range: LineRange) => void): void {
    this.needCb = cb;
  }

  setFetcher(fetcher: ((range: LineRange) => Promise<string>) | null): void {
    this.fetcher = fetcher;
  }

  absorb(tr: Transaction): boolean {
    if (!this.track || !isLazyLoad(tr)) return false;
    rebaseOverLoad(this.track, tr.changes);
    return true;
  }

  edits(): Edit[] {
    return this.track ? absoluteEdits(this.track, this.view.state) : [];
  }

  private request(range: LineRange): void {
    const key = keyOf(range);
    const now = Date.now();
    if (now - (this.asked.get(key) ?? -Infinity) < RETRY_AFTER_MS) return;
    this.asked.set(key, now);
    if (this.needCb) this.needCb(range);
    else void this.fetcher?.(range).then((text) => this.provideRange(range, text), () => this.asked.delete(key));
  }

  private onDemand(demand: GapDemand[]): void {
    if (!this.track) return;
    const layout = baseLayout(this.track, this.view.state);
    for (const { index, offset } of demand) {
      const { start, count } = layout[index];
      const first = Math.max(start, Math.floor((start + offset) / CHUNK_LINES) * CHUNK_LINES);
      this.request({ startLine: first + 1, endLine: Math.min(start + count, first + CHUNK_LINES) });
    }
  }

  provideRange(range: LineRange, text: string): void {
    const track = this.track;
    if (!track) throw new Error("CmEngine: provideRange needs a lazy source");
    this.asked.delete(keyOf(range));
    const lines = text.split("\n");
    const wanted = range.endLine - range.startLine + 1;
    if (lines.length === wanted + 1 && lines[wanted] === "") lines.pop();
    const from = range.startLine - 1;
    const to = Math.min(from + lines.length, range.endLine);
    const layout = baseLayout(track, this.view.state);
    const { segs } = gapTableOf(this.view.state);
    // Back to front, so loading one gap never shifts the index of a gap still to be loaded.
    for (let i = segs.length - 1; i >= 0; i--) {
      const { start, count } = layout[i];
      if (segs[i].kind !== "gap" || start >= to || start + count <= from) continue;
      const lo = Math.max(start, from);
      const hi = Math.min(start + count, to);
      loadIntoGap(this.view, i, lo - start, lines.slice(lo - from, hi - from).join("\n"), hi - lo);
    }
    this.enforceBudget();
    this.settleReveal();
  }

  evictRange(range: LineRange): void {
    const track = this.track;
    if (!track) throw new Error("CmEngine: evictRange needs a lazy source");
    const layout = baseLayout(track, this.view.state);
    const { segs } = gapTableOf(this.view.state);
    const from = range.startLine - 1;
    for (let i = segs.length - 1; i >= 0; i--) {
      const { start, count } = layout[i];
      const s = segs[i];
      if (s.kind !== "loaded" || start >= range.endLine || start + count <= from) continue;
      if (isDirtyRange(track, s.from, s.to)) continue;
      const lo = Math.max(start, from);
      evictFromLoaded(this.view, i, lo - start, Math.min(start + count, range.endLine) - lo);
    }
  }

  /** Drops the clean loaded segment farthest from the viewport until resident text fits the budget. */
  private enforceBudget(): void {
    const track = this.track;
    while (track && loadedChars(this.view.state) > RESIDENT_BUDGET_CHARS) {
      const { from, to } = this.view.viewport;
      const mid = (from + to) / 2;
      const { segs } = gapTableOf(this.view.state);
      let victim = -1;
      let far = -1;
      segs.forEach((s, i) => {
        const away = Math.abs((s.from + s.to) / 2 - mid);
        const visible = s.from <= to && s.to >= from;
        if (s.kind === "loaded" && !visible && away > far && !isDirtyRange(track, s.from, s.to)) [victim, far] = [i, away];
      });
      if (victim < 0 || !evictFromLoaded(this.view, victim, 0, segs[victim].lines)) return;
    }
  }

  reveal(line: number, col: number): void {
    const table = gapTableOf(this.view.state);
    const hit = segAtFileLine(table, Math.max(0, Math.floor(line) - 1));
    const last = table.prefix[table.segs.length - 1] + table.segs[table.segs.length - 1].lines;
    if (!hit) return this.reveal(Math.min(Math.max(1, line), last), col);
    this.revealTo = { line: Math.floor(line), col };
    if (table.segs[hit.index].kind === "loaded") return this.settleReveal();
    const seg = table.segs[hit.index];
    const top = this.view.lineBlockAt(seg.from).top;
    this.view.scrollDOM.scrollTop = top + hit.offset * LINE_HEIGHT;
    this.onDemand([{ index: hit.index, offset: hit.offset }]);
  }

  /** Completes a pending reveal once its line is loaded: cursor there and centered. */
  private settleReveal(): void {
    const want = this.revealTo;
    if (!want) return;
    const table = gapTableOf(this.view.state);
    const hit = segAtFileLine(table, want.line - 1);
    if (!hit || table.segs[hit.index].kind !== "loaded") return;
    this.revealTo = null;
    const doc = this.view.state.doc;
    const target = doc.line(doc.lineAt(table.segs[hit.index].from).number + hit.offset);
    const pos = Math.min(target.from + Math.max(0, want.col), target.to);
    this.view.dispatch({ selection: { anchor: pos }, effects: EditorView.scrollIntoView(pos, { y: "center" }) });
  }

  viewport(): LineRange {
    const { from, to } = this.view.viewport;
    const doc = this.view.state.doc;
    return {
      startLine: absLineOfDocLine(this.view.state, doc.lineAt(from).number),
      endLine: absLineOfDocLine(this.view.state, doc.lineAt(to).number),
    };
  }

  cursorLine(): number {
    const head = this.view.state.selection.main.head;
    return absLineOfDocLine(this.view.state, this.view.state.doc.lineAt(head).number);
  }

  destroy(): void {
    this.attach(null, null);
  }
}
