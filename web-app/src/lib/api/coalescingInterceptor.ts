import { clone } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import type { Interceptor, UnaryRequest, UnaryResponse } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { buildDedupeKey, isCoalescible } from "./idempotentMethods";

/** The shared request runs on its own deadline: no single caller's signal may cancel the others. */
const SHARED_TIMEOUT_MS = 30_000;

interface Flight {
  promise: Promise<UnaryResponse>;
  controller: AbortController;
  subscribers: number;
  delivered: boolean;
  globalGen: number;
  sessionGen: number;
}

/** Services whose methods may appear in IDEMPOTENT_METHODS.coalescible (string, so a partial gen mock in a test cannot break module load). */
const INSIGHTS_SERVICE_TYPE_NAME = "session.v1.InsightsService";
const isCoalescingService = (typeName: string) =>
  typeName === SessionService.typeName || typeName === INSIGHTS_SERVICE_TYPE_NAME;

const flights = new Map<string, Flight>();
// Bumped when a write starts and again when it ends, so a read started mid-write never joins a pre-write flight.
let globalGen = 0;
const sessionGens = new Map<string, number>();

export function resetCoalescingForTest(): void {
  for (const f of flights.values()) f.controller.abort();
  flights.clear();
  sessionGens.clear();
  globalGen = 0;
}

/** Session id carried by a request: `sessionId` on most RPCs, `id` on the session-scoped ones (GetVCSStatus). */
function sessionIdOf(message: unknown): string {
  const m = message as { sessionId?: unknown; id?: unknown };
  if (typeof m.sessionId === "string" && m.sessionId) return m.sessionId;
  return typeof m.id === "string" ? m.id : "";
}

function bumpGenerations(sessionId: string): void {
  globalGen++;
  if (sessionId) sessionGens.set(sessionId, (sessionGens.get(sessionId) ?? 0) + 1);
}

function canceled(): ConnectError {
  return new ConnectError("canceled", Code.Canceled);
}

function startFlight(
  key: string,
  req: UnaryRequest,
  sessionId: string,
  next: (r: UnaryRequest) => Promise<UnaryResponse>,
): Flight {
  const controller = new AbortController();
  const timer = setTimeout(
    () => controller.abort(new ConnectError("deadline exceeded", Code.DeadlineExceeded)),
    SHARED_TIMEOUT_MS,
  );
  const flight: Flight = {
    controller,
    subscribers: 0,
    delivered: false,
    globalGen,
    sessionGen: sessionGens.get(sessionId) ?? 0,
    promise: undefined as unknown as Promise<UnaryResponse>,
  };
  flight.promise = next({ ...req, signal: controller.signal }).finally(() => {
    clearTimeout(timer);
    // A newer flight may have replaced this one under the same key.
    if (flights.get(key) === flight) flights.delete(key);
  });
  // Every subscriber observes the rejection itself; this keeps an all-aborted flight from being unhandled.
  flight.promise.catch(() => {});
  flights.set(key, flight);
  return flight;
}

function join(flight: Flight, key: string, req: UnaryRequest): Promise<UnaryResponse> {
  flight.subscribers++;
  return new Promise<UnaryResponse>((resolve, reject) => {
    let settled = false;
    const leave = () => {
      flight.subscribers--;
      req.signal.removeEventListener("abort", onAbort);
    };
    const onAbort = () => {
      if (settled) return;
      settled = true;
      leave();
      if (flight.subscribers === 0) {
        flight.controller.abort();
        if (flights.get(key) === flight) flights.delete(key);
      }
      reject(canceled());
    };
    req.signal.addEventListener("abort", onAbort);
    flight.promise.then(
      (res) => {
        if (settled) return;
        settled = true;
        leave();
        // Messages are mutable: only the first subscriber gets the decoded original.
        const message = flight.delivered ? clone(req.method.output, res.message) : res.message;
        flight.delivered = true;
        resolve({ ...res, message } as UnaryResponse);
      },
      (err) => {
        if (settled) return;
        settled = true;
        leave();
        reject(err);
      },
    );
  });
}

/**
 * Merges identical in-flight idempotent unary reads (see IDEMPOTENT_METHODS) into one request.
 * Not a cache: a settled flight is forgotten, so failures are never replayed.
 */
export function createCoalescingInterceptor(opts: { isEnabled: () => boolean }): Interceptor {
  return (next) => async (req) => {
    if (req.stream) return next(req);
    const unary = req as UnaryRequest;
    const unaryNext = next as (r: UnaryRequest) => Promise<UnaryResponse>;
    const sessionId = sessionIdOf(unary.message);

    if (!isCoalescingService(unary.service.typeName) || !isCoalescible(unary.method.name)) {
      bumpGenerations(sessionId);
      try {
        return await unaryNext(unary);
      } finally {
        bumpGenerations(sessionId);
      }
    }
    if (!opts.isEnabled()) return unaryNext(unary);
    if (unary.signal.aborted) throw canceled();

    const key = buildDedupeKey(unary.method, unary.message);
    let flight = flights.get(key);
    const stale =
      flight &&
      (flight.globalGen !== globalGen || (sessionId !== "" && flight.sessionGen !== (sessionGens.get(sessionId) ?? 0)));
    if (!flight || stale) flight = startFlight(key, unary, sessionId, unaryNext);
    return join(flight, key, unary);
  };
}
