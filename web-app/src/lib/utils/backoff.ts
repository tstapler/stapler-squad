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
