// Bounded unary prefetch around the viewport (ADR-036). Never opens a stream: every fetch is one ReadFileRange.
export const MAX_IN_FLIGHT_PER_BUFFER = 2;
export const MAX_IN_FLIGHT_PER_TAB = 4;
/** Above this many windows of scroll per second, two windows are fetched ahead instead of one. */
export const FAST_WINDOWS_PER_SECOND = 1;
/** A viewport move over this many windows in one step is a jump, not a scroll. */
const JUMP_WINDOWS = 2;

/** In-flight counter shared by every buffer prefetcher of one tab. */
export class TabBudget {
  inFlight = 0;
}

export interface PrefetchDeps {
  /** Lines per window; windows sit on a fixed grid of this size. */
  windowLines: number;
  now(): number;
  /** True when lines [first, last] are already in the chunk cache. */
  isCached(first: number, last: number): boolean;
  /** Issues one unary window read; must honour `signal`. */
  fetchWindow(startLine: number, signal: AbortSignal): Promise<unknown>;
  /** Exact total, once known, so the scheduler stops at the end of the file. */
  lineCount?(): number | undefined;
}

/** Keeps the viewport window plus one ahead and one behind cached for a single buffer. */
export class BufferPrefetcher {
  private readonly inflight = new Map<number, AbortController>();
  private readonly failed = new Set<number>();
  private targets: number[] = [];
  private prev: { first: number; at: number } | null = null;
  private dir: -1 | 0 | 1 = 0;
  private disposed = false;

  constructor(private readonly deps: PrefetchDeps, private readonly tab: TabBudget = new TabBudget()) {}

  /** Call on every viewport change with the first visible line (1-based). */
  onViewport(firstVisible: number): void {
    const { windowLines: w } = this.deps;
    const at = this.deps.now();
    let fast = false;
    if (this.prev) {
      const delta = firstVisible - this.prev.first;
      const step = Math.sign(delta) as -1 | 0 | 1;
      const perSecond = Math.abs(delta) / w / (Math.max(at - this.prev.at, 1) / 1000);
      fast = perSecond > FAST_WINDOWS_PER_SECOND;
      if (Math.abs(delta) > JUMP_WINDOWS * w || (step !== 0 && this.dir !== 0 && step !== this.dir)) this.abortAll();
      if (step !== 0) this.dir = step;
    }
    this.prev = { first: firstVisible, at };
    this.failed.clear();

    const here = Math.floor((firstVisible - 1) / w);
    const ahead = this.dir >= 0 ? 1 : -1;
    this.targets = [here];
    for (let k = 1; k <= (fast ? 2 : 1); k++) this.targets.push(here + ahead * k);
    if (fast) this.abortOutside(this.targets); // a fast scroller has left the window behind it
    else this.targets.push(here - ahead);
    this.pump();
  }

  /** Aborts everything in flight and ignores later viewport calls. */
  dispose(): void {
    this.disposed = true;
    this.abortAll();
  }

  get inFlight(): number {
    return this.inflight.size;
  }

  private pump(): void {
    if (this.disposed) return;
    const { windowLines: w } = this.deps;
    const total = this.deps.lineCount?.();
    for (const idx of this.targets) {
      const start = idx * w + 1;
      if (idx < 0 || (total !== undefined && start > total) || this.inflight.has(idx) || this.failed.has(idx)) continue;
      if (this.deps.isCached(start, start + w - 1)) continue;
      if (this.inflight.size >= MAX_IN_FLIGHT_PER_BUFFER || this.tab.inFlight >= MAX_IN_FLIGHT_PER_TAB) return;
      this.launch(idx, start);
    }
  }

  private launch(idx: number, start: number): void {
    const ac = new AbortController();
    this.inflight.set(idx, ac);
    this.tab.inFlight++;
    this.deps.fetchWindow(start, ac.signal).then(
      () => undefined,
      () => { if (!ac.signal.aborted) this.failed.add(idx); },
    ).finally(() => {
      if (this.inflight.get(idx) !== ac) return; // abortAll already released this slot
      this.inflight.delete(idx);
      this.tab.inFlight--;
      this.pump();
    });
  }

  private abortOutside(keep: readonly number[]): void {
    for (const [idx, ac] of this.inflight) {
      if (keep.includes(idx)) continue;
      ac.abort();
      this.inflight.delete(idx);
      this.tab.inFlight--;
    }
  }

  private abortAll(): void {
    for (const ac of this.inflight.values()) ac.abort();
    this.tab.inFlight -= this.inflight.size;
    this.inflight.clear();
  }
}
