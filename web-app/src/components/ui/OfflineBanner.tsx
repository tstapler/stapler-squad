"use client";

import { createContext, useContext, useEffect, useRef, useState } from "react";
import { banner, bannerButton } from "./OfflineBanner.css";

export type ConnectivityBannerState = "offline" | "reconnecting" | "cant_reach" | "loading" | "still_loading";

export const BANNER_COPY: Record<ConnectivityBannerState, string> = {
  offline: "Offline. Showing last known state.",
  reconnecting: "Reconnecting to the server. Showing last known state.",
  cant_reach: "Can't reach the server.",
  loading: "Loading...",
  still_loading: "Still loading.",
};

export const LOADING_DELAY_MS = 150;
export const STILL_LOADING_MS = 8_000;
export const CANT_REACH_MS = 30_000;
export const STILL_SENDING_MS = 10_000;
export const NO_RESPONSE_MS = 30_000;
const BACKOFF_START_MS = 1_000;
const BACKOFF_MAX_MS = 15_000;

export const STILL_SENDING_COPY = "Still sending... this is taking longer than usual.";
export const NO_RESPONSE_COPY =
  "No response from the server. The action may or may not have been applied. Open the session to check.";

/** Auto-retry delay for reconnect attempt `attempt` (0-based): 1 s doubling to 15 s. */
export function backoffDelayMs(attempt: number): number {
  return Math.min(BACKOFF_START_MS * 2 ** attempt, BACKOFF_MAX_MS);
}

/** Clock time for "Showing data from" and "As of" lines. */
export function formatClockTime(ms: number): string {
  return new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

/** True once `active` has stayed true for `ms`; resets the moment it goes false. */
function useActiveFor(active: boolean, ms: number): boolean {
  const [elapsed, setElapsed] = useState(false);
  useEffect(() => {
    if (!active) {
      setElapsed(false);
      return;
    }
    const timer = setTimeout(() => setElapsed(true), ms);
    return () => clearTimeout(timer);
  }, [active, ms]);
  return active && elapsed;
}

export interface OfflineBannerProps {
  /** `navigator.onLine`, or the surface's own verdict that data cannot be reached. */
  online: boolean;
  /** The latest poll or stream failed while the browser is online. */
  failed?: boolean;
  /** A first fetch is in flight. */
  loading?: boolean;
  /** Cached data is on screen, so loading is not a cold start. */
  hasData?: boolean;
  /** Retry now; also drives the 1 s to 15 s auto-retry while reconnecting. */
  onRetry?: () => void;
  /** Hide the button when the surface already shows its own Retry. */
  showRetry?: boolean;
  testId?: string;
}

/**
 * The one connectivity banner (ux.md section 9): a persistent text element, not a live
 * region. The panel's single polite region announces the state change.
 */
export function OfflineBanner({
  online,
  failed = false,
  loading = false,
  hasData = true,
  onRetry,
  showRetry = true,
  testId = "offline-banner",
}: OfflineBannerProps) {
  const coldLoading = online && !failed && loading && !hasData;
  const showLoading = useActiveFor(coldLoading, LOADING_DELAY_MS);
  const stillLoading = useActiveFor(coldLoading, STILL_LOADING_MS);
  const reconnecting = online && failed;
  const cantReach = useActiveFor(reconnecting, CANT_REACH_MS);

  const retryRef = useRef(onRetry);
  retryRef.current = onRetry;
  const autoRetry = reconnecting && !!onRetry;
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (!autoRetry) {
      setAttempt(0);
      return;
    }
    const timer = setTimeout(() => {
      retryRef.current?.();
      setAttempt((a) => a + 1);
    }, backoffDelayMs(attempt));
    return () => clearTimeout(timer);
  }, [autoRetry, attempt]);

  let state: ConnectivityBannerState | null = null;
  if (!online) state = "offline";
  else if (reconnecting) state = cantReach ? "cant_reach" : "reconnecting";
  else if (stillLoading) state = "still_loading";
  else if (showLoading) state = "loading";
  if (!state) return null;

  const retryable = state === "cant_reach" || state === "still_loading";
  return (
    <div className={banner} data-testid={testId} data-state={state}>
      <span>{BANNER_COPY[state]}</span>
      {retryable && showRetry && onRetry && (
        // analytics-exempt
        <button type="button" className={bannerButton} onClick={onRetry}>
          Retry
        </button>
      )}
    </div>
  );
}

/** In-row notice for a request pending 10 s (not announced) and 30 s (Retry on the same idempotency key). */
export function PendingNotice({ pending, onRetry }: { pending: boolean; onRetry: () => void }) {
  const slow = useActiveFor(pending, STILL_SENDING_MS);
  const stuck = useActiveFor(pending, NO_RESPONSE_MS);
  if (!slow) return null;
  return (
    <div data-testid="pending-notice" data-state={stuck ? "no_response" : "still_sending"}>
      <span>{stuck ? NO_RESPONSE_COPY : STILL_SENDING_COPY}</span>
      {stuck && (
        // analytics-exempt
        <button type="button" className={bannerButton} onClick={onRetry}>
          Retry
        </button>
      )}
    </div>
  );
}

/**
 * Why action controls are disabled, or undefined when they may act. Export-only contract
 * for the inline action row (Epic 2): render `aria-disabled` with this string as its reason.
 */
export type ActionsDisabledReason = "Offline" | "Reconnecting" | "Updating...";

export const ActionsDisabledReasonContext = createContext<ActionsDisabledReason | undefined>(undefined);

export function useActionsDisabledReason(): ActionsDisabledReason | undefined {
  return useContext(ActionsDisabledReasonContext);
}

export function actionsDisabledReason(input: {
  online: boolean;
  reconnecting: boolean;
  awaitingFresh: boolean;
}): ActionsDisabledReason | undefined {
  if (!input.online) return "Offline";
  if (input.reconnecting) return "Reconnecting";
  if (input.awaitingFresh) return "Updating...";
  return undefined;
}

/**
 * After `unavailable` ends, stays true until `refreshedAt` moves past its value from when
 * the outage began, so a stale card cannot act on an item resolved meanwhile.
 */
export function useAwaitingFresh(unavailable: boolean, refreshedAt: number | null): boolean {
  const [outage, setOutage] = useState<{ at: number | null } | null>(null);
  if (unavailable && outage === null) setOutage({ at: refreshedAt });
  const fresh = outage !== null && refreshedAt !== null && (outage.at === null || refreshedAt > outage.at);
  if (outage !== null && !unavailable && fresh) setOutage(null);
  return outage !== null && !unavailable && !fresh;
}
