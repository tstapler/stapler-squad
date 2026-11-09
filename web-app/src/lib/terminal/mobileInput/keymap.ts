import { ALT_KEY_MAP, CTRL_KEY_MAP, SHIFT_KEY_MAP } from "../modifierKeys";
import { reduce, utf8Length } from "./composition";
import { resetValue, type MobileInputPolicy } from "./policy";
import { graphemes } from "./rewrite";
import { asTerminalBytes, type InputCommand, type MobileInputEvent, type MobileInputState, type ReduceResult } from "./types";

export interface Modifiers {
  readonly ctrl: boolean;
  readonly alt: boolean;
  readonly shift: boolean;
}

export const NO_MODIFIERS: Modifiers = { ctrl: false, alt: false, shift: false };

export interface KeymapDeps {
  getModifiers(): Modifiers;
  /** Clears the armed modifiers; called exactly once per consuming character. */
  consumeModifiers(): void;
  /** DECSET 2004 state of the active renderer; false when unreadable (closed or unready terminal). */
  bracketedPasteEnabled(): boolean;
}

export interface KeymapResult extends ReduceResult {
  /** True when a multi-line paste was joined with spaces because bracketed paste was off. */
  flattened: boolean;
}

type Composing = Extract<MobileInputState, { kind: "composing" }>;
type Beforeinput = Extract<MobileInputEvent, { kind: "beforeinput" }>;
type EmitCommand = Extract<InputCommand, { kind: "Emit" }>;

const ESC = "\x1b";
const BRACKETED_PASTE_START = `${ESC}[200~`;
const BRACKETED_PASTE_END = `${ESC}[201~`;
const LINE_BREAKS = /(?:\r\n|\r|\n)+/g;

const anyArmed = (m: Modifiers) => m.ctrl || m.alt || m.shift;

/** Terminal byte for Ctrl plus a single character; unmapped characters pass through. */
function ctrlChar(ch: string): string {
  const mapped = CTRL_KEY_MAP[ch];
  if (mapped !== undefined) return mapped;
  if (ch === " ") return "\x00";
  const code = ch.charCodeAt(0);
  return ch.length === 1 && code >= 0x40 && code <= 0x7e ? String.fromCharCode(code & 0x1f) : ch;
}

/** Bytes for one typed grapheme under armed modifiers: Shift, then Ctrl, then Alt (ESC prefix). */
export function mapTypedChar(ch: string, mods: Modifiers): string {
  let out = ch;
  if (mods.shift) out = SHIFT_KEY_MAP[ch] ?? ch.toUpperCase();
  if (mods.ctrl && out.length === 1) out = ctrlChar(out);
  if (mods.alt) out = ALT_KEY_MAP[out] ?? ESC + out;
  return out;
}

function withBytes(cmd: EmitCommand, bytes: string, policy: MobileInputPolicy): EmitCommand {
  return { ...cmd, bytes: asTerminalBytes(bytes), atomic: utf8Length(bytes) < policy.atomicEditMaxBytes };
}

/** Bracketed paste when the renderer reports 2004 on, else newlines become spaces (a raw CR would submit each line). */
function shapePaste(cmd: EmitCommand, bracketed: boolean, policy: MobileInputPolicy): { cmd: EmitCommand; flattened: boolean } {
  const text = cmd.bytes as string;
  if (bracketed) {
    const safe = text.split(BRACKETED_PASTE_END).join("");
    return { cmd: withBytes(cmd, BRACKETED_PASTE_START + safe + BRACKETED_PASTE_END, policy), flattened: false };
  }
  const flat = text.replace(LINE_BREAKS, " ");
  return { cmd: withBytes(cmd, flat, policy), flattened: flat !== text };
}

/** Ends a live composition right after applying `event`: the text stays as typed, no modifier touches it. */
function reduceAndFlush(state: Composing, event: Beforeinput, policy: MobileInputPolicy): ReduceResult {
  const first = reduce(state, event, policy);
  if (first.state.kind !== "composing") return first;
  const end = reduce(first.state, { kind: "compositionend", data: first.state.region, t: event.t }, policy);
  return { state: end.state, commands: [...first.commands, ...end.commands], preventDefault: false };
}

const isCompositionInput = (e: MobileInputEvent): e is Beforeinput =>
  e.kind === "beforeinput" && (e.inputType === "insertCompositionText" || e.inputType === "insertReplacementText");

/** No composition text sent yet: the state before the first composed character, whether or not compositionstart already arrived. */
const isFreshComposition = (s: MobileInputState): boolean =>
  s.kind === "idle" || (s.kind === "composing" && s.mode === "live" && s.region === "" && s.sent === "");

/**
 * Gboard with suggestions starts every word as `insertCompositionText`; under an armed modifier that
 * first character is a committed key press. Emits it (mapped when single-grapheme), consumes the
 * modifier and ends the composition, swallowing the browser's trailing compositionend.
 */
function commitFreshComposition(
  event: Beforeinput,
  mods: Modifiers,
  policy: MobileInputPolicy,
  deps: KeymapDeps,
): ReduceResult {
  const data = event.data ?? event.value;
  const idleAfter: MobileInputState = {
    kind: "idle",
    shadow: resetValue(policy),
    swallow: [{ kind: "compositionend" }],
    inputExpected: true,
  };
  if (!data) return { state: idleAfter, commands: [{ kind: "ResetTextarea" }], preventDefault: false };
  const bytes = graphemes(data).length === 1 ? mapTypedChar(data, mods) : data;
  deps.consumeModifiers();
  const emit: EmitCommand = { kind: "Emit", bytes: asTerminalBytes(bytes), atomic: utf8Length(bytes) < policy.atomicEditMaxBytes };
  return { state: idleAfter, commands: [emit, { kind: "ResetTextarea" }], preventDefault: false };
}

/** Applies armed modifiers to the one Emit a non-composing `insertText` produced; null when nothing was typed. */
function applyTypedModifiers(
  commands: readonly InputCommand[],
  data: string,
  mods: Modifiers,
  policy: MobileInputPolicy,
): InputCommand[] | null {
  const idx = commands.findIndex((c) => c.kind === "Emit" && !c.paste && c.bytes === data);
  if (idx < 0) return null;
  const cmd = commands[idx] as EmitCommand;
  const single = graphemes(data).length === 1;
  return commands.map((c, i) => (i === idx && single ? withBytes(cmd, mapTypedChar(data, mods), policy) : c));
}

/**
 * The single output choke point for typed IME text: wraps `reduce` so every byte string that
 * leaves the layer passes modifier mapping and paste shaping here. Rewrite DELs and composition
 * text are never modified. Armed modifiers during a live composition flush it as typed and stay
 * armed for the next committed single character.
 */
export function reduceWithKeymap(
  state: MobileInputState,
  event: MobileInputEvent,
  policy: MobileInputPolicy,
  deps: KeymapDeps,
): KeymapResult {
  const mods = deps.getModifiers();
  const armed = anyArmed(mods);
  if (armed && isFreshComposition(state) && isCompositionInput(event) && event.inputType === "insertCompositionText") {
    const r = commitFreshComposition(event, mods, policy, deps);
    return { ...r, flattened: false };
  }
  const base =
    armed && state.kind === "composing" && isCompositionInput(event)
      ? reduceAndFlush(state, event, policy)
      : reduce(state, event, policy);

  let commands: InputCommand[] = [...base.commands];
  if (armed && state.kind !== "composing" && event.kind === "beforeinput" && event.inputType === "insertText") {
    const typed = applyTypedModifiers(commands, event.data ?? "", mods, policy);
    if (typed) {
      commands = typed;
      deps.consumeModifiers();
    }
  }

  let flattened = false;
  if (commands.some((c) => c.kind === "Emit" && c.paste)) {
    const bracketed = deps.bracketedPasteEnabled();
    commands = commands.map((c) => {
      if (c.kind !== "Emit" || !c.paste) return c;
      const shaped = shapePaste(c, bracketed, policy);
      flattened ||= shaped.flattened;
      return shaped.cmd;
    });
  }
  return { ...base, commands, flattened };
}
