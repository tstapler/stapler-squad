import { registerInstrumentations } from "@opentelemetry/instrumentation";
import { DocumentLoadInstrumentation } from "@opentelemetry/instrumentation-document-load";
import { FetchInstrumentation } from "@opentelemetry/instrumentation-fetch";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { ATTR_SERVICE_NAME } from "@opentelemetry/semantic-conventions";
import { BatchSpanProcessor, WebTracerProvider } from "@opentelemetry/sdk-trace-web";
import type { SpanProcessor, Span, ReadableSpan } from "@opentelemetry/sdk-trace-web";
import { ZoneContextManager } from "@opentelemetry/context-zone";
import { getActiveSessionUUID } from "./active-session";
import { recordSpan } from "./span-buffer";

// Same-origin by default: the Go server proxies this to the real collector
// (server/handlers/otel_proxy_handler.go) so the browser never needs CORS
// access to the collector itself.
const tracesUrl =
  process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT ?? "/api/otel/v1/traces";

// Writes `session.uuid` onto every span started while a session is focused.
// State lives in active-session.ts (no OTel imports) so components can read
// it without triggering this file's module-level SDK initialisation.
class SessionSpanProcessor implements SpanProcessor {
  onStart(span: Span): void {
    const uuid = getActiveSessionUUID();
    if (uuid !== null) {
      span.setAttribute("session.uuid", uuid);
    }
  }

  onEnd(span: ReadableSpan): void {
    if (span.attributes["session.uuid"] === undefined) return;
    recordSpan({
      name: span.name,
      traceId: span.spanContext().traceId,
      spanId: span.spanContext().spanId,
      parentSpanId: span.parentSpanContext?.spanId,
      startTimeMs: span.startTime[0] * 1000 + span.startTime[1] / 1e6,
      durationMs: span.duration[0] * 1000 + span.duration[1] / 1e6,
      status: span.status.code,
      attributes: { ...span.attributes },
    });
  }

  shutdown(): Promise<void> {
    return Promise.resolve();
  }

  forceFlush(): Promise<void> {
    return Promise.resolve();
  }
}

const provider = new WebTracerProvider({
  resource: resourceFromAttributes({
    [ATTR_SERVICE_NAME]: "stapler-squad-web",
  }),
  spanProcessors: [
    new BatchSpanProcessor(new OTLPTraceExporter({ url: tracesUrl })),
    new SessionSpanProcessor(),
  ],
});

provider.register({
  contextManager: new ZoneContextManager(),
});

registerInstrumentations({
  instrumentations: [
    new DocumentLoadInstrumentation(),
    new FetchInstrumentation({
      // Don't trace the exporter's own export calls — every export would
      // otherwise emit a span that needs exporting, spamming the trace with
      // noise about itself.
      ignoreUrls: [new RegExp(tracesUrl.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))],
    }),
  ],
});
