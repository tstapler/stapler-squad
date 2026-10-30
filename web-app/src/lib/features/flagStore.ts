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
/** Renderer choice is read synchronously at terminal mount, so it needs a persisted pre-load value. */
export const GHOSTTY_RENDERER_FLAG = "ghostty_renderer";

// `etag` is the server validator for `flags`; empty when the server sent none or a local write made the body differ.
let cached: { at: number; flags: FeatureFlag[]; etag: string } | null = null;
let inFlight: Promise<FeatureFlag[]> | null = null;
// Bumped on every invalidation so a response that started before a write cannot overwrite the write.
let generation = 0;

const listeners = new Set<() => void>();
let timer: ReturnType<typeof setInterval> | null = null;

function notify(): void {
  for (const l of [...listeners]) l();
}

/**
 * Subscribes to store changes (a landed load or a recorded write). While at least one
 * subscriber exists, one shared timer re-reads every TTL so a kill switch reaches an open page.
 */
export function subscribeFlags(listener: () => void): () => void {
  listeners.add(listener);
  if (!timer) {
    timer = setInterval(() => void loadFlags(Date.now, { force: true }).catch(() => {}), CACHE_TTL_MS);
  }
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0 && timer) {
      clearInterval(timer);
      timer = null;
    }
  };
}

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
    // Absent or OFF must clear the key so a disabled flag reverts to xterm.js on the next load.
    const ghostty = flags.find((f) => f.name === GHOSTTY_RENDERER_FLAG);
    if (ghostty?.enabled) ls.setItem(PERSIST_PREFIX + GHOSTTY_RENDERER_FLAG, "1");
    else ls.removeItem(PERSIST_PREFIX + GHOSTTY_RENDERER_FLAG);
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
  if (name === GHOSTTY_RENDERER_FLAG) {
    try {
      return storage()?.getItem(PERSIST_PREFIX + name) === "1";
    } catch {
      return false;
    }
  }
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

/** `force` skips the TTL (explicit reload, periodic refresh) but still joins an in-flight read. */
export async function loadFlags(
  now: () => number = Date.now,
  opts?: { force?: boolean },
): Promise<FeatureFlag[]> {
  traceCall("flagStore.loadFlags");
  if (!opts?.force && cached && now() - cached.at < CACHE_TTL_MS) return cached.flags;
  if (!inFlight) {
    const startedAt = generation;
    const client = createClient(SessionService, getConnectTransport());
    const fetchFlags = (ifNoneMatch: string) =>
      // Shared across consumers, so a per-component abort would cancel others' read; bound it by time instead.
      client.getFeatureFlags({ ifNoneMatch }, { signal: AbortSignal.timeout(FETCH_TIMEOUT_MS) });
    inFlight = fetchFlags(cached?.etag ?? "")
      .then(async (res) => {
        if (res.notModified) {
          if (cached) {
            // Unchanged: keep the held body (same reference, so no re-render); extend the TTL only if no write intervened.
            if (startedAt === generation) cached = { ...cached, at: now() };
            return cached.flags;
          }
          // Nothing held to keep (cleared mid-flight): ask again without a validator.
          res = await fetchFlags("");
          // An empty validator can never match; a server claiming it did is misbehaving, so keep what we hold.
          if (res.notModified) return peekFlags() ?? [];
        }
        if (startedAt !== generation) return res.flags;
        cached = { at: now(), flags: res.flags, etag: res.etag };
        persistFlags(res.flags);
        notify();
        return res.flags;
      })
      .finally(() => {
        if (startedAt === generation) inFlight = null;
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
  cached = { at: Number.NEGATIVE_INFINITY, flags, etag: "" };
  inFlight = null;
  generation++;
  notify();
}

/** Clears cache and in-flight state (tests, and after a flag flip a caller knows about). */
export function resetFlagStoreForTest(): void {
  cached = null;
  inFlight = null;
  generation++;
  listeners.clear();
  if (timer) clearInterval(timer);
  timer = null;
}
