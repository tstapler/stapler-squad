import { createConnectTransport } from "@connectrpc/connect-web";
import type { Transport } from "@connectrpc/connect";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { createSessionWatchTransport } from "@/lib/transport/watch-ws-transport";

let _transport: Transport | null = null;
let _watchTransport: Transport | null = null;

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
 * Returns the shared ConnectRPC HTTP transport singleton.
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
export function getConnectTransport(): Transport {
  if (!_transport) {
    _transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      defaultTimeoutMs: DEFAULT_RPC_TIMEOUT_MS,
      interceptors: [createAuthInterceptor()],
    });
  }
  return _transport;
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
  _transport = null;
  _watchTransport = null;
}
