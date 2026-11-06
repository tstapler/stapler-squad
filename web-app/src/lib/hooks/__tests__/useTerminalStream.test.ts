/**
 * Tests for useTerminalStream — ResizeQuiescence state machine (R1.4).
 *
 * Mocks ConnectRPC client so tests can push messages into the stream
 * on demand and verify terminalState transitions without races.
 */

import { renderHook, act, waitFor } from '@testing-library/react';
import {
  FOREGROUND_CONNECT_TIMEOUT_MS, CONNECT_TIMEOUT_MS, FOREGROUND_FAST_ATTEMPTS, CONNECT_TIMEOUT_ABORT_CAP,
  RECONNECT_BASE_MS, RECONNECT_CAP_MS, GAVE_UP_AFTER_ATTEMPTS, SERVER_END_GIVE_UP_MS,
} from '@/lib/utils/backoff';
import {
  CONNECT_GATE_WIDTH, queueForConnectSlot, resetConnectGateForTests, tryAcquireConnectSlot,
} from '@/lib/utils/connectGate';

// Wrapped so individual tests can script the gate; beforeEach restores the real implementation.
jest.mock('@/lib/utils/connectGate', () => {
  const actual = jest.requireActual('@/lib/utils/connectGate');
  return {
    ...actual,
    queueForConnectSlot: jest.fn(actual.queueForConnectSlot),
    tryAcquireConnectSlot: jest.fn(actual.tryAcquireConnectSlot),
  };
});

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
    Code: { Unauthenticated: 16, NotFound: 5, FailedPrecondition: 9 },
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
import { defined } from "@/lib/utils/defined";

// The connect gate is module-level state shared by every hook instance in this file.
beforeEach(() => {
  const actual = jest.requireActual('@/lib/utils/connectGate');
  (queueForConnectSlot as jest.Mock).mockReset().mockImplementation(actual.queueForConnectSlot);
  (tryAcquireConnectSlot as jest.Mock).mockReset().mockImplementation(actual.tryAcquireConnectSlot);
  resetConnectGateForTests();
});

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
            return { value: defined(queue.shift()), done: false };
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

describe('useTerminalStream — auto-reconnect', () => {
  const RECONNECT_OPTIONS = {
    ...BASE_OPTIONS,
    autoConnect: false,
  };

  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'debug').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
    jest.spyOn(console, 'info').mockImplementation(() => {});
    mockStreamTerminal.mockReset();
  });

  afterEach(() => {
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

  it('connect_should_surfaceError_When_serverEndsStreamCleanly', async () => {
    // The surfaced error drives the pane's "Reconnecting (attempt n/5)" status.
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);
    const onError = jest.fn();

    const { result } = renderHook(() => useTerminalStream({ ...RECONNECT_OPTIONS, onError }));
    await act(async () => { result.current.connect(); });
    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    await act(async () => { stream.end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    expect(result.current.error?.message).toContain('closed by server');
    expect(onError).toHaveBeenCalledTimes(1);
  });

  it('connect_should_stopSurfacingErrors_When_streamsKeepEndingCleanlyRightAfterConnecting', async () => {
    mockStreamTerminal.mockImplementation(() => {
      const s = makePushStream<object>();
      s.end();
      return s.iterable;
    });
    const onError = jest.fn();

    const { result } = renderHook(() => useTerminalStream({ ...RECONNECT_OPTIONS, onError }));
    for (let i = 0; i < 8; i++) {
      await act(async () => { await result.current.connect(); });
      await act(async () => { await Promise.resolve(); });
    }

    expect(onError.mock.calls.length).toBeGreaterThan(0);
    expect(onError.mock.calls.length).toBeLessThanOrEqual(5);
  });

  it('connect_should_notSurfaceError_When_shellStreamEndsCleanly', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);
    const onError = jest.fn();

    const { result } = renderHook(() => useTerminalStream({ ...RECONNECT_OPTIONS, shellId: 'shell-1', onError }));
    await act(async () => { result.current.connect(); });
    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    await act(async () => { stream.end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    expect(onError).not.toHaveBeenCalled();
  });

  it('disconnect_should_notSurfaceError_When_streamEndsAfterUserDisconnect', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);
    const onError = jest.fn();

    const { result } = renderHook(() => useTerminalStream({ ...RECONNECT_OPTIONS, onError }));
    await act(async () => { result.current.connect(); });
    await act(async () => { stream.push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    await act(async () => { result.current.disconnect(); stream.end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    expect(onError).not.toHaveBeenCalled();
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
    // The backoff delay is Math.random() * 1000ms. Pin it near its ceiling so the
    // natural reconnect cannot fire before the visibility path (a ~0ms draw used to
    // make the natural reconnect win, leaving the visibility connect() a no-op).
    jest.spyOn(Math, 'random').mockReturnValue(0.999);

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // The natural backoff reconnect is still pending: nothing has reconnected yet.
    expect(callCount).toBe(1);

    await act(async () => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300); // 200ms debounce, well under the ~999ms backoff
    });

    // Exactly one connect, and it came from the visibility path.
    expect(callCount).toBe(2);
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
    // Backoff delay is Math.random() * 1000ms. Pin it near the ceiling so the natural
    // reconnect cannot fire inside the 300ms window and make the online connect() a no-op
    // (a ~0ms draw made it fire early, leaving the count flat).
    jest.spyOn(Math, 'random').mockReturnValue(0.999);

    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // The natural backoff reconnect is still pending: nothing has reconnected yet.
    expect(callCount).toBe(1);

    await act(async () => {
      window.dispatchEvent(new Event('online'));
      jest.advanceTimersByTime(300); // 200ms debounce, well under the ~999ms backoff
    });

    // Exactly one connect, and it came from the online path.
    expect(callCount).toBe(2);
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

  it('connect_should_escalateDelayToCapAndKeepRetrying_When_serverKeepsEndingStreamCleanly', async () => {
    // Full-jitter delay is Math.random() * ceiling — pin Math.random() to 1 (the
    // supremum) so each cycle's delay is deterministically the per-attempt
    // ceiling (2000/4000/8000/16000/30000ms) instead of anywhere in [0, ceiling),
    // making the escalation boundaries below exact rather than flaky.
    jest.spyOn(Math, 'random').mockReturnValue(1);
    const ceilings = [2000, 4000, 8000, 16000, 30_000];
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

    // Server-ended closes keep retrying at the capped ceiling (the server may still be loading after a
    // restart); they never show "server unreachable" and do not hard-fail until the give-up window ends.
    await act(async () => { streams[5].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));
    expect(result.current.isHardFailed).toBe(false);
    expect(result.current.isServerUnreachable).toBe(false);
    await act(async () => { jest.advanceTimersByTime(RECONNECT_CAP_MS); });
    await waitFor(() => expect(streams.length).toBe(7));
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
    // 4000ms).
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));
    await act(async () => { jest.advanceTimersByTime(RECONNECT_BASE_MS); });
    await waitFor(() => expect(streams.length).toBe(2));
    await act(async () => { streams[1].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { streams[1].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    // Sequence is now mid-backoff (waiting up to a 4000ms ceiling for the 3rd
    // attempt). Call connect() directly — the same call useVisibilityResync's
    // reconnect paths make — before that delay elapses.
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(3));
    await act(async () => { streams[2].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    // If connect() reset the sequence (as it must), the next automatic retry's
    // delay is back to the attempt-0 ceiling (2000ms) — not the stale 4000ms it
    // would still be if reset() were skipped for a direct call like this one.
    await act(async () => { streams[2].end(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));
    await act(async () => { jest.advanceTimersByTime(RECONNECT_BASE_MS); });
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
    jest.useFakeTimers();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'debug').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
    jest.spyOn(console, 'info').mockImplementation(() => {});
    mockStreamTerminal.mockReset();
  });

  afterEach(() => {
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
    // disconnect() itself aborts the in-flight attempt; that is not a timeout abort.
    expect(capturedSignal?.aborted).toBe(true);
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


  type Push = ReturnType<typeof makePushStream<object>>;

  /** Background connect, attach, clean drop, then foreground: a resume reconnect, eligible for the fast timeout. */
  async function resumeIntoForeground() {
    const signals: (AbortSignal | undefined)[] = [];
    const streams: Push[] = [];
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      signals.push(opts?.signal);
      const s = makePushStream<object>(opts?.signal);
      streams.push(s);
      return s.iterable;
    });
    const hook = renderHook(
      (props: { foreground: boolean }) => useTerminalStream({ ...RECONNECT_OPTIONS, foreground: props.foreground }),
      { initialProps: { foreground: false } }
    );
    await act(async () => { hook.result.current.connect(); });
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(hook.result.current.isConnected).toBe(true));
    await act(async () => { streams[0].end(); });
    await waitFor(() => expect(hook.result.current.isConnected).toBe(false));
    await act(async () => { hook.rerender({ foreground: true }); });
    await waitFor(() => expect(streams.length).toBe(2));
    return { ...hook, signals, streams };
  }

  /** Never delivers a first message; every attempt ends by connect-timeout abort. */
  function trackHangingStreams() {
    const signals: (AbortSignal | undefined)[] = [];
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      signals.push(opts?.signal);
      return makePushStream(opts?.signal).iterable;
    });
    return signals;
  }

  it('connect_should_useFastTimeout_When_foregroundResumeReconnects', async () => {
    const { signals } = await resumeIntoForeground();
    expect(signals[1]?.aborted).toBe(false);
    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    expect(signals[1]?.aborted).toBe(true);
  });

  it('connect_should_useNormalTimeout_When_foregroundPaneConnectsInitially', async () => {
    const signals = trackHangingStreams();
    const { result } = renderHook(() => useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true }));

    await act(async () => { result.current.connect(); });
    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    expect(signals[0]?.aborted).toBe(false);

    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS - FOREGROUND_CONNECT_TIMEOUT_MS); });
    expect(signals[0]?.aborted).toBe(true);
  });

  it('connect_should_returnToNormalTimeout_When_resumeFastWindowIsExhausted', async () => {
    jest.spyOn(Math, 'random').mockReturnValue(1);
    const { signals } = await resumeIntoForeground();
    // Two fast attempts: signals[1] and signals[2]; each abort retries after its jittered delay (500ms, 1000ms).
    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    await act(async () => { jest.advanceTimersByTime(500); });
    await waitFor(() => expect(signals.length).toBe(3));
    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    await act(async () => { jest.advanceTimersByTime(1000); });
    await waitFor(() => expect(signals.length).toBe(FOREGROUND_FAST_ATTEMPTS + 2));

    await act(async () => { jest.advanceTimersByTime(FOREGROUND_CONNECT_TIMEOUT_MS); });
    expect(signals[3]?.aborted).toBe(false);
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS - FOREGROUND_CONNECT_TIMEOUT_MS); });
    expect(signals[3]?.aborted).toBe(true);
  });

  it('connect_should_retryAfterEscalatingJitteredDelay_When_connectTimeoutAborts', async () => {
    jest.spyOn(Math, 'random').mockReturnValue(1);
    const signals = trackHangingStreams();
    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));
    await act(async () => { result.current.connect(); });

    for (const [i, ceiling] of [500, 1000, 2000, 4000, 5000].entries()) {
      await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
      expect(signals[i]?.aborted).toBe(true);
      await act(async () => { jest.advanceTimersByTime(ceiling - 1); });
      expect(signals.length).toBe(i + 1); // never an immediate retry
      await act(async () => { jest.advanceTimersByTime(1); });
      await waitFor(() => expect(signals.length).toBe(i + 2));
    }
  });

  it('connect_should_fallIntoNormalBackoffWithoutHardFail_When_timeoutAbortsExceedCap', async () => {
    jest.spyOn(Math, 'random').mockReturnValue(1);
    const signals = trackHangingStreams();
    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));
    await act(async () => { result.current.connect(); });

    for (let i = 0; i < CONNECT_TIMEOUT_ABORT_CAP; i++) {
      await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
      await act(async () => { jest.advanceTimersByTime(5000); });
      await waitFor(() => expect(signals.length).toBe(i + 2));
    }
    // Abort number CAP+1: retried through the regular backoff (2000ms ceiling), not the 5000ms abort schedule.
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
    expect(signals.length).toBe(CONNECT_TIMEOUT_ABORT_CAP + 1);
    await act(async () => { jest.advanceTimersByTime(RECONNECT_BASE_MS - 1); });
    expect(signals.length).toBe(CONNECT_TIMEOUT_ABORT_CAP + 1);
    await act(async () => { jest.advanceTimersByTime(1); });
    await waitFor(() => expect(signals.length).toBe(CONNECT_TIMEOUT_ABORT_CAP + 2));
    expect(result.current.isHardFailed).toBe(false);
  });

  it('connect_should_restartAbortSchedule_When_aFirstMessageArrivesBetweenAborts', async () => {
    jest.spyOn(Math, 'random').mockReturnValue(1);
    const streams: Push[] = [];
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      const s = makePushStream<object>(opts?.signal);
      streams.push(s);
      return s.iterable;
    });
    const { result } = renderHook(() => useTerminalStream(RECONNECT_OPTIONS));
    await act(async () => { result.current.connect(); });

    // Two aborts (delays 500, 1000), then a third attempt that attaches and later closes cleanly.
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
    await act(async () => { jest.advanceTimersByTime(500); });
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
    await act(async () => { jest.advanceTimersByTime(1000); });
    await waitFor(() => expect(streams.length).toBe(3));
    await act(async () => { streams[2].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { streams[2].end(); });
    await act(async () => { jest.advanceTimersByTime(RECONNECT_BASE_MS); });
    await waitFor(() => expect(streams.length).toBe(4));

    // The next abort is back at the first rung (500ms), not the third (2000ms).
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
    await act(async () => { jest.advanceTimersByTime(499); });
    expect(streams.length).toBe(4);
    await act(async () => { jest.advanceTimersByTime(1); });
    await waitFor(() => expect(streams.length).toBe(5));
  });

  it('connect_should_notCallOnError_When_connectTimeoutAborts', async () => {
    // A connect-timeout abort is our own retry, not a failure: it must not reach onError, which
    // callers count toward a user-visible "connection failed" state.
    const onError = jest.fn();
    trackHangingStreams();
    const { result } = renderHook(() => useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true, onError }));

    await act(async () => { result.current.connect(); });
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
    await waitFor(() => expect(result.current.terminalState).toBe('DISCONNECTED'));

    expect(onError).not.toHaveBeenCalled();
  });

  it('connect_should_preserveEscalatedBackoff_When_aConnectTimeoutRetryInterleavesWithRealCloses', async () => {
    // A timeout excursion between two real closes must not reset an escalated backoff (the abort
    // retry passes isAutoRetry: true). Math.random pinned to 1 makes each delay its ceiling.
    jest.spyOn(Math, 'random').mockReturnValue(1);
    const streams: Push[] = [];
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      const s = makePushStream<object>(opts?.signal);
      streams.push(s);
      return s.iterable;
    });
    const { result } = renderHook(() => useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true }));

    // Real close #1 escalates backoff attempt 0 -> 1.
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(streams.length).toBe(1));
    await act(async () => { streams[0].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { streams[0].end(); });
    await act(async () => { jest.advanceTimersByTime(RECONNECT_BASE_MS); });
    await waitFor(() => expect(streams.length).toBe(2));

    // streams[1] times out; its abort retry (500ms) leaves the backoff alone.
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
    await act(async () => { jest.advanceTimersByTime(500); });
    await waitFor(() => expect(streams.length).toBe(3));

    // Real close #2 waits the attempt-1 ceiling (4000ms), not a reset attempt-0 one (2000ms).
    await act(async () => { streams[2].push(makeOutputMsg()); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { streams[2].end(); });
    await act(async () => { jest.advanceTimersByTime(RECONNECT_BASE_MS * 2 - 1); });
    expect(streams.length).toBe(3);
    await act(async () => { jest.advanceTimersByTime(1); });
    await waitFor(() => expect(streams.length).toBe(4));
  });
});

describe('useTerminalStream — handshake zero-size guard (Task 3.1.2d)', () => {
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

  const handshakeOf = (index: number) =>
    (mockMessageQueueInstances[index].queue[0] as { data: { value: Record<string, unknown> } }).data.value;

  it.each([
    ['0x0 override', 0, 0],
    ['0xN override', 0, 24],
    ['1x1 override', 1, 1],
  ])('connect_should_NotSendHandshakeDimensions_When_ContainerIsZeroSize (%s)', async (_label, cols, rows) => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValueOnce(stream.iterable);
    const { result } = renderHook(() => useTerminalStream({ ...BASE_OPTIONS, autoConnect: false }));

    await act(async () => {
      result.current.connect(cols, rows);
    });

    const req = handshakeOf(0);
    expect(req.targetCols).toBeUndefined();
    expect(req.targetRows).toBeUndefined();
    stream.end();
  });

  it('connect_should_NotSendHandshakeDimensions_When_HiddenTerminalReportsZeroSize', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValueOnce(stream.iterable);
    const getTerminal = () => ({ cols: 0, rows: 0 }) as never;
    const { result } = renderHook(() => useTerminalStream({ ...BASE_OPTIONS, autoConnect: false, getTerminal }));

    await act(async () => {
      result.current.connect();
    });

    expect(handshakeOf(0).targetCols).toBeUndefined();
    stream.end();
  });

  it('connect_should_SendHandshakeDimensions_When_SizeIsReal', async () => {
    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValueOnce(stream.iterable);
    const { result } = renderHook(() => useTerminalStream({ ...BASE_OPTIONS, autoConnect: false }));

    await act(async () => {
      result.current.connect(120, 40);
    });

    expect(handshakeOf(0)).toMatchObject({ targetCols: 120, targetRows: 40 });
    stream.end();
  });
});

// ---------------------------------------------------------------------------
// Shared mux socket drop — the reconnect is unconditional (ADR-023)
// ---------------------------------------------------------------------------

/** A stream that delivers one output frame, then fails with the pool's `mux socket closed` error when dropped. */
function makeDroppableStream() {
  let rejectNext: (err: Error) => void = () => {};
  let delivered = false;
  const iterable: AsyncIterable<object> = {
    [Symbol.asyncIterator]() {
      return {
        async next(): Promise<IteratorResult<object>> {
          if (!delivered) {
            delivered = true;
            return { value: makeOutputMsg(), done: false };
          }
          return new Promise((_resolve, reject) => { rejectNext = reject; });
        },
      };
    },
  };
  const drop = () => {
    const { ConnectError } = require('@connectrpc/connect');
    const err = new ConnectError('WebSocket read error: mux socket closed');
    err.metadata = new Headers({ 'ws-close-code': '1006' });
    rejectNext(err);
  };
  return { iterable, drop };
}

describe('useTerminalStream — reconnect after the shared mux socket drops', () => {
  const OPTIONS = { ...BASE_OPTIONS, autoConnect: false };
  let randomSpy: jest.SpyInstance;

  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'debug').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
    jest.spyOn(console, 'info').mockImplementation(() => {});
    randomSpy = jest.spyOn(Math, 'random').mockReturnValue(0.5);
    mockStreamTerminal.mockReset();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  function trackDroppableStreams() {
    const droppable: ReturnType<typeof makeDroppableStream>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      const s = makeDroppableStream();
      droppable.push(s);
      return s.iterable;
    });
    return droppable;
  }

  it('connect_should_reconnectAfterBackoff_When_streamDropsWithMuxSocketClosed1006', async () => {
    const droppable = trackDroppableStreams();
    const { result } = renderHook(() => useTerminalStream(OPTIONS));

    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    await act(async () => { droppable[0].drop(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));
    expect(result.current.terminalState).toBe('DISCONNECTED');
    expect(result.current.isHardFailed).toBe(false);
    expect(mockStreamTerminal).toHaveBeenCalledTimes(1);

    // First retry delay = random(0.5) * 2000ms; nothing may fire earlier.
    await act(async () => { jest.advanceTimersByTime(999); });
    expect(mockStreamTerminal).toHaveBeenCalledTimes(1);
    await act(async () => { jest.advanceTimersByTime(1); });
    await waitFor(() => expect(mockStreamTerminal).toHaveBeenCalledTimes(2));

    // disconnected -> reconnecting -> connected
    await waitFor(() => expect(result.current.isConnected).toBe(true));
  });

  it('connect_should_staggerRetries_When_manyStreamsDropTogether', async () => {
    const droppable = trackDroppableStreams();
    const hooks = ['a', 'b', 'c'].map((sessionId) => renderHook(() => useTerminalStream({ ...OPTIONS, sessionId })));
    for (const h of hooks) {
      await act(async () => { h.result.current.connect(); });
      await waitFor(() => expect(h.result.current.isConnected).toBe(true));
    }
    expect(droppable).toHaveLength(3);
    const callsBeforeDrop = mockStreamTerminal.mock.calls.length;

    const randoms = [0.1, 0.5, 0.9];
    randomSpy.mockImplementation(() => randoms.shift() ?? 0.5);
    await act(async () => { droppable.forEach((d) => d.drop()); });
    await waitFor(() => hooks.forEach((h) => expect(h.result.current.isConnected).toBe(false)));

    const retried = () => mockStreamTerminal.mock.calls.length - callsBeforeDrop;
    await act(async () => { jest.advanceTimersByTime(199); });
    expect(retried()).toBe(0);
    await act(async () => { jest.advanceTimersByTime(1); });
    expect(retried()).toBe(1); // 0.1 * 2000ms = 200ms
    await act(async () => { jest.advanceTimersByTime(800); });
    expect(retried()).toBe(2); // 0.5 * 2000ms = 1000ms
    await act(async () => { jest.advanceTimersByTime(800); });
    expect(retried()).toBe(3); // 0.9 * 2000ms = 1800ms
  });

  it.each(['4001', '4004'])('connect_should_notReconnect_When_dropIsDefinitive_%s', async (code) => {
    const { ConnectError } = require('@connectrpc/connect');
    mockStreamTerminal.mockImplementation(() => ({
      [Symbol.asyncIterator]: () => ({
        async next() {
          const err = new ConnectError('gone');
          err.metadata = new Headers({ 'ws-close-code': code });
          throw err;
        },
      }),
    }));
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(result.current.isHardFailed).toBe(true));
    await act(async () => { jest.advanceTimersByTime(60_000); });
    expect(mockStreamTerminal).toHaveBeenCalledTimes(1);
  });

  it('connect_should_resetBackoff_When_streamStaysUpBeforeEachDrop', async () => {
    // Six drops far apart must each retry within the first-attempt window instead of
    // accumulating toward the 5-attempt hard-fail cap.
    randomSpy.mockReturnValue(0.99);
    const droppable = trackDroppableStreams();
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));

    for (let i = 0; i < 6; i++) {
      await act(async () => { jest.advanceTimersByTime(10_000); }); // stable attach
      await act(async () => { droppable[i].drop(); });
      await waitFor(() => expect(result.current.isConnected).toBe(false));
      expect(result.current.isHardFailed).toBe(false);
      await act(async () => { jest.advanceTimersByTime(RECONNECT_BASE_MS); }); // ceiling of attempt 0
      await waitFor(() => expect(result.current.isConnected).toBe(true));
    }
    expect(mockStreamTerminal).toHaveBeenCalledTimes(7);
  });

  it('connect_should_logWillReconnectTrue_When_dropScheduledARetry', async () => {
    const droppable = trackDroppableStreams();
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await act(async () => { droppable[0].drop(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));

    const line = (console.warn as jest.Mock).mock.calls.map((c) => String(c[0])).find((m) => m.includes('[stream-disconnect]'));
    expect(line).toContain('willReconnect=true');
  });

  /** Errors immediately with the given close code, the way a refused or unreachable socket does. */
  function failingStream(wsCloseCode: string, connectCode?: number) {
    const { ConnectError } = require('@connectrpc/connect');
    return {
      [Symbol.asyncIterator]: () => ({
        async next(): Promise<IteratorResult<object>> {
          const err = new ConnectError('stream failed');
          err.metadata = new Headers(wsCloseCode ? { 'ws-close-code': wsCloseCode } : {});
          if (connectCode !== undefined) err.code = connectCode;
          throw err;
        },
      }),
    };
  }

  async function advanceSeconds(seconds: number) {
    for (let i = 0; i < seconds; i++) {
      await act(async () => { jest.advanceTimersByTime(1000); });
    }
  }

  it('connect_should_reconnectAfterSixtySecondOutage_When_serverReturns', async () => {
    randomSpy.mockReturnValue(1);
    const outageEndsAt = Date.now() + 60_000;
    mockStreamTerminal.mockImplementation(() =>
      Date.now() < outageEndsAt ? failingStream('1006') : makeDroppableStream().iterable
    );
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });

    await advanceSeconds(45);
    expect(result.current.isServerUnreachable).toBe(true);
    expect(result.current.isHardFailed).toBe(false);

    await advanceSeconds(45); // outage over; the capped 30s backoff fires
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    expect(result.current.isServerUnreachable).toBe(false);
  });

  it.each([
    ['ws 4001', '4001', undefined],
    ['ws 4004', '4004', undefined],
    ['Unauthenticated', '', 16],
    ['NotFound', '', 5],
    ['FailedPrecondition (missing worktree)', '', 9],
  ])('connect_should_stayHardFailed_When_dropIsDefinitive_%s', async (_label, wsCode, connectCode) => {
    mockStreamTerminal.mockImplementation(() => failingStream(wsCode, connectCode));
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });
    await waitFor(() => expect(result.current.isHardFailed).toBe(true));
    await advanceSeconds(120);
    expect(mockStreamTerminal).toHaveBeenCalledTimes(1);
    expect(result.current.isServerUnreachable).toBe(false);
  });

  it('connect_should_reviveUnreachablePane_When_tabBecomesVisible', async () => {
    randomSpy.mockReturnValue(1);
    let serverUp = false;
    mockStreamTerminal.mockImplementation(() => (serverUp ? makeDroppableStream().iterable : failingStream('1006')));
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });
    await advanceSeconds(45);
    expect(result.current.isServerUnreachable).toBe(true);

    serverUp = true;
    await act(async () => { document.dispatchEvent(new Event('visibilitychange')); });
    await act(async () => { jest.advanceTimersByTime(200); }); // handler debounce, far below the 30s backoff
    await waitFor(() => expect(result.current.isConnected).toBe(true));
  });

  it('connect_should_notSpendUnreachableBudget_When_serverRefusesStreams1013', async () => {
    mockStreamTerminal.mockImplementation(() => failingStream('1013'));
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });

    await advanceSeconds(10 * GAVE_UP_AFTER_ATTEMPTS * 3);
    expect(mockStreamTerminal.mock.calls.length).toBeGreaterThan(GAVE_UP_AFTER_ATTEMPTS);
    expect(result.current.isServerUnreachable).toBe(false);
    expect(result.current.isHardFailed).toBe(false);
  });

  it('connect_should_hardFailWithoutUnreachable_When_serverKeepsSendingErrorFramesPastTheGiveUpWindow', async () => {
    mockStreamTerminal.mockImplementation(() => {
      const s = makePushStream<object>();
      s.push(makeErrorMsg('internal hiccup', 'INTERNAL'));
      s.end();
      return s.iterable;
    });
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });

    await advanceSeconds(SERVER_END_GIVE_UP_MS / 1000 + RECONNECT_CAP_MS / 1000);
    expect(result.current.isHardFailed).toBe(true);
    expect(result.current.isServerUnreachable).toBe(false);
    const calls = mockStreamTerminal.mock.calls.length;
    await advanceSeconds(120);
    expect(mockStreamTerminal.mock.calls.length).toBe(calls);
  });

  it('connect_should_keepUnreachable_When_streamDiesSoonAfterFirstMessage', async () => {
    randomSpy.mockReturnValue(1);
    let serverUp = false;
    const droppable: ReturnType<typeof makeDroppableStream>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      if (!serverUp) return failingStream('1006');
      const s = makeDroppableStream();
      droppable.push(s);
      return s.iterable;
    });
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });
    await advanceSeconds(45);
    expect(result.current.isServerUnreachable).toBe(true);

    serverUp = true;
    await act(async () => { result.current.handleManualReconnect(); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await advanceSeconds(3);
    await act(async () => { droppable[0].drop(); });
    await waitFor(() => expect(result.current.isConnected).toBe(false));
    expect(result.current.isServerUnreachable).toBe(true); // no flicker on a short-lived attach

    await act(async () => { jest.advanceTimersByTime(RECONNECT_CAP_MS); });
    await waitFor(() => expect(result.current.isConnected).toBe(true));
    await advanceSeconds(11); // stable: now it clears
    expect(result.current.isServerUnreachable).toBe(false);
  });

  it('connect_should_surviveTenMinuteRestartWindow_When_serverAnswersErrorFramesWhileLoading', async () => {
    randomSpy.mockReturnValue(1);
    const loadedAt = Date.now() + 8 * 60_000;
    mockStreamTerminal.mockImplementation(() => {
      if (Date.now() >= loadedAt) return makeDroppableStream().iterable;
      const s = makePushStream<object>();
      s.push(makeErrorMsg('sessions still loading', 'UNAVAILABLE'));
      s.end();
      return s.iterable;
    });
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });

    await advanceSeconds(10 * 60);
    expect(result.current.isHardFailed).toBe(false);
    await waitFor(() => expect(result.current.isConnected).toBe(true));
  });

  it('connect_should_notShowUnreachable_When_firstTransportFailureFollowsManyServerEndedCloses', async () => {
    randomSpy.mockReturnValue(1);
    let calls = 0;
    mockStreamTerminal.mockImplementation(() => {
      calls++;
      if (calls <= GAVE_UP_AFTER_ATTEMPTS) {
        const s = makePushStream<object>();
        s.push(makeErrorMsg('loading', 'UNAVAILABLE'));
        s.end();
        return s.iterable;
      }
      return failingStream('1006');
    });
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });

    await advanceSeconds(120);
    expect(calls).toBeGreaterThan(GAVE_UP_AFTER_ATTEMPTS);
    // Transport failures have their own attempt count, so a handful of them is not yet "unreachable".
    expect(result.current.isServerUnreachable).toBe(calls - GAVE_UP_AFTER_ATTEMPTS >= GAVE_UP_AFTER_ATTEMPTS);
  });

  it('connect_should_notHardFail_When_serverFlapsBetweenErrorFramesAndStableAttaches', async () => {
    randomSpy.mockReturnValue(1);
    let n = 0;
    const droppable: ReturnType<typeof makeDroppableStream>[] = [];
    mockStreamTerminal.mockImplementation(() => {
      n++;
      if (n % 2 === 1) {
        const s = makePushStream<object>();
        s.push(makeErrorMsg('loading', 'UNAVAILABLE'));
        s.end();
        return s.iterable;
      }
      const d = makeDroppableStream();
      droppable.push(d);
      return d.iterable;
    });
    const { result } = renderHook(() => useTerminalStream(OPTIONS));
    await act(async () => { result.current.connect(); });

    // A stable attach every other attempt resets the server-ended clock, so it never expires.
    for (let i = 0; i < 40 && !result.current.isHardFailed; i++) {
      await advanceSeconds(30);
      if (result.current.isConnected) {
        await advanceSeconds(11);
        await act(async () => { droppable[droppable.length - 1].drop(); });
      }
    }
    expect(result.current.isHardFailed).toBe(false);
    expect(n).toBeGreaterThan(20);
  });

  it('connect_should_surfaceError_When_timeoutAbortsExceedCap', async () => {
    const onError = jest.fn();
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => makePushStream(opts?.signal).iterable);
    const { result } = renderHook(() => useTerminalStream({ ...OPTIONS, onError }));
    await act(async () => { result.current.connect(); });

    for (let i = 0; i < CONNECT_TIMEOUT_ABORT_CAP; i++) {
      await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
      await advanceSeconds(6);
    }
    expect(onError).not.toHaveBeenCalled(); // within the cap the abort stays invisible
    await act(async () => { jest.advanceTimersByTime(CONNECT_TIMEOUT_MS); });
    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1));
  });

  describe('connect gate', () => {
    function trackSessions() {
      const order: string[] = [];
      const streams: ReturnType<typeof makePushStream<object>>[] = [];
      mockStreamTerminal.mockImplementation((queue: { queue: Array<{ sessionId: string }> }, opts?: { signal?: AbortSignal }) => {
        order.push(queue.queue[0].sessionId);
        const s = makePushStream<object>(opts?.signal);
        streams.push(s);
        return s.iterable;
      });
      return { order, streams };
    }

    it('connect_should_limitConcurrentAttempts_When_manyPanesConnectAtOnce', async () => {
      const { order, streams } = trackSessions();
      const hooks = Array.from({ length: CONNECT_GATE_WIDTH + 4 }, (_, i) =>
        renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: `s${i}` }))
      );
      await act(async () => { hooks.forEach((h) => h.result.current.connect()); });

      expect(order).toHaveLength(CONNECT_GATE_WIDTH);

      await act(async () => { streams[0].push(makeOutputMsg()); });
      await waitFor(() => expect(order).toHaveLength(CONNECT_GATE_WIDTH + 1));
    });

    it('connect_should_letForegroundPaneJumpTheQueue_When_slotsAreBusy', async () => {
      const { order, streams } = trackSessions();
      const busy = Array.from({ length: CONNECT_GATE_WIDTH }, (_, i) =>
        renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: `busy${i}` }))
      );
      await act(async () => { busy.forEach((h) => h.result.current.connect()); });
      const background = renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: 'background', foreground: false }));
      const foreground = renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: 'foreground', foreground: true }));
      await act(async () => { background.result.current.connect(); });
      await act(async () => { foreground.result.current.connect(); });
      expect(order).toHaveLength(CONNECT_GATE_WIDTH);

      await act(async () => { streams[0].push(makeOutputMsg()); });
      await waitFor(() => expect(order).toHaveLength(CONNECT_GATE_WIDTH + 1));
      expect(order[CONNECT_GATE_WIDTH]).toBe('foreground');
    });

    async function fillGate(prefix: string) {
      const busy = Array.from({ length: CONNECT_GATE_WIDTH }, (_, i) =>
        renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: `${prefix}${i}` }))
      );
      await act(async () => { busy.forEach((h) => h.result.current.connect()); });
      return busy;
    }

    it('connect_should_freeSlotForNextWaiter_When_queuedPaneDisconnects', async () => {
      const { order } = trackSessions();
      const busy = await fillGate('busy');
      const queued = renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: 'queued' }));
      const next = renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: 'next' }));
      await act(async () => { queued.result.current.connect(); });
      await act(async () => { next.result.current.connect(); });
      await act(async () => { await queued.result.current.disconnect(); });

      // A real slot frees up: the cancelled waiter must not consume it, the next waiter must.
      await act(async () => { await busy[0].result.current.disconnect(); });
      await waitFor(() => expect(order).toContain('next'));
      expect(order).not.toContain('queued');
    });

    it('connect_should_notStartAGhostConnect_When_slotIsGrantedAroundDisconnect', async () => {
      const { order } = trackSessions();
      const busy = await fillGate('busy');
      const queued = renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: 'queued' }));
      await act(async () => { queued.result.current.connect(); });

      await act(async () => {
        const freed = busy[0].result.current.disconnect();
        await queued.result.current.disconnect();
        await freed;
      });
      await advanceSeconds(2);
      expect(order).not.toContain('queued');
    });

    it('disconnect_should_abortAndHoldSlot_When_connectIsInFlightBeforeFirstMessage', async () => {
      const signals: AbortSignal[] = [];
      const order: string[] = [];
      mockStreamTerminal.mockImplementation((queue: { queue: Array<{ sessionId: string }> }, opts?: { signal?: AbortSignal }) => {
        order.push(queue.queue[0].sessionId);
        if (opts?.signal) signals.push(opts.signal);
        return makePushStream<object>(opts?.signal).iterable;
      });
      const busy = await fillGate('busy');
      const waiting = renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: 'waiting' }));
      await act(async () => { waiting.result.current.connect(); });
      expect(order).toHaveLength(CONNECT_GATE_WIDTH);

      await act(async () => { await busy[0].result.current.disconnect(); });
      expect(signals[0].aborted).toBe(true); // the in-flight stream is really closed, not just forgotten
      await waitFor(() => expect(order).toContain('waiting'));
    });

    describe('queued connect races', () => {
      function scriptedGate({ cancelResolves }: { cancelResolves: boolean }) {
        let resolveSlot: (r: (() => void) | null) => void = () => {};
        const release = jest.fn();
        const cancel = jest.fn(() => { if (cancelResolves) resolveSlot(null); });
        (tryAcquireConnectSlot as jest.Mock).mockReturnValueOnce(null);
        (queueForConnectSlot as jest.Mock).mockImplementationOnce(() => ({
          promise: new Promise((resolve) => { resolveSlot = resolve; }),
          cancel,
        }));
        return { grant: () => resolveSlot(release), release, cancel };
      }

      it('connect_should_notStartStream_When_slotIsGrantedAfterDisconnectFinished', async () => {
        const gate = scriptedGate({ cancelResolves: false });
        const { result } = renderHook(() => useTerminalStream(OPTIONS));
        await act(async () => { result.current.connect(); });
        await act(async () => { await result.current.disconnect(); });

        await act(async () => { gate.grant(); });
        expect(mockStreamTerminal).not.toHaveBeenCalled();
        expect(gate.release).toHaveBeenCalledTimes(1);
        expect(result.current.terminalState).toBe('DISCONNECTED');
      });

      it('connect_should_notStartStream_When_slotIsGrantedInTheSameTickAsDisconnect', async () => {
        const gate = scriptedGate({ cancelResolves: false });
        const { result } = renderHook(() => useTerminalStream(OPTIONS));
        await act(async () => { result.current.connect(); });
        await act(async () => {
          gate.grant();
          await result.current.disconnect();
        });
        expect(mockStreamTerminal).not.toHaveBeenCalled();
        expect(gate.release).toHaveBeenCalledTimes(1);
      });

      it('connect_should_ignoreStaleGrant_When_aNewerConnectOwnsTheFlags', async () => {
        trackSessions();
        const gate = scriptedGate({ cancelResolves: false });
        const { result } = renderHook(() => useTerminalStream(OPTIONS));
        await act(async () => { result.current.connect(); }); // queued, generation 1
        await act(async () => { await result.current.disconnect(); });
        await act(async () => { result.current.connect(); }); // gets a real slot, generation 2
        expect(mockStreamTerminal).toHaveBeenCalledTimes(1);

        await act(async () => { gate.grant(); }); // generation 1 is granted late
        expect(mockStreamTerminal).toHaveBeenCalledTimes(1);
        expect(gate.release).toHaveBeenCalledTimes(1);
        expect(result.current.terminalState).toBe('CONNECTING'); // not clobbered by the stale attempt
      });

      it('disconnect_should_settleToDisconnected_When_aQueuedAttemptIsCancelled', async () => {
        const gate = scriptedGate({ cancelResolves: true });
        const { result } = renderHook(() => useTerminalStream(OPTIONS));
        await act(async () => { result.current.connect(); });
        expect(result.current.terminalState).toBe('CONNECTING');

        await act(async () => { await result.current.disconnect(); });
        expect(gate.cancel).toHaveBeenCalledTimes(1);
        expect(result.current.terminalState).toBe('DISCONNECTED');
        expect(mockStreamTerminal).not.toHaveBeenCalled();
      });
    });

    it('connect_should_connect_When_calledRightAfterDisconnectOfAQueuedPane', async () => {
      const { order } = trackSessions();
      const busy = await fillGate('busy');
      const pane = renderHook(() => useTerminalStream({ ...OPTIONS, sessionId: 'queued' }));
      await act(async () => { pane.result.current.connect(); });

      await act(async () => {
        void pane.result.current.disconnect();
        void pane.result.current.connect();
      });
      expect(pane.result.current.terminalState).toBe('CONNECTING');

      await act(async () => { await busy[0].result.current.disconnect(); });
      await waitFor(() => expect(order.filter((id) => id === 'queued')).toHaveLength(1));
    });

    it('connect_should_connectNewSession_When_sessionSwitchesWhileFirstConnectIsInFlight', async () => {
      const { order } = trackSessions();
      const { rerender } = renderHook(
        (props: { sessionId: string }) => useTerminalStream({ ...OPTIONS, autoConnect: true, sessionId: props.sessionId }),
        { initialProps: { sessionId: 'a' } }
      );
      await waitFor(() => expect(order).toEqual(['a']));

      await act(async () => { rerender({ sessionId: 'b' }); });
      await waitFor(() => expect(order).toEqual(['a', 'b']));
    });

    it('connect_should_connectNewSession_When_sessionSwitchesWhileFirstConnectIsQueued', async () => {
      const { order } = trackSessions();
      const busy = await fillGate('busy');
      const { rerender } = renderHook(
        (props: { sessionId: string }) => useTerminalStream({ ...OPTIONS, autoConnect: true, sessionId: props.sessionId }),
        { initialProps: { sessionId: 'a' } }
      );
      await act(async () => { rerender({ sessionId: 'b' }); });

      await act(async () => { await busy[0].result.current.disconnect(); });
      await waitFor(() => expect(order).toContain('b'));
      expect(order).not.toContain('a');
    });

    it('connect_should_startSecondStream_When_connectFollowsDisconnectOfAnInFlightAttempt', async () => {
      const { order } = trackSessions();
      const { result } = renderHook(() => useTerminalStream(OPTIONS));
      await act(async () => { result.current.connect(); });
      expect(order).toHaveLength(1);

      await act(async () => {
        void result.current.disconnect();
        void result.current.connect();
      });
      await waitFor(() => expect(order).toHaveLength(2));
    });
  });
});
