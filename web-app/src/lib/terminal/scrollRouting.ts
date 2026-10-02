/**
 * Scroll routing for touch drags: decides where a scroll step is delivered
 * (xterm local scrollback vs. TUI via wheel reports or PgUp/PgDn) and encodes
 * the bytes. Pure and deterministic; clocks and loggers are injected.
 */

export type ScrollTarget = "xterm-local" | "tui-wheel" | "tui-pgkeys";
export type TuiScrollPolicy = "wheel" | "pgkeys";
export type ScrollOverride = "auto" | "local" | "tui";
export type BufferType = "normal" | "alternate";
export type MouseTrackingMode = "none" | "x10" | "vt200" | "drag" | "any";

export interface ScrollMode {
  bufferType: BufferType;
  mouseTrackingMode: MouseTrackingMode;
}

export interface ScrollRule {
  when: Partial<ScrollMode>;
  target: ScrollTarget;
}

export interface ScrollRoutingPolicy {
  rules: ScrollRule[];
  default: ScrollTarget;
}

/** Flips to true once the Phase 0 spike confirms the default rows. */
export const ROUTING_VERIFIED = false;
export const TUI_SCROLL_POLICY: TuiScrollPolicy = "pgkeys";

const TRACKING_ON: MouseTrackingMode[] = ["x10", "vt200", "drag", "any"];

export const DEFAULT_ROUTING_POLICY: ScrollRoutingPolicy = {
  rules: [
    { when: { bufferType: "alternate" }, target: "tui-pgkeys" },
    ...TRACKING_ON.map(
      (mouseTrackingMode): ScrollRule => ({ when: { mouseTrackingMode }, target: "tui-pgkeys" }),
    ),
  ],
  default: "xterm-local",
};

function ruleMatches(when: Partial<ScrollMode>, mode: ScrollMode): boolean {
  return (
    (when.bufferType === undefined || when.bufferType === mode.bufferType) &&
    (when.mouseTrackingMode === undefined || when.mouseTrackingMode === mode.mouseTrackingMode)
  );
}

/** First matching row wins; `tui-wheel` is only reachable when tuiPolicy is 'wheel'. */
export function decideScrollTarget(
  mode: ScrollMode,
  policy: ScrollRoutingPolicy = DEFAULT_ROUTING_POLICY,
  tuiPolicy: TuiScrollPolicy = TUI_SCROLL_POLICY,
  override: ScrollOverride = "auto",
): ScrollTarget {
  const tuiTarget: ScrollTarget = tuiPolicy === "wheel" ? "tui-wheel" : "tui-pgkeys";
  if (override === "local") return "xterm-local";
  if (override === "tui") return tuiTarget;
  const row = policy.rules.find((r) => ruleMatches(r.when, mode));
  const target = row ? row.target : policy.default;
  return target === "xterm-local" ? target : tuiTarget;
}

export interface RouteDecisionLog {
  target: ScrollTarget;
  source: "auto" | "override";
  unverified?: true;
  mode: ScrollMode;
}

export interface DecideAndLogOptions {
  policy?: ScrollRoutingPolicy;
  tuiPolicy?: TuiScrollPolicy;
  override?: ScrollOverride;
  routingVerified?: boolean;
  log?: (entry: RouteDecisionLog) => void;
}

/** decideScrollTarget plus a log entry carrying `unverified:true` while routing is unverified. */
export function decideAndLogScrollTarget(mode: ScrollMode, opts: DecideAndLogOptions = {}): ScrollTarget {
  const override = opts.override ?? "auto";
  const target = decideScrollTarget(mode, opts.policy, opts.tuiPolicy, override);
  const verified = opts.routingVerified ?? ROUTING_VERIFIED;
  const entry: RouteDecisionLog = {
    target,
    source: override === "auto" ? "auto" : "override",
    mode,
    ...(verified ? {} : { unverified: true as const }),
  };
  opts.log?.(entry);
  return target;
}

// ---- Encoders ----

export const MAX_WHEEL_REPORTS_PER_FRAME = 3;

export interface WheelCell {
  col: number;
  row: number;
  cols: number;
  rows: number;
}

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(Math.max(v, lo), hi);
}

/**
 * SGR wheel reports. lineDelta < 0 = toward older output (wheel up, button 64),
 * > 0 = toward newer (button 65). Emits at most MAX_WHEEL_REPORTS_PER_FRAME reports.
 */
export function encodeWheel(lineDelta: number, cell: WheelCell): string {
  const count = Math.min(Math.abs(Math.trunc(lineDelta)), MAX_WHEEL_REPORTS_PER_FRAME);
  if (count === 0) return "";
  const col = clamp(Math.trunc(cell.col), 1, Math.max(1, cell.cols));
  const row = clamp(Math.trunc(cell.row), 1, Math.max(1, cell.rows));
  const button = lineDelta < 0 ? 64 : 65;
  return `\x1b[<${button};${col};${row}M`.repeat(count);
}

/** X10 fallback; retained for the spike but deliberately not selectable via decideScrollTarget. */
export function encodeWheelX10(lineDelta: number, cell: WheelCell): string {
  const count = Math.min(Math.abs(Math.trunc(lineDelta)), MAX_WHEEL_REPORTS_PER_FRAME);
  if (count === 0) return "";
  const col = clamp(Math.trunc(cell.col), 1, Math.max(1, cell.cols));
  const row = clamp(Math.trunc(cell.row), 1, Math.max(1, cell.rows));
  const button = lineDelta < 0 ? 64 : 65;
  // X10 encodes each value as a single byte offset by 32 (limited to 223 cells).
  const ch = (n: number) => String.fromCharCode(clamp(n, 0, 223) + 32);
  return `\x1b[M${ch(button)}${ch(col)}${ch(row)}`.repeat(count);
}

export const PAGE_UP_BYTES = "\x1b[5~";
export const PAGE_DOWN_BYTES = "\x1b[6~";

/** direction 'older' = PgUp, 'newer' = PgDn (same bytes as the toolbar keys). */
export function encodePageKeys(direction: "older" | "newer", count = 1): string {
  return (direction === "older" ? PAGE_UP_BYTES : PAGE_DOWN_BYTES).repeat(Math.max(0, count));
}

// ---- Page accumulator ----

export const PAGE_RATE_LIMIT_MS = 100;
export const PAGE_FLING_CAP = 5;

/** Post-slop finger travel (lines) that emits one page key: half a page. */
export function tuiPageStepLines(rows: number): number {
  return Math.max(1, Math.floor((rows - 1) / 2));
}

export interface PageAccumulatorOptions {
  rows: number;
  now: () => number;
  rateLimitMs?: number;
  flingCap?: number;
}

/**
 * Accumulates line travel into page keys. Positive lines = newer (PgDn),
 * negative = older (PgUp). Emits at most one key per call (frame), carries the
 * remainder, rate-limits to one per rateLimitMs and caps keys per fling.
 */
export class PageAccumulator {
  private carry = 0;
  private emitted = 0;
  private lastEmitAt = Number.NEGATIVE_INFINITY;
  private rows: number;
  private readonly now: () => number;
  private readonly rateLimitMs: number;
  private readonly flingCap: number;

  constructor(opts: PageAccumulatorOptions) {
    this.rows = opts.rows;
    this.now = opts.now;
    this.rateLimitMs = opts.rateLimitMs ?? PAGE_RATE_LIMIT_MS;
    this.flingCap = opts.flingCap ?? PAGE_FLING_CAP;
  }

  setRows(rows: number): void {
    this.rows = rows;
  }

  /** True once the per-fling cap is hit; callers cancel momentum. */
  get capped(): boolean {
    return this.emitted >= this.flingCap;
  }

  /** Returns the page-key bytes for this frame ("" when none). */
  push(lines: number): string {
    this.carry += lines;
    const step = tuiPageStepLines(this.rows);
    if (Math.abs(this.carry) < step) return "";
    if (this.capped) {
      this.carry = 0;
      return "";
    }
    const t = this.now();
    if (t - this.lastEmitAt < this.rateLimitMs) return "";
    const older = this.carry < 0;
    this.carry += older ? step : -step;
    this.emitted += 1;
    this.lastEmitAt = t;
    return encodePageKeys(older ? "older" : "newer");
  }

  /** Reset at gesture start / mode flip. */
  reset(): void {
    this.carry = 0;
    this.emitted = 0;
    this.lastEmitAt = Number.NEGATIVE_INFINITY;
  }
}
