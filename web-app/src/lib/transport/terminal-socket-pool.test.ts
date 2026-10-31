import {
  createTerminalSocketPool,
  hashByStream,
  leastLoaded,
  type PooledSocketLike,
  type TerminalSocketPool,
} from "./terminal-socket-pool";
import {
  decodeMuxFrame,
  encodeClose,
  encodeConfig,
  encodeData,
  MUX_KIND_BINARY,
  MUX_KIND_TEXT,
  type MuxFrame,
} from "./terminal-mux-frame";

class FakeMux extends EventTarget {
  binaryType = "blob";
  sent: Uint8Array[] = [];
  closed = false;
  constructor(readonly url: string) {
    super();
  }
  send(data: Uint8Array): void {
    this.sent.push(data);
  }
  close(): void {
    this.closed = true;
  }
  open(): void {
    this.dispatchEvent(new Event("open"));
  }
  feed(bytes: Uint8Array): void {
    this.dispatchEvent(Object.assign(new Event("message"), { data: bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) }));
  }
  drop(): void {
    this.dispatchEvent(new Event("close"));
  }
  frames(type?: MuxFrame["type"]): MuxFrame[] {
    const all = this.sent.map((b) => decodeMuxFrame(b));
    return type ? all.filter((f) => f.type === type) : all;
  }
}

const HELLO = { poolSize: 2, window: 1000, chunk: 10, maxStreams: 64 };
const URL = "ws://localhost:8543/api/session.v1.SessionService/StreamTerminal";

interface Harness {
  pool: TerminalSocketPool;
  muxes: FakeMux[];
  fallbacks: string[];
  intervals: { fn: () => void; ms: number }[];
  clock: { t: number };
}

function setup(opts: { size?: number; policy?: Parameters<typeof createTerminalSocketPool>[0] extends infer O ? (O extends { policy?: infer P } ? P : never) : never } = {}): Harness {
  const muxes: FakeMux[] = [];
  const fallbacks: string[] = [];
  const intervals: { fn: () => void; ms: number }[] = [];
  const clock = { t: 1_000_000 };
  const pool = createTerminalSocketPool({
    size: opts.size,
    policy: opts.policy,
    now: () => clock.t,
    createSocket: (u) => {
      const m = new FakeMux(u);
      muxes.push(m);
      return m as unknown as WebSocket;
    },
    fallback: (u) => {
      fallbacks.push(u);
      return {} as PooledSocketLike;
    },
    setInterval: (fn, ms) => {
      intervals.push({ fn, ms });
      return intervals.length;
    },
    clearInterval: () => undefined,
  });
  return { pool, muxes, fallbacks, intervals, clock };
}

const flush = () => Promise.resolve();

describe("terminal socket pool", () => {
  it("opens mux sockets lazily up to size, then assigns least-loaded", () => {
    const h = setup({ size: 2 });
    expect(h.muxes).toHaveLength(0);
    h.pool.openStream(URL);
    h.pool.openStream(URL);
    expect(h.muxes).toHaveLength(2);
    h.pool.openStream(URL);
    h.pool.openStream(URL);
    expect(h.muxes).toHaveLength(2);
    expect(h.pool.stats().sockets.map((s) => s.streams)).toEqual([2, 2]);
    expect(h.muxes[0].url).toBe("ws://localhost:8543/api/ws/terminal-mux");
  });

  it("falls back to a plain WebSocket when size is 0", () => {
    const h = setup({ size: 0 });
    h.pool.openStream(URL);
    expect(h.fallbacks).toEqual([URL]);
    expect(h.muxes).toHaveLength(0);
  });

  it("emits open after the mux socket opens and sends OPEN then DATA", async () => {
    const h = setup({ size: 1 });
    const s = h.pool.openStream(URL);
    const onOpen = jest.fn();
    s.addEventListener("open", onOpen);
    expect(s.readyState).toBe(0);
    h.muxes[0].open();
    await flush();
    expect(onOpen).toHaveBeenCalledTimes(1);
    expect(s.readyState).toBe(1);
    s.send("hdr");
    expect(h.muxes[0].frames().map((f) => f.type)).toEqual(["OPEN", "DATA"]);
    const data = h.muxes[0].frames("DATA")[0] as Extract<MuxFrame, { type: "DATA" }>;
    expect(data.kind).toBe(MUX_KIND_TEXT);
    expect(data.fin).toBe(true);
  });

  it("chunks large binary sends using the server chunk size", async () => {
    const h = setup({ size: 1 });
    const s = h.pool.openStream(URL);
    h.muxes[0].open();
    h.muxes[0].feed(encodeConfig("HELLO", HELLO));
    await flush();
    s.send(new Uint8Array(25));
    const data = h.muxes[0].frames("DATA") as Extract<MuxFrame, { type: "DATA" }>[];
    expect(data.map((f) => [f.payload.length, f.fin, f.kind])).toEqual([
      [10, false, MUX_KIND_BINARY],
      [10, false, MUX_KIND_BINARY],
      [5, true, MUX_KIND_BINARY],
    ]);
  });

  it("reassembles chunks into one message and returns credit as it consumes", async () => {
    const h = setup({ size: 1 });
    const s = h.pool.openStream(URL);
    const msgs: unknown[] = [];
    s.addEventListener("message", (e) => msgs.push((e as MessageEvent).data));
    h.muxes[0].open();
    h.muxes[0].feed(encodeConfig("HELLO", HELLO));
    await flush();
    h.muxes[0].feed(encodeData(1, MUX_KIND_BINARY, false, new Uint8Array([1, 2, 3])));
    expect(msgs).toHaveLength(0);
    h.muxes[0].feed(encodeData(1, MUX_KIND_BINARY, true, new Uint8Array([4, 5])));
    expect(msgs).toHaveLength(1);
    expect(Array.from(new Uint8Array(msgs[0] as ArrayBuffer))).toEqual([1, 2, 3, 4, 5]);
    expect(h.muxes[0].frames("CREDIT")).toHaveLength(0); // 5 bytes < window/4
    h.muxes[0].feed(encodeData(1, MUX_KIND_TEXT, true, new TextEncoder().encode("x".repeat(250))));
    expect(typeof msgs[1]).toBe("string");
    const credit = h.muxes[0].frames("CREDIT")[0] as Extract<MuxFrame, { type: "CREDIT" }>;
    expect(credit.credit).toBe(255);
  });

  it("surfaces a server CLOSE as a close event and frees the stream", async () => {
    const h = setup({ size: 1 });
    const s = h.pool.openStream(URL);
    const closes: { code: number; wasClean: boolean }[] = [];
    s.addEventListener("close", (e) => closes.push(e as unknown as { code: number; wasClean: boolean }));
    h.muxes[0].open();
    await flush();
    h.muxes[0].feed(encodeClose(1, 1013, "full"));
    expect(closes).toMatchObject([{ code: 1013, wasClean: false }]);
    expect(s.readyState).toBe(3);
    expect(h.pool.stats().sockets[0].streams).toBe(0);
  });

  it("closing a stream sends CLOSE and reports a clean close", async () => {
    const h = setup({ size: 1 });
    const s = h.pool.openStream(URL);
    const closes: { code: number; wasClean: boolean }[] = [];
    s.addEventListener("close", (e) => closes.push(e as unknown as { code: number; wasClean: boolean }));
    h.muxes[0].open();
    await flush();
    s.close();
    expect(h.muxes[0].frames("CLOSE")).toHaveLength(1);
    expect(closes).toMatchObject([{ code: 1000, wasClean: true }]);
  });

  it("fails every stream on a dropped mux socket and opens a fresh socket for the next stream", async () => {
    const h = setup({ size: 1 });
    const a = h.pool.openStream(URL);
    const b = h.pool.openStream(URL);
    const closed: number[] = [];
    for (const s of [a, b]) s.addEventListener("close", (e) => closed.push((e as unknown as { code: number }).code));
    h.muxes[0].open();
    h.muxes[0].feed(encodeConfig("HELLO", HELLO));
    await flush();
    h.muxes[0].drop();
    expect(closed).toEqual([1006, 1006]);
    expect(h.pool.stats().sockets).toHaveLength(0);
    h.pool.openStream(URL);
    expect(h.muxes).toHaveLength(2);
  });

  it("uses the plain WebSocket path after repeated mux failures before any HELLO", () => {
    const h = setup({ size: 1 });
    h.pool.openStream(URL);
    h.muxes[0].drop();
    h.pool.openStream(URL);
    h.muxes[1].drop();
    h.pool.openStream(URL);
    expect(h.fallbacks).toEqual([URL]);
    h.clock.t += 61_000;
    h.pool.openStream(URL);
    expect(h.muxes).toHaveLength(3);
  });

  it("shrinks by draining: no new streams, socket closes when its last stream ends; grow is immediate", async () => {
    const h = setup({ size: 2 });
    const a = h.pool.openStream(URL); // mux 0
    h.pool.openStream(URL); // mux 1
    for (const m of h.muxes) m.open();
    await flush();
    h.pool.setSize(1);
    expect(h.pool.stats().sockets.map((s) => s.draining)).toEqual([true, false]);
    h.pool.openStream(URL);
    expect(h.pool.stats().sockets.map((s) => s.streams)).toEqual([1, 2]); // draining socket got nothing new
    expect(h.muxes[0].closed).toBe(false); // live stream is never dropped
    a.close();
    expect(h.muxes[0].closed).toBe(true);
    expect(h.pool.stats().sockets).toHaveLength(1);
    h.pool.setSize(3);
    h.pool.openStream(URL);
    h.pool.openStream(URL);
    expect(h.pool.stats().sockets.map((s) => s.draining)).toEqual([false, false, false]);
  });

  it("applies the server-advertised pool size unless one was pinned", async () => {
    const h = setup({});
    h.pool.openStream(URL);
    h.muxes[0].open();
    h.muxes[0].feed(encodeConfig("HELLO", { ...HELLO, poolSize: 4 }));
    expect(h.pool.stats().size).toBe(4);
    h.muxes[0].feed(encodeConfig("CONFIG", { ...HELLO, poolSize: 3 }));
    expect(h.pool.stats().size).toBe(3);

    const pinned = setup({ size: 1 });
    pinned.pool.openStream(URL);
    pinned.muxes[0].open();
    pinned.muxes[0].feed(encodeConfig("HELLO", { ...HELLO, poolSize: 8 }));
    expect(pinned.pool.stats().size).toBe(1);
  });

  it("pings on an interval and drops a socket that has been silent too long", async () => {
    const h = setup({ size: 1 });
    const s = h.pool.openStream(URL);
    const closed: number[] = [];
    s.addEventListener("close", (e) => closed.push((e as unknown as { code: number }).code));
    h.muxes[0].open();
    await flush();
    h.intervals[0].fn();
    expect(h.muxes[0].frames("PING")).toHaveLength(1);
    h.clock.t += 46_000;
    h.intervals[0].fn();
    expect(closed).toEqual([1006]);
    expect(h.muxes[0].closed).toBe(true);
  });

  it("hashByStream is deterministic and leastLoaded prefers the idle socket", () => {
    expect(hashByStream.pick([{ streams: 0, recentBytes: 0 }, { streams: 5, recentBytes: 0 }], 2, 3)).toBe(1);
    expect(hashByStream.pick([], 2, 3)).toBe(-1);
    expect(
      leastLoaded.pick(
        [
          { streams: 1, recentBytes: 50_000 },
          { streams: 3, recentBytes: 0 },
        ],
        2,
        1,
      ),
    ).toBe(1);
  });
});
