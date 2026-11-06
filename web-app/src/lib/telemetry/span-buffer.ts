// Bounded in-memory record of ended browser spans, read by the diagnostic
// bundle export. No OTel imports so components can use it without triggering
// otel-init.ts's module-level SDK setup.

export interface BufferedSpan {
  name: string;
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  startTimeMs: number;
  durationMs: number;
  status: number;
  attributes: Record<string, unknown>;
}

const MAX_SPANS = 2000;
const spans: BufferedSpan[] = [];

export function recordSpan(span: BufferedSpan): void {
  spans.push(span);
  if (spans.length > MAX_SPANS) spans.shift();
}

/** Spans tagged with sessionUuid, JSON-encoded for the export request. */
export function getSessionSpansJson(sessionUuid: string): Uint8Array {
  const matching = spans.filter((s) => s.attributes["session.uuid"] === sessionUuid);
  return new TextEncoder().encode(JSON.stringify(matching));
}
