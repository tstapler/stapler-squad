import { Code, ConnectError, createContextKey, createContextValues } from "@connectrpc/connect";
import type { CallOptions, ContextKey, Interceptor, UnaryRequest, UnaryResponse } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";

export type RequestPriority = "critical" | "deferred";

/** Methods that are `deferred` unless a call site says otherwise; everything else is `critical` (fail safe). */
const DEFERRED_METHODS: ReadonlySet<string> = new Set([
  "ListShells",
  "ListWorkflows",
  "GetFileTransportCapabilities",
  "ListProgramsConfig",
  "StatFile",
  "LogClientEvents",
]);

/**
 * Per-call override. GetVCSStatus/ListFiles are critical for the focused pane and deferred otherwise,
 * which only the call site knows (pass `priorityOptions("deferred")`).
 */
export const priorityKey: ContextKey<RequestPriority | undefined> =
  // Guard: this module loads transitively via lib/api/transport, and many Jest suites mock
  // "@connectrpc/connect" with a factory that omits createContextKey (real in production).
  typeof createContextKey === "function"
    ? createContextKey<RequestPriority | undefined>(undefined, { description: "request priority override" })
    : ({ id: Symbol("request priority override"), defaultValue: undefined } as ContextKey<RequestPriority | undefined>);

/** Call option tagging one call with a priority. */
export function priorityOptions(priority: RequestPriority): Pick<CallOptions, "contextValues"> {
  return { contextValues: createContextValues().set(priorityKey, priority) };
}

/** Pass for a call whose pane is not focused/visible; `undefined` leaves the method default. */
export function deferUnless(focused: boolean): Pick<CallOptions, "contextValues"> | undefined {
  return focused ? undefined : priorityOptions("deferred");
}

export function classify(methodName: string, override?: RequestPriority): RequestPriority {
  if (override) return override;
  return DEFERRED_METHODS.has(methodName) ? "deferred" : "critical";
}

/** At most this many deferred calls run at once, leaving >= 3 of the 6 HTTP/1.1 slots for critical ones. */
export const DEFERRED_MAX_CONCURRENT = 3;
const FIRST_PAINT_IDLE_CAP_MS = 2_000;

/** Last class seen per method name; lets the perf gauge attribute a resource-timing entry (URL only) to a class. */
const lastClassByMethod = new Map<string, RequestPriority>();
export function lastClassOfMethod(methodName: string): RequestPriority | undefined {
  return lastClassByMethod.get(methodName);
}

let defaultGate: Promise<void> | null = null;

/** Resolves after the first animation frame once the document has loaded plus an idle moment, never later than 2 s after the first call. */
function firstPaintGate(): Promise<void> {
  if (!defaultGate) {
    defaultGate = new Promise<void>((resolve) => {
      if (typeof window === "undefined") return resolve();
      const timer = setTimeout(resolve, FIRST_PAINT_IDLE_CAP_MS);
      const done = () => {
        clearTimeout(timer);
        resolve();
      };
      const afterLoad = () => {
        const raf = window.requestAnimationFrame ?? ((cb: FrameRequestCallback) => setTimeout(cb, 0));
        raf(() => {
          if (typeof window.requestIdleCallback === "function") {
            window.requestIdleCallback(done, { timeout: FIRST_PAINT_IDLE_CAP_MS });
          } else done();
        });
      };
      if (document.readyState === "complete") afterLoad();
      else window.addEventListener("load", afterLoad, { once: true });
    });
  }
  return defaultGate;
}

/** For tests. */
export function resetRequestPriorityForTest(): void {
  defaultGate = null;
  lastClassByMethod.clear();
}

interface PriorityOptions {
  gate?: () => Promise<void>;
  maxConcurrent?: number;
}

/**
 * Lets `critical` calls pass untouched and holds `deferred` ones until the route's first paint, then runs
 * at most `maxConcurrent` of them at once. Install outermost, once.
 */
export function createPriorityInterceptor(opts: PriorityOptions = {}): Interceptor {
  const gate = opts.gate ?? firstPaintGate;
  const max = opts.maxConcurrent ?? DEFERRED_MAX_CONCURRENT;
  let running = 0;
  const waiting: Array<() => void> = [];

  const acquire = (signal: AbortSignal): Promise<void> => {
    if (running < max) {
      running++;
      return Promise.resolve();
    }
    return new Promise<void>((resolve, reject) => {
      const grant = () => {
        signal.removeEventListener("abort", onAbort);
        resolve();
      };
      const onAbort = () => {
        const i = waiting.indexOf(grant);
        if (i >= 0) waiting.splice(i, 1);
        reject(new ConnectError("canceled", Code.Canceled));
      };
      signal.addEventListener("abort", onAbort, { once: true });
      waiting.push(grant);
    });
  };
  // A freed slot is handed straight to the next waiter, so `running` never dips.
  const release = () => {
    const next = waiting.shift();
    if (next) next();
    else running--;
  };

  return (next) => async (req) => {
    if (req.stream || req.service.typeName !== SessionService.typeName) return next(req);
    const priority = classify(req.method.name, req.contextValues.get(priorityKey));
    lastClassByMethod.set(req.method.name, priority);
    if (priority === "critical") return next(req);

    const unary = req as UnaryRequest;
    await gate();
    if (unary.signal.aborted) throw new ConnectError("canceled", Code.Canceled);
    await acquire(unary.signal);
    try {
      return await (next as (r: UnaryRequest) => Promise<UnaryResponse>)(unary);
    } finally {
      release();
    }
  };
}
