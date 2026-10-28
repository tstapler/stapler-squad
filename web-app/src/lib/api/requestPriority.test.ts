import { Code, ConnectError, createContextValues } from "@connectrpc/connect";
import type { UnaryRequest, UnaryResponse } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import {
  classify,
  createPriorityInterceptor,
  DEFERRED_MAX_CONCURRENT,
  deferUnless,
  priorityKey,
  priorityOptions,
} from "./requestPriority";
import { defined } from "@/lib/utils/defined";

type Call = { name: string; resolve: () => void };

function setup(gate: () => Promise<void> = () => Promise.resolve()) {
  const started: Call[] = [];
  const next = (req: UnaryRequest) =>
    new Promise<UnaryResponse>((resolve) => {
      started.push({
        name: req.method.name,
        resolve: () => resolve({} as UnaryResponse),
      });
    });
  const run = createPriorityInterceptor({ gate })(next as never) as (r: UnaryRequest) => Promise<UnaryResponse>;
  return { started, run };
}

function req(name: keyof typeof SessionService.method, opts?: { signal?: AbortSignal; deferred?: boolean }): UnaryRequest {
  const contextValues = createContextValues();
  if (opts?.deferred) contextValues.set(priorityKey, "deferred");
  return {
    stream: false,
    service: SessionService,
    method: SessionService.method[name],
    message: {},
    signal: opts?.signal ?? new AbortController().signal,
    header: new Headers(),
    url: "http://x/" + name,
    requestMethod: "POST",
    contextValues,
  } as unknown as UnaryRequest;
}

const flush = () => new Promise((r) => setTimeout(r, 0));

describe("requestPriority", () => {
  it("priority_should_StartCriticalImmediatelyAndCapDeferredAt3_When_12DeferredAnd1CriticalIssuedInOneTick", async () => {
    const { started, run } = setup();
    const deferred = Array.from({ length: 12 }, () => run(req("listShells")));
    const critical = run(req("getFeatureFlags"));
    await flush();

    expect(started.filter((s) => s.name === "GetFeatureFlags")).toHaveLength(1);
    expect(started.filter((s) => s.name === "ListShells")).toHaveLength(DEFERRED_MAX_CONCURRENT);

    // Draining never exceeds the cap and finishes all 12.
    let drained = 0;
    while (drained < 12) {
      const running = started.filter((s) => s.name === "ListShells").slice(drained);
      expect(running.length).toBeLessThanOrEqual(DEFERRED_MAX_CONCURRENT);
      running[0].resolve();
      drained++;
      await flush();
    }
    defined(started.find((s) => s.name === "GetFeatureFlags")).resolve();
    await Promise.all([...deferred, critical]);
    expect(started).toHaveLength(13);
  });

  it("classify_should_DefaultToCritical_When_MethodNotInTable", () => {
    expect(classify("SomeUnknownMethod")).toBe("critical");
    expect(classify("ListShells")).toBe("deferred");
    expect(classify("GetVCSStatus")).toBe("critical");
    expect(classify("GetVCSStatus", "deferred")).toBe("deferred");
  });

  it("deferUnless_should_TagDeferred_When_PaneNotFocused", () => {
    expect(deferUnless(true)).toBeUndefined();
    expect(deferUnless(false)?.contextValues?.get(priorityKey)).toBe("deferred");
    expect(priorityOptions("deferred").contextValues?.get(priorityKey)).toBe("deferred");
  });

  it("priority_should_HoldDeferredUntilGateOpens_And_NeverHoldCritical", async () => {
    let open!: () => void;
    const gated = new Promise<void>((r) => (open = r));
    const { started, run } = setup(() => gated);
    void run(req("listWorkflows"));
    void run(req("getVCSStatus", { deferred: true }));
    void run(req("getVCSStatus"));
    await flush();
    expect(started.map((s) => s.name)).toEqual(["GetVCSStatus"]);
    open();
    await flush();
    expect(started.map((s) => s.name).sort()).toEqual(["GetVCSStatus", "GetVCSStatus", "ListWorkflows"]);
  });

  it("priority_should_RejectCanceledAndFreeQueueSlot_When_QueuedDeferredCallAborts", async () => {
    const { started, run } = setup();
    for (let i = 0; i < DEFERRED_MAX_CONCURRENT; i++) void run(req("listShells"));
    const ac = new AbortController();
    const queued = run(req("listShells", { signal: ac.signal }));
    await flush();
    expect(started).toHaveLength(DEFERRED_MAX_CONCURRENT);
    ac.abort();
    await expect(queued).rejects.toMatchObject({ code: Code.Canceled });
    started[0].resolve();
    await flush();
    expect(started).toHaveLength(DEFERRED_MAX_CONCURRENT); // the aborted call never starts
    expect(ConnectError.from(new Error("x"))).toBeInstanceOf(ConnectError);
  });
});
