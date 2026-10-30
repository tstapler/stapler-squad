"use client";

import type {
  DescMessage,
  DescMethodUnary,
  MessageInitShape,
} from "@bufbuild/protobuf";
import { fromBinary, toBinary } from "@bufbuild/protobuf";
import type { ContextValues, Transport, UnaryResponse } from "@connectrpc/connect";
import type { ContextKey } from "@connectrpc/connect";
import { Code, ConnectError, createContextKey, createContextValues } from "@connectrpc/connect";
import { createMethodUrl, runUnaryCall } from "@connectrpc/connect/protocol";
import { codeFromString } from "@connectrpc/connect/protocol-connect";
import { IDEMPOTENT_METHODS } from "@/lib/api/idempotentMethods";
import { recordWsLifecycle, trackSharedEvent } from "@/lib/telemetry/rpcTiming";
import allowlist from "./__fixtures__/ws-unary-allowlist.json";
import {
  MAX_FRAME_BYTES,
  decodeFrame,
  encodeFrame,
  setFrameId,
  type ErrorJson,
  type ResponseMeta,
} from "./unary-mux-frame";

/**
 * Connect Transport whose `unary` multiplexes over one shared WebSocket
 * (`/api/ws/unary`, ADR-001/002/003). Streaming is not supported here: the policy
 * transport (Story 5.4.1) keeps streams on their own bridges.
 *
 * Interceptors, the default deadline and flag/eligibility checks are applied once,
 * outermost, by the policy transport; this class only runs `runUnaryCall` with an
 * empty interceptor list. Request headers beyond the ADR-001 allowlist never ride
 * the socket (credentials are the upgrade request's cookie).
 */

export const SOCKET_PATH = "/api/ws/unary";
export const CONNECT_WINDOW_MS = 250;
export const MAX_IN_FLIGHT: number = allowlist.maxInFlight;
export const SEND_QUEUE_MAX_FRAMES = 64;
export const SEND_QUEUE_MAX_BYTES = 1024 * 1024;
export const PING_INTERVAL_MS = 10_000;
export const PONG_DEADLINE_MS = 5_000;
export const RECONNECT_BASE_MS = 100;
export const RECONNECT_CAP_MS = 5_000;
export const BREAKER_THRESHOLD = 3;
export const BREAKER_OPEN_MS = 60_000;
const DEFAULT_TIMEOUT_MS = 30_000;
const REPLAYABLE = new Set<string>(IDEMPOTENT_METHODS.replayable);

/** Per-call hook the policy sets to learn when the REQUEST frame was written (wsQueueMs). */
export const frameWrittenKey: ContextKey<(() => void) | undefined> =
  // Guard: many Jest suites mock "@connectrpc/connect" without createContextKey.
  typeof createContextKey === "function"
    ? createContextKey<(() => void) | undefined>(undefined, { description: "ws request frame written" })
    : ({ id: Symbol("ws request frame written"), defaultValue: undefined } as ContextKey<(() => void) | undefined>);

// Guard: lib/api/transport imports this module, and many Jest suites mock "@connectrpc/connect" without ConnectError.
const ErrorBase: typeof ConnectError = ConnectError ?? (class {} as unknown as typeof ConnectError);

/** ConnectError from an ERROR frame; `marker` is `details.marker` (e.g. `ws_method_not_allowed`). */
export class WsUnaryError extends ErrorBase {
  readonly marker: string | undefined;
  constructor(json: ErrorJson) {
    const meta = new Headers();
    for (const [k, v] of Object.entries({ ...json.headers, ...json.trailers })) meta.set(k, v);
    super(json.message ?? "", codeFromString(json.code) ?? Code.Unknown, meta);
    this.marker = json.details?.marker;
  }
}

/** Consecutive-failure breaker: opens after 3 socket failures for 60 s, then allows one probe. */
export class UnaryWsBreaker {
  private count = 0;
  private openUntil = 0;

  get failures(): number {
    return this.count;
  }

  recordFailure(): void {
    this.count++;
    if (this.count >= BREAKER_THRESHOLD) this.openUntil = Date.now() + BREAKER_OPEN_MS;
  }

  recordSuccess(): void {
    this.count = 0;
    this.openUntil = 0;
  }

  /** True while the HTTP-only window is running. */
  isOpen(): boolean {
    return this.count >= BREAKER_THRESHOLD && Date.now() < this.openUntil;
  }

  msUntilProbe(): number {
    return this.count >= BREAKER_THRESHOLD ? Math.max(0, this.openUntil - Date.now()) : 0;
  }
}

export interface UnaryWsTransportOptions {
  baseUrl: string;
  /** HTTP transport used for ineligible, oversized, queue-timed-out and replayed calls. */
  fallback: Transport;
  /** Deadline when the caller passes none; sent as REQUEST meta `timeoutMs`. */
  defaultTimeoutMs?: number;
  isReplayable?: (method: DescMethodUnary) => boolean;
  breaker?: UnaryWsBreaker;
  /** Test seams. */
  WebSocketImpl?: typeof WebSocket;
  pageHref?: string;
  random?: () => number;
}

export interface UnaryWsTransport extends Transport {
  /** Starts (or restarts) the shared socket; idempotent. */
  connect(): void;
  /** Intentional close: no reconnect, no breaker count. */
  close(): void;
  readonly breaker: UnaryWsBreaker;
}

/** `http`->`ws`, `https`->`wss`, fixed path, no query/fragment; null when mixed content would result. */
export function deriveSocketUrl(baseUrl: string, pageHref: string): string | null {
  const page = new URL(pageHref);
  const base = new URL(baseUrl || "/", pageHref);
  const secure = base.protocol === "https:";
  if (!secure && base.protocol !== "http:") return null;
  if (page.protocol === "https:" && !secure) return null;
  return `${secure ? "wss" : "ws"}://${base.host}${SOCKET_PATH}`;
}

type SocketState = "idle" | "connecting" | "open" | "backoff";

interface Call {
  id: number;
  frame: Uint8Array;
  sent: boolean;
  replayable: boolean;
  resolve: (res: UnaryResponse<DescMessage, DescMessage>) => void;
  reject: (err: unknown) => void;
  /** Unary response bytes; set once RESPONSE (fin=0) arrives. */
  assembly?: { totalLen: number; parts: Uint8Array[]; received: number; meta: ResponseMeta };
  toResponse: (meta: ResponseMeta, payload: Uint8Array) => UnaryResponse<DescMessage, DescMessage>;
  viaHttp: () => Promise<UnaryResponse<DescMessage, DescMessage>>;
  detach: () => void;
  onWritten?: () => void;
  submittedAt: number;
  pendingAtSubmit: number;
}

function concatParts(parts: Uint8Array[], total: number): Uint8Array {
  if (parts.length === 1) return parts[0];
  const out = new Uint8Array(total);
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}

class UnaryWsTransportImpl implements UnaryWsTransport {
  readonly breaker: UnaryWsBreaker;
  private readonly fallback: Transport;
  private readonly baseUrl: string;
  private readonly defaultTimeoutMs: number;
  private readonly replayableFn: (method: DescMethodUnary) => boolean;
  private readonly Ctor: typeof WebSocket;
  private readonly random: () => number;
  private readonly socketUrl: string | null;

  private state: SocketState = "idle";
  private ws: WebSocket | null = null;
  private wanted = false;
  private connectStartedAt = 0;
  private windowTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pingTimer: ReturnType<typeof setInterval> | null = null;
  private pongTimer: ReturnType<typeof setTimeout> | null = null;
  private listening = false;

  private readonly calls = new Map<number, Call>();
  private queue: Call[] = [];
  private queuedBytes = 0;
  private outstanding = 0;
  private nextId = 1;

  constructor(opt: UnaryWsTransportOptions) {
    this.baseUrl = opt.baseUrl;
    this.fallback = opt.fallback;
    this.defaultTimeoutMs = opt.defaultTimeoutMs ?? DEFAULT_TIMEOUT_MS;
    this.replayableFn = opt.isReplayable ?? ((m) => REPLAYABLE.has(m.name));
    this.breaker = opt.breaker ?? new UnaryWsBreaker();
    this.Ctor = opt.WebSocketImpl ?? WebSocket;
    this.random = opt.random ?? Math.random;
    const pageHref = opt.pageHref ?? (typeof location !== "undefined" ? location.href : "http://localhost/");
    this.socketUrl = deriveSocketUrl(opt.baseUrl, pageHref);
  }

  unary<I extends DescMessage, O extends DescMessage>(
    method: DescMethodUnary<I, O>,
    signal: AbortSignal | undefined,
    timeoutMs: number | undefined,
    header: HeadersInit | undefined,
    input: MessageInitShape<I>,
    contextValues?: ContextValues
  ): Promise<UnaryResponse<I, O>> {
    const effectiveTimeout = timeoutMs ?? this.defaultTimeoutMs;
    return runUnaryCall<I, O>({
      interceptors: [],
      signal,
      timeoutMs: effectiveTimeout,
      req: {
        stream: false,
        service: method.parent,
        method,
        requestMethod: "POST",
        url: createMethodUrl(this.baseUrl, method),
        header: new Headers(header),
        contextValues: contextValues ?? createContextValues(),
        message: input,
      },
      next: (req) => this.invoke(method, req.message, req.signal, req.header, req.contextValues, effectiveTimeout),
    });
  }

  connect(): void {
    this.wanted = true;
    this.addPageListeners();
    if (this.state !== "idle" || this.socketUrl === null) return;
    if (this.breaker.msUntilProbe() > 0) {
      this.scheduleReconnect();
      return;
    }
    this.openSocket();
  }

  close(): void {
    this.wanted = false;
    this.removePageListeners();
    this.clearReconnect();
    if (this.ws) this.dropSocket(this.ws, false);
    this.state = "idle";
  }

  private invoke<I extends DescMessage, O extends DescMessage>(
    method: DescMethodUnary<I, O>,
    message: MessageInitShape<I> & object,
    signal: AbortSignal,
    header: Headers,
    contextValues: ContextValues,
    timeoutMs: number
  ): Promise<UnaryResponse<I, O>> {
    const viaHttp = () =>
      this.fallback.unary(method, signal, timeoutMs, header, message, contextValues);
    const frame = encodeFrame({
      type: "REQUEST",
      id: 0,
      method: `/${method.parent.typeName}/${method.name}`,
      meta: { headers: { "Connect-Protocol-Version": "1" }, timeoutMs },
      body: toBinary(method.input, message as never),
    });
    if (frame.length > MAX_FRAME_BYTES || !this.socketUsable()) return viaHttp();
    if (signal.aborted) return Promise.reject(ConnectError.from(signal.reason, Code.Canceled));

    return new Promise<UnaryResponse<I, O>>((resolve, reject) => {
      const call: Call = {
        id: this.allocId(),
        frame,
        sent: false,
        replayable: this.replayableFn(method as unknown as DescMethodUnary),
        resolve: resolve as Call["resolve"],
        reject,
        toResponse: (meta, payload) => ({
          stream: false,
          service: method.parent,
          method,
          header: new Headers(meta.headers),
          trailer: new Headers(meta.trailers),
          message: fromBinary(method.output, payload),
        }),
        viaHttp: viaHttp as Call["viaHttp"],
        detach: () => signal.removeEventListener("abort", onAbort),
        onWritten: contextValues.get(frameWrittenKey),
        submittedAt: Date.now(),
        pendingAtSubmit: this.outstanding + this.queue.length,
      };
      const onAbort = () => {
        const wasSent = call.sent;
        this.settle(call);
        if (wasSent) this.sendRaw(encodeFrame({ type: "CANCEL", id: call.id }));
        reject(ConnectError.from(signal.reason, Code.Canceled));
      };
      signal.addEventListener("abort", onAbort, { once: true });
      setFrameId(frame, call.id);
      this.calls.set(call.id, call);

      if (this.state === "open" && this.queue.length === 0 && this.outstanding < MAX_IN_FLIGHT) {
        this.send(call);
        return;
      }
      if (
        this.queue.length + 1 > SEND_QUEUE_MAX_FRAMES ||
        this.queuedBytes + frame.length > SEND_QUEUE_MAX_BYTES
      ) {
        this.settle(call);
        reject(new ConnectError("unary websocket send queue is full", Code.ResourceExhausted));
        return;
      }
      this.queue.push(call);
      this.queuedBytes += frame.length;
      this.connect();
    });
  }

  /** Decides per call whether the socket may carry it (ADR-002 item 4). */
  private socketUsable(): boolean {
    if (this.socketUrl === null || this.breaker.isOpen()) return false;
    if (this.state === "backoff") return false;
    if (this.state === "idle") {
      this.connect();
      const after = this.state as SocketState; // connect() mutates state
      return after === "connecting" || after === "open";
    }
    if (this.state === "connecting") return Date.now() - this.connectStartedAt < CONNECT_WINDOW_MS;
    return true;
  }

  private allocId(): number {
    // Odd ids only (even are reserved for server push, ADR-001); monotonic so a settled id is not reused soon.
    for (let i = 0; i < 0x80000000; i++) {
      const id = this.nextId;
      this.nextId = id >= 0xffffffff ? 1 : id + 2;
      if (!this.calls.has(id)) return id;
    }
    throw new ConnectError("no free unary websocket request id", Code.ResourceExhausted);
  }

  private openSocket(): void {
    this.state = "connecting";
    this.connectStartedAt = Date.now();
    let ws: WebSocket;
    try {
      ws = new this.Ctor(this.socketUrl as string);
    } catch {
      this.state = "idle";
      this.breaker.recordFailure();
      this.failQueueOverHttp();
      this.scheduleReconnect();
      return;
    }
    ws.binaryType = "arraybuffer";
    this.ws = ws;
    ws.onopen = () => {
      if (this.ws !== ws) return;
      recordWsLifecycle("unary", "open");
      this.clearWindow();
      this.state = "open";
      this.pump();
      this.updateLiveness();
    };
    ws.onmessage = (e) => this.onMessage(ws, e.data);
    ws.onerror = () => {
      if (this.ws === ws) recordWsLifecycle("unary", "error");
      this.dropSocket(ws, true);
    };
    ws.onclose = (ev?: CloseEvent) => {
      if (this.ws === ws) recordWsLifecycle("unary", "close", ev?.code);
      this.dropSocket(ws, true);
    };
    this.windowTimer = setTimeout(() => {
      this.windowTimer = null;
      if (this.state === "connecting") this.failQueueOverHttp();
    }, CONNECT_WINDOW_MS);
  }

  private clearWindow(): void {
    if (this.windowTimer) clearTimeout(this.windowTimer);
    this.windowTimer = null;
  }

  private clearReconnect(): void {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
  }

  /** Calls that never reached the wire are safe to send over HTTP whatever their method. */
  private failQueueOverHttp(): void {
    const queued = this.queue;
    this.queue = [];
    this.queuedBytes = 0;
    for (const call of queued) this.runOverHttp(call);
  }

  private runOverHttp(call: Call): void {
    this.settle(call);
    call.viaHttp().then(call.resolve, call.reject);
  }

  /** Removes a call from every index; safe to call twice. */
  private settle(call: Call): void {
    if (this.calls.get(call.id) !== call) return;
    this.calls.delete(call.id);
    call.detach();
    if (call.sent) {
      this.outstanding--;
    } else {
      const i = this.queue.indexOf(call);
      if (i >= 0) {
        this.queue.splice(i, 1);
        this.queuedBytes -= call.frame.length;
      }
    }
    this.pump();
    this.updateLiveness();
  }

  private send(call: Call): void {
    call.sent = true;
    this.outstanding++;
    this.sendRaw(call.frame);
    call.onWritten?.();
    trackSharedEvent({
      name: "cm.ws_send",
      category: "performance",
      durationMs: Date.now() - call.submittedAt,
      labels: { pending: String(call.pendingAtSubmit) },
    });
    this.updateLiveness();
  }

  private sendRaw(raw: Uint8Array): void {
    const ws = this.ws;
    if (!ws || this.state !== "open") return;
    try {
      ws.send(raw);
    } catch {
      this.dropSocket(ws, true);
    }
  }

  private pump(): void {
    while (this.state === "open" && this.queue.length > 0 && this.outstanding < MAX_IN_FLIGHT) {
      const call = this.queue.shift() as Call;
      this.queuedBytes -= call.frame.length;
      this.send(call);
    }
  }

  private onMessage(ws: WebSocket, data: unknown): void {
    if (this.ws !== ws) return;
    if (!(data instanceof ArrayBuffer)) {
      this.dropSocket(ws, true);
      return;
    }
    let frame;
    try {
      frame = decodeFrame(new Uint8Array(data));
    } catch {
      this.dropSocket(ws, true);
      return;
    }
    switch (frame.type) {
      case "RESPONSE": {
        this.breaker.recordSuccess();
        const call = this.calls.get(frame.id);
        if (!call?.sent) return; // late reply for a settled, cancelled or replayed id
        if (frame.fin) {
          this.finish(call, frame.meta, frame.body);
        } else {
          call.assembly = { totalLen: frame.totalLen, parts: [frame.body], received: frame.body.length, meta: frame.meta };
        }
        return;
      }
      case "CHUNK": {
        const call = this.calls.get(frame.id);
        const a = call?.assembly;
        if (!call || !a) return;
        a.parts.push(frame.body);
        a.received += frame.body.length;
        if (a.received > a.totalLen || (frame.fin && a.received !== a.totalLen)) {
          this.settle(call);
          this.sendRaw(encodeFrame({ type: "CANCEL", id: call.id }));
          call.reject(new ConnectError("unary websocket response length mismatch", Code.Internal));
        } else if (frame.fin) {
          this.finish(call, a.meta, concatParts(a.parts, a.totalLen));
        }
        return;
      }
      case "ERROR": {
        this.breaker.recordSuccess();
        const call = this.calls.get(frame.id);
        if (!call?.sent) return;
        this.settle(call);
        call.reject(new WsUnaryError(frame.json));
        return;
      }
      case "PONG":
        this.breaker.recordSuccess();
        this.clearPong();
        return;
      case "PING":
        this.sendRaw(encodeFrame({ type: "PONG", id: frame.id }));
        return;
      default:
        return;
    }
  }

  private finish(call: Call, meta: ResponseMeta, payload: Uint8Array): void {
    this.settle(call);
    try {
      call.resolve(call.toResponse(meta, payload));
    } catch (e) {
      call.reject(ConnectError.from(e, Code.Internal));
    }
  }

  /**
   * Tears down `ws`. With `failure`, counts toward the breaker and reconnects with
   * backoff; sent calls fail Unavailable unless replayable (then they replay over HTTP).
   */
  private dropSocket(ws: WebSocket, failure: boolean): void {
    if (this.ws !== ws) return;
    this.ws = null;
    this.clearWindow();
    this.clearLiveness();
    try {
      ws.close();
    } catch {
      // already closed
    }
    this.state = "idle";
    if (failure) this.breaker.recordFailure();
    this.resolveAllCalls();
    if (failure) this.scheduleReconnect();
  }

  private resolveAllCalls(): void {
    const all = [...this.calls.values()];
    for (const call of all) {
      if (!call.sent) {
        this.runOverHttp(call);
      } else if (call.replayable) {
        this.runOverHttp(call);
      } else {
        this.settle(call);
        call.reject(new ConnectError("unary websocket closed", Code.Unavailable));
      }
    }
  }

  private backoffDelayMs(): number {
    const exp = Math.min(RECONNECT_CAP_MS, RECONNECT_BASE_MS * 2 ** Math.max(0, this.breaker.failures - 1));
    return exp * (1 - 0.2 * this.random());
  }

  private scheduleReconnect(): void {
    if (!this.wanted || this.reconnectTimer || this.socketUrl === null) return;
    this.state = "backoff";
    recordWsLifecycle("unary", "reconnect");
    const delay = Math.max(this.backoffDelayMs(), this.breaker.msUntilProbe());
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.state = "idle";
      if (this.wanted) this.connect();
    }, delay);
  }

  // Liveness (Task 5.3.1d)

  private updateLiveness(): void {
    const needed = this.state === "open" && this.calls.size > 0;
    if (needed && !this.pingTimer) {
      this.pingTimer = setInterval(() => this.ping(), PING_INTERVAL_MS);
    } else if (!needed && this.pingTimer) {
      clearInterval(this.pingTimer);
      this.pingTimer = null;
    }
  }

  private ping(): void {
    const ws = this.ws;
    if (!ws || this.state !== "open" || this.pongTimer) return;
    this.sendRaw(encodeFrame({ type: "PING", id: 0 }));
    this.pongTimer = setTimeout(() => {
      this.pongTimer = null;
      this.dropSocket(ws, true);
    }, PONG_DEADLINE_MS);
  }

  private clearPong(): void {
    if (this.pongTimer) clearTimeout(this.pongTimer);
    this.pongTimer = null;
  }

  private clearLiveness(): void {
    this.clearPong();
    if (this.pingTimer) clearInterval(this.pingTimer);
    this.pingTimer = null;
  }

  private readonly onWake = (): void => {
    if (typeof document !== "undefined" && document.visibilityState === "hidden") return;
    this.ping();
  };

  private addPageListeners(): void {
    if (this.listening || typeof window === "undefined") return;
    this.listening = true;
    document.addEventListener("visibilitychange", this.onWake);
    window.addEventListener("online", this.onWake);
  }

  private removePageListeners(): void {
    if (!this.listening) return;
    this.listening = false;
    document.removeEventListener("visibilitychange", this.onWake);
    window.removeEventListener("online", this.onWake);
  }

  stream(): Promise<never> {
    return Promise.reject(new ConnectError("UnaryWsTransport does not support streaming", Code.Unimplemented));
  }
}

export function createUnaryWsTransport(opt: UnaryWsTransportOptions): UnaryWsTransport {
  return new UnaryWsTransportImpl(opt);
}
