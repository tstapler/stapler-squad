"use client";

import { useRef, useCallback, useEffect } from "react";
import { dimensionsEqual, type ResizeDimensions } from "@/lib/terminal/types";

const THROTTLE_MS = 200;
// Last BOUNCE_HISTORY_SIZE sizes sent (oldest first; the current lastSentDimsRef is
// excluded, handled by value-dedup separately). Lets resize() catch a repeat of any
// recent size, not just two sends back, since a real oscillating viewport can cycle
// through 3+ values (observed live: 10x6 -> 67x38 -> 67x22 -> 67x38).
const BOUNCE_HISTORY_SIZE = 4;
const BOUNCE_HOLD_BASE_MS = 3000;
const BOUNCE_HOLD_MAX_MS = 15000;

function isDuplicateOfLastSent(
  lastSent: ResizeDimensions | null,
  next: ResizeDimensions
): boolean {
  return lastSent !== null && dimensionsEqual(lastSent, next);
}

function matchesBounceHistory(history: ResizeDimensions[], next: ResizeDimensions): boolean {
  return history.some((d) => dimensionsEqual(d, next));
}

function nextBounceHoldMs(streak: number): number {
  return Math.min(BOUNCE_HOLD_BASE_MS * 2 ** streak, BOUNCE_HOLD_MAX_MS);
}

export interface UseResizeSettlingOptions {
  /**
   * Invoked with a size this boundary has judged settled -- sent immediately,
   * after the trailing-edge throttle, or after an oscillation's escalating
   * hold elapsed. Must return whether the send actually succeeded. A
   * false/throwing send is NOT recorded as the last-sent value or folded
   * into bounce history -- otherwise a failed send would wrongly dedupe or
   * bounce-detect against a size that was never actually delivered.
   */
  onSettled: (cols: number, rows: number) => boolean;
}

export interface UseResizeSettlingResult {
  /**
   * Call on every raw resize signal (e.g. every ResizeObserver tick). Applies
   * value-dedup, a trailing-edge throttle, and bounce/oscillation detection
   * with an escalating hold, then invokes onSettled with a value guaranteed
   * to be settled -- callers downstream of this hook never see a raw,
   * possibly-bouncing size. `force:true` bypasses dedup, throttle, and bounce
   * detection (e.g. a user-initiated explicit resize).
   */
  resize: (cols: number, rows: number, force?: boolean) => void;
}

/**
 * useResizeSettling -- the single "resize settling" boundary for the
 * terminal resize pipeline (BUG-101). Owns debounce, oscillation (bounce)
 * detection, and escalating backoff in one place, consolidating logic that
 * was previously patched independently and repeatedly at the resize
 * send-path (see BUG-101 doc): `20da32505`, `417d370dc`/`e5a42e730`,
 * `9651edd5e`, `e8060fb1b`/`b5dba385e`, and the generalized bounce-history
 * fix. Any future oscillation shape belongs here, not as another special
 * case bolted onto a call site.
 */
export function useResizeSettling({
  onSettled,
}: UseResizeSettlingOptions): UseResizeSettlingResult {
  const lastResizeTimeRef = useRef<number>(0);
  const lastSentDimsRef = useRef<ResizeDimensions | null>(null);
  const sentHistoryRef = useRef<ResizeDimensions[]>([]);
  // Consecutive bounces held back-to-back with no genuinely-new size sent in
  // between. Drives an escalating hold (doubling, capped) so a viewport
  // stuck oscillating for an extended period backs off further apart instead
  // of retrying every flat hold duration -- resets to 0 the moment a resize
  // actually sends (a real new size, or a held one whose hold elapsed).
  const bounceStreakRef = useRef(0);
  const pendingResizeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Cancel any pending deferred resize timer on unmount so it doesn't fire
  // against a torn-down component/connection.
  useEffect(() => {
    return () => {
      if (pendingResizeTimerRef.current) {
        clearTimeout(pendingResizeTimerRef.current);
        pendingResizeTimerRef.current = null;
      }
    };
  }, []);

  const doSend = useCallback(
    (cols: number, rows: number) => {
      const succeeded = onSettled(cols, rows);
      if (!succeeded) return;

      lastResizeTimeRef.current = Date.now();
      if (lastSentDimsRef.current !== null) {
        sentHistoryRef.current.push(lastSentDimsRef.current);
        if (sentHistoryRef.current.length > BOUNCE_HISTORY_SIZE) {
          sentHistoryRef.current.shift();
        }
      }
      lastSentDimsRef.current = { cols, rows };
      bounceStreakRef.current = 0;
    },
    [onSettled]
  );

  // Defers `doSend(dims)` by delayMs, replacing whatever deferred send (if
  // any) was already pending. Shared by the bounce-hold and trailing-edge-
  // throttle branches of resize() below -- the only difference between them
  // is how delayMs and the log line are computed.
  const scheduleDeferredSend = useCallback(
    (dims: ResizeDimensions, delayMs: number) => {
      pendingResizeTimerRef.current = setTimeout(() => {
        pendingResizeTimerRef.current = null;
        doSend(dims.cols, dims.rows);
      }, delayMs);
    },
    [doSend]
  );

  const resize = useCallback(
    (cols: number, rows: number, force: boolean = false) => {
      const next = { cols, rows };

      // Cancel any previously deferred resize -- we have newer dimensions now.
      // This MUST run before the value-dedup early-return below: otherwise a
      // bounce-back call whose dimensions match lastSentDimsRef (e.g. A -> B
      // deferred within the throttle window -> back to A) would dedup-return
      // without cancelling the still-pending deferred send for B, letting
      // that stale send fire later with the wrong dimensions.
      if (pendingResizeTimerRef.current) {
        clearTimeout(pendingResizeTimerRef.current);
        pendingResizeTimerRef.current = null;
      }

      // Value-dedup: skip if this exact (cols, rows) pair was the last one
      // actually sent, independent of (and checked before) the time throttle
      // below. An unchanged value must not keep the throttle window "warm" --
      // lastResizeTimeRef is deliberately left untouched here.
      if (!force && isDuplicateOfLastSent(lastSentDimsRef.current, next)) {
        console.log(`[useResizeSettling] Resize skipped, value unchanged (${cols}x${rows})`);
        return;
      }

      // Bounce detection: matches ANY of the last BOUNCE_HISTORY_SIZE sizes
      // sent, not just the one two sends ago, since a real oscillating
      // viewport can wander through 3+ values on a slower cadence than
      // THROTTLE_MS catches. Held out past an escalating hold instead of
      // sent immediately, coalescing the oscillation into one settled resize.
      if (!force && matchesBounceHistory(sentHistoryRef.current, next)) {
        const holdMs = nextBounceHoldMs(bounceStreakRef.current);
        bounceStreakRef.current += 1;
        console.log(
          `[useResizeSettling] Resize bounce detected (${cols}x${rows} matches recent history), holding ${holdMs}ms (streak ${bounceStreakRef.current})`
        );
        scheduleDeferredSend(next, holdMs);
        return;
      }

      const timeSinceLastResize = Date.now() - lastResizeTimeRef.current;
      if (!force && timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
        // Defer instead of drop: schedule the trailing-edge send so the final
        // settled size always reaches the server after rapid resize sequences.
        const remaining = THROTTLE_MS - timeSinceLastResize;
        console.log(`[useResizeSettling] Resize deferred ${remaining}ms (${cols}x${rows})`);
        scheduleDeferredSend(next, remaining + 1);
        return;
      }

      doSend(cols, rows);
    },
    [doSend, scheduleDeferredSend]
  );

  return { resize };
}
