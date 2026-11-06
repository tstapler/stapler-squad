"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export interface ScrollLoadingPillState {
  visible: boolean;
  stalled: boolean;
  program?: string;
}

const SHOW_DELAY_MS = 150;
const STALL_AFTER_MS = 8000;

/**
 * Story 1.4.5 — scroll-forward loading pill state.
 *
 * Shared by BOTH the tmux-native paged-scrollback trigger and Story 1.4.0's
 * app-forwarded alt-screen triggers (design/ux.md Surface 1), so the pill can't
 * drift out of sync with which trigger fired. Owns its timers and clears them on
 * unmount.
 */
export function useScrollLoadingPill() {
  const [pillState, setPillState] = useState<ScrollLoadingPillState>({ visible: false, stalled: false });
  const showTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stallTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearScrollLoadingTimers = useCallback(() => {
    if (showTimerRef.current) {
      clearTimeout(showTimerRef.current);
      showTimerRef.current = null;
    }
    if (stallTimerRef.current) {
      clearTimeout(stallTimerRef.current);
      stallTimerRef.current = null;
    }
  }, []);

  // Called from every response handler (tmux-native ScrollbackResponse and every
  // AppScrollbackResponse outcome) so the pill never persists past outcome
  // delivery (Task 1.4.5b).
  const clearScrollLoadingState = useCallback(() => {
    clearScrollLoadingTimers();
    setPillState({ visible: false, stalled: false });
  }, [clearScrollLoadingTimers]);

  // Starts the show-delay / stalled timers; the caller then sends the request.
  // `program`, if known, drives the pill's app-forwarded copy; omitted entirely
  // for the tmux-native path.
  const startScrollLoadingTimers = useCallback((program?: string) => {
    clearScrollLoadingTimers();
    showTimerRef.current = setTimeout(() => {
      setPillState({ visible: true, stalled: false, program });
    }, SHOW_DELAY_MS);
    stallTimerRef.current = setTimeout(() => {
      setPillState({ visible: true, stalled: true, program });
    }, STALL_AFTER_MS);
  }, [clearScrollLoadingTimers]);

  useEffect(() => clearScrollLoadingTimers, [clearScrollLoadingTimers]);

  return { pillState, clearScrollLoadingState, startScrollLoadingTimers };
}
