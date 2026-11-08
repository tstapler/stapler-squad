/** Resyncs allowed in flight across every terminal in the tab. */
export const MAX_RESYNCS_IN_FLIGHT = 2;
/** Spacing between watchdog-forced disconnects, so a stall cannot drop every stream at once. */
export const DISCONNECT_SPACING_MS = 500;
/** Largest random delay added before a queued resync runs. */
export const RESYNC_JITTER_MS = 200;

export type ResyncPriority = "foreground" | "background";

interface Waiter {
  run: () => void;
  priority: ResyncPriority;
  cancelled: boolean;
}

export interface ResyncGateOptions {
  maxInFlight?: number;
  random?: () => number;
  setTimer?: (fn: () => void, ms: number) => unknown;
}

/**
 * Tab-wide limiter for the work a focus event fans out to every mounted terminal. Each resync
 * costs the server a full-pane capture, and ten at once queue behind one another past the stall
 * watchdog, which then drops every stream and re-attaches them all: a thundering herd. The gate
 * runs at most `maxInFlight` resyncs, foreground before background, queued ones after a jitter,
 * and spaces forced disconnects apart.
 */
export class ResyncGate {
  private inFlight = 0;
  private queue: Waiter[] = [];
  private nextDisconnectAt = 0;
  private readonly max: number;
  private readonly random: () => number;
  private readonly setTimer: (fn: () => void, ms: number) => unknown;
  private readonly now: () => number;

  constructor(opts: ResyncGateOptions & { now?: () => number } = {}) {
    this.max = opts.maxInFlight ?? MAX_RESYNCS_IN_FLIGHT;
    this.random = opts.random ?? Math.random;
    this.setTimer = opts.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
    this.now = opts.now ?? Date.now;
  }

  /** Forgets every slot and queued request. For tests, which share the tab-wide gate. */
  reset(): void {
    this.inFlight = 0;
    this.queue = [];
    this.nextDisconnectAt = 0;
  }

  /** Resyncs waiting for a slot. */
  depth(): number {
    return this.queue.length;
  }

  /** Resyncs currently holding a slot. */
  active(): number {
    return this.inFlight;
  }

  /**
   * Runs `run` when a slot is free; the caller owns the slot until it calls release(). The
   * returned cancel drops a still-queued request (a no-op once it has run).
   */
  acquire(priority: ResyncPriority, run: () => void): { cancel: () => void } {
    const waiter: Waiter = { run, priority, cancelled: false };
    if (this.inFlight < this.max && this.queue.length === 0) {
      this.inFlight++;
      run();
    } else {
      this.queue.push(waiter);
      this.queue.sort((a, b) => Number(b.priority === "foreground") - Number(a.priority === "foreground"));
    }
    return {
      cancel: () => {
        waiter.cancelled = true;
        this.queue = this.queue.filter((w) => w !== waiter);
      },
    };
  }

  /** Frees a slot and starts the next queued resync after a jitter. */
  release(): void {
    if (this.inFlight > 0) this.inFlight--;
    this.pump();
  }

  private pump(): void {
    while (this.inFlight < this.max && this.queue.length > 0) {
      const next = this.queue.shift();
      if (!next || next.cancelled) continue;
      this.inFlight++; // reserved now so a concurrent acquire cannot jump the queue
      this.setTimer(() => {
        if (next.cancelled) {
          this.release();
          return;
        }
        next.run();
      }, Math.floor(this.random() * RESYNC_JITTER_MS));
    }
  }

  /** Runs `fn` no sooner than DISCONNECT_SPACING_MS after the previous forced disconnect. */
  scheduleDisconnect(fn: () => void): void {
    const t = this.now();
    const at = Math.max(t, this.nextDisconnectAt);
    this.nextDisconnectAt = at + DISCONNECT_SPACING_MS;
    const wait = at - t;
    if (wait <= 0) fn();
    else this.setTimer(fn, wait);
  }
}

/** The tab-wide gate every terminal uses. */
export const resyncGate = new ResyncGate();
