/** Timing for the "Typing... (N characters waiting)" affordance (Story 1.2.5). DOM-free, injected clock. */
import type { MobileInputClock } from "./types";

export class DeferredIndicator {
  private timer: number | null = null;
  private shown = false;
  private latest: number | null = null;
  private reported: number | null = null;

  constructor(
    private readonly clock: MobileInputClock,
    private readonly delayMs: number,
    private readonly onChange: (waiting: number | null) => void,
  ) {}

  /** `waiting` is the held-back grapheme count, or null once nothing is deferred. */
  update(waiting: number | null): void {
    this.latest = waiting;
    if (waiting === null) {
      this.cancelTimer();
      if (this.shown) {
        this.shown = false;
        this.reported = null;
        this.onChange(null);
      }
      return;
    }
    if (this.shown) {
      if (waiting !== this.reported) {
        this.reported = waiting;
        this.onChange(waiting);
      }
      return;
    }
    this.timer ??= this.clock.setTimeout(() => {
      this.timer = null;
      if (this.latest === null) return;
      this.shown = true;
      this.reported = this.latest;
      this.onChange(this.latest);
    }, this.delayMs);
  }

  dispose(): void {
    this.update(null);
  }

  private cancelTimer(): void {
    if (this.timer === null) return;
    this.clock.clearTimeout(this.timer);
    this.timer = null;
  }
}
