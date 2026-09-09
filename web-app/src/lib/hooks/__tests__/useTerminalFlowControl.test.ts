/**
 * Tests for useTerminalFlowControl - Resync, resize throttle, message dispatch.
 *
 * Mocks protobuf types and terminal to avoid environment issues.
 */

import { renderHook, act } from '@testing-library/react';

// Mock @bufbuild/protobuf's create() to bypass real schema-based construction — the
// events_pb mock below exports plain classes, not GenMessage schema descriptors, so the
// real create(SomeSchema, init) would receive `undefined` for the schema and throw.
jest.mock('@bufbuild/protobuf', () => ({
  create: (_schema: any, init: any) => init,
}));

// Mock protobuf modules
jest.mock('@/gen/session/v1/events_pb', () => {
  class MockTerminalData {
    sessionId: string;
    data: any;
    constructor(init: any) {
      this.sessionId = init.sessionId;
      this.data = init.data;
    }
  }
  return {
    TerminalData: MockTerminalData,
    TerminalInput: class { data: any; constructor(init: any) { this.data = init?.data; } },
    TerminalResize: class { cols: number; rows: number; constructor(init: any) { this.cols = init?.cols; this.rows = init?.rows; } },
    ScrollbackRequest: class { fromSequence: any; limit: any; constructor(init: any) { this.fromSequence = init?.fromSequence; this.limit = init?.limit; } },
    CurrentPaneRequest: class {
      lines: any; includeEscapes: any; targetCols: any; targetRows: any;
      constructor(init: any) { Object.assign(this, init); }
    },
    FlowControl: class { paused: any; watermark: any; constructor(init: any) { this.paused = init?.paused; this.watermark = init?.watermark; } },
  };
});

// Epic 3.1 — requestFullResync's resync_id/stale_dimensions correlation is
// gated on the terminal:resync-correlation-id feature flag. Mocked here
// (default false, matching pre-Epic-3.1 behavior) so existing tests that
// don't care about the flag aren't affected; individual tests below flip it.
jest.mock('@/lib/contexts/FeatureFlagsContext', () => ({
  useFeatureFlag: jest.fn().mockReturnValue(false),
}));

import { useFeatureFlag } from '@/lib/contexts/FeatureFlagsContext';
import { useTerminalFlowControl, type UseTerminalFlowControlOptions } from '../useTerminalFlowControl';

// Helper to create a test wrapper with refs
function createTestOptions(overrides: Partial<UseTerminalFlowControlOptions> = {}) {
  const pushMessageFn = jest.fn();
  const pushMessageRef = { current: pushMessageFn };
  const isConnectedRef = { current: true };
  const mockTerminal = { cols: 80, rows: 24 };
  const getTerminal = () => mockTerminal as any;

  return {
    options: {
      sessionId: 'test-session',
      getTerminal,
      pushMessageRef,
      isConnectedRef,
      onError: jest.fn(),
      ...overrides,
    },
    pushMessageFn,
    pushMessageRef,
    isConnectedRef,
    mockTerminal,
  };
}

describe('useTerminalFlowControl', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    (useFeatureFlag as jest.Mock).mockReturnValue(false);
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  describe('sendInput', () => {
    it('should call pushMessage with correct TerminalData', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.sendInput('hello');
      });

      expect(pushMessageFn).toHaveBeenCalledTimes(1);
      const msg = pushMessageFn.mock.calls[0][0];
      expect(msg.sessionId).toBe('test-session');
      expect(msg.data.case).toBe('input');
    });

    it('should not send when disconnected', () => {
      const { options, pushMessageFn, isConnectedRef } = createTestOptions();
      isConnectedRef.current = false;
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.sendInput('hello');
      });

      expect(pushMessageFn).not.toHaveBeenCalled();
    });

    // Regression: this early return used to be completely silent (no
    // console output at all), making a disconnected terminal that still
    // showed "Connected" in the UI indistinguishable from a working one at
    // the browser console — exactly the "shows Connected, nothing happens
    // when typing" symptom. resize()'s equivalent disconnected-path warning
    // was the only one of the two dispatch functions that logged anything.
    it('warns when input is dropped because the stream is not connected', () => {
      const { options, isConnectedRef } = createTestOptions();
      isConnectedRef.current = false;
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.sendInput('hello');
      });

      expect(console.warn).toHaveBeenCalledWith(expect.stringMatching(/not connected/i));
    });
  });

  describe('resize', () => {
    it('should send resize message', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(120, 40);
      });

      expect(pushMessageFn).toHaveBeenCalled();
      const msg = pushMessageFn.mock.calls[0][0];
      expect(msg.data.case).toBe('resize');
    });

    it('warns and does not send when disconnected', () => {
      const { options, pushMessageFn, isConnectedRef } = createTestOptions();
      isConnectedRef.current = false;
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(120, 40);
      });

      expect(pushMessageFn).not.toHaveBeenCalled();
      expect(console.warn).toHaveBeenCalledWith(expect.stringMatching(/not connected/i));
    });

    it('should throttle to 200ms', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(100, 30);
      });

      const firstCallCount = pushMessageFn.mock.calls.length;

      act(() => {
        result.current.resize(110, 35);
      });

      // Second resize should be throttled (only first resize message sent)
      expect(pushMessageFn.mock.calls.length).toBe(firstCallCount);
    });

    it('should send follow-up CurrentPaneRequest after 100ms delay', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(120, 40);
      });

      const afterResize = pushMessageFn.mock.calls.length;

      act(() => {
        jest.advanceTimersByTime(100);
      });

      // Should have the follow-up pane request
      expect(pushMessageFn.mock.calls.length).toBe(afterResize + 1);
      const followUp = pushMessageFn.mock.calls[pushMessageFn.mock.calls.length - 1][0];
      expect(followUp.data.case).toBe('currentPaneRequest');
    });

    // Task 4.3.1, AC3: value-dedup against lastSentDimsRef, isolated from the
    // 200ms time throttle by advancing well past it before the repeat call.
    it('does not resend TerminalResize when (cols, rows) equals lastSentDimsRef even after the 200ms throttle window has elapsed', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(120, 40);
      });

      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(0);

      act(() => {
        // Past both the 200ms resize throttle AND the 100ms follow-up
        // CurrentPaneRequest scheduled by the first resize() call.
        jest.advanceTimersByTime(201);
      });

      const beforeSecondCall = pushMessageFn.mock.calls.length;

      act(() => {
        result.current.resize(120, 40);
      });

      // Same (cols, rows) as last sent -- dedup should skip it even though
      // the time throttle window has long since elapsed.
      expect(pushMessageFn.mock.calls.length).toBe(beforeSecondCall);
    });

    // Bounce detection: a direct flip-flop back to the size sent two resizes
    // ago (A -> B -> A) is held out past BOUNCE_HOLD_MS instead of applied
    // immediately, coalescing an oscillating viewport (e.g. mobile browser
    // chrome show/hide) into one settled resize instead of a real server-side
    // tmux resize-window call per bounce.
    it('holds a direct bounce-back (A -> B -> A) instead of sending immediately, then sends after the hold elapses', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(100, 30); // A
      });
      act(() => {
        jest.advanceTimersByTime(201); // clear the 200ms throttle
      });

      act(() => {
        result.current.resize(120, 40); // B
      });
      act(() => {
        jest.advanceTimersByTime(201);
      });

      const beforeBounce = pushMessageFn.mock.calls.length;

      act(() => {
        result.current.resize(100, 30); // back to A -- a direct bounce
      });

      // Not sent immediately.
      expect(pushMessageFn.mock.calls.length).toBe(beforeBounce);

      act(() => {
        jest.advanceTimersByTime(3001); // past BOUNCE_HOLD_MS
      });

      // Sent after the hold elapses.
      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(beforeBounce);
      const sent = pushMessageFn.mock.calls.find((c) => c[0].data.case === 'resize' && c[0].data.value.cols === 100);
      expect(sent).toBeDefined();
    });

    it('does not hold a resize that does not match the size from two sends ago', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(100, 30); // A
      });
      act(() => {
        jest.advanceTimersByTime(201);
      });

      act(() => {
        result.current.resize(120, 40); // B
      });
      act(() => {
        jest.advanceTimersByTime(201);
      });

      const beforeThird = pushMessageFn.mock.calls.length;

      act(() => {
        result.current.resize(140, 50); // C -- not a bounce, a genuinely new size
      });

      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(beforeThird);
    });

    // Regression: the original bounce detection only compared against the
    // size sent exactly two sends ago, so a 3-value cycle (A -> B -> C -> A)
    // sailed straight through — observed live (session staplersquad_stelekit,
    // mobile client) wandering across 10x6 -> 67x38 -> 67x22 before repeating
    // 67x38, none of which is a clean two-value flip-flop.
    it('holds a repeat of a size from 3 sends ago (a wider oscillation than a direct A -> B -> A bounce)', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      const sizes: [number, number][] = [[10, 6], [67, 38], [67, 22]];
      for (const [cols, rows] of sizes) {
        act(() => {
          result.current.resize(cols, rows);
        });
        act(() => {
          jest.advanceTimersByTime(201);
        });
      }

      const beforeRepeat = pushMessageFn.mock.calls.length;

      act(() => {
        // Repeats the size from 3 sends ago -- not caught by a 2-back-only check.
        result.current.resize(10, 6);
      });

      expect(pushMessageFn.mock.calls.length).toBe(beforeRepeat);

      act(() => {
        jest.advanceTimersByTime(3001);
      });

      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(beforeRepeat);
      const sent = pushMessageFn.mock.calls.find((c) => c[0].data.case === 'resize' && c[0].data.value.cols === 10);
      expect(sent).toBeDefined();
    });

    // BOUNCE_HISTORY_SIZE is 4 — a repeat of a size old enough to have aged out of that
    // window must be treated as genuinely new, not a bounce.
    it('does not treat a repeat of a size older than BOUNCE_HISTORY_SIZE sends ago as a bounce', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));
      const sizes: [number, number][] = [[10, 6], [20, 7], [30, 8], [40, 9], [50, 10], [60, 11]];
      for (const [cols, rows] of sizes) {
        act(() => { result.current.resize(cols, rows); });
        act(() => { jest.advanceTimersByTime(201); });
      }
      const before = pushMessageFn.mock.calls.length;
      act(() => { result.current.resize(10, 6); }); // aged out of the history window
      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(before);
    });

    // Regression: a viewport still oscillating WHILE a bounce is already
    // being held should back off further on each consecutive re-trigger
    // instead of retrying at a flat 3s cadence forever — otherwise a
    // sustained oscillation just keeps re-arming the same 3s hold and never
    // actually settles. The streak resets once a hold genuinely resolves
    // (doSend runs), so this only escalates while bounces keep re-firing
    // before the previous hold elapses.
    it('escalates the hold duration when a bounce re-triggers before the previous hold elapses', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => { result.current.resize(100, 30); }); // A -- sends immediately
      act(() => { jest.advanceTimersByTime(201); });
      act(() => { result.current.resize(120, 40); }); // B -- sends immediately, history now [A]
      act(() => { jest.advanceTimersByTime(201); });

      const beforeBounces = pushMessageFn.mock.calls.length;

      act(() => { result.current.resize(100, 30); }); // bounce 1: matches history -- holds 3000ms
      act(() => { jest.advanceTimersByTime(1000); }); // well before the 3000ms hold elapses

      act(() => { result.current.resize(100, 30); }); // bounce 2: re-triggers before bounce 1 resolved -- escalates to 6000ms, restarts from now

      act(() => { jest.advanceTimersByTime(3001); }); // 3001ms since bounce 2 -- the old 3000ms hold would have fired by now
      expect(pushMessageFn.mock.calls.length).toBe(beforeBounces); // still held -- proves escalation took effect

      act(() => { jest.advanceTimersByTime(3000); }); // total 6001ms since bounce 2 -- escalated hold elapses
      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(beforeBounces);
    });

    // Regression: the escalating hold (3000 * 2^streak) must cap at BOUNCE_HOLD_MAX_MS
    // (15000ms) rather than growing unbounded -- streak 3 would otherwise hold 24000ms.
    it('caps the escalating hold at BOUNCE_HOLD_MAX_MS instead of growing unbounded', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => { result.current.resize(100, 30); }); // A -- sends immediately
      act(() => { jest.advanceTimersByTime(201); });
      act(() => { result.current.resize(120, 40); }); // B -- sends immediately, history now [A]
      act(() => { jest.advanceTimersByTime(201); });

      act(() => { result.current.resize(100, 30); }); // bounce streak 1 -- holds 3000ms
      act(() => { jest.advanceTimersByTime(500); });
      act(() => { result.current.resize(100, 30); }); // bounce streak 2 -- holds 6000ms
      act(() => { jest.advanceTimersByTime(500); });
      act(() => { result.current.resize(100, 30); }); // bounce streak 3 -- holds 12000ms
      act(() => { jest.advanceTimersByTime(500); });

      const beforeFinalBounce = pushMessageFn.mock.calls.length;
      act(() => { result.current.resize(100, 30); }); // bounce streak 4 -- would hold 24000ms uncapped

      act(() => { jest.advanceTimersByTime(14999); });
      expect(pushMessageFn.mock.calls.length).toBe(beforeFinalBounce); // still held just under the 15000ms cap

      act(() => { jest.advanceTimersByTime(2); });
      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(beforeFinalBounce); // cap elapsed (would not have if uncapped at 24000ms)
    });

    // Regression: bounceStreakRef must reset to 0 once a held bounce actually resolves,
    // so an unrelated bounce afterward restarts at the base 3000ms hold instead of
    // continuing to escalate from the resolved bounce's streak.
    it('resets the bounce hold to the base duration after a bounce resolves', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => { result.current.resize(100, 30); }); // A -- sends immediately
      act(() => { jest.advanceTimersByTime(201); });
      act(() => { result.current.resize(120, 40); }); // B -- sends immediately, history [A]
      act(() => { jest.advanceTimersByTime(201); });
      act(() => { result.current.resize(140, 50); }); // C -- sends immediately, history [A, B]
      act(() => { jest.advanceTimersByTime(201); });

      act(() => { result.current.resize(100, 30); }); // bounce on A -- holds base 3000ms
      // Advance past both the 3000ms hold and its trailing 100ms pane-request send, so
      // no timer from this bounce is left pending to bias the counts below.
      act(() => { jest.advanceTimersByTime(3101); });

      const beforeSecondBounce = pushMessageFn.mock.calls.length;
      act(() => { result.current.resize(120, 40); }); // unrelated bounce on B

      act(() => { jest.advanceTimersByTime(2999); });
      expect(pushMessageFn.mock.calls.length).toBe(beforeSecondBounce); // still held at the base hold

      act(() => { jest.advanceTimersByTime(2); });
      // Base 3000ms hold elapsed -- if the streak hadn't reset this would still be held
      // (the escalated 6000ms hold from before the first bounce resolved).
      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(beforeSecondBounce);
    });

    // Task 4.3.2, AC4: force:true bypasses both value-dedup and the time
    // throttle, mirroring the existing 'should allow urgent resync to bypass
    // throttle' test for requestFullResync.
    it('force:true bypasses both value-dedup and the time throttle and still sends TerminalResize', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(100, 30);
      });

      const afterFirst = pushMessageFn.mock.calls.length;
      expect(afterFirst).toBeGreaterThan(0);

      act(() => {
        // Immediately (0ms elapsed), same value, but forced.
        result.current.resize(100, 30, true);
      });

      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(afterFirst);
      const forcedMsg = pushMessageFn.mock.calls[pushMessageFn.mock.calls.length - 1][0];
      expect(forcedMsg.data.case).toBe('resize');
      expect(forcedMsg.data.value.cols).toBe(100);
      expect(forcedMsg.data.value.rows).toBe(30);
    });

    // Task 4.3.3: reordered lastResizeTimeRef/lastSentDimsRef update timing
    // (Task 2.1.4) -- a throwing send must not update lastSentDimsRef, so a
    // subsequent identical resize() call is not falsely deduped.
    it('does not dedupe a same-value resize following a failed send, since lastSentDimsRef only updates after pushMessage succeeds', () => {
      const { options, pushMessageFn } = createTestOptions();
      pushMessageFn.mockImplementationOnce(() => {
        throw new Error('send failed');
      });
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(90, 20);
      });

      expect(options.onError).toHaveBeenCalledTimes(1);
      const callsAfterFailure = pushMessageFn.mock.calls.length;

      act(() => {
        // Same (cols, rows) as the failed attempt -- must NOT be deduped,
        // since the throwing call never reached the lastSentDimsRef update.
        result.current.resize(90, 20);
      });

      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(callsAfterFailure);
      const msg = pushMessageFn.mock.calls[pushMessageFn.mock.calls.length - 1][0];
      expect(msg.data.case).toBe('resize');
      expect(msg.data.value.cols).toBe(90);
      expect(msg.data.value.rows).toBe(20);
    });

    // Regression: a bounce-back resize call that dedups against lastSentDimsRef
    // must still cancel any still-pending deferred resize timer. Scenario:
    // resize(80,24) sends immediately -> resize(85,24) within the 200ms
    // throttle window gets deferred (not sent yet, lastSentDimsRef still
    // {80,24}) -> resize(80,24) again matches lastSentDimsRef and dedup-
    // returns. If the pending-timer cancellation ran AFTER the dedup
    // early-return, the deferred {85,24} send would still be pending and
    // would fire later with a stale, wrong size.
    it('cancels a still-pending deferred resize when a bounce-back call dedups against lastSentDimsRef', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        // A: sends immediately, lastSentDimsRef = {80, 24}.
        result.current.resize(80, 24);
      });

      const afterFirstSend = pushMessageFn.mock.calls.length;
      expect(afterFirstSend).toBeGreaterThan(0);

      act(() => {
        jest.advanceTimersByTime(10);
      });

      act(() => {
        // B: within the 200ms throttle window -- deferred, not sent yet.
        result.current.resize(85, 24);
      });

      // Still no additional send -- {85, 24} is only pending via pendingResizeTimerRef.
      expect(pushMessageFn.mock.calls.length).toBe(afterFirstSend);

      act(() => {
        // Bounce back to A -- matches lastSentDimsRef, so this dedup-returns.
        // It must ALSO cancel the still-pending deferred {85, 24} timer.
        result.current.resize(80, 24);
      });

      act(() => {
        // Advance well past when the deferred {85, 24} send would have fired.
        jest.advanceTimersByTime(300);
      });

      // If the pending timer wasn't cancelled, a stale resize(85, 24) would
      // have fired during the advance above.
      const staleResize = pushMessageFn.mock.calls.find(
        ([call]) =>
          call.data.case === 'resize' &&
          call.data.value.cols === 85 &&
          call.data.value.rows === 24
      );
      expect(staleResize).toBeUndefined();
    });
  });

  describe('requestFullResync', () => {
    it('warns and does not send when disconnected', () => {
      const { options, pushMessageFn, isConnectedRef } = createTestOptions();
      isConnectedRef.current = false;
      const { result } = renderHook(() => useTerminalFlowControl(options));

      let returned: string | undefined;
      act(() => { returned = result.current.requestFullResync(true); });

      expect(returned).toBeUndefined();
      expect(pushMessageFn).not.toHaveBeenCalled();
      expect(console.warn).toHaveBeenCalledWith(expect.stringMatching(/not connected/i));
    });

    it('should throttle to 2s unless urgent', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.requestFullResync(false);
      });

      const firstCallCount = pushMessageFn.mock.calls.length;

      act(() => {
        result.current.requestFullResync(false);
      });

      // Second non-urgent resync should be throttled
      expect(pushMessageFn.mock.calls.length).toBe(firstCallCount);
    });

    it('should allow urgent resync to bypass throttle', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.requestFullResync(false);
      });

      const afterFirst = pushMessageFn.mock.calls.length;

      act(() => {
        result.current.requestFullResync(true);
      });

      // Urgent resync should bypass throttle
      expect(pushMessageFn.mock.calls.length).toBeGreaterThan(afterFirst);
    });

    // Epic 3.1 (AC2), Task 3.1.1.2-4.
    it('requestFullResync_should_GenerateResyncIdAndAttachToRequest_When_CorrelationFlagOn', () => {
      (useFeatureFlag as jest.Mock).mockReturnValue(true);
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      let returnedId: string | undefined;
      act(() => {
        returnedId = result.current.requestFullResync(true);
      });

      expect(returnedId).toBeTruthy();
      expect(pushMessageFn).toHaveBeenCalledTimes(1);
      const msg = pushMessageFn.mock.calls[0][0];
      expect(msg.data.case).toBe('currentPaneRequest');
      expect(msg.data.value.resyncId).toBe(returnedId);
    });

    it('requestFullResync_should_LeaveResyncIdEmptyAndReturnUndefined_When_CorrelationFlagOff', () => {
      (useFeatureFlag as jest.Mock).mockReturnValue(false);
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      let returnedId: string | undefined;
      act(() => {
        returnedId = result.current.requestFullResync(true);
      });

      expect(returnedId).toBeUndefined();
      const msg = pushMessageFn.mock.calls[0][0];
      expect(msg.data.value.resyncId).toBe('');
    });

    // Task 3.1.1.4d, pre-mortem P1 risk: stale_dimensions must NOT be
    // unconditionally true on every visibility-triggered resync. A genuine
    // resize while backgrounded (dimensions now differ from the last synced
    // ones) must report stale_dimensions=false.
    it('requestFullResync_should_ReportStaleDimensionsFalse_When_TerminalWasResizedWhileBackgrounded', () => {
      (useFeatureFlag as jest.Mock).mockReturnValue(true);
      const { options, pushMessageFn, mockTerminal } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      // First resync establishes lastSyncedDimensionsRef via
      // markPaneResponseReceived.
      act(() => {
        result.current.requestFullResync(true, true);
      });
      act(() => {
        result.current.markPaneResponseReceived();
      });

      // Terminal is resized while backgrounded before the next resync.
      mockTerminal.cols = 100;
      mockTerminal.rows = 30;

      act(() => {
        jest.advanceTimersByTime(2100); // clear the 2s throttle
      });

      let msg: any;
      act(() => {
        result.current.requestFullResync(true, true);
      });
      msg = pushMessageFn.mock.calls[pushMessageFn.mock.calls.length - 1][0];

      expect(msg.data.value.staleDimensions).toBe(false);
    });

    it('requestFullResync_should_ReportStaleDimensionsTrue_When_DimensionsUnchangedSinceLastSync', () => {
      (useFeatureFlag as jest.Mock).mockReturnValue(true);
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.requestFullResync(true, true);
      });
      act(() => {
        result.current.markPaneResponseReceived();
      });

      act(() => {
        jest.advanceTimersByTime(2100);
      });

      let msg: any;
      act(() => {
        result.current.requestFullResync(true, true);
      });
      msg = pushMessageFn.mock.calls[pushMessageFn.mock.calls.length - 1][0];

      expect(msg.data.value.staleDimensions).toBe(true);
    });

    it('requestFullResync_should_ReportStaleDimensionsFalse_When_NotVisibilityTriggered', () => {
      (useFeatureFlag as jest.Mock).mockReturnValue(true);
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.requestFullResync(true, true);
      });
      act(() => {
        result.current.markPaneResponseReceived();
      });

      act(() => {
        jest.advanceTimersByTime(2100);
      });

      let msg: any;
      act(() => {
        // Not a visibility-triggered resync this time.
        result.current.requestFullResync(true, false);
      });
      msg = pushMessageFn.mock.calls[pushMessageFn.mock.calls.length - 1][0];

      expect(msg.data.value.staleDimensions).toBe(false);
    });
  });

  describe('sendFlowControl', () => {
    it('should send correct FlowControl message', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.sendFlowControl(true, 50000);
      });

      expect(pushMessageFn).toHaveBeenCalled();
      const msg = pushMessageFn.mock.calls[0][0];
      expect(msg.data.case).toBe('flowControl');
    });

    // Regression: every dispatch function shares one ensureConnected() gate now instead of
    // independently reimplementing the connectivity check — this test, requestScrollback's
    // and requestFullResync's equivalents, and resize/sendInput's above cover every call site.
    it('warns and does not send when disconnected', () => {
      const { options, pushMessageFn, isConnectedRef } = createTestOptions();
      isConnectedRef.current = false;
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.sendFlowControl(true, 50000);
      });

      expect(pushMessageFn).not.toHaveBeenCalled();
      expect(console.warn).toHaveBeenCalledWith(expect.stringMatching(/not connected/i));
    });
  });

  describe('requestScrollback', () => {
    it('should send correct ScrollbackRequest message', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.requestScrollback(100, 50);
      });

      expect(pushMessageFn).toHaveBeenCalled();
      const msg = pushMessageFn.mock.calls[0][0];
      expect(msg.data.case).toBe('scrollbackRequest');
    });

    it('warns and does not send when disconnected', () => {
      const { options, pushMessageFn, isConnectedRef } = createTestOptions();
      isConnectedRef.current = false;
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.requestScrollback(100, 50);
      });

      expect(pushMessageFn).not.toHaveBeenCalled();
      expect(console.warn).toHaveBeenCalledWith(expect.stringMatching(/not connected/i));
    });
  });
});
