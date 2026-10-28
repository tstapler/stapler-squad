import { create, toBinary } from "@bufbuild/protobuf";
import type { DescMethodUnary } from "@bufbuild/protobuf";
import { GetVCSStatusResponseSchema, SessionService } from "@/gen/session/v1/session_pb";
import { setRpcTimingSink } from "@/lib/telemetry/rpcTiming";
import { FakeWebSocket } from "@/lib/transport/__fixtures__/fakeUnaryWebSocket";
import { _resetAuthRequiredForTesting } from "@/lib/transport/wsUnaryPolicy";
import { _resetTransportForTesting, getConnectTransport } from "../transport";

const httpUnary = jest.fn(async (method: DescMethodUnary) => ({
  stream: false,
  service: method.parent,
  method,
  header: new Headers(),
  trailer: new Headers(),
  message: create(method.output, {} as never),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: (...a: unknown[]) => (httpUnary as jest.Mock)(...a.slice(0, 1)), stream: jest.fn() })),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: jest.fn(() => "http://localhost:62871/api"),
  createAuthInterceptor: jest.fn(() => (next: (r: unknown) => unknown) => next),
}));
jest.mock("@/lib/features/flagStore", () => ({
  peekFlags: jest.fn(() => [{ name: "unary_over_ws", enabled: true }]),
  preloadFlagValue: jest.fn(() => undefined),
}));

const getVCSStatus = SessionService.method.getVCSStatus;
const track = jest.fn();

beforeEach(() => {
  _resetTransportForTesting();
  _resetAuthRequiredForTesting();
  FakeWebSocket.instances = [];
  jest.clearAllMocks();
  (globalThis as { WebSocket: unknown }).WebSocket = FakeWebSocket;
  setRpcTimingSink({ track });
});
afterEach(() => setRpcTimingSink(undefined));

describe("getConnectTransport policy wiring", () => {
  it("rpcTiming_should_BeEmittedOncePerCall_When_CallRoutesOverWsOrHttp", async () => {
    const transport = getConnectTransport();

    await transport.unary(SessionService.method.listSessions, undefined, undefined, undefined, {});
    expect(httpUnary).toHaveBeenCalledTimes(1);
    expect(track.mock.calls.filter(([e]) => e.name === "rpc.ListSessions")).toHaveLength(1);

    const call = transport.unary(getVCSStatus, undefined, undefined, undefined, { id: "s" } as never);
    const sock = FakeWebSocket.instances[0];
    sock.open();
    await Promise.resolve();
    const body = toBinary(GetVCSStatusResponseSchema, create(GetVCSStatusResponseSchema, {}));
    sock.receive({ type: "RESPONSE", id: sock.requestIds()[0], totalLen: body.length, fin: true, meta: { headers: {}, trailers: {} }, body });
    await call;
    expect(httpUnary).toHaveBeenCalledTimes(1);
    expect(track.mock.calls.filter(([e]) => e.name === "rpc.GetVCSStatus")).toHaveLength(1);
  });

  it("transport_should_SendOneRequestFrameWithDefaultDeadline_When_14ConcurrentGetVCSStatusCallsStartInOneTick", async () => {
    const transport = getConnectTransport();
    const calls = Array.from({ length: 14 }, () =>
      transport.unary(getVCSStatus, undefined, undefined, undefined, { id: "s1" } as never),
    );
    const sock = FakeWebSocket.instances[0];
    sock.open();
    await Promise.resolve();
    const reqs = sock.frames().filter((f) => f.type === "REQUEST");
    expect(reqs).toHaveLength(1);
    expect((reqs[0] as { meta: { timeoutMs: number } }).meta.timeoutMs).toBe(30_000);
    const body = toBinary(GetVCSStatusResponseSchema, create(GetVCSStatusResponseSchema, {}));
    sock.receive({ type: "RESPONSE", id: sock.requestIds()[0], totalLen: body.length, fin: true, meta: { headers: {}, trailers: {} }, body });
    await Promise.all(calls);
  });
});
