/**
 * Client sender for the phone input layer's `cm.input` and `cm.ime_anomaly` beacons (ADR-005).
 * Content-free by construction: the only inputs are enums (result class, anomaly kind, renderer), so no
 * typed text can reach the beacon. Hook `onDiagnostic` and `onAnomaly` to the matching attach streams.
 */
import type { AnalyticsEvent } from "@/lib/analytics/types";
import { trackSharedEvent } from "@/lib/telemetry/rpcTiming";
import type { ImeAnomaly, MobileInputClock, MobileInputDiagnostic } from "./types";

export type TelemetryClock = MobileInputClock;
export type InputRenderer = "xterm" | "ghostty";
type InputEventClass = "emitted" | "queued" | "dropped_disconnected";

/** One anomaly beacon per kind per window, with a burst allowance. */
export const ANOMALY_WINDOW_MS = 10_000;
export const ANOMALY_BURST = 5;
/** Per-keystroke counts are summed and sent at most once per window. */
export const INPUT_FLUSH_MS = 10_000;

const ANOMALY_KINDS: ReadonlySet<string> = new Set<ImeAnomaly>([
  "orphaned_input",
  "rewrite_bound_exceeded",
  "unrecognized_sequence",
  "stuck_composition",
  "selection_in_region",
  "duplicate_suppressed",
]);

export interface MobileInputTelemetryOptions {
  renderer: InputRenderer;
  clock: TelemetryClock;
  /** Defaults to the shared analytics sink used by the other `cm.*` events. */
  track?: (event: AnalyticsEvent) => void;
}

export interface MobileInputTelemetry {
  onDiagnostic(d: MobileInputDiagnostic): void;
  onAnomaly(kind: ImeAnomaly): void;
  flush(): void;
  dispose(): void;
  stats(): { anomaliesSuppressed: number };
}

export function createMobileInputTelemetry(opts: MobileInputTelemetryOptions): MobileInputTelemetry {
  const { renderer, clock } = opts;
  const track = opts.track ?? trackSharedEvent;
  const pending: Record<InputEventClass, number> = { emitted: 0, queued: 0, dropped_disconnected: 0 };
  const buckets = new Map<string, { tokens: number; at: number }>();
  let timer: number | null = null;
  let disposed = false;
  let suppressed = 0;

  const send = (event: AnalyticsEvent): void => {
    try {
      track(event);
    } catch {
      // Telemetry must never disturb typing.
    }
  };

  const flush = (): void => {
    if (timer !== null) {
      clock.clearTimeout(timer);
      timer = null;
    }
    for (const event of Object.keys(pending) as InputEventClass[]) {
      const count = pending[event];
      if (count === 0) continue;
      pending[event] = 0;
      send({ name: "cm.input", category: "performance", labels: { event, renderer, count: String(count) } });
    }
  };

  const add = (event: InputEventClass): void => {
    pending[event]++;
    if (timer === null) timer = clock.setTimeout(flush, INPUT_FLUSH_MS);
  };

  const takeToken = (kind: string): boolean => {
    const now = clock.now();
    const b = buckets.get(kind) ?? { tokens: ANOMALY_BURST, at: now };
    b.tokens = Math.min(ANOMALY_BURST, b.tokens + (now - b.at) / ANOMALY_WINDOW_MS);
    b.at = now;
    const ok = b.tokens >= 1;
    if (ok) b.tokens -= 1;
    buckets.set(kind, b);
    return ok;
  };

  return {
    onDiagnostic(d) {
      if (disposed || d.event !== "unit") return;
      add("emitted");
      if (d.result === "queued") add("queued");
      else if (d.result === "dropped") add("dropped_disconnected");
    },
    onAnomaly(kind) {
      if (disposed || !ANOMALY_KINDS.has(kind)) return;
      if (!takeToken(kind)) {
        suppressed++;
        return;
      }
      send({ name: "cm.ime_anomaly", category: "performance", labels: { kind, renderer } });
    },
    flush,
    dispose() {
      flush();
      disposed = true;
    },
    stats: () => ({ anomaliesSuppressed: suppressed }),
  };
}
