/**
 * Converts an RTK Query error value to a standard `Error | null`.
 *
 * RTK Query surfaces errors as an opaque `unknown` value. This helper
 * normalises it into a typed `Error` so callers always receive a consistent
 * `Error | null` shape rather than reimplementing the conversion inline.
 */
export function toErrorOrNull(queryError: unknown): Error | null {
  if (!queryError) return null;
  const msg =
    typeof queryError === "object" && "error" in queryError
      ? String((queryError as { error: unknown }).error)
      : "Unknown error";
  return new Error(msg);
}
