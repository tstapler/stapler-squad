import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { traceCall } from "@/lib/perf/perfDebug";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { FeatureFlag } from "@/gen/session/v1/session_pb";

/** Module-level GetFeatureFlags store: one 30 s TTL cache and one shared in-flight promise per document. */

const CACHE_TTL_MS = 30_000;
const FETCH_TIMEOUT_MS = 10_000;

/** Pre-load constants: the value before the first GetFeatureFlags returns and no persisted value exists. */
export const REQUEST_COALESCING_PRELOAD = true;
/** Matches `defaultValue` in `knownFeatureFlags`; Story 4.1.6 flips both together. */
export const WINDOW_KEEP_ALIVE_DEFAULT = false;

const PERSIST_PREFIX = "ssq:flag:";
const PERSISTED_WS_FLAGS = ["unary_over_ws", "unary_over_ws_remote"] as const;
const KEEP_ALIVE_FLAG = "window_keep_alive";

let cached: { at: number; flags: FeatureFlag[] } | null = null;
let inFlight: Promise<FeatureFlag[]> | null = null;

function storage(): Storage | null {
  try {
    return typeof localStorage === "undefined" ? null : localStorage;
  } catch {
    return null;
  }
}

function persistFlags(flags: FeatureFlag[]): void {
  const ls = storage();
  if (!ls) return;
  try {
    // A removed or OFF flag must not leave a stale key that arms a boot-time WS open.
    for (const name of PERSISTED_WS_FLAGS) {
      const flag = flags.find((f) => f.name === name);
      if (flag?.enabled) ls.setItem(PERSIST_PREFIX + name, "1");
      else ls.removeItem(PERSIST_PREFIX + name);
    }
    const keepAlive = flags.find((f) => f.name === KEEP_ALIVE_FLAG);
    if (keepAlive) ls.setItem(PERSIST_PREFIX + KEEP_ALIVE_FLAG, keepAlive.enabled ? "1" : "0");
    else ls.removeItem(PERSIST_PREFIX + KEEP_ALIVE_FLAG);
  } catch {
    // Quota or privacy-mode failures only cost the pre-load hint.
  }
}

/**
 * Synchronous pre-load value for a flag that has one (request_coalescing,
 * unary_over_ws, unary_over_ws_remote, window_keep_alive); undefined otherwise.
 */
export function preloadFlagValue(name: string): boolean | undefined {
  if (name === "request_coalescing") return REQUEST_COALESCING_PRELOAD;
  const isWs = (PERSISTED_WS_FLAGS as readonly string[]).includes(name);
  if (!isWs && name !== KEEP_ALIVE_FLAG) return undefined;
  let persisted: string | null = null;
  try {
    persisted = storage()?.getItem(PERSIST_PREFIX + name) ?? null;
  } catch {
    persisted = null;
  }
  if (persisted === "1") return true;
  if (persisted === "0") return false;
  return isWs ? false : WINDOW_KEEP_ALIVE_DEFAULT;
}

/** Last-known flags regardless of TTL (kept across failed refreshes); null before the first success. */
export function peekFlags(): FeatureFlag[] | null {
  return cached ? cached.flags : null;
}

export async function loadFlags(now: () => number = Date.now): Promise<FeatureFlag[]> {
  traceCall("flagStore.loadFlags");
  if (cached && now() - cached.at < CACHE_TTL_MS) return cached.flags;
  if (!inFlight) {
    const client = createClient(SessionService, getConnectTransport());
    inFlight = client
      // Shared across consumers, so a per-component abort would cancel others' read; bound it by time instead.
      .getFeatureFlags({}, { signal: AbortSignal.timeout(FETCH_TIMEOUT_MS) })
      .then((res) => {
        cached = { at: now(), flags: res.flags };
        persistFlags(res.flags);
        return res.flags;
      })
      .finally(() => {
        inFlight = null;
      });
  }
  return inFlight;
}

/**
 * Records a successful flag write: last-known reads see it immediately and the
 * next load refetches instead of serving the pre-write cache for up to 30 s.
 */
export function recordFlagWrite(name: string, enabled: boolean, scope?: string): void {
  if (!cached) return;
  const flags = cached.flags.map((f) => {
    if (f.name !== name) return f;
    if (!scope) return { ...f, enabled } as FeatureFlag;
    return { ...f, scopes: f.scopes.map((s) => (s.scope === scope ? { ...s, enabled } : s)) } as FeatureFlag;
  });
  cached = { at: Number.NEGATIVE_INFINITY, flags };
  inFlight = null;
}

/** Clears cache and in-flight state (tests, and after a flag flip a caller knows about). */
export function resetFlagStoreForTest(): void {
  cached = null;
  inFlight = null;
}
