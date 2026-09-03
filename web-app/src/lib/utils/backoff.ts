import { Code, ConnectError } from "@connectrpc/connect";

/**
 * Full-jitter exponential backoff delay.
 * Formula: Math.random() * Math.min(capMs, baseMs * 2^attempt)
 * Recommended by the AWS architecture blog to prevent thundering-herd bursts.
 */
export function jitteredDelay(baseMs: number, capMs: number, attempt: number): number {
  const ceiling = Math.min(capMs, baseMs * Math.pow(2, attempt));
  return Math.random() * ceiling;
}

/**
 * Stateful backoff tracker that increments the attempt counter on each call to next().
 */
export class BackoffState {
  private _attempt = 0;

  constructor(
    private readonly baseMs: number,
    private readonly capMs: number
  ) {}

  get attempt(): number {
    return this._attempt;
  }

  /** Returns the next jittered delay in milliseconds and increments the attempt counter. */
  next(): number {
    const delay = jitteredDelay(this.baseMs, this.capMs, this._attempt);
    this._attempt++;
    return delay;
  }

  /** Resets the attempt counter to 0. */
  reset(): void {
    this._attempt = 0;
  }
}

// ---------------------------------------------------------------------------
// WebSocket close-code helpers
// ---------------------------------------------------------------------------

/**
 * WebSocket close codes that should NOT trigger a reconnect.
 *   4001 — authentication failure (reconnecting won't help)
 *   4004 — session not found (session is gone; reconnecting is pointless)
 */
export const NON_RETRIABLE_WS_CODES = new Set([4001, 4004]);

/**
 * Returns true if the given WebSocket close code should trigger a reconnect attempt.
 */
export function isRetriableCloseCode(code: number): boolean {
  return !NON_RETRIABLE_WS_CODES.has(code);
}

/**
 * Extracts the `ws-close-code` header value from a ConnectError, returning the
 * numeric code or null if the error is not a ConnectError or the header is absent.
 */
export function getWsCloseCode(err: unknown): number | null {
  if (!(err instanceof ConnectError)) return null;
  const raw = err.metadata?.get("ws-close-code") ?? "";
  const code = parseInt(raw, 10);
  return isNaN(code) ? null : code;
}

/**
 * Returns true if `err` represents a non-retriable failure, across BOTH the
 * WS-bridge transport (createWatchTransport) and the native ConnectRPC
 * transport (createConnectTransport).
 *
 * The WS-bridge signals this via a `ws-close-code` header (see getWsCloseCode);
 * createConnectTransport never sets that header, so it's checked first and,
 * when absent, falls back to comparing the ConnectError's own `code` against
 * the native transport's equivalent signal for the same underlying
 * server-side conditions (auth failure, session gone). Mirrors
 * NON_RETRIABLE_WS_CODES' semantics:
 *   Code.Unauthenticated — equivalent to ws-close-code 4001 (auth failure)
 *   Code.NotFound        — equivalent to ws-close-code 4004 (session not found)
 *
 * The `Code.*` comparison is deferred until this function actually runs (not
 * hoisted into a module-level constant) so importing this module never
 * touches the `Code` enum eagerly — several existing tests mock
 * `@connectrpc/connect` with only a subset of its exports (e.g. just
 * `createClient`), and a top-level `Code.Unauthenticated` reference would
 * throw at import time under those mocks even though the mocked test never
 * exercises this branch.
 *
 * Code.FailedPrecondition covers the WebSocket EndStream-error path
 * specifically (no ws-close-code header, since the connection wasn't
 * abnormally closed) — server/services/connectrpc_websocket.go's
 * sendEndStreamError uses it for a session whose backing tmux working
 * directory no longer exists (e.g. a pruned git worktree). Reconnecting
 * would just repeat the identical failure, so this is treated the same as
 * an auth failure or a gone session: stop retrying immediately instead of
 * burning the full reconnect budget.
 */
export function isNonRetriableConnectError(err: unknown): err is ConnectError {
  if (!(err instanceof ConnectError)) return false;
  const wsCode = getWsCloseCode(err);
  if (wsCode !== null) {
    return !isRetriableCloseCode(wsCode);
  }
  return err.code === Code.Unauthenticated || err.code === Code.NotFound || err.code === Code.FailedPrecondition;
}

/**
 * Returns true if `err` is specifically the "session's working directory no
 * longer exists" stream failure (server/services/connectrpc_websocket.go's
 * handleTmuxRestoreFailure sends Code.FailedPrecondition only for this
 * case — see isNonRetriableConnectError above). Lets a caller show a more
 * actionable message than the generic hard-failed banner: reconnecting the
 * terminal stream can never fix this on its own, but the session-level
 * "Retry now" action re-creates the worktree before restarting.
 */
export function isWorktreeMissingError(err: unknown): err is ConnectError {
  return err instanceof ConnectError && err.code === Code.FailedPrecondition;
}

// ---------------------------------------------------------------------------
// Connect-timeout policy: per-attempt duration cap (foreground vs. background),
// distinct from BackoffState's delay-between-attempts above.
// ---------------------------------------------------------------------------

/**
 * Fast connect-timeout for the first FOREGROUND_FAST_ATTEMPTS attempts since a
 * terminal became foreground. Unvalidated starting guess (herdr-web's real value
 * isn't inspectable) — validate against real p95/p99 connect-to-first-message
 * latency on VPN/high-RTT links before enabling NEXT_PUBLIC_RECONNECT_V2 broadly.
 */
export const FOREGROUND_CONNECT_TIMEOUT_MS = 1200;

/** Normal connect-timeout: background attempts, and foreground attempts beyond FOREGROUND_FAST_ATTEMPTS. */
export const CONNECT_TIMEOUT_MS = 3500;

/** Number of connect attempts (since the most recent foreground transition) eligible for the fast timeout. */
export const FOREGROUND_FAST_ATTEMPTS = 2;

/**
 * Returns the connect-timeout (ms) for a reconnect attempt: the maximum time
 * to wait for the first stream message before abandoning the attempt.
 * Foreground terminals get a shorter timeout for their first
 * FOREGROUND_FAST_ATTEMPTS attempts since becoming foreground; all other
 * attempts (background, or foreground beyond the fast window) use the
 * normal timeout.
 */
export function connectTimeoutMs(foreground: boolean, attemptsSinceForeground: number): number {
  if (foreground && attemptsSinceForeground < FOREGROUND_FAST_ATTEMPTS) {
    return FOREGROUND_CONNECT_TIMEOUT_MS;
  }
  return CONNECT_TIMEOUT_MS;
}
