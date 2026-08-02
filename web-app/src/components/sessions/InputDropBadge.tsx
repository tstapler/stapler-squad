"use client";

import { useEffect, useRef, useState } from "react";
import { LiveRegion, useLiveRegion } from "@/components/ui/LiveRegion";
import type { DroppedInputEvent } from "@/lib/hooks/useTerminalStream";
import { badge, icon, text } from "./InputDropBadge.css";

// Task 4.2.1.1 — how long the badge (and its running total) stays visible
// with no further drops before auto-dismissing (ux.md Surface B / AC-VIS-3).
const DWELL_MS = 4000;

interface InputDropBadgeProps {
  droppedInputEvent: DroppedInputEvent | null;
}

function formatMessage(count: number): string {
  const noun = count === 1 ? "keystroke" : "keystrokes";
  return `${count} ${noun} not sent — connection interrupted`;
}

/**
 * Small pill, mounted inside the terminal's own chrome, that surfaces
 * dropped-but-unsent input (AC3's signal half). Owns ALL coalescing across
 * drop occurrences — both the visual running-total pill and the assertive
 * screen-reader announcement — per architecture-review.md's remediation:
 * `useTerminalStream` and `TerminalOutput.tsx` must not compute a running
 * total or call `announce()` themselves. See design/ux.md Surfaces A-D.
 *
 * `runningTotal === 0` is the "not currently showing" state — a fresh
 * episode (no badge visible / prior episode already dismissed) starts a new
 * count from that drop's own `count`; a drop that lands while the badge is
 * still visible accumulates onto the existing total (Surface C).
 */
export function InputDropBadge({ droppedInputEvent }: InputDropBadgeProps) {
  const { message, announce } = useLiveRegion();
  const [runningTotal, setRunningTotal] = useState(0);
  const dwellTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Guards against re-running the coalescing/announce logic for a render
  // that didn't actually introduce a new occurrence (e.g. an unrelated
  // parent re-render with the same droppedInputEvent reference/value).
  const lastHandledAtRef = useRef<number | null>(null);

  const clearDwellTimer = () => {
    if (dwellTimerRef.current !== null) {
      clearTimeout(dwellTimerRef.current);
      dwellTimerRef.current = null;
    }
  };

  // Surfaces A and C — a new distinct droppedInputEvent (by `at`) updates the
  // running total, fires one announcement, and (re)starts the dwell timer.
  useEffect(() => {
    // Loose null check: tolerates callers/mocks that pass `undefined` as
    // well as the documented `null` — this prop should never be strictly
    // required to be exactly `null` when absent.
    if (droppedInputEvent == null) return;
    if (droppedInputEvent.at === lastHandledAtRef.current) return;
    lastHandledAtRef.current = droppedInputEvent.at;

    setRunningTotal((prev) => {
      const next = prev > 0 ? prev + droppedInputEvent.count : droppedInputEvent.count;
      announce(formatMessage(next));
      return next;
    });

    clearDwellTimer();
    dwellTimerRef.current = setTimeout(() => {
      dwellTimerRef.current = null;
      setRunningTotal(0); // Surface B — auto-dismiss, no announcement.
    }, DWELL_MS);
    // `announce` is intentionally omitted: useLiveRegion() returns a new
    // function identity every render, but its behavior only depends on the
    // stable setState dispatcher it closes over, so including it would
    // re-run this effect (and re-announce) on unrelated re-renders while
    // `at` is unchanged — the exact per-render spam AC-SR-3 forbids.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [droppedInputEvent?.at, droppedInputEvent?.count]);

  // Unmount safety (ux.md AC-RESOLVE-2) — clear any pending dwell timer so it
  // never fires a state update against an unmounted component, and no
  // orphaned timer leaks into a later remount for the same session.
  useEffect(() => {
    return () => clearDwellTimer();
  }, []);

  if (runningTotal <= 0) {
    return null;
  }

  const pillMessage = formatMessage(runningTotal);

  return (
    <>
      <div className={badge} title={pillMessage} aria-label={pillMessage}>
        <svg
          className={icon}
          viewBox="0 0 16 16"
          fill="currentColor"
          aria-hidden="true"
        >
          {/* Circle-slash / no-entry glyph */}
          <path d="M8 0a8 8 0 1 0 0 16A8 8 0 0 0 8 0ZM1.5 8a6.5 6.5 0 0 1 10.39-5.2L2.8 11.89A6.47 6.47 0 0 1 1.5 8Zm6.5 6.5a6.47 6.47 0 0 1-3.89-1.3l9.09-9.09A6.5 6.5 0 0 1 8 14.5Z" />
        </svg>
        <span className={text}>{pillMessage}</span>
      </div>
      <LiveRegion message={message} politeness="assertive" role="alert" />
    </>
  );
}
