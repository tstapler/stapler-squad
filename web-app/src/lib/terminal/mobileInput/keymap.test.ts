import { DEFAULT_POLICY, type MobileInputPolicy } from "./policy";
import { reduceWithKeymap, NO_MODIFIERS, mapTypedChar, type KeymapDeps, type Modifiers } from "./keymap";
import { composing, emitted, ev, idle } from "./testHelpers";
import type { InputCommand, MobileInputEvent, MobileInputState } from "./types";

interface Harness {
  deps: KeymapDeps;
  consumed: () => number;
  arm: (m: Partial<Modifiers>) => void;
}

function harness(bracketed = false, initial: Partial<Modifiers> = {}): Harness {
  let mods: Modifiers = { ...NO_MODIFIERS, ...initial };
  let consumed = 0;
  return {
    deps: {
      getModifiers: () => mods,
      consumeModifiers: () => {
        consumed++;
        mods = NO_MODIFIERS;
      },
      bracketedPasteEnabled: () => bracketed,
    },
    consumed: () => consumed,
    arm: (m) => {
      mods = { ...mods, ...m };
    },
  };
}

function step(
  h: Harness,
  state: MobileInputState,
  event: MobileInputEvent,
  policy: MobileInputPolicy = DEFAULT_POLICY,
) {
  const r = reduceWithKeymap(state, event, policy, h.deps);
  return { ...r, bytes: emitted(r.commands) };
}

describe("keymap modifiers", () => {
  it("keymap_should_EmitCtrlC_When_ArmedCtrlAndSingleCharC", () => {
    const h = harness(false, { ctrl: true });
    const r = step(h, idle(), ev.beforeinput("insertText", "c"));
    expect(r.bytes).toEqual(["\x03"]);
    expect(h.consumed()).toBe(1);
  });

  it("keymap_should_IgnoreModifierAndClearIt_When_MultiCharCommitUnderCtrl", () => {
    const h = harness(false, { ctrl: true });
    const r = step(h, idle(), ev.beforeinput("insertText", "hello "));
    expect(r.bytes).toEqual(["hello "]);
    expect(h.consumed()).toBe(1);
  });

  it("keymap_should_PrefixEsc_When_ArmedAltAndSingleChar", () => {
    const h = harness(false, { alt: true });
    expect(step(h, idle(), ev.beforeinput("insertText", "b")).bytes).toEqual(["\x1bb"]);
    expect(h.consumed()).toBe(1);
  });

  it("keymap_should_UppercaseLetterAndMapTab_When_ArmedShift", () => {
    expect(mapTypedChar("a", { ...NO_MODIFIERS, shift: true })).toBe("A");
    expect(mapTypedChar("\t", { ...NO_MODIFIERS, shift: true })).toBe("\x1b[Z");
  });

  it("keymap_should_ComposeShiftCtrlAlt_When_Combined", () => {
    expect(mapTypedChar("a", { ctrl: true, alt: true, shift: false })).toBe("\x1b\x01");
    expect(mapTypedChar("a", { ctrl: true, alt: false, shift: true })).toBe("\x01");
    expect(mapTypedChar("/", { ctrl: true, alt: false, shift: false })).toBe("\x1f");
    expect(mapTypedChar(" ", { ctrl: true, alt: false, shift: false })).toBe("\x00");
    expect(mapTypedChar("7", { ctrl: true, alt: false, shift: false })).toBe("7");
  });

  it("keymap_should_NotTouchOrConsume_When_NoModifierArmed", () => {
    const h = harness();
    const r = step(h, idle(), ev.beforeinput("insertText", "c"));
    expect(r.bytes).toEqual(["c"]);
    expect(h.consumed()).toBe(0);
  });

  it("keymap_should_NotConsume_When_EchoOfCommittedTextIsSuppressed", () => {
    const h = harness(false, { ctrl: true });
    const afterEnd = step(h, composing("l", "l"), ev.compositionend("l"));
    const echo = step(h, afterEnd.state, ev.beforeinput("insertText", "l"));
    expect(echo.bytes).toEqual([]);
    expect(h.consumed()).toBe(0);
  });

  it("keymap_should_FlushCompositionUnmodifiedThenApplyCtrlToNextChar_When_CtrlArmedMidComposition", () => {
    const h = harness(false, { ctrl: true });
    const flush = step(h, composing("he", "he"), ev.beforeinput("insertCompositionText", "hel"));
    expect(flush.bytes).toEqual(["l"]);
    expect(flush.state.kind).toBe("idle");
    expect(h.consumed()).toBe(0);
    const typed = step(h, flush.state, ev.beforeinput("insertText", "c"));
    expect(typed.bytes).toEqual(["\x03"]);
    expect(h.consumed()).toBe(1);
  });

  it("keymap_should_NeverModifyRewriteDels_When_CtrlArmedAndAutocorrectFlushes", () => {
    const h = harness(false, { ctrl: true });
    const r = step(h, composing("teh", "teh"), ev.beforeinput("insertReplacementText", "the"));
    expect(r.bytes).toEqual(["\x7f\x7fhe"]);
    expect(r.state.kind).toBe("idle");
  });

  it("keymap_should_ContinueLiveComposition_When_NoModifierArmed", () => {
    const h = harness();
    const r = step(h, composing("he", "he"), ev.beforeinput("insertCompositionText", "hel"));
    expect(r.state.kind).toBe("composing");
  });

  it("keymap_should_NotModifyBackspaceOrEnter_When_CtrlArmed", () => {
    const h = harness(false, { ctrl: true });
    expect(step(h, idle(), ev.beforeinput("deleteContentBackward", null)).bytes).toEqual(["\x7f"]);
    expect(step(h, idle(), ev.beforeinput("insertLineBreak", null)).bytes).toEqual(["\r"]);
    expect(h.consumed()).toBe(0);
  });
});

describe("keymap armed modifier on a fresh composition (Gboard suggestions on)", () => {
  const fresh = (kind: "idle" | "composingEmpty") => (kind === "idle" ? idle() : composing("", ""));

  it.each(["idle", "composingEmpty"] as const)(
    "keymap_should_EmitCtrlCAndConsume_When_ArmedCtrlAndFirstEventIsCompositionC_from_%s",
    (kind) => {
      const h = harness(false, { ctrl: true });
      const r = step(h, fresh(kind), ev.beforeinput("insertCompositionText", "c"));
      expect(r.bytes).toEqual(["\x03"]);
      expect(h.consumed()).toBe(1);
      expect(r.state.kind).toBe("idle");
      expect(r.commands.some((c) => c.kind === "ResetTextarea")).toBe(true);
    },
  );

  it("keymap_should_NotTurnFollowingSpaceIntoCtrlSpace_When_CtrlConsumedByComposedLetter", () => {
    const h = harness(false, { ctrl: true });
    const first = step(h, composing("", ""), ev.beforeinput("insertCompositionText", "c"));
    const end = step(h, first.state, ev.compositionend("c"));
    expect(end.bytes).toEqual([]);
    const space = step(h, end.state, ev.beforeinput("insertText", " "));
    expect(space.bytes).toEqual([" "]);
    expect(h.consumed()).toBe(1);
  });

  it("keymap_should_PrefixEscAndConsume_When_ArmedAltAndComposedLetter", () => {
    const h = harness(false, { alt: true });
    const r = step(h, composing("", ""), ev.beforeinput("insertCompositionText", "b"));
    expect(r.bytes).toEqual(["\x1bb"]);
    expect(h.consumed()).toBe(1);
  });

  it("keymap_should_UppercaseAndConsume_When_ArmedShiftAndComposedLetter", () => {
    const h = harness(false, { shift: true });
    const r = step(h, composing("", ""), ev.beforeinput("insertCompositionText", "a"));
    expect(r.bytes).toEqual(["A"]);
    expect(h.consumed()).toBe(1);
  });

  it("keymap_should_ConsumeWithoutMapping_When_ArmedCtrlAndMultiGraphemeFreshComposition", () => {
    const h = harness(false, { ctrl: true });
    const r = step(h, composing("", ""), ev.beforeinput("insertCompositionText", "hi"));
    expect(r.bytes).toEqual(["hi"]);
    expect(h.consumed()).toBe(1);
    expect(r.state.kind).toBe("idle");
  });

  it("keymap_should_KeepLiveComposition_When_NoModifierArmedAndFreshComposition", () => {
    const h = harness();
    const r = step(h, composing("", ""), ev.beforeinput("insertCompositionText", "c"));
    expect(r.state.kind).toBe("composing");
    expect(h.consumed()).toBe(0);
  });
});

describe("keymap word delete policy", () => {
  it("keymap_should_EmitCtrlW_When_WordDeleteStrategyIsCtrlW", () => {
    const h = harness();
    const r = step(h, idle(), ev.beforeinput("deleteWordBackward", null, "foo bar"));
    expect(r.bytes).toEqual(["\x17"]);
  });

  it("keymap_should_EmitBoundedDelRun_When_WordDeleteStrategyIsDelRun", () => {
    const h = harness();
    const policy: MobileInputPolicy = { ...DEFAULT_POLICY, wordDeleteStrategy: "del-run" };
    const r = step(h, idle(), ev.beforeinput("deleteWordBackward", null, "foo bar"), policy);
    expect(r.bytes).toEqual(["\x7f\x7f\x7f"]);
  });
});

describe("keymap paste", () => {
  const pasteCmds = (cmds: readonly InputCommand[]) => cmds.filter((c) => c.kind === "Emit" && c.paste);

  it("keymap_should_WrapInBracketedPaste_When_MultilineInsertTextAnd2004On", () => {
    const h = harness(true);
    const r = step(h, idle(), ev.beforeinput("insertText", "a\nb"));
    expect(r.bytes).toEqual(["\x1b[200~a\nb\x1b[201~"]);
    expect(pasteCmds(r.commands)).toHaveLength(1);
    expect(r.flattened).toBe(false);
  });

  it("keymap_should_FlattenNewlines_When_MultilinePasteAndBracketedPasteUnavailable", () => {
    const h = harness(false);
    const r = step(h, idle(), ev.beforeinput("insertText", "a\nb"));
    expect(r.bytes).toEqual(["a b"]);
    expect(r.bytes.join("")).not.toContain("\r");
    expect(r.flattened).toBe(true);
  });

  it("keymap_should_JoinWithSingleSpacePerBreak_When_CrLfAndBlankLinesPasted", () => {
    const h = harness(false);
    const r = step(h, idle(), ev.beforeinput("insertFromPaste", "a\r\n\r\nb\nc"));
    expect(r.bytes).toEqual(["a b c"]);
  });

  it("keymap_should_LeaveSingleLinePasteRawAndUnflagged_When_BracketedPasteOff", () => {
    const h = harness(false);
    const r = step(h, idle(), ev.beforeinput("insertFromPaste", "ls -la"));
    expect(r.bytes).toEqual(["ls -la"]);
    expect(r.flattened).toBe(false);
  });

  it("keymap_should_StripEmbeddedPasteEnd_When_BracketedPasteOn", () => {
    const h = harness(true);
    const r = step(h, idle(), ev.beforeinput("insertFromPaste", "x\x1b[201~rm -rf"));
    expect(r.bytes).toEqual(["\x1b[200~xrm -rf\x1b[201~"]);
  });

  it("keymap_should_ShapePasteFlushedFromComposition_When_PasteArrivesMidComposition", () => {
    const h = harness(false);
    const r = step(h, composing("ab", "ab"), ev.beforeinput("insertFromPaste", "x\ny"));
    expect(r.bytes).toEqual(["x y"]);
  });

  it("keymap_should_NotConsumeModifier_When_PasteArrivesUnderCtrl", () => {
    const h = harness(true, { ctrl: true });
    step(h, idle(), ev.beforeinput("insertFromPaste", "a\nb"));
    expect(h.consumed()).toBe(0);
  });
});
