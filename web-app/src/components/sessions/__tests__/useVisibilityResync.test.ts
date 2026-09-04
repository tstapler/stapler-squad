import { renderHook, act } from '@testing-library/react';
import { useVisibilityResync } from '../useVisibilityResync';

// Mock useFeatureFlag — default to false (terminal:resync-visibility-scope
// off), overridden per-test for the Epic 2.1 (AC1) flag-on cases. Mirrors the
// established pattern in Navigation.test.tsx.
jest.mock('@/lib/contexts/FeatureFlagsContext', () => ({
  useFeatureFlag: jest.fn().mockReturnValue(false),
}));

// Task 7.1.1.5 — useVisibilityResync now calls useAnalytics() directly (it
// throws outside an AnalyticsContextProvider), so every test in this file
// needs this mock, not just the new stall-watchdog-analytics ones. Mirrors
// the established pattern in TerminalOutput.toolbar-analytics.test.tsx.
const mockTrack = jest.fn();
jest.mock('@/lib/contexts/AnalyticsContext', () => ({
  useAnalytics: () => ({ track: mockTrack }),
}));

import { useFeatureFlag } from '@/lib/contexts/FeatureFlagsContext';

function makeParams(overrides: Partial<Parameters<typeof useVisibilityResync>[0]> = {}) {
  return {
    sessionId: 's1',
    isVisible: true,
    isConnected: true,
    terminalState: 'STABLE' as const,
    connect: jest.fn().mockResolvedValue(undefined),
    disconnect: jest.fn().mockResolvedValue(undefined),
    requestFullResync: jest.fn(),
    markResyncComplete: jest.fn(),
    markPaneResponseReceived: jest.fn(),
    setShowReconnectButton: jest.fn(),
    setShowReconnectBanner: jest.fn(),
    ...overrides,
  };
}

describe('useVisibilityResync', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(console, 'info').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'log').mockImplementation(() => {});
    (useFeatureFlag as jest.Mock).mockReturnValue(false);
  });
  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  // ── Story 2.1.1 (AC1) ────────────────────────────────────────────────────

  it('useVisibilityResync_should_callRequestFullResyncExactlyOnce_When_visibilityAndFocusFireInSameTick', () => {
    const params = makeParams();
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      window.dispatchEvent(new Event('focus'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.requestFullResync).toHaveBeenCalledTimes(1);
    expect(params.requestFullResync).toHaveBeenCalledWith(true, true);
  });

  it('useVisibilityResync_should_callRequestFullResyncOnce_When_onlyFocusEventFires', () => {
    const params = makeParams();
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      window.dispatchEvent(new Event('focus'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.requestFullResync).toHaveBeenCalledTimes(1);
  });

  it('useVisibilityResync_should_callRequestFullResyncTwice_When_transitionsAreMoreThan300msApart', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(400); });
    expect(params.requestFullResync).toHaveBeenCalledTimes(1);

    // Simulate the first resync completing (real server round-trip) before the
    // second, independent transition fires — otherwise the re-entrancy guard
    // (Story 2.1.3) would correctly suppress a second call while one is still
    // outstanding, which is a different scenario (Task 2.1.3e).
    act(() => {
      result.current.notifyResyncOutputReceived();
    });

    act(() => {
      window.dispatchEvent(new Event('focus'));
    });
    act(() => { jest.advanceTimersByTime(400); });

    expect(params.requestFullResync).toHaveBeenCalledTimes(2);
  });

  // ── Epic 2.1 (AC1) — visibility-scoped resync ───────────────────────────

  it('handleVisibilityOrFocusResyncInner_should_CallRequestFullResync_When_IsVisibleTrueAndFlagOn', () => {
    (useFeatureFlag as jest.Mock).mockReturnValue(true);
    const params = makeParams({ isVisible: true });
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.requestFullResync).toHaveBeenCalledTimes(1);
    expect(params.requestFullResync).toHaveBeenCalledWith(true, true);
  });

  it('handleVisibilityOrFocusResyncInner_should_BeNoOp_When_IsVisibleFalseAndFlagOn', () => {
    (useFeatureFlag as jest.Mock).mockReturnValue(true);
    const params = makeParams({ isVisible: false });
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.requestFullResync).not.toHaveBeenCalled();
    expect(params.connect).not.toHaveBeenCalled();
  });

  it('handleVisibilityOrFocusResyncInner_should_CallRequestFullResync_When_IsVisibleFalseAndFlagOff', () => {
    (useFeatureFlag as jest.Mock).mockReturnValue(false);
    const params = makeParams({ isVisible: false });
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    // Flag off ⇒ unchanged pre-project behavior: every mounted instance
    // resyncs on visibility/focus regardless of isVisible.
    expect(params.requestFullResync).toHaveBeenCalledTimes(1);
    expect(params.requestFullResync).toHaveBeenCalledWith(true, true);
  });

  // ── Story 2.1.2 (AC4) ────────────────────────────────────────────────────

  it('useVisibilityResync_should_callConnectAndShowReconnectButton_When_visibilityFiresWhileDisconnected', () => {
    const params = makeParams({ isConnected: false, terminalState: 'DISCONNECTED' });
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.connect).toHaveBeenCalledTimes(1);
    // Reconnect-backoff-escalation fix (backlog: fix-terminal-reconnect-backoff-
    // escalation): connect() only skips its default backoff reset when called
    // with { isAutoRetry: true } from useTerminalStream's own automatic-retry
    // path. This call site must keep calling connect() with no arguments so it
    // keeps getting a fresh backoff sequence, not a silent continuation of
    // whatever attempt count a prior automatic retry left behind.
    expect(params.connect).toHaveBeenCalledWith();
    expect(params.setShowReconnectButton).toHaveBeenCalledWith(true);
  });

  it('useVisibilityResync_should_notCallConnect_When_visibilityFiresWhileTerminalStateIsConnecting', () => {
    const params = makeParams({ isConnected: false, terminalState: 'CONNECTING' });
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.connect).toHaveBeenCalledTimes(0);
    expect(params.setShowReconnectButton).not.toHaveBeenCalledWith(true);
  });

  it('useVisibilityResync_should_notCallConnect_When_visibilityFiresWhileTerminalStateIsLoading', () => {
    const params = makeParams({ isConnected: false, terminalState: 'LOADING' });
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.connect).toHaveBeenCalledTimes(0);
    expect(params.setShowReconnectButton).not.toHaveBeenCalledWith(true);
  });

  // ── Story 2.1.3 (AC5) ────────────────────────────────────────────────────

  it('useVisibilityResync_should_forceDisconnectThenConnect_When_resyncStallsPastWatchdogTimeout', async () => {
    const params = makeParams();
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    await act(async () => { jest.advanceTimersByTime(4000); });

    expect(params.disconnect).toHaveBeenCalledTimes(1);
    // flush the disconnect().then(connect) chain
    await act(async () => { await Promise.resolve(); });
    expect(params.connect).toHaveBeenCalledTimes(1);
    // Same fresh-backoff-sequence contract as the disconnected-fallback path
    // above: the 4s stall-watchdog forced reconnect must call connect() with no
    // arguments, not silently continue a prior escalated attempt count.
    expect(params.connect).toHaveBeenCalledWith();
  });

  it('useVisibilityResync_should_notForceDisconnect_When_resyncCompletesBeforeWatchdogTimeout', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    act(() => {
      jest.advanceTimersByTime(1000);
      result.current.notifyResyncOutputReceived();
    });

    act(() => { jest.advanceTimersByTime(3500); });

    expect(params.disconnect).not.toHaveBeenCalled();
  });

  it('useVisibilityResync_should_stillArmWatchdog_When_requestFullResyncThrowsSynchronously', async () => {
    const params = makeParams({
      requestFullResync: jest.fn(() => { throw new Error('boom'); }),
    });
    renderHook(() => useVisibilityResync(params));

    expect(() => {
      act(() => {
        Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
        document.dispatchEvent(new Event('visibilitychange'));
      });
      act(() => { jest.advanceTimersByTime(300); });
    }).not.toThrow();

    await act(async () => { jest.advanceTimersByTime(4000); });

    expect(params.disconnect).toHaveBeenCalledTimes(1);
  });

  it('useVisibilityResync_should_notFireWatchdog_When_unrelatedOutputArrivesMidWindow', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    act(() => {
      result.current.notifyResyncOutputReceived();
    });

    act(() => { jest.advanceTimersByTime(4000); });

    expect(params.disconnect).not.toHaveBeenCalled();
  });

  it('useVisibilityResync_should_notReissueResyncOrRearmWatchdog_When_pendingResyncAlreadyOutstanding', () => {
    const params = makeParams();
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.requestFullResync).toHaveBeenCalledTimes(1);

    act(() => {
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.requestFullResync).toHaveBeenCalledTimes(1);
  });

  // ── Story 2.1.4 (AC3) ────────────────────────────────────────────────────

  it('useVisibilityResync_should_notStealFocus_When_resyncAndWatchdogFire', async () => {
    const sibling = document.createElement('input');
    document.body.appendChild(sibling);
    sibling.focus();
    expect(document.activeElement).toBe(sibling);

    const params = makeParams();
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300);
    });
    expect(document.activeElement).toBe(sibling);

    await act(async () => { jest.advanceTimersByTime(4000); });
    expect(document.activeElement).toBe(sibling);

    document.body.removeChild(sibling);
  });

  // ── Story 2.1.5 (supports AC5) ───────────────────────────────────────────

  it('useVisibilityResync_should_clearPendingStateAndCallMarkFunctions_When_notifyResyncOutputReceivedCalledWhilePending', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    act(() => {
      result.current.notifyResyncOutputReceived();
    });

    expect(params.markResyncComplete).toHaveBeenCalledTimes(1);
    expect(params.markPaneResponseReceived).toHaveBeenCalledTimes(1);

    act(() => { jest.advanceTimersByTime(4000); });
    expect(params.disconnect).not.toHaveBeenCalled();
  });

  it('useVisibilityResync_should_noOp_When_notifyResyncOutputReceivedCalledWithNoPendingResync', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      result.current.notifyResyncOutputReceived();
    });

    expect(params.markResyncComplete).not.toHaveBeenCalled();
    expect(params.markPaneResponseReceived).not.toHaveBeenCalled();
  });

  // ── Story 2.1.6 (session-switch cleanup) ─────────────────────────────────

  it('useVisibilityResync_should_clearPendingWatchdog_When_sessionIdChangesWhileResyncPending', async () => {
    const paramsA = makeParams({ sessionId: 'a' });
    const { rerender } = renderHook((p) => useVisibilityResync(p), { initialProps: paramsA });

    act(() => {
      document.dispatchEvent(new Event('visibilitychange'));
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      jest.advanceTimersByTime(300);
    });
    expect(paramsA.requestFullResync).toHaveBeenCalledTimes(1);

    const paramsB = makeParams({ sessionId: 'b' });
    rerender(paramsB);

    await act(async () => { jest.advanceTimersByTime(4000); });

    expect(paramsA.disconnect).not.toHaveBeenCalled();
    expect(paramsA.connect).not.toHaveBeenCalled();
    expect(paramsB.disconnect).not.toHaveBeenCalled();
    expect(paramsB.connect).not.toHaveBeenCalled();
    expect(paramsA.markResyncComplete).toHaveBeenCalled();
    expect(paramsA.markPaneResponseReceived).toHaveBeenCalled();
  });

  it('useVisibilityResync_should_notFireResync_When_sessionIdChangesWhileDebouncePending', () => {
    const paramsA = makeParams({ sessionId: 'a' });
    const { rerender } = renderHook((p) => useVisibilityResync(p), { initialProps: paramsA });

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    // Switch sessions BEFORE the 300ms debounce elapses — the debounce timer
    // armed for 'a' is still pending when this happens.
    const paramsB = makeParams({ sessionId: 'b' });
    rerender(paramsB);

    act(() => { jest.advanceTimersByTime(300); });

    // The stale debounced call (armed while viewing 'a') must not act on
    // either session once it fires against a mismatched sessionIdRef.
    expect(paramsA.requestFullResync).not.toHaveBeenCalled();
    expect(paramsB.requestFullResync).not.toHaveBeenCalled();
  });

  // ── Story 2.1.8 (reconnecting-banner reuse) ──────────────────────────────

  it('useVisibilityResync_should_showReconnectBanner_When_resyncPendingPast2Seconds', () => {
    const params = makeParams();
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300); // debounce elapses, resync fires
    });
    expect(params.setShowReconnectBanner).not.toHaveBeenCalled();

    act(() => { jest.advanceTimersByTime(2000); });
    expect(params.setShowReconnectBanner).toHaveBeenCalledWith(true);
  });

  it('useVisibilityResync_should_notShowReconnectBanner_When_resyncCompletesBefore2Seconds', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300);
    });
    act(() => { jest.advanceTimersByTime(500); result.current.notifyResyncOutputReceived(); });
    act(() => { jest.advanceTimersByTime(2000); });

    expect(params.setShowReconnectBanner).not.toHaveBeenCalled();
  });

  it('useVisibilityResync_should_hideReconnectBanner_When_resyncCompletesAfterBannerShown', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300);
    });
    act(() => { jest.advanceTimersByTime(2000); }); // banner shown
    expect(params.setShowReconnectBanner).toHaveBeenCalledWith(true);

    act(() => { jest.advanceTimersByTime(1000); result.current.notifyResyncOutputReceived(); });
    expect(params.setShowReconnectBanner).toHaveBeenCalledWith(false);
  });

  // ── Epic 3.1 (AC2): resync_id correlation ────────────────────────────────

  it('notifyResyncOutputReceived_should_NotClearPendingResync_When_ResyncIdMismatch', () => {
    const params = makeParams({
      requestFullResync: jest.fn().mockReturnValue('resync-abc'),
    });
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.requestFullResync).toHaveBeenCalledWith(true, true);

    // Output arrives tagged with a DIFFERENT resync_id (e.g. the reply to a
    // stale/unrelated resize-triggered resync) — must not clear this hook's
    // pending state.
    act(() => {
      result.current.notifyResyncOutputReceived('resync-xyz');
    });

    expect(params.markResyncComplete).not.toHaveBeenCalled();
    expect(params.markPaneResponseReceived).not.toHaveBeenCalled();

    // The watchdog must still be live since pending state was never cleared.
    act(() => { jest.advanceTimersByTime(4000); });
    expect(params.disconnect).toHaveBeenCalledTimes(1);
  });

  // ── Story 3.1.2 (Tasks 3.1.2.1-3.1.2.4c) — cross-hook stall watchdog reset ─

  it('TerminalOutput_should_ResetStallWatchdogWithoutClearingBanner_When_DifferentOutstandingResyncIdRespondsFirst', () => {
    // Simulates TerminalOutput.tsx's handleOutput: a sibling hook's (e.g.
    // useTerminalFlowControl's resize-triggered) resync_id responds first and
    // calls this hook's exposed resetStallWatchdog() directly — this hook's
    // OWN pending resync ('resync-own') is unrelated and must remain pending
    // (no markResyncComplete/markPaneResponseReceived, banner not cleared).
    const t0 = Date.now();
    const outstandingResyncIdsRef = { current: new Map<string, number>([['resync-own', t0]]) };
    const params = makeParams({
      requestFullResync: jest.fn().mockReturnValue('resync-own'),
      outstandingResyncIdsRef,
    });
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });
    expect(params.requestFullResync).toHaveBeenCalledWith(true, true);

    // Past the 2s banner delay, the reconnecting banner is already showing.
    act(() => { jest.advanceTimersByTime(2000); });
    expect(params.setShowReconnectBanner).toHaveBeenCalledWith(true);
    (params.setShowReconnectBanner as jest.Mock).mockClear();

    // A different resync_id's output arrives (1000ms further along, well
    // short of the 8s escalation ceiling) and resets this hook's watchdog.
    act(() => { jest.advanceTimersByTime(1000); });
    act(() => { result.current.resetStallWatchdog(); });

    // Own resync is still unresolved — its completion bookkeeping and the
    // still-showing banner must be untouched by the sibling-triggered reset.
    expect(params.markResyncComplete).not.toHaveBeenCalled();
    expect(params.markPaneResponseReceived).not.toHaveBeenCalled();
    expect(params.setShowReconnectBanner).not.toHaveBeenCalledWith(false);
    expect(params.disconnect).not.toHaveBeenCalled();

    // The watchdog was re-armed (not left stale) by the reset: advancing a
    // full new 4s window from the reset point fires it, proving resetStallWatchdog
    // actually rearmed rather than being a no-op.
    act(() => { jest.advanceTimersByTime(4000); });
    expect(params.disconnect).toHaveBeenCalledTimes(1);
  });

  it('TerminalOutput_should_EscalateAndForceReconnect_When_OwnResyncIdExceedsTwiceStallTimeoutDespiteRepeatedResets', () => {
    // Task 3.1.2.4c — sibling-triggered resets alone must not mask a resync
    // whose own response never arrives: once THIS hook's own resync_id has
    // been outstanding at/past 2x RESYNC_STALL_TIMEOUT_MS (8000ms), the next
    // reset must escalate (force disconnect+reconnect) instead of re-arming.
    const t0 = Date.now();
    const outstandingResyncIdsRef = { current: new Map<string, number>([['resync-own', t0]]) };
    const params = makeParams({
      requestFullResync: jest.fn().mockReturnValue('resync-own'),
      outstandingResyncIdsRef,
    });
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });
    expect(params.requestFullResync).toHaveBeenCalledWith(true, true);

    // Sibling traffic keeps resetting the watchdog every 3s — comfortably
    // under the 4s single-fire timeout each time — for two rounds (6000ms
    // total elapsed since t0), which stays under the 8000ms ceiling.
    act(() => { jest.advanceTimersByTime(3000); });
    act(() => { result.current.resetStallWatchdog(); });
    expect(params.disconnect).not.toHaveBeenCalled();

    act(() => { jest.advanceTimersByTime(3000); });
    act(() => { result.current.resetStallWatchdog(); });
    expect(params.disconnect).not.toHaveBeenCalled();

    // A third reset arrives at 9000ms total elapsed — past the 8000ms
    // escalation ceiling — so this reset must escalate immediately rather
    // than granting yet another 4s grace period.
    act(() => { jest.advanceTimersByTime(3000); });
    act(() => { result.current.resetStallWatchdog(); });

    expect(params.disconnect).toHaveBeenCalledTimes(1);
    expect(mockTrack).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'resync_stall_escalation_fired',
        category: 'performance',
        durationMs: 8000,
        labels: expect.objectContaining({ resync_id: 'resync-own' }),
      })
    );
  });

  it('notifyResyncOutputReceived_should_ClearPendingResync_When_ResyncIdMatches', () => {
    const params = makeParams({
      requestFullResync: jest.fn().mockReturnValue('resync-abc'),
    });
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    act(() => {
      result.current.notifyResyncOutputReceived('resync-abc');
    });

    expect(params.markResyncComplete).toHaveBeenCalledTimes(1);
    expect(params.markPaneResponseReceived).toHaveBeenCalledTimes(1);

    act(() => { jest.advanceTimersByTime(4000); });
    expect(params.disconnect).not.toHaveBeenCalled();
  });

  it('notifyResyncOutputReceived_should_ClearPendingResync_When_NoResyncIdProvided', () => {
    // Correlation-ID flag off (or output without a resyncId) — preserves the
    // pre-Epic-3.1 any-output-clears heuristic.
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    act(() => {
      result.current.notifyResyncOutputReceived();
    });

    expect(params.markResyncComplete).toHaveBeenCalledTimes(1);
    expect(params.markPaneResponseReceived).toHaveBeenCalledTimes(1);
  });

  // ── Epic 6.1 (terminal:resync-stagger) — scheduleResync wiring ──────────

  it('useVisibilityResync_should_RouteResyncThroughScheduleResync_When_ScheduleResyncProvided', () => {
    const scheduleResync = jest.fn();
    const params = makeParams({ scheduleResync });
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    // The actual resync call is deferred to the scheduler, not fired inline.
    expect(scheduleResync).toHaveBeenCalledTimes(1);
    expect(scheduleResync).toHaveBeenCalledWith(expect.any(Function), { preempt: false });
    expect(params.requestFullResync).not.toHaveBeenCalled();

    // Invoking the scheduler's `fire` callback performs the real resync call
    // (identical body to the flag-off synchronous path).
    act(() => {
      scheduleResync.mock.calls[0][0]();
    });
    expect(params.requestFullResync).toHaveBeenCalledWith(true, true);
  });

  it('useVisibilityResync_should_FireResyncSynchronously_When_ScheduleResyncOmitted', () => {
    // AC7 flag-off parity: omitting scheduleResync entirely (terminal:resync-stagger
    // off) must preserve the exact pre-Epic-6.1 synchronous-fire behavior.
    const params = makeParams();
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(params.requestFullResync).toHaveBeenCalledWith(true, true);
  });

  it('useVisibilityResync_should_PreemptViaScheduleResync_When_IsVisibleTransitionsFalseToTrue', () => {
    // Task 6.1.1.3 — a newly-focused isVisible false->true transition must
    // preempt the stagger queue rather than joining it, but only when a
    // scheduler is actually provided (i.e. the flag is on).
    const scheduleResync = jest.fn();
    const paramsA = makeParams({ scheduleResync, isVisible: false });
    const { rerender } = renderHook((p) => useVisibilityResync(p), { initialProps: paramsA });

    expect(scheduleResync).not.toHaveBeenCalled();

    const paramsB = makeParams({ scheduleResync, isVisible: true });
    act(() => {
      rerender(paramsB);
    });

    expect(scheduleResync).toHaveBeenCalledTimes(1);
    expect(scheduleResync).toHaveBeenCalledWith(expect.any(Function), { preempt: true });
  });

  it('useVisibilityResync_should_NotTriggerIsVisibleTransition_When_ScheduleResyncOmitted', () => {
    // Flag-off parity: without a scheduler, no isVisible-transition trigger
    // exists at all — the false->true transition must not call
    // requestFullResync on its own (only the pre-existing visibilitychange/
    // focus listeners may do that).
    const paramsA = makeParams({ isVisible: false });
    const { rerender } = renderHook((p) => useVisibilityResync(p), { initialProps: paramsA });

    const paramsB = makeParams({ isVisible: true });
    act(() => {
      rerender(paramsB);
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(paramsB.requestFullResync).not.toHaveBeenCalled();
  });

  // ── Task 7.1.1.5 (Epic 7.1 observability) — stall watchdog analytics ────

  it('resyncStallWatchdog_should_TrackAnalyticsEventWithVisibilityState_When_WatchdogFiresWhileHidden', async () => {
    const params = makeParams({
      requestFullResync: jest.fn().mockReturnValue('resync-hidden-1'),
    });
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    // The resync is pending; the page goes to the background before the
    // watchdog fires — this is the scenario the test name asserts against.
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });

    await act(async () => { jest.advanceTimersByTime(4000); });

    expect(mockTrack).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'resync_stall_watchdog_fired',
        category: 'performance',
        durationMs: 4000,
        labels: expect.objectContaining({
          resync_id: 'resync-hidden-1',
          visibility_state: 'hidden',
        }),
      })
    );
  });

  // ── Task 7.1.1.3 (Epic 7.1 observability) — client-side correlation-ID mismatch log ──

  it('notifyResyncOutputReceived_should_LogDebugOnMismatch_When_ResyncIdDoesNotMatchEitherPendingId', () => {
    const params = makeParams({
      requestFullResync: jest.fn().mockReturnValue('resync-abc'),
    });
    const { result } = renderHook(() => useVisibilityResync(params));
    const debugSpy = jest.spyOn(console, 'debug').mockImplementation(() => {});

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    act(() => {
      result.current.notifyResyncOutputReceived('resync-xyz');
    });

    expect(debugSpy).toHaveBeenCalledWith(expect.stringContaining('resync_id mismatch'));
  });

  // ── Task 7.1.1.4 (Epic 7.1 observability) — success-path resync duration log ──

  it('notifyResyncOutputReceived_should_LogResyncDurationInMs_When_ResyncCompletesSuccessfully', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));
    const debugSpy = jest.spyOn(console, 'debug').mockImplementation(() => {});

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    act(() => { jest.advanceTimersByTime(300); });

    act(() => { jest.advanceTimersByTime(500); });

    act(() => {
      result.current.notifyResyncOutputReceived();
    });

    expect(debugSpy).toHaveBeenCalledWith(expect.stringMatching(/resync completed in \d+ms/));
  });
});
