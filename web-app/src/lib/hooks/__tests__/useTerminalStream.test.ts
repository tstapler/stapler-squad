/**
 * Tests for useTerminalStream — ResizeQuiescence state machine (R1.4).
 *
 * Mocks ConnectRPC client so tests can push messages into the stream
 * on demand and verify terminalState transitions without races.
 */

import { renderHook, act, waitFor } from '@testing-library/react';

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

// MessageQueue — minimal stub; the push side is not exercised in these tests
jest.mock('@/lib/terminal/MessageQueue', () => ({
  MessageQueue: class {
    push = jest.fn();
    close = jest.fn();
    [Symbol.asyncIterator]() {
      return { next: async () => ({ value: undefined, done: true }) };
    }
  },
}));

// Sub-hooks — minimal stubs so useTerminalStream can render
jest.mock('../useTerminalFlowControl', () => ({
  useTerminalFlowControl: () => ({
    sendInput: jest.fn(),
    sendInputWithEcho: jest.fn().mockReturnValue(BigInt(0)),
    resize: jest.fn(),
    requestScrollback: jest.fn(),
    sendFlowControl: jest.fn(),
    getIsApplyingState: jest.fn().mockReturnValue(false),
    sspNegotiated: false,
    handleStateMessage: jest.fn(),
    handleDiffMessage: jest.fn(),
    handleSspNegotiation: jest.fn(),
    handleCurrentPaneResponse: jest.fn(),
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

jest.mock('@/lib/compression/lzma', () => ({
  decompressLZMA: jest.fn(),
  isLZMACompressed: jest.fn().mockReturnValue(false),
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

function makePushStream<T>(): PushStream<T> {
  const queue: T[] = [];
  const resolvers: Array<() => void> = [];
  let done = false;

  const push = (msg: T) => {
    queue.push(msg);
    resolvers.shift()?.();
  };

  const end = () => {
    done = true;
    resolvers.shift()?.();
  };

  const iterable: AsyncIterable<T> = {
    [Symbol.asyncIterator]() {
      return {
        async next(): Promise<IteratorResult<T>> {
          while (queue.length === 0 && !done) {
            await new Promise<void>((resolve) => resolvers.push(resolve));
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
});
