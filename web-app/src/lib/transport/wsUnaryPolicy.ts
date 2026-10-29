"use client";

import type { DescMessage, DescMethodUnary, MessageInitShape } from "@bufbuild/protobuf";
import type { ConnectError, ContextValues, Interceptor, Transport, UnaryResponse } from "@connectrpc/connect";
import { Code, createContextValues } from "@connectrpc/connect";
import { IDEMPOTENT_METHODS } from "@/lib/api/idempotentMethods";
import { peekFlags, preloadFlagValue } from "@/lib/features/flagStore";
import { trackSharedEvent } from "@/lib/telemetry/rpcTiming";
import {
  frameWrittenKey,
  UnaryWsBreaker,
  WsUnaryError,
  type UnaryWsTransport,
} from "./unary-ws-transport";

/**
 * Chooses WebSocket or HTTP per unary call (Story 5.4.1, ADR-001..003). This is the
 * innermost layer of getConnectTransport(): interceptors and the default deadline wrap
 * it once, so both paths share them.
 */

export const WS_CALL_TIMEOUT_MS = 8_000;
export const FORCE_HTTP_KEY = "ssq:ws-unary:force-http";
export const MASTER_FLAG = "unary_over_ws";
export const REMOTE_FLAG = "unary_over_ws_remote";
const DEFAULT_TIMEOUT_MS = 30_000;
const WS_METHOD_NOT_ALLOWED = "ws_method_not_allowed";
const WS_ELIGIBLE = new Set<string>(IDEMPOTENT_METHODS.wsEligible);
const REPLAYABLE = new Set<string>(IDEMPOTENT_METHODS.replayable);

export type FallbackReason = "breaker-open" | "force-http" | "ws-method-not-allowed" | "ws-call-timeout";

// A browser WebSocket hides a rejected handshake's 401, so a 401 seen over HTTP is the only signal.
let authRequired = false;
export function markAuthRequired(): void {
  authRequired = true;
}
export function isAuthRequired(): boolean {
  return authRequired;
}
export function _resetAuthRequiredForTesting(): void {
  authRequired = false;
}

/** Records a 401 so failed WS opens in the same page life stop counting toward the breaker. */
export function createAuthRequiredInterceptor(): Interceptor {
  return (next) => async (req) => {
    try {
      return await next(req);
    } catch (err) {
      if ((err as ConnectError | undefined)?.code === Code?.Unauthenticated) markAuthRequired();
      throw err;
    }
  };
}

/** Breaker that ignores failures while the page is waiting on login (the failure is a 401, not a network fault). */
export class AuthAwareBreaker extends UnaryWsBreaker {
  override recordFailure(): void {
    if (!authRequired) super.recordFailure();
  }
}

/** `remote` iff the page is served over https (protocol, never port). */
export function currentOriginClass(): "local" | "remote" {
  return typeof location !== "undefined" && location.protocol === "https:" ? "remote" : "local";
}

/** Server value once flags loaded (absent = OFF, so an old server degrades to HTTP), else the persisted last-known value. */
export function readWsFlag(name: string): boolean {
  const loaded = peekFlags();
  if (loaded) return loaded.find((f) => f.name === name)?.enabled ?? false;
  return preloadFlagValue(name) ?? false;
}

function forceHttpSet(): boolean {
  try {
    return typeof localStorage !== "undefined" && localStorage.getItem(FORCE_HTTP_KEY) !== null;
  } catch {
    return false;
  }
}

function recordFallback(reason: FallbackReason): void {
  trackSharedEvent({ name: "ws_unary_fallback_total", category: "performance", labels: { reason } });
}

function recordQueueMs(ms: number): void {
  const perf = typeof window === "undefined" ? undefined : window.__ssqPerf;
  if (!perf) return;
  perf.wsQueueMs.push(ms);
  if (ms > 1000) perf.wsQueuedOver1s++;
}

export interface WsUnaryPolicyOptions {
  http: Transport;
  /** Built lazily so HTTP-only sessions never construct a socket. */
  getWs: () => UnaryWsTransport;
  breaker: UnaryWsBreaker;
  /** Test seams. */
  readFlag?: (name: string) => boolean;
  originClass?: () => "local" | "remote";
  isForceHttp?: () => boolean;
  onFallback?: (reason: FallbackReason) => void;
  isReplayable?: (methodName: string) => boolean;
  now?: () => number;
}

class WsUnaryPolicy implements Transport {
  private readonly denylist = new Set<string>();
  private parked = false;
  private readonly readFlag: (name: string) => boolean;
  private readonly originClass: () => "local" | "remote";
  private readonly isForceHttp: () => boolean;
  private readonly onFallback: (reason: FallbackReason) => void;
  private readonly now: () => number;

  constructor(private readonly opt: WsUnaryPolicyOptions) {
    this.readFlag = opt.readFlag ?? readWsFlag;
    this.originClass = opt.originClass ?? currentOriginClass;
    this.isForceHttp = opt.isForceHttp ?? forceHttpSet;
    this.onFallback = opt.onFallback ?? recordFallback;
    this.now = opt.now ?? Date.now;
  }

  stream: Transport["stream"] = (...args) => this.opt.http.stream(...args);

  unary<I extends DescMessage, O extends DescMessage>(
    method: DescMethodUnary<I, O>,
    signal: AbortSignal | undefined,
    timeoutMs: number | undefined,
    header: HeadersInit | undefined,
    input: MessageInitShape<I>,
    contextValues?: ContextValues
  ): Promise<UnaryResponse<I, O>> {
    const viaHttp = (timeout = timeoutMs) =>
      this.opt.http.unary(method, signal, timeout, header, input, contextValues);
    if (!this.wsAllowed(method.name)) return viaHttp();
    if (this.denylist.has(method.name)) {
      this.onFallback("ws-method-not-allowed");
      return viaHttp();
    }
    if (this.isForceHttp()) {
      this.onFallback("force-http");
      return viaHttp();
    }
    if (this.opt.breaker.isOpen()) {
      this.onFallback("breaker-open");
      return viaHttp();
    }
    return this.viaWs(method, signal, timeoutMs, header, input, contextValues, viaHttp);
  }

  /** Flag, origin and eligibility gates; routine HTTP routing, not counted as a fallback. */
  private wsAllowed(methodName: string): boolean {
    if (this.parked || !WS_ELIGIBLE.has(methodName) || !this.readFlag(MASTER_FLAG)) return false;
    return this.originClass() === "local" || this.readFlag(REMOTE_FLAG);
  }

  private async viaWs<I extends DescMessage, O extends DescMessage>(
    method: DescMethodUnary<I, O>,
    signal: AbortSignal | undefined,
    timeoutMs: number | undefined,
    header: HeadersInit | undefined,
    input: MessageInitShape<I>,
    contextValues: ContextValues | undefined,
    viaHttp: (timeout?: number) => Promise<UnaryResponse<I, O>>
  ): Promise<UnaryResponse<I, O>> {
    const decidedAt = this.now();
    const ctx = (contextValues ?? createContextValues()).set(frameWrittenKey, () =>
      recordQueueMs(this.now() - decidedAt)
    );
    const ac = new AbortController();
    const relay = () => ac.abort(signal?.reason);
    if (signal?.aborted) relay();
    signal?.addEventListener("abort", relay, { once: true });
    const ws = this.opt.getWs();
    const call = ws.unary(method, ac.signal, timeoutMs, header, input, ctx);
    let timer: ReturnType<typeof setTimeout> | undefined;
    const timedOut = Symbol("ws-call-timeout");
    const guard: Promise<UnaryResponse<I, O> | typeof timedOut> = (this.opt.isReplayable ?? ((n) => REPLAYABLE.has(n)))(method.name)
      ? Promise.race([call, new Promise<typeof timedOut>((r) => (timer = setTimeout(() => r(timedOut), WS_CALL_TIMEOUT_MS)))])
      : call;
    try {
      const res = await guard;
      if (res !== timedOut) return res;
    } catch (err) {
      return this.onWsError(err, method.name, ws, viaHttp);
    } finally {
      clearTimeout(timer);
      signal?.removeEventListener("abort", relay);
    }
    ac.abort();
    call.catch(() => undefined);
    this.onFallback("ws-call-timeout");
    return viaHttp(Math.max(1, (timeoutMs ?? DEFAULT_TIMEOUT_MS) - WS_CALL_TIMEOUT_MS));
  }

  private onWsError<T>(
    err: unknown,
    methodName: string,
    ws: UnaryWsTransport,
    viaHttp: () => Promise<T>
  ): Promise<T> {
    if (err instanceof WsUnaryError) {
      if (err.marker === WS_METHOD_NOT_ALLOWED) {
        this.denylist.add(methodName);
        this.onFallback("ws-method-not-allowed");
        return viaHttp();
      }
      if (err.code === Code.Unauthenticated) {
        // Session expired mid-socket: the outer auth interceptor redirects; stop using WS until reload.
        this.parked = true;
        ws.close();
      }
    }
    return Promise.reject(err);
  }
}

export function createWsUnaryPolicyTransport(opt: WsUnaryPolicyOptions): Transport {
  return new WsUnaryPolicy(opt);
}
