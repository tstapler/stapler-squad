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
  });
});
