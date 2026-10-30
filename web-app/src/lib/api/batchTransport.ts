import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import type { DescMessage, DescMethodUnary, MessageInitShape } from "@bufbuild/protobuf";
import type { ContextValues, Transport, UnaryResponse } from "@connectrpc/connect";
import { Code, ConnectError } from "@connectrpc/connect";
import { codeFromString } from "@connectrpc/connect/protocol-connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { isBatchEligible } from "./idempotentMethods";

/**
 * Transport decorator (flag `batch_unary`): independent cheap reads issued within one
 * short window are sent as a single POST to the server's batch endpoint, freeing
 * HTTP/1.1 connection slots. It is transparent to callers and strictly fail-open:
 * anything the batch cannot serve (flag off, ineligible method, custom headers, a
 * failed or malformed batch, an item the server declined) runs as an ordinary call on
 * `inner`, so behaviour never regresses. Server counterpart: server/services/ws_unary_batch.go.
 */

export const BATCH_FLAG = "batch_unary";
export const BATCH_PATH = "/batch/unary";
export const BATCH_WINDOW_MS = 4;
export const BATCH_MAX_CALLS = 16;
/** Matches the server's 256 KiB cap on the JSON request body. */
export const BATCH_MAX_REQUEST_BYTES = 256 * 1024;
const PER_CALL_OVERHEAD_BYTES = 128; // JSON keys, method path and punctuation around the base64 body
const FETCH_GRACE_MS = 2_000;
const DEFAULT_TIMEOUT_MS = 30_000;

/**
 * Counters that make the batching effect visible: how many eligible calls travelled in a batch and how many
 * were eligible but left the window alone (a batch of one is sent as an ordinary request). Also readable in
 * the console as `window.__ssqBatchStats`.
 */
export const batchStats = { batches: 0, batchedCalls: 0, eligibleAlone: 0 };

export function resetBatchStats(): void {
  batchStats.batches = 0;
  batchStats.batchedCalls = 0;
  batchStats.eligibleAlone = 0;
}

if (typeof window !== "undefined") {
  (window as unknown as { __ssqBatchStats?: typeof batchStats }).__ssqBatchStats = batchStats;
}

export interface BatchTransportOptions {
  inner: Transport;
  baseUrl: string;
  fetch?: typeof fetch;
  /** Read per call, so the flag can be flipped without rebuilding the transport. */
  isEnabled: () => boolean;
  /** True when the call already travels over the multiplexed WebSocket and needs no batching. */
  isWsRouted?: () => boolean;
}

interface Entry {
  id: number;
  method: DescMethodUnary<DescMessage, DescMessage>;
  signal: AbortSignal | undefined;
  timeoutMs: number | undefined;
  header: HeadersInit | undefined;
  input: unknown;
  contextValues: ContextValues | undefined;
  body: Uint8Array;
  resolve: (res: UnaryResponse<DescMessage, DescMessage>) => void;
  reject: (err: unknown) => void;
  settled: boolean;
  cleanup: () => void;
}

interface WireResult {
  id: number;
  body?: string;
  error?: { code: string; message?: string };
  fallback?: boolean;
}

function toBase64(bytes: Uint8Array): string {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

function fromBase64(text: string): Uint8Array {
  const s = atob(text);
  const out = new Uint8Array(s.length);
  for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
  return out;
}

function abortError(signal: AbortSignal): ConnectError {
  const reason = signal.reason;
  return reason instanceof ConnectError ? reason : new ConnectError("canceled", Code.Canceled);
}

function hasHeaders(header: HeadersInit | undefined): boolean {
  if (!header) return false;
  let any = false;
  new Headers(header).forEach(() => (any = true));
  return any;
}

function wireSize(e: Entry): number {
  return Math.ceil((e.body.length * 4) / 3) + PER_CALL_OVERHEAD_BYTES + e.method.name.length;
}

class BatchTransport implements Transport {
  private pending: Entry[] = [];
  private pendingBytes = 0;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private nextId = 1;

  constructor(private readonly opt: BatchTransportOptions) {}

  stream: Transport["stream"] = (...args) => this.opt.inner.stream(...args);

  unary<I extends DescMessage, O extends DescMessage>(
    method: DescMethodUnary<I, O>,
    signal: AbortSignal | undefined,
    timeoutMs: number | undefined,
    header: HeadersInit | undefined,
    input: MessageInitShape<I>,
    contextValues?: ContextValues,
  ): Promise<UnaryResponse<I, O>> {
    const direct = () => this.opt.inner.unary(method, signal, timeoutMs, header, input, contextValues);
    if (!this.eligible(method, header)) return direct();
    if (signal?.aborted) return Promise.reject(abortError(signal));
    return new Promise<UnaryResponse<I, O>>((resolve, reject) => {
      this.enqueue({
        id: this.nextId++,
        method: method as unknown as Entry["method"],
        signal,
        timeoutMs,
        header,
        input,
        contextValues,
        body: toBinary(method.input, create(method.input, input)),
        resolve: resolve as Entry["resolve"],
        reject,
        settled: false,
        cleanup: () => undefined,
      });
    });
  }

  private eligible(method: DescMethodUnary<DescMessage, DescMessage>, header: HeadersInit | undefined): boolean {
    return (
      this.opt.isEnabled() &&
      !(this.opt.isWsRouted?.() ?? false) &&
      method.parent.typeName === SessionService.typeName &&
      isBatchEligible(method.name) &&
      !hasHeaders(header)
    );
  }

  private enqueue(entry: Entry): void {
    const size = wireSize(entry);
    if (this.pending.length > 0 && this.pendingBytes + size > BATCH_MAX_REQUEST_BYTES) this.flush();
    if (size > BATCH_MAX_REQUEST_BYTES) {
      this.runIndividually(entry);
      return;
    }
    this.watchAbort(entry);
    this.pending.push(entry);
    this.pendingBytes += size;
    if (this.pending.length >= BATCH_MAX_CALLS) this.flush();
    else if (!this.timer) this.timer = setTimeout(() => this.flush(), BATCH_WINDOW_MS);
  }

  /** Rejects only this caller on abort; the batch and its other callers carry on. */
  private watchAbort(e: Entry): void {
    const signal = e.signal;
    if (!signal) return;
    const onAbort = () => {
      if (e.settled) return;
      const idx = this.pending.indexOf(e);
      if (idx >= 0) {
        this.pending.splice(idx, 1);
        this.pendingBytes -= wireSize(e);
      }
      this.settle(e, () => e.reject(abortError(signal)));
    };
    signal.addEventListener("abort", onAbort, { once: true });
    e.cleanup = () => signal.removeEventListener("abort", onAbort);
  }

  private settle(e: Entry, fn: () => void): void {
    if (e.settled) return;
    e.settled = true;
    e.cleanup();
    fn();
  }

  private runIndividually(e: Entry): void {
    if (e.settled) return;
    this.opt.inner
      .unary(e.method, e.signal, e.timeoutMs, e.header, e.input as never, e.contextValues)
      .then(
        (res) => this.settle(e, () => e.resolve(res)),
        (err) => this.settle(e, () => e.reject(err)),
      );
  }

  private flush(): void {
    clearTimeout(this.timer);
    this.timer = undefined;
    const batch = this.pending.filter((e) => !e.settled);
    this.pending = [];
    this.pendingBytes = 0;
    if (batch.length === 0) return;
    if (batch.length === 1) {
      batchStats.eligibleAlone++;
      this.runIndividually(batch[0]);
      return;
    }
    batchStats.batches++;
    batchStats.batchedCalls += batch.length;
    if (process.env.NODE_ENV !== "production") console.debug(`[batch] ${batch.length} calls in one request`);
    void this.send(batch);
  }

  private async send(batch: Entry[]): Promise<void> {
    const controller = new AbortController();
    const timeoutMs = Math.max(...batch.map((e) => e.timeoutMs ?? DEFAULT_TIMEOUT_MS)) + FETCH_GRACE_MS;
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    // Aborting every caller of an in-flight batch is the only thing that may cancel it.
    const settledAll = () => batch.every((e) => e.settled);
    const stopIfAbandoned = () => settledAll() && controller.abort();
    for (const e of batch) e.signal?.addEventListener("abort", stopIfAbandoned, { once: true });
    try {
      const results = await this.post(batch, controller.signal);
      for (const e of batch) this.deliver(e, results.get(e.id));
    } catch {
      for (const e of batch) this.runIndividually(e);
    } finally {
      clearTimeout(timer);
      for (const e of batch) e.signal?.removeEventListener("abort", stopIfAbandoned);
    }
  }

  private async post(batch: Entry[], signal: AbortSignal): Promise<Map<number, WireResult>> {
    const doFetch = this.opt.fetch ?? fetch;
    const res = await doFetch(this.opt.baseUrl + BATCH_PATH, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        calls: batch.map((e) => ({
          id: e.id,
          method: `/${e.method.parent.typeName}/${e.method.name}`,
          body: toBase64(e.body),
          timeoutMs: e.timeoutMs ?? DEFAULT_TIMEOUT_MS,
        })),
      }),
      signal,
    });
    if (!res.ok) throw new Error(`batch http ${res.status}`);
    const json = (await res.json()) as { results?: WireResult[] };
    if (!Array.isArray(json.results)) throw new Error("malformed batch response");
    return new Map(json.results.map((r) => [r.id, r]));
  }

  private deliver(e: Entry, r: WireResult | undefined): void {
    if (e.settled) return;
    if (!r || r.fallback) return this.runIndividually(e);
    if (r.error) {
      const err = r.error;
      return this.settle(e, () => e.reject(new ConnectError(err.message ?? "", codeFromString(err.code) ?? Code.Unknown)));
    }
    try {
      const message = fromBinary(e.method.output, fromBase64(r.body ?? ""));
      const res = {
        stream: false,
        service: e.method.parent,
        method: e.method,
        header: new Headers(),
        trailer: new Headers(),
        message,
      } as UnaryResponse<DescMessage, DescMessage>;
      this.settle(e, () => e.resolve(res));
    } catch {
      this.runIndividually(e);
    }
  }
}

export function createBatchTransport(opt: BatchTransportOptions): Transport {
  return new BatchTransport(opt);
}
