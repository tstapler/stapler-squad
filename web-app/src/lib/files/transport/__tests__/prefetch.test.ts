import { BufferPrefetcher, MAX_IN_FLIGHT_PER_BUFFER, MAX_IN_FLIGHT_PER_TAB, TabBudget } from "../prefetch";

const W = 100;

/** Fetches stay pending until `finish`, so tests see exactly what is in flight. */
function setup(over: { cached?: (first: number) => boolean; tab?: TabBudget; lineCount?: number } = {}) {
  let now = 0;
  const done = new Set<number>();
  const started: Array<{ start: number; signal: AbortSignal; finish: () => void }> = [];
  const make = (tab?: TabBudget) => new BufferPrefetcher({
    windowLines: W,
    now: () => now,
    isCached: (first) => done.has(first) || (over.cached?.(first) ?? false),
    lineCount: () => over.lineCount,
    fetchWindow: (start, signal) => new Promise((resolve) => started.push({ start, signal, finish: () => { done.add(start); resolve(undefined); } })),
  }, tab);
  const starts = () => started.map((s) => s.start);
  return { make, started, starts, advance: (ms: number) => { now += ms; }, p: make(over.tab) };
}

const flush = () => new Promise((r) => setTimeout(r, 0));

describe("BufferPrefetcher", () => {
  it("prefetch_should_KeepOneAheadOneBehind_TwoAtSpeed_AtMostTwoInFlight_And_AbortOnDirectionChange", async () => {
    // Slow reader: the window under the viewport, then one ahead; the cap of 2 holds the one behind back.
    const t = setup();
    t.p.onViewport(301);
    expect(t.starts()).toEqual([301, 401]);
    expect(t.p.inFlight).toBe(MAX_IN_FLIGHT_PER_BUFFER);
    t.started[0].finish();
    await flush();
    expect(t.starts()).toEqual([301, 401, 201]); // the freed slot goes to the window behind

    // Scrolling faster than one window per second: two windows ahead in the scroll direction.
    const fast = setup({ cached: (first) => first === 501 });
    fast.p.onViewport(501);
    fast.advance(100);
    fast.p.onViewport(521); // 20 lines / 100ms = 2 windows per second
    expect(fast.started.filter((x) => !x.signal.aborted).map((x) => x.start)).toEqual([601, 701]); // the window behind was dropped

    // A direction change aborts what is in flight and refills toward the new direction.
    const rev = setup();
    rev.p.onViewport(501);
    rev.advance(1000);
    rev.p.onViewport(521);
    const [ahead] = rev.started;
    rev.advance(1000);
    rev.p.onViewport(481);
    expect(ahead.signal.aborted).toBe(true);
    expect(rev.started.at(-1)?.start).toBeLessThan(481);
    expect(rev.p.inFlight).toBeLessThanOrEqual(MAX_IN_FLIGHT_PER_BUFFER);

    // A jump aborts too; the new neighbourhood is fetched.
    const jump = setup();
    jump.p.onViewport(1);
    const first = jump.started[0];
    jump.advance(500);
    jump.p.onViewport(90_001);
    expect(first.signal.aborted).toBe(true);
    expect(jump.starts()).toContain(90_001);
  });

  it("prefetch_should_CapFourInFlightPerTab_SkipCachedWindows_StopAtEndOfFile_And_NeverRetryAFailure", async () => {
    const tab = new TabBudget();
    const a = setup({ tab });
    const b = setup({ tab });
    a.p.onViewport(1);
    b.p.onViewport(1);
    const c = a.make(tab);
    c.onViewport(1000);
    expect(tab.inFlight).toBeLessThanOrEqual(MAX_IN_FLIGHT_PER_TAB);
    expect(tab.inFlight).toBe(MAX_IN_FLIGHT_PER_TAB);
    expect(c.inFlight).toBe(0); // the tab budget, not the buffer cap, blocked it
    a.started[0].finish();
    await flush();
    expect(tab.inFlight).toBe(MAX_IN_FLIGHT_PER_TAB - 1);
    c.onViewport(1000); // the next viewport event of a starved buffer takes the freed slot
    expect(c.inFlight).toBe(1);
    expect(tab.inFlight).toBe(MAX_IN_FLIGHT_PER_TAB);
    a.p.dispose();
    b.p.dispose();
    c.dispose();
    expect(tab.inFlight).toBe(0);

    const cached = setup({ cached: () => true });
    cached.p.onViewport(1);
    expect(cached.started).toHaveLength(0);

    const end = setup({ lineCount: 150 });
    end.p.onViewport(101);
    expect(end.starts()).toEqual([101, 1]); // window 201 starts past the last line

    const failing = setup();
    failing.p.onViewport(301);
    failing.started.forEach((s) => s.signal.addEventListener("abort", () => undefined));
    const before = failing.started.length;
    const rejecting = new BufferPrefetcher({
      windowLines: W, now: () => 0, isCached: () => false,
      fetchWindow: () => { failing.started.push({ start: 0, signal: new AbortController().signal, finish: () => undefined }); return Promise.reject(new Error("boom")); },
    });
    rejecting.onViewport(1);
    await flush();
    await flush();
    expect(failing.started.length - before).toBeLessThanOrEqual(3); // each target tried once
  });
});
