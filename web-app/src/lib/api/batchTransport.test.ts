import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import type { DescMethodUnary } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import {
  ListShellsRequestSchema,
  ListShellsResponseSchema,
  SessionService,
  StatFileRequestSchema,
} from "@/gen/session/v1/session_pb";
import {
  BATCH_MAX_CALLS,
  BATCH_PATH,
  BATCH_WINDOW_MS,
  createBatchTransport,
} from "./batchTransport";
import { setRpcTimingSink } from "@/lib/telemetry/rpcTiming";

const listShells = SessionService.method.listShells;
const statFile = SessionService.method.statFile;
const getVCSStatus = SessionService.method.getVCSStatus;

interface WireCall {
  id: number;
  method: string;
  body: string;
  timeoutMs: number;
}

type Reply = (calls: WireCall[]) => unknown[] | Promise<unknown[]>;

function b64(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes));
}

/** A reply that answers every ListShells call with one shell named after its session id. */
function okReply(calls: WireCall[]): unknown[] {
  return calls.map((c) => ({
    id: c.id,
    body: b64(toBinary(ListShellsResponseSchema, create(ListShellsResponseSchema, { shells: [{ id: c.id.toString() }] }))),
  }));
}

function setup(opts: { reply?: Reply; enabled?: boolean; wsRouted?: boolean } = {}) {
  const inner = {
    unary: jest.fn(async (method: DescMethodUnary) => ({
      stream: false,
      service: method.parent,
      method,
      header: new Headers(),
      trailer: new Headers(),
      message: create(method.output, {}),
    })),
    stream: jest.fn(),
  };
  const sent: WireCall[][] = [];
  const signals: AbortSignal[] = [];
  const fetchMock = jest.fn(async (_url: string, init: RequestInit) => {
    const calls = (JSON.parse(init.body as string) as { calls: WireCall[] }).calls;
    sent.push(calls);
    signals.push(init.signal as AbortSignal);
    const results = await (opts.reply ?? okReply)(calls);
    return { ok: true, status: 200, json: async () => ({ results }) } as Response;
  });
  const transport = createBatchTransport({
    inner: inner as unknown as Transport,
    baseUrl: "http://h/api",
    fetch: fetchMock as unknown as typeof fetch,
    isEnabled: () => opts.enabled ?? true,
    isWsRouted: () => opts.wsRouted ?? false,
  });
  return { transport, inner, fetchMock, sent, signals };
}

function shells(t: Transport, sessionId: string, signal?: AbortSignal) {
  return t.unary(listShells, signal, undefined, undefined, create(ListShellsRequestSchema, { sessionId }));
}

function rejection(p: Promise<unknown>): Promise<ConnectError> {
  return p.then(
    () => {
      throw new Error("expected rejection");
    },
    (e: ConnectError) => e,
  );
}

async function flushWindow(): Promise<void> {
  await jest.advanceTimersByTimeAsync(BATCH_WINDOW_MS);
}

beforeEach(() => jest.useFakeTimers());
afterEach(() => jest.useRealTimers());

describe("batchTransport client metrics events", () => {
  const track = jest.fn();
  beforeEach(() => {
    track.mockClear();
    setRpcTimingSink({ track });
  });
  afterEach(() => setRpcTimingSink(undefined));

  it("batchTransport_should_EmitBatchSize_When_CallsShareTheWindow", async () => {
    const { transport } = setup();
    const calls = [shells(transport, "a"), shells(transport, "b"), shells(transport, "c")];
    await flushWindow();
    await Promise.all(calls);
    expect(track).toHaveBeenCalledWith({ name: "cm.batch", category: "performance", labels: { size: "3" } });
    expect(track.mock.calls.filter(([e]) => e.name === "cm.batch_fallback")).toHaveLength(0);
  });

  it("batchTransport_should_CountServerDeclinedFallbacks", async () => {
    const reply: Reply = (calls) => [okReply(calls)[0], { id: calls[1].id, fallback: true }];
    const { transport } = setup({ reply });
    const calls = [shells(transport, "a"), shells(transport, "b")];
    await flushWindow();
    await Promise.all(calls);
    expect(track).toHaveBeenCalledWith({ name: "cm.batch_fallback", category: "performance", labels: { reason: "server-declined" } });
  });

  it("batchTransport_should_CountRequestFailedFallbackPerMember_When_TheBatchRequestRejects", async () => {
    const inner = { unary: jest.fn(async (m: DescMethodUnary) => ({ stream: false, service: m.parent, method: m, header: new Headers(), trailer: new Headers(), message: create(m.output, {}) })), stream: jest.fn() };
    const transport = createBatchTransport({
      inner: inner as unknown as Transport,
      baseUrl: "http://h/api",
      fetch: (async () => { throw new TypeError("network"); }) as unknown as typeof fetch,
      isEnabled: () => true,
    });
    const calls = [shells(transport, "a"), shells(transport, "b")];
    await flushWindow();
    await Promise.all(calls);
    expect(track.mock.calls.filter(([e]) => e.name === "cm.batch_fallback" && e.labels.reason === "request-failed")).toHaveLength(2);
  });
});

describe("batchTransport", () => {
  it("batchTransport_should_NotTouchCalls_When_FlagIsOff", async () => {
    const { transport, inner, fetchMock } = setup({ enabled: false });
    await Promise.all([shells(transport, "a"), shells(transport, "b")]);
    expect(inner.unary).toHaveBeenCalledTimes(2);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("batchTransport_should_SendOneRequest_When_CallsShareTheWindow", async () => {
    const { transport, inner, fetchMock, sent } = setup();
    const calls = [shells(transport, "a"), shells(transport, "b"), shells(transport, "c")];
    expect(fetchMock).not.toHaveBeenCalled();
    await flushWindow();
    const results = await Promise.all(calls);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe("http://h/api" + BATCH_PATH);
    expect(sent[0].map((c) => c.method)).toEqual(Array(3).fill("/session.v1.SessionService/ListShells"));
    expect(inner.unary).not.toHaveBeenCalled();
    const decoded = results.map((r) => (r.message as { shells: { id: string }[] }).shells[0].id);
    expect(decoded).toEqual(sent[0].map((c) => c.id.toString()));
  });

  it("batchTransport_should_PassABatchOfOneThroughUnchanged", async () => {
    const { transport, inner, fetchMock } = setup();
    const call = shells(transport, "solo");
    await flushWindow();
    await call;
    expect(fetchMock).not.toHaveBeenCalled();
    expect(inner.unary).toHaveBeenCalledTimes(1);
  });

  it("batchTransport_should_SendIneligibleCallsStraightThrough_And_NeverBatchThem", async () => {
    const { transport, inner, fetchMock } = setup();
    await transport.unary(getVCSStatus, undefined, undefined, undefined, { id: "s" } as never);
    await transport.unary(listShells, undefined, undefined, { "X-Custom": "1" }, { sessionId: "a" } as never);
    expect(inner.unary).toHaveBeenCalledTimes(2);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("batchTransport_should_SkipBatching_When_CallsAlreadyTravelOverTheWebSocket", async () => {
    const { transport, inner, fetchMock } = setup({ wsRouted: true });
    await Promise.all([shells(transport, "a"), shells(transport, "b")]);
    expect(inner.unary).toHaveBeenCalledTimes(2);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("batchTransport_should_RejectOnlyTheFailedCall_When_OneItemErrors", async () => {
    const reply: Reply = (calls) => [
      okReply(calls)[0],
      { id: calls[1].id, error: { code: "not_found", message: "no such session" } },
    ];
    const { transport } = setup({ reply });
    const ok = shells(transport, "a");
    const failed = shells(transport, "b");
    const failure = rejection(failed);
    await flushWindow();

    await expect(ok).resolves.toBeDefined();
    const err = await failure;
    expect(err).toBeInstanceOf(ConnectError);
    expect(err.code).toBe(Code.NotFound);
    expect(err.rawMessage).toBe("no such session");
  });

  it("batchTransport_should_RunDeclinedItemsIndividually_When_ServerSetsFallback", async () => {
    const reply: Reply = (calls) => [okReply(calls)[0], { id: calls[1].id, fallback: true }];
    const { transport, inner } = setup({ reply });
    const calls = [shells(transport, "a"), shells(transport, "b")];
    await flushWindow();
    await Promise.all(calls);
    expect(inner.unary).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["the request rejects", async () => { throw new TypeError("network"); }],
    ["the envelope is not ok", async () => ({ ok: false, status: 503, json: async () => ({}) })],
    ["the envelope is malformed", async () => ({ ok: true, status: 200, json: async () => ({ nope: 1 }) })],
    ["an item body cannot be decoded", async () => ({ ok: true, status: 200, json: async () => ({ results: [{ id: 1, body: "////" }, { id: 2, body: "////" }] }) })],
  ])("batchTransport_should_FallBackToIndividualCalls_When_%s", async (_name, doFetch) => {
    const inner = { unary: jest.fn(async (m: DescMethodUnary) => ({ stream: false, service: m.parent, method: m, header: new Headers(), trailer: new Headers(), message: create(m.output, {}) })), stream: jest.fn() };
    const transport = createBatchTransport({
      inner: inner as unknown as Transport,
      baseUrl: "http://h/api",
      fetch: doFetch as unknown as typeof fetch,
      isEnabled: () => true,
    });
    const calls = [shells(transport, "a"), shells(transport, "b")];
    await flushWindow();
    await Promise.all(calls);
    expect(inner.unary).toHaveBeenCalledTimes(2);
  });

  it("batchTransport_should_DropAnAbortedCallFromTheWindow_And_BatchTheRest", async () => {
    const { transport, sent } = setup();
    const ac = new AbortController();
    const aborted = rejection(shells(transport, "a", ac.signal));
    const rest = [shells(transport, "b"), shells(transport, "c")];
    ac.abort();

    expect((await aborted).code).toBe(Code.Canceled);
    await flushWindow();
    await Promise.all(rest);
    expect(sent[0]).toHaveLength(2);
  });

  it("batchTransport_should_RejectAlreadyAbortedCallsWithoutSending", async () => {
    const { transport, fetchMock, inner } = setup();
    const ac = new AbortController();
    ac.abort();
    await expect(shells(transport, "a", ac.signal)).rejects.toMatchObject({ code: Code.Canceled });
    await flushWindow();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(inner.unary).not.toHaveBeenCalled();
  });

  it("batchTransport_should_IsolateAbortAfterFlush_When_OnlyOneCallerAborts", async () => {
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const { transport, signals } = setup({ reply: async (calls) => (await gate, okReply(calls)) });
    const ac = new AbortController();
    const aborted = rejection(shells(transport, "a", ac.signal));
    const kept = shells(transport, "b");
    await flushWindow();

    ac.abort();
    expect((await aborted).code).toBe(Code.Canceled);
    expect(signals[0].aborted).toBe(false);
    release();
    await expect(kept).resolves.toBeDefined();
  });

  it("batchTransport_should_AbortTheRequest_When_EveryCallerAborts", async () => {
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const { transport, signals } = setup({ reply: async (calls) => (await gate, okReply(calls)) });
    const a = new AbortController();
    const b = new AbortController();
    const pa = rejection(shells(transport, "a", a.signal));
    const pb = rejection(shells(transport, "b", b.signal));
    await flushWindow();

    a.abort();
    expect(signals[0].aborted).toBe(false);
    b.abort();
    expect(signals[0].aborted).toBe(true);
    await Promise.all([pa, pb]);
    release();
  });

  it("batchTransport_should_FlushAtTheCallCap_And_PassASingleLeftoverThrough", async () => {
    const { transport, sent, inner } = setup();
    const calls = Array.from({ length: BATCH_MAX_CALLS + 1 }, (_, i) => shells(transport, `s${i}`));
    await flushWindow();
    await Promise.all(calls);

    expect(sent).toHaveLength(1);
    expect(sent[0]).toHaveLength(BATCH_MAX_CALLS);
    expect(inner.unary).toHaveBeenCalledTimes(1);
  });

  it("batchTransport_should_SplitBatches_When_RequestBytesExceedTheCap", async () => {
    const { transport, sent } = setup({ reply: (calls) => calls.map((c) => ({ id: c.id, body: "" })) });
    const big = (sessionId: string) =>
      transport.unary(statFile, undefined, undefined, undefined, create(StatFileRequestSchema, { sessionId, path: "p".repeat(60 * 1024) }));
    const calls = ["a", "b", "c", "d", "e"].map(big);
    await flushWindow();
    await Promise.all(calls);

    expect(sent.length).toBeGreaterThan(1);
    for (const batch of sent) expect(JSON.stringify({ calls: batch }).length).toBeLessThanOrEqual(256 * 1024);
  });

  it("batchTransport_should_RoundTripTheProtoBody_When_ServerEchoesTheRequest", async () => {
    const reply: Reply = (calls) => calls.map((c) => {
      const req = fromBinary(ListShellsRequestSchema, Uint8Array.from(atob(c.body), (ch) => ch.charCodeAt(0)));
      return { id: c.id, body: b64(toBinary(ListShellsResponseSchema, create(ListShellsResponseSchema, { shells: [{ id: req.sessionId }] }))) };
    });
    const { transport } = setup({ reply });
    const [a, b] = [shells(transport, "alpha"), shells(transport, "beta")];
    await flushWindow();
    const ids = (await Promise.all([a, b])).map((r) => (r.message as { shells: { id: string }[] }).shells[0].id);
    expect(ids).toEqual(["alpha", "beta"]);
  });
});
