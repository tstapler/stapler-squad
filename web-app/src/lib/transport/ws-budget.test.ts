/**
 * Socket-count tests for the per-tab Watch* WebSocket budget, driven through
 * createWatchTransport with a fake WebSocket that counts open/closed sockets.
 */

import type { ConnectTransportOptions } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { WsBudget } from "./ws-budget";
import { createWatchTransport } from "./watch-ws-transport";

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({ unary: jest.fn() }),
}));

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  readyState = FakeWebSocket.CONNECTING;
  binaryType = "";
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  private listeners: Array<() => void> = [];
  sent: unknown[] = [];

  constructor(public url: string) {
    FakeWebSocket.instances.push(this);
  }
  addEventListener(type: string, fn: () => void) {
    if (type === "close") this.listeners.push(fn);
  }
  send(d: unknown) {
    this.sent.push(d);
  }
  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }
  close() {
    if (this.readyState === FakeWebSocket.CLOSED) return;
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.({ code: 1000, wasClean: true, reason: "" } as CloseEvent);
    this.listeners.forEach((l) => l());
  }
  static get open() {
    return FakeWebSocket.instances.filter((w) => w.readyState !== FakeWebSocket.CLOSED).length;
  }
}

const flush = () => new Promise((r) => setTimeout(r, 0));

describe("WsBudget", () => {
  it("never grants more than the limit and hands slots to waiters in order", async () => {
    const b = new WsBudget(2);
    const r1 = await b.acquire();
    await b.acquire();
    const third = b.acquire();
    expect(b.active).toBe(2);
    expect(b.queued).toBe(1);
    r1();
    const r3 = await third;
    expect(b.active).toBe(2);
    expect(b.queued).toBe(0);
    r3();
    r3(); // double release is a no-op
    expect(b.active).toBe(1);
  });

  it("drops an aborted waiter without consuming a slot", async () => {
    const b = new WsBudget(1);
    const r1 = await b.acquire();
    const ac = new AbortController();
    const waiting = b.acquire(ac.signal);
    ac.abort(new Error("gone"));
    await expect(waiting).rejects.toThrow("gone");
    expect(b.queued).toBe(0);
    r1();
    expect(b.active).toBe(0);
  });
});

describe("createWatchTransport socket budget", () => {
  const OriginalWS = global.WebSocket;
  beforeEach(() => {
    FakeWebSocket.instances = [];
    Object.defineProperty(globalThis, "WebSocket", { value: FakeWebSocket, configurable: true, writable: true });
  });
  afterEach(() => {
    Object.defineProperty(globalThis, "WebSocket", { value: OriginalWS, configurable: true, writable: true });
  });

  const method = SessionService.method.watchSessions;

  async function* oneRequest() {
    yield {} as never;
  }

  function startStream(transport: ReturnType<typeof createWatchTransport>, signal: AbortSignal) {
    return transport
      .stream(method, signal, undefined, undefined, oneRequest())
      .catch(() => undefined);
  }

  it("holds 100 concurrent watch streams to at most 32 live sockets, then drains the queue", async () => {
    const transport = createWatchTransport({ baseUrl: "http://localhost:8543" } as ConnectTransportOptions);
    const acs = Array.from({ length: 100 }, () => new AbortController());
    const calls = acs.map((ac) => startStream(transport, ac.signal));
    await flush();

    expect(FakeWebSocket.instances.length).toBe(32);
    expect(FakeWebSocket.open).toBe(32);

    acs.forEach((ac) => ac.abort());
    await flush();
    await Promise.all(calls);
    expect(FakeWebSocket.open).toBe(0);
    // Queued streams were aborted while waiting, so they never became sockets.
    expect(FakeWebSocket.instances.length).toBe(32);
  });

  it("closes a socket whose stream is aborted while still CONNECTING", async () => {
    const transport = createWatchTransport({ baseUrl: "http://localhost:8543" } as ConnectTransportOptions);
    const ac = new AbortController();
    const call = startStream(transport, ac.signal);
    await flush();
    expect(FakeWebSocket.open).toBe(1);

    ac.abort();
    await call;
    expect(FakeWebSocket.open).toBe(0);
  });
});
