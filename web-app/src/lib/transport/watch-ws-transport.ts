"use client";

import type {
  DescMessage,
  DescMethodStreaming,
  MessageInitShape,
  MessageShape,
} from "@bufbuild/protobuf";
import type { ContextValues, StreamResponse, Transport } from "@connectrpc/connect";
import { Code, ConnectError, createContextValues } from "@connectrpc/connect";
import type { ConnectTransportOptions } from "@connectrpc/connect-web";
import { createConnectTransport } from "@connectrpc/connect-web";
import {
  compressedFlag,
  createClientMethodSerializers,
  createMethodUrl,
  runStreamingCall,
} from "@connectrpc/connect/protocol";
import {
  endStreamFlag,
  endStreamFromJson,
} from "@connectrpc/connect/protocol-connect";

function encodeEnvelope(flags: number, data: Uint8Array): Uint8Array {
  const buf = new Uint8Array(5 + data.length);
  buf[0] = flags;
  new DataView(buf.buffer).setUint32(1, data.length, false);
  buf.set(data, 5);
  return buf;
}

export async function* fromWebSocket(
  ws: WebSocket,
  signal: AbortSignal | undefined
): AsyncGenerator<Uint8Array> {
  const queue: (Uint8Array | null | Error)[] = [];
  let notify: (() => void) | null = null;

  const push = (item: Uint8Array | null | Error) => {
    queue.push(item);
    notify?.();
    notify = null;
  };

  ws.onmessage = (e) => push(new Uint8Array(e.data as ArrayBuffer));
  ws.onerror = () => push(new ConnectError("WebSocket error", Code.Unavailable));
  ws.onclose = (ev: CloseEvent) => {
    if (signal?.aborted || ev.code === 1000) {
      push(null); // intentional close (1000=normal) or signal abort
    } else {
      // Code 1001 (Going Away / server restart) sets wasClean=true via TCP FIN but IS retriable.
      // Only 1000 and explicit signal abort are treated as non-retriable clean closes.
      push(new ConnectError("WebSocket closed", Code.Unavailable, new Headers({ "ws-close-code": String(ev.code) })));
    }
  };

  const abortHandler = () => {
    ws.close();
    push(null);
  };
  signal?.addEventListener("abort", abortHandler);

  try {
    while (true) {
      while (queue.length === 0) {
        await new Promise<void>((r) => {
          notify = r;
        });
      }
      const item = queue.shift()!;
      if (item === null) return;
      if (item instanceof Error) throw item;
      yield item;
    }
  } finally {
    signal?.removeEventListener("abort", abortHandler);
  }
}

/**
 * Creates a ConnectRPC transport that uses standard HTTP fetch for unary calls
 * and WebSocket for server-streaming calls. Compatible with StreamingWSBridge on
 * the server (server/services/ws_stream_bridge.go).
 *
 * Protocol (streaming):
 *   Client → first binary WS message: Connect request envelope (5-byte header + proto)
 *   Server → N binary WS messages: Connect response envelopes
 *   Server → final binary WS message: Connect end-stream envelope
 *
 * Auth: cookie-based (browser sends cookies automatically on WS upgrade).
 */
export function createWatchTransport(opt: ConnectTransportOptions): Transport {
  const httpTransport = createConnectTransport(opt);

  return {
    unary: httpTransport.unary.bind(httpTransport),

    async stream<I extends DescMessage, O extends DescMessage>(
      method: DescMethodStreaming<I, O>,
      signal: AbortSignal | undefined,
      timeoutMs: number | undefined,
      header: HeadersInit | undefined,
      input: AsyncIterable<MessageInitShape<I>>,
      contextValues?: ContextValues
    ): Promise<StreamResponse<I, O>> {
      const useBinaryFormat = opt.useBinaryFormat ?? true;
      const { serialize, parse } = createClientMethodSerializers(
        method,
        useBinaryFormat,
        opt.jsonOptions,
        opt.binaryOptions
      );

      const methodUrl = createMethodUrl(opt.baseUrl, method);

      return runStreamingCall<I, O>({
        interceptors: opt.interceptors,
        timeoutMs,
        signal,
        req: {
          stream: true as const,
          service: method.parent,
          method,
          url: methodUrl,
          requestMethod: "POST",
          header: new Headers(header instanceof Headers ? header : undefined),
          contextValues: contextValues ?? createContextValues(),
          message: input,
        },
        next: async (req) => {
          const wsUrl = req.url.replace(/^http/, "ws");

          const ws = new WebSocket(wsUrl);
          ws.binaryType = "arraybuffer";

          await new Promise<void>((resolve, reject) => {
            ws.onopen = () => resolve();
            ws.onerror = () =>
              reject(new ConnectError("WebSocket connection failed", Code.Unavailable));
          });

          // Read the first (and only) request message and send it as a Connect envelope.
          const iter = req.message[Symbol.asyncIterator]();
          const first = await iter.next();
          if (first.done) {
            ws.close();
            throw new ConnectError("missing request message", Code.Internal);
          }
          ws.send(encodeEnvelope(0, serialize(first.value)));

          const trailer = new Headers();

          async function* parseResponses(): AsyncGenerator<MessageShape<O>> {
            let buf = new Uint8Array(0);
            let endStreamReceived = false;

            for await (const chunk of fromWebSocket(ws, signal)) {
              // Accumulate chunk into buffer
              const merged = new Uint8Array(buf.length + chunk.length);
              merged.set(buf);
              merged.set(chunk, buf.length);
              buf = merged;

              // Parse all complete envelopes
              while (buf.length >= 5) {
                const flags = buf[0];
                const msgLen = new DataView(buf.buffer, buf.byteOffset).getUint32(1, false);
                if (buf.length < 5 + msgLen) break;

                const data = buf.slice(5, 5 + msgLen);
                buf = buf.slice(5 + msgLen);

                if (flags & compressedFlag) {
                  throw new ConnectError(
                    "unexpected compressed response",
                    Code.Internal
                  );
                }
                if (flags & endStreamFlag) {
                  endStreamReceived = true;
                  const end = endStreamFromJson(data);
                  if (end.error) throw end.error;
                  end.metadata.forEach((v, k) => trailer.set(k, v));
                } else {
                  yield parse(data);
                }
              }
            }

            if (!endStreamReceived) {
              throw new ConnectError(
                "stream ended without end-stream message",
                Code.Internal
              );
            }
          }

          return {
            ...req,
            header: new Headers({ "Content-Type": "application/connect+proto" }),
            trailer,
            message: parseResponses(),
          };
        },
      });
    },
  };
}
