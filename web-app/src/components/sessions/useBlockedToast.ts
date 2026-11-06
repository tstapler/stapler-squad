"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ScrollBlockedReason } from "@/gen/session/v1/events_pb";
import { DEFAULT_TOAST_MS } from "@/lib/notification-policy";

export interface BlockedToastState {
  reason: ScrollBlockedReason;
  program: string;
  /** Monotonic per toast, so two identical consecutive toasts still re-announce. */
  seq: number;
}

const CONNECTION_PULSE_MS = 600;

/**
 * Story 1.4.3 — blocked-outcome toast plus the ConnectionCountIndicator pulse.
 * Owns its timers and clears them on unmount.
 */
export function useBlockedToast() {
  const [blockedToast, setBlockedToast] = useState<BlockedToastState | null>(null);
  const seqRef = useRef(0);
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [connectionPulse, setConnectionPulse] = useState(false);
  const pulseTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const dismissBlockedToast = useCallback(() => {
    if (toastTimerRef.current) {
      clearTimeout(toastTimerRef.current);
      toastTimerRef.current = null;
    }
    setBlockedToast(null);
  }, []);

  // Keyed on blocked_reason. UNSPECIFIED (a gate failure the client should never
  // actually attempt a request for, per Epic 1.1's eligibility check) falls back
  // to the MULTIPLE_VIEWERS copy as the least-wrong default rather than showing no
  // message — a deliberate P2 tradeoff (pre-mortem.md Failure #2), accepted since
  // UNSPECIFIED should be unreachable in practice.
  const showBlockedOutcome = useCallback((frame: { blockedReason: ScrollBlockedReason; program: string }) => {
    seqRef.current += 1;
    setBlockedToast({ reason: frame.blockedReason, program: frame.program, seq: seqRef.current });
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
    toastTimerRef.current = setTimeout(() => setBlockedToast(null), DEFAULT_TOAST_MS);

    // Pulse ConnectionCountIndicator only for the reason that actually has
    // connected-viewer state to point to (and its UNSPECIFIED fallback, which
    // renders the same copy).
    if (
      frame.blockedReason === ScrollBlockedReason.MULTIPLE_VIEWERS ||
      frame.blockedReason === ScrollBlockedReason.SCROLL_BLOCKED_REASON_UNSPECIFIED
    ) {
      if (pulseTimerRef.current) clearTimeout(pulseTimerRef.current);
      setConnectionPulse(true);
      pulseTimerRef.current = setTimeout(() => setConnectionPulse(false), CONNECTION_PULSE_MS);
    }
  }, []);

  useEffect(() => {
    return () => {
      if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
      if (pulseTimerRef.current) clearTimeout(pulseTimerRef.current);
    };
  }, []);

  return { blockedToast, connectionPulse, showBlockedOutcome, dismissBlockedToast };
}
