// Window-swap model (ADR-036 a1, Task 1.6.4d): one window of the file per EditorState; reaching a window edge
// swaps in a new state centred on the viewport. Not the default: selected through CmEngine's `lazyMode` only
// if the owner phone gate records gap-line (a2) FAIL. A window with unsaved edits is pinned, never swapped.
import { EditorState, Facet, type Extension, type Transaction } from "@codemirror/state";
import { EditorView, lineNumbers } from "@codemirror/view";
import type { Edit, LineRange } from "../EditorEngine";
import { changesToEdits } from "../pendingEdits";
import type { LazyBufferInfo, LazyController } from "./controller";
import { LINE_HEIGHT } from "./gapTable";
import { unacked, type EditTrack } from "./lazyPending";

export const WINDOW_LINES = 2000;
export const EDGE_LINES = 200;
const EDGE_CHECK_MS = 50;

/** 0-based file line of the window's first line. */
const windowStart = Facet.define<number, number>({ combine: (v) => v[0] ?? 0 });

export const windowExtension = (start: number): Extension => [
  windowStart.of(start),
  lineNumbers({ formatNumber: (n, state) => String(state.facet(windowStart) + n) }),
  EditorView.theme({ ".cm-line": { height: `${LINE_HEIGHT}px`, lineHeight: `${LINE_HEIGHT}px` } }),
];

export class WindowSwapController implements LazyController {
  readonly mode = "window-swap" as const;
  private track: EditTrack | null = null;
  private info: LazyBufferInfo | null = null;
  private needCb: ((range: LineRange) => void) | null = null;
  private fetcher: ((range: LineRange) => Promise<string>) | null = null;
  private asked: string | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private revealTo: { line: number; col: number } | null = null;
  private readonly onScroll = () => {
    if (this.timer === null) this.timer = setTimeout(() => ((this.timer = null), this.checkEdges()), EDGE_CHECK_MS);
  };

  constructor(private readonly view: EditorView) {
    view.scrollDOM.addEventListener("scroll", this.onScroll, { passive: true });
  }

  private get start(): number {
    return this.view.state.facet(windowStart);
  }

  attach(track: EditTrack | null, info: LazyBufferInfo | null): void {
    this.track = track;
    this.info = info;
    this.asked = null;
    this.revealTo = null;
    if (info) setTimeout(() => this.loaded() || this.requestAround(1), 0);
  }

  private loaded = () => this.view.state.doc.length > 0 || this.start > 0;

  onNeedRange(cb: (range: LineRange) => void): void {
    this.needCb = cb;
  }

  setFetcher(fetcher: ((range: LineRange) => Promise<string>) | null): void {
    this.fetcher = fetcher;
  }

  absorb(_tr: Transaction): boolean {
    return false;
  }

  private isDirty(): boolean {
    return this.track !== null && unacked(this.track) !== null;
  }

  /** 1-based file line at the top of the viewport. */
  private topLine(): number {
    const block = this.view.lineBlockAtHeight(this.view.scrollDOM.getBoundingClientRect().top - this.view.documentTop);
    return this.start + this.view.state.doc.lineAt(block.from).number;
  }

  private requestAround(line: number): void {
    const total = this.info?.lineCount ?? 0;
    const first = Math.max(0, Math.min(Math.max(0, total - WINDOW_LINES), line - 1 - WINDOW_LINES / 2));
    const range = { startLine: first + 1, endLine: Math.min(total, first + WINDOW_LINES) };
    const key = `${range.startLine}-${range.endLine}`;
    if (this.asked === key) return;
    this.asked = key;
    if (this.needCb) this.needCb(range);
    else void this.fetcher?.(range).then((text) => this.provideRange(range, text), () => (this.asked = null));
  }

  private checkEdges(): void {
    if (!this.info || this.isDirty() || this.view.scrollDOM.clientHeight === 0) return;
    const top = this.topLine();
    const rel = top - this.start;
    const total = this.info.lineCount;
    if ((rel < EDGE_LINES && this.start > 0) || (rel > WINDOW_LINES - EDGE_LINES && this.start + WINDOW_LINES < total)) {
      this.requestAround(top);
    }
  }

  provideRange(range: LineRange, text: string): void {
    const { track, info } = this;
    if (!track || !info?.makeState) throw new Error("CmEngine: provideRange needs a lazy source");
    this.asked = null;
    if (this.isDirty()) return; // pinned: history cannot follow the document to a new state
    const anchor = this.loaded() ? this.topLine() : range.startLine;
    const state = info.makeState(text, range.startLine - 1);
    this.view.setState(state);
    track.base = state.doc;
    track.pending = null;
    track.inflight = null;
    const line = this.revealTo?.line ?? anchor;
    const rel = Math.max(0, line - range.startLine);
    // setState resets CodeMirror's scroll anchor on its next measure, so the offset is written after that.
    this.view.requestMeasure({ read: () => null, write: () => {
      const scroller = this.view.scrollDOM;
      scroller.scrollTop = rel * LINE_HEIGHT;
      requestAnimationFrame(() => (scroller.scrollTop = rel * LINE_HEIGHT));
    } });
    this.settleReveal();
  }

  evictRange(): void {
    // Nothing to evict: a window is replaced as a whole.
  }

  edits(): Edit[] {
    const changes = this.track ? unacked(this.track) : null;
    if (!changes || !this.track) return [];
    const start = this.start;
    return changesToEdits(changes, this.track.base).map((e) => ({
      ...e, start: { ...e.start, line: e.start.line + start }, end: { ...e.end, line: e.end.line + start },
    }));
  }

  reveal(line: number, col: number): void {
    this.revealTo = { line: Math.floor(line), col };
    if (!this.settleReveal()) this.requestAround(Math.floor(line));
  }

  private settleReveal(): boolean {
    const want = this.revealTo;
    const rel = want ? want.line - this.start : 0;
    const doc = this.view.state.doc;
    if (!want || rel < 1 || rel > doc.lines) return false;
    this.revealTo = null;
    const target = doc.line(rel);
    const pos = Math.min(target.from + Math.max(0, want.col), target.to);
    this.view.dispatch({ selection: { anchor: pos }, effects: EditorView.scrollIntoView(pos, { y: "center" }) });
    return true;
  }

  viewport(): LineRange {
    const { from, to } = this.view.viewport;
    const doc = this.view.state.doc;
    return { startLine: this.start + doc.lineAt(from).number, endLine: this.start + doc.lineAt(to).number };
  }

  cursorLine(): number {
    return this.start + this.view.state.doc.lineAt(this.view.state.selection.main.head).number;
  }

  destroy(): void {
    this.view.scrollDOM.removeEventListener("scroll", this.onScroll);
    if (this.timer !== null) clearTimeout(this.timer);
  }
}
