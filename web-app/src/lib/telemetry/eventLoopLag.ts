import type { AnalyticsProvider } from "@/lib/analytics/types";

export const LAG_TICK_MS = 100;
/** One event per window, carrying the worst drift seen in it. */
export const LAG_WINDOW_MS = 5000;
/** Drift under this is timer noise, not a busy main thread. */
export const LAG_REPORT_MIN_MS = 16;

export interface EventLoopLagOptions {
  track: AnalyticsProvider["track"];
  now?: () => number;
  isHidden?: () => boolean;
  setInterval?: (cb: () => void, ms: number) => unknown;
  clearInterval?: (handle: unknown) => void;
  listener?: () => string;
}

/**
 * Samples main-thread responsiveness: a 100 ms timer that fires late means the thread was busy,
 * which is exactly when a keystroke waits. Reports the worst drift per 5 s window as the
 * `eventloop_lag` terminal stage. A hidden tab is skipped because browsers throttle its timers
 * to 1 s, which would read as lag. Returns a stop function.
 */
export function observeEventLoopLag(opts: EventLoopLagOptions): () => void {
  const now = opts.now ?? (() => performance.now());
  const isHidden = opts.isHidden ?? (() => typeof document !== "undefined" && document.hidden);
  const setIv = opts.setInterval ?? ((cb, ms) => setInterval(cb, ms));
  const clearIv = opts.clearInterval ?? ((h) => clearInterval(h as ReturnType<typeof setInterval>));
  const listener =
    opts.listener ??
    (() => (typeof window !== "undefined" && ["localhost", "127.0.0.1", "[::1]", "::1"].includes(window.location.hostname) ? "local" : "remote"));

  let last = now();
  let worst = 0;
  const tickHandle = setIv(() => {
    const t = now();
    const drift = t - last - LAG_TICK_MS;
    last = t;
    if (!isHidden() && drift > worst) worst = drift;
  }, LAG_TICK_MS);
  const flushHandle = setIv(() => {
    const w = worst;
    worst = 0;
    if (w < LAG_REPORT_MIN_MS) return;
    opts.track({
      name: "cm.term_latency",
      category: "performance",
      durationMs: Math.round(w),
      labels: { stage: "eventloop_lag", listener: listener() },
    });
  }, LAG_WINDOW_MS);
  return () => {
    clearIv(tickHandle);
    clearIv(flushHandle);
  };
}
