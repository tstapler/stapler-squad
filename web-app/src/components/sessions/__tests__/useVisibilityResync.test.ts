import { renderHook, act } from '@testing-library/react';
import { useVisibilityResync } from '../useVisibilityResync';

function makeParams(overrides: Partial<Parameters<typeof useVisibilityResync>[0]> = {}) {
  return {
    sessionId: 's1',
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
    expect(params.requestFullResync).toHaveBeenCalledWith(true);
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
});
