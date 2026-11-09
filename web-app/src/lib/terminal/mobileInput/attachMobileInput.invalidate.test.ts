import { attachMobileInput, type AttachOptions, type MobileInputController } from "./attachMobileInput";
import { beforeinput, composition, inputEvent, LinkSink, ManualClock, restoreEnv, setAndroidEnv, setValue } from "./domTestKit";
import { NO_MODIFIERS } from "./keymap";
import { DEFAULT_POLICY, resetValue } from "./policy";
import type { ImeAnomaly, InvalidateReason } from "./types";

const RESET = resetValue(DEFAULT_POLICY);
let cleanup: Array<() => void> = [];

function setup() {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const sink = new LinkSink();
  const anomalies: ImeAnomaly[] = [];
  const c = attachMobileInput(host, {
    onData: sink.onData as AttachOptions["onData"],
    getModifiers: () => NO_MODIFIERS,
    consumeModifiers: () => undefined,
    isReadOnly: () => false,
    onAnomaly: (a) => anomalies.push(a),
    clock: new ManualClock(),
  }) as MobileInputController;
  cleanup.push(() => (c.dispose(), host.remove()));
  return { c, ta: c.element, sink, anomalies };
}

const typeHel = (ta: HTMLTextAreaElement) => {
  composition(ta, "compositionstart");
  for (const r of ["h", "he", "hel"]) {
    setValue(ta, RESET + r);
    beforeinput(ta, "insertCompositionText", r);
  }
};

beforeEach(() => {
  setAndroidEnv();
  jest.spyOn(console, "error").mockImplementation(() => undefined);
});
afterEach(() => {
  cleanup.forEach((f) => f());
  cleanup = [];
  restoreEnv();
  jest.restoreAllMocks();
});

describe("controller.invalidate", () => {
  it.each(["session", "pool_reuse", "visibility", "queue_discard"] as InvalidateReason[])(
    "invalidate_should_EmitZeroBytesAndClearTextarea_When_%sInvalidatesMidComposition",
    (reason) => {
      const { c, ta, sink } = setup();
      typeHel(ta);
      expect(sink.bytes).toBe("hel");
      c.invalidate(reason);
      setValue(ta, `${RESET}yo`);
      beforeinput(ta, "insertCompositionText", "yo"); // hel -> yo would be \x7f\x7f\x7fyo into the new target
      expect(ta.value).toBe(`${RESET}yo`); // never reset inside a live composition
      composition(ta, "compositionend", "yo");
      expect(sink.bytes).toBe("hel");
      expect(ta.value).toBe(RESET);
      beforeinput(ta, "insertText", "x");
      expect(sink.bytes).toBe("helx");
      expect(sink.handed.some((u) => u.includes("\x7f"))).toBe(false);
    },
  );

  it("invalidate_should_ApplyInsertOnlyTail_When_ReasonIsTransport", () => {
    const { c, ta, sink } = setup();
    typeHel(ta);
    c.invalidate("transport");
    beforeinput(ta, "insertCompositionText", "help");
    expect(sink.bytes).toBe("hel");
    composition(ta, "compositionend", "help");
    expect(sink.bytes).toBe("help");
    expect(sink.handed.some((u) => u.includes("\x7f"))).toBe(false);
  });

  it("invalidate_should_EmitZeroBytes_When_TransportThenAutocorrectToUnrelatedWord", () => {
    const { c, ta, sink, anomalies } = setup();
    typeHel(ta);
    c.invalidate("transport");
    beforeinput(ta, "insertCompositionText", "yo");
    composition(ta, "compositionend", "yo");
    expect(sink.bytes).toBe("hel");
    expect(anomalies).toEqual(["unrecognized_sequence"]);
  });

  it("invalidate_should_BeNoOpForTextAndStillBumpEpoch_When_Idle", () => {
    const { c, ta, sink } = setup();
    c.invalidate("session");
    beforeinput(ta, "insertText", "a");
    expect(sink.bytes).toBe("a");
  });

  it("invalidate_should_KeepSessionTwoTextareaEmpty_When_SessionIdChangesMidComposition", () => {
    const { c, ta, sink } = setup();
    typeHel(ta);
    c.invalidate("session"); // what the hook calls on a sessionId change
    setValue(ta, `${RESET}yo`);
    beforeinput(ta, "insertCompositionText", "yo");
    expect(sink.bytes).toBe("hel");
    composition(ta, "compositionend", "yo");
    expect(ta.value).toBe(RESET);
  });
});

describe("stale region stays silent", () => {
  it("staleRegion_should_EmitNothing_When_TrailingCompositionEndOrphanInputAndBackspaceArrive", () => {
    const { c, ta, sink, anomalies } = setup();
    typeHel(ta);
    c.invalidate("session");
    // Gboard-style stragglers before the reset.
    beforeinput(ta, "deleteContentBackward", null);
    inputEvent(ta, `${RESET}he`);
    expect(sink.bytes).toBe("hel");
    expect(ta.value).not.toBe(RESET); // reset deferred until the composition ends
    composition(ta, "compositionend", "hel");
    expect(sink.bytes).toBe("hel");
    expect(ta.value).toBe(RESET);
    expect(anomalies.filter((a) => a === "orphaned_input")).toEqual([]);
  });

  it("staleRegion_should_StillEmitOneDel_When_UserBackspaceInIdleAfterReset", () => {
    const { c, ta, sink } = setup();
    typeHel(ta);
    c.invalidate("session");
    composition(ta, "compositionend", "hel");
    beforeinput(ta, "deleteContentBackward", null);
    expect(sink.bytes).toBe("hel\x7f");
  });

  it("staleRegion_should_EmitNothing_When_EmptyRegionBackspaceArrivesWhileAbandoned", () => {
    const { c, ta, sink } = setup();
    composition(ta, "compositionstart");
    c.invalidate("session");
    beforeinput(ta, "deleteContentBackward", null);
    expect(sink.bytes).toBe("");
  });
});

describe("drop rule through the adapter", () => {
  it("drop_should_LeavePromptUntouchedAndLineThe_When_TehTypedWhileDisconnectedThenAutocorrected", () => {
    const { c, ta, sink } = setup();
    sink.linkUp = false;
    composition(ta, "compositionstart");
    for (const r of ["t", "te", "teh"]) beforeinput(ta, "insertCompositionText", r);
    sink.linkUp = true;
    c.invalidate("transport");
    beforeinput(ta, "insertReplacementText", "the");
    composition(ta, "compositionend", "the");
    expect(sink.handed).toEqual(["the"]);
    expect(sink.term.text).toBe("the");
    expect(sink.term.overDeletes).toBe(0);
  });
});
