import { createConnectTransport } from "@connectrpc/connect-web";
import { createContextValues } from "@connectrpc/connect";
import type { Interceptor, Transport } from "@connectrpc/connect";
import { createMethodUrl, runStreamingCall, runUnaryCall } from "@connectrpc/connect/protocol";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { createCoalescingInterceptor } from "./coalescingInterceptor";
import { createPriorityInterceptor } from "./requestPriority";
import { peekFlags, preloadFlagValue } from "@/lib/features/flagStore";
import { withInFlightGauge } from "@/lib/perf/perfDebug";
import { createSharedRpcTimingInterceptor } from "@/lib/telemetry/rpcTiming";
import { createSessionWatchTransport } from "@/lib/transport/watch-ws-transport";
import { createUnaryWsTransport, type UnaryWsTransport } from "@/lib/transport/unary-ws-transport";
import {
  AuthAwareBreaker,
  createAuthRequiredInterceptor,
  createWsUnaryPolicyTransport,
} from "@/lib/transport/wsUnaryPolicy";

const _transports = new Map<string, Transport>();
let _watchTransport: Transport | null = null;
let _keepaliveTransport: Transport | null = null;

/**
 * Every unary RPC through this transport sends a Connect-Timeout-Ms header
 * with this value unless a call site passes its own timeoutMs — connect-go
 * reads that header and derives the handler's ctx deadline automatically, so
 * this is what makes "the backend can tell a caller disconnected" true for
 * ordinary RPCs (streamViaHub's WebSocket path uses connection-scoped
 * cancellation instead, since a live terminal stream has no fixed deadline).
 */
const DEFAULT_RPC_TIMEOUT_MS = 30_000;

/** Server value once flags have loaded, else the pre-load default; the flag-store import cycle is safe because this runs per call. */
function isRequestCoalescingEnabled(): boolean {
  const loaded = peekFlags()?.find((f) => f.name === "request_coalescing");
  return loaded ? loaded.enabled : (preloadFlagValue("request_coalescing") ?? true);
}

/**
 * Runs `interceptors` and the default deadline once around `inner` (the policy transport),
 * so they apply identically to HTTP and WebSocket calls. `inner` must carry neither.
 */
function withOuterInterceptors(
  inner: Transport,
  baseUrl: string,
  interceptors: Interceptor[],
  defaultTimeoutMs: number,
): Transport {
  return {
    unary: (method, signal, timeoutMs, header, input, contextValues) =>
      runUnaryCall({
        interceptors,
        signal,
        timeoutMs: timeoutMs ?? defaultTimeoutMs,
        req: {
          stream: false,
          service: method.parent,
          method,
          requestMethod: "POST",
          url: createMethodUrl(baseUrl, method),
          header: new Headers(header),
          contextValues: contextValues ?? createContextValues(),
          message: input,
        } as never,
        next: (req) =>
          inner.unary(method, req.signal, timeoutMs ?? defaultTimeoutMs, req.header, req.message as never, req.contextValues),
      }) as never,
    stream: (method, signal, timeoutMs, header, input, contextValues) =>
      runStreamingCall({
        interceptors,
        signal,
        timeoutMs: timeoutMs ?? defaultTimeoutMs,
        req: {
          stream: true,
          service: method.parent,
          method,
          requestMethod: "POST",
          url: createMethodUrl(baseUrl, method),
          header: new Headers(header),
          contextValues: contextValues ?? createContextValues(),
          message: input,
        } as never,
        next: (req) =>
          inner.stream(method, req.signal, timeoutMs ?? defaultTimeoutMs, req.header, req.message as never, req.contextValues),
      }) as never,
  };
}

/**
 * Returns the shared ConnectRPC HTTP transport, memoized per {baseUrl, timeoutMs}
 * (default: one singleton).
 *
 * Every non-streaming hook should call this instead of constructing its own
 * transport. This ensures a single transport instance for all unary/streaming
 * non-watch RPCs, carrying the auth interceptor (redirects to /login on a
 * 401) in one place instead of every call site wiring it up separately.
 *
 * The streaming watch transport (createWatchTransport) is separate and should
 * NOT use this singleton — it has its own reconnect logic. Use
 * getWatchTransport() for that.
 */
export interface ConnectTransportOptions {
  /** Overrides getApiBaseUrl(); rare (tests, cross-origin tooling). */
  baseUrl?: string;
  /** Overrides the default Connect-Timeout-Ms for long-running unary RPCs. */
  timeoutMs?: number;
}

export function getConnectTransport(opts: ConnectTransportOptions = {}): Transport {
  const baseUrl = opts.baseUrl ?? getApiBaseUrl();
  const timeoutMs = opts.timeoutMs ?? DEFAULT_RPC_TIMEOUT_MS;
  const key = `${baseUrl}|${timeoutMs}`;
  let transport = _transports.get(key);
  if (!transport) {
    // The inner HTTP transport carries no interceptors and no deadline: withOuterInterceptors owns both, once, for HTTP and WS calls alike.
    const http = createConnectTransport({ baseUrl, fetch: withInFlightGauge() });
    const breaker = new AuthAwareBreaker();
    let ws: UnaryWsTransport | null = null;
    const policy = createWsUnaryPolicyTransport({
      http,
      breaker,
      getWs: () => (ws ??= createUnaryWsTransport({ baseUrl, fallback: http, defaultTimeoutMs: timeoutMs, breaker })),
    });
    // Priority first (once per transport, ADR-002 item 7); coalescing next: one flight is one auth/timing/network pass, however many callers join it.
    transport = withOuterInterceptors(
      policy,
      baseUrl,
      [
        createPriorityInterceptor(),
        createCoalescingInterceptor({ isEnabled: isRequestCoalescingEnabled }),
        createAuthInterceptor(),
        createAuthRequiredInterceptor(),
        createSharedRpcTimingInterceptor(),
      ],
      timeoutMs,
    );
    _transports.set(key, transport);
  }
  return transport;
}

/**
 * A unary transport whose requests use `fetch(..., { keepalive: true })`, so a
 * request started as the page hides or unloads still reaches the server. Bodies
 * must stay under the browser's 64KB keepalive limit; only use it for small
 * "complete this action now" RPCs (the tray's deferred clear).
 */
export function getKeepaliveConnectTransport(): Transport {
  if (!_keepaliveTransport) {
    _keepaliveTransport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      defaultTimeoutMs: DEFAULT_RPC_TIMEOUT_MS,
      interceptors: [createAuthInterceptor()],
      // Cast: Bun's global fetch type also requires `preconnect`, which a wrapper cannot provide (tsconfig.bun.json).
      fetch: ((input, init) => fetch(input, { ...init, keepalive: true })) as typeof fetch,
    });
  }
  return _keepaliveTransport;
}

/**
 * Returns the shared transport singleton for server-streaming Watch* RPCs.
 *
 * Selects WebSocket-bridge vs. native ConnectRPC streaming per
 * createSessionWatchTransport's rules (see watch-ws-transport.ts) — every
 * Watch* hook should call this instead of constructing its own transport, so
 * that choice and the auth interceptor live in one place.
 */
export function getWatchTransport(): Transport {
  if (!_watchTransport) {
    _watchTransport = createSessionWatchTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
  }
  return _watchTransport;
}

/** Reset the transport singletons (for testing only). */
export function _resetTransportForTesting(): void {
  _transports.clear();
  _watchTransport = null;
  _keepaliveTransport = null;
}
