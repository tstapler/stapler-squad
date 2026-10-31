"use client";

import {
  decodeMuxFrame,
  encodeClose,
  encodeCredit,
  encodeData,
  encodeOpen,
  encodePing,
  MUX_KIND_BINARY,
  MUX_KIND_TEXT,
  MUX_PATH,
  type MuxConfig,
} from "./terminal-mux-frame";

/**
 * Terminal socket pool (ADR-033). Callers ask for a stream and get a WebSocket-shaped
 * object; how many physical sockets exist, and which one carries a stream, is the pool's
 * business. Size, assignment policy and the flag are the only knobs.
 */

export const DEFAULT_POOL_SIZE = 2;
export const MAX_POOL_SIZE = 16;
const DEFAULT_CHUNK = 16 * 1024;
const DEFAULT_WINDOW = 256 * 1024;
const PING_INTERVAL_MS = 20_000;
const DEAD_AFTER_MS = 45_000;
const BREAKER_MS = 60_000;
const LOAD_HALF_LIFE_MS = 1_000;
const NORMAL_CLOSE = 1000;
const GOING_AWAY = 1001;
const ABNORMAL_CLOSE = 1006;

const READY_CONNECTING = 0;
const READY_OPEN = 1;
const READY_CLOSING = 2;
const READY_CLOSED = 3;

/** The subset of WebSocket that it-ws and websocket-transport.ts use. */
export interface PooledSocketLike {
  readonly url: string;
  readonly readyState: number;
  readonly CONNECTING: number;
  readonly OPEN: number;
  readonly CLOSING: number;
  readonly CLOSED: number;
  binaryType: string;
  readonly bufferedAmount: number;
  send(data: string | ArrayBufferLike | ArrayBufferView): void;
  close(code?: number, reason?: string): void;
  addEventListener(type: string, listener: (ev: Event) => void, opts?: unknown): void;
  removeEventListener(type: string, listener: (ev: Event) => void): void;
}

export interface TerminalSocketPool {
  /** Opens one terminal stream. Never throws; failures arrive as error/close events. */
  openStream(url: string): PooledSocketLike;
  /** Changes the target socket count at runtime; live streams are never moved or dropped. */
  setSize(n: number): void;
  stats(): PoolStats;
  dispose(): void;
}

export interface PoolStats {
  size: number;
  sockets: { streams: number; draining: boolean; recentBytes: number }[];
}

export interface SocketLoad {
  streams: number;
  /** Output bytes received recently (exponentially decayed), a proxy for how busy the socket is. */
  recentBytes: number;
}

/** Picks the socket for a new stream: an index into `loads`, or -1 to open another socket. Sticky once assigned. */
export interface AssignmentPolicy {
  pick(loads: readonly SocketLoad[], size: number, streamId: number): number;
}

export const leastLoaded: AssignmentPolicy = {
  pick(loads, size) {
    if (loads.length < size) return -1;
    let best = 0;
    for (let i = 1; i < loads.length; i++) {
      const a = loads[i];
      const b = loads[best];
      const ka = Math.floor(a.recentBytes / 4096);
      const kb = Math.floor(b.recentBytes / 4096);
      if (ka < kb || (ka === kb && a.streams < b.streams)) best = i;
    }
    return best;
  },
};

export const hashByStream: AssignmentPolicy = {
  pick(loads, size, streamId) {
    if (loads.length < size) return -1;
    return loads.length === 0 ? -1 : streamId % loads.length;
  },
};

export interface PoolOptions {
  size?: number;
  policy?: AssignmentPolicy;
  /** Creates the physical socket; tests inject a fake. */
  createSocket?: (url: string) => WebSocket;
  now?: () => number;
  setInterval?: (fn: () => void, ms: number) => unknown;
  clearInterval?: (handle: unknown) => void;
  /** Builds the plain per-stream socket used when the pool cannot serve a stream. */
  fallback?: (url: string) => PooledSocketLike;
  /** Mux endpoint URL for a stream URL; defaults to the stream URL's origin plus MUX_PATH. */
  muxUrl?: (streamUrl: string) => string;
}

type Listener = (ev: Event) => void;

function makeEvent(type: string, props: Record<string, unknown> = {}): Event {
  const ev = new Event(type);
  return Object.assign(ev, props);
}

/** One virtual WebSocket carried by a MuxConn. */
class PooledSocket extends EventTarget implements PooledSocketLike {
  readonly CONNECTING = READY_CONNECTING;
  readonly OPEN = READY_OPEN;
  readonly CLOSING = READY_CLOSING;
  readonly CLOSED = READY_CLOSED;
  binaryType = "arraybuffer";
  readonly bufferedAmount = 0;
  private state = READY_CONNECTING;
  private partial: Uint8Array[] = [];
  private partialKind = MUX_KIND_BINARY;
  private consumed = 0;
  private decoder = new TextDecoder();

  constructor(
    readonly url: string,
    readonly id: number,
    private conn: MuxConn,
  ) {
    super();
  }

  get readyState(): number {
    return this.state;
  }

  send(data: string | ArrayBufferLike | ArrayBufferView): void {
    if (this.state !== READY_OPEN) throw new Error("terminal stream is not open");
    const isText = typeof data === "string";
    const bytes = isText
      ? new TextEncoder().encode(data)
      : ArrayBuffer.isView(data)
        ? new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
        : new Uint8Array(data);
    this.conn.sendData(this.id, isText ? MUX_KIND_TEXT : MUX_KIND_BINARY, bytes);
  }

  close(code: number = NORMAL_CLOSE, reason = ""): void {
    if (this.state >= READY_CLOSING) return;
    this.state = READY_CLOSING;
    this.conn.sendClose(this.id, code, reason);
    this.finish(code, reason, true);
  }

  /** @internal called by MuxConn once the physical socket is open and OPEN was sent. */
  markOpen(): void {
    if (this.state !== READY_CONNECTING) return;
    this.state = READY_OPEN;
    this.dispatchEvent(makeEvent("open"));
  }

  /** @internal */
  receive(kind: number, fin: boolean, payload: Uint8Array, window: number): void {
    if (this.state >= READY_CLOSING) return;
    this.partialKind = this.partial.length === 0 ? kind : this.partialKind;
    this.partial.push(payload);
    this.consumed += payload.length;
    if (this.consumed >= window / 4) {
      this.conn.sendCredit(this.id, this.consumed);
      this.consumed = 0;
    }
    if (!fin) return;
    const whole = joinChunks(this.partial);
    this.partial = [];
    const data = this.partialKind === MUX_KIND_TEXT ? this.decoder.decode(whole) : whole.buffer.slice(whole.byteOffset, whole.byteOffset + whole.byteLength);
    this.dispatchEvent(makeEvent("message", { data }));
  }

  /** @internal: server CLOSE or socket loss. */
  finish(code: number, reason: string, wasClean: boolean): void {
    if (this.state === READY_CLOSED) return;
    const wasOpen = this.state !== READY_CONNECTING;
    this.state = READY_CLOSED;
    if (!wasOpen && !wasClean) this.dispatchEvent(makeEvent("error", { error: new Error("terminal mux unavailable") }));
    this.dispatchEvent(makeEvent("close", { code, reason, wasClean }));
    this.conn.forget(this.id);
  }
}

function joinChunks(chunks: Uint8Array[]): Uint8Array {
  if (chunks.length === 1) return chunks[0];
  let n = 0;
  for (const c of chunks) n += c.length;
  const out = new Uint8Array(n);
  let off = 0;
  for (const c of chunks) {
    out.set(c, off);
    off += c.length;
  }
  return out;
}

/** One physical mux socket. */
class MuxConn {
  readonly streams = new Map<number, PooledSocket>();
  draining = false;
  helloSeen = false;
  private ws: WebSocket;
  private opened = false;
  private closed = false;
  private chunk = DEFAULT_CHUNK;
  private window = DEFAULT_WINDOW;
  private bytesEwma = 0;
  private ewmaAt: number;
  private lastRx: number;
  private pingTimer: unknown = null;

  constructor(
    private pool: PoolImpl,
    url: string,
  ) {
    this.ewmaAt = this.lastRx = pool.now();
    this.ws = pool.createSocket(url);
    this.ws.binaryType = "arraybuffer";
    this.ws.addEventListener("open", () => this.onOpen());
    this.ws.addEventListener("message", (ev) => this.onMessage(ev as MessageEvent));
    this.ws.addEventListener("close", () => this.onGone(ABNORMAL_CLOSE, "mux socket closed"));
    this.ws.addEventListener("error", () => undefined);
  }

  get load(): SocketLoad {
    return { streams: this.streams.size, recentBytes: this.decayedBytes() };
  }

  private decayedBytes(): number {
    const now = this.pool.now();
    const decayed = this.bytesEwma * Math.pow(0.5, (now - this.ewmaAt) / LOAD_HALF_LIFE_MS);
    this.bytesEwma = decayed;
    this.ewmaAt = now;
    return decayed;
  }

  attach(id: number, url: string): PooledSocket {
    const s = new PooledSocket(url, id, this);
    this.streams.set(id, s);
    if (this.opened) this.openStream(s);
    return s;
  }

  private openStream(s: PooledSocket): void {
    this.ws.send(encodeOpen(s.id));
    queueMicrotask(() => s.markOpen());
  }

  private onOpen(): void {
    this.opened = true;
    this.pingTimer = this.pool.setIntervalFn(() => this.heartbeat(), PING_INTERVAL_MS);
    for (const s of [...this.streams.values()]) this.openStream(s);
  }

  private heartbeat(): void {
    if (this.pool.now() - this.lastRx > DEAD_AFTER_MS) {
      this.ws.close();
      this.onGone(ABNORMAL_CLOSE, "mux heartbeat timeout");
      return;
    }
    this.ws.send(encodePing());
  }

  private onMessage(ev: MessageEvent): void {
    if (!(ev.data instanceof ArrayBuffer)) return;
    this.lastRx = this.pool.now();
    let frame;
    try {
      frame = decodeMuxFrame(new Uint8Array(ev.data));
    } catch {
      this.ws.close();
      this.onGone(ABNORMAL_CLOSE, "malformed mux frame");
      return;
    }
    switch (frame.type) {
      case "HELLO":
      case "CONFIG":
        this.helloSeen = true;
        this.window = frame.config.window || this.window;
        this.chunk = frame.config.chunk || this.chunk;
        this.pool.onServerConfig(frame.config);
        break;
      case "DATA": {
        this.bytesEwma = this.decayedBytes() + frame.payload.length;
        this.streams.get(frame.id)?.receive(frame.kind, frame.fin, frame.payload, this.window);
        break;
      }
      case "CLOSE":
        this.streams.get(frame.id)?.finish(frame.code, frame.reason, frame.code === NORMAL_CLOSE || frame.code === GOING_AWAY);
        break;
      default:
        break;
    }
  }

  sendData(id: number, kind: number, bytes: Uint8Array): void {
    if (this.closed) return;
    if (bytes.length === 0) {
      this.ws.send(encodeData(id, kind, true, bytes));
      return;
    }
    for (let off = 0; off < bytes.length; off += this.chunk) {
      const end = Math.min(off + this.chunk, bytes.length);
      this.ws.send(encodeData(id, kind, end === bytes.length, bytes.subarray(off, end)));
    }
  }

  sendCredit(id: number, n: number): void {
    if (!this.closed) this.ws.send(encodeCredit(id, n));
  }

  sendClose(id: number, code: number, reason: string): void {
    if (!this.closed && this.opened) this.ws.send(encodeClose(id, code, reason));
  }

  forget(id: number): void {
    this.streams.delete(id);
    this.pool.onStreamEnded(this);
  }

  /** Closes the physical socket once drained. */
  closeIfIdle(): void {
    if (this.streams.size === 0 && !this.closed) {
      this.teardown();
      this.ws.close();
      this.pool.removeConn(this);
    }
  }

  private teardown(): void {
    this.closed = true;
    if (this.pingTimer !== null) this.pool.clearIntervalFn(this.pingTimer);
    this.pingTimer = null;
  }

  private onGone(code: number, reason: string): void {
    if (this.closed) return;
    const neverUsable = !this.helloSeen;
    this.teardown();
    this.pool.removeConn(this);
    if (neverUsable) this.pool.recordMuxFailure();
    for (const s of [...this.streams.values()]) s.finish(code, reason, false);
  }

  dispose(): void {
    this.teardown();
    this.ws.close();
    for (const s of [...this.streams.values()]) s.finish(GOING_AWAY, "pool disposed", true);
  }
}

class PoolImpl implements TerminalSocketPool {
  private conns: MuxConn[] = [];
  private nextId = 1;
  private size: number;
  private sizeOverridden: boolean;
  private breakerUntil = 0;
  private failures = 0;
  readonly now: () => number;
  readonly createSocket: (url: string) => WebSocket;
  private readonly policy: AssignmentPolicy;
  private readonly fallback: (url: string) => PooledSocketLike;
  private readonly muxUrl: (streamUrl: string) => string;
  private readonly setIntervalRef: (fn: () => void, ms: number) => unknown;
  private readonly clearIntervalRef: (handle: unknown) => void;

  constructor(opts: PoolOptions) {
    this.sizeOverridden = opts.size !== undefined;
    this.size = opts.size === 0 ? 0 : clampSize(opts.size ?? DEFAULT_POOL_SIZE);
    this.policy = opts.policy ?? leastLoaded;
    this.now = opts.now ?? Date.now;
    this.createSocket = opts.createSocket ?? ((u) => new WebSocket(u));
    this.fallback = opts.fallback ?? ((u) => new WebSocket(u));
    this.muxUrl = opts.muxUrl ?? defaultMuxUrl;
    // Wrapped so the browser's setInterval is never invoked with the pool as `this`.
    this.setIntervalRef = opts.setInterval ?? ((fn, ms) => setInterval(fn, ms));
    this.clearIntervalRef = opts.clearInterval ?? ((h) => clearInterval(h as number));
  }

  setIntervalFn = (fn: () => void, ms: number): unknown => this.setIntervalRef(fn, ms);
  clearIntervalFn = (h: unknown): void => this.clearIntervalRef(h);

  openStream(url: string): PooledSocketLike {
    if (this.size <= 0 || this.now() < this.breakerUntil) return this.fallback(url);
    const id = this.nextId++;
    const live = this.conns.filter((c) => !c.draining);
    const idx = this.policy.pick(
      live.map((c) => c.load),
      this.size,
      id,
    );
    const conn = idx >= 0 && idx < live.length ? live[idx] : this.addConn(url);
    return conn.attach(id, url);
  }

  private addConn(streamUrl: string): MuxConn {
    const c = new MuxConn(this, this.muxUrl(streamUrl));
    this.conns.push(c);
    return c;
  }

  setSize(n: number): void {
    this.sizeOverridden = true;
    this.applySize(n);
  }

  private applySize(n: number): void {
    this.size = n <= 0 ? 0 : clampSize(n);
    const live = this.conns.filter((c) => !c.draining);
    // Shrink by draining the emptiest sockets first; they close once their streams end.
    const excess = live.length - this.size;
    if (excess > 0) {
      const byLoad = [...live].sort((a, b) => a.streams.size - b.streams.size);
      for (const c of byLoad.slice(0, excess)) {
        c.draining = true;
        c.closeIfIdle();
      }
    }
  }

  /** Server-advertised size applies unless the caller or the user pinned one. */
  onServerConfig(cfg: MuxConfig): void {
    this.failures = 0;
    if (!this.sizeOverridden && cfg.poolSize >= 0) this.applySize(cfg.poolSize);
  }

  onStreamEnded(c: MuxConn): void {
    if (c.draining) c.closeIfIdle();
  }

  removeConn(c: MuxConn): void {
    this.conns = this.conns.filter((x) => x !== c);
  }

  recordMuxFailure(): void {
    this.failures = 0;
    this.breakerUntil = this.now() + BREAKER_MS;
  }

  stats(): PoolStats {
    return {
      size: this.size,
      sockets: this.conns.map((c) => ({ streams: c.streams.size, draining: c.draining, recentBytes: Math.round(c.load.recentBytes) })),
    };
  }

  dispose(): void {
    for (const c of [...this.conns]) c.dispose();
    this.conns = [];
  }
}

function clampSize(n: number): number {
  if (!Number.isFinite(n)) return DEFAULT_POOL_SIZE;
  return Math.min(MAX_POOL_SIZE, Math.max(1, Math.floor(n)));
}

function defaultMuxUrl(streamUrl: string): string {
  const u = new URL(streamUrl);
  return `${u.protocol}//${u.host}${MUX_PATH}`;
}

export function createTerminalSocketPool(opts: PoolOptions = {}): TerminalSocketPool {
  return new PoolImpl(opts);
}
