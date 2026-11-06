import type { AnalyticsEvent } from "@/lib/analytics/types";
import {
  FILES_EVENTS, bucketBytes, bucketPayload, bucketQueryLen, extClass, searchOpts, type FilesEventName,
} from "../events";
import { createFilesEmitter, type EmitName } from "../filesAnalytics";
import { CANARIES, canaryLeaks, fakeClock, fixtureLabels, labelProblems } from "../../__tests__/fixtures";
import { createFakeAnalytics } from "../../__tests__/fixtures/fakeAnalytics";

const emittable = (Object.keys(FILES_EVENTS) as FilesEventName[]).filter(
  (n): n is EmitName => !n.startsWith("files.server.") && n !== "files.meta.dropped" && n !== "files.mobile.extra_key_used",
);

function drive(events: (emit: ReturnType<typeof createFilesEmitter>) => void, clock = fakeClock()) {
  const sink = createFakeAnalytics();
  const emitter = createFilesEmitter(sink.track, clock.clock, { editing: true, v2: true });
  events(emitter);
  return { sink, emitter, clock };
}

describe("files analytics privacy", () => {
  it("filesAnalytics_should_EmitOnlyEnumBooleanBucketLabelsOfValidShape_When_EveryEventFixtureEmitted", () => {
    const clock = fakeClock();
    const { sink, emitter } = drive((e) => {
      for (const name of emittable) (e.emit as (n: string, l: object) => void)(name, fixtureLabels(name));
      for (let i = 0; i < 40; i++) e.emit("files.tab.evict_undo");
      e.emitExtraKey("esc");
    }, clock);
    emitter.flush();
    clock.advance(60_000);
    const names = new Set(sink.events.map((e) => e.name));
    expect(names).toContain("files.meta.dropped");
    expect(names).toContain("files.mobile.extra_key_used");
    expect(sink.events.length).toBeGreaterThan(emittable.length - 3);
    for (const event of sink.events) expect([event.name, labelProblems(event)]).toEqual([event.name, []]);
  });

  it("filesAnalytics_should_NeverEmitCanaryPathNameQueryGlobDraft_When_AllFlowsDriven", () => {
    const { sink } = drive((e) => {
      const size = bucketBytes(CANARIES.draft.length);
      e.emit("files.search.run", { query_len: bucketQueryLen(CANARIES.query.length), opts: searchOpts({ regex: false, caseSensitive: false, wholeWord: false, globs: CANARIES.glob.length > 0, ignored: false, large: false }) });
      e.emit("files.tab.open", { kind: "preview", source: "search", ext_class: extClass(CANARIES.path), size });
      e.emit("files.edit.enter", { via: "button", ext_class: extClass(CANARIES.name) });
      e.emit("files.edit.save", { outcome: "saved", via: "shortcut", size });
      e.emit("files.edit.draft_not_stored", { reason: "too_large" });
    });
    expect(sink.events).toHaveLength(5);
    expect(canaryLeaks(sink.events)).toEqual([]);
    expect(sink.events.map((e: AnalyticsEvent) => e.component)).toEqual(Array(5).fill("FilesTab"));
  });

  it("privacy_should_KeepCanaryPathOffsetLineAndTextOutOfTransportEvents_And_RejectNumericOrStringLabels", () => {
    const { sink } = drive((e) => {
      e.emit("files.transport.select", { op: "read", transport: "connect_unary", reason: "capabilities" });
      e.emit("files.transport.fallback", { op: "read", from: "ws_stream", to: "connect_stream", fallback_reason: "network" });
      e.emit("files.read.window", { transport: "connect_unary", payload: bucketPayload(CANARIES.path.length * 1_000_000), outcome: "ok", cache: "miss" });
      e.emit("files.edit.patch_rejected", { patch_reason: "offset_mismatch", tier: "t1", transport: "connect_unary" });
      e.emit("files.edit.preflight", { outcome: "lines_moved" });
    });
    expect(sink.events).toHaveLength(5);
    expect(canaryLeaks(sink.events)).toEqual([]);
    for (const event of sink.events) expect(labelProblems(event)).toEqual([]);
    expect(labelProblems({ name: "files.read.window", category: "performance", labels: { transport: "full", payload: "4096", outcome: "ok", cache: "hit" } })).toContain("payload=4096 is outside its set");
    expect(labelProblems({ name: "files.edit.save", category: "user_action", labels: { outcome: "saved", via: "button", size: CANARIES.path } })).not.toEqual([]);
  });
});
