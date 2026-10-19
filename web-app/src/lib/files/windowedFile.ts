// Host side of a windowed (T1/T2) file: reads line windows through the FileTransport port and feeds the lazy
// document of the editor engine (VirtualDoc bridge) or the Accessible view. Holds no file text beyond the call.
import type { CodeEditorHandle } from "@/components/sessions/CodeEditor";
import type { LineRange } from "@/components/sessions/files/engine/EditorEngine";
import { Rejected, type FileRef, type FileTransportPort, type ReadResult } from "@/lib/files/transport/fileTransport";
import { lineCountOf } from "./windowedSource";

/** Lines per window; matches the engine's chunk size so a demanded gap is one request. */
export const WINDOW_LINES = 500;
const WINDOW_BYTES = 256 * 1024;
const MAX_CONCURRENT = 2;
const INDEX_POLL_MS = 1500;
const INDEX_POLL_MAX = 80;

export type WindowedEngine = Pick<
  CodeEditorHandle,
  "createBufferState" | "setBufferState" | "onNeedRange" | "provideRange" | "gotoLine" | "getCursor" | "highlightRange" | "getViewport"
>;

export interface WindowedState {
  lineCount: number;
  /** False while `lineCount` is the size-based estimate ("about N"). */
  exact: boolean;
  opened: boolean;
  /** The server is still building the line index. */
  indexing: boolean;
  loading: LineRange[];
  failed: LineRange[];
  /** Target line of a far jump in flight. */
  jump: number | null;
  notice: string | null;
  /** The router lost range reads: the viewer falls back to the truncated whole-file read. */
  demoted: boolean;
  openFailed: boolean;
  /** A load found the file changed under the reader (torn twice); Reload from disk re-anchors the same line. */
  stale: boolean;
}

export const EMPTY_WINDOWED_STATE: WindowedState = {
  lineCount: 1, exact: false, opened: false, indexing: false, loading: [], failed: [], jump: null, notice: null, demoted: false, openFailed: false, stale: false,
};

export type GoResult = { line: number; clamped: boolean; cancelled: boolean };

const sleep = (ms: number, signal: AbortSignal) => new Promise<void>((resolve, reject) => {
  const t = setTimeout(resolve, ms);
  signal.addEventListener("abort", () => { clearTimeout(t); reject(signal.reason); }, { once: true });
});

/** Settles with `p`, or rejects as soon as `signal` aborts (the shared index wait itself keeps running for other readers). */
const abortable = <T,>(p: Promise<T>, signal: AbortSignal) => new Promise<T>((resolve, reject) => {
  if (signal.aborted) return reject(signal.reason);
  signal.addEventListener("abort", () => reject(signal.reason), { once: true });
  p.then(resolve, reject);
});

const sameRange = (a: LineRange, b: LineRange) => a.startLine === b.startLine && a.endLine === b.endLine;
const aborted = (err: unknown) => err instanceof DOMException && err.name === "AbortError";

export class WindowedFile {
  private state: WindowedState = EMPTY_WINDOWED_STATE;
  private landing: Promise<unknown> = Promise.resolve();
  private readonly listeners = new Set<() => void>();
  private readonly life = new AbortController();
  private engine: WindowedEngine | null = null;
  private language = "text";
  private queue: LineRange[] = [];
  private running = 0;
  private jumpCtl: AbortController | null = null;
  private indexWait: Promise<void> | null = null;

  constructor(private readonly transport: FileTransportPort, private readonly ref: FileRef, private readonly size: number) {}

  subscribe = (fn: () => void): (() => void) => {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  };

  getSnapshot = (): WindowedState => this.state;

  private set(patch: Partial<WindowedState>): void {
    this.state = { ...this.state, ...patch };
    this.listeners.forEach((fn) => fn());
  }

  dispose(): void {
    this.life.abort();
    this.jumpCtl?.abort();
    this.queue = [];
    this.engine = null;
    this.listeners.clear();
  }

  // ---- reads ----

  private readOnce(anchorLine: number, dir: "around" | "fwd", wantLines: number, signal: AbortSignal = this.life.signal): Promise<ReadResult> {
    return this.transport.read(this.ref, { kind: "lines", anchorLine, dir, wantLines, maxBytes: WINDOW_BYTES }, signal);
  }

  /** Like `readOnce`, but waits out "line index not ready" (the request is queued behind the index). */
  private async read(anchorLine: number, dir: "around" | "fwd", wantLines: number, signal?: AbortSignal): Promise<ReadResult> {
    for (let attempt = 0; ; attempt++) {
      try {
        return await this.readOnce(anchorLine, dir, wantLines, signal);
      } catch (err) {
        if (!(err instanceof Rejected) || err.reason !== "index_pending" || attempt >= 2) throw err;
        await this.waitForIndex();
      }
    }
  }

  private waitForIndex(): Promise<void> {
    this.indexWait ??= (async () => {
      this.set({ indexing: true });
      try {
        for (let i = 0; i < INDEX_POLL_MAX; i++) {
          await sleep(INDEX_POLL_MS, this.life.signal);
          const stat = await this.transport.stat(this.ref, { wantAnalysis: true }, this.life.signal);
          if (stat.analysis !== "pending") return;
        }
      } finally {
        this.indexWait = null;
        if (!this.life.signal.aborted) this.set({ indexing: false });
      }
    })();
    return this.indexWait;
  }

  /** Lines [first, first + count) as plain strings, for the Accessible view; also settles the line count. */
  async pageLines(first: number, count: number, signal?: AbortSignal): Promise<string[]> {
    const r = await this.read(first, "fwd", count, signal);
    this.noteCount(r);
    const lines = r.text.split("\n");
    if (lines[lines.length - 1] === "") lines.pop();
    return lines.slice(0, count);
  }

  // ---- line count ----

  /** Records the count a result implies; true when the document must be rebuilt for a different count. */
  private noteCount(r: ReadResult): boolean {
    const next = lineCountOf(this.size, r);
    if (this.state.exact && next.count === this.state.lineCount) return false;
    if (!next.exact && this.state.exact) return false;
    const changed = next.count !== this.state.lineCount;
    this.set({ lineCount: next.count, exact: next.exact });
    return changed && this.engine !== null && this.state.opened;
  }

  // ---- engine bridge ----

  private async swapBuffer(): Promise<void> {
    const e = this.engine;
    if (!e) return;
    const state = await e.createBufferState({ doc: "", language: this.language, lazy: { lineCount: this.state.lineCount } });
    if (this.engine === e) e.setBufferState(state);
  }

  /** Hands the engine the text of a window; a window cut mid-line gives up its last line. */
  private provide(r: ReadResult): void {
    if (!this.engine || r.range.endLine < r.range.startLine) return;
    let { endLine } = r.range;
    let text = r.text;
    if (r.partialLastLine && endLine > r.range.startLine) {
      text = text.slice(0, text.lastIndexOf("\n") + 1);
      endLine -= 1;
    }
    this.engine.provideRange({ startLine: r.range.startLine, endLine }, text);
  }

  /** Mounts the lazy document on `engine` and paints the window around `anchorLine`; the cursor lands on that exact line. */
  async attach(engine: WindowedEngine, language: string, anchorLine: number): Promise<void> {
    this.engine = engine;
    this.language = language;
    engine.onNeedRange((range) => this.request(range));
    let first: ReadResult;
    let deferred: number | null = null;
    try {
      try {
        first = await this.readOnce(Math.max(1, anchorLine), "around", WINDOW_LINES);
      } catch (err) {
        // A far line needs the index; line 1 never does, so paint that and jump once the index is ready.
        if (!(err instanceof Rejected) || err.reason !== "index_pending") throw err;
        deferred = anchorLine;
        first = await this.readOnce(1, "fwd", WINDOW_LINES);
      }
    } catch (err) {
      return this.fail(err, true);
    }
    if (this.engine !== engine) return;
    const count = lineCountOf(this.size, first);
    this.set({ lineCount: count.count, exact: count.exact });
    await this.swapBuffer();
    this.set({ opened: true });
    this.provide(first);
    engine.gotoLine(deferred === null ? Math.min(Math.max(1, anchorLine), Math.max(first.range.endLine, 1)) : 1);
    if (deferred !== null) void this.goTo(deferred);
  }

  private fail(err: unknown, opening = false): void {
    if (aborted(err)) return;
    if (err instanceof Rejected && err.reason === "unsupported") this.set({ demoted: true });
    else if (err instanceof Rejected && err.reason === "stale") this.set({ stale: true });
    else if (opening) this.set({ openFailed: true });
  }

  // ---- demand loading ----

  private request(range: LineRange): void {
    const known = [...this.queue, ...this.state.loading].some((r) => sameRange(r, range));
    if (known) return;
    this.queue.push(range);
    this.pump();
  }

  retry(range: LineRange): void {
    this.set({ failed: this.state.failed.filter((r) => !sameRange(r, range)) });
    this.request(range);
  }

  private pump(): void {
    while (this.running < MAX_CONCURRENT && this.queue.length > 0) {
      const range = this.queue.pop() as LineRange; // newest demand first: the viewport has moved on from older ones
      this.running++;
      this.set({ loading: [...this.state.loading, range] });
      void this.fill(range).finally(() => {
        this.running--;
        this.set({ loading: this.state.loading.filter((r) => !sameRange(r, range)) });
        this.pump();
      });
    }
  }

  private async fill(range: LineRange): Promise<void> {
    try {
      let next = range.startLine;
      while (next <= range.endLine && !this.life.signal.aborted) {
        const r = await this.read(next, "fwd", range.endLine - next + 1);
        if (await this.settleCount(r)) return;
        this.provide(r);
        if (r.range.eof || r.range.endLine < next) return;
        next = r.range.endLine + 1;
      }
    } catch (err) {
      if (aborted(err)) return;
      this.fail(err);
      if (!this.state.demoted) this.set({ failed: [...this.state.failed, range] });
    }
  }

  /** Rebuilds the document for a newly learned count, keeping the viewport's top line; true when it did. */
  private async settleCount(r: ReadResult): Promise<boolean> {
    const e = this.engine;
    if (!e || !this.noteCount(r)) return false;
    const top = e.getViewport().startLine;
    await this.swapBuffer();
    const again = await this.read(top, "around", WINDOW_LINES);
    this.provide(again);
    e.gotoLine(top);
    return true;
  }

  // ---- far jumps ----

  /** Loads the window around `requested` first, then puts the cursor on that exact line (clamped to the end). */
  goTo(requested: number): Promise<GoResult> {
    const done = this.jump(requested);
    this.landing = done;
    return done;
  }

  /** Resolves once the latest far jump has landed (or been cancelled); a hit highlight waits for it. */
  settled(): Promise<unknown> {
    return this.landing;
  }

  private async jump(requested: number): Promise<GoResult> {
    const e = this.engine;
    let line = Math.max(1, Math.trunc(requested));
    let clamped = false;
    if (this.state.exact && line > this.state.lineCount) { line = this.state.lineCount; clamped = true; }
    this.jumpCtl?.abort();
    const ctl = (this.jumpCtl = new AbortController());
    this.life.signal.addEventListener("abort", () => ctl.abort(), { once: true });
    this.set({ jump: line, notice: null });
    try {
      const r = await abortable(this.read(line, "around", WINDOW_LINES, ctl.signal), ctl.signal);
      if (ctl.signal.aborted) return { line, clamped, cancelled: true };
      if (line > r.range.endLine && r.range.eof) { line = r.range.endLine; clamped = true; }
      const recount = this.noteCount(r);
      if (e && this.engine === e) {
        if (recount) await this.swapBuffer();
        this.provide(r);
        e.gotoLine(line);
      }
      if (clamped) this.set({ notice: `Line ${requested.toLocaleString("en-US")} is past the end; showing line ${line.toLocaleString("en-US")}` });
      return { line, clamped, cancelled: false };
    } catch (err) {
      if (!aborted(err)) this.fail(err);
      return { line, clamped, cancelled: true };
    } finally {
      if (this.jumpCtl === ctl) { this.jumpCtl = null; this.set({ jump: null }); }
    }
  }

  cancelJump(): void {
    this.jumpCtl?.abort();
  }
}
