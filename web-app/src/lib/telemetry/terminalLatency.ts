import type { AnalyticsProvider } from "@/lib/analytics/types";
import type { InputSendSample } from "./terminalInputTiming";

/** One keystroke is sampled per this gap, so a held key or a paste costs one probe, not hundreds. */
export const ECHO_MIN_GAP_MS = 250;
/** A reply later than this is unrelated output (a silent prompt, a long command), not an echo. */
export const ECHO_TIMEOUT_MS = 3000;
/** Largest sample the server accepts (see maxTermLatencyMs); anything slower is a throttled tab. */
export const MAX_SAMPLE_MS = 10_000;
/** Most terminal-latency events one tab may send per minute. */
export const MAX_TERM_EVENTS_PER_MINUTE = 300;

export type TermLatencyStage =
  | "echo_arrive"
  | "echo_paint"
  | "echo_render"
  | "input_queue"
  | "ws_buffered_kib"
  | "eventloop_lag"
  | "ttfp_connect"
  | "ttfp_first_data"
  | "ttfp_first_paint";

export interface TerminalLatencyProbe {
  /** A keystroke or paste is being sent to the session. */
  noteInput(): void;
  /** An output frame reached the component (call before writing it to xterm). */
  noteOutput(): void;
  /** The stream opened. */
  noteConnected(): void;
  /** An input frame reached the socket; counts only for the keystroke currently being sampled. */
  noteInputSent(sample: InputSendSample): void;
}

export interface TerminalLatencyProbeOptions {
  track: AnalyticsProvider["track"];
  now?: () => number;
  /** Runs cb on the next animation frame; injectable for tests. */
  raf?: (cb: () => void) => void;
  /** "local" for loopback pages, else "remote"; the server folds anything else to "unknown". */
  listener?: () => string;
  /** Whether the tab is visible; rAF is paused in hidden tabs, so paint samples would be unbounded. */
  isVisible?: () => boolean;
}

function defaultListener(): string {
  if (typeof window === "undefined") return "unknown";
  const h = window.location.hostname;
  return h === "localhost" || h === "127.0.0.1" || h === "[::1]" || h === "::1" ? "local" : "remote";
}

function defaultIsVisible(): boolean {
  return typeof document === "undefined" || document.visibilityState === "visible";
}

/**
 * Measures, per terminal mount, what the user feels: keystroke sent -> echo frame arrived ->
 * echo painted (two animation frames after the write, i.e. past the next paint), and mount ->
 * stream open / first data / first paint. Samples carry only a stage name and a duration:
 * no keystroke content, no session id. The echo is the next output frame after the keystroke,
 * which is a heuristic and an upper bound when other output is streaming.
 */
export function createTerminalLatencyProbe(opts: TerminalLatencyProbeOptions): TerminalLatencyProbe {
  const now = opts.now ?? (() => performance.now());
  const raf =
    opts.raf ?? ((cb) => (typeof requestAnimationFrame === "function" ? requestAnimationFrame(() => cb()) : cb()));
  const listener = opts.listener ?? defaultListener;
  const isVisible = opts.isVisible ?? defaultIsVisible;
  const mountedAt = now();
  let pendingInputAt: number | null = null;
  let sendNoted = false;
  let lastSampledAt = -Infinity;
  let windowStart = mountedAt;
  let sent = 0;
  let connected = false;
  let firstData = false;

  const emit = (stage: TermLatencyStage, durationMs: number) => {
    if (!isVisible() || durationMs > MAX_SAMPLE_MS) return;
    const t = now();
    if (t - windowStart >= 60_000) {
      windowStart = t;
      sent = 0;
    }
    if (sent >= MAX_TERM_EVENTS_PER_MINUTE) return;
    sent++;
    opts.track({
      name: "cm.term_latency",
      category: "performance",
      durationMs: Math.max(0, Math.round(durationMs)),
      labels: { stage, listener: listener() },
    });
  };

  // Two frames: the first runs before the paint that follows the write, the second after it.
  const afterPaint = (cb: () => void) => raf(() => raf(cb));

  // rAF is paused in a hidden tab, so a paint callback that fires long after the frame was
  // requested measures the pause, not the paint: drop it rather than report an unbounded value.
  const emitPaint = (stage: TermLatencyStage, startedAt: number, requestedAt: number) => {
    const t = now();
    if (t - requestedAt > ECHO_TIMEOUT_MS) return;
    emit(stage, t - startedAt);
  };

  return {
    noteInput() {
      const t = now();
      if (pendingInputAt !== null && t - pendingInputAt <= ECHO_TIMEOUT_MS) return;
      if (t - lastSampledAt < ECHO_MIN_GAP_MS) return;
      pendingInputAt = t;
      sendNoted = false;
      lastSampledAt = t;
    },
    noteInputSent(sample) {
      if (pendingInputAt === null || sendNoted) return;
      sendNoted = true;
      emit("input_queue", sample.queuedMs);
      emit("ws_buffered_kib", Math.ceil(sample.bufferedBytes / 1024)); // ceil: any backlog reads as at least 1 KiB
    },
    noteOutput() {
      const arrivedAt = now();
      if (!firstData) {
        firstData = true;
        emit("ttfp_first_data", arrivedAt - mountedAt);
        afterPaint(() => emitPaint("ttfp_first_paint", mountedAt, arrivedAt));
      }
      if (pendingInputAt === null) return;
      const sentAt = pendingInputAt;
      pendingInputAt = null;
      if (arrivedAt - sentAt > ECHO_TIMEOUT_MS) return;
      emit("echo_arrive", arrivedAt - sentAt);
      afterPaint(() => {
        emitPaint("echo_paint", sentAt, arrivedAt);
        emitPaint("echo_render", arrivedAt, arrivedAt);
      });
    },
    noteConnected() {
      if (connected) return;
      connected = true;
      emit("ttfp_connect", now() - mountedAt);
    },
  };
}
