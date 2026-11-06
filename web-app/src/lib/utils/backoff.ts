import { Code, ConnectError } from "@connectrpc/connect";

import { POLICY_VIOLATION } from "../transport/terminal-socket-pool";

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
 * Maps a stream that never got a 200 handshake to a Connect code. A pooled stream refused by
 * the server's per-OPEN auth re-check closes with 1008 before any header arrives (status
 * parses as -1); treat it like a 401 so it is non-retriable and surfaces the login UX.
 */
export function connectCodeForRejectedStream(statusCode: number, wsCloseCode: number | null): Code {
  return statusCode === 401 || wsCloseCode === POLICY_VIOLATION ? Code.Unauthenticated : Code.Unknown;
}

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
 * Deliberately does NOT include Code.FailedPrecondition: this helper is
 * shared by the session-list "watch" stream (useSessionService.ts) as well
 * as the terminal stream, and FailedPrecondition's one current meaning
 * (see isWorktreeMissingError below) is specific to the terminal stream's
 * missing-working-directory case. A future unrelated FailedPrecondition on
 * the watch stream must not silently stop reconnecting there too — callers
 * that need the worktree-missing behavior check isWorktreeMissingError
 * explicitly alongside this function instead.
 */
export function isNonRetriableConnectError(err: unknown): err is ConnectError {
  if (!(err instanceof ConnectError)) return false;
  const wsCode = getWsCloseCode(err);
  if (wsCode !== null) {
    return !isRetriableCloseCode(wsCode);
  }
  return err.code === Code.Unauthenticated || err.code === Code.NotFound;
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
 * Fast connect-timeout, used only for the first FOREGROUND_FAST_ATTEMPTS reconnects after a
 * terminal is foregrounded or the tab/network resumes, where a half-open socket is the likely
 * cause. Unvalidated starting guess: it can misfire on a slow socket open (VPN, load, a
 * 20-pane reattach), which is why initial and background connects use CONNECT_TIMEOUT_MS and
 * every timeout abort is retried with escalating jittered delay and a cap (see below).
 */
export const FOREGROUND_CONNECT_TIMEOUT_MS = 1200;

/** Timeout for initial connects, background panes, and resume attempts beyond FOREGROUND_FAST_ATTEMPTS. */
export const CONNECT_TIMEOUT_MS = 8000;

/** Number of resume reconnects (since the most recent foreground/resume) eligible for the fast timeout. */
export const FOREGROUND_FAST_ATTEMPTS = 2;

/** Consecutive connect-timeout aborts retried on their own schedule before falling into the normal backoff. */
export const CONNECT_TIMEOUT_ABORT_CAP = 5;

/** Delay before retrying after a connect-timeout abort: full jitter, 0.5 s doubling to 5 s. */
export function connectTimeoutAbortDelay(consecutiveAborts: number): number {
  return jitteredDelay(500, 5000, Math.max(0, consecutiveAborts - 1));
}

/** First-retry jitter window (2 s) spreads a mux-drop reattach storm; doubles to the cap. */
export const RECONNECT_BASE_MS = 2000;
/** Retry ceiling for a long outage (server restart): retry forever, never slower than this. */
export const RECONNECT_CAP_MS = 30_000;
/** Consecutive connection failures after which the pane shows "Server unreachable" (retries continue). */
export const GAVE_UP_AFTER_ATTEMPTS = 5;
/** Server-ended closes (error frame, clean EndStream) hard-fail only after this much continuous time:
 * squad answers that way while loading sessions after a restart, which can take minutes. */
export const SERVER_END_GIVE_UP_MS = 10 * 60_000;
/** Close code the mux server sends when it refuses a stream (admission control); retriable, no budget used. */
export const STREAM_REFUSED_CLOSE_CODE = 1013;

/**
 * Returns the connect-timeout (ms) for an attempt: the maximum time to wait for the first
 * stream message. `resumeForeground` is true only for a foreground pane reconnecting after a
 * foreground/visibility/online resume; everything else uses the normal timeout.
 */
export function connectTimeoutMs(resumeForeground: boolean, attemptsSinceForeground: number): number {
  if (resumeForeground && attemptsSinceForeground < FOREGROUND_FAST_ATTEMPTS) {
    return FOREGROUND_CONNECT_TIMEOUT_MS;
  }
  return CONNECT_TIMEOUT_MS;
}
