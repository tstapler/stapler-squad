import { createConnectTransport } from "@connectrpc/connect-web";
import type { Transport } from "@connectrpc/connect";
import { getApiBaseUrl } from "@/lib/config";

let _transport: Transport | null = null;

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
 * non-watch RPCs, making it easy to add interceptors (auth, logging) in one
 * place.
 *
 * The streaming watch transport (createWatchTransport) is separate and should
 * NOT use this singleton — it has its own reconnect logic.
 */
export function getConnectTransport(): Transport {
  if (!_transport) {
    _transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      defaultTimeoutMs: DEFAULT_RPC_TIMEOUT_MS,
    });
  }
  return _transport;
}

/** Reset the transport singleton (for testing only). */
export function _resetTransportForTesting(): void {
  _transport = null;
}
