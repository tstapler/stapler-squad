import { registerInstrumentations } from "@opentelemetry/instrumentation";
import { DocumentLoadInstrumentation } from "@opentelemetry/instrumentation-document-load";
import { FetchInstrumentation } from "@opentelemetry/instrumentation-fetch";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { ATTR_SERVICE_NAME } from "@opentelemetry/semantic-conventions";
import { BatchSpanProcessor, WebTracerProvider } from "@opentelemetry/sdk-trace-web";
import { ZoneContextManager } from "@opentelemetry/context-zone";

// Same-origin by default: the Go server proxies this to the real collector
// (server/handlers/otel_proxy_handler.go) so the browser never needs CORS
// access to the collector itself.
const tracesUrl =
  process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT ?? "/api/otel/v1/traces";

const provider = new WebTracerProvider({
  resource: resourceFromAttributes({
    [ATTR_SERVICE_NAME]: "stapler-squad-web",
  }),
  spanProcessors: [new BatchSpanProcessor(new OTLPTraceExporter({ url: tracesUrl }))],
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
