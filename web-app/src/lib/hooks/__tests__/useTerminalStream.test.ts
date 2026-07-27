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
jest.mock('@connectrpc/connect', () => ({
  createClient: () => ({ streamTerminal: mockStreamTerminal }),
}));

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

function makeOutputMsg() {
  return {
    data: { case: 'output', value: { data: new TextEncoder().encode('hello') } },
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
    mockMessageQueueInstances.length = 0;
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

  // Task 2.2.5
  it("overlapping connect() only lets the newer generation's state win", async () => {
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
      result.current.connect(); // generation 2, before stream1's first message resolves
    });

    // Deliver stream1's first message — the superseded generation must not
    // flip isConnected/terminalState.
    await act(async () => {
      stream1.push(makeOutputMsg());
    });

    expect(result.current.isConnected).toBe(false);

    // Deliver stream2's first message — only the newer generation's state wins.
    await act(async () => {
      stream2.push(makeOutputMsg());
    });

    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });
    expect(result.current.terminalState).toBe('STABLE');

    stream2.end();
  });

  // Task 2.2.6
  it('three rapid connect() calls do not throw or leak', async () => {
    const stream1 = makePushStream<object>();
    const stream2 = makePushStream<object>();
    const stream3 = makePushStream<object>();
    mockStreamTerminal
      .mockReturnValueOnce(stream1.iterable)
      .mockReturnValueOnce(stream2.iterable)
      .mockReturnValueOnce(stream3.iterable);

    const { result } = renderHook(() =>
      useTerminalStream({ ...BASE_OPTIONS, autoConnect: false }),
    );

    await act(async () => {
      // All three calls happen synchronously in the same tick — simulates
      // StrictMode's double-invoke plus a genuine reconnect.
      result.current.connect();
      result.current.connect();
      result.current.connect();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockMessageQueueInstances).toHaveLength(3);
    // The first two generations' queues were torn down by the later connect() calls.
    expect(mockMessageQueueInstances[0].close).toHaveBeenCalled();
    expect(mockMessageQueueInstances[1].close).toHaveBeenCalled();
    // The third instance is the one referenced going forward — never closed.
    expect(mockMessageQueueInstances[2].closed).toBe(false);

    stream3.end();
  });

  // Task 2.2.7
  it("disconnect() racing a concurrent connect() does not tear down the newer generation's queue/controller", async () => {
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
      // Simulate an unmount cleanup (disconnect) racing an auto-reconnect
      // timer (connect) firing back-to-back in the same tick.
      result.current.disconnect();
      result.current.connect(); // generation 2
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockMessageQueueInstances).toHaveLength(2);
    const gen1Queue = mockMessageQueueInstances[0];
    const gen2Queue = mockMessageQueueInstances[1];

    // Generation 1's queue was torn down (by the interleaved disconnect()).
    expect(gen1Queue.close).toHaveBeenCalled();
    // Generation 2's queue/controller survive — not torn down or left null.
    expect(gen2Queue.closed).toBe(false);

    // Confirm generation 2 is actually the live connection: its stream can
    // still flip isConnected.
    await act(async () => {
      stream2.push(makeOutputMsg());
    });
    await waitFor(() => {
      expect(result.current.isConnected).toBe(true);
    });

    stream2.end();
  });

  // Task 2.2.8
  it('a message pushed to the live MessageQueue right as a reconnect closes it is dropped, not delivered to either connection', async () => {
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

    await act(async () => {
      // Genuinely interleave: push input, then trigger the reconnect that
      // closes this exact queue, in the same tick — not push-then-close-
      // then-check in separate steps.
      gen1Queue.push(droppedMessage);
      result.current.connect(); // generation 2 — closes gen1Queue synchronously
      await Promise.resolve();
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
