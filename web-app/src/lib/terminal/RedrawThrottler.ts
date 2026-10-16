/**
 * RedrawThrottler - Coalesces rapid full-screen redraws to max 30 FPS.
 *
 * Claude performs complete screen redraws at 12-25 FPS, causing visible flicker.
 * This throttler holds rapid redraws and flushes the latest one at a capped rate.
 *
 * Split out of TerminalStreamManager.ts (which still owns the one instance per
 * manager) — it has no dependency on TerminalStreamManager's internals, only
 * the onFlush callback passed to its constructor.
 */
export class RedrawThrottler {
  private pendingRedraw: string | null = null;
  // Cursor-up count (the \d+ in \x1b[\d+A) of the currently pending redraw —
  // identifies which screen region it repaints, so a same-region redraw can
  // be coalesced onto it while a different-region one cannot (see below).
  private pendingRedrawUpCount: number | null = null;
  private throttleTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly throttleMs = 33; // ~30fps; coalesces burst full-screen redraws
  private onFlush: (data: string) => void;

  constructor(onFlush: (data: string) => void) {
    this.onFlush = onFlush;
  }

  process(chunk: string): string | null {
    // Detect genuine full-screen redraws: cursor-up followed immediately by an erase sequence.
    // NOTE: \x1b[H (cursor-home) is intentionally excluded - it is also emitted during
    // incremental Ink-style renders and must not be classified as a full redraw.
    // Scoping the check to the first 32 bytes avoids false positives in large output chunks.
    const match = /^\x1b\[(\d+)A(?:\x1b\[2K|\x1b\[J)/.exec(chunk.substring(0, 32));

    if (!match) {
      this.flushPending();
      return chunk;
    }

    // \x1b[NA\x1b[2K (cursor-up N, erase ONE line) is also exactly how a multi-line
    // Ink-style TUI redraws a SINGLE targeted line (e.g. a spinner or token counter),
    // not just how it repaints the whole viewport — the leading bytes alone can't tell
    // the two apart. Coalescing is only safe when the new candidate targets the SAME
    // cursor-up region as the one already pending (repeated repaints of one region,
    // which is the flicker this throttler exists to suppress). A different region
    // arriving mid-window is flushed immediately instead of silently overwriting the
    // pending one — otherwise the pending region's update is dropped forever, since
    // nothing else resends it.
    const upCount = Number(match[1]);
    if (this.pendingRedraw !== null && upCount !== this.pendingRedrawUpCount) {
      this.flushPending();
    }

    this.pendingRedraw = chunk;
    this.pendingRedrawUpCount = upCount;

    if (!this.throttleTimer) {
      this.throttleTimer = setTimeout(() => {
        this.flushPending();
      }, this.throttleMs);
    }

    return null; // Don't output yet
  }

  private flushPending() {
    if (this.pendingRedraw) {
      this.onFlush(this.pendingRedraw);
      this.pendingRedraw = null;
      this.pendingRedrawUpCount = null;
    }
    if (this.throttleTimer) {
      clearTimeout(this.throttleTimer);
      this.throttleTimer = null;
    }
  }

  cleanup() {
    this.flushPending();
  }
}
