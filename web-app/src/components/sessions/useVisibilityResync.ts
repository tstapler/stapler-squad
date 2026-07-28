import { useCallback, useEffect, useRef } from 'react';
import { useDebouncedCallback } from '@/lib/hooks/useDebounce';
import type { TerminalState } from '@/lib/hooks/useTerminalStream';

const RESYNC_DEBOUNCE_MS = 300;
const RESYNC_STALL_TIMEOUT_MS = 4000;
// Delay before surfacing the existing reconnecting-banner UI for a pending
// resync (Story 2.1.8) — shorter than RESYNC_STALL_TIMEOUT_MS so a slow
// resync gets a visible affordance well before the 4s forced-reconnect path.
const RESYNC_BANNER_DELAY_MS = 2000;

export interface UseVisibilityResyncParams {
  sessionId: string;
  isConnected: boolean;
  terminalState: TerminalState;
  connect: (cols?: number, rows?: number) => Promise<void>;
  disconnect: () => Promise<void>;
  requestFullResync: (urgent?: boolean) => void;
  markResyncComplete: () => void;
  markPaneResponseReceived: () => void;
  setShowReconnectButton: (value: boolean) => void;
  /** Reuses the existing 2s reconnecting-banner UI (Pattern Decisions:
   * "Transient-state UI during the 0-4s resync/watchdog window") once a
   * connected-branch resync has been pending ≥2s. Optional so tests that
   * don't care about this affordance can omit it. */
  setShowReconnectBanner?: (value: boolean) => void;
}

export interface UseVisibilityResyncResult {
  notifyResyncOutputReceived: () => void;
}

export function useVisibilityResync(params: UseVisibilityResyncParams): UseVisibilityResyncResult {
  const {
    sessionId, isConnected, terminalState, connect, disconnect,
    requestFullResync, markResyncComplete, markPaneResponseReceived, setShowReconnectButton,
    setShowReconnectBanner,
  } = params;

  const resyncStallTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Distinct ref/timer from resyncStallTimerRef and useDebouncedCallback's own
  // internal timer (pitfalls.md guardrail: never share a ref between timers).
  // Fires 2s into a pending resync to surface the existing reconnecting-banner
  // UI (Story 2.1.8) — separate from the 4s stall watchdog above.
  const resyncBannerTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // No-correlation-ID heuristic: cleared by the *next* output of any kind (see
  // notifyResyncOutputReceived below), not specifically the CurrentPaneRequest
  // response — same imprecise heuristic the pre-existing resize->resync path
  // already relies on (isResyncingRef/waitingForPaneResponseRef). Accepted
  // trade-off, locked in by Task 2.1.3d's test; worst case bounded by the
  // RESYNC_STALL_TIMEOUT_MS watchdog below.
  const pendingResyncCompletionRef = useRef(false);
  // Tracks whether the reconnecting-banner was actually shown (i.e. the 2s
  // resyncBannerTimerRef fired) for the *current* pending resync — distinct
  // from pendingResyncCompletionRef so notifyResyncOutputReceived only calls
  // setShowReconnectBanner(false) when there is actually a shown banner to
  // hide (Story 2.1.8 AC: "If the resync completes before 2000ms elapses...
  // setShowReconnectBanner is never called").
  const bannerShownRef = useRef(false);

  const isConnectedRef = useRef(isConnected);
  const terminalStateRef = useRef(terminalState);
  const connectRef = useRef(connect);
  const disconnectRef = useRef(disconnect);
  const requestFullResyncRef = useRef(requestFullResync);
  const markResyncCompleteRef = useRef(markResyncComplete);
  const markPaneResponseReceivedRef = useRef(markPaneResponseReceived);
  const setShowReconnectButtonRef = useRef(setShowReconnectButton);
  const setShowReconnectBannerRef = useRef(setShowReconnectBanner);
  // Mirrors the *latest* sessionId on every render, independent of which
  // sessionId a given `handleVisibilityOrFocusResyncInner` closure was created
  // for. Lets a stale, already-scheduled debounced/watchdog callback detect
  // mid-flight that the session has since switched and abort instead of
  // firing against the new session's connect()/disconnect() (2nd-review
  // architecture/adversarial finding: session-switch cleanup, Story 2.1.6,
  // only tears down state for a resync/watchdog that had already started —
  // it doesn't cancel a debounce timer that's still pending when the switch
  // happens, since useDebouncedCallback exposes no cancel handle).
  const sessionIdRef = useRef(sessionId);

  useEffect(() => {
    isConnectedRef.current = isConnected;
    terminalStateRef.current = terminalState;
    connectRef.current = connect;
    disconnectRef.current = disconnect;
    requestFullResyncRef.current = requestFullResync;
    markResyncCompleteRef.current = markResyncComplete;
    markPaneResponseReceivedRef.current = markPaneResponseReceived;
    setShowReconnectButtonRef.current = setShowReconnectButton;
    setShowReconnectBannerRef.current = setShowReconnectBanner;
    sessionIdRef.current = sessionId;
  });

  const clearStallTimer = useCallback(() => {
    if (resyncStallTimerRef.current) {
      clearTimeout(resyncStallTimerRef.current);
      resyncStallTimerRef.current = null;
    }
  }, []);

  const clearBannerTimer = useCallback(() => {
    if (resyncBannerTimerRef.current) {
      clearTimeout(resyncBannerTimerRef.current);
      resyncBannerTimerRef.current = null;
    }
  }, []);

  const handleVisibilityOrFocusResyncInner = useCallback(() => {
    if (document.visibilityState !== 'visible') return;
    // Abort if the session changed since this debounced call was scheduled —
    // e.g. visibilitychange fires while viewing session A, the debounce timer
    // is armed, then the user switches to session B before the 300ms elapses.
    // `sessionId` here is this closure's frozen value (a real useCallback dep);
    // `sessionIdRef.current` is the latest live value. A mismatch means this is
    // a stale callback that must not act on the new session's connect/disconnect
    // (2nd-review finding: Story 2.1.6's cleanup only handles a resync/watchdog
    // that already started, not one still pending in the debounce window).
    if (sessionId !== sessionIdRef.current) return;

    if (isConnectedRef.current) {
      // Rapid-flap re-entrancy guard — see Story 2.1.3.
      if (pendingResyncCompletionRef.current) return;

      pendingResyncCompletionRef.current = true;
      try {
        console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus delay=0ms`);
        requestFullResyncRef.current(true);
      } catch (err) {
        console.warn(`[resync] sessionId=${sessionId} requestFullResync threw synchronously`, err);
      } finally {
        // Arm unconditionally — even on a synchronous throw — so the watchdog
        // remains the single recovery path. See Story 2.1.3.
        clearStallTimer();
        resyncStallTimerRef.current = setTimeout(() => {
          resyncStallTimerRef.current = null;
          if (pendingResyncCompletionRef.current) {
            pendingResyncCompletionRef.current = false;
            bannerShownRef.current = false;
            markResyncCompleteRef.current();
            markPaneResponseReceivedRef.current();
            console.warn(`[resync] sessionId=${sessionId} stall watchdog fired after ${RESYNC_STALL_TIMEOUT_MS}ms, forcing disconnect+reconnect`);
            clearBannerTimer();
            disconnectRef.current().then(() => connectRef.current());
          }
        }, RESYNC_STALL_TIMEOUT_MS);
        // Surface the existing reconnecting-banner UI once a resync has been
        // pending ≥2s — see Story 2.1.8. Independent of, and shorter than, the
        // 4s stall watchdog above; reuses `TerminalOutput.tsx`'s already-shipped
        // banner rather than adding a new resync-specific indicator (Pattern
        // Decisions: "Transient-state UI during the 0-4s resync/watchdog window").
        clearBannerTimer();
        bannerShownRef.current = false;
        resyncBannerTimerRef.current = setTimeout(() => {
          resyncBannerTimerRef.current = null;
          if (pendingResyncCompletionRef.current) {
            bannerShownRef.current = true;
            setShowReconnectBannerRef.current?.(true);
          }
        }, RESYNC_BANNER_DELAY_MS);
      }
    } else {
      // Don't take the disconnected fallback mid-handshake. See Story 2.1.2.
      if (terminalStateRef.current === 'CONNECTING' || terminalStateRef.current === 'LOADING') return;
      console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus fallback=connect`);
      connectRef.current();
      setShowReconnectButtonRef.current(true);
    }
  }, [sessionId, clearStallTimer, clearBannerTimer]);

  // Epic 1.2's useDebouncedCallback (ref-backed timer, memoized return) IS the
  // debounce mechanism here — not a hand-rolled setTimeout/ref pair — making
  // Epic 1.2's "first real consumer" framing true of the code (resolves
  // architecture review Concern "Epic 1.2 vs Task 2.1.1d mismatch").
  const debouncedResync = useDebouncedCallback(handleVisibilityOrFocusResyncInner, RESYNC_DEBOUNCE_MS);

  useEffect(() => {
    document.addEventListener('visibilitychange', debouncedResync);
    window.addEventListener('focus', debouncedResync);
    return () => {
      document.removeEventListener('visibilitychange', debouncedResync);
      window.removeEventListener('focus', debouncedResync);
    };
  }, [debouncedResync]);

  // sessionId-keyed cleanup: a watchdog/resync armed for the previous session
  // must never fire against the next one's connect()/disconnect() (adversarial
  // review Blocker 1 / research features.md race #4).
  useEffect(() => {
    return () => {
      clearStallTimer();
      clearBannerTimer();
      pendingResyncCompletionRef.current = false;
      bannerShownRef.current = false;
      setShowReconnectBannerRef.current?.(false);
      markResyncCompleteRef.current();
      markPaneResponseReceivedRef.current();
    };
  }, [sessionId, clearStallTimer, clearBannerTimer]);

  const notifyResyncOutputReceived = useCallback(() => {
    if (pendingResyncCompletionRef.current) {
      pendingResyncCompletionRef.current = false;
      clearStallTimer();
      clearBannerTimer();
      // The success path never flips isConnected false, so the pre-existing
      // isConnected-driven banner effect (TerminalOutput.tsx:736-761) won't
      // hide a banner shown by the 2s timer above — hide it explicitly here,
      // but only if the banner was actually shown (Story 2.1.8 AC: a resync
      // that completes before the 2s banner delay must never touch
      // setShowReconnectBanner at all).
      if (bannerShownRef.current) {
        setShowReconnectBannerRef.current?.(false);
      }
      bannerShownRef.current = false;
      markResyncCompleteRef.current();
      markPaneResponseReceivedRef.current();
      console.log(`[resync] sessionId=${sessionId} pane response received, resync complete`);
    }
  }, [sessionId, clearStallTimer, clearBannerTimer]);

  return { notifyResyncOutputReceived };
}
