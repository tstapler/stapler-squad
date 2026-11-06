import fs from "fs";
import path from "path";
import { create, toBinary } from "@bufbuild/protobuf";
import type { DescMethodUnary } from "@bufbuild/protobuf";
import type { Transport, UnaryResponse } from "@connectrpc/connect";
import { Code, ConnectError } from "@connectrpc/connect";
import { GetVCSStatusResponseSchema, SessionService } from "@/gen/session/v1/session_pb";
import { FakeWebSocket } from "./__fixtures__/fakeUnaryWebSocket";
import type { MuxFrame } from "./unary-mux-frame";
import { BREAKER_OPEN_MS, createUnaryWsTransport } from "./unary-ws-transport";
import {
  _resetAuthRequiredForTesting,
  AuthAwareBreaker,
  createWsUnaryPolicyTransport,
  FALLBACK_REASONS,
  markAuthRequired,
  WS_CALL_TIMEOUT_MS,
  type FallbackReason,
} from "./wsUnaryPolicy";

const { getVCSStatus, listSessions, getFeatureFlags, getSessionDiff } = SessionService.method;
const emptyMeta = { headers: {}, trailers: {} };

interface Opts {
  flags?: Record<string, boolean>;
  origin?: "local" | "remote";
  forceHttp?: () => boolean;
  isReplayable?: (n: string) => boolean;
}

function setup(opts: Opts = {}) {
  FakeWebSocket.instances = [];
  const flags: Record<string, boolean> = { unary_over_ws: true, unary_over_ws_remote: false, ...opts.flags };
  const http = {
    unary: jest.fn(async (method: DescMethodUnary) => ({
      stream: false,
      service: method.parent,
      method,
      header: new Headers(),
      trailer: new Headers(),
      message: create(method.output, {} as never),
    })),
    stream: jest.fn(),
  };
  const breaker = new AuthAwareBreaker();
  const ws = createUnaryWsTransport({
    baseUrl: "http://localhost:62871/api",
    pageHref: "http://localhost:62871/",
    fallback: http as unknown as Transport,
    WebSocketImpl: FakeWebSocket as unknown as typeof WebSocket,
    random: () => 0,
    breaker,
  });
  const reasons: FallbackReason[] = [];
  const policy = createWsUnaryPolicyTransport({
    http: http as unknown as Transport,
    getWs: () => ws,
    breaker,
    readFlag: (n) => flags[n] ?? false,
    originClass: () => opts.origin ?? "local",
    isForceHttp: opts.forceHttp,
    isReplayable: opts.isReplayable,
    onFallback: (r) => reasons.push(r),
  });
  return { policy, http, ws, breaker, flags, reasons, sock: (i = 0) => FakeWebSocket.instances[i] };
}

type Harness = ReturnType<typeof setup>;

const vcs = (p: Transport) => p.unary(getVCSStatus, undefined, undefined, undefined, { id: "s" });

function vcsReply(id: number): MuxFrame {
  const body = toBinary(GetVCSStatusResponseSchema, create(GetVCSStatusResponseSchema, {}));
  return { type: "RESPONSE", id, totalLen: body.length, fin: true, meta: emptyMeta, body };
}

async function openAndAnswer(h: Harness, call: Promise<UnaryResponse<never, never>>) {
  h.sock().open();
  await Promise.resolve();
  h.sock().receive(vcsReply(h.sock().requestIds()[0]));
  return call;
}

beforeEach(() => {
  jest.useFakeTimers();
  _resetAuthRequiredForTesting();
});
afterEach(() => {
  jest.useRealTimers();
});

describe("WsUnaryPolicy", () => {
  it("policy_should_RouteWsOnlyForWsEligible_When_FlagOn_And_HttpForGetFeatureFlagsAndListSessions", async () => {
    const h = setup();
    await h.policy.unary(listSessions, undefined, undefined, undefined, {});
    await h.policy.unary(getFeatureFlags, undefined, undefined, undefined, {});
    expect(h.http.unary).toHaveBeenCalledTimes(2);
    expect(FakeWebSocket.instances).toHaveLength(0);

    const call = vcs(h.policy) as unknown as Promise<UnaryResponse<never, never>>;
    await openAndAnswer(h, call);
    expect(h.http.unary).toHaveBeenCalledTimes(2);
    expect(h.sock().requestIds()).toHaveLength(1);
  });

  it("policy_should_ReportWhyACallWentOverHttp", async () => {
    const h = setup();
    await h.policy.unary(listSessions, undefined, undefined, undefined, {});
    expect(h.reasons).toEqual(["not-eligible"]);

    const off = setup({ flags: { unary_over_ws: false } });
    await vcs(off.policy);
    // No flag payload is loaded in this harness, so "off" is reported as not-yet-loaded.
    expect(off.reasons).toEqual(["flags-not-loaded"]);

    const remote = setup({ origin: "remote" });
    await vcs(remote.policy);
    expect(remote.reasons).toEqual(["remote-flag-off"]);
  });

  it("policy_should_UseHttpWithoutOpen_When_FlagOff", async () => {
    const h = setup({ flags: { unary_over_ws: false } });
    await vcs(h.policy);
    expect(h.http.unary).toHaveBeenCalledTimes(1);
    expect(FakeWebSocket.instances).toHaveLength(0);
  });

  it("policy_should_UseWsOnHttpPort62871_And_GateHttpsOriginOnRemoteSubFlag_And_ClassifyByProtocolNotPort", async () => {
    const local = setup({ origin: "local", flags: { unary_over_ws_remote: false } });
    const call = vcs(local.policy) as unknown as Promise<UnaryResponse<never, never>>;
    await openAndAnswer(local, call);
    expect(local.sock().url).toBe("ws://localhost:62871/api/ws/unary");
    expect(local.http.unary).not.toHaveBeenCalled();

    const remoteOff = setup({ origin: "remote", flags: { unary_over_ws_remote: false } });
    await vcs(remoteOff.policy);
    expect(remoteOff.http.unary).toHaveBeenCalledTimes(1);
    expect(FakeWebSocket.instances).toHaveLength(0);

    const remoteOn = setup({ origin: "remote", flags: { unary_over_ws_remote: true } });
    const call2 = vcs(remoteOn.policy) as unknown as Promise<UnaryResponse<never, never>>;
    await openAndAnswer(remoteOn, call2);
    expect(remoteOn.http.unary).not.toHaveBeenCalled();
  });

  it.each([
    ["master off, sub on", { unary_over_ws: false, unary_over_ws_remote: true }],
    ["both off", { unary_over_ws: false, unary_over_ws_remote: false }],
    ["master on, sub absent", { unary_over_ws: true }],
  ])("policy_should_UseHttp_When_HttpsPageAndRemoteSubFlagOffOrAbsentOrMasterOff (%s)", async (_n, flags) => {
    const h = setup({ origin: "remote", flags: { unary_over_ws_remote: undefined as never, ...flags } });
    await vcs(h.policy);
    expect(h.http.unary).toHaveBeenCalledTimes(1);
    expect(FakeWebSocket.instances).toHaveLength(0);
  });

  it("policy_should_UseHttpWithoutOpen_When_3FailedOpensWithin60s_And_OneProbeAfter60s", async () => {
    const h = setup();
    for (let i = 0; i < 3; i++) {
      h.breaker.recordFailure();
    }
    expect(h.breaker.isOpen()).toBe(true);
    await vcs(h.policy);
    expect(h.http.unary).toHaveBeenCalledTimes(1);
    expect(h.reasons).toEqual(["breaker-open"]);
    expect(FakeWebSocket.instances).toHaveLength(0);

    jest.advanceTimersByTime(BREAKER_OPEN_MS + 1);
    void vcs(h.policy).catch(() => undefined);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });

  it("policy_should_NotCountFailedOpens_When_Http401SeenInSamePageLife_And_CountWhenNone", () => {
    const withAuth = setup();
    markAuthRequired();
    for (let i = 0; i < 3; i++) withAuth.breaker.recordFailure();
    expect(withAuth.breaker.isOpen()).toBe(false);

    _resetAuthRequiredForTesting();
    const without = setup();
    for (let i = 0; i < 3; i++) without.breaker.recordFailure();
    expect(without.breaker.isOpen()).toBe(true);
  });

  it("policy_should_DenylistMethodAndReplayOverHttp_When_ServerAnswersWsMethodNotAllowed", async () => {
    const h = setup();
    const first = getSessionDiff;
    const call = h.policy.unary(first, undefined, undefined, undefined, { id: "s" });
    h.sock().open();
    await Promise.resolve();
    h.sock().receive({
      type: "ERROR",
      id: h.sock().requestIds()[0],
      json: { code: "permission_denied", message: "no", details: { marker: "ws_method_not_allowed" } },
    } as MuxFrame);
    await call;
    expect(h.http.unary).toHaveBeenCalledTimes(1);
    expect(h.reasons).toEqual(["ws-method-not-allowed"]);

    await h.policy.unary(first, undefined, undefined, undefined, { id: "s" });
    expect(h.http.unary).toHaveBeenCalledTimes(2);
    expect(h.sock().requestIds()).toHaveLength(1);
  });

  it("policy_should_CancelAndReplayReadOverHttp_When_WsCallExceeds8s", async () => {
    const h = setup();
    const call = vcs(h.policy);
    h.sock().open();
    await Promise.resolve();
    jest.advanceTimersByTime(WS_CALL_TIMEOUT_MS);
    await call;
    expect(h.sock().frames().map((f) => f.type)).toEqual(["REQUEST", "CANCEL"]);
    expect(h.http.unary).toHaveBeenCalledTimes(1);
    expect((h.http.unary.mock.calls[0] as unknown[])[2]).toBe(30_000 - WS_CALL_TIMEOUT_MS);
    expect(h.reasons).toEqual(["ws-call-timeout"]);
    expect(h.breaker.failures).toBe(0);
  });

  it("policy_should_KeepDeadline_When_NonReplayableCallIsSlow", async () => {
    const h = setup({ isReplayable: () => false });
    const call = vcs(h.policy);
    h.sock().open();
    await Promise.resolve();
    jest.advanceTimersByTime(WS_CALL_TIMEOUT_MS + 1);
    expect(h.http.unary).not.toHaveBeenCalled();
    h.sock().receive(vcsReply(h.sock().requestIds()[0]));
    await call;
    expect(h.reasons).toEqual([]);
  });

  it.each([
    ["resource_exhausted", "ws-resource-exhausted"],
    ["unavailable", "ws-unavailable"],
  ])("policy_should_ReplayOverHttp_When_ReplayableCallGetsServerError_%s", async (code, reason) => {
    const h = setup();
    const call = vcs(h.policy);
    h.sock().open();
    await Promise.resolve();
    jest.advanceTimersByTime(2_000);
    h.sock().receive({ type: "ERROR", id: h.sock().requestIds()[0], json: { code, message: "shed" } } as MuxFrame);
    await call;
    expect(h.http.unary).toHaveBeenCalledTimes(1);
    expect((h.http.unary.mock.calls[0] as unknown[])[2]).toBe(30_000 - 2_000); // only the time left
    expect(h.reasons).toEqual([reason]);
  });

  it("policy_should_RejectServerResourceExhausted_When_CallIsNotReplayable", async () => {
    const h = setup({ isReplayable: () => false });
    const call = vcs(h.policy);
    h.sock().open();
    await Promise.resolve();
    h.sock().receive({
      type: "ERROR",
      id: h.sock().requestIds()[0],
      json: { code: "resource_exhausted", message: "shed" },
    } as MuxFrame);
    const err = await call.catch((e) => e);
    expect((err as ConnectError).code).toBe(Code.ResourceExhausted);
    expect(h.http.unary).not.toHaveBeenCalled();
  });

  it("policy_should_UseHttpOnNextCall_When_ForceHttpOverrideSetMidSession", async () => {
    let force = false;
    const h = setup({ forceHttp: () => force });
    const call = vcs(h.policy) as unknown as Promise<UnaryResponse<never, never>>;
    await openAndAnswer(h, call);
    force = true;
    await vcs(h.policy);
    expect(h.http.unary).toHaveBeenCalledTimes(1);
    expect(h.reasons).toEqual(["force-http"]);
  });

  it("policy_should_RedirectOnceParkWsAndNotCountBreaker_When_ErrorUnauthenticatedArrivesOnOpenSocket", async () => {
    const h = setup();
    const call = vcs(h.policy);
    h.sock().open();
    await Promise.resolve();
    h.sock().receive({
      type: "ERROR",
      id: h.sock().requestIds()[0],
      json: { code: "unauthenticated", message: "expired" },
    } as MuxFrame);
    const err = await call.catch((e) => e);
    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.Unauthenticated);
    expect(h.breaker.failures).toBe(0);
    expect(h.sock().closed).toBe(true);

    await vcs(h.policy);
    expect(h.http.unary).toHaveBeenCalledTimes(1);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });
});

describe("fallback reasons vs the server allow-list", () => {
  it("every FallbackReason is accepted by server/analytics/client_metrics.go", () => {
    const file = path.join(process.cwd(), "..", "server", "analytics", "client_metrics.go");
    const src = fs.readFileSync(file, "utf8");
    const block = /fallbacks\s*=\s*map\[string\]bool\{([\s\S]*?)\}/.exec(src)?.[1] ?? "";
    const allowed = new Set([...block.matchAll(/"([a-z-]+)":\s*true/g)].map((m) => m[1]));
    // Add the reason to the Go `fallbacks` map when this fails, or the server labels it "unknown".
    expect(FALLBACK_REASONS.filter((r) => !allowed.has(r))).toEqual([]);
  });
});

describe("routine HTTP routing counts", () => {
  it("recordFallback_should_AggregateRoutineReasonsIntoOneEventPerFlush", async () => {
    const track = jest.fn();
    const rpcTiming = await import("@/lib/telemetry/rpcTiming");
    const spy = jest.spyOn(rpcTiming, "trackSharedEvent").mockImplementation(track);
    const { createWsUnaryPolicyTransport: make, _flushRoutineFallbacksForTesting: flush } = await import("./wsUnaryPolicy");
    const h = setup();
    const policy = make({ http: h.http as unknown as Transport, getWs: () => h.ws, breaker: h.breaker, readFlag: () => true });
    for (let i = 0; i < 25; i++) await policy.unary(listSessions, undefined, undefined, undefined, {});
    expect(track).not.toHaveBeenCalled();
    flush();
    expect(track).toHaveBeenCalledTimes(1);
    expect(track.mock.calls[0][0].labels).toEqual({ reason: "not-eligible", count: "25" });
    spy.mockRestore();
  });
});
