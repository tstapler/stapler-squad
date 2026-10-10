/**
 * Pure, DOM-free touch-scroll kinematics: fractional-line accumulation,
 * fling momentum, and per-frame output clamping. Time is injected by callers.
 */

/** Signed whole-line count; positive = toward newer output (finger moved up). */
export type LineDelta = number & { readonly __brand: "LineDelta" };

/** Pixels of finger travel before a touch becomes a scroll; also the tap tolerance. */
export const SLOP_PX = 15;

/** Canonical momentum constants (shared with design/ux.md; tune on device). */
export const MOMENTUM_CONSTANTS = {
  windowMs: 100,
  minFlingPxPerMs: 0.3,
  decayPerFrame: 0.95,
  frameMs: 16,
  stopPxPerMs: 0.02,
  maxVelocityPxPerMs: 8,
  maxFrames: 120,
} as const;

/** Converts pixel deltas to whole lines, carrying the fractional remainder (px). */
export class ScrollAccumulator {
  private acc = 0;

  get remainder(): number {
    return this.acc;
  }

  /** Seed with the slop overshoot (travel minus SLOP_PX) so crossing emits no jump. */
  seed(overshootPx: number): void {
    this.acc = Number.isFinite(overshootPx) ? overshootPx : 0;
  }

  push(dyPx: number, cellH: number): LineDelta {
    if (!(cellH > 0) || !Number.isFinite(dyPx)) return 0 as LineDelta;
    this.acc += dyPx;
    const lines = Math.trunc(this.acc / cellH);
    this.acc -= lines * cellH;
    return lines as LineDelta;
  }

  reset(): void {
    this.acc = 0;
  }
}

interface Sample {
  t: number;
  y: number;
}

/**
 * Tracks recent samples, then after release yields decaying per-frame pixel
 * deltas. Output keeps the sign of the sampled y movement.
 */
export class MomentumTracker {
  private samples: Sample[] = [];
  private velocity = 0; // px/ms
  private frames = 0;
  private running = false;

  get active(): boolean {
    return this.running;
  }

  addSample(t: number, y: number): void {
    this.samples.push({ t, y });
    const cutoff = t - MOMENTUM_CONSTANTS.windowMs;
    while (this.samples.length > 2 && this.samples[0].t < cutoff) this.samples.shift();
  }

  /** Returns true if a fling started; false for reduced motion or a slow release. */
  release(reducedMotion: boolean): boolean {
    const samples = this.samples;
    this.samples = [];
    this.running = false;
    this.velocity = 0;
    this.frames = 0;
    if (reducedMotion || samples.length < 2) return false;

    const last = samples[samples.length - 1];
    const inWindow = samples.filter((s) => last.t - s.t <= MOMENTUM_CONSTANTS.windowMs);
    const first = inWindow[0];
    const dt = last.t - first.t;
    if (inWindow.length < 2 || dt <= 0) return false;

    const v = (last.y - first.y) / dt;
    if (Math.abs(v) < MOMENTUM_CONSTANTS.minFlingPxPerMs) return false;

    const cap = MOMENTUM_CONSTANTS.maxVelocityPxPerMs;
    this.velocity = Math.max(-cap, Math.min(cap, v));
    this.running = true;
    return true;
  }

  /** Advances by dtMs and returns the pixel delta; 0 once momentum has ended. */
  step(dtMs: number): number {
    if (!this.running) return 0;
    const dy = this.velocity * dtMs;
    this.frames++;
    this.velocity *= Math.pow(MOMENTUM_CONSTANTS.decayPerFrame, dtMs / MOMENTUM_CONSTANTS.frameMs);
    if (
      Math.abs(this.velocity) < MOMENTUM_CONSTANTS.stopPxPerMs ||
      this.frames >= MOMENTUM_CONSTANTS.maxFrames
    ) {
      this.running = false;
    }
    return dy;
  }

  cancel(): void {
    this.samples = [];
    this.velocity = 0;
    this.frames = 0;
    this.running = false;
  }
}

/** Clamps one frame's line output to +-rows; the excess is discarded, not carried. */
export function clampLinesPerFrame(lines: number, rows: number): LineDelta {
  return Math.max(-rows, Math.min(rows, lines)) as LineDelta;
}
