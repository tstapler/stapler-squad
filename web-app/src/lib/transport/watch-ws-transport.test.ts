/**
 * Tests for the fromWebSocket close-event propagation logic in watch-ws-transport.ts.
 *
 * Tests the three close-event paths:
 *  1. Non-clean WS close  → ConnectError with ws-close-code header
 *  2. AbortSignal.abort() → generator returns (no error)
 *  3. wasClean=true / code=1000 close → generator returns (no error)
 */

import { ConnectError, Code } from "@connectrpc/connect";
import { fromWebSocket } from "./watch-ws-transport";

// ---------------------------------------------------------------------------
// Helper: build a minimal mock WebSocket that satisfies the subset of the
// WebSocket API used by fromWebSocket (onmessage, onerror, onclose, close).
// ---------------------------------------------------------------------------

interface MockWS {
  onmessage: ((e: MessageEvent) => void) | null;
  onerror: (() => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  close: () => void;
  simulateClose: (ev: Partial<CloseEvent>) => void;
}

function makeMockWS(): MockWS {
  const ws: MockWS = {
    onmessage: null,
    onerror: null,
    onclose: null,
    close: jest.fn(),
    simulateClose(ev: Partial<CloseEvent>) {
      const fullEv = {
        code: ev.code ?? 1006,
        wasClean: ev.wasClean ?? false,
        reason: ev.reason ?? "",
      } as CloseEvent;
      ws.onclose?.(fullEv);
    },
  };
  return ws;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("fromWebSocket", () => {
  it("fromWebSocket_should_pushConnectError_When_wsClosesWithNonCleanCode", async () => {
    const ws = makeMockWS();
    const gen = fromWebSocket(ws as unknown as WebSocket, undefined);

    // Trigger a non-clean close on the next tick
    setTimeout(() => {
      ws.simulateClose({ code: 4001, wasClean: false });
    }, 0);

    let caught: unknown = null;
    try {
      await gen.next();
    } catch (e) {
      caught = e;
    }

    expect(caught).toBeInstanceOf(ConnectError);
    const err = caught as ConnectError;
    expect(err.rawMessage).toBe("WebSocket closed");
    expect(err.code).toBe(Code.Unavailable);
    expect(err.metadata.get("ws-close-code")).toBe("4001");
  });

  it("fromWebSocket_should_pushNull_When_abortSignalFires", async () => {
    const controller = new AbortController();
    const ws = makeMockWS();
    const gen = fromWebSocket(ws as unknown as WebSocket, controller.signal);

    // Abort before close fires — the abortHandler calls push(null) immediately
    setTimeout(() => {
      controller.abort();
    }, 0);

    // After abort the generator should return without throwing
    const result = await gen.next();
    expect(result.done).toBe(true);
    expect(result.value).toBeUndefined();
  });

  it("fromWebSocket_should_pushNull_When_wsClosesWithCode1000", async () => {
    const ws = makeMockWS();
    const gen = fromWebSocket(ws as unknown as WebSocket, undefined);

    setTimeout(() => {
      ws.simulateClose({ code: 1000, wasClean: true });
    }, 0);

    const result = await gen.next();
    expect(result.done).toBe(true);
    expect(result.value).toBeUndefined();
  });

  it("fromWebSocket_should_pushConnectError_When_wsClosesWithCode1001", async () => {
    // Code 1001 (Going Away) is NOT a clean close — the production code treats
    // only code 1000 and abort as non-retriable clean closes.
    const ws = makeMockWS();
    const gen = fromWebSocket(ws as unknown as WebSocket, undefined);

    setTimeout(() => {
      ws.simulateClose({ code: 1001, wasClean: true });
    }, 0);

    let caught: unknown = null;
    try {
      await gen.next();
    } catch (e) {
      caught = e;
    }

    expect(caught).toBeInstanceOf(ConnectError);
    const err = caught as ConnectError;
    expect(err.metadata.get("ws-close-code")).toBe("1001");
  });
});
