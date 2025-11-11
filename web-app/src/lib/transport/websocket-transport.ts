"use client";

import {
  AnyMessage,
  Message,
  MethodInfo,
  MethodKind,
  PartialMessage,
  ServiceType,
} from "@bufbuild/protobuf";
import type {
  ContextValues,
  StreamResponse,
  Transport,
  UnaryRequest,
  UnaryResponse,
} from "@connectrpc/connect";
import { Code, ConnectError, createContextValues } from "@connectrpc/connect";
import { ConnectTransportOptions } from "@connectrpc/connect-web";
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
  validateResponse,
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
    async unary<
      I extends Message<I> = AnyMessage,
      O extends Message<O> = AnyMessage
    >(
      service: ServiceType,
      method: MethodInfo<I, O>,
      signal: AbortSignal | undefined,
      timeoutMs: number | undefined,
      header: Headers,
      message: PartialMessage<I>,
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
          stream: false,
          service,
          method,
          url: createMethodUrl(opt.baseUrl, service, method),
          init: {
            method: "POST",
            mode: "cors",
          },
          header: requestHeader(
            MethodKind.Unary,
            useBinaryFormat,
            timeoutMs,
            header,
            false
          ),
          contextValues: contextValues ?? createContextValues(),
          message,
        },
        next: async (
          req: UnaryRequest<I, O>
        ): Promise<UnaryResponse<I, O>> => {
          const response = await fetch(req.url, {
            method: req.init.method ?? "POST",
            mode: req.init.mode as RequestMode,
            headers: req.header,
            body: serialize(req.message) as BodyInit, // Uint8Array is valid BodyInit
            signal: req.signal,
          });

          // Check response status
          if (!response.ok) {
            throw new ConnectError(
              `HTTP ${response.status}: ${response.statusText}`,
              Code.Unknown
            );
          }

          const bodyBytes = new Uint8Array(await response.arrayBuffer());
          const trailer = new Headers();
          const message = parse(bodyBytes);

          return {
            stream: false,
            header: response.headers,
            message,
            trailer,
            service,
            method,
          } satisfies UnaryResponse<I, O>;
        },
      });
    },

    // Use WebSocket for streaming calls
    async stream<
      I extends Message<I> = AnyMessage,
      O extends Message<O> = AnyMessage
    >(
      service: ServiceType,
      method: MethodInfo<I, O>,
      signal: AbortSignal | undefined,
      timeoutMs: number | undefined,
      header: Headers | undefined,
      input: AsyncIterable<PartialMessage<I>>,
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
        headerRef: Headers
      ) {
        const reader = createEnvelopeReadableStreamForWS(body).getReader();
        let endStreamReceived = false;

        for (;;) {
          const result = await reader.read();
          if (result.done) {
            break;
          }

          const { flags, data } = result.value;

          if ((flags & compressedFlag) === compressedFlag) {
            throw new ConnectError(
              `protocol error: received unsupported compressed output`,
              Code.Internal
            );
          }

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

          yield parse(data);
        }

        if (!endStreamReceived) {
          throw new Error("missing EndStreamResponse");
        }
      }

      // Create request body for streaming call
      // For server streaming: send single request message
      // For bidirectional streaming: send initial request, then stream via WebSocket
      async function createRequestBody(
        input: AsyncIterable<I>
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
          stream: true,
          service,
          method,
          url: createMethodUrl(opt.baseUrl, service, method),
          init: {
            method: "POST",
            credentials: opt.credentials ?? "same-origin",
            redirect: "error",
            mode: "cors",
          },
          header: requestHeader(
            method.kind,
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

          if (signal !== undefined) {
            if (signal.aborted) stream.destroy();
            else signal.onabort = () => stream.destroy();
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
                const serialized = serialize(msg);
                if (isDebugEnabled()) {
                  console.log(`[WebSocket] Sending message:`, {
                    sessionId: (msg as any).sessionId,
                    dataCase: (msg as any).data?.case,
                    serializedLength: serialized.length,
                    envelopeLength: 5 + serialized.length
                  });
                }
                const msgBytes = encodeEnvelope(0, serialized);
                stream.socket.send(msgBytes);
              }
              if (isDebugEnabled()) {
                console.log("[WebSocket] Message iterator completed");
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
            throw new ConnectError(
              `WebSocket response status: ${statusCode}`,
              Code.Unknown
            );
          }

          const trailer = new Headers();

          const res: StreamResponse<I, O> = {
            ...req,
            header: connectHeaders,
            trailer,
            message: parseResponseBody(stream.source, trailer, connectHeaders),
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
