import { DISCONNECT_SPACING_MS, MAX_RESYNCS_IN_FLIGHT, ResyncGate } from "../resyncGate";

function makeGate() {
  const timers: Array<{ fn: () => void; ms: number }> = [];
  let now = 0;
  const gate = new ResyncGate({
    random: () => 0.5,
    setTimer: (fn, ms) => timers.push({ fn, ms }),
    now: () => now,
  });
  const flush = () => timers.splice(0).forEach((t) => t.fn());
  return { gate, timers, flush, advance: (ms: number) => (now += ms) };
}

describe("ResyncGate", () => {
  it("runs at most two of ten simultaneous resyncs and queues the rest", () => {
    const { gate } = makeGate();
    const ran: number[] = [];
    for (let i = 0; i < 10; i++) gate.acquire("foreground", () => ran.push(i));
    expect(ran).toEqual([0, 1]);
    expect(gate.active()).toBe(MAX_RESYNCS_IN_FLIGHT);
    expect(gate.depth()).toBe(8);
  });

  it("starts the next queued resync, after a jitter, only when a slot is released", () => {
    const { gate, timers, flush } = makeGate();
    const ran: number[] = [];
    for (let i = 0; i < 4; i++) gate.acquire("foreground", () => ran.push(i));
    gate.release();
    expect(ran).toEqual([0, 1]); // not yet: the jitter timer has not fired
    expect(timers).toHaveLength(1);
    expect(timers[0].ms).toBeGreaterThan(0);
    flush();
    expect(ran).toEqual([0, 1, 2]);
    expect(gate.active()).toBe(MAX_RESYNCS_IN_FLIGHT);
  });

  it("serves foreground resyncs before background ones", () => {
    const { gate, flush } = makeGate();
    const ran: string[] = [];
    gate.acquire("foreground", () => ran.push("a"));
    gate.acquire("foreground", () => ran.push("b"));
    gate.acquire("background", () => ran.push("bg"));
    gate.acquire("foreground", () => ran.push("fg-late"));
    gate.release();
    flush();
    expect(ran).toEqual(["a", "b", "fg-late"]);
  });

  it("drops a cancelled request and frees its reserved slot", () => {
    const { gate, flush } = makeGate();
    const ran: string[] = [];
    gate.acquire("foreground", () => ran.push("a"));
    gate.acquire("foreground", () => ran.push("b"));
    const c = gate.acquire("foreground", () => ran.push("c"));
    gate.release();
    c.cancel(); // cancelled while its jitter timer is pending
    flush();
    expect(ran).toEqual(["a", "b"]);
    expect(gate.active()).toBe(1);
  });

  it("spaces forced disconnects apart instead of dropping every stream at once", () => {
    const { gate, timers, flush } = makeGate();
    const dropped: number[] = [];
    for (let i = 0; i < 10; i++) gate.scheduleDisconnect(() => dropped.push(i));
    expect(dropped).toEqual([0]);
    expect(timers.map((t) => t.ms)).toEqual(Array.from({ length: 9 }, (_, i) => (i + 1) * DISCONNECT_SPACING_MS));
    flush();
    expect(dropped).toHaveLength(10);
  });

  it("a cancelled delayed disconnect never runs", () => {
    const { gate, flush } = makeGate();
    const dropped: number[] = [];
    gate.scheduleDisconnect(() => dropped.push(0));
    const second = gate.scheduleDisconnect(() => dropped.push(1));
    second.cancel();
    flush();
    expect(dropped).toEqual([0]);
  });
});
