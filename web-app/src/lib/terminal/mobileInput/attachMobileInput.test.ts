import { attachMobileInput, type AttachOptions, type MobileInputController } from "./attachMobileInput";
import {
  beforeinput,
  composition,
  dispatchStep,
  inputEvent,
  key,
  LinkSink,
  ManualClock,
  restoreEnv,
  setAndroidEnv,
  setEnv,
  setValue,
} from "./domTestKit";
import { loadFixtures } from "./fixtureCorpus";
import type { ReplayFixture } from "./fixtureSchema";
import { NO_MODIFIERS, type Modifiers } from "./keymap";
import { countingDiagnostic, setInputCounterSink, type InputCounterEvent } from "./inputCounters";
import { DEFAULT_POLICY, resetValue } from "./policy";
import { replayFixture } from "./replayHarness";
import type { ImeAnomaly, MobileInputDiagnostic, SendResult } from "./types";

const fixtures = loadFixtures();
const fixture = (id: string): ReplayFixture => fixtures.find((f) => f.id === id) as ReplayFixture;

interface Rig {
  host: HTMLElement;
  renderer: HTMLElement;
  clock: ManualClock;
  sink: LinkSink;
  anomalies: ImeAnomaly[];
  diagnostics: MobileInputDiagnostic[];
  waiting: Array<number | null>;
  c: MobileInputController;
  ta: HTMLTextAreaElement;
  modifiers: { value: Modifiers; consumed: number };
  readOnly: { value: boolean };
}

function rig(over: Partial<AttachOptions> = {}): Rig {
  const host = document.createElement("div");
  const renderer = document.createElement("div");
  host.appendChild(renderer);
  document.body.appendChild(host);
  const clock = new ManualClock();
  const sink = new LinkSink();
  const anomalies: ImeAnomaly[] = [];
  const diagnostics: MobileInputDiagnostic[] = [];
  const waiting: Array<number | null> = [];
  const modifiers = { value: NO_MODIFIERS, consumed: 0 };
  const readOnly = { value: false };
  const c = attachMobileInput(host, {
    onData: sink.onData as AttachOptions["onData"],
    getModifiers: () => modifiers.value,
    consumeModifiers: () => {
      modifiers.consumed++;
      modifiers.value = NO_MODIFIERS;
    },
    isReadOnly: () => readOnly.value,
    onAnomaly: (a) => anomalies.push(a),
    onDiagnostic: (d) => diagnostics.push(d),
    onDeferredWaiting: (n) => waiting.push(n),
    clock,
    rendererElement: renderer,
    ...over,
  }) as MobileInputController;
  return { host, renderer, clock, sink, anomalies, diagnostics, waiting, c, ta: c.element, modifiers, readOnly };
}

let rigs: Rig[] = [];
const make = (over: Partial<AttachOptions> = {}) => {
  const r = rig(over);
  rigs.push(r);
  return r;
};

beforeEach(() => {
  setAndroidEnv();
  jest.spyOn(console, "error").mockImplementation(() => undefined);
});
afterEach(() => {
  for (const r of rigs) {
    r.c?.dispose();
    r.host.remove();
  }
  rigs = [];
  restoreEnv();
  jest.restoreAllMocks();
});

const opts = (): AttachOptions => ({
  onData: () => "sent",
  getModifiers: () => NO_MODIFIERS,
  consumeModifiers: () => undefined,
  isReadOnly: () => false,
  onAnomaly: () => undefined,
});

describe("attachMobileInput gate", () => {
  const ANDROID = undefined;
  const IPHONE = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Safari/604.1";
  it.each([
    ["desktop fine pointer", { ua: "Mozilla/5.0 (X11; Linux x86_64) Chrome/120", coarse: false, hover: true }],
    ["iPhone UA", { ua: IPHONE, coarse: true, hover: false }],
    ["touch laptop", { ua: ANDROID, coarse: true, hover: true }],
  ])("attach_should_ReturnNullAndAddZeroListenersAndNodes_When_%s", (_n, env) => {
    setEnv(env);
    const host = document.createElement("div");
    const add = jest.spyOn(host, "addEventListener");
    const docAdd = jest.spyOn(document, "addEventListener");
    expect(attachMobileInput(host, opts())).toBeNull();
    expect(add).not.toHaveBeenCalled();
    expect(docAdd).not.toHaveBeenCalled();
    expect(host.childNodes).toHaveLength(0);
  });

  it("attach_should_ReturnNull_When_BeforeInputUnsupported", () => {
    const desc = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "onbeforeinput");
    delete (HTMLElement.prototype as unknown as Record<string, unknown>).onbeforeinput;
    try {
      const host = document.createElement("div");
      expect(attachMobileInput(host, opts())).toBeNull();
      expect(host.childNodes).toHaveLength(0);
    } finally {
      if (desc) Object.defineProperty(HTMLElement.prototype, "onbeforeinput", desc);
    }
  });

  it("attach_should_AddZeroListeners_When_DesktopHardwareKeyboard", () => {
    setEnv({ ua: "Mozilla/5.0 (X11; Linux x86_64) Chrome/120", coarse: false, hover: true });
    const host = document.createElement("div");
    const add = jest.spyOn(host, "addEventListener");
    expect(attachMobileInput(host, opts())).toBeNull();
    expect(add).not.toHaveBeenCalled();
  });

  it("attach_should_MountTextareaAsSiblingOutsideRendererElement_When_Attached", () => {
    const r = make();
    expect(r.ta.parentElement).toBe(r.host);
    expect(r.renderer.contains(r.ta)).toBe(false);
    const rendererSpy = jest.fn();
    r.renderer.addEventListener("beforeinput", rendererSpy);
    beforeinput(r.ta, "insertText", "a");
    expect(rendererSpy).not.toHaveBeenCalled();
  });

  it("attach_should_ReturnNullAndLog_When_ContainerIsInsideRendererElement", () => {
    const err = jest.spyOn(console, "error").mockImplementation(() => undefined);
    const renderer = document.createElement("div");
    const inner = document.createElement("div");
    renderer.appendChild(inner);
    expect(attachMobileInput(inner, { ...opts(), rendererElement: renderer })).toBeNull();
    expect(attachMobileInput(renderer, { ...opts(), rendererElement: renderer })).toBeNull();
    expect(inner.childNodes).toHaveLength(0);
    expect(err).toHaveBeenCalled();
  });
});

describe("attachMobileInput event execution", () => {
  it("attach_should_CallOnDataTehThenAtomicRewrite_When_GboardAutocorrectEventsDispatched", () => {
    const r = make();
    const f = fixture("gboard-autocorrect-teh-the");
    for (const step of f.events) dispatchStep(r.ta, step, r.clock, () => undefined);
    expect(r.sink.handed).toEqual(["t", "e", "h", "\x7f\x7fhe"]);
    expect(r.sink.term.text).toBe("the");
  });

  it.each(fixtures.map((f) => [f.id, f] as const))(
    "domReplay_should_MatchPureHarness_When_FixtureReplayedThroughRealEvents: %s",
    (_id, f) => {
      const r = make({ policy: { ...DEFAULT_POLICY, ...f.policy }, bracketedPasteEnabled: () => f.bracketedPaste === true });
      const expected = replayFixture(f);
      const boundary = (kind: string) => {
        if (kind === "linkDown") {
          r.sink.linkUp = false;
          r.c.invalidate("transport");
        } else if (kind === "linkUp") {
          r.sink.linkUp = true;
          r.c.invalidate("transport");
        } else if (kind === "sessionSwitch") r.c.invalidate("session");
        else if (kind === "dropNext") r.sink.dropNext++;
        else r.sink.loseNext++;
      };
      for (const step of f.events) dispatchStep(r.ta, step, r.clock, boundary as never);
      expect({ bytes: r.sink.bytes, text: r.sink.term.text, anomalies: r.anomalies.filter(Boolean) }).toEqual({
        bytes: expected.bytes,
        text: expected.finalText,
        anomalies: expected.anomalies,
      });
    },
  );

  it("attach_should_SuppressEmissionAndSetInputmodeNone_When_IsReadOnlyTrue", () => {
    const r = make();
    r.readOnly.value = true;
    r.c.syncReadOnly();
    expect(r.ta.getAttribute("inputmode")).toBe("none");
    beforeinput(r.ta, "insertText", "x");
    expect(r.sink.handed).toEqual([]);
    expect(r.sink.dropped).toEqual([]);
    r.readOnly.value = false;
    r.c.syncReadOnly();
    expect(r.ta.hasAttribute("inputmode")).toBe(false);
    beforeinput(r.ta, "insertText", "y");
    expect(r.sink.handed).toEqual(["y"]);
  });

  it("attach_should_PreventDefaultEnterAndSendOneCr_When_PhysicalEnterKeydown", () => {
    const r = make();
    const down = key(r.ta, "keydown", { key: "Enter", keyCode: 13 });
    expect(down.defaultPrevented).toBe(true);
    beforeinput(r.ta, "insertLineBreak", null);
    expect(r.sink.handed).toEqual(["\r"]);
  });

  it("attach_should_NotPreventDefault_When_Keydown229OrCompositionInput", () => {
    const r = make();
    const k229 = key(r.ta, "keydown", { key: "Unidentified", keyCode: 229, isComposing: true });
    expect(k229.defaultPrevented).toBe(false);
    composition(r.ta, "compositionstart");
    const bi = beforeinput(r.ta, "insertCompositionText", "he");
    expect(bi.defaultPrevented).toBe(false);
    const bad = key(r.ta, "keydown", { key: "Backspace", keyCode: 229, isComposing: true });
    expect(bad.defaultPrevented).toBe(false);
  });

  it.each([
    ["Tab", { key: "Tab", keyCode: 9 }, "\t"],
    ["Escape", { key: "Escape", keyCode: 27 }, "\x1b"],
    ["ArrowUp", { key: "ArrowUp", keyCode: 38 }, "\x1b[A"],
    ["Ctrl-C", { key: "c", keyCode: 67, ctrlKey: true }, "\x03"],
  ])("attach_should_PreventDefaultAndMapPhysicalKey_When_%s", (_n, init, bytes) => {
    const r = make();
    const e = key(r.ta, "keydown", init);
    expect(e.defaultPrevented).toBe(true);
    expect(r.sink.handed).toEqual([bytes]);
  });

  it("attach_should_KeepTextareaValue_When_CompositionUpdateDispatched", () => {
    const r = make();
    composition(r.ta, "compositionstart");
    setValue(r.ta, `${resetValue(DEFAULT_POLICY)}hel`);
    composition(r.ta, "compositionupdate", "hel");
    expect(r.ta.value).toBe(`${resetValue(DEFAULT_POLICY)}hel`);
    composition(r.ta, "compositionend", "hel");
    expect(r.ta.value).toBe(resetValue(DEFAULT_POLICY));
  });

  it("attach_should_EmitDeltaOnceAndResetTextarea_When_OrphanedInputWithoutBeforeInput", () => {
    const r = make();
    inputEvent(r.ta, `${resetValue(DEFAULT_POLICY)}/`);
    expect(r.sink.handed).toEqual(["/"]);
    expect(r.anomalies).toEqual(["orphaned_input"]);
    expect(r.ta.value).toBe(resetValue(DEFAULT_POLICY));
  });

  it("attach_should_ApplyArmedCtrlOnceThroughKeymap_When_SingleCharTyped", () => {
    const r = make();
    r.modifiers.value = { ctrl: true, alt: false, shift: false };
    beforeinput(r.ta, "insertText", "c");
    expect(r.sink.handed).toEqual(["\x03"]);
    expect(r.modifiers.consumed).toBe(1);
  });

  it("attach_should_SendCtrlCAndConsume_When_ArmedCtrlAndGboardStartsWordWithCompositionC", () => {
    const r = make();
    r.modifiers.value = { ctrl: true, alt: false, shift: false };
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", "c", `${resetValue(DEFAULT_POLICY)}c`);
    composition(r.ta, "compositionupdate", "c");
    inputEvent(r.ta, `${resetValue(DEFAULT_POLICY)}c`);
    composition(r.ta, "compositionend", "c");
    expect(r.sink.handed).toEqual(["\x03"]);
    expect(r.modifiers.consumed).toBe(1);
    expect(r.ta.value).toBe(resetValue(DEFAULT_POLICY));
    expect(r.anomalies).toEqual([]);
    beforeinput(r.ta, "insertText", " ");
    expect(r.sink.handed).toEqual(["\x03", " "]);
    expect(r.modifiers.consumed).toBe(1);
  });

  it("attach_should_ConsumeArmedCtrlAndNotLeakOntoLaterKey_When_HardwareTabPressed", () => {
    const r = make();
    r.modifiers.value = { ctrl: true, alt: false, shift: false };
    const e = key(r.ta, "keydown", { key: "Tab", keyCode: 9 });
    expect(e.defaultPrevented).toBe(true);
    expect(r.modifiers.consumed).toBe(1);
    beforeinput(r.ta, "insertText", "c");
    expect(r.sink.handed[r.sink.handed.length - 1]).toBe("c");
  });

  it("attach_should_NotConsumeArmedModifier_When_PhysicalKeyIsUnmapped", () => {
    const r = make();
    r.modifiers.value = { ctrl: true, alt: false, shift: false };
    key(r.ta, "keydown", { key: "x", keyCode: 88 });
    expect(r.modifiers.consumed).toBe(0);
  });

  it("attach_should_FlattenMultilinePaste_When_BracketedPasteOff", () => {
    const r = make();
    beforeinput(r.ta, "insertFromPaste", "a\nb");
    expect(r.sink.handed).toEqual(["a b"]);
  });

  it("attach_should_DropStaleCompositionEventsUntilCompositionEnd_When_ArmedModifierFlushedMidComposition", () => {
    const r = make();
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", "ab");
    r.modifiers.value = { ctrl: true, alt: false, shift: false };
    beforeinput(r.ta, "insertCompositionText", "abc"); // flushes: text stays as typed, reducer composition ended
    expect(r.sink.bytes).toBe("abc");
    expect(r.ta.value).toBe(resetValue(DEFAULT_POLICY));
    // The browser composition is still live; its further updates must not re-send the flushed text.
    beforeinput(r.ta, "insertCompositionText", "abcd");
    inputEvent(r.ta, "abcd");
    composition(r.ta, "compositionend", "abcd");
    expect(r.sink.bytes).toBe("abc");
    r.modifiers.value = NO_MODIFIERS;
    beforeinput(r.ta, "insertText", "z");
    expect(r.sink.bytes).toBe("abcz");
  });
});

describe("attachMobileInput robustness", () => {
  it("attach_should_NotThrowAndNotLogTypedData_When_OnDataThrows", () => {
    const err = jest.spyOn(console, "error").mockImplementation(() => undefined);
    const r = make({
      onData: () => {
        throw new Error("boom secret");
      },
    });
    expect(() => beforeinput(r.ta, "insertText", "secret")).not.toThrow();
    expect(r.anomalies).toEqual(["unrecognized_sequence"]);
    const logged = err.mock.calls.flat().map((a) => String(a instanceof Error ? a.message : a));
    expect(logged.length).toBeGreaterThan(0);
    expect(logged.join("|")).not.toContain("secret");
  });

  it("consoleError_should_ExcludeTypedData_When_OnDataThrowsAfterTypingSecret", () => {
    const err = jest.spyOn(console, "error").mockImplementation(() => undefined);
    let calls = 0;
    const r = make({
      onData: () => {
        if (++calls > 2) throw new Error("kaboom");
        return "sent";
      },
    });
    for (const ch of "secret") beforeinput(r.ta, "insertText", ch);
    expect(r.anomalies.length).toBeGreaterThan(0);
    expect(JSON.stringify(r.anomalies)).not.toContain("secret");
    expect(JSON.stringify(r.diagnostics)).not.toContain("secret");
    expect(JSON.stringify(err.mock.calls)).not.toContain("secret");
  });

  it("executor_should_ReportAnomalyKindOnlyOncePerKindOnConsole_When_RepeatedAnomalies", () => {
    const err = jest.spyOn(console, "error").mockImplementation(() => undefined);
    const r = make();
    for (let i = 0; i < 4; i++) beforeinput(r.ta, "formatBold", null);
    expect(r.anomalies).toEqual(["unrecognized_sequence", "unrecognized_sequence", "unrecognized_sequence", "unrecognized_sequence"]);
    expect(err).toHaveBeenCalledTimes(1);
  });

  it("executor_should_ReportDroppedResultsToDiagnostics_When_SinkReturnsDropped", () => {
    const r = make({ onData: () => "dropped" as SendResult });
    beforeinput(r.ta, "insertText", "a");
    expect(r.diagnostics).toContainEqual(expect.objectContaining({ event: "unit", result: "dropped" }));
  });

  it("executor_should_TypeCheckAndObserveResult_When_OnDataReturnsSent", () => {
    const seqs: number[] = [];
    const r = make({
      onData: (_b, seq) => {
        seqs.push(seq);
        return "sent";
      },
    });
    beforeinput(r.ta, "insertText", "a");
    beforeinput(r.ta, "insertText", "b");
    expect(seqs[1]).toBeGreaterThan(seqs[0]);
    expect(r.diagnostics.filter((d) => d.event === "unit")).toHaveLength(2);
  });

  it("attach_should_StopHandlingAndRemoveNode_When_Disposed", () => {
    const r = make();
    r.c.dispose();
    expect(r.host.contains(r.ta)).toBe(false);
    beforeinput(r.ta, "insertText", "a");
    expect(r.sink.handed).toEqual([]);
    expect(r.c.active).toBe(false);
    r.c.dispose();
  });
});

describe("attachMobileInput latency and timers", () => {
  it("executor_should_CallOnDataSynchronouslyInHandler_When_BeforeInputDispatchedWithFakeTimersAndNoRaf", () => {
    const raf = jest.spyOn(window, "requestAnimationFrame");
    const setT = jest.spyOn(window, "setTimeout");
    const order: string[] = [];
    const r = make({
      onData: (() => {
        order.push("onData");
        return "sent";
      }) as AttachOptions["onData"],
    });
    r.ta.addEventListener("beforeinput", () => order.push("later-listener"));
    beforeinput(r.ta, "insertText", "a");
    // onData ran inside the layer's own listener: before later listeners, before any timer or frame.
    expect(order).toEqual(["onData", "later-listener"]);
    expect(raf).not.toHaveBeenCalled();
    expect(setT).not.toHaveBeenCalled();
    expect(r.clock.pending).toBe(0);
  });

  it("executor_should_NotScheduleTimersOrRaf_When_NotComposing", () => {
    const raf = jest.spyOn(window, "requestAnimationFrame");
    const r = make();
    beforeinput(r.ta, "insertText", "a");
    key(r.ta, "keydown", { key: "Enter", keyCode: 13 });
    inputEvent(r.ta, `${resetValue(DEFAULT_POLICY)}/`);
    expect(r.clock.pending).toBe(0);
    expect(raf).not.toHaveBeenCalled();
  });

  it("executor_should_RunWatchdogOnlyWhileComposing_When_CompositionStartsAndEnds", () => {
    const r = make();
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", "he");
    expect(r.clock.pending).toBe(1);
    composition(r.ta, "compositionend", "he");
    expect(r.clock.pending).toBe(0);
  });

  it("executor_should_ForceEndStuckCompositionAndRecordWatchdog_When_NoEventsForTwoSeconds", () => {
    const r = make();
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", "he");
    r.clock.advance(2600);
    expect(r.anomalies).toContain("stuck_composition");
    expect(r.diagnostics).toContainEqual({ event: "watchdog" });
    expect(r.clock.pending).toBe(0);
    expect(r.ta.value).toBe(resetValue(DEFAULT_POLICY));
  });

  it("rewrite_should_CallOnDataOnceBeforeAnyTimerOrRaf_When_InsertReplacementText", () => {
    const raf = jest.spyOn(window, "requestAnimationFrame");
    const r = make();
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", "teh");
    const before = r.sink.handed.length;
    const timersBefore = r.clock.pending;
    beforeinput(r.ta, "insertReplacementText", "the");
    expect(r.sink.handed.slice(before)).toEqual(["\x7f\x7fhe"]);
    expect(r.clock.pending).toBe(timersBefore);
    expect(raf).not.toHaveBeenCalled();
  });
});

describe("attachMobileInput delivery", () => {
  it("sink_should_ReceiveUnitsInOrder_When_StubQueueFlushingAndKeystrokeTypedBeforeFlushCompletes", () => {
    // Stand-in for PR-B's queue: strict FIFO, a live unit enqueues behind a non-empty queue.
    const queue: string[] = ["he"];
    const wire: string[] = [];
    let flushing = true;
    const r = make({
      onData: ((b: string) => {
        if (flushing || queue.length) {
          queue.push(b);
          return "queued";
        }
        wire.push(b);
        return "sent";
      }) as AttachOptions["onData"],
    });
    beforeinput(r.ta, "insertText", "l");
    flushing = false;
    wire.push(...queue.splice(0));
    expect(wire).toEqual(["he", "l"]);
  });

  it("executor_should_KeepRegionLive_When_SinkReturnsQueued", () => {
    const r = make({ onData: () => "queued" });
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", "teh");
    beforeinput(r.ta, "insertReplacementText", "the");
    expect(r.diagnostics.filter((d) => d.event === "unit").length).toBe(2);
  });

  it("queueDiscard_should_AbandonRegionAndEmitNothingFurther_When_ReportedMidComposition", () => {
    let listener: ((rep: { deliveredThroughSeq: null }) => void) | undefined;
    const r = make({ onQueueDiscard: (l) => ((listener = l as typeof listener), () => undefined) });
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", "he");
    listener?.({ deliveredThroughSeq: null });
    beforeinput(r.ta, "insertCompositionText", "help");
    composition(r.ta, "compositionend", "help");
    expect(r.sink.bytes).toBe("he");
    beforeinput(r.ta, "insertText", "x");
    expect(r.sink.bytes).toBe("hex");
  });
});

describe("attachMobileInput deferred indicator", () => {
  const hundred = "ab".repeat(50);

  it("indicator_should_ShowCharactersWaitingAfter300msAndClearOnCommit_When_100GraphemeDictation", () => {
    const r = make();
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", hundred);
    expect(r.sink.handed).toEqual([]);
    r.clock.advance(299);
    expect(r.waiting).toEqual([]);
    r.clock.advance(1);
    expect(r.waiting).toEqual([100]);
    composition(r.ta, "compositionend", hundred);
    expect(r.waiting).toEqual([100, null]);
    expect(r.sink.handed).toEqual([hundred]);
  });

  it("indicator_should_NeverShow_When_CommitCompletesUnder300ms", () => {
    const r = make();
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", hundred);
    r.clock.advance(200);
    composition(r.ta, "compositionend", hundred);
    r.clock.advance(1000);
    expect(r.waiting).toEqual([]);
  });

  it("indicator_should_BeRemovedAndWatchdogRecorded_When_DeferredNeverCommits", () => {
    const r = make();
    composition(r.ta, "compositionstart");
    beforeinput(r.ta, "insertCompositionText", hundred);
    r.clock.advance(300);
    expect(r.waiting).toEqual([100]);
    r.clock.advance(2500);
    expect(r.waiting).toEqual([100, null]);
    expect(r.diagnostics).toContainEqual({ event: "watchdog" });
  });
});

describe("host callbacks", () => {
  it("attach_should_CallOnPasteFlattenedOnce_When_MultilinePasteJoinedWithBracketedPasteOff", () => {
    const flattened = jest.fn();
    const r = make({ onPasteFlattened: flattened });
    beforeinput(r.ta, "insertFromPaste", "a\nb");
    expect(flattened).toHaveBeenCalledTimes(1);
  });

  it("attach_should_NotCallOnPasteFlattened_When_BracketedPasteOn", () => {
    const flattened = jest.fn();
    const r = make({ onPasteFlattened: flattened, bracketedPasteEnabled: () => true });
    beforeinput(r.ta, "insertFromPaste", "a\nb");
    expect(flattened).not.toHaveBeenCalled();
  });

  it("setFocusOwner_should_SetInputmodeNone_When_NotOwnerAndRestoreWhenOwnerAgain", () => {
    const r = make();
    r.c.setFocusOwner(false);
    expect(r.ta.getAttribute("inputmode")).toBe("none");
    r.c.setFocusOwner(true);
    expect(r.ta.hasAttribute("inputmode")).toBe(false);
  });

  it("setFocusOwner_should_KeepInputmodeNone_When_OwnerButReadOnly", () => {
    const r = make();
    r.readOnly.value = true;
    r.c.setFocusOwner(true);
    expect(r.ta.getAttribute("inputmode")).toBe("none");
  });
});

describe("input counters", () => {
  const seen: Array<[InputCounterEvent, string]> = [];
  beforeEach(() => {
    seen.length = 0;
    setInputCounterSink({ increment: (event, renderer) => seen.push([event, renderer]) });
  });
  afterEach(() => setInputCounterSink(null));

  it("executor_should_IncrementDroppedDisconnectedCounter_When_SinkReturnsDropped", () => {
    const r = make({ onDiagnostic: countingDiagnostic("xterm") });
    r.sink.linkUp = false;
    beforeinput(r.ta, "insertText", "x");
    expect(seen).toEqual([["dropped_disconnected", "xterm"]]);
  });

  it("executor_should_IncrementEmittedCounter_When_SinkAcceptsUnit", () => {
    const r = make({ onDiagnostic: countingDiagnostic("ghostty") });
    beforeinput(r.ta, "insertText", "x");
    expect(seen).toEqual([["emitted", "ghostty"]]);
  });

  it("counters_should_BeNoOp_When_NoSinkInstalled", () => {
    setInputCounterSink(null);
    const r = make({ onDiagnostic: countingDiagnostic("xterm") });
    expect(() => beforeinput(r.ta, "insertText", "x")).not.toThrow();
  });
});
