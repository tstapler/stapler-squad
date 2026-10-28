import { lastClassOfMethod } from "@/lib/api/requestPriority";
import { observeQueuedRequests } from "./resourceTiming";

/** Runtime-opt-in diagnostics surface at `window.__ssqPerf`; absent unless `?ssqperf=1|trace` or `localStorage["ssq:perf"]` is set. */
export interface SsqPerf {
  /** Enables `traceCall` stacks; set by opt-in value "trace". */
  trace: boolean;
  inFlight: number;
  peakInFlight: number;
  queuedOver1s: number;
  /** `queuedOver1s` split by request priority class (unattributed entries, e.g. non-RPC fetches, count under `critical`). */
  queuedOver1sByClass: { critical: number; deferred: number };
  /** Calls whose method+body matched a request already in flight, by method name. */
  dupCount: Record<string, number>;
  /** Unary calls that rejected before the browser started the request (status 0). */
  status0: number;
  /** Filled by the WS transport (Task 5.4.1b); always empty/0 before it exists. */
  wsQueueMs: number[];
  wsQueuedOver1s: number;
  /** Max messages ever queued in one `fromWebSocket` streaming reader (instrumentation only; no bound). */
  wsReaderQueueHighWatermark: number;
}

declare global {
  interface Window {
    __ssqPerf?: SsqPerf;
  }
}

const STORAGE_KEY = "ssq:perf";
const QUERY_PARAM = "ssqperf";

function optInValue(): string | null {
  if (typeof window === "undefined") return null;
  try {
    const fromQuery = new URLSearchParams(window.location.search).get(QUERY_PARAM);
    if (fromQuery) return fromQuery;
    return window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

/** Creates `window.__ssqPerf` when opted in; idempotent. Returns undefined (and creates nothing) otherwise. */
export function installSsqPerf(): SsqPerf | undefined {
  if (typeof window === "undefined") return undefined;
  if (window.__ssqPerf) return window.__ssqPerf;
  const value = optInValue();
  if (!value || value === "0") return undefined;
  const perf: SsqPerf = {
    trace: value === "trace",
    inFlight: 0,
    peakInFlight: 0,
    queuedOver1s: 0,
    queuedOver1sByClass: { critical: 0, deferred: 0 },
    dupCount: {},
    status0: 0,
    wsQueueMs: [],
    wsQueuedOver1s: 0,
    wsReaderQueueHighWatermark: 0,
  };
  window.__ssqPerf = perf;
  observeQueuedRequests((entry) => {
    perf.queuedOver1s++;
    perf.queuedOver1sByClass[lastClassOfMethod(methodOf(entry.name ?? "")) ?? "critical"]++;
  });
  return perf;
}

/** Raises `__ssqPerf.wsReaderQueueHighWatermark` to `depth` when opted in; no-op otherwise. */
export function recordReaderQueueDepth(depth: number): void {
  const perf = typeof window === "undefined" ? undefined : window.__ssqPerf;
  if (perf && depth > perf.wsReaderQueueHighWatermark) perf.wsReaderQueueHighWatermark = depth;
}

/** Logs the call stack for `label` when `__ssqPerf.trace` is on; no-op otherwise. */
export function traceCall(label: string): void {
  if (typeof window === "undefined" || !window.__ssqPerf?.trace) return;
  console.trace(`[ssqPerf] ${label}`);
}

function methodOf(input: RequestInfo | URL): string {
  const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
  return url.split("?")[0].split("/").pop() || url;
}

function bodyKey(body: BodyInit | null | undefined): string {
  if (body == null) return "";
  if (typeof body === "string") return body;
  if (ArrayBuffer.isView(body)) return Array.from(new Uint8Array(body.buffer, body.byteOffset, body.byteLength)).join(",");
  if (body instanceof ArrayBuffer) return Array.from(new Uint8Array(body)).join(",");
  return "";
}

/**
 * Wraps a fetch so it feeds the in-flight gauge when opted in; resolves `fetchImpl` per call
 * (default globalThis.fetch) and passes straight through when `__ssqPerf` is absent.
 */
export function withInFlightGauge(fetchImpl?: typeof fetch): typeof fetch {
  const wrapped = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const impl = fetchImpl ?? globalThis.fetch;
    const perf = typeof window === "undefined" ? undefined : window.__ssqPerf;
    if (!perf) return impl(input, init);
    const method = methodOf(input);
    const key = `${method}|${bodyKey(init?.body)}`;
    const dup = (activeKeys.get(key) ?? 0) > 0;
    activeKeys.set(key, (activeKeys.get(key) ?? 0) + 1);
    if (dup) perf.dupCount[method] = (perf.dupCount[method] ?? 0) + 1;
    perf.inFlight++;
    perf.peakInFlight = Math.max(perf.peakInFlight, perf.inFlight);
    const done = () => {
      perf.inFlight--;
      const left = (activeKeys.get(key) ?? 1) - 1;
      if (left <= 0) activeKeys.delete(key);
      else activeKeys.set(key, left);
    };
    return impl(input, init).then(
      (res) => {
        done();
        return res;
      },
      (err) => {
        perf.status0++;
        done();
        throw err;
      }
    );
  };
  return wrapped as typeof fetch;
}

const activeKeys = new Map<string, number>();

installSsqPerf();
