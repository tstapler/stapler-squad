"use client";

import { useEffect, type DependencyList } from "react";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";

/**
 * Runs `effect(signal)` on mount and whenever `deps` change, cancelling the
 * previous run first (and on unmount) via useAbortableRequest — the common
 * "declare an AbortController ref, wrap the fetch in useCallback, call it
 * from useEffect" boilerplate collapsed into one call.
 *
 * `effect` still must check `signal.aborted` before every state update
 * after an `await` — this hook only owns cancellation timing, not your
 * component's state (same contract as useAbortableRequest). It intentionally
 * does not manage data/loading/error state itself: hooks in this codebase
 * differ too much in error handling (NotFound stops polling, some errors
 * are non-fatal, some fall back to a cached value) for one generic shape to
 * fit without fighting those differences — see useAbortableRequest's doc
 * comment for the lower-level primitive this builds on.
 *
 * Usage:
 *
 *   useAbortableEffect(async (signal) => {
 *     setLoading(true);
 *     try {
 *       const res = await client.getThing({ id }, { signal });
 *       if (signal.aborted) return;
 *       setData(res.thing);
 *     } catch (err) {
 *       if (signal.aborted) return;
 *       setError(err);
 *     } finally {
 *       if (!signal.aborted) setLoading(false);
 *     }
 *   }, [id]);
 */
export function useAbortableEffect(
  effect: (signal: AbortSignal) => Promise<void>,
  deps: DependencyList
): void {
  const start = useAbortableRequest();

  useEffect(() => {
    const signal = start();
    // effect() is documented to handle its own errors; this catch only
    // guards against a caller that doesn't, so a stray rejection doesn't
    // surface as an unhandled promise rejection.
    void effect(signal).catch(() => {});
  // effect and start are intentionally excluded: callers list their own
  // fetch dependencies in `deps`, same convention as every hook this
  // replaces (e.g. useSessionVcs.ts's fetchStatus effect).
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
}
