import type { AnalyticsEvent } from "@/lib/analytics/types";
import {
  createTerminalLatencyProbe,
  ECHO_MIN_GAP_MS,
  ECHO_TIMEOUT_MS,
  MAX_TERM_EVENTS_PER_MINUTE,
} from "../terminalLatency";

function setup() {
  let t = 1000;
  const events: AnalyticsEvent[] = [];
  const frames: Array<() => void> = [];
  const probe = createTerminalLatencyProbe({
    track: (e) => events.push(e),
    now: () => t,
    raf: (cb) => frames.push(cb),
    listener: () => "local",
  });
  const runFrame = () => frames.splice(0).forEach((cb) => cb());
  const stages = () => events.map((e) => `${e.labels?.stage}:${e.durationMs}`);
  return { probe, events, advance: (ms: number) => (t += ms), runFrame, stages };
}

describe("terminal latency probe", () => {
  it("reports echo arrival at the output and echo paint two frames later", () => {
    const { probe, advance, runFrame, stages } = setup();
    probe.noteInput();
    advance(30);
    probe.noteOutput();
    expect(stages()).toContain("ttfp_first_data:30");
    expect(stages()).toContain("echo_arrive:30");
    advance(16);
    runFrame(); // first frame
    advance(16);
    runFrame(); // second frame: after the paint
    expect(stages()).toContain("echo_paint:62");
  });

  it("samples one keystroke per gap and ignores a held key", () => {
    const { probe, advance, runFrame, events } = setup();
    probe.noteOutput();
    runFrame();
    runFrame();
    events.length = 0;
    probe.noteInput();
    advance(10);
    probe.noteInput(); // inside the pending probe: not a new sample
    advance(10);
    probe.noteOutput();
    runFrame();
    runFrame();
    expect(events.filter((e) => e.labels?.stage === "echo_arrive")).toHaveLength(1);
    expect(events.find((e) => e.labels?.stage === "echo_arrive")?.durationMs).toBe(20);

    probe.noteInput(); // right after: under the min gap
    advance(ECHO_MIN_GAP_MS - 1);
    probe.noteOutput();
    expect(events.filter((e) => e.labels?.stage === "echo_arrive")).toHaveLength(1);
  });

  it("drops a reply that arrives after the echo timeout", () => {
    const { probe, advance, events } = setup();
    probe.noteOutput();
    events.length = 0;
    probe.noteInput();
    advance(ECHO_TIMEOUT_MS + 1);
    probe.noteOutput();
    expect(events).toHaveLength(0);
  });

  it("reports connect and first paint once per mount", () => {
    const { probe, advance, runFrame, stages } = setup();
    advance(120);
    probe.noteConnected();
    probe.noteConnected();
    advance(80);
    probe.noteOutput();
    probe.noteOutput();
    runFrame();
    advance(16);
    runFrame();
    expect(stages().filter((s) => s.startsWith("ttfp_connect"))).toEqual(["ttfp_connect:120"]);
    expect(stages().filter((s) => s.startsWith("ttfp_first_data"))).toEqual(["ttfp_first_data:200"]);
    expect(stages().filter((s) => s.startsWith("ttfp_first_paint"))).toEqual(["ttfp_first_paint:216"]);
  });

  it("never sends keystroke content or ids and caps events per minute", () => {
    const { probe, advance, events } = setup();
    for (let i = 0; i < MAX_TERM_EVENTS_PER_MINUTE + 20; i++) {
      advance(ECHO_MIN_GAP_MS);
      probe.noteInput();
      advance(5);
      probe.noteOutput();
    }
    expect(events.length).toBeLessThanOrEqual(MAX_TERM_EVENTS_PER_MINUTE);
    for (const e of events) {
      expect(Object.keys(e.labels ?? {}).sort()).toEqual(["listener", "stage"]);
      expect(e.sessionId).toBeUndefined();
    }
  });
});
