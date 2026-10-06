/**
 * Tests for the fromWebSocket close-event propagation logic in watch-ws-transport.ts,
 * and for createSessionWatchTransport's flag + https-guard branching.
 *
 * Tests the three close-event paths:
 *  1. Non-clean WS close  → ConnectError with ws-close-code header
 *  2. AbortSignal.abort() → generator returns (no error)
 *  3. wasClean=true / code=1000 close → generator returns (no error)
 */

import { ConnectError, Code } from "@connectrpc/connect";
import type { ConnectTransportOptions } from "@connectrpc/connect-web";
import { createConnectTransport } from "@connectrpc/connect-web";
import { fromWebSocket, createSessionWatchTransport } from "./watch-ws-transport";

// createSessionWatchTransport (via createWatchTransport, in the WS-bridge branch)
// always constructs a createConnectTransport internally for its unary half, so the
// mock's return value — not "was it called" — is what distinguishes the two branches:
// the native branch returns this sentinel directly, the WS-bridge branch wraps it in
// a new object with its own .stream implementation.
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(),
}));

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

// ---------------------------------------------------------------------------
// createSessionWatchTransport
// ---------------------------------------------------------------------------

describe("createSessionWatchTransport", () => {
  const mockCreateConnectTransport = createConnectTransport as jest.Mock;
  // Distinctive sentinel so we can tell "returned createConnectTransport's result
  // directly" (native branch) apart from "wrapped in createWatchTransport's object"
  // (WS-bridge branch) by reference, even though the WS-bridge branch also calls
  // createConnectTransport internally (for its unary half).
  const NATIVE_TRANSPORT_SENTINEL = { unary: jest.fn(), __sentinel: "native" };
  const originalFlag = process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING;

  beforeEach(() => {
    mockCreateConnectTransport.mockReset();
    mockCreateConnectTransport.mockReturnValue(NATIVE_TRANSPORT_SENTINEL);
  });

  afterEach(() => {
    if (originalFlag === undefined) {
      delete process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING;
    } else {
      process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING = originalFlag;
    }
  });

  function makeOpt(baseUrl: string): ConnectTransportOptions {
    return { baseUrl } as ConnectTransportOptions;
  }

  it("createSessionWatchTransport_should_ReturnNativeConnectTransport_When_FlagOnAndBaseUrlIsHttps", () => {
    process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING = "true";
    const opt = makeOpt("https://onyx.staplerhome.internal:8444/api");

    const result = createSessionWatchTransport(opt);

    expect(result).toBe(NATIVE_TRANSPORT_SENTINEL);
  });

  it("createSessionWatchTransport_should_ReturnWsBridgeTransport_When_FlagOnButBaseUrlIsHttp", () => {
    process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING = "true";
    const opt = makeOpt("http://localhost:8543/api");

    const result = createSessionWatchTransport(opt);

    // Not the native sentinel — createWatchTransport wraps it in a new object
    // with its own WebSocket-backed .stream implementation.
    expect(result).not.toBe(NATIVE_TRANSPORT_SENTINEL);
    expect(typeof (result as { stream: unknown }).stream).toBe("function");
  });

  it.each([
    ["https://onyx.staplerhome.internal:8444/api"],
    ["http://localhost:8543/api"],
  ])(
    "createSessionWatchTransport_should_ReturnWsBridgeTransport_When_FlagUnset (baseUrl=%s)",
    (baseUrl) => {
      delete process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING;
      const opt = makeOpt(baseUrl);

      const result = createSessionWatchTransport(opt);

      expect(result).not.toBe(NATIVE_TRANSPORT_SENTINEL);
      expect(typeof (result as { stream: unknown }).stream).toBe("function");
    }
  );

  it("createSessionWatchTransport_should_ForwardOptionsUnchanged_When_SelectingEitherTransport", () => {
    const interceptors = [jest.fn()];
    const jsonOptions = { ignoreUnknownFields: true };

    process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING = "true";
    const httpsOpt = {
      baseUrl: "https://onyx.staplerhome.internal:8444/api",
      useBinaryFormat: true,
      interceptors,
      jsonOptions,
    } as unknown as ConnectTransportOptions;
    createSessionWatchTransport(httpsOpt);
    expect(mockCreateConnectTransport).toHaveBeenLastCalledWith(httpsOpt);

    const httpOpt = {
      baseUrl: "http://localhost:8543/api",
      useBinaryFormat: true,
      interceptors,
      jsonOptions,
    } as unknown as ConnectTransportOptions;
    createSessionWatchTransport(httpOpt);
    // createWatchTransport's unary half also constructs createConnectTransport
    // with the same, unmodified opt.
    expect(mockCreateConnectTransport).toHaveBeenLastCalledWith(httpOpt);
  });
});
