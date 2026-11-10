/**
 * Optional in-process counter hook for `onDiagnostic`. Production beacons go through
 * `telemetry.ts` (installed by `useMobileInput`); this sink stays a no-op unless a caller sets one.
 */
import type { MobileInputDiagnostic } from "./types";

export type InputCounterEvent = "emitted" | "dropped_disconnected";

export interface InputCounterSink {
  increment(event: InputCounterEvent, renderer: string): void;
}

let sink: InputCounterSink | null = null;

export function setInputCounterSink(next: InputCounterSink | null): void {
  sink = next;
}

/** An `onDiagnostic` handler that counts each unit by outcome. A failing sink never breaks typing. */
export function countingDiagnostic(renderer: string): (entry: MobileInputDiagnostic) => void {
  return (entry) => {
    if (entry.event !== "unit") return;
    try {
      sink?.increment(entry.result === "dropped" ? "dropped_disconnected" : "emitted", renderer);
    } catch {
      /* telemetry must not break typing */
    }
  };
}
