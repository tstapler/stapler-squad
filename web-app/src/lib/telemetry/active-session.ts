// Lightweight session UUID state shared between otel-init.ts (SpanProcessor)
// and any component that needs to read it (e.g. WebVitalsReporter).
//
// Kept in its own file so components can import these functions WITHOUT
// pulling in the full OTel SDK initialisation, which runs module-level code
// (new WebTracerProvider / new OTLPTraceExporter) that fails during Next.js
// SSR prerendering when the relative trace URL can't be parsed.
let activeSessionUUID: string | null = null;

export function setActiveSessionUUID(uuid: string | null): void {
  activeSessionUUID = uuid;
}

export function getActiveSessionUUID(): string | null {
  return activeSessionUUID;
}
