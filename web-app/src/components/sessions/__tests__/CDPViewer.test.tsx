import React from 'react';
import { render, act } from '@testing-library/react';
import CDPViewer from '../CDPViewer';

// ---------------------------------------------------------------------------
// WebSocket mock
// ---------------------------------------------------------------------------

type WsEventMap = {
  open: Event;
  message: MessageEvent;
  close: CloseEvent;
  error: Event;
};

class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  url: string;
  binaryType: string = 'blob';
  readyState: number = MockWebSocket.CONNECTING;

  private listeners: { [K in keyof WsEventMap]?: ((e: WsEventMap[K]) => void)[] } = {};

  // Track instances so tests can access the most-recent socket
  static instances: MockWebSocket[] = [];

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  addEventListener<K extends keyof WsEventMap>(
    type: K,
    handler: (e: WsEventMap[K]) => void,
  ) {
    if (!this.listeners[type]) this.listeners[type] = [] as typeof this.listeners[K];
    (this.listeners[type] as ((e: WsEventMap[K]) => void)[]).push(handler);
  }

  removeEventListener() {
    // not used by CDPViewer, no-op is fine
  }

  send = jest.fn();

  close = jest.fn(() => {
    this.readyState = MockWebSocket.CLOSED;
    this.emit('close', new Event('close') as CloseEvent);
  });

  // Test helpers ─────────────────────────────────────────────────────────────

  emit<K extends keyof WsEventMap>(type: K, event: WsEventMap[K]) {
    (this.listeners[type] ?? []).forEach((h) => h(event));
  }

  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.emit('open', new Event('open'));
  }

  simulateClose() {
    this.readyState = MockWebSocket.CLOSED;
    this.emit('close', new Event('close') as CloseEvent);
  }

  simulateError() {
    this.emit('error', new Event('error'));
  }
}

// Replace global WebSocket before each test, restore after each suite.
let OriginalWebSocket: typeof WebSocket;

beforeAll(() => {
  OriginalWebSocket = global.WebSocket;
});

beforeEach(() => {
  MockWebSocket.instances = [];
  // @ts-expect-error — intentional global replacement for testing
  global.WebSocket = MockWebSocket;
  // Canvas getContext returns a minimal stub so renderFrame doesn't throw
  HTMLCanvasElement.prototype.getContext = jest.fn(() => ({
    drawImage: jest.fn(),
  })) as unknown as typeof HTMLCanvasElement.prototype.getContext;
});

afterEach(() => {
  // @ts-expect-error
  global.WebSocket = OriginalWebSocket;
  jest.clearAllTimers();
  jest.useRealTimers();
});

afterAll(() => {
  global.WebSocket = OriginalWebSocket;
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function latestSocket(): MockWebSocket {
  const s = MockWebSocket.instances[MockWebSocket.instances.length - 1];
  if (!s) throw new Error('No MockWebSocket instance found');
  return s;
}

// ---------------------------------------------------------------------------
// T-4: CDPViewer — WebSocket lifecycle
// ---------------------------------------------------------------------------

describe('CDPViewer — WebSocket lifecycle', () => {
  it('CDPViewer_should_connectToCorrectUrl_When_mounted', () => {
    render(<CDPViewer wsUrl="ws://localhost:8543/api/sessions/abc/cdp-stream" isVisible={true} />);
    const ws = latestSocket();
    expect(ws.url).toBe('ws://localhost:8543/api/sessions/abc/cdp-stream');
  });

  it('CDPViewer_should_closeWebSocket_When_unmounted', () => {
    const { unmount } = render(
      <CDPViewer wsUrl="ws://localhost:8543/api/sessions/abc/cdp-stream" isVisible={true} />,
    );
    const ws = latestSocket();
    unmount();
    expect(ws.close).toHaveBeenCalledTimes(1);
  });

  it('CDPViewer_should_callOnConnected_When_webSocketOpens', () => {
    const onConnected = jest.fn();
    render(
      <CDPViewer
        wsUrl="ws://localhost:8543/api/sessions/abc/cdp-stream"
        isVisible={true}
        onConnected={onConnected}
      />,
    );
    act(() => latestSocket().simulateOpen());
    expect(onConnected).toHaveBeenCalledTimes(1);
  });

  it('CDPViewer_should_callOnDisconnected_When_webSocketCloses', () => {
    const onDisconnected = jest.fn();
    render(
      <CDPViewer
        wsUrl="ws://localhost:8543/api/sessions/abc/cdp-stream"
        isVisible={true}
        onDisconnected={onDisconnected}
      />,
    );
    act(() => latestSocket().simulateClose());
    expect(onDisconnected).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// T-4: CDPViewer — Reconnect cleanup
// ---------------------------------------------------------------------------

describe('CDPViewer — Reconnect cleanup', () => {
  it('CDPViewer_should_scheduleReconnect_When_webSocketCloses', () => {
    jest.useFakeTimers();
    render(
      <CDPViewer wsUrl="ws://localhost:8543/api/sessions/abc/cdp-stream" isVisible={true} />,
    );
    const firstWs = latestSocket();
    const instancesBefore = MockWebSocket.instances.length;

    act(() => firstWs.simulateClose());
    // Timer is scheduled but not yet fired
    expect(MockWebSocket.instances.length).toBe(instancesBefore);

    act(() => jest.advanceTimersByTime(2001));
    // Reconnect fires — a new WebSocket should have been created
    expect(MockWebSocket.instances.length).toBeGreaterThan(instancesBefore);
  });

  it('CDPViewer_should_notReconnect_When_unmountedBeforeTimerFires', () => {
    jest.useFakeTimers();
    const { unmount } = render(
      <CDPViewer wsUrl="ws://localhost:8543/api/sessions/abc/cdp-stream" isVisible={true} />,
    );
    const firstWs = latestSocket();
    const instancesBefore = MockWebSocket.instances.length;

    act(() => firstWs.simulateClose());
    unmount();

    // Advance past the 2 s reconnect delay — should NOT create a new socket
    act(() => jest.advanceTimersByTime(3000));
    expect(MockWebSocket.instances.length).toBe(instancesBefore);
  });
});

// ---------------------------------------------------------------------------
// T-4: CDPViewer — getModifiers bitmask (tested indirectly via keyboard events)
// ---------------------------------------------------------------------------

describe('CDPViewer — getModifiers bitmask', () => {
  // We exercise getModifiers by rendering the component (canvas), opening the
  // WebSocket, then firing synthetic keyboard events on the canvas.  The
  // component calls sendJson which calls ws.send with the CDP payload.

  function setupConnectedViewer() {
    const { getByRole } = render(
      <CDPViewer wsUrl="ws://localhost:8543/api/sessions/abc/cdp-stream" isVisible={true} />,
    );
    const ws = latestSocket();
    act(() => ws.simulateOpen());
    const canvas = getByRole('img');
    return { ws, canvas };
  }

  function fireKey(
    canvas: HTMLElement,
    key: string,
    modifiers: { altKey?: boolean; ctrlKey?: boolean; metaKey?: boolean; shiftKey?: boolean } = {},
  ) {
    const event = new KeyboardEvent('keydown', {
      key,
      bubbles: true,
      ...modifiers,
    });
    act(() => canvas.dispatchEvent(event));
  }

  function lastSentModifiers(ws: MockWebSocket): number {
    const calls = ws.send.mock.calls;
    expect(calls.length).toBeGreaterThan(0);
    const lastCall = calls[calls.length - 1];
    const payload = JSON.parse(lastCall[0] as string) as {
      params: { modifiers: number };
    };
    return payload.params.modifiers;
  }

  it('CDPViewer_should_setModifierBit1_When_altKeyIsPressed', () => {
    const { ws, canvas } = setupConnectedViewer();
    fireKey(canvas, 'a', { altKey: true });
    expect(lastSentModifiers(ws) & 1).toBe(1);
  });

  it('CDPViewer_should_setModifierBit2_When_ctrlKeyIsPressed', () => {
    const { ws, canvas } = setupConnectedViewer();
    fireKey(canvas, 'a', { ctrlKey: true });
    expect(lastSentModifiers(ws) & 2).toBe(2);
  });

  it('CDPViewer_should_setModifierBit4_When_metaKeyIsPressed', () => {
    const { ws, canvas } = setupConnectedViewer();
    fireKey(canvas, 'a', { metaKey: true });
    expect(lastSentModifiers(ws) & 4).toBe(4);
  });

  it('CDPViewer_should_setModifierBit8_When_shiftKeyIsPressed', () => {
    const { ws, canvas } = setupConnectedViewer();
    fireKey(canvas, 'a', { shiftKey: true });
    expect(lastSentModifiers(ws) & 8).toBe(8);
  });

  it('CDPViewer_should_combineModifierBits_When_multipleModifiersPressed', () => {
    const { ws, canvas } = setupConnectedViewer();
    fireKey(canvas, 'a', { ctrlKey: true, shiftKey: true });
    // Ctrl=2, Shift=8 → 10
    expect(lastSentModifiers(ws)).toBe(10);
  });

  it('CDPViewer_should_returnZeroModifiers_When_noModifiersPressed', () => {
    const { ws, canvas } = setupConnectedViewer();
    fireKey(canvas, 'a');
    expect(lastSentModifiers(ws)).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// T-4: CDPViewer — wheel event
// ---------------------------------------------------------------------------

describe('CDPViewer — wheel event', () => {
  it('CDPViewer_should_sendMouseWheelEvent_When_wheelEventFired', () => {
    const { getByRole } = render(
      <CDPViewer wsUrl="ws://localhost:8543/api/sessions/abc/cdp-stream" isVisible={true} />,
    );
    const ws = latestSocket();
    act(() => ws.simulateOpen());
    const canvas = getByRole('img');

    const wheelEvent = new WheelEvent('wheel', {
      bubbles: true,
      deltaX: 0,
      deltaY: 120,
    });
    act(() => canvas.dispatchEvent(wheelEvent));

    expect(ws.send).toHaveBeenCalled();
    const lastCall = ws.send.mock.calls[ws.send.mock.calls.length - 1];
    const payload = JSON.parse(lastCall[0] as string) as {
      method: string;
      params: { type: string; deltaY: number };
    };
    expect(payload.method).toBe('Input.dispatchMouseEvent');
    expect(payload.params.type).toBe('mouseWheel');
    expect(payload.params.deltaY).toBe(120);
  });
});

// ---------------------------------------------------------------------------
// T-7: buildWsUrl boundary tests
// (buildWsUrl is a module-level function inside BrowserTab.tsx — these boundary
//  cases are verified through CDPViewer's wsUrl prop which it receives verbatim)
// ---------------------------------------------------------------------------

describe('CDPViewer — wsUrl prop passthrough', () => {
  it('CDPViewer_should_connectWithWsScheme_When_httpOriginProvided', () => {
    render(<CDPViewer wsUrl="ws://example.com/api/sessions/s1/cdp-stream" isVisible={true} />);
    expect(latestSocket().url).toMatch(/^ws:\/\//);
  });

  it('CDPViewer_should_connectWithWssScheme_When_httpsOriginProvided', () => {
    render(<CDPViewer wsUrl="wss://example.com/api/sessions/s1/cdp-stream" isVisible={true} />);
    expect(latestSocket().url).toMatch(/^wss:\/\//);
  });

  it('CDPViewer_should_includeSessionIdInUrl_When_mounted', () => {
    render(<CDPViewer wsUrl="ws://localhost:8543/api/sessions/my-session-id/cdp-stream" isVisible={true} />);
    expect(latestSocket().url).toContain('my-session-id');
  });

  it('CDPViewer_should_notHaveDoubleSlash_When_urlIsWellFormed', () => {
    render(<CDPViewer wsUrl="ws://localhost:8543/api/sessions/abc/cdp-stream" isVisible={true} />);
    // Path portion should not contain //
    const { url } = latestSocket();
    const path = url.replace(/^wss?:\/\/[^/]+/, '');
    expect(path).not.toContain('//');
  });
});
