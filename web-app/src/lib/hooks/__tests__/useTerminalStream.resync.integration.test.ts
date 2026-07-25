/**
 * AC7 (terminal-visibility-resync) end-to-end integration test.
 *
 * AC7's text ("StateApplicator", "resetSequence()", "applyState()") describes
 * the mechanism speculated in this bug's original report. No such class was
 * ever built: `git grep -rn StateApplicator web-app/src` outside test-file
 * comments returns nothing. The implementation that shipped
 * (useVisibilityResync.ts) instead reuses the pre-existing, already-proven
 * resize-resync path: requestFullResync(true) -> a fresh CurrentPaneRequest ->
 * the server's clearAndHome-prefixed full pane capture, delivered as a plain
 * `output` message and written through TerminalStreamManager.write(). That IS
 * this codebase's "reset + fresh apply" full-repaint primitive, and this test
 * (plus TerminalStreamManager.resync.test.ts) exercises it for real.
 *
 * TerminalStreamManager.resync.test.ts proves that TerminalStreamManager.write()
 * turns a clear+home-prefixed payload into a genuine full repaint. It calls
 * manager.write() directly, though — it never proves that payload actually
 * reaches the manager via the real wire-message pipeline.
 *
 * This test closes that gap: it drives the REAL (non-mocked) useTerminalStream
 * hook's message loop with a scripted server "output" message and feeds it into
 * a REAL (non-mocked) TerminalStreamManager backed by a REAL (non-mocked)
 * @xterm/xterm Terminal — exactly the wiring TerminalOutput.tsx uses in
 * production (onOutput -> manager.write()). Only the ConnectRPC client/transport
 * and MessageQueue are stubbed, since no real network exists in a unit test.
 *
 * Given a terminal already showing stale/corrupted content (simulating what a
 * backgrounded tab's coalesced deltas leave behind), when a scripted
 * clear+home-prefixed resync payload arrives over the stream as a plain
 * `output` message (the same message type CurrentPaneRequest/resync responses
 * use), then the buffer must show only the fresh content — a true full
 * repaint, not a mocked call-count assertion.
 */

import { renderHook, act, waitFor } from '@testing-library/react';
import { Terminal } from '@xterm/xterm';
import { TerminalStreamManager, type ITerminal } from '@/lib/terminal/TerminalStreamManager';

// ---------------------------------------------------------------------------
// Mock only the network-facing layer — everything terminal-related (xterm,
// TerminalStreamManager, useTerminalStream, useTerminalFlowControl) is real.
// ---------------------------------------------------------------------------

jest.mock('@bufbuild/protobuf', () => ({
  create: (_schema: unknown, init: Record<string, unknown> = {}) => ({ ...init }),
}));

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

jest.mock('@/lib/transport/websocket-transport', () => ({
  createWebsocketBasedTransport: () => ({}),
}));

jest.mock('@/lib/config', () => ({
  createAuthInterceptor: () => () => ({}),
}));

jest.mock('@/gen/session/v1/session_pb', () => ({}));
jest.mock('@/gen/session/v1/events_pb', () => ({
  TerminalDataSchema: {},
  CurrentPaneRequestSchema: {},
  TerminalData: class {},
  CurrentPaneRequest: class {},
}));

jest.mock('@/lib/terminal/MessageQueue', () => ({
  MessageQueue: class {
    push = jest.fn();
    close = jest.fn();
    [Symbol.asyncIterator]() {
      return { next: async () => ({ value: undefined, done: true }) };
    }
  },
}));

import { useTerminalStream } from '../useTerminalStream';

// ---------------------------------------------------------------------------
// Controllable push-based async iterable, mirroring useTerminalStream.test.ts
// ---------------------------------------------------------------------------

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

function makeOutputMsg(text: string) {
  return { data: { case: 'output', value: { data: new TextEncoder().encode(text) } } };
}

// Mirrors ansiSnapshotPrefix in server/services/connectrpc_websocket.go
// (ansiDECSTR + ansiEraseScreen + ansiCursorHome) — see TerminalStreamManager.resync.test.ts.
const clearAndHome = '\x1b[!p\x1b[2J\x1b[H';

describe('useTerminalStream -> TerminalStreamManager (AC7 wire-to-repaint integration)', () => {
  let terminal: Terminal;
  let manager: TerminalStreamManager;

  beforeEach(() => {
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'debug').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
    mockStreamTerminal.mockReset();

    terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
    // Intentionally not calling terminal.open(...) — see TerminalStreamManager.resync.test.ts.
    manager = new TerminalStreamManager(terminal as unknown as ITerminal, jest.fn());
  });

  afterEach(() => {
    jest.restoreAllMocks();
    terminal.dispose();
  });

  it('onOutput_should_produceCleanFullRepaintInRealTerminal_When_streamDeliversScriptedResyncPayload', async () => {
    // Given: the terminal already has stale/corrupted content, exactly as a
    // backgrounded tab's coalesced/dropped incremental deltas would leave.
    await new Promise<void>((resolve) => {
      terminal.write('STALE LINE ONE\r\nSTALE LINE TWO — CORRUPTED\r\n', () => resolve());
    });
    expect(terminal.buffer.active.getLine(0)?.translateToString(true)).toBe('STALE LINE ONE');

    const stream = makePushStream<object>();
    mockStreamTerminal.mockReturnValue(stream.iterable);

    // When: the real useTerminalStream hook connects and its message loop
    // receives a scripted resync payload as a plain `output` message — the
    // same message shape a CurrentPaneRequest resync response uses on the
    // wire — and routes it to onOutput exactly as TerminalOutput.tsx's
    // handleOutput does in production (manager.write(text)).
    renderHook(() =>
      useTerminalStream({
        baseUrl: 'ws://localhost:8543',
        sessionId: 'test-session',
        autoConnect: true,
        getTerminal: () => terminal,
        onOutput: (text) => manager.write(text),
      }),
    );

    await act(async () => {
      stream.push(makeOutputMsg(clearAndHome + 'FRESH LINE ONE\r\nFRESH LINE TWO\r\n'));
    });

    // Flush xterm's parser queue (write() below is a no-op payload; its
    // callback fires only once everything queued ahead of it has parsed).
    await waitFor(() => {
      expect(terminal.buffer.active.getLine(0)?.translateToString(true)).toBe('FRESH LINE ONE');
    });
    await new Promise<void>((resolve) => terminal.write('', () => resolve()));

    // Then: the buffer shows only the fresh content...
    expect(terminal.buffer.active.getLine(0)?.translateToString(true)).toBe('FRESH LINE ONE');
    expect(terminal.buffer.active.getLine(1)?.translateToString(true)).toBe('FRESH LINE TWO');

    // ...and no stale glyph survives anywhere in the populated buffer — a true
    // full repaint reached through the real wire-message pipeline, not a
    // mocked call-count assertion.
    let sawStale = false;
    let sawCorrupted = false;
    for (let i = 0; i < terminal.buffer.active.length; i++) {
      const lineText = terminal.buffer.active.getLine(i)?.translateToString(true) ?? '';
      if (lineText.includes('STALE')) sawStale = true;
      if (lineText.includes('CORRUPTED')) sawCorrupted = true;
    }
    expect(sawStale).toBe(false);
    expect(sawCorrupted).toBe(false);

    stream.end();
  });
});
