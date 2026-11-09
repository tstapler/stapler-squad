import { reduce, demoteToDeferred, abandonRegion } from "./composition";
import { BACKSPACE_SENTINEL, DEFAULT_POLICY, MAX_REGION } from "./policy";
import { anomalies, composing, emitted, ev, idle, run } from "./testHelpers";

const KEYDOWN8_POLICY = { ...DEFAULT_POLICY, backspaceStrategy: "keydown8" } as const;

describe("reduce: Event-Command-Policy rows", () => {
  it("reduce_should_EmitLoAndStayComposing_When_InsertCompositionTextGrowsHelToHello", () => {
    const r = reduce(composing("hel", "hel"), ev.beforeinput("insertCompositionText", "hello"), DEFAULT_POLICY);
    expect(emitted(r.commands)).toEqual(["lo"]);
    expect(r.state).toMatchObject({ kind: "composing", region: "hello", sent: "hello" });
    expect(r.preventDefault).toBe(false);
  });

  it("reduce_should_EmitRewriteAsOneAtomicUnit_When_AutocorrectReplacesTehWithThe", () => {
    const r = reduce(composing("teh", "teh"), ev.beforeinput("insertCompositionText", "the"), DEFAULT_POLICY);
    expect(r.commands.filter((c) => c.kind === "Emit")).toEqual([
      { kind: "Emit", bytes: "\x7f\x7fhe", atomic: true },
    ]);
  });

  it("reduce_should_MarkEmitNonAtomic_When_RewriteExceedsAtomicEditCeiling", () => {
    const big = "é".repeat(MAX_REGION); // 2 bytes each, still under ceiling
    const small = reduce(composing("", ""), ev.beforeinput("insertCompositionText", big), DEFAULT_POLICY);
    expect(small.commands[0]).toMatchObject({ kind: "Emit", atomic: true });
    const policy = { ...DEFAULT_POLICY, atomicEditMaxBytes: 10 };
    const tight = reduce(composing("", ""), ev.beforeinput("insertCompositionText", big), policy);
    expect(tight.commands[0]).toMatchObject({ kind: "Emit", atomic: false });
  });

  it("reduce_should_StartCompositionFromIdle_When_InsertCompositionTextWithoutCompositionStart", () => {
    const r = run(idle(), [ev.beforeinput("insertCompositionText", "hi")]);
    expect(r.bytes).toBe("hi");
    expect(r.state).toMatchObject({ kind: "composing", region: "hi", sent: "hi" });
  });

  it("reduce_should_EnterComposingWithEmptyRegion_When_CompositionStart", () => {
    const r = reduce(idle(), ev.compositionstart(5), DEFAULT_POLICY);
    expect(r.commands).toEqual([]);
    expect(r.state).toMatchObject({ kind: "composing", region: "", sent: "", mode: "live", lastT: 5 });
  });

  it("reduce_should_ApplyRegionFromCompositionUpdate_When_NoBeforeInputArrived", () => {
    const r = run(composing("he", "he"), [ev.compositionupdate("hey")]);
    expect(r.bytes).toBe("y");
  });

  it("reduce_should_NotDoubleEmit_When_CompositionUpdateRepeatsBeforeInputRegion", () => {
    const r = run(composing("he", "he"), [
      ev.beforeinput("insertCompositionText", "hey"),
      ev.compositionupdate("hey"),
    ]);
    expect(r.bytes).toBe("y");
  });

  it("reduce_should_RewriteSentToReplacement_When_InsertReplacementTextInsideComposition", () => {
    const r = run(composing("teh", "teh"), [ev.beforeinput("insertReplacementText", "the")]);
    expect(r.bytes).toBe("\x7f\x7fhe");
  });

  it("reduce_should_EmitNothingWithAnomaly_When_InsertReplacementTextOutsideComposition", () => {
    const r = run(idle(), [ev.beforeinput("insertReplacementText", "the")]);
    expect(r.bytes).toBe("");
    expect(anomalies(r.commands)).toEqual(["unrecognized_sequence"]);
  });

  it("reduce_should_EmitTextAndResetTextarea_When_InsertTextOutsideComposition", () => {
    const r = reduce(idle(), ev.beforeinput("insertText", "x"), DEFAULT_POLICY);
    expect(emitted(r.commands)).toEqual(["x"]);
    expect(r.commands).toContainEqual({ kind: "ResetTextarea" });
  });

  it("reduce_should_FinalizeComposition_When_InsertTextCommitsLiveRegion", () => {
    const r = run(composing("hel", "hel"), [ev.beforeinput("insertText", "hello")]);
    expect(r.bytes).toBe("lo");
    expect(r.state).toMatchObject({ kind: "idle" });
  });

  it("reduce_should_FlushTailAndFlagPaste_When_InsertFromPasteDuringDeferredComposition", () => {
    const r = reduce(composing("ab", ""), ev.beforeinput("insertFromPaste", "XY"), DEFAULT_POLICY);
    expect(r.commands.filter((c) => c.kind === "Emit")).toEqual([
      { kind: "Emit", bytes: "ab", atomic: true },
      { kind: "Emit", bytes: "XY", atomic: true, paste: true },
    ]);
  });

  it("reduce_should_ReconcileSentWithFinalData_When_CompositionEndDiffers", () => {
    const r = run(composing("teh", "teh"), [ev.compositionend("the")]);
    expect(r.bytes).toBe("\x7f\x7fhe");
    expect(r.state).toMatchObject({ kind: "idle" });
    expect(r.commands).toContainEqual({ kind: "ResetTextarea" });
  });

  it("reduce_should_EmitNothing_When_CompositionEndMatchesSent", () => {
    const r = run(composing("ok", "ok"), [ev.compositionend("ok")]);
    expect(r.bytes).toBe("");
  });

  it("reduce_should_IgnoreCompositionEndWithAnomaly_When_IdleWithoutComposition", () => {
    const r = run(idle(), [ev.compositionend("ok")]);
    expect(r.bytes).toBe("");
    expect(anomalies(r.commands)).toEqual(["unrecognized_sequence"]);
  });

  it("reduce_should_FlushOnBlurAndGoIdle_When_ComposingBlurs", () => {
    const r = run(composing("ab", ""), [ev.blur()]);
    expect(r.bytes).toBe("ab");
    expect(r.state).toMatchObject({ kind: "idle" });
  });

  it("reduce_should_EmitNothing_When_BlurWhileIdle", () => {
    expect(run(idle(), [ev.blur()]).commands).toEqual([]);
  });
});

describe("reduce: keydown", () => {
  it("reduce_should_EmitNothingAndNotPreventDefault_When_Keydown229", () => {
    const r = reduce(idle(), ev.keydown("Unidentified", 229), DEFAULT_POLICY);
    expect(r.commands).toEqual([]);
    expect(r.preventDefault).toBe(false);
  });

  it("reduce_should_EmitNothingAndNotPreventDefault_When_KeydownIsComposing", () => {
    const r = reduce(composing("a", "a"), ev.keydown("a", 65, true), DEFAULT_POLICY);
    expect(r.commands).toEqual([]);
    expect(r.preventDefault).toBe(false);
    expect(r.state).toMatchObject({ kind: "composing", region: "a" });
  });

  it("reduce_should_EmitCarriageReturnAndPreventDefault_When_HardwareEnterKeydown", () => {
    const r = reduce(idle(), ev.keydown("Enter", 13), DEFAULT_POLICY);
    expect(emitted(r.commands)).toEqual(["\r"]);
    expect(r.preventDefault).toBe(true);
  });

  it("reduce_should_EmitOneCarriageReturn_When_HardwareEnterKeydownThenBeforeInputLineBreak", () => {
    const r = run(idle(), [ev.keydown("Enter", 13), ev.beforeinput("insertLineBreak", null)]);
    expect(r.bytes).toBe("\r");
  });
});

describe("reduce: Enter wrapped in composition", () => {
  it("reduce_should_EmitExactlyOneCarriageReturn_When_EnterWrappedInComposition (Enter first)", () => {
    const r = run(composing("ls", "ls"), [ev.beforeinput("insertLineBreak", null), ev.compositionend("ls")]);
    expect(r.results.map((x) => emitted(x.commands))).toEqual([["\r"], []]);
    expect(r.state).toMatchObject({ kind: "idle" });
  });

  it("reduce_should_EmitExactlyOneCarriageReturn_When_EnterWrappedInComposition (compositionend first)", () => {
    const r = run(composing("ls", "ls"), [ev.compositionend("ls"), ev.beforeinput("insertLineBreak", null)]);
    expect(r.results.map((x) => emitted(x.commands).join(""))).toEqual(["", "\r"]);
  });

  it("reduce_should_EmitExactlyOneCarriageReturn_When_EnterWrappedInComposition (deferred)", () => {
    const r = run(composing("ls", "", "deferred"), [
      ev.beforeinput("insertLineBreak", null),
      ev.compositionend("ls"),
    ]);
    expect(r.results.map((x) => emitted(x.commands))).toEqual([["ls", "\r"], []]);
  });

  it("reduce_should_EmitLsThenCarriageReturnOnce_When_EnterAndCompositionEndInEitherOrder", () => {
    const a = run(composing("ls", "ls"), [ev.beforeinput("insertLineBreak", null), ev.compositionend("ls")]);
    const b = run(composing("ls", "ls"), [ev.compositionend("ls"), ev.beforeinput("insertLineBreak", null)]);
    // "ls" was already sent live, so the stream tail from Enter onward is one "\r".
    expect(a.bytes).toBe("\r");
    expect(b.bytes).toBe("\r");
  });

  it("reduce_should_FlushInsertOnlyTailThenCarriageReturn_When_EnterAfterMidRegionDrop", () => {
    const dropped = demoteToDeferred(composing("ls", "ls"), "l");
    const r = run(dropped, [ev.beforeinput("insertLineBreak", null)]);
    expect(r.results.map((x) => emitted(x.commands))).toEqual([["s", "\r"]]);
    expect(r.bytes).toBe("s\r");
  });

  it("reduce_should_EmitNoBytesAndAnomaly_When_DeferredTailDoesNotStartWithSent", () => {
    const r = run(composing("xs", "l", "deferred"), [ev.beforeinput("insertLineBreak", null)]);
    expect(r.bytes).toBe("\r");
    expect(anomalies(r.commands)).toEqual(["unrecognized_sequence"]);
  });

  it("reduce_should_EmitPlainCarriageReturn_When_EnterWithEmptyRegion", () => {
    const r = run(idle(), [ev.beforeinput("insertLineBreak", null)]);
    expect(r.bytes).toBe("\r");
  });

  it("reduce_should_TreatInsertParagraphLikeLineBreak_When_ComposingBeforeEnter", () => {
    const r = run(composing("ab", "ab"), [ev.beforeinput("insertParagraph", null)]);
    expect(r.bytes).toBe("\r");
  });

  it("reduce_should_ConsumeEnterFlushWithoutDel_When_RegionWasRewrittenBeforeEnter", () => {
    const r = run(composing("teh", "teh"), [
      ev.beforeinput("insertCompositionText", "the"),
      ev.beforeinput("insertLineBreak", null),
    ]);
    expect(r.bytes).toBe("\x7f\x7fhe\r");
  });
});

describe("reduce: Backspace and word delete", () => {
  it("reduce_should_EmitDel_When_DeleteContentBackwardInIdle", () => {
    const r = reduce(idle(), ev.beforeinput("deleteContentBackward", null), DEFAULT_POLICY);
    expect(emitted(r.commands)).toEqual(["\x7f"]);
  });

  it("reduce_should_ShrinkRegionByOneGrapheme_When_DeleteContentBackwardInsideComposition", () => {
    const r = run(composing("a👨‍👩‍👧", "a👨‍👩‍👧"), [ev.beforeinput("deleteContentBackward", null)]);
    expect(r.bytes).toBe("\x7f");
    expect(r.state).toMatchObject({ kind: "composing", region: "a", sent: "a" });
  });

  it("reduce_should_EmitPlainDelAndEndComposition_When_DeleteBackwardOnEmptyRegion", () => {
    const r = run(composing("", ""), [ev.beforeinput("deleteContentBackward", null)]);
    expect(r.bytes).toBe("\x7f");
    expect(r.state).toMatchObject({ kind: "idle" });
  });

  it("reduce_should_NeverDeleteMoreThanSent_When_BackspaceInsideDeferredComposition", () => {
    const r = run(composing("abc", "", "deferred"), [ev.beforeinput("deleteContentBackward", null)]);
    expect(r.bytes).toBe("");
    expect(r.state).toMatchObject({ kind: "composing", region: "ab", sent: "" });
  });

  it("reduce_should_EmitCtrlW_When_DeleteWordBackwardInIdle", () => {
    const r = run(idle(), [ev.beforeinput("deleteWordBackward", null)]);
    expect(r.bytes).toBe("\x17");
  });

  it("reduce_should_EmitBoundedDelRun_When_WordDeleteStrategyIsDelRun", () => {
    const policy = { ...DEFAULT_POLICY, wordDeleteStrategy: "del-run" } as const;
    const r = run(idle(), [ev.beforeinput("deleteWordBackward", null, "foo bar")], policy);
    expect(r.bytes).toBe("\x7f".repeat(3));
  });

  it("reduce_should_RewriteRegionWithoutLastWord_When_DeleteWordBackwardInsideComposition", () => {
    const r = run(composing("foo bar", "foo bar"), [ev.beforeinput("deleteWordBackward", null)]);
    expect(r.bytes).toBe("\x7f".repeat(3));
    expect(r.state).toMatchObject({ kind: "composing", region: "foo ", sent: "foo " });
  });
});

describe("reduce: Backspace on empty textarea after commit (P1-3)", () => {
  const afterCommit = () =>
    run(composing("hi", "hi"), [ev.compositionend("hi")]).state; // Idle, textarea reset

  it("reduce_should_EmitOneDelExactly_When_BackspaceAfterCommitOnResetTextarea (beforeinput only, sentinel)", () => {
    const r = run(afterCommit(), [ev.beforeinput("deleteContentBackward", null, "")]);
    expect(r.bytes).toBe("\x7f");
  });

  it("reduce_should_EmitOneDelExactly_When_BackspaceAfterCommitOnResetTextarea (keydown-8 only)", () => {
    const r = run(afterCommit(), [ev.keydown("Backspace", 8)], KEYDOWN8_POLICY);
    expect(r.bytes).toBe("\x7f");
    expect(r.results[0].preventDefault).toBe(true);
  });

  it("reduce_should_EmitOneDelExactly_When_BackspaceAfterCommitOnResetTextarea (both)", () => {
    for (const policy of [DEFAULT_POLICY, KEYDOWN8_POLICY]) {
      const r = run(afterCommit(), [
        ev.keydown("Backspace", 8),
        ev.beforeinput("deleteContentBackward", null, ""),
      ], policy);
      expect(r.bytes).toBe("\x7f");
    }
  });

  it("reduce_should_EmitOneDelExactly_When_Keydown229WithBackspaceKeyAndKeydown8Strategy", () => {
    const r = run(afterCommit(), [
      ev.keydown("Backspace", 229),
      ev.beforeinput("deleteContentBackward", null, ""),
    ], KEYDOWN8_POLICY);
    expect(r.bytes).toBe("\x7f");
    expect(r.results[0].preventDefault).toBe(false);
  });

  it("reduce_should_EmitNothingFromKeydown229_When_SentinelStrategy", () => {
    const r = run(afterCommit(), [ev.keydown("Backspace", 229)]);
    expect(r.bytes).toBe("");
  });

  it("reduce_should_EmitTwoDels_When_TwoSeparateBackspacePresses", () => {
    const r = run(afterCommit(), [
      ev.keydown("Backspace", 8),
      ev.beforeinput("deleteContentBackward", null, ""),
      ev.keydown("Backspace", 8),
      ev.beforeinput("deleteContentBackward", null, ""),
    ]);
    expect(r.bytes).toBe("\x7f\x7f");
  });

  it("reduce_should_ResetTextareaToSentinel_When_SentinelStrategyAndBackspaceHandled", () => {
    const r = reduce(idle(BACKSPACE_SENTINEL), ev.beforeinput("deleteContentBackward", null, ""), DEFAULT_POLICY);
    expect(r.commands).toContainEqual({ kind: "ResetTextarea" });
    expect(r.state).toMatchObject({ kind: "idle", shadow: BACKSPACE_SENTINEL });
  });
});

describe("reduce: orphaned input and Degraded", () => {
  it("reduce_should_EmitDeltaOnceAndResetTextarea_When_OrphanedInputWithoutBeforeInput", () => {
    const r = reduce(idle(), ev.input("/"), DEFAULT_POLICY);
    expect(r.commands).toEqual([
      { kind: "Emit", bytes: "/", atomic: true },
      { kind: "Anomaly", anomaly: "orphaned_input" },
      { kind: "ResetTextarea" },
    ]);
    expect(r.state.kind).toBe("degraded");
  });

  it("reduce_should_DiffAgainstSentinelShadow_When_OrphanedInputUnderSentinelStrategy", () => {
    const r = run(idle(BACKSPACE_SENTINEL), [ev.input(`${BACKSPACE_SENTINEL}/`)]);
    expect(r.bytes).toBe("/");
  });

  it("reduce_should_NotTreatInputAsOrphan_When_BeforeInputPrecededIt", () => {
    const r = run(idle(), [ev.beforeinput("insertText", "a"), ev.input("a")]);
    expect(r.bytes).toBe("a");
    expect(anomalies(r.commands)).toEqual([]);
  });

  it("reduce_should_BehaveAsIdle_When_EventFollowsDegraded", () => {
    const r = run(idle(), [ev.input("/"), ev.beforeinput("insertText", "a")]);
    expect(r.bytes).toBe("/a");
    expect(r.state.kind).toBe("idle");
  });

  it("reduce_should_IgnoreInput_When_ComposingState", () => {
    const r = run(composing("a", "a"), [ev.input("a")]);
    expect(r.commands).toEqual([]);
  });
});

describe("reduce: selection and watchdog", () => {
  it("reduce_should_FinalizeDeferredWithoutDel_When_SelectionChangeInsideRegion", () => {
    const r = reduce(composing("abc", "abc"), ev.selectionchange(1), DEFAULT_POLICY);
    expect(r.state.kind).toBe("idle");
    expect(emitted(r.commands)).toEqual([]);
    expect(anomalies(r.commands)).toEqual(["selection_in_region"]);
  });

  it("reduce_should_EmitInsertOnlyTail_When_SelectionChangeInsideDeferredRegion", () => {
    const r = run(composing("abc", "", "deferred"), [ev.selectionchange(1)]);
    expect(r.bytes).toBe("abc");
  });

  it("reduce_should_EmitNothing_When_CaretAtEndOfRegion", () => {
    const r = reduce(composing("abc", "abc"), ev.selectionchange(3), DEFAULT_POLICY);
    expect(r.commands).toEqual([]);
    expect(r.state.kind).toBe("composing");
  });

  it("reduce_should_EmitNothing_When_PureCaretMoveWhileIdle", () => {
    const r = reduce(idle(), ev.selectionchange(0), DEFAULT_POLICY);
    expect(r.commands).toEqual([]);
    expect(r.state.kind).toBe("idle");
  });

  it("reduce_should_ReturnToIdleWithStuckCompositionAnomaly_When_TickAfterTwoSeconds", () => {
    const r = reduce(composing("ab", "ab", "live", 0), ev.tick(2000), DEFAULT_POLICY);
    expect(r.state.kind).toBe("idle");
    expect(anomalies(r.commands)).toEqual(["stuck_composition"]);
  });

  it("reduce_should_StayComposing_When_TickBeforeTimeout", () => {
    const r = reduce(composing("ab", "ab", "live", 0), ev.tick(1999), DEFAULT_POLICY);
    expect(r.state.kind).toBe("composing");
    expect(r.commands).toEqual([]);
  });

  it("reduce_should_ResetTimeout_When_CompositionEventsArrive", () => {
    const r = run(composing("a", "a", "live", 0), [
      ev.beforeinput("insertCompositionText", "ab", "ab", 1500),
      ev.tick(3000),
    ]);
    expect(r.state.kind).toBe("composing");
  });

  it("reduce_should_CommitDeferredTextOnStuckTick_When_RegionWasNotSent", () => {
    const r = run(composing("ab", "", "deferred", 0), [ev.tick(2500)]);
    expect(r.bytes).toBe("ab");
  });

  it("reduce_should_EmitNothing_When_TickWhileIdle", () => {
    expect(reduce(idle(), ev.tick(99999), DEFAULT_POLICY).commands).toEqual([]);
  });
});

describe("reduce: bounds and deferral", () => {
  it("reduce_should_DeferRegionWithAnomaly_When_RegionExceedsMaxRegion", () => {
    const long = "a".repeat(MAX_REGION + 1);
    const r = reduce(composing("", ""), ev.beforeinput("insertCompositionText", long), DEFAULT_POLICY);
    expect(emitted(r.commands)).toEqual([]);
    expect(anomalies(r.commands)).toEqual(["rewrite_bound_exceeded"]);
    expect(r.commands).toContainEqual({ kind: "EnterDeferred" });
    expect(r.state).toMatchObject({ kind: "composing", mode: "deferred", sent: "" });
  });

  it("reduce_should_CommitWholeRegionOnCompositionEnd_When_Deferred", () => {
    const long = "a".repeat(MAX_REGION + 1);
    const r = run(composing("", ""), [
      ev.beforeinput("insertCompositionText", long),
      ev.compositionend(long),
    ]);
    expect(r.bytes).toBe(long);
  });

  it("reduce_should_DeferRegion_When_RegionContainsNewline", () => {
    const r = reduce(composing("", ""), ev.beforeinput("insertCompositionText", "a\nb"), DEFAULT_POLICY);
    expect(r.state).toMatchObject({ mode: "deferred" });
    expect(emitted(r.commands)).toEqual([]);
  });

  it("reduce_should_NotReportAnomalyAgain_When_AlreadyDeferred", () => {
    const r = run(composing("abc", "", "deferred"), [ev.beforeinput("insertCompositionText", "abcd")]);
    expect(anomalies(r.commands)).toEqual([]);
    expect(r.state).toMatchObject({ region: "abcd", mode: "deferred" });
  });
});

describe("reduce: delivery boundary (hooks for the ledger)", () => {
  it("reduce_should_EmitSThenCarriageReturn_When_DroppedSendDemotedRegionAndLinkIsBack", () => {
    const r = run(demoteToDeferred(composing("ls", "ls"), "l"), [ev.beforeinput("insertLineBreak", null)]);
    expect(r.results[0].commands.filter((c) => c.kind === "Emit").map((c) => (c as { bytes: string }).bytes)).toEqual([
      "s",
      "\r",
    ]);
  });

  it("reduce_should_EmitInsertOnlyTail_When_DemotedRegionCommitsOnCompositionEnd", () => {
    const r = run(demoteToDeferred(composing("ls", "ls"), "l"), [ev.compositionend("ls")]);
    expect(r.bytes).toBe("s");
    expect(delCountOf(r.bytes)).toBe(0);
  });

  it("reduce_should_NeverEmitDel_When_DemotedRegionIsAutocorrected", () => {
    const r = run(demoteToDeferred(composing("teh", "teh"), "te"), [
      ev.beforeinput("insertCompositionText", "the"),
      ev.compositionend("the"),
    ]);
    expect(r.bytes).toBe("");
    expect(anomalies(r.commands)).toEqual(["unrecognized_sequence"]);
  });

  it("reduce_should_EmitZeroBytes_When_RegionAbandoned", () => {
    const r = run(abandonRegion(composing("ls", "ls")), [
      ev.beforeinput("insertCompositionText", "lsx"),
      ev.compositionend("lsx"),
    ]);
    expect(r.bytes).toBe("");
    expect(r.state.kind).toBe("idle");
  });

  it("reduce_should_ReturnNonCompositionStateUnchanged_When_Demoting", () => {
    expect(demoteToDeferred(idle(), "")).toEqual(idle());
    expect(abandonRegion(idle())).toEqual(idle());
  });
});

function delCountOf(s: string): number {
  return [...s].filter((c) => c === "\x7f").length;
}
