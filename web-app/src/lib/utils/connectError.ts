import { ConnectError } from "@connectrpc/connect";

/**
 * Converts a thrown value from a ConnectRPC client call into a human-readable
 * message, using `ConnectError.rawMessage` (no `[code]` prefix) instead of the
 * `[code] message`-formatted `.message`. Non-ConnectError Errors fall back to
 * `.message`; anything else (including an empty `rawMessage`) falls back to
 * the caller-supplied fallback string.
 */
export function getErrorMessage(e: unknown, fallback: string): string {
  if (e instanceof ConnectError) return e.rawMessage || fallback;
  if (e instanceof Error) return e.message || fallback;
  return fallback;
}
