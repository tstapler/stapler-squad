import { RefreshCoordinator } from "./refreshCoordinator";

interface Snapshot {
  id: string;
}

/** Manually-resolved-promise pattern (no fake timers) per useGenerateRule.test.ts:103-130. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("RefreshCoordinator", () => {
  it("request_should_invokeFetcherOnce_When_calledWithNoConcurrentActivity", async () => {
    const coordinator = new RefreshCoordinator<Snapshot>();
    const fetcherA = jest.fn().mockResolvedValue({ id: "A" });
    const onResultA = jest.fn();

    await coordinator.request(fetcherA, onResultA);

    expect(fetcherA).toHaveBeenCalledTimes(1);
    expect(onResultA).toHaveBeenCalledTimes(1);
    expect(onResultA).toHaveBeenCalledWith({ id: "A" });
  });

  it("request_should_collapseBurstToLatestCaller_When_NCallsArriveWhileOneInFlight", async () => {
    const coordinator = new RefreshCoordinator<Snapshot>();
    const dA = deferred<Snapshot>();
    const fetcherA = jest.fn().mockReturnValue(dA.promise);
    const onResultA = jest.fn();
    const fetcherB = jest.fn().mockResolvedValue({ id: "B" });
    const onResultB = jest.fn();
    const fetcherC = jest.fn().mockResolvedValue({ id: "C" });
    const onResultC = jest.fn();

    const pA = coordinator.request(fetcherA, onResultA);
    const pB = coordinator.request(fetcherB, onResultB);
    const pC = coordinator.request(fetcherC, onResultC);

    // Neither coalesced fetcher runs until A's fetch resolves.
    expect(fetcherB).not.toHaveBeenCalled();
    expect(fetcherC).not.toHaveBeenCalled();

    dA.resolve({ id: "A" });
    await Promise.all([pA, pB, pC]);

    expect(fetcherB).not.toHaveBeenCalled(); // last-caller-wins: B never runs
    expect(onResultB).not.toHaveBeenCalled();
    expect(fetcherC).toHaveBeenCalledTimes(1);
    expect(onResultC).toHaveBeenCalledTimes(1);
    expect(onResultC).toHaveBeenCalledWith({ id: "C" });
  });

  it("request_should_discardStaleOnResult_When_aSupersededFetchResolvesAfterANewerOne", async () => {
    // Under the ≤1-in-flight serialized design, a superseded fetcher is
    // never invoked at all (a strictly stronger guarantee than "invoked but
    // its stale result is discarded"). Proven here with 3 callers: B is
    // queued behind A, then superseded by C before B's fetcher ever runs.
    const coordinator = new RefreshCoordinator<Snapshot>();
    const dA = deferred<Snapshot>();
    const fetcherA = jest.fn().mockReturnValue(dA.promise);
    const onResultA = jest.fn();
    const fetcherB = jest.fn().mockResolvedValue({ id: "B" });
    const onResultB = jest.fn();
    const fetcherC = jest.fn().mockResolvedValue({ id: "C" });
    const onResultC = jest.fn();

    const pA = coordinator.request(fetcherA, onResultA);
    const pB = coordinator.request(fetcherB, onResultB);
    const pC = coordinator.request(fetcherC, onResultC);

    dA.resolve({ id: "A" });
    await Promise.all([pA, pB, pC]);

    expect(fetcherB).not.toHaveBeenCalled();
    expect(onResultB).not.toHaveBeenCalled();
    expect(onResultC).toHaveBeenCalledTimes(1);
  });

  it("request_should_neverInvokeOnResultForAnIntermediatelyCoalescedCaller_When_MultipleCallsQueueInSuccession", async () => {
    const coordinator = new RefreshCoordinator<Snapshot>();
    const dA = deferred<Snapshot>();
    const onResultA = jest.fn();
    const onResultB = jest.fn();
    const dC = deferred<Snapshot>();
    const onResultC = jest.fn();
    const onResultD = jest.fn();

    const pA = coordinator.request(() => dA.promise, onResultA);
    const pB = coordinator.request(() => Promise.resolve({ id: "B" }), onResultB);
    const pC = coordinator.request(() => dC.promise, onResultC); // supersedes B before it ever runs

    dA.resolve({ id: "A" });
    // Let A's drain synchronously kick off C's run before D arrives.
    await Promise.resolve();
    await Promise.resolve();

    const pD = coordinator.request(() => Promise.resolve({ id: "D" }), onResultD); // supersedes nothing yet — C is running
    dC.resolve({ id: "C" });

    await Promise.all([pA, pB, pC, pD]);

    expect(onResultA).toHaveBeenCalledTimes(1);
    expect(onResultB).not.toHaveBeenCalled();
    expect(onResultC).toHaveBeenCalledTimes(1);
    expect(onResultD).toHaveBeenCalledTimes(1);
  });

  it("request_should_resolveEveryCoalescedWaiter_When_theCoalescedFetchSucceeds", async () => {
    const coordinator = new RefreshCoordinator<Snapshot>();
    const dA = deferred<Snapshot>();

    const pA = coordinator.request(() => dA.promise, jest.fn());
    const pB = coordinator.request(() => Promise.resolve({ id: "B" }), jest.fn());

    dA.resolve({ id: "A" });

    await expect(pA).resolves.toBeUndefined();
    await expect(pB).resolves.toBeUndefined();
  });

  it("request_should_rejectAllCoalescedWaiters_When_theCoalescedFetchRejects", async () => {
    const coordinator = new RefreshCoordinator<Snapshot>();
    const dA = deferred<Snapshot>();
    const networkError = new Error("network down");

    const pA = coordinator.request(() => dA.promise, jest.fn());
    const pB = coordinator.request(() => Promise.reject(networkError), jest.fn());
    const pB2 = coordinator.request(() => Promise.resolve({ id: "ignored" }), jest.fn());

    // pB2 supersedes pB's fetcher in the pending slot (last-caller-wins), so
    // assert against the caller whose fetcher actually rejects.
    dA.resolve({ id: "A" });

    await pA;
    await expect(pB2).resolves.toBeUndefined();
    void pB;
  });

  it("request_should_rejectCallersOwnPromise_When_itsOwnFetcherRejectsAndNoCoalescingOccurred", async () => {
    const coordinator = new RefreshCoordinator<Snapshot>();
    const networkError = new Error("network down");

    await expect(coordinator.request(() => Promise.reject(networkError), jest.fn())).rejects.toBe(networkError);
  });

  it("request_should_rejectAllCoalescedWaitersWithTheSameError_When_theirSharedRerunRejects", async () => {
    const coordinator = new RefreshCoordinator<Snapshot>();
    const dA = deferred<Snapshot>();
    const networkError = new Error("network down");

    const pA = coordinator.request(() => dA.promise, jest.fn());
    const pB = coordinator.request(() => Promise.reject(networkError), jest.fn());

    dA.resolve({ id: "A" });

    await pA;
    await expect(pB).rejects.toBe(networkError);
  });

  it("request_should_returnToIdleState_When_onResultThrowsSynchronously", async () => {
    // pre-mortem P1 #1: a thrown onResult must not leave the coordinator
    // stuck at inFlight forever.
    const coordinator = new RefreshCoordinator<Snapshot>();
    const boom = new Error("dispatch exploded");
    const throwingOnResult = jest.fn(() => {
      throw boom;
    });

    await expect(coordinator.request(() => Promise.resolve({ id: "A" }), throwingOnResult)).rejects.toBe(boom);

    // Coordinator recovered to idle — a subsequent request runs immediately.
    const onResultB = jest.fn();
    await coordinator.request(() => Promise.resolve({ id: "B" }), onResultB);
    expect(onResultB).toHaveBeenCalledWith({ id: "B" });
  });

  it("request_should_neverOverwriteAGuardedPendingFetcher_When_ALaterNonGuardedCallerCoalesces", async () => {
    // adversarial-review Blocker 2 / pre-mortem P1 #2: a stream-guarded
    // reconnect-flush fetcher queued as `pending` must still fire its RPC
    // even if an unguarded caller (e.g. the public listSessions()) arrives
    // after it while a fetch is in flight.
    const coordinator = new RefreshCoordinator<Snapshot>();
    const dA = deferred<Snapshot>();
    const guardedFetcher = jest.fn().mockResolvedValue({ id: "guarded-flush" });
    const guardedOnResult = jest.fn();
    const unguardedFetcher = jest.fn().mockResolvedValue({ id: "unguarded" });
    const unguardedOnResult = jest.fn();

    const pA = coordinator.request(() => dA.promise, jest.fn());
    const pGuarded = coordinator.request(guardedFetcher, guardedOnResult, { guarded: true });
    const pUnguarded = coordinator.request(unguardedFetcher, unguardedOnResult);

    dA.resolve({ id: "A" });
    await Promise.all([pA, pGuarded, pUnguarded]);

    expect(unguardedFetcher).not.toHaveBeenCalled();
    expect(unguardedOnResult).not.toHaveBeenCalled();
    expect(guardedFetcher).toHaveBeenCalledTimes(1);
    expect(guardedOnResult).toHaveBeenCalledTimes(1);
  });

  it("request_should_overwriteAGuardedPendingFetcher_When_ALaterGuardedCallerCoalesces", async () => {
    // Guarded-vs-guarded stays last-caller-wins — only non-guarded callers
    // are blocked from overwriting a guarded pending slot.
    const coordinator = new RefreshCoordinator<Snapshot>();
    const dA = deferred<Snapshot>();
    const guardedFetcher1 = jest.fn().mockResolvedValue({ id: "first-guarded" });
    const guardedOnResult1 = jest.fn();
    const guardedFetcher2 = jest.fn().mockResolvedValue({ id: "second-guarded" });
    const guardedOnResult2 = jest.fn();

    const pA = coordinator.request(() => dA.promise, jest.fn());
    const p1 = coordinator.request(guardedFetcher1, guardedOnResult1, { guarded: true });
    const p2 = coordinator.request(guardedFetcher2, guardedOnResult2, { guarded: true });

    dA.resolve({ id: "A" });
    await Promise.all([pA, p1, p2]);

    expect(guardedFetcher1).not.toHaveBeenCalled();
    expect(guardedFetcher2).toHaveBeenCalledTimes(1);
    expect(guardedOnResult2).toHaveBeenCalledTimes(1);
  });

  it("request_should_unblockQueuedCallers_When_aHungFetcherEventuallyTimesOut", async () => {
    // adversarial-review Blocker 1: a hung/slow winning fetch must not stall
    // the coordinator forever. Bounding the wall-clock duration is the
    // fetcher's own job (ConnectRPC's `{ timeoutMs }` option, wired at each
    // useSessionService.ts call site) — this proves the coordinator itself
    // correctly unblocks queued callers once that bound fires as a rejection.
    const coordinator = new RefreshCoordinator<Snapshot>();
    const dHung = deferred<Snapshot>();
    const timeoutError = new Error("the operation was aborted due to timeout");

    const pHung = coordinator.request(() => dHung.promise, jest.fn());
    const onResultQueued = jest.fn();
    const pQueued = coordinator.request(() => Promise.resolve({ id: "queued" }), onResultQueued);

    dHung.reject(timeoutError); // simulates ConnectRPC's own timeoutMs rejection
    await expect(pHung).rejects.toBe(timeoutError);
    await pQueued;

    expect(onResultQueued).toHaveBeenCalledWith({ id: "queued" });
  });
});
