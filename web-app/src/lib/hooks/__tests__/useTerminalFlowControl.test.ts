/**
 * Tests for useTerminalFlowControl - Resync, resize throttle, message dispatch.
 *
 * Mocks protobuf types and terminal to avoid environment issues.
 */

import { renderHook, act } from '@testing-library/react';

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
