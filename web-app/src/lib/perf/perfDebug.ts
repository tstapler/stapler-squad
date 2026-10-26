/** Runtime-opt-in diagnostics surface at `window.__ssqPerf`; absent unless opted in. */
export interface SsqPerf {
  /** Enables `traceCall` stacks. */
  trace: boolean;
}

declare global {
  interface Window {
    __ssqPerf?: SsqPerf;
  }
}

/** Logs the call stack for `label` when `__ssqPerf.trace` is on; no-op otherwise. */
export function traceCall(label: string): void {
  if (typeof window === "undefined" || !window.__ssqPerf?.trace) return;
  console.trace(`[ssqPerf] ${label}`);
}
