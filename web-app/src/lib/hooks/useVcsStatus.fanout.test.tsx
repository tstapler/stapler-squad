/**
 * GetVCSStatus fan-out: a focused caller must not start a duplicate beside a background request, forced
 * refreshes during a flight share one trailing refresh, and background calls are capped on the wire.
 */
const mockGetVCSStatus = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getVCSStatus: mockGetVCSStatus }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

import { fetchVcsEntry, MAX_BACKGROUND_VCS_CALLS, prefetchVcsStatus, resetVcsCacheForTests } from "@/lib/hooks/useVcsStatus";

interface Pending {
  args: { id: string; waitForFresh: boolean };
  opts?: { signal?: AbortSignal };
  resolve: (branch: string) => void;
}

/** Each getVCSStatus call stays pending until the test resolves it; aborted calls reject like fetch would. */
function installPendingTransport(): Pending[] {
  const pending: Pending[] = [];
  mockGetVCSStatus.mockImplementation(
    (args: Pending["args"], opts?: Pending["opts"]) =>
      new Promise((resolve, reject) => {
        pending.push({ args, opts, resolve: (branch) => resolve({ vcsStatus: { branch }, error: "" }) });
        opts?.signal?.addEventListener("abort", () => reject(new Error("aborted")));
      }),
  );
  return pending;
}

/** Drains the promise chains (microtasks only; no timers) so queued continuations have run. */
const flush = async () => {
  for (let i = 0; i < 20; i++) await Promise.resolve();
};

describe("fetchVcsEntry fan-out", () => {
  beforeEach(() => {
    resetVcsCacheForTests();
    mockGetVCSStatus.mockReset();
  });

  it("fetchVcsEntry_should_SupersedeBackgroundFlight_When_FocusedCallerArrives", async () => {
    const pending = installPendingTransport();
    const background = fetchVcsEntry("s1", false, false); // hook effect runs before the parent's prefetch
    await flush();
    const focused = fetchVcsEntry("s1", false, true);
    await flush();

    expect(pending).toHaveLength(2);
    expect(pending[0].opts?.signal?.aborted).toBe(true); // the background request is withdrawn, not left running
    expect(pending[1].args.waitForFresh).toBe(false); // superseding must not turn a plain poll into a fresh-only request
    pending[1].resolve("main");
    await expect(focused).resolves.toMatchObject({ data: { branch: "main" } });
    await expect(background).resolves.toMatchObject({ data: { branch: "main" } }); // its callers get the focused answer
  });

  it("fetchVcsEntry_should_RunOneTrailingRefresh_When_ManyFreshRequestsDuringOneFlight", async () => {
    const pending = installPendingTransport();
    const first = fetchVcsEntry("s2", false, true);
    await flush();
    const refreshes = [
      fetchVcsEntry("s2", true, true, true),
      fetchVcsEntry("s2", true, true, true),
      fetchVcsEntry("s2", true, true, true),
    ];
    await flush();
    expect(pending).toHaveLength(1); // nothing new goes out while the flight is running

    pending[0].resolve("old");
    await first;
    await flush();
    expect(pending).toHaveLength(2);
    expect(pending[1].args.waitForFresh).toBe(true);
    pending[1].resolve("new");
    const results = await Promise.all(refreshes);
    expect(results.map((r) => r.data)).toEqual([{ branch: "new" }, { branch: "new" }, { branch: "new" }]);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(2);
  });

  it("fetchVcsEntry_should_JoinFlight_When_ForcedPollHasNoFreshnessRequirement", async () => {
    const pending = installPendingTransport();
    const first = fetchVcsEntry("s3", false, true);
    await flush();
    const poll = fetchVcsEntry("s3", true, true, false);
    pending[0].resolve("main");
    await Promise.all([first, poll]);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(1);
  });

  it("fetchVcsEntry_should_CapBackgroundCallsOnTheWire_When_ManySessionsPollAtOnce", async () => {
    const pending = installPendingTransport();
    const sessions = ["a", "b", "c", "d", "e"];
    const all = sessions.map((id) => fetchVcsEntry(id, true, false));
    await flush();
    expect(pending).toHaveLength(MAX_BACKGROUND_VCS_CALLS);

    const focused = fetchVcsEntry("focus", true, true);
    await flush();
    expect(pending).toHaveLength(MAX_BACKGROUND_VCS_CALLS + 1); // the focused pane never queues behind them

    pending[0].resolve("x");
    await flush();
    expect(pending).toHaveLength(MAX_BACKGROUND_VCS_CALLS + 2); // a freed slot starts the next one
    pending.forEach((p) => p.resolve("x"));
    await flush();
    pending.forEach((p) => p.resolve("x"));
    await flush();
    pending.forEach((p) => p.resolve("x"));
    await Promise.all([...all, focused]);
    expect(mockGetVCSStatus).toHaveBeenCalledTimes(sessions.length + 1);
  });

  it("prefetchVcsStatus_should_RunAsBackground_When_PaneNotFocused", async () => {
    const pending = installPendingTransport();
    const p = prefetchVcsStatus("s9", "", false);
    await flush();
    expect(pending[0].opts?.signal).toBeDefined(); // background calls carry the supersede signal; focused ones do not
    pending[0].resolve("x");
    await p;
  });

  it("fetchVcsEntry_should_CarryWaitForFresh_When_SupersedingBackgroundFlight", async () => {
    const pending = installPendingTransport();
    void fetchVcsEntry("w1", true, false, true); // background event-driven refresh needs fresh data
    await flush();
    void fetchVcsEntry("w1", false, true, false); // focused cold-cache prefetch supersedes it
    await flush();
    expect(pending[1].args.waitForFresh).toBe(true);

    void fetchVcsEntry("w2", true, false, false);
    await flush();
    void fetchVcsEntry("w2", true, true, false); // a focused interval poll must stay a plain poll
    await flush();
    expect(pending[3].args.waitForFresh).toBe(false);
  });
});
