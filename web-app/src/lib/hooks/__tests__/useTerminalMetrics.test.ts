/**
 * Tests for useTerminalMetrics - RAF batching and recording.
 *
 * Uses manual RAF mock for deterministic testing.
 */

import { renderHook, act } from '@testing-library/react';
import { useTerminalMetrics } from '../useTerminalMetrics';

// RAF mock
let rafCallback: FrameRequestCallback | null = null;

function setupRAFMock() {
  rafCallback = null;
  global.requestAnimationFrame = (cb: FrameRequestCallback): number => {
    rafCallback = cb;
    return 1;
  };
  global.cancelAnimationFrame = (): void => {
    rafCallback = null;
  };
}

function flushRAF(): void {
  if (rafCallback) {
    const cb = rafCallback;
    rafCallback = null;
    cb(performance.now());
  }
}

describe('useTerminalMetrics', () => {
  beforeEach(() => {
    setupRAFMock();
    jest.spyOn(console, 'log').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('scheduleOutputUpdate', () => {
    it('should schedule RAF for small text (not immediate flush)', () => {
      const { result } = renderHook(() => useTerminalMetrics({}));

      act(() => {
        result.current.scheduleOutputUpdate('small text');
      });

      // RAF should be scheduled but not yet flushed
      expect(rafCallback).not.toBeNull();
      // Output state not yet updated (waiting for RAF)
      expect(result.current.output).toBe('');
    });

    it('should flush immediately for text exceeding 4KB', () => {
      const { result } = renderHook(() => useTerminalMetrics({}));
      const largeText = 'X'.repeat(5000);

      act(() => {
        result.current.scheduleOutputUpdate(largeText);
      });

      // Should have flushed immediately (no RAF needed)
      expect(result.current.output).toBe(largeText);
    });

    it('should batch multiple small updates into one RAF flush', () => {
      const { result } = renderHook(() => useTerminalMetrics({}));

      act(() => {
        result.current.scheduleOutputUpdate('part1');
        result.current.scheduleOutputUpdate('part2');
        result.current.scheduleOutputUpdate('part3');
      });

      // Flush the pending RAF
      act(() => {
        flushRAF();
      });

      expect(result.current.output).toBe('part1part2part3');
    });

    it('should use onOutput callback when provided (bypass state)', () => {
      const onOutput = jest.fn();
      const { result } = renderHook(() => useTerminalMetrics({ onOutput }));

      act(() => {
        result.current.scheduleOutputUpdate('callback text');
      });

      expect(onOutput).toHaveBeenCalledWith('callback text');
      // State should NOT be updated when callback is used
      expect(result.current.output).toBe('');
    });
  });

  describe('flushOutputBuffer', () => {
    it('should join buffered text and update state', () => {
      const { result } = renderHook(() => useTerminalMetrics({}));

      act(() => {
        result.current.scheduleOutputUpdate('a');
        result.current.scheduleOutputUpdate('b');
      });

      act(() => {
        result.current.flushOutputBuffer();
      });

      expect(result.current.output).toBe('ab');
    });

    it('should be a no-op when buffer is empty', () => {
      const { result } = renderHook(() => useTerminalMetrics({}));

      act(() => {
        result.current.flushOutputBuffer();
      });

      expect(result.current.output).toBe('');
    });
  });

  describe('recording', () => {
    it('should set recording flag on startRecording', () => {
      const { result } = renderHook(() => useTerminalMetrics({}));

      expect(result.current.isRecording).toBe(false);

      act(() => {
        result.current.startRecording();
      });

      expect(result.current.isRecording).toBe(true);
    });

    it('should create download on stopRecording', () => {
      const { result } = renderHook(() => useTerminalMetrics({}));

      // Mock URL.createObjectURL and document.createElement AFTER renderHook
      // to avoid interfering with React's DOM operations
      const mockClick = jest.fn();
      const originalCreateElement = document.createElement.bind(document);
      const mockCreateElement = jest.spyOn(document, 'createElement').mockImplementation((tag: string) => {
        if (tag === 'a') {
          return { href: '', download: '', click: mockClick } as any;
        }
        return originalCreateElement(tag);
      });
      const mockCreateObjectURL = jest.fn().mockReturnValue('blob:test');
      global.URL.createObjectURL = mockCreateObjectURL;

      act(() => {
        result.current.startRecording();
        result.current.recordMessage({
          timestamp: Date.now(),
          type: 'raw',
          data: new Uint8Array([72, 101, 108, 108, 111]),
          decoded: 'Hello',
        });
        result.current.stopRecording();
      });

      expect(result.current.isRecording).toBe(false);
      expect(mockCreateObjectURL).toHaveBeenCalled();
      expect(mockClick).toHaveBeenCalled();

      mockCreateElement.mockRestore();
    });
  });

  describe('cleanup', () => {
    it('should cancel pending RAF on unmount', () => {
      const cancelRAF = jest.fn();
      global.cancelAnimationFrame = cancelRAF;

      const { result, unmount } = renderHook(() => useTerminalMetrics({}));

      act(() => {
        result.current.scheduleOutputUpdate('pending');
      });

      unmount();

      // cancelAnimationFrame should have been called
      expect(cancelRAF).toHaveBeenCalled();
    });
  });
});
