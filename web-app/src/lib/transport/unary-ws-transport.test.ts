import { create, toBinary, toJson } from "@bufbuild/protobuf";
import type { DescMessage, DescMethodUnary, MessageInitShape } from "@bufbuild/protobuf";
import type { Transport, UnaryResponse } from "@connectrpc/connect";
import { Code, ConnectError } from "@connectrpc/connect";
import { runUnaryCall } from "@connectrpc/connect/protocol";
import {
  CreateSessionRequestSchema,
  ListFilesRequestSchema,
  ListFilesResponseSchema,
  SessionService,
} from "@/gen/session/v1/session_pb";
import { createRpcTimingInterceptor, setRpcTimingSink } from "@/lib/telemetry/rpcTiming";
import { encodeFrame, MAX_FRAME_BYTES, type MuxFrame } from "./unary-mux-frame";
import { FakeWebSocket } from "./__fixtures__/fakeUnaryWebSocket";
import {
  createUnaryWsTransport,
  deriveSocketUrl,
  MAX_IN_FLIGHT,
  PONG_DEADLINE_HIDDEN_MS,
  PONG_DEADLINE_MS,
  UnaryWsBreaker,
  WsUnaryError,
  type UnaryWsTransport,
} from "./unary-ws-transport";

const listFiles = SessionService.method.listFiles;
const getVCSStatus = SessionService.method.getVCSStatus;
const createSession = SessionService.method.createSession;
const emptyMeta = { headers: {}, trailers: {} };

function listFilesBody(basePath: string): Uint8Array {
  return toBinary(ListFilesResponseSchema, create(ListFilesResponseSchema, { basePath }));
}

function response(id: number, basePath: string, meta = emptyMeta): MuxFrame {
  const body = listFilesBody(basePath);
  return { type: "RESPONSE", id, totalLen: body.length, fin: true, meta, body };
}

function httpResponse<I extends DescMessage, O extends DescMessage>(
  method: DescMethodUnary<I, O>,
  message: unknown
): UnaryResponse<I, O> {
  return {
    stream: false,
    service: method.parent,
    method,
    header: new Headers(),
    trailer: new Headers(),
    message: message as never,
  };
}

interface Harness {
  t: UnaryWsTransport;
  fallback: { unary: jest.Mock; stream: jest.Mock };
  sock: (i?: number) => FakeWebSocket;
}

function setup(opts: { baseUrl?: string; pageHref?: string; breaker?: UnaryWsBreaker } = {}): Harness {
  const fallback = {
    unary: jest.fn(async (method: DescMethodUnary) =>
      httpResponse(method, create(method.output, {} as never))
    ),
    stream: jest.fn(),
  };
  const t = createUnaryWsTransport({
    baseUrl: opts.baseUrl ?? "http://localhost:8543",
    pageHref: opts.pageHref ?? "http://localhost:8543/",
    fallback: fallback as unknown as Transport,
    WebSocketImpl: FakeWebSocket as unknown as typeof WebSocket,
    random: () => 0,
    breaker: opts.breaker,
  });
  return { t, fallback, sock: (i = 0) => FakeWebSocket.instances[i] };
}

function callListFiles(
  t: Transport,
  path = "/",
  signal?: AbortSignal,
  timeoutMs?: number
): Promise<UnaryResponse<typeof ListFilesRequestSchema, typeof ListFilesResponseSchema>> {
  return t.unary(listFiles, signal, timeoutMs, undefined, { sessionId: "s", path });
}

function callVcs(t: Transport) {
  return t.unary(getVCSStatus, undefined, undefined, undefined, { id: "s" });
}

beforeEach(() => {
  jest.useFakeTimers();
  FakeWebSocket.instances = [];
});

afterEach(() => {
  jest.useRealTimers();
});

/** Rejection captured without leaving an unhandled rejection behind. */
function settled<T>(p: Promise<T>): Promise<{ ok: true; value: T } | { ok: false; error: unknown }> {
  return p.then(
    (value) => ({ ok: true as const, value }),
    (error: unknown) => ({ ok: false as const, error })
  );
}

describe("socket url", () => {
  it("socketUrl_should_BeWssForHttpsAndWsForHttp_And_StripQueryAndFragment_And_PassNoSubprotocol_And_RefuseMixedContent", () => {
    expect(deriveSocketUrl("https://host:8444", "https://host:8444/")).toBe("wss://host:8444/api/ws/unary");
    expect(deriveSocketUrl("http://localhost:62871", "http://localhost:62871/")).toBe("ws://localhost:62871/api/ws/unary");
    expect(deriveSocketUrl("https://host:8444/?token=x#x", "https://host:8444/")).toBe("wss://host:8444/api/ws/unary");
    expect(deriveSocketUrl("", "http://localhost:8543/page")).toBe("ws://localhost:8543/api/ws/unary");
    // mixed content: an http baseUrl on an https page is refused
    expect(deriveSocketUrl("http://host:8543", "https://host:8444/")).toBeNull();

    const h = setup({ baseUrl: "https://host:8444/?token=x#x", pageHref: "https://host:8444/" });
    void callListFiles(h.t);
    expect(h.sock().url).toBe("wss://host:8444/api/ws/unary");
    expect(h.sock().ctorArgs).toEqual(["wss://host:8444/api/ws/unary"]); // no subprotocols
    expect(h.sock().binaryType).toBe("arraybuffer");
    h.t.close();

    FakeWebSocket.instances = [];
    const mixed = setup({ baseUrl: "http://host:8543", pageHref: "https://host:8444/" });
    void callListFiles(mixed.t);
    expect(FakeWebSocket.instances).toHaveLength(0);
    expect(mixed.fallback.unary).toHaveBeenCalledTimes(1);
  });
});

describe("unary over the socket", () => {
  it("unary_should_ResolveDecodedMessageEqualToHttp_When_ResponseAndMetaHeadersTrailersReturned", async () => {
    const h = setup();
    const p = callListFiles(h.t, "/src");
    h.sock().open();
    const [req] = h.sock().frames();
    expect(req.type).toBe("REQUEST");
    if (req.type !== "REQUEST") return;
    expect(req.method).toBe("/session.v1.SessionService/ListFiles");
    expect(req.meta.headers).toEqual({ "Connect-Protocol-Version": "1" });
    h.sock().receive(
      response(req.id, "/src", { headers: { "x-a": "1" }, trailers: { "x-t": "2" } })
    );
    const res = await p;

    const httpMsg = create(ListFilesResponseSchema, { basePath: "/src" });
    expect(toJson(ListFilesResponseSchema, res.message)).toEqual(toJson(ListFilesResponseSchema, httpMsg));
    expect(res.header.get("x-a")).toBe("1");
    expect(res.trailer.get("x-t")).toBe("2");
    h.t.close();
  });

  it("unary_should_SendDefaultTimeoutInMeta_When_NoExplicitTimeoutMs", () => {
    const h = setup();
    void callListFiles(h.t);
    h.sock().open();
    const [req] = h.sock().frames();
    expect(req.type === "REQUEST" && req.meta.timeoutMs).toBe(30000);
    h.t.close();
  });

  it("unary_should_ResolveEachOwnPayload_When_ResponsesArriveOutOfOrder3_1_2", async () => {
    const h = setup();
    const ps = ["/1", "/2", "/3"].map((p) => callListFiles(h.t, p));
    h.sock().open();
    const ids = h.sock().requestIds();
    expect(ids).toHaveLength(3);
    h.sock().receive(response(ids[2], "/3"));
    h.sock().receive(response(ids[0], "/1"));
    h.sock().receive(response(ids[1], "/2"));
    const out = await Promise.all(ps);
    expect(out.map((r) => r.message.basePath)).toEqual(["/1", "/2", "/3"]);
    h.t.close();
  });

  it("unary_should_ReassembleByTotalLenAndFin_When_ResponsePlusChunksArrive", async () => {
    const h = setup();
    const p = callListFiles(h.t, "/big");
    h.sock().open();
    const id = h.sock().requestIds()[0];
    const body = listFilesBody("/big/".repeat(10));
    const cut1 = 3;
    const cut2 = 9;
    h.sock().receive({ type: "RESPONSE", id, totalLen: body.length, fin: false, meta: emptyMeta, body: body.slice(0, cut1) });
    h.sock().receive({ type: "CHUNK", id, fin: false, body: body.slice(cut1, cut2) });
    h.sock().receive({ type: "CHUNK", id, fin: true, body: body.slice(cut2) });
    expect((await p).message.basePath).toBe("/big/".repeat(10));
    h.t.close();
  });

  it("unary_should_RejectInternal_When_ChunkedLengthDiffersFromTotalLen", async () => {
    const h = setup();
    const p = settled(callListFiles(h.t));
    h.sock().open();
    const id = h.sock().requestIds()[0];
    h.sock().receive({ type: "RESPONSE", id, totalLen: 10, fin: false, meta: emptyMeta, body: new Uint8Array(4) });
    h.sock().receive({ type: "CHUNK", id, fin: true, body: new Uint8Array(2) });
    const r = await p;
    expect(!r.ok && ConnectError.from(r.error).code).toBe(Code.Internal);
    expect(h.sock().frames().some((f) => f.type === "CANCEL" && f.id === id)).toBe(true);
    h.t.close();
  });

  it("unary_should_MapErrorFrameToConnectErrorWithMarker", async () => {
    const h = setup();
    const p = settled(callListFiles(h.t));
    h.sock().open();
    h.sock().receive({
      type: "ERROR",
      id: h.sock().requestIds()[0],
      json: { code: "permission_denied", message: "nope", details: { marker: "ws_method_not_allowed" }, trailers: { "x-t": "9" } },
    });
    const r = await p;
    expect(r.ok).toBe(false);
    if (r.ok) return;
    expect(r.error).toBeInstanceOf(WsUnaryError);
    const e = r.error as WsUnaryError;
    expect(e.code).toBe(Code.PermissionDenied);
    expect(e.rawMessage).toBe("nope");
    expect(e.marker).toBe("ws_method_not_allowed");
    expect(e.metadata.get("x-t")).toBe("9");
    h.t.close();
  });

  it("unary_should_SendCancelAndRejectCanceled_When_AbortSignalFires", async () => {
    const h = setup();
    const ac = new AbortController();
    const p = settled(callListFiles(h.t, "/", ac.signal));
    h.sock().open();
    const id = h.sock().requestIds()[0];
    ac.abort();
    const r = await p;
    expect(!r.ok && ConnectError.from(r.error).code).toBe(Code.Canceled);
    expect(h.sock().frames().filter((f) => f.type === "CANCEL")).toEqual([{ type: "CANCEL", id }]);
    h.t.close();
  });

  it("unary_should_SendCancelAndRejectDeadlineExceeded_When_TimeoutElapses", async () => {
    const h = setup();
    const p = settled(callListFiles(h.t, "/", undefined, 1000));
    h.sock().open();
    const id = h.sock().requestIds()[0];
    jest.advanceTimersByTime(1000);
    const r = await p;
    expect(!r.ok && ConnectError.from(r.error).code).toBe(Code.DeadlineExceeded);
    expect(h.sock().frames().some((f) => f.type === "CANCEL" && f.id === id)).toBe(true);
    h.t.close();
  });

  it("unary_should_NotSendCancel_When_AbortedWhileStillQueued", async () => {
    const h = setup();
    const ac = new AbortController();
    const p = settled(callListFiles(h.t, "/", ac.signal));
    ac.abort();
    expect((await p).ok).toBe(false);
    h.sock().open();
    expect(h.sock().sent).toHaveLength(0);
    h.t.close();
  });
});

describe("frame size routing", () => {
  function sessionIdLenForFrameSize(target: number): number {
    for (let n = target - 200; n < target; n++) {
      const body = toBinary(ListFilesRequestSchema, create(ListFilesRequestSchema, { sessionId: "x".repeat(n), path: "/" }));
      const size = encodeFrame({
        type: "REQUEST",
        id: 0,
        method: "/session.v1.SessionService/ListFiles",
        meta: { headers: { "Connect-Protocol-Version": "1" }, timeoutMs: 30000 },
        body,
      }).length;
      if (size === target) return n;
    }
    throw new Error("no size match");
  }

  it("unary_should_UseHttp_When_WholeFrameExceeds262144Bytes_AndWs_WhenExactlyAtLimit", () => {
    const n = sessionIdLenForFrameSize(MAX_FRAME_BYTES);
    const h = setup();
    void h.t.unary(listFiles, undefined, undefined, undefined, { sessionId: "x".repeat(n), path: "/" });
    h.sock().open();
    expect(h.sock().sent).toHaveLength(1);
    expect(h.sock().sent[0].length).toBe(MAX_FRAME_BYTES);
    expect(h.fallback.unary).not.toHaveBeenCalled();

    void h.t.unary(listFiles, undefined, undefined, undefined, { sessionId: "x".repeat(n + 1), path: "/" });
    expect(h.sock().sent).toHaveLength(1);
    expect(h.fallback.unary).toHaveBeenCalledTimes(1);

    void h.t.unary(listFiles, undefined, undefined, undefined, { sessionId: "x".repeat(300 * 1024), path: "/" });
    expect(h.fallback.unary).toHaveBeenCalledTimes(2);
    h.t.close();
  });
});

describe("send queue and window", () => {
  it("unary_should_ReplayReadOverHttp_When_SendQueueFullWhileConnecting", async () => {
    const track = jest.fn();
    setRpcTimingSink({ track });
    const h = setup();
    const ps = Array.from({ length: 65 }, (_, i) => settled(callListFiles(h.t, `/${i}`)));
    const last = await ps[64];
    expect(last.ok).toBe(true);
    expect(h.fallback.unary).toHaveBeenCalledTimes(1);
    expect(track).toHaveBeenCalledWith(
      expect.objectContaining({ name: "ws_unary_fallback_total", labels: { reason: "ws-send-queue-full" } })
    );
    h.sock().open();
    expect(h.sock().requestIds().length).toBe(MAX_IN_FLIGHT); // 64 queued, all fit the window
    setRpcTimingSink(undefined);
    h.t.close();
  });

  it("unary_should_RejectResourceExhausted_When_SendQueueFull_And_CallNotReplayable", async () => {
    const h = setup();
    for (let i = 0; i < 64; i++) void callListFiles(h.t, `/${i}`);
    const p = settled(h.t.unary(createSession, undefined, undefined, undefined, {}));
    const r = await p;
    expect(!r.ok && ConnectError.from(r.error).code).toBe(Code.ResourceExhausted);
    expect(h.fallback.unary).not.toHaveBeenCalled();
    h.t.close();
  });

  it("unary_should_KeepAtMostMaxInFlightOutstanding_And_SendNextWhenOneSettles", async () => {
    const h = setup();
    h.t.connect();
    h.sock().open();
    const ps = Array.from({ length: MAX_IN_FLIGHT + 2 }, (_, i) => callListFiles(h.t, `/${i}`));
    expect(h.sock().requestIds()).toHaveLength(MAX_IN_FLIGHT);
    h.sock().receive(response(h.sock().requestIds()[0], "/0"));
    await ps[0];
    expect(h.sock().requestIds()).toHaveLength(MAX_IN_FLIGHT + 1);
    h.t.close();
  });

  it("unary_should_UseHttpForQueuedCalls_When_SocketStillConnectingAfter250ms", async () => {
    const h = setup();
    const p = callListFiles(h.t, "/q");
    jest.advanceTimersByTime(250);
    await p;
    expect(h.fallback.unary).toHaveBeenCalledTimes(1);
    // a call arriving after the window goes straight to HTTP without waiting
    void callListFiles(h.t, "/late");
    expect(h.fallback.unary).toHaveBeenCalledTimes(2);
    h.t.close();
  });
});

describe("reconnect and replay", () => {
  it("reconnect_should_ReplayReadOverHttp_And_RejectMutationUnavailable_When_Socket1006", async () => {
    const h = setup();
    const read = callVcs(h.t);
    const write = settled(h.t.unary(createSession, undefined, undefined, undefined, create(CreateSessionRequestSchema, {})));
    h.sock().open();
    expect(h.sock().requestIds()).toHaveLength(2);
    h.sock().serverClose(1006);

    const w = await write;
    expect(!w.ok && ConnectError.from(w.error).code).toBe(Code.Unavailable);
    await read;
    expect(h.fallback.unary).toHaveBeenCalledTimes(1);
    expect(h.fallback.unary.mock.calls[0][0].name).toBe("GetVCSStatus");
    h.t.close();
  });

  it("reconnect_should_BackOff100_200ms_ThenOpenBreakerAfter3Failures_And_ProbeAfter60s", async () => {
    const h = setup();
    h.t.connect();
    h.sock(0).open();
    h.sock(0).serverClose(1006); // failure 1 -> retry in 100ms
    jest.advanceTimersByTime(99);
    expect(FakeWebSocket.instances).toHaveLength(1);
    jest.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(2);

    h.sock(1).onerror?.(); // handshake failure 2 -> 200ms
    jest.advanceTimersByTime(199);
    expect(FakeWebSocket.instances).toHaveLength(2);
    jest.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(3);

    h.sock(2).serverClose(1006); // failure 3 -> breaker opens
    expect(h.t.breaker.isOpen()).toBe(true);
    void callListFiles(h.t);
    expect(h.fallback.unary).toHaveBeenCalledTimes(1); // HTTP-only while open
    jest.advanceTimersByTime(59_999);
    expect(FakeWebSocket.instances).toHaveLength(3);
    jest.advanceTimersByTime(1);
    expect(FakeWebSocket.instances).toHaveLength(4); // single probe

    h.sock(3).open();
    const p = callListFiles(h.t, "/ok");
    h.sock(3).receive(response(h.sock(3).requestIds()[0], "/ok"));
    await p;
    expect(h.t.breaker.failures).toBe(0);
    h.t.close();
  });

  it("lateReply_should_BeDropped_When_IdAlreadySettledCancelledOrReplayedOverHttp", async () => {
    const h = setup();
    // replayed: HTTP result stands even if a WS reply for the old id shows up on the next socket
    let release: (r: UnaryResponse<DescMessage, DescMessage>) => void = () => undefined;
    h.fallback.unary.mockImplementationOnce(
      () => new Promise((r) => { release = r as typeof release; })
    );
    const replayed = callVcs(h.t);
    h.sock(0).open();
    const replayedId = h.sock(0).requestIds()[0];
    h.sock(0).serverClose(1006);
    jest.advanceTimersByTime(100);
    h.sock(1).open();
    h.sock(1).receive(response(replayedId, "/stale"));
    release(httpResponse(getVCSStatus, create(getVCSStatus.output, {})));
    const res = await replayed;
    expect(res.message.$typeName).toBe(getVCSStatus.output.typeName);

    // settled: a duplicate reply is ignored
    const p = callListFiles(h.t, "/a");
    const id = h.sock(1).requestIds()[0];
    h.sock(1).receive(response(id, "/a"));
    expect((await p).message.basePath).toBe("/a");
    expect(() => h.sock(1).receive(response(id, "/dup"))).not.toThrow();

    // cancelled: a reply after abort is ignored, socket stays up
    const ac = new AbortController();
    const c = settled(callListFiles(h.t, "/c", ac.signal));
    const cid = h.sock(1).requestIds().at(-1) as number;
    ac.abort();
    await c;
    expect(() => h.sock(1).receive(response(cid, "/late"))).not.toThrow();
    expect(h.sock(1).closed).toBe(false);
    h.t.close();
  });
});

describe("liveness", () => {
  it("liveness_should_CloseRejectUnavailableAndCountBreaker_When_PongMissingFor5sWhileCallInFlight", async () => {
    const h = setup();
    const read = callVcs(h.t);
    const write = settled(h.t.unary(createSession, undefined, undefined, undefined, create(CreateSessionRequestSchema, {})));
    h.sock().open();

    jest.advanceTimersByTime(10_000);
    expect(h.sock().frames().filter((f) => f.type === "PING")).toHaveLength(1);
    expect(h.sock().closed).toBe(false);
    jest.advanceTimersByTime(5_000);

    expect(h.sock().closed).toBe(true);
    expect(h.t.breaker.failures).toBe(1);
    const w = await write;
    expect(!w.ok && ConnectError.from(w.error).code).toBe(Code.Unavailable);
    await read;
    expect(h.fallback.unary).toHaveBeenCalledTimes(1);
    h.t.close();
  });

  it("liveness_should_StayOpen_When_PongArrivesWithin5s", () => {
    const h = setup();
    void callVcs(h.t);
    h.sock().open();
    jest.advanceTimersByTime(10_000);
    h.sock().receive({ type: "PONG", id: 0 });
    jest.advanceTimersByTime(9_999); // next ping is at 20s, nothing pending
    expect(h.sock().closed).toBe(false);
    expect(h.t.breaker.failures).toBe(0);
    h.t.close();
  });

  it("liveness_should_StayOpen_When_AnyInboundFrameArrivesInsteadOfPong", () => {
    const h = setup();
    void callVcs(h.t);
    h.sock().open();
    jest.advanceTimersByTime(10_000); // PING sent, pong deadline armed
    h.sock().receive({ type: "PING", id: 7 }); // any frame proves the socket is alive
    jest.advanceTimersByTime(5_000);
    expect(h.sock().closed).toBe(false);
    expect(h.t.breaker.failures).toBe(0);
    h.t.close();
  });

  it("liveness_should_WaitLonger_When_DocumentIsHidden", () => {
    const hidden = jest.spyOn(document, "hidden", "get").mockReturnValue(true);
    try {
      const h = setup();
      callVcs(h.t).catch(() => undefined); // the 30s call deadline fires during the advance
      h.sock().open();
      jest.advanceTimersByTime(10_000);
      jest.advanceTimersByTime(PONG_DEADLINE_MS + 1);
      expect(h.sock().closed).toBe(false);
      jest.advanceTimersByTime(PONG_DEADLINE_HIDDEN_MS);
      expect(h.sock().closed).toBe(true);
      h.t.close();
    } finally {
      hidden.mockRestore();
    }
  });

  it("liveness_should_GiveHiddenGrace_When_TabHidesWhileWaitingForPong", () => {
    const h = setup();
    callVcs(h.t).catch(() => undefined);
    h.sock().open();
    jest.advanceTimersByTime(10_000); // PING sent while visible
    const hidden = jest.spyOn(document, "hidden", "get").mockReturnValue(true);
    try {
      jest.advanceTimersByTime(PONG_DEADLINE_MS + 1);
      expect(h.sock().closed).toBe(false);
      jest.advanceTimersByTime(PONG_DEADLINE_HIDDEN_MS - PONG_DEADLINE_MS);
      expect(h.sock().closed).toBe(true);
    } finally {
      hidden.mockRestore();
      h.t.close();
    }
  });

  it("liveness_should_NotPing_When_NoCallInFlight", () => {
    const h = setup();
    h.t.connect();
    h.sock().open();
    jest.advanceTimersByTime(60_000);
    expect(h.sock().sent).toHaveLength(0);
    h.t.close();
  });

  it("liveness_should_PingImmediately_When_PageBecomesVisibleOrOnline", () => {
    const h = setup();
    void callVcs(h.t);
    h.sock().open();
    window.dispatchEvent(new Event("online"));
    expect(h.sock().frames().filter((f) => f.type === "PING")).toHaveLength(1);
    h.sock().receive({ type: "PONG", id: 0 });
    document.dispatchEvent(new Event("visibilitychange"));
    expect(h.sock().frames().filter((f) => f.type === "PING")).toHaveLength(2);
    h.t.close();
  });
});

describe("interceptor placement", () => {
  it("rpcTiming_should_BeEmittedOncePerCall_On_WsPathAndHttpPath", async () => {
    const h = setup();
    const track = jest.fn();
    const timing = createRpcTimingInterceptor({ track });
    const outer = (inner: Transport) => (path: string) =>
      runUnaryCall({
        interceptors: [timing],
        req: {
          stream: false,
          service: listFiles.parent,
          method: listFiles,
          requestMethod: "POST",
          url: "http://localhost:8543/x",
          header: new Headers(),
          contextValues: undefined as never,
          message: { sessionId: "s", path } as MessageInitShape<typeof ListFilesRequestSchema>,
        },
        next: (req) => inner.unary(listFiles, req.signal, undefined, req.header, req.message),
      });

    const viaWs = outer(h.t)("/ws");
    h.sock().open();
    h.sock().receive(response(h.sock().requestIds()[0], "/ws"));
    await viaWs;
    expect(track.mock.calls.filter((c) => c[0].name === "rpc.ListFiles")).toHaveLength(1);

    track.mockClear();
    h.t.close();
    const breaker = new UnaryWsBreaker();
    for (let i = 0; i < 3; i++) breaker.recordFailure(); // open breaker: HTTP only
    const httpH = setup({ breaker });
    await outer(httpH.t)("/http");
    expect(httpH.fallback.unary).toHaveBeenCalledTimes(1);
    expect(track.mock.calls.filter((c) => c[0].name === "rpc.ListFiles")).toHaveLength(1);
  });
});
