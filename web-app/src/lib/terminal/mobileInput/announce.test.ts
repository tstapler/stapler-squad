import {
  ANNOUNCE_KIND_REGION,
  COMMIT_ANNOUNCE_CLEAR_MS,
  createAnnouncer,
  createCommitAnnouncer,
  type AnnounceKind,
  type Region,
} from "./announce";
import { createDiagnostics } from "./diagnostics";

function harness() {
  const writes: Array<{ region: Region; text: string }> = [];
  const content: Record<Region, string> = { polite: "", assertive: "" };
  const announcer = createAnnouncer({
    write(region, text) {
      content[region] = text;
      writes.push({ region, text });
    },
  });
  const nonEmpty = (region: Region) => writes.filter((w) => w.region === region && w.text !== "");
  return { announcer, writes, content, nonEmpty };
}

const POLITE: AnnounceKind[] = [
  "chip-disconnected",
  "chip-reconnecting",
  "chip-reconnected",
  "chip-still-disconnected",
  "chip-keys-not-sent",
  "read-only-hint",
  "flag-off-hint",
  "paste-flattened-hint",
  "ime-fallback-notice",
  "compose-offer",
  "deferred-commit",
  "report-copied",
  "committed-text",
];
const ASSERTIVE: AnnounceKind[] = ["discard-overflow", "discard-ttl", "discard-session-change", "sending-stuck"];

describe("announce", () => {
  it.each([
    ...POLITE.map((k) => [k, "polite"] as const),
    ...ASSERTIVE.map((k) => [k, "assertive"] as const),
  ])("announce_should_RouteEachMessageToItsRegionAndWriteOnce_When_StateRerenders (%s -> %s)", (kind, region) => {
    const h = harness();
    const other: Region = region === "polite" ? "assertive" : "polite";
    for (let i = 0; i < 5; i++) h.announcer.announce(kind, `text for ${kind}`, kind);
    expect(h.nonEmpty(region)).toHaveLength(1);
    expect(h.content[region]).toBe(`text for ${kind}`);
    expect(h.writes.filter((w) => w.region === other)).toHaveLength(0);
  });

  it("classification_should_CoverEveryKindExactlyOnce_When_TableIsEnumerated", () => {
    expect(Object.keys(ANNOUNCE_KIND_REGION).sort()).toEqual([...POLITE, ...ASSERTIVE].sort());
  });

  it("announce_should_ClearThenSetAndReplaceOlderMessage_When_NewerKeyArrives", () => {
    const h = harness();
    h.announcer.announce("chip-disconnected", "Disconnected", "disc");
    h.announcer.announce("chip-reconnecting", "Reconnecting", "reco");
    expect(h.content.polite).toBe("Reconnecting");
    expect(h.writes.map((w) => w.text)).toEqual(["Disconnected", "", "Reconnecting"]);
  });

  it("announce_should_NotMirrorAssertiveMessageInPolite_When_DiscardNotice", () => {
    const h = harness();
    h.announcer.announce("discard-overflow", "Typed text was dropped", "ovf");
    expect(h.nonEmpty("assertive")).toHaveLength(1);
    expect(h.writes.filter((w) => w.region === "polite")).toHaveLength(0);
  });

  it("announce_should_AnnounceAgain_When_SameKeyAfterRegionCleared", () => {
    const h = harness();
    h.announcer.announce("chip-disconnected", "Disconnected", "disc");
    h.announcer.clear("polite");
    h.announcer.announce("chip-disconnected", "Disconnected", "disc");
    expect(h.nonEmpty("polite")).toHaveLength(2);
  });
});

describe("commit announcer", () => {
  function commitHarness(readOnly = false) {
    const timers: Array<{ fn: () => void; ms: number }> = [];
    const h = harness();
    const diag = createDiagnostics();
    diag.open();
    const commit = createCommitAnnouncer({
      announcer: h.announcer,
      diagnostics: diag,
      isReadOnly: () => readOnly,
      schedule: (fn, ms) => {
        timers.push({ fn, ms });
        return timers.length;
      },
      cancel: () => {},
    });
    return { ...h, commit, diag, timers };
  }

  it("commit_should_WriteCommittedWordOnceToPoliteRegionAndKeepRingClassOnly_When_HelloCommitted", () => {
    const c = commitHarness();
    c.commit.onCommit("hello ");
    expect(c.nonEmpty("polite")).toHaveLength(1);
    expect(c.content.polite).toBe("hello");
    expect(c.nonEmpty("assertive")).toHaveLength(0);
    expect(JSON.stringify(c.diag.displayEvents())).toContain('"length":5');
    expect(JSON.stringify(c.diag.displayEvents())).not.toContain("hello");
    expect(c.diag.displayEvents()[0]).toMatchObject({ cls: "text", length: 5 });
  });

  it("commit_should_ClearPoliteRegionAfterOneSecond_When_WordAnnounced", () => {
    const c = commitHarness();
    c.commit.onCommit("hello ");
    expect(c.timers).toHaveLength(1);
    expect(c.timers[0].ms).toBe(COMMIT_ANNOUNCE_CLEAR_MS);
    expect(COMMIT_ANNOUNCE_CLEAR_MS).toBe(1000);
    c.timers[0].fn();
    expect(c.content.polite).toBe("");
  });

  it("commit_should_AnnounceTwice_When_SameWordCommittedTwiceInARow", () => {
    const c = commitHarness();
    c.commit.onCommit("no ");
    c.commit.onCommit("no ");
    expect(c.nonEmpty("polite")).toHaveLength(2);
  });

  it("commit_should_AnnounceNothingAndRecordNothingToRegion_When_ReadOnlyOrBlank", () => {
    const ro = commitHarness(true);
    ro.commit.onCommit("hello ");
    expect(ro.writes).toHaveLength(0);
    const blank = commitHarness();
    blank.commit.onCommit("  ");
    expect(blank.writes).toHaveLength(0);
  });
});
