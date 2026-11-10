import { createInputSendTimer, createKeyedInputSendTimers } from "../terminalInputTiming";

describe("input send timer", () => {
  it("reports how long the oldest queued keystroke waited and the socket backlog at send", () => {
    let t = 100;
    const timer = createInputSendTimer(() => t);
    const seen: Array<{ queuedMs: number; bufferedBytes: number }> = [];
    timer.subscribe((s) => seen.push(s));

    timer.markQueued();
    t = 112;
    timer.markQueued();
    t = 130;
    timer.markSent(2048);
    timer.markSent(0);

    expect(seen).toEqual([
      { queuedMs: 30, bufferedBytes: 2048 },
      { queuedMs: 18, bufferedBytes: 0 },
    ]);
  });

  it("ignores a send with nothing queued (a resize or handshake frame)", () => {
    const timer = createInputSendTimer(() => 0);
    const seen: unknown[] = [];
    timer.subscribe((s) => seen.push(s));
    timer.markSent(10);
    expect(seen).toEqual([]);
  });

  it("keeps the queue bounded when sends never happen", () => {
    const timer = createInputSendTimer(() => 0);
    for (let i = 0; i < 10_000; i++) timer.markQueued();
    expect(timer.pending()).toBeLessThanOrEqual(256);
  });

  it("forgets queued messages on reset so later sends do not pair with stale ones", () => {
    let t = 0;
    const timer = createInputSendTimer(() => t);
    const seen: Array<{ queuedMs: number }> = [];
    timer.subscribe((s) => seen.push(s));
    timer.markQueued(); // dropped when the queue closes
    timer.reset();
    t = 5000;
    timer.markQueued();
    t = 5010;
    timer.markSent(0);
    expect(seen).toEqual([{ queuedMs: 10, bufferedBytes: 0 }]);
  });

  it("stops notifying after unsubscribe", () => {
    const timer = createInputSendTimer(() => 0);
    const seen: unknown[] = [];
    const off = timer.subscribe((s) => seen.push(s));
    off();
    timer.markQueued();
    timer.markSent(1);
    expect(seen).toEqual([]);
  });
});

describe("keyed input send timers", () => {
  it("keeps each terminal's queue separate so one reset or send never touches another", () => {
    let t = 0;
    const timers = createKeyedInputSendTimers(() => t);
    const a: number[] = [];
    const b: number[] = [];
    timers.subscribe("a", (s) => a.push(s.queuedMs));
    timers.subscribe("b", (s) => b.push(s.queuedMs));

    timers.markQueued("a");
    t = 5;
    timers.markQueued("b");
    timers.reset("a"); // a's queue closed; b's pending input must survive
    t = 20;
    timers.markSent("b", 0);
    timers.markSent("a", 0); // nothing queued for a any more

    expect(a).toEqual([]);
    expect(b).toEqual([15]);
  });

  it("forgets a key once it has no listeners and nothing queued", () => {
    const timers = createKeyedInputSendTimers(() => 0);
    const off = timers.subscribe("a", () => {});
    off();
    const seen: unknown[] = [];
    timers.subscribe("a", (s) => seen.push(s));
    timers.markSent("a", 1); // fresh entry: no stale queue
    expect(seen).toEqual([]);
  });
});
