import { createConnectTransport } from "@connectrpc/connect-web";
import type { Transport } from "@connectrpc/connect";
import { getApiBaseUrl } from "@/lib/config";

let _transport: Transport | null = null;

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
    _transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
  }
  return _transport;
}

/** Reset the transport singleton (for testing only). */
export function _resetTransportForTesting(): void {
  _transport = null;
}
