"use client";

import type {
  DescMessage,
  DescMethodUnary,
  DescMethodStreaming,
  MessageShape,
  MessageInitShape,
} from "@bufbuild/protobuf";
import type {
  ContextValues,
  StreamResponse,
  Transport,
  UnaryRequest,
  UnaryResponse,
} from "@connectrpc/connect";
import { Code, ConnectError, createContextValues } from "@connectrpc/connect";
import type { ConnectTransportOptions } from "@connectrpc/connect-web";
import {
  compressedFlag,
  createClientMethodSerializers,
  createMethodUrl,
  EnvelopedMessage,
  runStreamingCall,
  runUnaryCall,
} from "@connectrpc/connect/protocol";
import {
  endStreamFlag,
  endStreamFromJson,
  requestHeader,
} from "@connectrpc/connect/protocol-connect";
import { connect } from "it-ws/client";

/**
 * Check if debug logging is enabled via localStorage
 */
function isDebugEnabled(): boolean {
  if (typeof window === "undefined") return false;
  return localStorage.getItem("debug-terminal") === "true";
}

/**
 * Encodes a message in ConnectRPC envelope format.
 * Format: [1 byte flags][4 bytes big-endian length][N bytes data]
 */
function encodeEnvelope(flags: number, data: Uint8Array): Uint8Array {
  const envelope = new Uint8Array(5 + data.length);
  envelope[0] = flags;
  // Write length as big-endian uint32
  const view = new DataView(envelope.buffer, envelope.byteOffset, envelope.byteLength);
  view.setUint32(1, data.length, false); // false = big-endian
  envelope.set(data, 5);
  return envelope;
}

/**
 * Decompresses a gzip-compressed envelope payload (terminal-resync-reliability Epic 5.1,
 * Task 5.1.1.0/5.1.1.2). The server sets the envelope's CompressedFlag (see
 * server/protocol/envelope.go) when a payload exceeds its size threshold; this mirrors that
 * on the client so parseResponseBody can decode the frame instead of hard-throwing.
 *
 * Built on ReadableStream + DecompressionStream directly (rather than Response/Blob.stream())
 * so it also runs under Jest's jsdom environment, which polyfills those two from
 * node:stream/web (see jest.setup.js) but does not provide a global Response.
 */
export async function decompressGzipPayload(data: Uint8Array): Promise<Uint8Array> {
  // Typed as Uint8Array<ArrayBuffer> (rather than the bare Uint8Array default of
  // Uint8Array<ArrayBufferLike>) to match DecompressionStream.readable's declared type in
  // lib.dom.d.ts exactly — otherwise pipeThrough's generic inference can't reconcile the two
  // and tsc rejects the call.
  const stream = new ReadableStream<Uint8Array<ArrayBuffer>>({
    start(controller) {
      controller.enqueue(data as Uint8Array<ArrayBuffer>);
      controller.close();
    },
  });
  const decompressed = stream.pipeThrough(new DecompressionStream("gzip"));
  const reader = decompressed.getReader();

  const chunks: Uint8Array[] = [];
  let totalLength = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    totalLength += value.length;
  }

  const result = new Uint8Array(totalLength);
  let offset = 0;
  for (const chunk of chunks) {
    result.set(chunk, offset);
    offset += chunk.length;
  }
  return result;
}

/**
 * Creates a ConnectRPC transport that uses WebSocket for streaming calls.
 * This works around browser fetch() API limitations for bidirectional streaming.
 *
 * Based on: https://gist.github.com/Cyberax/3956c935a7971627e2ce8e2df3fafb8e
 */
export function createWebsocketBasedTransport(
  opt: ConnectTransportOptions
): Transport {
  return {
    // Use standard fetch for unary calls
    async unary<I extends DescMessage, O extends DescMessage>(
      method: DescMethodUnary<I, O>,
      signal: AbortSignal | undefined,
      timeoutMs: number | undefined,
      header: HeadersInit | undefined,
      input: MessageInitShape<I>,
      contextValues?: ContextValues
    ): Promise<UnaryResponse<I, O>> {
      const useBinaryFormat = opt.useBinaryFormat ?? true;
      const { serialize, parse } = createClientMethodSerializers(
        method,
        useBinaryFormat,
        opt.jsonOptions,
        opt.binaryOptions
      );

      return await runUnaryCall<I, O>({
        signal,
        interceptors: opt.interceptors,
        req: {
          stream: false as const,
          service: method.parent,
          method,
          url: createMethodUrl(opt.baseUrl, method),
          requestMethod: "POST",
          header: requestHeader(
            method.methodKind,
            useBinaryFormat,
            timeoutMs,
            header,
            false
          ),
          contextValues: contextValues ?? createContextValues(),
          message: input,
        },
        next: async (
          req: UnaryRequest<I, O>
        ): Promise<UnaryResponse<I, O>> => {
          const response = await fetch(req.url, {
            method: "POST",
            mode: "cors",
            headers: req.header,
            body: serialize(req.message) as BodyInit, // Uint8Array is valid BodyInit
            signal: req.signal,
          });

          // Check response status
          if (!response.ok) {
            const code = response.status === 401 ? Code.Unauthenticated : Code.Unknown;
            throw new ConnectError(
              `HTTP ${response.status}: ${response.statusText}`,
              code
            );
          }

          const bodyBytes = new Uint8Array(await response.arrayBuffer());
          const trailer = new Headers();
          const message = parse(bodyBytes);

          return {
            stream: false as const,
            header: response.headers,
            message,
            trailer,
            service: method.parent,
            method,
          } satisfies UnaryResponse<I, O>;
        },
      });
    },

    // Use WebSocket for streaming calls
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

      // Parse envelope protocol messages from WebSocket stream
      async function* parseResponseBody(
        body: AsyncGenerator<Uint8Array>,
        trailerTarget: Headers,
        headerRef: Headers,
        getCloseCode: () => number | null
      ) {
        const reader = createEnvelopeReadableStreamForWS(body).getReader();
        let endStreamReceived = false;

        for (;;) {
          const result = await reader.read();
          if (result.done) {
            break;
          }

          const { flags, data } = result.value;

          // The end-stream frame carries JSON trailer/error metadata (see endStreamFromJson
          // below) and is never compressed by the server, so it must be checked ahead of the
          // compressed-flag branch.
          if ((flags & endStreamFlag) === endStreamFlag) {
            endStreamReceived = true;
            const endStream = endStreamFromJson(data);

            if (endStream.error) {
              const error = endStream.error;
              headerRef.forEach((value: string, key: string) => {
                error.metadata.append(key, value);
              });
              throw error;
            }

            endStream.metadata.forEach((value: string, key: string) =>
              trailerTarget.set(key, value)
            );
            continue;
          }

          let payload = data;
          if ((flags & compressedFlag) === compressedFlag) {
            try {
              payload = await decompressGzipPayload(data);
            } catch (err) {
              throw new ConnectError(
                `protocol error: failed to decompress gzip payload: ${err instanceof Error ? err.message : String(err)}`,
                Code.DataLoss
              );
            }
          }

          yield parse(payload);
        }

        if (!endStreamReceived) {
          const code = getCloseCode();
          if (code !== null && code !== 1000 && !(signal?.aborted)) {
            throw new ConnectError(
              "WebSocket closed",
              Code.Unavailable,
              new Headers({ "ws-close-code": String(code) })
            );
          }
          throw new ConnectError("stream ended without end-stream message", Code.Internal);
        }
      }

      // Create request body for streaming call
      // For server streaming: send single request message
      // For bidirectional streaming: send initial request, then stream via WebSocket
      async function createRequestBody(
        input: AsyncIterable<MessageShape<I>>
      ): Promise<Uint8Array> {
        const r = await input[Symbol.asyncIterator]().next();
        if (r.done === true) {
          throw new Error("missing request message");
        }

        return encodeEnvelope(0, serialize(r.value));
      }

      timeoutMs =
        timeoutMs === undefined
          ? opt.defaultTimeoutMs
          : timeoutMs <= 0
          ? undefined
          : timeoutMs;

      return await runStreamingCall<I, O>({
        interceptors: opt.interceptors,
        timeoutMs,
        signal,
        req: {
          stream: true as const,
          service: method.parent,
          method,
          url: createMethodUrl(opt.baseUrl, method),
          requestMethod: "POST",
          header: requestHeader(
            method.methodKind,
            useBinaryFormat,
            timeoutMs,
            header,
            false
          ),
          contextValues: contextValues ?? createContextValues(),
          message: input,
        },
        next: async (req) => {
          // Convert HTTP URL to WebSocket URL
          const wsUrl = req.url.replace(/^http/, "ws");

          // Connect to WebSocket using it-ws
          const stream = connect(wsUrl);

          // Capture the WebSocket close code so we can propagate it to the hook
          // via a ConnectError metadata field. The it-ws source generator calls
          // EventIterator `stop` on close (which ends the async generator without
          // an error), so we capture it externally here.
          let wsCloseCode: number | null = null;
          (stream.socket as unknown as WebSocket).addEventListener("close", (ev: CloseEvent) => {
            wsCloseCode = ev.code;
          });

          if (signal !== undefined) {
            if (signal.aborted) stream.destroy();
            else signal.addEventListener("abort", () => stream.destroy(), { once: true });
          }

          // Wait for connection
          await stream.connected();

          // Send headers as text message
          let headerText = "";
          req.header.forEach((value: string, key: string) => {
            headerText += `${key}: ${value}\r\n`;
          });
          stream.socket.send(headerText + "\r\n");

          // Send all messages from input iterator (including initial handshake)
          (async () => {
            try {
              for await (const msg of req.message) {
                const serialized = serialize(msg as MessageShape<I>);
                if (isDebugEnabled()) {
                  console.log(`[WebSocket] Sending message:`, {
                    sessionId: (msg as any).sessionId,
                    dataCase: (msg as any).data?.case,
                    serializedLength: serialized.length,
                    envelopeLength: 5 + serialized.length,
                    readyState: (stream.socket as unknown as WebSocket).readyState,
                    bufferedAmount: (stream.socket as unknown as WebSocket).bufferedAmount,
                  });
                }
                const msgBytes = encodeEnvelope(0, serialized);
                stream.socket.send(msgBytes);
              }
              if (isDebugEnabled()) {
                console.log("[WebSocket] Message iterator completed, sending EndStream");
              }

              // CRITICAL: Send EndStream envelope to signal graceful close
              // Without this, the server detects an abnormal WebSocket closure (code 1005)
              // and cannot send EndStreamResponse, causing "missing EndStreamResponse" error
              const endStreamEnvelope = encodeEnvelope(endStreamFlag, new Uint8Array(0));
              stream.socket.send(endStreamEnvelope);

              if (isDebugEnabled()) {
                console.log("[WebSocket] EndStream sent successfully");
              }
            } catch (err) {
              console.error("[WebSocket] Error sending input messages:", err);
            }
          })();

          // Parse response headers from first message
          const headerMsg = await stream.source.next();
          const connectHeaders = parseHeaders(
            new TextDecoder().decode(headerMsg.value)
          );

          // Validate response
          const status = connectHeaders.get("Status-Code") ?? "-1";
          const statusCode = parseInt(status);

          if (statusCode !== 200) {
            const code = statusCode === 401 ? Code.Unauthenticated : Code.Unknown;
            throw new ConnectError(
              `WebSocket response status: ${statusCode}`,
              code
            );
          }

          const trailer = new Headers();

          const res: StreamResponse<I, O> = {
            ...req,
            header: connectHeaders,
            trailer,
            message: parseResponseBody(stream.source, trailer, connectHeaders, () => wsCloseCode),
          };

          return res;
        },
      });
    },
  };
}

// Parse HTTP headers from string format
function parseHeaders(allHeaders: string): Headers {
  return allHeaders
    .trim()
    .split(/[\r\n]+/)
    .reduce((memo, header) => {
      const [key, value] = header.split(": ");
      if (key && value) {
        memo.append(key, value);
      }
      return memo;
    }, new Headers());
}

// Create readable stream from async generator for envelope parsing
function createEnvelopeReadableStreamForWS(
  stream: AsyncGenerator<Uint8Array>
): ReadableStream<EnvelopedMessage> {
  let reader: AsyncGenerator<Uint8Array>;
  let buffer = new Uint8Array(0);

  function append(chunk: Uint8Array): void {
    const n = new Uint8Array(buffer.length + chunk.length);
    n.set(buffer);
    n.set(chunk, buffer.length);
    buffer = n;
  }

  return new ReadableStream<EnvelopedMessage>({
    start() {
      reader = stream;
    },

    async pull(controller): Promise<void> {
      let header: { length: number; flags: number } | undefined;

      for (;;) {
        // Try to parse header (5 bytes)
        if (header === undefined && buffer.byteLength >= 5) {
          let length = 0;
          for (let i = 1; i < 5; i++) {
            length = (length << 8) + buffer[i];
          }
          header = { flags: buffer[0], length };
        }

        // Check if we have full message
        if (header !== undefined && buffer.byteLength >= header.length + 5) {
          break;
        }

        // Read more data
        const result = await reader.next();
        if (result.done) {
          break;
        }
        append(result.value);
      }

      if (header === undefined) {
        if (buffer.byteLength === 0) {
          controller.close();
          return;
        }
        controller.error(
          new ConnectError("premature end of stream", Code.DataLoss)
        );
        return;
      }

      // Extract message data
      const data = buffer.subarray(5, 5 + header.length);
      buffer = buffer.subarray(5 + header.length);

      controller.enqueue({
        flags: header.flags,
        data,
      });
    },
  });
}
