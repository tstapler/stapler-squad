import { loadFixtures } from "./fixtureCorpus";
import { validateFixture, type ReplayFixture } from "./fixtureSchema";
import { FrameSampler, intermediateLines, ModelTerminal, replayFixture, summarize } from "./replayHarness";

const fixtures = loadFixtures();
const plain = fixtures.find((f) => f.id === "gboard-typing-hello") as ReplayFixture;

describe("replay corpus", () => {
  it("corpus_should_HaveAtLeast14SyntheticFixturesEachWithSource_When_DirectoryScanned", () => {
    expect(fixtures.filter((f) => f.source === "synthetic").length).toBeGreaterThanOrEqual(14);
    for (const f of fixtures) {
      expect({ id: f.id, errors: validateFixture(f) }).toEqual({ id: f.id, errors: [] });
    }
    expect(new Set(fixtures.map((f) => f.id)).size).toBe(fixtures.length);
  });

  it.each(fixtures.map((f) => [f.id, f] as const))(
    "replay_should_EmitHardwareEquivalentBytes_When_EachFixtureReplayed: %s",
    (_id, f) => {
      const r = replayFixture(f);
      expect(r.failures).toEqual([]);
      expect(r.bytes).toBe(f.expectedBytes);
      expect(r.finalText).toBe(f.finalText);
    },
  );

  it("replay_should_EmitHardwareEquivalentBytes_When_AutocorrectTehReplayed", () => {
    const f = fixtures.find((x) => x.id === "gboard-autocorrect-teh-the");
    expect(f && replayFixture(f).bytes).toBe("teh\x7f\x7fhe");
    expect(f && replayFixture(f).finalText).toBe("the");
  });

  it("replay_should_FailNamingFixture_When_ExpectedBytesDiffer", () => {
    const f = { ...plain, expectedBytes: plain.expectedBytes + "!" };
    const r = replayFixture(f);
    expect(r.passed).toBe(false);
    expect(r.failures[0]).toContain(f.id);
  });

  it("replay_should_FailNamingFixture_When_AnomalyOrFinalTextDiffer", () => {
    const f = { ...plain, finalText: "nope", expectedAnomalies: ["orphaned_input"] };
    const r = replayFixture(f);
    expect(r.failures).toHaveLength(2);
    expect(r.failures.every((m) => m.startsWith(f.id))).toBe(true);
  });

  it("summary_should_PrintSyntheticAndDeviceCountsSeparately_When_SuiteRuns", () => {
    const line = summarize(fixtures.map((f) => replayFixture(f)));
    const synthetic = fixtures.filter((f) => f.source === "synthetic").length;
    const device = fixtures.filter((f) => f.source === "device").length;
    expect(line).toBe(`synthetic ${synthetic}/${synthetic}, device ${device}/${device}`);
    // Surfaced in the jest run output as the per-source report (ADR-007).
    console.info(line);
  });

  it("summary_should_CountFailuresPerSource_When_DeviceFixtureFails", () => {
    const base = plain;
    const results = [
      replayFixture(base),
      replayFixture({ ...base, id: "dev-ok", source: "device" }),
      replayFixture({ ...base, id: "dev-bad", source: "device", expectedBytes: "zzz" }),
    ];
    expect(summarize(results)).toBe("synthetic 1/1, device 1/2");
  });
});

describe("replay harness boundary steps", () => {
  const base: ReplayFixture = {
    id: "inline",
    source: "synthetic",
    keyboard: "test",
    notes: "inline trace",
    events: [],
    expectedBytes: "",
    finalText: "",
  };
  const bi = (data: string, t: number) =>
    ({ kind: "beforeinput", inputType: "insertCompositionText", data, value: data, selStart: data.length, selEnd: data.length, t }) as const;

  it("harness_should_ApplyLinkDownLinkUpAndSessionSwitchSteps_When_FixtureUsesThem", () => {
    const down = replayFixture({
      ...base,
      events: [{ kind: "compositionstart", t: 0 }, bi("he", 1), { kind: "linkDown" }, bi("hel", 2), { kind: "linkUp" }, { kind: "compositionend", data: "hel", t: 3 }],
      expectedBytes: "hel",
      finalText: "hel",
    });
    // Region went deferred at linkDown: nothing sent while down, insert-only tail "l" after commit.
    expect(down.failures).toEqual([]);
    expect(down.droppedBytes).toBe("");

    const switched = replayFixture({
      ...base,
      events: [{ kind: "compositionstart", t: 0 }, bi("hel", 1), { kind: "sessionSwitch" }, bi("yo", 2), { kind: "compositionend", data: "yo", t: 3 }],
      expectedBytes: "hel",
      finalText: "hel",
    });
    expect(switched.failures).toEqual([]);
  });

  it("harness_should_RecordDroppedBytesAndNotApplyThem_When_LinkIsDownDuringTyping", () => {
    const r = replayFixture({
      ...base,
      events: [
        { kind: "linkDown" },
        { kind: "beforeinput", inputType: "insertText", data: "a", value: "a", selStart: 1, selEnd: 1, t: 1 },
        { kind: "linkUp" },
        { kind: "beforeinput", inputType: "insertText", data: "b", value: "b", selStart: 1, selEnd: 1, t: 2 },
      ],
      expectedBytes: "b",
      finalText: "b",
    });
    expect(r.failures).toEqual([]);
    expect(r.droppedBytes).toBe("a");
  });

  it("harness_should_FlagOverDelete_When_DelRunsPastLineStart", () => {
    const r = replayFixture({
      ...base,
      events: [{ kind: "beforeinput", inputType: "deleteContentBackward", data: null, value: "", selStart: 0, selEnd: 0, t: 1 }],
      expectedBytes: "\x7f",
      finalText: "",
    });
    expect(r.overDeletes).toBe(1);
    expect(r.passed).toBe(false);
  });
});

describe("Story 1.2.4 boundary fixtures", () => {
  const ids = {
    "partial-delivery-then-drop": "help",
    "keystroke-after-reconnect-before-flush": "l",
    "session-switch-mid-composition": "helx",
    "sent-then-close-then-autocorrect": "he",
    "enter-after-mid-region-drop": "ls\r",
  } as const;

  it("fiveBoundaryFixtures_should_HoldFinalTextAndInvariant_When_Replayed", () => {
    for (const [id, finalText] of Object.entries(ids)) {
      const f = fixtures.find((x) => x.id === id);
      expect({ id, found: f !== undefined }).toEqual({ id, found: true });
      const r = replayFixture(f as ReplayFixture);
      expect({ id, failures: r.failures, violations: r.invariantViolations }).toEqual({ id, failures: [], violations: [] });
      expect(r.finalText).toBe(finalText);
    }
  });

  it("harness_should_RecordPerUnitDropAndDeliveredPrefix_When_UnitDroppedMidRegion", () => {
    const r = replayFixture(fixtures.find((x) => x.id === "partial-delivery-then-drop") as ReplayFixture);
    expect(r.droppedBytes).toBe("l");
    expect(r.bytes).toBe("help");
    expect(r.bytes).toContain("lp");
    expect(r.bytes).not.toMatch(/\x7f/);
  });

  it("harness_should_ModelSilentlyLostFrame_When_LoseNextStepPrecedesUnit", () => {
    const r = replayFixture(fixtures.find((x) => x.id === "sent-then-close-then-autocorrect") as ReplayFixture);
    expect(r.bytes).toBe("hel"); // handed to the transport
    expect(r.finalText).toBe("he"); // but the frame never arrived
    expect(r.anomalies).toEqual(["unrecognized_sequence"]);
  });

  it("harness_should_NeverShortenPrompt_When_SessionSwitchOrCloseThenAutocorrect", () => {
    for (const id of ["session-switch-mid-composition", "sent-then-close-then-autocorrect"]) {
      const r = replayFixture(fixtures.find((x) => x.id === id) as ReplayFixture, { prompt: "$ " });
      expect({ id, over: r.overDeletes }).toEqual({ id, over: 0 });
      expect(r.invariantViolations).toEqual([]);
    }
  });
});

describe("Story 1.2.5 flicker sampler", () => {
  const base: ReplayFixture = {
    id: "inline-autocorrect",
    source: "synthetic",
    keyboard: "test",
    notes: "inline trace",
    events: [],
    expectedBytes: "",
    finalText: "",
  };
  const comp = (data: string, t: number, inputType = "insertCompositionText") =>
    ({ kind: "beforeinput", inputType, data, value: data, selStart: data.length, selEnd: data.length, t }) as const;

  const cases: Array<[string, string, string]> = [
    ["teh", "the", "gboard-autocorrect-teh-the"],
    ["helo wrld", "hello world", "inline"],
  ];

  it.each(cases)(
    "rewrite_should_ShowAtMostOneIntermediateFrameAndNeverShortenPrompt_When_AutocorrectSampledPerFrame: %s -> %s",
    (pre, post, id) => {
      const f: ReplayFixture =
        id === "inline"
          ? { ...base, events: [{ kind: "compositionstart", t: 0 }, comp(pre, 16), comp(post, 48, "insertReplacementText"), { kind: "compositionend", data: post, t: 80 }] }
          : (fixtures.find((x) => x.id === id) as ReplayFixture);
      const sampler = new FrameSampler(16);
      const r = replayFixture(f, { prompt: "$ ", sampler });
      expect(r.overDeletes).toBe(0);
      const lines = sampler.lines;
      expect(lines.every((l) => l.startsWith("$ "))).toBe(true);
      expect(lines).toContain(`$ ${pre}`);
      expect(lines.at(-1)).toBe(`$ ${post}`);
      expect(intermediateLines(lines, `$ ${pre}`, `$ ${post}`).length).toBeLessThanOrEqual(1);
    },
  );

  it("sampler_should_FlagMoreThanOneIntermediate_When_RewriteBytesLandOnePerFrame", () => {
    const f = fixtures.find((x) => x.id === "gboard-autocorrect-teh-the") as ReplayFixture;
    const sampler = new FrameSampler(16);
    replayFixture(f, { prompt: "$ ", sampler, framePerByte: true });
    expect(intermediateLines(sampler.lines, "$ teh", "$ the").length).toBeGreaterThan(1);
  });

  it("model_should_CountPromptDeletion_When_DelEatsProtectedPrefix", () => {
    const t = new ModelTerminal("$ ");
    t.apply("\x7f");
    expect(t.overDeletes).toBe(1);
    expect(t.text).toBe("$ ");
  });
});
