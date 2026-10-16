"use client";

import { useCallback, useEffect, useRef } from "react";

/**
 * Coalesces rapid-fire `report(count)` calls into a single `onFlush(total)`
 * call after `windowMs` of inactivity, so a burst of N dropped keystrokes
 * (Story 2.3 — InputDropBadge) produces exactly one coalesced-episode
 * announcement rather than one per dropped keystroke.
 *
 * Internally: a ref-held running total plus a `setTimeout` that (re)schedules
 * on each `report()` call within the window. Once the window elapses without
 * a new `report()`, `onFlush(total)` fires once with the summed count and the
 * total resets to 0 — a `report()` call after that point starts a brand-new,
 * independent episode (design/ux.md §2.3 Case C: "replace, don't merge
 * across episodes").
 *
 * Extracted (Task 2.3.3a) into its own testable unit rather than left inline
 * in `TerminalOutput.tsx` (1500+ lines) — mirrors this plan's existing
 * precedent of pulling coalescing/latch logic out of large files
 * (`answerDialogOnce`, `runInputReadLoop`).
 */
export function useDropEpisodeCoalescer(
  onFlush: (count: number) => void,
  windowMs: number
): (count: number) => void {
  const totalRef = useRef(0);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Keep the latest onFlush without forcing report() to be recreated on
  // every render (mirrors pushMessageRef's "read the ref, not a stale
  // closure" idiom in useTerminalStream.ts).
  const onFlushRef = useRef(onFlush);
  useEffect(() => {
    onFlushRef.current = onFlush;
  }, [onFlush]);

  useEffect(() => {
    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, []);

  const report = useCallback(
    (count: number) => {
      totalRef.current += count;

      if (timerRef.current) {
        clearTimeout(timerRef.current);
      }

      timerRef.current = setTimeout(() => {
        const total = totalRef.current;
        totalRef.current = 0;
        timerRef.current = null;
        onFlushRef.current(total);
      }, windowMs);
    },
    [windowMs]
  );

  return report;
}
