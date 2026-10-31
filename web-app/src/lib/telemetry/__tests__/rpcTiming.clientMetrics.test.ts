import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, createContextValues } from "@connectrpc/connect";
import type { UnaryRequest } from "@connectrpc/connect";
import { GetFeatureFlagsRequestSchema, SessionService } from "@/gen/session/v1/session_pb";
import {
  codeLabel,
  createRpcTimingInterceptor,
  markRpcTransport,
  recordWsLifecycle,
  setRpcTimingSink,
  trackSharedEvent,
} from "../rpcTiming";

function req(): UnaryRequest {
  return {
    stream: false,
    service: SessionService,
    method: SessionService.method.getFeatureFlags,
    message: create(GetFeatureFlagsRequestSchema, {}),
    signal: new AbortController().signal,
    header: new Headers(),
    url: "http://x/GetFeatureFlags",
    requestMethod: "POST",
    contextValues: createContextValues(),
  } as UnaryRequest;
}

type Handler = (r: UnaryRequest) => Promise<unknown>;

function deferredNext() {
  const pending: Array<{ resolve: () => void; reject: (e: unknown) => void; req: UnaryRequest }> = [];
  const next = (r: UnaryRequest) =>
    new Promise<unknown>((resolve, reject) => pending.push({ resolve: () => resolve({}), reject, req: r }));
  return { pending, next };
}

describe("client RPC metrics labels", () => {
  it("labels the call with the transport the inner layer marked and code ok", async () => {
    const track = jest.fn();
    const handler = createRpcTimingInterceptor({ track })(((r: UnaryRequest) => {
      markRpcTransport(r.contextValues, "ws");
      return Promise.resolve({});
    }) as never) as Handler;
    await handler(req());
    expect(track.mock.calls[0][0].labels).toMatchObject({ transport: "ws", code: "ok", inflight: "0" });
    expect(track.mock.calls[0][0].labels).not.toHaveProperty("dup");
  });

  it("defaults the transport to http when no inner layer marks it", async () => {
    const track = jest.fn();
    const handler = createRpcTimingInterceptor({ track })((() => Promise.resolve({})) as never) as Handler;
    await handler(req());
    expect(track.mock.calls[0][0].labels).toMatchObject({ transport: "http" });
  });

  it("labels server-streaming calls as stream so dashboards can exclude them from unary latency", async () => {
    const track = jest.fn();
    const handler = createRpcTimingInterceptor({ track })((() => Promise.resolve({})) as never) as Handler;
    await handler({ ...req(), stream: true } as unknown as UnaryRequest);
    expect(track.mock.calls[0][0].labels).toMatchObject({ transport: "stream" });
    expect(track.mock.calls[0][0].labels).not.toHaveProperty("dup");
  });

  it("reports the Connect code as snake_case when the call fails", async () => {
    const track = jest.fn();
    const handler = createRpcTimingInterceptor({ track })((() =>
      Promise.reject(new ConnectError("slow", Code.DeadlineExceeded))) as never) as Handler;
    await expect(handler(req())).rejects.toThrow();
    expect(track.mock.calls[0][0].labels).toMatchObject({ ok: "false", code: "deadline_exceeded" });
  });

  it("flags an identical request started while another is in flight, and counts concurrency", async () => {
    const track = jest.fn();
    const { pending, next } = deferredNext();
    const handler = createRpcTimingInterceptor({ track }, { detectDuplicates: true })(next as never) as Handler;
    const first = handler(req());
    const second = handler(req());
    pending[0].resolve();
    pending[1].resolve();
    await Promise.all([first, second]);

    const [firstEvent, secondEvent] = track.mock.calls.map((c) => c[0]);
    expect(firstEvent.labels).toMatchObject({ inflight: "0" });
    expect(firstEvent.labels).not.toHaveProperty("dup");
    expect(secondEvent.labels).toMatchObject({ inflight: "1", dup: "1", coalescible: "1" });
  });

  it("marks a duplicated read that coalescing never covers as coalescible=0 (a candidate to add)", async () => {
    const track = jest.fn();
    const { pending, next } = deferredNext();
    const handler = createRpcTimingInterceptor({ track }, { detectDuplicates: true })(next as never) as Handler;
    const read = () => ({ ...req(), method: SessionService.method.getSessionDefaults }) as unknown as UnaryRequest;
    const both = Promise.all([handler(read()), handler(read())]);
    pending[0].resolve();
    pending[1].resolve();
    await both;
    expect(track.mock.calls[1][0].labels).toMatchObject({ dup: "1", coalescible: "0" });
  });

  it("does not detect duplicates unless asked (the non-shared interceptor sits outside coalescing)", async () => {
    const track = jest.fn();
    const { pending, next } = deferredNext();
    const handler = createRpcTimingInterceptor({ track })(next as never) as Handler;
    const both = Promise.all([handler(req()), handler(req())]);
    pending[0].resolve();
    pending[1].resolve();
    await both;
    expect(track.mock.calls[1][0].labels).not.toHaveProperty("dup");
  });

  it("a throwing analytics sink never changes the RPC result", async () => {
    const track = jest.fn(() => {
      throw new Error("sink exploded");
    });
    const handler = createRpcTimingInterceptor({ track })((() => Promise.resolve({ ok: 1 })) as never) as Handler;
    await expect(handler(req())).resolves.toEqual({ ok: 1 });
    expect(track).toHaveBeenCalled();
  });

  it("never flags overlapping identical writes as duplicates", async () => {
    const track = jest.fn();
    const { pending, next } = deferredNext();
    const handler = createRpcTimingInterceptor({ track }, { detectDuplicates: true })(next as never) as Handler;
    const write = () => ({ ...req(), method: SessionService.method.createSession }) as unknown as UnaryRequest;
    const first = handler(write());
    const second = handler(write());
    pending[0].resolve();
    pending[1].resolve();
    await Promise.all([first, second]);
    expect(track.mock.calls[1][0].labels).not.toHaveProperty("dup");
  });

  it("does not flag sequential identical requests as duplicates", async () => {
    const track = jest.fn();
    const handler = createRpcTimingInterceptor({ track })((() => Promise.resolve({})) as never) as Handler;
    await handler(req());
    await handler(req());
    expect(track.mock.calls[1][0].labels).not.toHaveProperty("dup");
  });
});

describe("codeLabel", () => {
  it("maps non-Connect errors to unknown", () => {
    expect(codeLabel(new Error("boom"))).toBe("unknown");
    expect(codeLabel(new ConnectError("x", Code.ResourceExhausted))).toBe("resource_exhausted");
  });
});

describe("recordWsLifecycle", () => {
  afterEach(() => setRpcTimingSink(undefined));

  it("emits a cm.ws event with kind, event and the close code when present", () => {
    const track = jest.fn();
    setRpcTimingSink({ track });
    recordWsLifecycle("unary", "close", 1006);
    recordWsLifecycle("watch", "open");
    expect(track.mock.calls[0][0]).toEqual({
      name: "cm.ws",
      category: "performance",
      labels: { kind: "unary", event: "close", code: "1006" },
    });
    expect(track.mock.calls[1][0].labels).toEqual({ kind: "watch", event: "open" });
  });

  it("drops events silently until a sink is registered", () => {
    expect(() => trackSharedEvent({ name: "cm.ws", category: "performance" })).not.toThrow();
  });
});
