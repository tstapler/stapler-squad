/**
 * Tests for TerminalStreamManager - Write buffering, watermarks, chunked writes,
 * redraw throttling, and escape sequence safety.
 *
 * Follows StateApplicator.test.ts pattern with MockTerminal class.
 */

import { TerminalStreamManager, HIGH_WATERMARK, LOW_WATERMARK, CHUNK_SIZE, type ITerminal, type SendFlowControlFn } from '../TerminalStreamManager';

// RAF mock for deterministic testing
let rafCallback: FrameRequestCallback | null = null;
const rafCallbacks: FrameRequestCallback[] = [];

function setupRAFMock() {
  rafCallback = null;
  rafCallbacks.length = 0;
  global.requestAnimationFrame = (cb: FrameRequestCallback): number => {
    rafCallback = cb;
    rafCallbacks.push(cb);
    return rafCallbacks.length;
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

function flushAllRAF(): void {
  while (rafCallback) {
    flushRAF();
  }
}

// Mock Terminal
class MockTerminal implements ITerminal {
  rows = 24;
  cols = 80;
  private written: Array<{ data: string; callback?: () => void }> = [];
  private cleared = false;
  private refreshed: Array<{ start: number; end: number }> = [];
  private scrolledToBottom = false;

  write(data: string | Uint8Array, callback?: () => void): void {
    const str = typeof data === 'string' ? data : new TextDecoder().decode(data);
    this.written.push({ data: str, callback });
    // Auto-invoke callback to simulate xterm.js processing
    callback?.();
  }

  clear(): void {
    this.cleared = true;
  }

  refresh(start: number, end: number): void {
    this.refreshed.push({ start, end });
  }

  scrollToBottom(): void {
    this.scrolledToBottom = true;
  }

  get buffer() {
    return {
      active: { cursorY: 0, viewportY: 0, length: 0 },
      normal: { length: 0 },
    };
  }

  // Test helpers
  getWrittenData(): string[] {
    return this.written.map(w => w.data);
  }

  getWrittenItems(): Array<{ data: string; callback?: () => void }> {
    return [...this.written];
  }

  wasCleared(): boolean {
    return this.cleared;
  }

  getRefreshCalls(): Array<{ start: number; end: number }> {
    return [...this.refreshed];
  }

  wasScrolledToBottom(): boolean {
    return this.scrolledToBottom;
  }

  resetTracking(): void {
    this.written = [];
    this.cleared = false;
    this.refreshed = [];
    this.scrolledToBottom = false;
  }
}

// Mock terminal that does NOT auto-invoke callbacks (for watermark testing)
class DelayedCallbackTerminal extends MockTerminal {
  private pendingCallbacks: Array<() => void> = [];

  write(data: string | Uint8Array, callback?: () => void): void {
    const str = typeof data === 'string' ? data : new TextDecoder().decode(data);
    // Store but do NOT auto-invoke callback
    (this as any).written = (this as any).written || [];
    // Access private via cast - just push tracking
    if (callback) {
      this.pendingCallbacks.push(callback);
    }
    // Call super's write without callback to avoid auto-invoke
    Object.getPrototypeOf(Object.getPrototypeOf(this)).write.call(this, data);
  }

  flushCallbacks(): void {
    const cbs = [...this.pendingCallbacks];
    this.pendingCallbacks = [];
    cbs.forEach(cb => cb());
  }

  getPendingCallbackCount(): number {
    return this.pendingCallbacks.length;
  }
}

describe('TerminalStreamManager', () => {
  let terminal: MockTerminal;
  let sendFlowControl: jest.Mock<ReturnType<SendFlowControlFn>, Parameters<SendFlowControlFn>>;
  let manager: TerminalStreamManager;

  beforeEach(() => {
    jest.useFakeTimers();
    // Set up RAF mock AFTER fake timers to ensure our mock isn't overridden
    setupRAFMock();

    terminal = new MockTerminal();
    sendFlowControl = jest.fn();
    manager = new TerminalStreamManager(terminal, sendFlowControl);

    // Suppress console output in tests
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  describe('small writes', () => {
    it('should write small data directly to terminal', () => {
      manager.write('Hello, World!');

      const written = terminal.getWrittenData();
      expect(written).toContain('Hello, World!');
    });

    it('should write multiple small outputs sequentially', () => {
      manager.write('Line 1\n');
      manager.write('Line 2\n');
      manager.write('Line 3\n');

      const written = terminal.getWrittenData();
      expect(written).toContain('Line 1\n');
      expect(written).toContain('Line 2\n');
      expect(written).toContain('Line 3\n');
    });
  });

  describe('large writes / chunking', () => {
    it('should write large data through enqueueWrite', async () => {
      // For the chunking test, use real timers since the async processWriteQueue
      // yields between chunks using RAF and awaits Promise resolution.
      jest.useRealTimers();

      // Re-setup RAF mock after switching to real timers.
      // Use an immediate-resolve mock so the queue processes without actual frames.
      global.requestAnimationFrame = (cb: FrameRequestCallback): number => {
        Promise.resolve().then(() => cb(performance.now()));
        return 1;
      };
      global.cancelAnimationFrame = (): void => {};

      const largeData = 'A'.repeat(CHUNK_SIZE + 1000);

      await manager.enqueueWrite(largeData);

      const written = terminal.getWrittenData();
      const totalWritten = written.join('').length;
      expect(totalWritten).toBe(largeData.length);

      // Restore fake timers and RAF mock for subsequent tests
      jest.useFakeTimers();
      setupRAFMock();
    });
  });

  describe('watermark flow control', () => {
    it('should start with watermark at 0 and not paused', () => {
      expect(manager.getWatermark()).toBe(0);
      expect(manager.getIsPaused()).toBe(false);
    });

    it('should not trigger pause for small writes', () => {
      manager.write('small data');

      expect(manager.getIsPaused()).toBe(false);
      expect(sendFlowControl).not.toHaveBeenCalled();
    });
  });

  describe('redraw throttling', () => {
    it('should pass through non-redraw output immediately', () => {
      manager.write('Normal text output');

      const written = terminal.getWrittenData();
      expect(written.some(w => w.includes('Normal text output'))).toBe(true);
    });

    it('should throttle full-screen redraw patterns', () => {
      // Full redraw pattern: cursor up at start
      const redraw1 = '\x1b[10ARedraw content 1';
      const redraw2 = '\x1b[10ARedraw content 2';

      manager.write(redraw1);
      manager.write(redraw2);

      // Only the latest should be pending (throttled)
      // After timer fires, latest should be flushed
      jest.advanceTimersByTime(100);

      const written = terminal.getWrittenData();
      // Should contain the latest redraw content
      expect(written.some(w => w.includes('Redraw content 2'))).toBe(true);
    });
  });

  describe('writeInitialContent', () => {
    it('should clear terminal and write content', async () => {
      await manager.writeInitialContent('Initial content here');

      expect(terminal.wasCleared()).toBe(true);
      const written = terminal.getWrittenData();
      expect(written.some(w => w.includes('Initial content here'))).toBe(true);
    });

    it('should scroll to bottom after writing', async () => {
      await manager.writeInitialContent('Content');

      expect(terminal.wasScrolledToBottom()).toBe(true);
    });
  });

  describe('escape sequence safety', () => {
    it('should handle partial ANSI escape at chunk boundary', () => {
      // Write with partial escape at end
      manager.write('Hello \x1b[31');
      // The escape parser buffers the partial sequence
      // Complete it with next write
      manager.write('mRed text\x1b[0m');

      const written = terminal.getWrittenData();
      const allText = written.join('');
      // Should contain complete color codes
      expect(allText).toContain('\x1b[31m');
      expect(allText).toContain('Red text');
    });
  });

  describe('mode transition refresh', () => {
    it('should schedule refresh on alternate screen exit', () => {
      manager.write('Before\x1b[?1049lAfter');

      // The mode transition schedules a refresh via RAF -- flush all pending
      flushAllRAF();

      const refreshes = terminal.getRefreshCalls();
      expect(refreshes.length).toBeGreaterThan(0);
      expect(refreshes[0].start).toBe(0);
      expect(refreshes[0].end).toBe(terminal.rows - 1);
    });
  });

  describe('first output tracking', () => {
    it('should call onFirstOutput callback on first write', () => {
      const onFirstOutput = jest.fn();
      manager.setOnFirstOutput(onFirstOutput);

      manager.write('First');
      expect(onFirstOutput).toHaveBeenCalledTimes(1);

      manager.write('Second');
      expect(onFirstOutput).toHaveBeenCalledTimes(1); // Not called again
    });
  });

  describe('cleanup', () => {
    it('should reset escape parser on cleanup', () => {
      // Write partial escape
      manager.write('Hello \x1b[31');
      manager.cleanup();

      // After cleanup, writing should start fresh (no buffered partial)
      terminal.resetTracking();
      const newManager = new TerminalStreamManager(terminal, sendFlowControl);
      newManager.write('Clean text');

      const written = terminal.getWrittenData();
      expect(written.some(w => w.includes('Clean text'))).toBe(true);
    });
  });
});
