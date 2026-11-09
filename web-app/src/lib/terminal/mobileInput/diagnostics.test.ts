import {
  DIAGNOSTICS_DISPLAY_COUNT,
  DIAGNOSTICS_RING_CAPACITY,
  RevealBuffer,
  buildReport,
  classifyInput,
  createDiagnostics,
  reportToSkeleton,
} from "./diagnostics";

const HEADER = { renderer: "xterm", flag: "on", streamState: "connected", keyboardHint: "gboard" };

describe("diagnostics ring buffer", () => {
  it("ring_should_StayEmptyWhenClosedAndCapAt200WhenOpenAndClearOnClose_When_1000EventsArrive", () => {
    const d = createDiagnostics();
    for (let i = 0; i < 1000; i++) d.pushInput("beforeinput", "a", i);
    expect(d.length).toBe(0);
    d.open();
    for (let i = 0; i < 1000; i++) d.pushInput("beforeinput", "a", i);
    expect(d.length).toBe(DIAGNOSTICS_RING_CAPACITY);
    expect(DIAGNOSTICS_RING_CAPACITY).toBe(200);
    d.close();
    expect(d.length).toBe(0);
  });

  it("ring_should_DisplayOnlyTheLast50InOrder_When_MoreArrive", () => {
    const d = createDiagnostics();
    d.open();
    for (let i = 0; i < 120; i++) d.pushInput("input", "a", i);
    const shown = d.displayEvents();
    expect(shown).toHaveLength(DIAGNOSTICS_DISPLAY_COUNT);
    expect(shown[0].t).toBe(70);
    expect(shown[49].t).toBe(119);
  });

  it("ring_should_NotifySubscribersAndStopAfterUnsubscribe_When_EventsPush", () => {
    const d = createDiagnostics();
    d.open();
    const seen = jest.fn();
    const off = d.subscribe(seen);
    d.pushInput("input", "a", 1);
    expect(seen).toHaveBeenCalledTimes(1);
    off();
    d.pushInput("input", "a", 2);
    expect(seen).toHaveBeenCalledTimes(1);
  });
});

describe("classifyInput", () => {
  it.each([
    ["h", "text", 1],
    ["hello", "text", 5],
    ["\u{1F600}", "text", 1],
    ["\x03", "control", 1],
    ["\x7f", "control", 1],
    ["\r", "control", 1],
    ["\x1b[A", "escape", 3],
    ["   ", "space", 3],
    ["", "empty", 0],
  ])("classifyInput_should_Map_%j_To_%s_%i", (data, cls, length) => {
    expect(classifyInput(data)).toEqual({ cls, length });
  });
});

describe("redacted report", () => {
  it("report_should_ContainTextFiveAndNotHello_When_HelloTyped", () => {
    const d = createDiagnostics();
    d.open();
    d.pushInput("beforeinput", "hello", 10);
    const json = JSON.stringify(buildReport(HEADER, d.displayEvents()));
    expect(json).toContain("text(5)");
    expect(json).not.toContain("hello");
  });

  it("report_should_HaveTextOneEntriesAndNoTypedCharacter_When_FiveCharsTyped", () => {
    const d = createDiagnostics();
    d.open();
    for (const [i, ch] of [..."qzxjk"].entries()) d.pushInput("input", ch, i);
    const json = JSON.stringify(buildReport(HEADER, d.displayEvents()));
    expect(json.match(/text\(1\)/g)).toHaveLength(5);
    for (const ch of "qzxjk") expect(json).not.toContain(`"${ch}"`);
  });

  it("copiedReport_should_ContainNoTypedText_When_RevealOn", () => {
    const d = createDiagnostics();
    const reveal = new RevealBuffer();
    d.open();
    reveal.setEnabled(true);
    reveal.append("hello");
    d.pushInput("beforeinput", "hello", 1);
    expect(reveal.read()).toBe("hello");
    const json = JSON.stringify(buildReport(HEADER, d.displayEvents()));
    expect(json).not.toContain("hello");
  });

  it("revealBuffer_should_BeEmptyAfterRevealOffAndOnClose_When_TextWasRevealed", () => {
    const reveal = new RevealBuffer();
    reveal.append("ignored while off");
    expect(reveal.read()).toBe("");
    reveal.setEnabled(true);
    reveal.append("hello");
    reveal.setEnabled(false);
    expect(reveal.read()).toBe("");
    reveal.setEnabled(true);
    reveal.append("again");
    reveal.close();
    expect(reveal.read()).toBe("");
    expect(reveal.enabled).toBe(false);
  });

  it("reportToSkeleton_should_YieldShapeSummaryOnly_When_GivenReport", () => {
    const d = createDiagnostics();
    d.open();
    d.pushInput("beforeinput", "hello", 100);
    d.pushInput("keydown", "\x7f", 250);
    d.pushInput("beforeinput", "ab", 400);
    const skeleton = reportToSkeleton(buildReport(HEADER, d.displayEvents()));
    expect(skeleton).toEqual({
      eventCount: 3,
      byClass: { text: 2, control: 1 },
      totalLength: 8,
      spanMs: 300,
    });
    expect(JSON.stringify(skeleton)).not.toContain("hello");
    expect("events" in skeleton).toBe(false);
  });
});
