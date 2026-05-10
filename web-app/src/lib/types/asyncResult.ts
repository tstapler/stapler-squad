/**
 * Shared loading/error shape returned by async hooks.
 *
 * Hooks that expose `loading` and `error` fields should include this interface
 * in their return type so generic loading and error-display components can
 * depend on a single contract rather than re-declaring the same fields.
 */
export interface AsyncResult {
  loading: boolean;
  error: Error | null;
}
