import { trace } from "@opentelemetry/api";
import type { Interceptor } from "@connectrpc/connect";
import type { AnalyticsProvider } from "@/lib/analytics/types";

/**
 * Network-queueing-delay attribute name set on the active OTel span (see
 * getNetworkQueueDelayMs below). Named to sort next to the standard
 * `http.*`/`url.*` fetch-instrumentation attributes in a trace viewer.
 */
const NETWORK_QUEUE_TIME_ATTRIBUTE = "network.queue_time_ms";

/**
 * Returns a proxy for how long this call's underlying fetch spent queued or
 * negotiating a connection before the browser actually put bytes on the
 * wire — the gap between the Resource Timing API's `fetchStart` and
 * `requestStart` for the matching entry. On an HTTP/1.1 origin capped at 6
 * concurrent connections per browser (this app, prior to the WS-bridge fix
 * for the Watch-family RPCs and the terminal stream — see
 * docs/how-to/enable-opentelemetry.md's "Browser network queueing delay"
 * section), a request that has to wait for a free connection shows a large
 * gap here even though the server never saw it start late. Distinguishing
 * that from genuine server-side latency was previously only possible by
 * hand-querying Tempo and noticing an RPC had *no* server-side span at all.
 *
 * Matches by exact URL (same-origin, so ConnectRPC's per-method absolute
 * URL is unique enough in practice) and picks the most recently recorded
 * matching entry, since a prior call to the same RPC method would have left
 * an earlier, already-superseded entry in the resource timing buffer.
 * Returns undefined if the Resource Timing API is unavailable (non-browser
 * test environment) or no matching/complete entry is found.
 */
function getNetworkQueueDelayMs(url: string): number | undefined {
  if (typeof performance === "undefined" || typeof performance.getEntriesByType !== "function") {
    return undefined;
  }
  const entries = performance.getEntriesByType("resource") as PerformanceResourceTiming[];
  for (let i = entries.length - 1; i >= 0; i--) {
    const entry = entries[i];
    if (entry.name !== url) {
      continue;
    }
    if (entry.requestStart > 0 && entry.fetchStart > 0) {
      return entry.requestStart - entry.fetchStart;
    }
    return undefined;
  }
  return undefined;
}

/**
 * ConnectRPC interceptor that records timing for every unary RPC call.
 *
 * Each call emits a Performance API entry:
 *   performance.mark(`rpc:<MethodName>:start`)
 *   performance.measure(`rpc:<MethodName>`, start, end)
 *
 * Entries are visible in Chrome DevTools > Performance > Timings track and
 * accessible via window.performance.getEntriesByType('measure').
 *
 * Attributes recorded in the measure detail:
 *   { method, url, ok, durationMs, queueDelayMs }
 *
 * queueDelayMs (see getNetworkQueueDelayMs) is also set as the
 * `network.queue_time_ms` attribute on the currently active OTel span —
 * normally the fetch span web-app/src/lib/telemetry/otel-init.ts's
 * FetchInstrumentation created for this exact call, which stays active
 * through the fetch promise chain via ZoneContextManager — so a queued
 * request is visibly distinguishable from a slow server-side one directly
 * in Tempo/Grafana. A no-op (nothing thrown) when OTel tracing isn't
 * enabled, since trace.getActiveSpan() returns undefined in that case.
 *
 * When an optional analytics provider is supplied, each RPC call also enqueues
 * an analytics event with category "rpc" for latency tracking.
 */
export function createRpcTimingInterceptor(analytics?: Pick<AnalyticsProvider, "track">): Interceptor {
  return (next) => async (req) => {
    const method = req.method.name;
    const startMark = `rpc:${method}:start`;

    if (typeof performance !== "undefined") {
      performance.mark(startMark);
    }
    const wallStart = Date.now();

    let ok = true;
    try {
      return await next(req);
    } catch (err) {
      ok = false;
      throw err;
    } finally {
      const durationMs = Date.now() - wallStart;
      const queueDelayMs = getNetworkQueueDelayMs(req.url);

      if (typeof performance !== "undefined") {
        try {
          performance.measure(`rpc:${method}`, {
            start: startMark,
            detail: { method, url: req.url, ok, durationMs, queueDelayMs },
          });
        } catch {
          // start mark may be missing in some edge cases; ignore
        }
      }

      if (queueDelayMs !== undefined) {
        trace.getActiveSpan()?.setAttribute(NETWORK_QUEUE_TIME_ATTRIBUTE, queueDelayMs);
      }

      if (process.env.NODE_ENV !== "production") {
        console.debug(
          `[rpc] ${method} ${durationMs}ms ${ok ? "✓" : "✗"}${queueDelayMs !== undefined ? ` (queued ${queueDelayMs}ms)` : ""}`,
        );
      }

      analytics?.track({
        name: `rpc.${method}`,
        category: "rpc",
        durationMs,
        labels: {
          method,
          ok: String(ok),
          ...(queueDelayMs !== undefined ? { networkQueueDelayMs: String(queueDelayMs) } : {}),
        },
      });
    }
  };
}
