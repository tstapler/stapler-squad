/**
 * Client counters for the layer: `cm.input{event, renderer}`. The beacon sink is installed by the
 * observability slice (Story 2.2.2); until then every increment is a no-op, so PR-A1 ships the
 * adapter and its call sites without any telemetry dependency.
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
