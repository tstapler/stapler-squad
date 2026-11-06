import { createFilesEmitter } from "../filesAnalytics";
import { bucketPayload } from "../events";
import { createFakeAnalytics } from "../../__tests__/fixtures/fakeAnalytics";
import { createFakeTransport, fakeCapabilities, fakeClock, labelProblems } from "../../__tests__/fixtures";
import { TransportFailure } from "../../transport/fileTransport";
import { FileTransportRouter } from "../../transport/router";

const REF = { sessionId: "s1", path: "a.go" };

function setup() {
  const sink = createFakeAnalytics();
  const clock = fakeClock();
  const analytics = createFilesEmitter(sink.track, clock.clock);
  const files = { "a.go": "x" };
  const ops = ["stat", "read_all", "write_full"] as const;
  const full = createFakeTransport({ id: "full", ops, files });
  const unary = createFakeTransport({ id: "connect_unary", ops, files });
  const router = new FileTransportRouter({ full, adapters: [unary], analytics, fetchCapabilities: async () => fakeCapabilities({ ops: [...ops], wires: ["connect_unary"] }) });
  const named = (name: string) => sink.events.filter((e) => e.name === name);
  return { sink, clock, analytics, unary, router, named };
}

describe("transport analytics", () => {
  it("transportEvents_should_EmitSelectOncePerOpChange_And_FallbackOnce", async () => {
    const t = setup();
    await t.router.read(REF, { kind: "all" });
    await t.router.read(REF, { kind: "all" });
    await t.router.stat(REF);
    expect(t.named("files.transport.select").map((e) => e.labels?.op).sort()).toEqual(["read", "stat", "write_full"]);
    expect(t.named("files.transport.select").every((e) => e.labels?.transport === "connect_unary" && e.labels?.reason === "capabilities")).toBe(true);

    t.unary.failures.read = [1, 2, 3].map(() => new TransportFailure("unavailable"));
    for (let i = 0; i < 3; i++) await t.router.read(REF, { kind: "all" }).catch(() => undefined);
    await t.router.read(REF, { kind: "all" });

    expect(t.named("files.transport.fallback")).toHaveLength(1);
    expect(t.named("files.transport.fallback")[0].labels).toMatchObject({ op: "read", from: "connect_unary", to: "full", fallback_reason: "unavailable" });
    expect(t.named("files.transport.select").filter((e) => e.labels?.op === "read")).toHaveLength(2);
    for (const e of t.sink.events) expect(labelProblems(e)).toEqual([]);
  });

  it("transportEvents_should_SampleReadWindowWithTheTokenBucket_And_ReportDrops", () => {
    const t = setup();
    for (let i = 0; i < 50; i++) t.analytics.emit("files.read.window", { transport: "connect_unary", payload: bucketPayload(5000), outcome: "ok", cache: "miss" });
    expect(t.named("files.read.window")).toHaveLength(30);
    t.clock.advance(60_000);
    expect(t.named("files.meta.dropped").map((e) => e.labels)).toEqual([expect.objectContaining({ event: "read_window", count: "6-20" })]);
  });
});
