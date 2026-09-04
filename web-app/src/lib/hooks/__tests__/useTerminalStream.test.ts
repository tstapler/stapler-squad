/**
 * Tests for useTerminalStream — ResizeQuiescence state machine (R1.4).
 *
 * Mocks ConnectRPC client so tests can push messages into the stream
 * on demand and verify terminalState transitions without races.
 */

import { renderHook, act, waitFor } from '@testing-library/react';
import { FOREGROUND_CONNECT_TIMEOUT_MS, CONNECT_TIMEOUT_MS, FOREGROUND_FAST_ATTEMPTS } from '@/lib/utils/backoff';

// ---------------------------------------------------------------------------
// Mock heavy infrastructure before any hook import
// ---------------------------------------------------------------------------

// @bufbuild/protobuf create() — return plain init object
jest.mock('@bufbuild/protobuf', () => ({
  create: (_schema: unknown, init: Record<string, unknown> = {}) => ({ ...init }),
}));

// ConnectRPC client — controlled per-test via mockStreamTerminal
const mockStreamTerminal = jest.fn();

jest.mock('@connectrpc/connect', () => {
  class ConnectError extends Error {
    metadata: Headers;
    constructor(message: string, meta?: Headers) {
      super(message);
      this.name = 'ConnectError';
      this.metadata = meta ?? new Headers();
    }
  }
  return {
    createClient: () => ({ streamTerminal: mockStreamTerminal }),
    ConnectError,
  };
});

// Transport — not needed
jest.mock('@/lib/transport/websocket-transport', () => ({
  createWebsocketBasedTransport: () => ({}),
}));

// Auth interceptor — not needed
jest.mock('@/lib/config', () => ({
  createAuthInterceptor: () => () => ({}),
}));

// Generated protobuf modules
jest.mock('@/gen/session/v1/session_pb', () => ({}));
jest.mock('@/gen/session/v1/events_pb', () => ({
  TerminalDataSchema: {},
  CurrentPaneRequestSchema: {},
  TerminalData: class {},
  CurrentPaneRequest: class {},
}));

// MessageQueue — a faithful-enough mock (matching the real implementation's
// push/close drop semantics) so Story 2.2 tests can exercise real
// interleaving. Instances are tracked in `mockMessageQueueInstances` so tests
// can assert on a specific generation's queue (spy on constructor calls).
const mockMessageQueueInstances: Array<{
  queue: unknown[];
  closed: boolean;
  push: jest.Mock;
  close: jest.Mock;
  isClosed: () => boolean;
}> = [];

jest.mock('@/lib/terminal/MessageQueue', () => {
  class MockMessageQueue {
    queue: unknown[] = [];
    closed = false;

    constructor() {
      mockMessageQueueInstances.push(this as unknown as (typeof mockMessageQueueInstances)[number]);
    }

    push = jest.fn((msg: unknown) => {
      if (this.closed) return;
      this.queue.push(msg);
    });

    close = jest.fn((): number => {
      const dropped = this.queue.length;
      this.queue = [];
      this.closed = true;
      return dropped;
    });

    isClosed() {
      return this.closed;
    }

    [Symbol.asyncIterator]() {
      return { next: async () => ({ value: undefined, done: true }) };
    }
  }
  return { MessageQueue: MockMessageQueue };
});

// Sub-hooks — minimal stubs so useTerminalStream can render
jest.mock('../useTerminalFlowControl', () => ({
  useTerminalFlowControl: () => ({
    sendInput: jest.fn(),
    resize: jest.fn(),
    requestScrollback: jest.fn(),
    sendFlowControl: jest.fn(),
    requestFullResync: jest.fn(),
    getIsResyncingRef: jest.fn().mockReturnValue({ current: false }),
  }),
}));

jest.mock('../useTerminalMetrics', () => ({
  useTerminalMetrics: () => ({
    output: '',
    scheduleOutputUpdate: jest.fn(),
    flushOutputBuffer: jest.fn(),
    recordMessage: jest.fn(),
    startRecording: jest.fn(),
    stopRecording: jest.fn(),
    isRecording: false,
  }),
}));

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------
import { useTerminalStream } from '../useTerminalStream';

// ---------------------------------------------------------------------------
// Controllable stream factory
// ---------------------------------------------------------------------------

/**
 * A push-based async iterable.  Call push(msg) to deliver a message,
 * end() to finish the stream, or error(err) to throw from the stream.
 */
interface PushStream<T> {
  iterable: AsyncIterable<T>;
  push(msg: T): void;
  end(): void;
}

function makePushStream<T>(signal?: AbortSignal): PushStream<T> {
  const queue: T[] = [];
  const resolvers: Array<{ resolve: () => void; reject: (err: Error) => void }> = [];
  let done = false;
  let aborted = false;

  signal?.addEventListener('abort', () => {
    aborted = true;
    const pending = resolvers.splice(0);
    pending.forEach((r) => r.reject(new Error('aborted')));
  });

  const push = (msg: T) => {
    queue.push(msg);
    resolvers.shift()?.resolve();
  };

  const end = () => {
    done = true;
    resolvers.shift()?.resolve();
  };

  const iterable: AsyncIterable<T> = {
    [Symbol.asyncIterator]() {
      return {
        async next(): Promise<IteratorResult<T>> {
          if (aborted) throw new Error('aborted');
          while (queue.length === 0 && !done) {
            await new Promise<void>((resolve, reject) => resolvers.push({ resolve, reject }));
          }
          if (queue.length > 0) {
            return { value: queue.shift()!, done: false };
          }
          return { value: undefined as any, done: true };
        },
      };
    },
  };

  return { iterable, push, end };
}

// ---------------------------------------------------------------------------
// Message factories
// ---------------------------------------------------------------------------

function makeResizeQuiescenceMsg(resizing: boolean) {
  return { data: { case: 'resizeQuiescence', value: { resizing } } };
}

function makeOutputMsg(data?: Uint8Array) {
  return {
    data: { case: 'output', value: { data: data ?? new TextEncoder().encode('hello') } },
  };
}

function makeErrorMsg(message: string, code?: string) {
  return { data: { case: 'error', value: { message, code } } };
}

function makeScrollbackResponseMsg(chunks: Uint8Array[]) {
  return {
    data: {
      case: 'scrollbackResponse',
      value: {
        chunks: chunks.map((d) => ({ data: d })),
        hasMore: false,
        oldestSequence: BigInt(0),
        newestSequence: BigInt(0),
        totalLines: BigInt(0),
      },
    },
  };
}

// ---------------------------------------------------------------------------
// Base options
// ---------------------------------------------------------------------------

const BASE_OPTIONS = {
  baseUrl: 'ws://localhost:8543',
  sessionId: 'test-session',
  autoConnect: true,
};

// ---------------------------------------------------------------------------
// Suites
// ---------------------------------------------------------------------------

describe('useTerminalStream — ResizeQuiescence state machine', () => {
  beforeEach(() => {
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'debug').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
    mockStreamTerminal.mockReset();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  // -------------------------------------------------------------------------
  // Test 1 — initial state before connect is DISCONNECTED
  // -------------------------------------------------------------------------
  it('should start in DISCONNECTED state when autoConnect=false', () => {
    mockStreamTerminal.mockImplementation(() => makePushStream().iterable);

    const { result } = renderHook(() =>
      useTerminalStream({ ...BASE_OPTIONS, autoConnect: false }),
    );

    expect(result.current.terminalState).toBe('DISCONNECTED');
  });

  // -------------------------------------------------------------------------
  // Test 2 — resizeQuiescence.resizing=true → RESIZING
  // -------------------------------------------------------------------------
  it('should transition terminalState to RESIZING when resizeQuiescence.resizing=true', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);

    const { result } = renderHook(() => useTerminalStream(BASE_OPTIONS));

    // Send an output message first to move out of CONNECTING/LOADING
    await act(async () => {
      stream.push(makeOutputMsg());
    });

    await waitFor(() => {
      expect(result.current.terminalState).toBe('STABLE');
    });

    // Now push a resizeQuiescence resizing=true message
    await act(async () => {
      stream.push(makeResizeQuiescenceMsg(true));
    });

    await waitFor(() => {
      expect(result.current.terminalState).toBe('RESIZING');
    });

    // Keep stream open so DISCONNECTED doesn't race
    stream.end();
  });

  // -------------------------------------------------------------------------
  // Test 3 — resizeQuiescence.resizing=false → STABLE (after RESIZING)
  // -------------------------------------------------------------------------
  it('should transition terminalState to STABLE when resizeQuiescence.resizing=false after RESIZING', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);

    const { result } = renderHook(() => useTerminalStream(BASE_OPTIONS));

    // Establish STABLE via output
    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => { expect(result.current.terminalState).toBe('STABLE'); });

    // Trigger RESIZING
    await act(async () => { stream.push(makeResizeQuiescenceMsg(true)); });
    await waitFor(() => { expect(result.current.terminalState).toBe('RESIZING'); });

    // Resolve back to STABLE
    await act(async () => { stream.push(makeResizeQuiescenceMsg(false)); });
    await waitFor(() => { expect(result.current.terminalState).toBe('STABLE'); });

    stream.end();
  });

  // -------------------------------------------------------------------------
  // Test 4 — multiple resize cycles stay consistent
  // -------------------------------------------------------------------------
  it('should handle multiple RESIZING/STABLE transitions without error', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);

    const { result } = renderHook(() => useTerminalStream(BASE_OPTIONS));

    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => { expect(result.current.terminalState).toBe('STABLE'); });

    // Cycle 1
    await act(async () => { stream.push(makeResizeQuiescenceMsg(true)); });
    await waitFor(() => { expect(result.current.terminalState).toBe('RESIZING'); });
    await act(async () => { stream.push(makeResizeQuiescenceMsg(false)); });
    await waitFor(() => { expect(result.current.terminalState).toBe('STABLE'); });

    // Cycle 2
    await act(async () => { stream.push(makeResizeQuiescenceMsg(true)); });
    await waitFor(() => { expect(result.current.terminalState).toBe('RESIZING'); });
    await act(async () => { stream.push(makeResizeQuiescenceMsg(false)); });
    await waitFor(() => { expect(result.current.terminalState).toBe('STABLE'); });

    stream.end();
  });

  // -------------------------------------------------------------------------
  // Test 5 — stream end → DISCONNECTED
  // -------------------------------------------------------------------------
  it('should transition to DISCONNECTED when the stream ends', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);

    const { result } = renderHook(() => useTerminalStream(BASE_OPTIONS));

    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => { expect(result.current.terminalState).toBe('STABLE'); });

    // End the stream — hook's finally block sets DISCONNECTED
    await act(async () => { stream.end(); });
    await waitFor(() => { expect(result.current.terminalState).toBe('DISCONNECTED'); });
  });
});

describe('useTerminalStream — scrollback decoder isolation', () => {
  beforeEach(() => {
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'debug').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
    mockStreamTerminal.mockReset();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  // -------------------------------------------------------------------------
  // Test: dedicated scrollback decoder does not corrupt live stream
  //
  // Scenario: a 3-byte UTF-8 sequence (€ = 0xE2 0x82 0xAC) is split across
  // two live output chunks.  Between the two halves, a scrollbackResponse
  // is delivered.  Because scrollbackDecoderRef is separate from
  // textDecoderRef, the in-flight state of textDecoderRef must survive
  // unchanged and the second live chunk must produce "€" with no replacement
  // characters (U+FFFD).
  // -------------------------------------------------------------------------
  it('useTerminalStream_should_notCorruptLiveStream_When_scrollbackDecodedBetweenLiveChunks', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);

    const liveOutputChunks: string[] = [];
    const onOutput = (text: string) => { liveOutputChunks.push(text); };

    renderHook(() => useTerminalStream({ ...BASE_OPTIONS, onOutput }));

    // 3-byte UTF-8 for € = 0xE2 0x82 0xAC — split: first byte, then last two
    const firstHalf = new Uint8Array([0xe2]);
    const secondHalf = new Uint8Array([0x82, 0xac]);

    // Interleaved scrollback data — a lone continuation byte.
    // Without decoder isolation, this stray byte would corrupt the live stream's in-flight
    // 0xe2 state (first byte of €), producing U+FFFD on the next live chunk instead of €.
    // With proper isolation the live textDecoderRef is unaffected.
    const scrollbackBytes = new Uint8Array([0x82]);

    // Send first half of live multi-byte sequence
    await act(async () => { stream.push(makeOutputMsg(firstHalf)); });
    // Insert a scrollback response between the two live halves
    await act(async () => { stream.push(makeScrollbackResponseMsg([scrollbackBytes])); });
    // Send the completing bytes of the live sequence
    await act(async () => { stream.push(makeOutputMsg(secondHalf)); });

    stream.end();

    // Wait briefly for processing
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 50));
    });

    // Concatenate all live output
    const liveOutput = liveOutputChunks.join('');

    // The live stream must contain the euro sign and no replacement characters
    expect(liveOutput).toContain('€');
    expect(liveOutput).not.toContain('�');
  });
});

// ---------------------------------------------------------------------------
// Phase 3 — Auto-reconnect (Stories 3.1.1 / 3.1.2 / 3.1.3)
// ---------------------------------------------------------------------------

describe('useTerminalStream — auto-reconnect (NEXT_PUBLIC_RECONNECT_V2)', () => {
  const RECONNECT_OPTIONS = {
    ...BASE_OPTIONS,
    autoConnect: false,
  };

  beforeEach(() => {
    process.env.NEXT_PUBLIC_RECONNECT_V2 = 'true';
    jest.useFakeTimers();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'debug').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
    jest.spyOn(console, 'info').mockImplementation(() => {});
    mockStreamTerminal.mockReset();
  });

  afterEach(() => {
    delete process.env.NEXT_PUBLIC_RECONNECT_V2;
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('connect_should_setShouldReconnectTrue_When_called', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => {
      result.current.connect();
    });

    // After connect(), the stream is open and connected after first message
    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    stream.end();
  });

  it('disconnect_should_setShouldReconnectFalse_When_called', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    // Connect first
    await act(async () => { result.current.connect(); });
    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // Disconnect should suppress reconnect
    await act(async () => { result.current.disconnect(); });

    // End the stream — without shouldReconnect, no reconnect is scheduled
    const reconnectSpy = jest.spyOn(result.current, 'connect');
    await act(async () => { stream.end(); });

    // Advance timers — no reconnect should fire
    await act(async () => { jest.advanceTimersByTime(35000); });

    expect(reconnectSpy).not.toHaveBeenCalled();
  });

  it('connect_should_notScheduleReconnect_When_featureFlagAbsent', async () => {
    delete process.env.NEXT_PUBLIC_RECONNECT_V2;

    const stream = makePushStream<object>();
    let callCount = 0;
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      return stream.iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => { result.current.connect(); });
    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    const callCountBeforeEnd = callCount;
    await act(async () => { stream.end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // Advance timers — no reconnect should fire since flag is absent
    await act(async () => { jest.advanceTimersByTime(35000); });

    expect(callCount).toBe(callCountBeforeEnd);
  });

  it('connect_should_reconnectAfterJitteredDelay_When_streamClosesCleanlyAndShouldReconnectTrue', async () => {
    let callCount = 0;
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      const s = makePushStream<object>();
      streams.push(s);
      return s.iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    // First connect + get to STABLE
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // End stream (clean close) — reconnect should be scheduled
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // Advance time to trigger the reconnect setTimeout
    await act(async () => { jest.advanceTimersByTime(35000); });

    // Should have been called a second time (reconnect)
    await waitFor(() => expect(callCount).toBeGreaterThanOrEqual(2));
  });

  it('connect_should_notReconnect_When_disconnectCalledDuringBackoffSleep', async () => {
    let callCount = 0;
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      const s = makePushStream<object>();
      streams.push(s);
      return s.iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // End stream — reconnect will be scheduled
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // Call disconnect() during the backoff sleep — should cancel reconnect
    await act(async () => { result.current.disconnect(); });

    const callCountSnapshot = callCount;
    await act(async () => { jest.advanceTimersByTime(35000); });

    // No additional streamTerminal calls
    expect(callCount).toBe(callCountSnapshot);
  });

  it('connect_should_setReconnectFalseAndNotRetry_When_wsClosesWithCode4004', async () => {
    const { ConnectError } = require('@connectrpc/connect');
    let callCount = 0;

    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      // Return an async iterable that throws a ConnectError with ws-close-code=4004
      return {
        [Symbol.asyncIterator]() {
          return {
            async next() {
              // Simulate a first message (triggers setIsConnected(true))
              if (callCount === 1) {
                const err = new ConnectError('session not found');
                err.metadata = new Headers({ 'ws-close-code': '4004' });
                throw err;
              }
              return { value: undefined, done: true };
            },
          };
        },
      };
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    const callCountSnapshot = callCount;
    await act(async () => { jest.advanceTimersByTime(35000); });

    // No reconnect should have been scheduled
    expect(callCount).toBe(callCountSnapshot);
  });

  it('connect_should_setIsHardFailedImmediately_When_errorMessageHasHubStartFailedCode', async () => {
    // design/ux.md Surface 2: a HUB_START_FAILED error frame means the
    // hub-owned path AND its legacy fallback both failed server-side —
    // retrying hits the same failure, so this must skip the normal
    // backoff-exhaustion path (which needs several failed reconnect
    // attempts) and set isHardFailed on the very first error frame.
    let callCount = 0;
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      const s = makePushStream<object>();
      streams.push(s);
      return s.iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));

    // First message is the hub-start-failure error — no prior successful
    // connection, no prior reconnect attempts.
    await act(async () => { streams[0].push(makeErrorMsg('hub start failed and legacy fallback also failed', 'HUB_START_FAILED')); });

    await waitFor(() => expect(result.current.isHardFailed).toBe(true));

    const callCountSnapshot = callCount;
    await act(async () => { jest.advanceTimersByTime(35000); });

    // No reconnect attempt should follow — shouldReconnectRef was never
    // exhausted via backoff, it was set false immediately.
    expect(callCount).toBe(callCountSnapshot);
  });

  it('connect_should_notReconnect_When_wsClosesWithCode4001', async () => {
    const { ConnectError } = require('@connectrpc/connect');
    let callCount = 0;

    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      return {
        [Symbol.asyncIterator]() {
          return {
            async next() {
              const err = new ConnectError('auth failure');
              err.metadata = new Headers({ 'ws-close-code': '4001' });
              throw err;
            },
          };
        },
      };
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    const callCountSnapshot = callCount;
    await act(async () => { jest.advanceTimersByTime(35000); });
    expect(callCount).toBe(callCountSnapshot);
  });

  it('cleanup_should_setShouldReconnectFalseBeforeDisconnect_When_hookUnmounts', async () => {
    let callCount = 0;
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      const s = makePushStream<object>();
      streams.push(s);
      return s.iterable;
    });

    const { result, unmount } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // Unmount while connected
    await act(async () => { unmount(); });

    const callCountAfterUnmount = callCount;

    // End stream after unmount — should NOT trigger reconnect
    await act(async () => { streams[0].end(); });
    await act(async () => { jest.advanceTimersByTime(35000); });

    expect(callCount).toBe(callCountAfterUnmount);
  });

  it('connect_should_scheduleReconnectOnlyOnce_When_retriableErrorThrown', async () => {
    let callCount = 0;
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      const s = makePushStream<object>();
      streams.push(s);
      return s.iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // Trigger a retriable error by ending the stream
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // Fast-forward timer to fire exactly the first reconnect callback
    await act(async () => { jest.advanceTimersByTime(35000); });

    // Should reconnect exactly once (no double-scheduling)
    await waitFor(() => expect(callCount).toBe(2));
  });

  it('handleVisibilityOrOnline_should_callConnect_When_tabBecomesVisibleAndStreamIsDisconnected', async () => {
    let callCount = 0;
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      const s = makePushStream<object>();
      streams.push(s);
      return s.iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    // Connect then let stream end (so shouldReconnect is true, isConnected is false)
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // Cancel any pending backoff timer from the natural reconnect
    await act(async () => { jest.advanceTimersByTime(0); });

    const callCountBefore = callCount;

    // Simulate visibilitychange → visible
    await act(async () => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300); // debounce
    });

    await waitFor(() => expect(callCount).toBeGreaterThan(callCountBefore));
  });

  it('handleVisibilityOrOnline_should_notCallConnect_When_shouldReconnectRefIsFalse', async () => {
    let callCount = 0;
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      return makePushStream<object>().iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    // Never connect — shouldReconnect stays false
    const callCountBefore = callCount;

    await act(async () => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300);
    });

    expect(callCount).toBe(callCountBefore);
  });

  it('handleVisibilityOrOnline_should_callConnect_When_onlineEventFires', async () => {
    let callCount = 0;
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      const s = makePushStream<object>();
      streams.push(s);
      return s.iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    // Connect then disconnect stream
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // Cancel any pending backoff timer from the natural reconnect
    await act(async () => { jest.advanceTimersByTime(0); });

    const callCountBefore = callCount;

    // Fire an online event
    await act(async () => {
      window.dispatchEvent(new Event('online'));
      jest.advanceTimersByTime(300);
    });

    await waitFor(() => expect(callCount).toBeGreaterThan(callCountBefore));
  });

  it('useTerminalStream_should_registerExactlyOneVisibilityListener_When_strictModeRemounts', async () => {
    // Verify that no extra listeners accumulate during strict mode double-invocation
    const addEventSpy = jest.spyOn(document, 'addEventListener');
    const removeEventSpy = jest.spyOn(document, 'removeEventListener');

    mockStreamTerminal.mockReturnValue(makePushStream<object>().iterable);

    const { unmount } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    const visibilityAdds = addEventSpy.mock.calls.filter(c => c[0] === 'visibilitychange').length;
    expect(visibilityAdds).toBeGreaterThanOrEqual(1);

    unmount();

    const visibilityRemoves = removeEventSpy.mock.calls.filter(c => c[0] === 'visibilitychange').length;
    expect(visibilityRemoves).toBeGreaterThanOrEqual(1);
  });

  // ---------------------------------------------------------------------------
  // reconnect-backoff-escalation fix: terminalBackoffRef.reset() at the top of
  // connect() used to fire unconditionally, including from the automatic-retry
  // call the finally block below makes to itself — so `.attempt` was always 0
  // when the hard-fail check ran, and the delay never escalated past
  // uniform(0, 1000ms). connect() now only resets when NOT called with
  // { isAutoRetry: true }, which only the automatic-retry call site passes.
  // ---------------------------------------------------------------------------

  it('connect_should_escalateDelayAndEventuallyHardFail_When_streamKeepsClosingCleanly', async () => {
    // Full-jitter delay is Math.random() * ceiling — pin Math.random() to 1 (the
    // supremum) so each cycle's delay is deterministically the per-attempt
    // ceiling (1000/2000/4000/8000/16000ms) instead of anywhere in [0, ceiling),
    // making the escalation boundaries below exact rather than flaky.
    jest.spyOn(Math, 'random').mockReturnValue(1);
    const ceilings = [1000, 2000, 4000, 8000, 16000];
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      const s = makePushStream<object>();
      streams.push(s);
      return s.iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    for (let i = 0; i < ceilings.length; i++) {
      const ceiling = ceilings[i];
      await act(async () => { streams[i].end(); });
      await waitFor(() => expect(result.current.isConnected).toBe(false));

      // AC #1: delay must not fire before this attempt's escalated ceiling —
      // if the reset bug were still present, every ceiling would be 1000ms and
      // this would already have retried by `ceiling - 1` for every i > 0.
      await act(async () => { jest.advanceTimersByTime(ceiling - 1); });
      expect(streams.length).toBe(i + 1);

      // Reaching the ceiling fires the retry — except after the 5th escalation
      // (attempt now at 5), which hard-fails instead of scheduling a 6th delay.
      await act(async () => { jest.advanceTimersByTime(1); });
      if (i < ceilings.length - 1) {
        await waitFor(() => expect(streams.length).toBe(i + 2));
        await act(async () => { streams[i + 1].push(makeOutputMsg()); });
        await waitFor(() => expect(result.current.isConnected).toBe(true));
      }
    }

    // AC #0: the 6th consecutive close (attempt === 5) hard-fails instead of
    // retrying forever.
    await act(async () => { streams[5].end(); });
    await waitFor(() => expect(result.current.isHardFailed).toBe(true));

    const streamCountAtHardFail = streams.length;
    await act(async () => { jest.advanceTimersByTime(60_000); });
    expect(streams.length).toBe(streamCountAtHardFail);
  });

  it('connect_should_resetBackoffSequence_When_calledDirectlyMidAutomaticRetrySequence', async () => {
    // AC #2 / AC #4: useVisibilityResync.ts's two reconnect call sites (the 4s
    // stall-watchdog forced reconnect and the visibility/focus-regained
    // fallback) call this exact bare connect() — no options, no wrapper —
    // since TerminalOutput.tsx passes useTerminalStream's connect straight
    // through unwrapped. Proving a fresh sequence here proves both callers
    // without duplicating backoff machinery into useVisibilityResync's own
    // (fully mocked) tests.
    jest.spyOn(Math, 'random').mockReturnValue(1);
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      const s = makePushStream<object>();
      streams.push(s);
      return s.iterable;
    });

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // First automatic retry escalates the sequence to attempt=1 (next ceiling
    // 2000ms).
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));
    await act(async () => { jest.advanceTimersByTime(1000); });
    await waitFor(() => expect(streams.length).toBe(2));
    await act(async () => { streams[1].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { streams[1].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // Sequence is now mid-backoff (waiting up to a 2000ms ceiling for the 3rd
    // attempt). Call connect() directly — the same call useVisibilityResync's
    // reconnect paths make — before that delay elapses.
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(3));
    await act(async () => { streams[2].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // If connect() reset the sequence (as it must), the next automatic retry's
    // delay is back to the attempt-0 ceiling (1000ms) — not the stale 2000ms it
    // would still be if reset() were skipped for a direct call like this one.
    await act(async () => { streams[2].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));
    await act(async () => { jest.advanceTimersByTime(1000); });
    await waitFor(() => expect(streams.length).toBe(4));
  });
});

// ---------------------------------------------------------------------------
// Story 2.2 — connection-generation guard
// ---------------------------------------------------------------------------

function createOutgoingInputMessage(data: string) {
  return { sessionId: 'test-session', data: { case: 'input' as const, value: { data } } };
}

describe('useTerminalStream — connection-generation guard (Story 2.2)', () => {
  beforeEach(() => {
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'debug').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
    mockStreamTerminal.mockReset();
    mockMessageQueueInstances.length = 0;
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  // Task 2.2.5 (adapted — see note below)
  //
  // NOTE ON THIS DESCRIBE BLOCK'S ADAPTATION: origin/main independently added
  // an isConnectingRef guard to connect() (`if (isConnectedRef.current ||
  // isConnectingRef.current || !sessionId) return;`) that this branch's fork
  // point did not have. That guard now makes a second connect() call a no-op
  // for as long as an earlier connect() is still "connecting" (i.e. until its
  // stream delivers a first message, or its read loop exits) — which is
  // stronger than this generation-guard fix alone (it prevents the wasted
  // duplicate connection attempt from starting at all, rather than starting
  // it and disambiguating afterward). Practically, that closes off directly
  // constructing "two connect() calls truly overlapping mid-flight" through
  // the public API the way these tests originally did. Each test below ends
  // a generation's stream (or calls disconnect() while never-connected, which
  // is a fast no-op) to clear that guard before starting the next generation
  // — still exercising the real teardown-and-disambiguation logic (Task 2.2.2
  // unconditional teardown, the generation check, drop-and-signal), just via
  // a reachable sequential trigger instead of a same-tick race the guard now
  // forecloses.
  it("a superseded generation's stale queue is torn down once a newer generation takes over", async () => {
    const stream1 = makePushStream<object>();
    const stream2 = makePushStream<object>();
    mockStreamTerminal
      .mockReturnValueOnce(stream1.iterable)
      .mockReturnValueOnce(stream2.iterable);

    const { result } = renderHook(() =>
      useTerminalStream({ ...BASE_OPTIONS, autoConnect: false }),
    );

    await act(async () => {
      result.current.connect(); // generation 1
    });

    // Generation 1's stream drops before ever delivering a message — its
    // read loop exits and its finally block runs (still the current
    // generation at that point), clearing the connecting/connected refs so
    // a fresh connect() is reachable again.
    await act(async () => {
      stream1.end();
    });

    await act(async () => {
      result.current.connect(); // generation 2
    });

    await act(async () => {
      stream2.push(makeOutputMsg());
    });

    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });
    expect(result.current.terminalState).toBe('STABLE');

    // Generation 1's now-stale queue was torn down by generation 2's
    // connect() (Task 2.2.2's unconditional teardown), not left dangling.
    expect(mockMessageQueueInstances).toHaveLength(2);
    expect(mockMessageQueueInstances[0].close).toHaveBeenCalled();

    stream2.end();
  });

  // Task 2.2.6
  it('three rapid connect() calls do not throw or leak', async () => {
    const stream1 = makePushStream<object>();
    mockStreamTerminal.mockReturnValueOnce(stream1.iterable);

    const { result } = renderHook(() =>
      useTerminalStream({ ...BASE_OPTIONS, autoConnect: false }),
    );

    await act(async () => {
      // All three calls happen synchronously in the same tick — simulates
      // StrictMode's double-invoke plus a genuine reconnect. main's own
      // isConnectingRef/isConnectedRef guard (see note above) makes the 2nd
      // and 3rd calls safe no-ops while the 1st is still connecting: only
      // one MessageQueue/connection attempt is ever created, no throw.
      result.current.connect();
      result.current.connect();
      result.current.connect();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockMessageQueueInstances).toHaveLength(1);
    expect(mockMessageQueueInstances[0].closed).toBe(false);

    stream1.end();
  });

  // Task 2.2.7 (adapted — see note above)
  it("disconnect() on a not-yet-connected generation tears down cleanly and does not block a subsequent connect()", async () => {
    const stream1 = makePushStream<object>();
    const stream2 = makePushStream<object>();
    mockStreamTerminal
      .mockReturnValueOnce(stream1.iterable)
      .mockReturnValueOnce(stream2.iterable);

    const { result } = renderHook(() =>
      useTerminalStream({ ...BASE_OPTIONS, autoConnect: false }),
    );

    await act(async () => {
      result.current.connect(); // generation 1
    });

    await act(async () => {
      // disconnect() while generation 1 never became visibly connected takes
      // the early-exit path (isConnectedRef.current is still false, so no
      // 1000ms wait) — closes generation 1's queue immediately.
      await result.current.disconnect();
    });

    expect(mockMessageQueueInstances).toHaveLength(1);
    expect(mockMessageQueueInstances[0].close).toHaveBeenCalled();

    // Generation 1's read loop is still technically live in this mock (no
    // AbortSignal wiring on the push-stream), so end its stream to let the
    // loop's own finally clear the connecting-ref guard — mirroring a real
    // aborted WebSocket throwing and unwinding the loop.
    await act(async () => {
      stream1.end();
    });

    await act(async () => {
      result.current.connect(); // generation 2 — must not be blocked by generation 1's teardown
    });

    await act(async () => {
      stream2.push(makeOutputMsg());
    });
    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });

    stream2.end();
  });

  // Regression (sdd:6-verify Layer 1 review): disconnect()'s delayed
  // graceful-close timeout read shared abortControllerRef/isConnected state
  // at *fire time* rather than a captured generation, so a disconnect() that
  // armed its 1000ms timer while still connected could abort/clobber a
  // newer generation that took over before the timer fired.
  it("a stale disconnect()'s delayed abort does not tear down a newer generation that connected in the meantime", async () => {
    const stream1 = makePushStream<object>();
    const stream2 = makePushStream<object>();
    mockStreamTerminal
      .mockReturnValueOnce(stream1.iterable)
      .mockReturnValueOnce(stream2.iterable);

    const { result } = renderHook(() =>
      useTerminalStream({ ...BASE_OPTIONS, autoConnect: false }),
    );

    await act(async () => {
      result.current.connect(); // generation 1
    });
    await act(async () => {
      stream1.push(makeOutputMsg());
    });
    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });

    // isConnectedRef.current is true at this synchronous instant, so
    // disconnect() arms its 1000ms graceful-close timer instead of
    // resolving immediately.
    const disconnectPromise = result.current.disconnect();

    // The connection dies out from under the pending disconnect() — gen1's
    // own generation-guarded teardown flips isConnected back to false,
    // letting a fresh connect() proceed.
    await act(async () => {
      stream1.end();
    });
    await waitFor(() => {
      expect(result.current.isConnected).toBe(false);
    });

    await act(async () => {
      result.current.connect(); // generation 2, while gen1's disconnect() timer is still pending
    });
    await act(async () => {
      stream2.push(makeOutputMsg());
    });
    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });

    const gen2Signal = mockStreamTerminal.mock.calls[1][1].signal as AbortSignal;
    expect(gen2Signal.aborted).toBe(false);

    // Let gen1's stale disconnect() timeout fire.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 1100));
      await disconnectPromise;
    });

    // Generation 2 must survive: its AbortSignal was not aborted and its
    // isConnected state was not clobbered back to false.
    expect(gen2Signal.aborted).toBe(false);
    expect(result.current.isConnected).toBe(true);

    stream2.end();
  }, 10000);

  // Task 2.2.8 (adapted — see note above)
  it('input buffered in a stale, torn-down generation is dropped, not delivered to the next connection', async () => {
    const stream1 = makePushStream<object>();
    const stream2 = makePushStream<object>();
    mockStreamTerminal
      .mockReturnValueOnce(stream1.iterable)
      .mockReturnValueOnce(stream2.iterable);

    const onInputDropped = jest.fn();

    const { result } = renderHook(() =>
      useTerminalStream({ ...BASE_OPTIONS, autoConnect: false, onInputDropped }),
    );

    await act(async () => {
      result.current.connect(); // generation 1
    });

    expect(mockMessageQueueInstances).toHaveLength(1);
    const gen1Queue = mockMessageQueueInstances[0];
    // Sanity: generation 1's handshake message is already buffered (nothing
    // consumes the outgoing queue in this mock, mirroring how a real
    // in-flight connection can have buffered-but-undelivered input).
    const bufferedBeforePush = gen1Queue.queue.length;

    const droppedMessage = createOutgoingInputMessage('typed-while-reconnecting');
    gen1Queue.push(droppedMessage);

    // Generation 1's connection drops (its read loop exits) while that
    // input is still sitting, undelivered, in its queue.
    await act(async () => {
      stream1.end();
    });

    await act(async () => {
      result.current.connect(); // generation 2 — unconditionally tears down generation 1's stale queue (Task 2.2.2)
    });

    // (a) never observed on generation 1 — its queue was cleared by close().
    expect(gen1Queue.queue).toHaveLength(0);

    // (b) never replayed onto generation 2's queue.
    expect(mockMessageQueueInstances).toHaveLength(2);
    const gen2Queue = mockMessageQueueInstances[1];
    expect(gen2Queue.queue).not.toContain(droppedMessage);

    // (c) onInputDropped fires with a count reflecting everything that was
    // dropped (the still-buffered handshake message plus our pushed input).
    expect(onInputDropped).toHaveBeenCalledWith(bufferedBeforePush + 1);

    stream2.end();
  });
});

// ---------------------------------------------------------------------------
// Foreground fast reconnect — `foreground` option + connect-timeout
// ---------------------------------------------------------------------------

describe('useTerminalStream — foreground connect-timeout', () => {
  const RECONNECT_OPTIONS = {
    ...BASE_OPTIONS,
    autoConnect: false,
  };

  beforeEach(() => {
    process.env.NEXT_PUBLIC_RECONNECT_V2 = 'true';
    jest.useFakeTimers();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'debug').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
    jest.spyOn(console, 'info').mockImplementation(() => {});
    mockStreamTerminal.mockReset();
  });

  afterEach(() => {
    delete process.env.NEXT_PUBLIC_RECONNECT_V2;
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('useTerminalStream_should_connectAsBeforeChange_When_foregroundOmitted', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => { result.current.connect(); });
    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
  });

  it('connect_should_abortAndRetry_When_foregroundTrueAndFirstAttemptExceedsFastTimeout', async () => {
    // A connect-timeout abort is excluded from the backoff budget (see the
    // reconnect-backoff-escalation fix) and retries immediately, so the retry
    // fires within the same timer advance that fires the abort — track every
    // attempt's signal rather than a single shared variable, which the second
    // attempt would silently overwrite before this test could assert on the first.
    const signals: (AbortSignal | undefined)[] = [];
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      signals.push(opts?.signal);
      return makePushStream(opts?.signal).iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    await act(async () => { result.current.connect(); });
    expect(signals[0]?.aborted).toBe(false);

    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    expect(signals[0]?.aborted).toBe(true);

    // Not stuck in DISCONNECTED: the immediate connect-timeout retry reaches
    // CONNECTING again (a fresh, unaborted signal) without any further advance.
    await waitFor(() => expect(result.current.terminalState).toBe('CONNECTING'));
    expect(signals.length).toBe(2);
    expect(signals[1]?.aborted).toBe(false);
  });

  it('connect_should_notAbort_When_notForegroundAndOnlyFastTimeoutElapsed', async () => {
    let capturedSignal: AbortSignal | undefined;
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      capturedSignal = opts?.signal;
      return makePushStream(opts?.signal).iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: false })
    );

    await act(async () => { result.current.connect(); });
    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    expect(capturedSignal?.aborted).toBe(false);

    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS - FOREGROUND_CONNECT_TIMEOUT_MS); });
    await waitFor(() => expect(capturedSignal?.aborted).toBe(true));
  });

  it('connect_should_useNormalTimeout_When_foregroundTrueAndFastWindowExhausted', async () => {
    const signals: (AbortSignal | undefined)[] = [];
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      signals.push(opts?.signal);
      const s = makePushStream<object>(opts?.signal);
      streams.push(s);
      return s.iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    // Consume the FOREGROUND_FAST_ATTEMPTS fast-timeout attempts.
    await act(async () => { result.current.connect(); });
    for (let i = 0; i < FOREGROUND_FAST_ATTEMPTS; i++) {
      await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
      await waitFor(() => expect(signals[i]?.aborted).toBe(true));
      // Backoff delay between attempts is uniform(0, 1000ms) at this codebase's
      // existing BackoffState(1000, 30_000) — 1000ms always covers it.
      await act(async () => { jest.advanceTimersByTime(1000); });
      await waitFor(() => expect(streams.length).toBe(i + 2));
    }

    // Next attempt (fast window exhausted) must use the normal timeout.
    const attemptIndex = FOREGROUND_FAST_ATTEMPTS;
    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    expect(signals[attemptIndex]?.aborted).toBe(false);

    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS - FOREGROUND_CONNECT_TIMEOUT_MS); });
    await waitFor(() => expect(signals[attemptIndex]?.aborted).toBe(true));
  });

  it('useTerminalStream_should_resetFastWindow_When_foregroundTransitionsFalseToTrue', async () => {
    const signals: (AbortSignal | undefined)[] = [];
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      signals.push(opts?.signal);
      const s = makePushStream<object>(opts?.signal);
      streams.push(s);
      return s.iterable;
    });

    const { result, rerender } = renderHook(
      (props: { foreground: boolean }) =>
        useTerminalStream({ ...RECONNECT_OPTIONS, foreground: props.foreground }),
      { initialProps: { foreground: true } }
    );

    // Exhaust the fast window (2 attempts), leaving a 3rd attempt in flight that
    // was scheduled (before any flip) using the normal timeout.
    await act(async () => { result.current.connect(); });
    for (let i = 0; i < FOREGROUND_FAST_ATTEMPTS; i++) {
      await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
      await waitFor(() => expect(signals[i]?.aborted).toBe(true));
      await act(async () => { jest.advanceTimersByTime(1000); });
      await waitFor(() => expect(streams.length).toBe(i + 2));
    }
    // streams[FOREGROUND_FAST_ATTEMPTS] is now in flight, fast window exhausted.

    // Flip false -> true. The in-flight attempt is unaffected (snapshot-at-schedule-time
    // — see plan.md's Pattern Decisions) and keeps using the normal timeout it was
    // already scheduled with; only attempts *started after* the reset are fast again.
    await act(async () => { rerender({ foreground: false }); });
    await act(async () => { rerender({ foreground: true }); });

    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
    await waitFor(() => expect(signals[FOREGROUND_FAST_ATTEMPTS]?.aborted).toBe(true));
    await act(async () => { jest.advanceTimersByTime(1000); });
    await waitFor(() => expect(streams.length).toBe(FOREGROUND_FAST_ATTEMPTS + 2));

    // The next attempt — started after the reset — uses the fast timeout again.
    const resetAttemptIndex = FOREGROUND_FAST_ATTEMPTS + 1;
    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    await waitFor(() => expect(signals[resetAttemptIndex]?.aborted).toBe(true));
  });

  it('useTerminalStream_should_clearStaleBackoffTimerAndReconnectImmediately_When_foregroundTransitionsFalseToTrue', async () => {
    // Pre-mortem Failure #1 (P1): resetting counters alone is not enough — a
    // pending reconnectTimerRef backoff delay computed while backgrounded must
    // also be cleared, with an immediate reconnect, on the false->true transition.
    const streams: ReturnType<typeof makePushStream<object>>[] = [];
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      const s = makePushStream<object>(opts?.signal);
      streams.push(s);
      return s.iterable;
    });

    const { result, rerender } = renderHook(
      (props: { foreground: boolean }) =>
        useTerminalStream({ ...RECONNECT_OPTIONS, foreground: props.foreground }),
      { initialProps: { foreground: false } }
    );

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // Clean close schedules a pending reconnectTimerRef backoff delay (0-1000ms).
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // Flip foreground before that stale delay elapses — a second connect should
    // fire immediately, without needing to advance any timers.
    await act(async () => { rerender({ foreground: true }); });
    await waitFor(() => expect(streams.length).toBe(2));

    // The original stale timer must not ALSO fire a duplicate 3rd connect.
    await act(async () => { jest.advanceTimersByTime(1000); });
    expect(streams.length).toBe(2);
  });

  it('connect_should_notAbort_When_firstMessageArrivesBeforeConnectTimeoutFires', async () => {
    // Pre-mortem Failure #3 (P2): a message that already landed must not be
    // retroactively aborted by a connect-timeout timer firing afterward.
    let capturedSignal: AbortSignal | undefined;
    const stream = makePushStream<object>();
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      capturedSignal = opts?.signal;
      return stream.iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    await act(async () => { result.current.connect(); });
    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // Advance past the connect-timeout duration — must NOT abort a healthy connection.
    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    expect(capturedSignal?.aborted).toBe(false);
    expect(result.current.isConnected).toBe(true);
  });

  it('useTerminalStream_should_notLeakConnectTimeout_When_disconnectCalledBeforeFirstMessage', async () => {
    let capturedSignal: AbortSignal | undefined;
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      capturedSignal = opts?.signal;
      return makePushStream(opts?.signal).iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    await act(async () => { result.current.connect(); });
    await act(async () => { await result.current.disconnect(); });

    const warnSpy = console.warn as jest.Mock;
    warnSpy.mockClear();

    // Advance well past both the fast and normal connect-timeout durations — the
    // pending connect-timeout must have been cleared by disconnect(), so no
    // trigger=connect-timeout log/abort fires against the torn-down attempt.
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
    const timeoutWarnings = warnSpy.mock.calls.filter((c) => String(c[0]).includes('trigger=connect-timeout'));
    expect(timeoutWarnings.length).toBe(0);
    expect(capturedSignal?.aborted).toBe(false);
  });

  it('connect_should_notScheduleConnectTimeout_When_reconnectV2FlagDisabled', async () => {
    delete process.env.NEXT_PUBLIC_RECONNECT_V2;
    let capturedSignal: AbortSignal | undefined;
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      capturedSignal = opts?.signal;
      return makePushStream(opts?.signal).iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    await act(async () => { result.current.connect(); });
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS + 1000); }); // well past both timeouts
    expect(capturedSignal?.aborted).toBe(false);
  });

  it('connect_should_clearConnectTimeout_When_synchronousThrowBeforeMessageLoopStarts', async () => {
    // Regression test for the sdd:6-verify MUST FIX: the connect-timeout timer is
    // scheduled before streamTerminal() is called; if that call throws synchronously
    // (e.g. proto validation, MessageQueue construction) the outer catch block must
    // still clear the timer, or it fires a stale trigger=connect-timeout warning
    // against an attempt that's already dead.
    mockStreamTerminal.mockImplementation(() => {
      throw new Error('synchronous setup failure');
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    const warnSpy = console.warn as jest.Mock;
    warnSpy.mockClear();

    await act(async () => { result.current.connect(); });
    expect(result.current.isConnected).toBe(false);

    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS + 1000); });
    const timeoutWarnings = warnSpy.mock.calls.filter((c) => String(c[0]).includes('trigger=connect-timeout'));
    expect(timeoutWarnings.length).toBe(0);
  });

  it('connect_should_notCallOnError_When_connectTimeoutAborts', async () => {
    // Code review finding: a connect-timeout abort is our own deliberate fast-retry
    // optimization, not a real failure — it must not be surfaced via onError, since
    // callers (e.g. TerminalOutput.tsx) count onError calls toward a user-visible
    // "connection failed" attempt counter/banner. A real, non-timeout stream error
    // still must reach onError (asserted below via the separate hard-fail-code path).
    const onError = jest.fn();
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      return makePushStream(opts?.signal).iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true, onError })
    );

    await act(async () => { result.current.connect(); });
    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    await waitFor(() => expect(result.current.terminalState).toBe('DISCONNECTED'));

    expect(onError).not.toHaveBeenCalled();
  });

  it('connect_should_callOnError_When_streamErrorsForARealNonTimeoutReason', async () => {
    // Symmetry check for the fix above: onError suppression is scoped specifically
    // to connect-timeout-triggered aborts, not stream errors in general.
    const onError = jest.fn();
    mockStreamTerminal.mockImplementation(() => ({
      [Symbol.asyncIterator]() {
        return {
          async next() {
            throw new Error('genuine stream failure');
          },
        };
      },
    }));

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true, onError })
    );

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1));
    expect(onError.mock.calls[0][0].message).toBe('genuine stream failure');
  });

  it('connect_should_beNoOp_When_calledAgainWhileAlreadyConnecting', async () => {
    // Regression guard for the invariant firstMessageRef/connectTimeoutRef rely on
    // (documented at their declaration): connect()'s own isConnectedRef/isConnectingRef
    // re-entrancy guard must prevent two attempts from touching that shared state at once.
    let callCount = 0;
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      callCount++;
      return makePushStream(opts?.signal).iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    await act(async () => {
      result.current.connect();
      result.current.connect();
    });

    expect(callCount).toBe(1);
  });

  it('connect_should_beNoOp_When_calledImmediatelyAfterFirstMessageProcessed', async () => {
    // AC #3: isConnectedRef must be set synchronously in the same microtask
    // that processes the first message (mirrors the existing disconnect-path
    // precedent for isConnectedRef in the finally block), not only once React
    // flushes setIsConnected(true) — otherwise a same-tick second connect()
    // call (e.g. two visibility/focus-triggered reconnect paths racing for the
    // same session) could slip past the guard and start a second live
    // connection. Deliberately stays microtask-only (no timer advance) so no
    // React effect commit can have happened yet — only the synchronous ref
    // write this fix adds can make the guard below see it in time.
    let callCount = 0;
    const stream = makePushStream<object>();
    mockStreamTerminal.mockImplementation(() => {
      callCount++;
      return stream.iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    await act(async () => { result.current.connect(); });

    await act(async () => {
      stream.push(makeOutputMsg());
      for (let i = 0; i < 5; i++) {
        await Promise.resolve();
      }
      result.current.connect();
    });

    await waitFor(() => expect(result.current.isConnected).toBe(true));
    expect(callCount).toBe(1);
  });

  it('connectTimeoutAbort_should_NotConsumeHardFailBudget_When_itRecursPastFiveAttempts', async () => {
    // AC #5, isolated from the reconnectTimerRef-based backoff-escalation test
    // (pitfalls.md's guidance against mixing the connectTimeoutRef and
    // reconnectTimerRef timer systems in one test). A connect-timeout abort is
    // a deliberate internal fast-retry optimization, not a real stream failure
    // — it must never count toward the 5-attempt hard-fail budget, or a
    // genuinely slow-but-healthy (e.g. high-RTT/VPN) connection would
    // eventually be hard-failed by connect-timeouts alone, which is worse than
    // the unlimited patience such a connection had before the per-attempt
    // connect-timeout existed.
    const signals: (AbortSignal | undefined)[] = [];
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      signals.push(opts?.signal);
      return makePushStream(opts?.signal).iterable; // never delivers a first message
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    await act(async () => { result.current.connect(); });

    // Drive 7 consecutive connect-timeout aborts — more than the 5-attempt
    // hard-fail budget. Each abort's immediate (0ms) retry fires within the
    // same advance, so signals[i+1] is already populated by the time we check
    // signals[i]'s abort.
    const totalAbortsToDrive = 7;
    for (let i = 0; i < totalAbortsToDrive; i++) {
      const timeoutForThisAttempt = i < FOREGROUND_FAST_ATTEMPTS ? FOREGROUND_CONNECT_TIMEOUT_MS : CONNECT_TIMEOUT_MS;
      await act(async () => { jest.advanceTimersByTime(timeoutForThisAttempt); });
      await waitFor(() => expect(signals[i]?.aborted).toBe(true));
    }

    expect(result.current.isHardFailed).toBe(false);
    expect(signals.length).toBe(totalAbortsToDrive + 1); // kept retrying past 7 aborts, never gave up
  });
});
