import { ConnectError } from "@connectrpc/connect";

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

// ---------------------------------------------------------------------------
// Connect-timeout policy (foreground vs. background)
// ---------------------------------------------------------------------------

/**
 * Fast connect-timeout for the first FOREGROUND_FAST_ATTEMPTS attempts since a
 * terminal became foreground. Re-derived from AC2's "~1200-1500ms" range —
 * herdr-web's real TERMINAL_FOREGROUND_CONNECT_TIMEOUT_MS isn't inspectable
 * (different, unvendored repo), so this is an unvalidated starting guess, not
 * a measured value. Validate against real p95/p99 connect-to-first-message
 * latency (esp. VPN/high-RTT links) before enabling NEXT_PUBLIC_RECONNECT_V2
 * broadly.
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
