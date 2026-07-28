/**
 * Tests for useTerminalFlowControl - Resync, resize throttle, message dispatch.
 *
 * Mocks protobuf types and terminal to avoid environment issues.
 */

import { renderHook, act } from '@testing-library/react';
import { useRef } from 'react';

// Mock @bufbuild/protobuf so create() returns a plain object mirroring the init fields
jest.mock('@bufbuild/protobuf', () => ({
  create: (_schema: unknown, init: Record<string, unknown> = {}) => ({ ...init }),
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
      lines: any; includeEscapes: any; targetCols: any; targetRows: any; streamingMode: any;
      constructor(init: any) { Object.assign(this, init); }
    },
    FlowControl: class { paused: any; watermark: any; constructor(init: any) { this.paused = init?.paused; this.watermark = init?.watermark; } },
    InputWithEcho: class { data: any; echoNum: any; clientTimestampMs: any; constructor(init: any) { Object.assign(this, init); } },
    SSPNegotiation: class {},
    SSPCapabilities: class {},
  };
});

jest.mock('@/lib/terminal/StateApplicator', () => ({
  StateApplicator: class {
    applyState = jest.fn().mockReturnValue(true);
    applyDiff = jest.fn().mockReturnValue(true);
    getCurrentSequence = jest.fn().mockReturnValue(BigInt(0));
    setOnDimensionMismatch = jest.fn();
    setEchoOverlay = jest.fn();
    setOnEchoAck = jest.fn();
    getIsApplyingState = jest.fn().mockReturnValue(false);
    resetSequence = jest.fn();
  },
}));

jest.mock('@/lib/terminal/EchoOverlay', () => ({
  EchoOverlay: class {
    attach = jest.fn();
    detach = jest.fn();
    showPredictiveEcho = jest.fn();
  },
}));

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
      streamingMode: 'raw' as const,
      enablePredictiveEcho: false,
      getTerminal,
      pushMessageRef,
      isConnectedRef,
      onError: jest.fn(),
      onEchoAck: jest.fn(),
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

    it('should skip resize when the same value is sent again after the throttle window elapses (AC3 value-dedup)', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(100, 30);
      });

      act(() => {
        jest.advanceTimersByTime(250);
      });

      const callCountBeforeRepeat = pushMessageFn.mock.calls.length;

      act(() => {
        result.current.resize(100, 30);
      });

      // Repeated identical value must not produce any additional pushMessage calls,
      // even though the 200ms time-throttle has fully elapsed.
      expect(pushMessageFn.mock.calls.length).toBe(callCountBeforeRepeat);
    });

    it('should not dedup a genuinely new value after a prior send (AC3/AC6 no-regression)', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      act(() => {
        result.current.resize(100, 30);
      });

      act(() => {
        jest.advanceTimersByTime(250);
      });

      act(() => {
        result.current.resize(150, 45);
      });

      const resizeSends = pushMessageFn.mock.calls.filter(
        (call) => call[0].data.case === 'resize'
      );
      expect(resizeSends.length).toBe(2);
      expect(resizeSends[0][0].data.value.cols).toBe(100);
      expect(resizeSends[0][0].data.value.rows).toBe(30);
      expect(resizeSends[1][0].data.value.cols).toBe(150);
      expect(resizeSends[1][0].data.value.rows).toBe(45);
    });

    it('should bypass value-dedup with force=true and re-establish lastSentSizeRef for subsequent dedup (AC3 force + reconnect)', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      // Send #1
      act(() => {
        result.current.resize(100, 30);
      });

      act(() => {
        jest.advanceTimersByTime(250);
      });

      // Send #2: same value, force=true bypasses the AC3 value-dedup (reconnect-resync simulation)
      act(() => {
        result.current.resize(100, 30, true);
      });

      const resizeSendsAfterForce = pushMessageFn.mock.calls.filter(
        (call) => call[0].data.case === 'resize'
      );
      expect(resizeSendsAfterForce.length).toBe(2);

      // Advance past the throttle window again so the next assertion isolates the
      // value-dedup check from the time-throttle (per adversarial-review.md Minor #2).
      act(() => {
        jest.advanceTimersByTime(250);
      });

      // No-force call with the same value should now be deduped: no 3rd send.
      act(() => {
        result.current.resize(100, 30);
      });

      const resizeSendsFinal = pushMessageFn.mock.calls.filter(
        (call) => call[0].data.case === 'resize'
      );
      expect(resizeSendsFinal.length).toBe(2);
    });

    it('should bypass the time-throttle with force=true when within the 200ms throttle window (AC3 force + throttle bypass)', () => {
      const { options, pushMessageFn } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      // Send #1
      act(() => {
        result.current.resize(100, 30);
      });

      // Advance by only 50ms — still inside the 200ms THROTTLE_MS window.
      act(() => {
        jest.advanceTimersByTime(50);
      });

      // Send #2: force=true must bypass the time-throttle too, not just the value-dedup.
      // Without the fix, the throttle's early return fires before the force check is
      // ever reached and this second send is silently dropped.
      act(() => {
        result.current.resize(100, 30, true);
      });

      const resizeSends = pushMessageFn.mock.calls.filter(
        (call) => call[0].data.case === 'resize'
      );
      expect(resizeSends.length).toBe(2);
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

  describe('sendInputWithEcho', () => {
    it('should increment echo counter and store timestamp', () => {
      const { options } = createTestOptions({ enablePredictiveEcho: true });
      const { result } = renderHook(() => useTerminalFlowControl(options));

      let echoNum1: bigint = BigInt(0);
      let echoNum2: bigint = BigInt(0);

      act(() => {
        echoNum1 = result.current.sendInputWithEcho('a');
      });
      act(() => {
        echoNum2 = result.current.sendInputWithEcho('b');
      });

      expect(echoNum1).toBe(BigInt(1));
      expect(echoNum2).toBe(BigInt(2));
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

  describe('getIsApplyingState', () => {
    it('should return false when no state applicator exists', () => {
      const { options } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      expect(result.current.getIsApplyingState()).toBe(false);
    });
  });

  describe('sspNegotiated', () => {
    it('should start as false', () => {
      const { options } = createTestOptions();
      const { result } = renderHook(() => useTerminalFlowControl(options));

      expect(result.current.sspNegotiated).toBe(false);
    });
  });
});
