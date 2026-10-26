import { createConnectTransport } from "@connectrpc/connect-web";
import type { Transport } from "@connectrpc/connect";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { withInFlightGauge } from "@/lib/perf/perfDebug";
import { createSharedRpcTimingInterceptor } from "@/lib/telemetry/rpcTiming";
import { createSessionWatchTransport } from "@/lib/transport/watch-ws-transport";

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
    transport = createConnectTransport({
      baseUrl,
      defaultTimeoutMs: timeoutMs,
      interceptors: [createAuthInterceptor(), createSharedRpcTimingInterceptor()],
      fetch: withInFlightGauge(),
    });
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
