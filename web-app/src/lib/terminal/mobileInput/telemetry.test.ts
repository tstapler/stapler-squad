import { createMobileInputTelemetry, type TelemetryClock } from "./telemetry";
import type { AnalyticsEvent } from "@/lib/analytics/types";
import type { ImeAnomaly } from "./types";

function fakeClock(): TelemetryClock & { advance(ms: number): void } {
  let now = 1_000;
  let nextId = 1;
  const timers = new Map<number, { at: number; fn: () => void }>();
  return {
    now: () => now,
    setTimeout(fn, ms) {
      const id = nextId++;
      timers.set(id, { at: now + ms, fn });
      return id;
    },
    clearTimeout: (id) => void timers.delete(id),
    advance(ms) {
      now += ms;
      for (const [id, t] of [...timers]) {
        if (t.at <= now) {
          timers.delete(id);
          t.fn();
        }
      }
    },
  };
}

function setup(renderer: "xterm" | "ghostty" = "xterm") {
  const clock = fakeClock();
  const events: AnalyticsEvent[] = [];
  const telemetry = createMobileInputTelemetry({ track: (e) => events.push(e), clock, renderer });
  return { clock, events, telemetry };
}

describe("mobile input telemetry", () => {
  it("sender_should_SendAtMostFiveBeacons_When_50OrphanedInputAnomaliesInOneSecond", () => {
    const { events, telemetry, clock } = setup();
    for (let i = 0; i < 50; i++) {
      telemetry.onAnomaly("orphaned_input");
      clock.advance(20);
    }
    expect(events.filter((e) => e.name === "cm.ime_anomaly")).toHaveLength(5);
    expect(telemetry.stats().anomaliesSuppressed).toBe(45);
  });

  it("sender_should_RefillOnePerTenSeconds_When_BurstExhausted", () => {
    const { events, telemetry, clock } = setup();
    for (let i = 0; i < 6; i++) telemetry.onAnomaly("stuck_composition");
    expect(events).toHaveLength(5);
    clock.advance(10_000);
    telemetry.onAnomaly("stuck_composition");
    telemetry.onAnomaly("stuck_composition");
    expect(events).toHaveLength(6);
  });

  it("sender_should_RateLimitPerKindIndependently_When_TwoKindsFlood", () => {
    const { events, telemetry } = setup();
    for (let i = 0; i < 10; i++) {
      telemetry.onAnomaly("orphaned_input");
      telemetry.onAnomaly("unrecognized_sequence");
    }
    const kinds = events.map((e) => e.labels?.kind);
    expect(kinds.filter((k) => k === "orphaned_input")).toHaveLength(5);
    expect(kinds.filter((k) => k === "unrecognized_sequence")).toHaveLength(5);
  });

  it("sender_should_MapDroppedDisconnectedAndUnrecognizedSequenceToCmSeries_When_Reported", () => {
    const { events, telemetry, clock } = setup("ghostty");
    telemetry.onDiagnostic({ event: "unit", seq: 1 as never, epoch: 1 as never, result: "dropped" });
    telemetry.onAnomaly("unrecognized_sequence");
    clock.advance(10_000);
    expect(events).toContainEqual({
      name: "cm.ime_anomaly",
      category: "performance",
      labels: { kind: "unrecognized_sequence", renderer: "ghostty" },
    });
    expect(events).toContainEqual({
      name: "cm.input",
      category: "performance",
      labels: { event: "dropped_disconnected", renderer: "ghostty", count: "1" },
    });
  });

  it("sender_should_AggregateKeystrokesIntoOneBeaconPerWindow_When_TypingFast", () => {
    const { events, telemetry, clock } = setup();
    for (let i = 0; i < 40; i++) {
      telemetry.onDiagnostic({ event: "unit", seq: i as never, epoch: 1 as never, result: i % 10 === 0 ? "queued" : "sent" });
    }
    expect(events).toHaveLength(0);
    clock.advance(10_000);
    const byEvent = Object.fromEntries(events.map((e) => [e.labels?.event, e.labels?.count]));
    expect(byEvent).toEqual({ emitted: "40", queued: "4" });
    clock.advance(60_000);
    expect(events).toHaveLength(2);
  });

  it("sender_should_IgnoreNonUnitDiagnostics_When_InvalidateOrWatchdog", () => {
    const { events, telemetry, clock } = setup();
    telemetry.onDiagnostic({ event: "watchdog" });
    telemetry.onDiagnostic({ event: "invalidate", reason: "transport" });
    telemetry.onDiagnostic({ event: "anomaly", kind: "orphaned_input" });
    clock.advance(10_000);
    expect(events).toHaveLength(0);
  });

  it("flush_should_SendPendingCountsImmediately_When_Called", () => {
    const { events, telemetry } = setup();
    telemetry.onDiagnostic({ event: "unit", seq: 1 as never, epoch: 1 as never, result: "sent" });
    telemetry.flush();
    expect(events).toHaveLength(1);
  });

  it("dispose_should_FlushAndStopTimers_When_Called", () => {
    const { events, telemetry, clock } = setup();
    telemetry.onDiagnostic({ event: "unit", seq: 1 as never, epoch: 1 as never, result: "sent" });
    telemetry.dispose();
    expect(events).toHaveLength(1);
    telemetry.onDiagnostic({ event: "unit", seq: 2 as never, epoch: 1 as never, result: "sent" });
    clock.advance(60_000);
    expect(events).toHaveLength(1);
  });

  it("beacon_should_ContainNoTypedSubstring_When_EmittedTextIsSecret", () => {
    const { events, telemetry, clock } = setup();
    // Diagnostics and anomalies carry no text by type; smuggle one in to prove nothing is copied through.
    telemetry.onDiagnostic({ event: "unit", seq: 1 as never, epoch: 1 as never, result: "sent", text: "secret" } as never);
    (telemetry.onAnomaly as (...a: unknown[]) => void)("orphaned_input", "secret");
    telemetry.onAnomaly("secret" as unknown as ImeAnomaly);
    clock.advance(10_000);
    expect(events.length).toBeGreaterThan(0);
    expect(JSON.stringify(events)).not.toContain("secret");
  });

  it("sender_should_NeverThrow_When_TrackThrows", () => {
    const clock = fakeClock();
    const telemetry = createMobileInputTelemetry({
      track: () => {
        throw new Error("boom");
      },
      clock,
      renderer: "xterm",
    });
    expect(() => {
      telemetry.onAnomaly("orphaned_input");
      telemetry.onDiagnostic({ event: "unit", seq: 1 as never, epoch: 1 as never, result: "sent" });
      clock.advance(10_000);
    }).not.toThrow();
  });
});
