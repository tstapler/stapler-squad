/**
 * Lazily loads and starts browser OpenTelemetry tracing when
 * NEXT_PUBLIC_OTEL_ENABLED=true. Disabled by default, mirroring the Go
 * server's OTEL_ENABLED gate (see docs/how-to/enable-opentelemetry.md).
 *
 * Split into its own dynamically-imported module (./otel-init) so the OTel
 * SDK never ships to browsers that have it disabled — this is a static
 * export with no server runtime, so NEXT_PUBLIC_* values are baked in at
 * `next build` time, not read at request time.
 */
let started = false;

export function initBrowserOtel(): void {
  if (started || typeof window === "undefined") return;
  if (process.env.NEXT_PUBLIC_OTEL_ENABLED !== "true") return;
  started = true;

  import("./otel-init").catch((err) => {
    console.error("[otel] failed to initialize browser tracing", err);
  });
}
