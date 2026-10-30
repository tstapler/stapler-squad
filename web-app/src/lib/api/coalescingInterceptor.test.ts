import { create } from "@bufbuild/protobuf";
import { Code, ConnectError, createContextValues } from "@connectrpc/connect";
import type { UnaryRequest, UnaryResponse } from "@connectrpc/connect";
import { priorityKey } from "./requestPriority";
import {
  SessionService,
  GetFeatureFlagsRequestSchema,
  GetFeatureFlagsResponseSchema,
  GetVCSStatusRequestSchema,
  GetVCSStatusResponseSchema,
  ListFilesRequestSchema,
  ListAliasesRequestSchema,
  ListPendingApprovalsRequestSchema,
  ListSessionsRequestSchema,
  UpdateFeatureFlagRequestSchema,
  WriteFileRequestSchema,
} from "@/gen/session/v1/session_pb";
import { InsightsService, GetInsightsSummaryRequestSchema } from "@/gen/session/v1/insights_pb";
import { createCoalescingInterceptor, resetCoalescingForTest } from "./coalescingInterceptor";
import { setRpcTimingSink } from "@/lib/telemetry/rpcTiming";

type Deferred = { resolve: (r: UnaryResponse) => void; reject: (e: unknown) => void; req: UnaryRequest };

function setup(enabled = () => true) {
  const calls: Deferred[] = [];
  const next = (req: UnaryRequest) =>
    new Promise<UnaryResponse>((resolve, reject) => {
      calls.push({ resolve, reject, req });
    });
  const run = createCoalescingInterceptor({ isEnabled: enabled })(next as never) as (r: UnaryRequest) => Promise<UnaryResponse>;
  return { calls, run };
}

function req(method: UnaryRequest["method"], message: unknown, signal = new AbortController().signal): UnaryRequest {
  return {
    stream: false,
    service: SessionService,
    method,
    message,
    signal,
    header: new Headers(),
    url: "http://x/" + method.name,
    requestMethod: "POST",
    contextValues: createContextValues(),
  } as UnaryRequest;
}

const flagsReq = () => req(SessionService.method.getFeatureFlags, create(GetFeatureFlagsRequestSchema, {}));
const flagsRes = (r: UnaryRequest, name = "a"): UnaryResponse =>
  ({ stream: false, method: r.method, service: r.service, header: new Headers(), trailer: new Headers(),
    message: create(GetFeatureFlagsResponseSchema, { flags: [{ name, enabled: true }] }) }) as unknown as UnaryResponse;
const vcsReq = (id = "s1", signal?: AbortSignal) => req(SessionService.method.getVCSStatus, create(GetVCSStatusRequestSchema, { id }), signal);
const vcsRes = (r: UnaryRequest): UnaryResponse =>
  ({ stream: false, method: r.method, service: r.service, header: new Headers(), trailer: new Headers(),
    message: create(GetVCSStatusResponseSchema, {}) }) as unknown as UnaryResponse;

const withPriority = (r: UnaryRequest, priority: "critical" | "deferred"): UnaryRequest => {
  r.contextValues.set(priorityKey, priority);
  return r;
};

const flagsOf = (r: UnaryResponse) => r.message as unknown as { flags: { name: string }[] };

beforeEach(() => resetCoalescingForTest());

describe("coalescingInterceptor client metrics events", () => {
  afterEach(() => setRpcTimingSink(undefined));

  it("coalesce_should_EmitStartedOnceAndJoinedForEachFollower", async () => {
    const track = jest.fn();
    setRpcTimingSink({ track });
    const { calls, run } = setup();
    const results = Array.from({ length: 3 }, () => run(flagsReq()));
    calls[0].resolve(flagsRes(calls[0].req));
    await Promise.all(results);
    const outcomes = track.mock.calls.filter(([e]) => e.name === "cm.coalesce").map(([e]) => e.labels);
    expect(outcomes).toEqual([
      { method: "GetFeatureFlags", outcome: "started" },
      { method: "GetFeatureFlags", outcome: "joined" },
      { method: "GetFeatureFlags", outcome: "joined" },
    ]);
  });
});

describe("coalescingInterceptor", () => {
  it("coalesce_should_NotMergeCriticalCallerIntoDeferredFlight_When_PrioritiesDiffer", () => {
    const { calls, run } = setup();
    void run(withPriority(flagsReq(), "deferred"));
    void run(withPriority(flagsReq(), "critical"));
    void run(withPriority(flagsReq(), "deferred"));
    expect(calls).toHaveLength(2);
  });

  it("coalesce_should_IssueExactlyOneFetch_When_14IdenticalGetFeatureFlagsStartInSameTick", async () => {
    const { calls, run } = setup();
    const results = Array.from({ length: 14 }, () => run(flagsReq()));
    expect(calls).toHaveLength(1);
    calls[0].resolve(flagsRes(calls[0].req));
    const done = await Promise.all(results);
    expect(done).toHaveLength(14);
    done.forEach((d) => expect(flagsOf(d).flags[0].name).toBe("a"));
  });

  it("coalesce_should_ReturnIndependentClones_When_OneSubscriberMutatesSharedResult", async () => {
    const { calls, run } = setup();
    const [a, b] = [run(flagsReq()), run(flagsReq())];
    calls[0].resolve(flagsRes(calls[0].req));
    const [ra, rb] = await Promise.all([a, b]);
    expect(ra.message).not.toBe(rb.message);
    flagsOf(ra).flags[0].name = "mutated";
    expect(flagsOf(rb).flags[0].name).toBe("a");
  });

  it("coalesce_should_RefetchAndNotCache_When_SharedCallRejectsUnavailable", async () => {
    const { calls, run } = setup();
    const first = run(flagsReq());
    calls[0].reject(new ConnectError("down", Code.Unavailable));
    await expect(first).rejects.toMatchObject({ code: Code.Unavailable });
    const second = run(flagsReq());
    expect(calls).toHaveLength(2);
    calls[1].resolve(flagsRes(calls[1].req));
    await second;
  });

  it("coalesce_should_RejectOnlyAbortingCaller_When_OneOfTwoSubscribersAborts", async () => {
    const { calls, run } = setup();
    const abortA = new AbortController();
    const a = run(vcsReq("s1", abortA.signal));
    const b = run(vcsReq("s1"));
    expect(calls).toHaveLength(1);
    abortA.abort();
    await expect(a).rejects.toMatchObject({ code: Code.Canceled });
    expect(calls[0].req.signal.aborted).toBe(false);
    calls[0].resolve(vcsRes(calls[0].req));
    await expect(b).resolves.toBeDefined();
  });

  it("coalesce_should_AbortSharedRequest_When_AllSubscribersAbort", async () => {
    const { calls, run } = setup();
    const c = new AbortController();
    const a = run(vcsReq("s1", c.signal));
    c.abort();
    await expect(a).rejects.toMatchObject({ code: Code.Canceled });
    expect(calls[0].req.signal.aborted).toBe(true);
    run(vcsReq("s1"));
    expect(calls).toHaveLength(2);
  });

  it("coalesce_should_NotJoinPreWriteFlight_When_SessionWriteOrGlobalMutationIssuedAfterFlightStarted", async () => {
    const { calls, run } = setup();
    void run(vcsReq("s1"));
    void run(req(SessionService.method.writeFile, create(WriteFileRequestSchema, { sessionId: "s1", path: "a" })));
    expect(calls).toHaveLength(2);
    void run(vcsReq("s1"));
    expect(calls).toHaveLength(3);
    expect(calls[2].req.method.name).toBe("GetVCSStatus");

    void run(flagsReq());
    void run(req(SessionService.method.updateFeatureFlag, create(UpdateFeatureFlagRequestSchema, { name: "x" })));
    void run(flagsReq());
    expect(calls.filter((c) => c.req.method.name === "GetFeatureFlags")).toHaveLength(2);
  });

  it("coalesce_should_IssueTwoFetches_When_FlagRequestCoalescingOffOrArgsDiffer", () => {
    const off = setup(() => false);
    void off.run(flagsReq());
    void off.run(flagsReq());
    expect(off.calls).toHaveLength(2);

    resetCoalescingForTest();
    const on = setup();
    void on.run(req(SessionService.method.listFiles, create(ListFilesRequestSchema, { sessionId: "s", path: "." })));
    void on.run(req(SessionService.method.listFiles, create(ListFilesRequestSchema, { sessionId: "s", path: "docs" })));
    expect(on.calls).toHaveLength(2);
  });

  it("coalesce_should_NeverCoalesce_When_MethodIsMutationOrUnknown", () => {
    const { calls, run } = setup();
    const mk = () => req(SessionService.method.writeFile, create(WriteFileRequestSchema, { sessionId: "s", path: "a" }));
    void run(mk());
    void run(mk());
    expect(calls).toHaveLength(2);
  });

  describe("load-time duplicate reads (HAR perf-har-2)", () => {
    const insightsReq = (includeOrphans = true) => ({
      ...req(SessionService.method.listAliases, create(ListAliasesRequestSchema, {})),
      service: InsightsService,
      method: InsightsService.method.getInsightsSummary,
      message: create(GetInsightsSummaryRequestSchema, { includeOrphans }),
    }) as UnaryRequest;
    const cases: [string, () => UnaryRequest, () => UnaryRequest][] = [
      ["GetInsightsSummary", () => insightsReq(true), () => insightsReq(false)],
      [
        "ListAliases",
        () => req(SessionService.method.listAliases, create(ListAliasesRequestSchema, {})),
        () => req(SessionService.method.listAliases, create(ListAliasesRequestSchema, {})),
      ],
      [
        "ListSessions",
        () => req(SessionService.method.listSessions, create(ListSessionsRequestSchema, {})),
        () => req(SessionService.method.listSessions, create(ListSessionsRequestSchema, { includeArchived: true })),
      ],
      [
        "ListPendingApprovals",
        () => req(SessionService.method.listPendingApprovals, create(ListPendingApprovalsRequestSchema, {})),
        () => req(SessionService.method.listPendingApprovals, create(ListPendingApprovalsRequestSchema, { sessionId: "s1" })),
      ],
    ];

    it.each(cases)("coalesce_should_Issue1Fetch_When_3Identical%sStartTogether", (name, same) => {
      const { calls, run } = setup();
      void run(same());
      void run(same());
      void run(same());
      expect(calls).toHaveLength(1);
      expect(calls[0].req.method.name).toBe(name);
    });

    // ListAliases has an empty request message: no body can differ.
    it.each(cases.filter(([n]) => n !== "ListAliases"))("coalesce_should_NotMerge_When_%sBodiesDiffer", (_name, same, other) => {
      const { calls, run } = setup();
      void run(same());
      void run(other());
      expect(calls).toHaveLength(2);
    });

    it("coalesce_should_NotJoinPreWriteListPendingApprovals_When_ResolveApprovalRunsBetween", () => {
      const { calls, run } = setup();
      const list = () => req(SessionService.method.listPendingApprovals, create(ListPendingApprovalsRequestSchema, {}));
      void run(list());
      void run(req(SessionService.method.resolveApproval, {} as never));
      void run(list());
      expect(calls.filter((c) => c.req.method.name === "ListPendingApprovals")).toHaveLength(2);
    });
  });
});
