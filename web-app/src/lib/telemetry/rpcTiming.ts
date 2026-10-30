import { trace } from "@opentelemetry/api";
import { Code, ConnectError, createContextKey } from "@connectrpc/connect";
import type { ContextKey, ContextValues, Interceptor, UnaryRequest } from "@connectrpc/connect";
import type { AnalyticsEvent, AnalyticsProvider } from "@/lib/analytics/types";
import { buildDedupeKey, isCoalescible } from "@/lib/api/idempotentMethods";

/** "stream" marks server-streaming calls, whose duration is time-to-headers, so dashboards can exclude them from unary latency. */
export type RpcTransportLabel = "ws" | "http" | "batch" | "stream";

/** Mutable per-call record the inner transports fill in; the timing interceptor reads it after the call. */
export interface RpcCallInfo {
  transport: RpcTransportLabel;
}

// Guard: many Jest suites mock "@connectrpc/connect" without createContextKey, and this module is imported transitively.
export const rpcCallInfoKey: ContextKey<RpcCallInfo | undefined> =
  typeof createContextKey === "function"
    ? createContextKey<RpcCallInfo | undefined>(undefined, { description: "ssq rpc call info (client metrics)" })
    : ({ id: Symbol("ssq rpc call info"), defaultValue: undefined } as ContextKey<RpcCallInfo | undefined>);

/** Tells the timing interceptor which path actually served the call; a no-op outside an instrumented call. */
export function markRpcTransport(contextValues: ContextValues | undefined, transport: RpcTransportLabel): void {
  const info = contextValues?.get(rpcCallInfoKey);
  if (info) info.transport = transport;
}

/** Connect code as the snake_case name the server's client-metrics allow-list expects. */
export function codeLabel(err: unknown): string {
  const code = typeof ConnectError?.from === "function" ? ConnectError.from(err).code : undefined;
  const name = code === undefined ? undefined : Code?.[code];
  return typeof name === "string" ? name.replace(/([a-z])([A-Z])/g, "$1_$2").toLowerCase() : "unknown";
}

// Tab-wide, shared by every interceptor instance, so concurrency and duplicates are seen across hooks.
let inFlightCalls = 0;
const inFlightByKey = new Map<string, number>();

const READ_METHOD = /^(Get|List|Stat|Search|Find|Read)/;

/** Identical-call key for read-shaped unary methods only: two overlapping identical writes are legitimate, not duplicates. */
function dedupeKeyOf(req: { stream: boolean }): string | undefined {
  if (req.stream) return undefined;
  try {
    const u = req as unknown as UnaryRequest;
    if (!READ_METHOD.test(u.method.name)) return undefined;
    return buildDedupeKey(u.method, u.message);
  } catch {
    return undefined;
  }
}

// jsdom (Jest) defines `performance` without mark/measure.
const canMeasure = () =>
  typeof performance !== "undefined" && typeof performance.mark === "function" && typeof performance.measure === "function";

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
export interface RpcTimingOptions {
  /**
   * Flag identical in-flight reads (`dup=1`). Only meaningful on the shared transport, where
   * the coalescing interceptor sits outside this one; elsewhere every overlap would look like a miss.
   */
  detectDuplicates?: boolean;
}

export function createRpcTimingInterceptor(
  analytics?: Pick<AnalyticsProvider, "track">,
  options: RpcTimingOptions = {},
): Interceptor {
  const safeSink = analytics ? { track: (e: AnalyticsEvent) => safeTrack(analytics, e) } : undefined;
  return (next) => async (req) => {
    const method = req.method.name;
    const startMark = `rpc:${method}:start`;

    if (canMeasure()) {
      performance.mark(startMark);
    }
    const wallStart = Date.now();

    const info: RpcCallInfo = { transport: req.stream ? "stream" : "http" };
    req.contextValues?.set(rpcCallInfoKey, info);
    const dedupeKey = options.detectDuplicates ? dedupeKeyOf(req) : undefined;
    const inflightAtStart = inFlightCalls;
    const duplicate = dedupeKey !== undefined && (inFlightByKey.get(dedupeKey) ?? 0) > 0;
    inFlightCalls++;
    if (dedupeKey !== undefined) inFlightByKey.set(dedupeKey, (inFlightByKey.get(dedupeKey) ?? 0) + 1);

    let ok = true;
    let code = "ok";
    try {
      return await next(req);
    } catch (err) {
      ok = false;
      code = codeLabel(err);
      throw err;
    } finally {
      inFlightCalls--;
      if (dedupeKey !== undefined) {
        const left = (inFlightByKey.get(dedupeKey) ?? 1) - 1;
        if (left <= 0) inFlightByKey.delete(dedupeKey);
        else inFlightByKey.set(dedupeKey, left);
      }
      const durationMs = Date.now() - wallStart;
      const queueDelayMs = getNetworkQueueDelayMs(req.url);

      if (canMeasure()) {
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

      safeSink?.track({
        name: `rpc.${method}`,
        category: "rpc",
        durationMs,
        labels: {
          method,
          ok: String(ok),
          code,
          transport: info.transport,
          inflight: String(inflightAtStart),
          ...(duplicate ? { dup: "1", coalescible: isCoalescible(method) ? "1" : "0" } : {}),
          ...(queueDelayMs !== undefined ? { networkQueueDelayMs: String(queueDelayMs) } : {}),
        },
      });
    }
  };
}

/** Telemetry must never change an RPC's outcome: a throwing sink is swallowed. */
function safeTrack(sink: Pick<AnalyticsProvider, "track">, event: AnalyticsEvent): void {
  try {
    sink.track(event);
  } catch {
    // dropped on purpose
  }
}

type RpcTimingSink = Pick<AnalyticsProvider, "track">;
let rpcTimingSink: RpcTimingSink | undefined;

/** Registered once by the analytics provider; shared-transport rpc events are dropped until then. */
export function setRpcTimingSink(sink: RpcTimingSink | undefined): void {
  rpcTimingSink = sink;
}

/** rpc-timing interceptor for the shared transport; reads the sink at call time since the transport outlives React context. */
export function createSharedRpcTimingInterceptor(): Interceptor {
  return createRpcTimingInterceptor({ track: (event) => rpcTimingSink?.track(event) }, { detectDuplicates: true });
}

/** Emits a non-RPC event (e.g. `ws_unary_fallback_total`) to the same sink; dropped until one is registered. */
export function trackSharedEvent(event: Parameters<RpcTimingSink["track"]>[0]): void {
  if (rpcTimingSink) safeTrack(rpcTimingSink, event);
}

/** Socket lifecycle for the client-metrics beacon; `code` is the WebSocket close code when the browser gave one. */
export function recordWsLifecycle(kind: "unary" | "watch", event: "open" | "close" | "error" | "reconnect", code?: number): void {
  trackSharedEvent({
    name: "cm.ws",
    category: "performance",
    labels: { kind, event, ...(code !== undefined ? { code: String(code) } : {}) },
  });
}
