import { useCallback, useEffect, useRef } from 'react';
import { useDebouncedCallback } from '@/lib/hooks/useDebounce';
import { useFeatureFlag } from '@/lib/contexts/FeatureFlagsContext';
import { useAnalytics } from '@/lib/contexts/AnalyticsContext';
import type { TerminalState } from '@/lib/hooks/useTerminalStream';

const RESYNC_DEBOUNCE_MS = 300;
const RESYNC_STALL_TIMEOUT_MS = 4000;
// Task 3.1.2.4b — hard ceiling on how long a single resync_id may stay
// outstanding, even while sibling resync traffic keeps proving the
// connection alive and resetting this hook's own 4s stall watchdog. Without
// this, a resync whose own server-side handling silently hangs/fails would
// never surface: every reset from an unrelated ID's response looks
// identical to progress. 2x RESYNC_STALL_TIMEOUT_MS gives sibling traffic
// real room to legitimately postpone the watchdog before treating it as
// evidence of an actual (not just slow) failure.
const RESYNC_STALL_ESCALATION_CEILING_MS = RESYNC_STALL_TIMEOUT_MS * 2;
// Delay before surfacing the existing reconnecting-banner UI for a pending
// resync (Story 2.1.8) — shorter than RESYNC_STALL_TIMEOUT_MS so a slow
// resync gets a visible affordance well before the 4s forced-reconnect path.
const RESYNC_BANNER_DELAY_MS = 2000;
// Epic 2.1 (AC1) — scopes visibility/focus-triggered resync to only the
// instance the user is actually looking at. Flag off preserves pre-project
// behavior (every mounted instance resyncs on every visibility/focus event).
const RESYNC_VISIBILITY_SCOPE_FLAG = 'terminal:resync-visibility-scope';

export interface UseVisibilityResyncParams {
  sessionId: string;
  /** Whether this terminal instance is the one currently in the foreground
   * (mirrors `TerminalOutput.tsx`'s `isVisible` prop). Only consulted when
   * `terminal:resync-visibility-scope` is on — see Epic 2.1 (AC1). */
  isVisible: boolean;
  isConnected: boolean;
  terminalState: TerminalState;
  connect: (cols?: number, rows?: number) => Promise<void>;
  disconnect: () => Promise<void>;
  requestFullResync: (urgent?: boolean, isVisibilityTriggered?: boolean) => string | undefined;
  markResyncComplete: () => void;
  markPaneResponseReceived: () => void;
  setShowReconnectButton: (value: boolean) => void;
  /** Reuses the existing 2s reconnecting-banner UI (Pattern Decisions:
   * "Transient-state UI during the 0-4s resync/watchdog window") once a
   * connected-branch resync has been pending ≥2s. Optional so tests that
   * don't care about this affordance can omit it. */
  setShowReconnectBanner?: (value: boolean) => void;
  /**
   * Epic 6.1 (terminal:resync-stagger) — when provided, routes the actual
   * `requestFullResync` call through this scheduler instead of firing it
   * synchronously, letting `SessionDetailView.tsx`'s per-view stagger queue
   * spread simultaneous multi-instance resyncs across a jittered window.
   * `fire` performs the real resync call (identical to today's inline
   * behavior); `opts.preempt` is true when this call originates from the
   * new isVisible-false->true transition below (Task 6.1.1.3 "newly-focused
   * preempts queued"), so the scheduler knows to jump this entry to the
   * front rather than queuing it behind already-pending instances.
   * Deliberately optional and only consulted when set — omitting it (flag
   * off) preserves the exact pre-Epic-6.1 synchronous-fire behavior (AC7
   * flag-off parity), including the fact that no isVisible-transition
   * trigger exists at all in that case.
   */
  scheduleResync?: (fire: () => void, opts: { preempt: boolean }) => void;
  /**
   * Epic 3.1, Task 3.1.2.4a/b — the same shared `Map<resyncId, addedAtMs>`
   * `TerminalOutput.tsx` and `useTerminalFlowControl.ts` use to reconcile
   * cross-hook stall-watchdog resets. `resetStallWatchdog` below reads this
   * hook's own pending `resyncId`'s recorded start time out of it (rather
   * than the local `resyncStartTimeRef`) so the ceiling check is anchored to
   * the moment the resync was actually issued, unaffected by anything
   * `resetStallWatchdog` itself does. Optional so tests exercising the
   * pre-Epic-3.1.2.4 behavior can omit it — resetStallWatchdog then always
   * just re-arms, never escalates.
   */
  outstandingResyncIdsRef?: React.MutableRefObject<Map<string, number>>;
}

export interface UseVisibilityResyncResult {
  /**
   * `resyncId` echoes `TerminalOutput.resync_id` (Epic 3.1, AC2) when the
   * output that just arrived is the reply to a correlation-ID-tagged resync
   * request. Only clears pending resync state when the ID matches the one
   * this hook is currently waiting on — or when either side has no ID
   * (correlation-ID flag off), preserving the pre-Epic-3.1 any-output-clears
   * heuristic described on pendingResyncCompletionRef above.
   */
  notifyResyncOutputReceived: (resyncId?: string) => void;
  /**
   * Shared cross-hook watchdog reconciliation (Epic 3.1, Task 3.1.2.1):
   * lets `TerminalOutput.tsx` reset this hook's stall watchdog when a
   * matching resync_id arrives via a *different* hook's output path
   * (useTerminalFlowControl's resize-triggered resync), without merging the
   * two hooks. No-op if no resync is currently pending here. Task 3.1.2.4b:
   * escalates (forces disconnect+reconnect) instead of resetting once this
   * hook's own pending resync_id has been outstanding at/past 2x
   * `RESYNC_STALL_TIMEOUT_MS`, so repeated sibling-triggered resets can't
   * mask a resync whose own response never arrives.
   */
  resetStallWatchdog: () => void;
}

export function useVisibilityResync(params: UseVisibilityResyncParams): UseVisibilityResyncResult {
  const {
    sessionId, isVisible, isConnected, terminalState, connect, disconnect,
    requestFullResync, markResyncComplete, markPaneResponseReceived, setShowReconnectButton,
    setShowReconnectBanner, scheduleResync, outstandingResyncIdsRef,
  } = params;

  const resyncVisibilityScopeEnabled = useFeatureFlag(RESYNC_VISIBILITY_SCOPE_FLAG);

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
  // Sibling ref to pendingResyncCompletionRef (Epic 3.1, Task 3.1.1.4c) —
  // kept separate rather than folding the ID into pendingResyncCompletionRef
  // itself, since requestFullResync returns undefined when the correlation-ID
  // flag is off, which would break the boolean pending/re-entrancy-guard
  // semantics above if it were the sole "is pending" indicator. Holds the
  // specific resync_id this hook is currently waiting on, or undefined when
  // there's no pending resync or the flag is off.
  const pendingResyncIdRef = useRef<string | undefined>(undefined);
  // Tracks whether the reconnecting-banner was actually shown (i.e. the 2s
  // resyncBannerTimerRef fired) for the *current* pending resync — distinct
  // from pendingResyncCompletionRef so notifyResyncOutputReceived only calls
  // setShowReconnectBanner(false) when there is actually a shown banner to
  // hide (Story 2.1.8 AC: "If the resync completes before 2000ms elapses...
  // setShowReconnectBanner is never called").
  const bannerShownRef = useRef(false);
  // Task 7.1.1.4 (Epic 7.1 observability) — timestamp the current pending
  // resync started, so notifyResyncOutputReceived's success path can log how
  // long it actually took (as a companion metric to the 4s stall-watchdog
  // ceiling above, not a replacement for it).
  const resyncStartTimeRef = useRef<number | null>(null);

  const { track } = useAnalytics();

  const isVisibleRef = useRef(isVisible);
  const resyncVisibilityScopeEnabledRef = useRef(resyncVisibilityScopeEnabled);
  const isConnectedRef = useRef(isConnected);
  const terminalStateRef = useRef(terminalState);
  const connectRef = useRef(connect);
  const disconnectRef = useRef(disconnect);
  const requestFullResyncRef = useRef(requestFullResync);
  const markResyncCompleteRef = useRef(markResyncComplete);
  const markPaneResponseReceivedRef = useRef(markPaneResponseReceived);
  const setShowReconnectButtonRef = useRef(setShowReconnectButton);
  const setShowReconnectBannerRef = useRef(setShowReconnectBanner);
  const scheduleResyncRef = useRef(scheduleResync);
  const trackRef = useRef(track);
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
    isVisibleRef.current = isVisible;
    resyncVisibilityScopeEnabledRef.current = resyncVisibilityScopeEnabled;
    isConnectedRef.current = isConnected;
    terminalStateRef.current = terminalState;
    connectRef.current = connect;
    disconnectRef.current = disconnect;
    requestFullResyncRef.current = requestFullResync;
    markResyncCompleteRef.current = markResyncComplete;
    markPaneResponseReceivedRef.current = markPaneResponseReceived;
    setShowReconnectButtonRef.current = setShowReconnectButton;
    setShowReconnectBannerRef.current = setShowReconnectBanner;
    scheduleResyncRef.current = scheduleResync;
    trackRef.current = track;
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

  // Shared terminal-recovery path (Epic 3.1, Tasks 3.1.2.2/3.1.2.4b) for both
  // ways a pending resync can be declared lost: the ordinary 4s stall
  // watchdog firing with no sibling traffic to reset it, or resetStallWatchdog
  // below discovering this hook's own resync_id has been outstanding past the
  // 2x escalation ceiling despite repeated resets. Identical recovery
  // (disconnect+reconnect) either way — only the log/analytics label differs,
  // so a stall dashboard can distinguish "no reset ever happened" fires from
  // "resets kept happening but the real response never came" ones.
  const forceResyncRecovery = useCallback((reason: 'watchdog' | 'escalation') => {
    if (!pendingResyncCompletionRef.current) return;
    const stalledResyncId = pendingResyncIdRef.current;
    // Actual wall-clock time since this resync's request was sent, not just the
    // nominal watchdog constant — an escalation-triggered fire can have been
    // outstanding far longer than RESYNC_STALL_ESCALATION_CEILING_MS if sibling
    // traffic kept resetting the per-fire timer (see this function's own doc
    // comment). Falls back to the nominal constant if the start time was
    // somehow never recorded, so the log line never shows an undefined/NaN
    // duration.
    const actualElapsedMs = resyncStartTimeRef.current !== null ? Date.now() - resyncStartTimeRef.current : undefined;
    pendingResyncCompletionRef.current = false;
    pendingResyncIdRef.current = undefined;
    bannerShownRef.current = false;
    resyncStartTimeRef.current = null;
    markResyncCompleteRef.current();
    markPaneResponseReceivedRef.current();
    clearStallTimer();
    clearBannerTimer();
    const elapsedMs = reason === 'escalation' ? RESYNC_STALL_ESCALATION_CEILING_MS : RESYNC_STALL_TIMEOUT_MS;
    const label = reason === 'escalation' ? 'escalation ceiling' : 'stall watchdog';
    // resyncId is included so a future occurrence can be grepped directly out of
    // the server's structured log (matches the resync_id field logged wherever
    // EchoResyncID/terminal:resync-correlation-id echoes it back) without first
    // having to correlate on session name + rough timestamp, which is how the
    // 2026-08-25 investigation into this exact symptom (see
    // session/tmux/tmux.go's resyncFastLaneTimeout doc comment) had to be done.
    console.warn(
      `[resync] sessionId=${sessionIdRef.current} resyncId=${stalledResyncId ?? '(none)'} ${label} fired after ${elapsedMs}ms nominal (${actualElapsedMs ?? 'unknown'}ms actual), forcing disconnect+reconnect`,
    );
    // Task 7.1.1.5 (Epic 7.1 observability) — structured analytics event
    // alongside the console.warn above, so stall-watchdog/escalation fires
    // are queryable (e.g. correlated with visibility_state) rather than only
    // discoverable by grepping console output.
    trackRef.current({
      name: reason === 'escalation' ? 'resync_stall_escalation_fired' : 'resync_stall_watchdog_fired',
      category: 'performance',
      durationMs: elapsedMs,
      labels: {
        resync_id: stalledResyncId ?? '',
        visibility_state: document.visibilityState,
      },
    });
    disconnectRef.current().then(() => connectRef.current());
  }, [clearStallTimer, clearBannerTimer]);

  // Extracted (Epic 3.1, Task 3.1.2.2) so both the initial resync trigger and
  // the shared resetStallWatchdog() below can (re)arm the same 4s watchdog
  // without duplicating its body.
  const armStallTimer = useCallback(() => {
    clearStallTimer();
    resyncStallTimerRef.current = setTimeout(() => {
      resyncStallTimerRef.current = null;
      forceResyncRecovery('watchdog');
    }, RESYNC_STALL_TIMEOUT_MS);
  }, [clearStallTimer, forceResyncRecovery]);

  const handleVisibilityOrFocusResyncInner = useCallback((preempt = false) => {
    if (document.visibilityState !== 'visible') return;
    // Epic 2.1 (AC1) — with the flag on, only the instance actually in the
    // foreground resyncs; a backgrounded instance is a no-op here and will
    // resync on its own once the user switches to it (its own isVisible flips
    // true and fires this same handler). Flag off preserves pre-project
    // behavior: every mounted instance resyncs on every visibility/focus event
    // regardless of isVisible.
    if (resyncVisibilityScopeEnabledRef.current && !isVisibleRef.current) return;
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
      // Epic 6.1 (terminal:resync-stagger) — `fire` is exactly today's
      // pre-Epic-6.1 synchronous resync logic, unchanged. When no scheduler
      // is wired (flag off, or no SessionDetailView-level stagger queue),
      // it's invoked immediately below, preserving byte-for-byte behavior.
      const fire = () => {
        try {
          console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus delay=0ms`);
          resyncStartTimeRef.current = Date.now();
          pendingResyncIdRef.current = requestFullResyncRef.current(true, true);
        } catch (err) {
          console.warn(`[resync] sessionId=${sessionId} requestFullResync threw synchronously`, err);
        } finally {
          // Arm unconditionally — even on a synchronous throw — so the watchdog
          // remains the single recovery path. See Story 2.1.3.
          armStallTimer();
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
      };
      if (scheduleResyncRef.current) {
        scheduleResyncRef.current(fire, { preempt });
      } else {
        fire();
      }
    } else {
      // Don't take the disconnected fallback mid-handshake. See Story 2.1.2.
      if (terminalStateRef.current === 'CONNECTING' || terminalStateRef.current === 'LOADING') return;
      console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus fallback=connect`);
      connectRef.current();
      setShowReconnectButtonRef.current(true);
    }
  }, [sessionId, clearBannerTimer, armStallTimer]);

  // Zero-arg wrapper: `debouncedResync` is registered directly as a DOM event
  // listener below, so useDebouncedCallback forwards whatever argument the
  // browser passes it (the Event object) straight through to the wrapped
  // callback. Without this indirection that Event would land in
  // handleVisibilityOrFocusResyncInner's `preempt` parameter (added for Task
  // 6.1.1.3) and every ordinary visibility/focus resync would be
  // misidentified as a preempting one.
  const handleVisibilityOrFocusResync = useCallback(() => {
    handleVisibilityOrFocusResyncInner(false);
  }, [handleVisibilityOrFocusResyncInner]);

  // Epic 1.2's useDebouncedCallback (ref-backed timer, memoized return) IS the
  // debounce mechanism here — not a hand-rolled setTimeout/ref pair — making
  // Epic 1.2's "first real consumer" framing true of the code (resolves
  // architecture review Concern "Epic 1.2 vs Task 2.1.1d mismatch").
  const debouncedResync = useDebouncedCallback(handleVisibilityOrFocusResync, RESYNC_DEBOUNCE_MS);

  useEffect(() => {
    document.addEventListener('visibilitychange', debouncedResync);
    window.addEventListener('focus', debouncedResync);
    return () => {
      document.removeEventListener('visibilitychange', debouncedResync);
      window.removeEventListener('focus', debouncedResync);
    };
  }, [debouncedResync]);

  // Task 6.1.1.3 ("newly-focused preempts queued") — today, nothing reacts to
  // an `isVisible` prop transition at all; only the global visibilitychange/
  // focus listeners above trigger a resync. Gated strictly on
  // `scheduleResync` being provided (i.e. `terminal:resync-stagger` is on and
  // SessionDetailView has wired its stagger queue in) so this new trigger
  // point is entirely absent when the flag is off, preserving AC7 flag-off
  // byte-for-byte parity — without a scheduler there is nothing for
  // "preempt" to mean, since there's no queue to jump ahead of.
  const wasVisibleRef = useRef(isVisible);
  useEffect(() => {
    const becameVisible = isVisible && !wasVisibleRef.current;
    wasVisibleRef.current = isVisible;
    if (becameVisible && scheduleResyncRef.current) {
      handleVisibilityOrFocusResyncInner(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isVisible]);

  // sessionId-keyed cleanup: a watchdog/resync armed for the previous session
  // must never fire against the next one's connect()/disconnect() (adversarial
  // review Blocker 1 / research features.md race #4).
  useEffect(() => {
    return () => {
      clearStallTimer();
      clearBannerTimer();
      pendingResyncCompletionRef.current = false;
      pendingResyncIdRef.current = undefined;
      bannerShownRef.current = false;
      resyncStartTimeRef.current = null;
      setShowReconnectBannerRef.current?.(false);
      markResyncCompleteRef.current();
      markPaneResponseReceivedRef.current();
    };
  }, [sessionId, clearStallTimer, clearBannerTimer]);

  const notifyResyncOutputReceived = useCallback((resyncId?: string) => {
    if (!pendingResyncCompletionRef.current) return;
    // Only clear pending state when the IDs match, or when either side lacks
    // an ID (correlation-ID flag off, or this output isn't a resync reply at
    // all) — preserves the pre-Epic-3.1 any-output-clears heuristic in that
    // case while preventing a *different* resync's stray output from
    // clearing this one's pending state when IDs are present and mismatch
    // (Epic 3.1, Task 3.1.1.4c / validation.md
    // notifyResyncOutputReceived_should_NotClearPendingResync_When_ResyncIdMismatch).
    if (resyncId && pendingResyncIdRef.current && resyncId !== pendingResyncIdRef.current) {
      // Task 7.1.1.3 (Epic 7.1 observability) — client-side correlation-ID
      // mismatch, mirrored by the server-side log in handleCurrentPaneRequest
      // (connectrpc_websocket.go) for the case where the mismatch instead
      // stems from the server never having echoed an ID at all.
      console.debug(`[resync] sessionId=${sessionId} resync_id mismatch: expected=${pendingResyncIdRef.current} received=${resyncId}`);
      return;
    }
    const resyncDurationMs = resyncStartTimeRef.current !== null ? Date.now() - resyncStartTimeRef.current : undefined;
    resyncStartTimeRef.current = null;
    pendingResyncCompletionRef.current = false;
    pendingResyncIdRef.current = undefined;
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
    // Task 7.1.1.4 (Epic 7.1 observability) — success-path duration, alongside
    // the existing stall-watchdog warning (armStallTimer above), so a normal
    // resync's actual latency is visible without waiting for one to stall.
    if (resyncDurationMs !== undefined) {
      console.debug(`[resync] sessionId=${sessionId} resync completed in ${resyncDurationMs}ms`);
    }
  }, [sessionId, clearStallTimer, clearBannerTimer]);

  // Epic 3.1, Task 3.1.2.1/3.1.2.2/3.1.2.4b — lets TerminalOutput.tsx reset
  // this hook's stall watchdog when a matching resync_id arrives via the
  // sibling useTerminalFlowControl/useTerminalStream resize-resync path,
  // without merging the two hooks. No-op if no resync is pending here.
  //
  // Task 3.1.2.4b: a reset must not be able to mask a genuinely-stalled
  // resync forever just because sibling traffic keeps proving the
  // connection alive. Before re-arming, check this hook's own pending
  // resync_id's recorded start time in the shared outstandingResyncIdsRef
  // map (Task 3.1.2.4a) — if it's been outstanding at or past the 2x
  // escalation ceiling, escalate (force disconnect+reconnect) instead of
  // resetting again. `outstandingResyncIdsRef` is optional (pre-3.1.2.4
  // callers/tests) — resetStallWatchdog always just re-arms when it's absent
  // or the ID's start time isn't recorded, preserving prior behavior.
  const resetStallWatchdog = useCallback(() => {
    if (!pendingResyncCompletionRef.current) return;
    const ownResyncId = pendingResyncIdRef.current;
    const startedAt = ownResyncId ? outstandingResyncIdsRef?.current.get(ownResyncId) : undefined;
    if (startedAt !== undefined && Date.now() - startedAt >= RESYNC_STALL_ESCALATION_CEILING_MS) {
      forceResyncRecovery('escalation');
      return;
    }
    armStallTimer();
  }, [armStallTimer, forceResyncRecovery, outstandingResyncIdsRef]);

  return { notifyResyncOutputReceived, resetStallWatchdog };
}
