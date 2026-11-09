import { computeRewrite, graphemes } from "./rewrite";
import { resetValue, type MobileInputPolicy } from "./policy";
import {
  asRegion,
  asSentText,
  asTerminalBytes,
  type CompositionRegion,
  type InputCommand,
  type MobileInputEvent,
  type MobileInputState,
  type ReduceResult,
  type Swallow,
} from "./types";

type Idle = Extract<MobileInputState, { kind: "idle" }>;
type Composing = Extract<MobileInputState, { kind: "composing" }>;
type Beforeinput = Extract<MobileInputEvent, { kind: "beforeinput" }>;

const DEL = "\x7f";
const CTRL_W = "\x17";
const CR = "\r";

function assertNever(x: never): never {
  throw new Error(`unhandled variant: ${JSON.stringify(x)}`);
}

export function utf8Length(s: string): number {
  let n = 0;
  for (const ch of s) {
    const cp = ch.codePointAt(0) ?? 0;
    n += cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4;
  }
  return n;
}

function emit(bytes: string, policy: MobileInputPolicy, paste = false): InputCommand {
  const cmd: InputCommand = {
    kind: "Emit",
    bytes: asTerminalBytes(bytes),
    atomic: utf8Length(bytes) < policy.atomicEditMaxBytes,
  };
  return paste ? { ...cmd, paste: true } : cmd;
}

const anomaly = (a: Extract<InputCommand, { kind: "Anomaly" }>["anomaly"]): InputCommand => ({
  kind: "Anomaly",
  anomaly: a,
});
const RESET: InputCommand = { kind: "ResetTextarea" };

function idleState(policy: MobileInputPolicy, swallow: readonly Swallow[] = [], inputExpected = true): Idle {
  return { kind: "idle", shadow: resetValue(policy), swallow, inputExpected };
}

const result = (state: MobileInputState, commands: InputCommand[] = [], preventDefault = false): ReduceResult => ({
  state,
  commands,
  preventDefault,
});

/** Removes the first swallow entry matching `pred`; reports whether one matched. */
function takeSwallow(s: Idle, pred: (w: Swallow) => boolean): { matched: boolean; rest: Swallow[] } {
  const i = s.swallow.findIndex(pred);
  if (i < 0) return { matched: false, rest: [] };
  return { matched: true, rest: s.swallow.filter((_, j) => j !== i) };
}

/** Insert-only tail: `final` minus `sent`, or null when `final` does not start with `sent`. */
function insertOnlyTail(sent: string, final: string): string | null {
  return final.startsWith(sent) ? final.slice(sent.length) : null;
}

/** Bytes to turn what was sent into `final` when the region has to commit (live mode may rewrite). */
function commitCommands(c: Composing, final: string, policy: MobileInputPolicy): InputCommand[] {
  if (c.mode === "abandoned") return [];
  if (c.mode === "live") {
    const edit = computeRewrite(c.sent, asRegion(final), policy.maxRegion);
    if (edit.kind === "edit") {
      const bytes = DEL.repeat(edit.backspaces) + edit.insert;
      return bytes ? [emit(bytes, policy)] : [];
    }
  }
  const tail = insertOnlyTail(c.sent, final);
  if (tail === null) return [anomaly("unrecognized_sequence")];
  return tail ? [emit(tail, policy)] : [];
}

/** Ends the composition: commits `final`, then `after`, resets the textarea, returns to Idle. */
function finish(
  c: Composing,
  final: string,
  policy: MobileInputPolicy,
  swallow: readonly Swallow[],
  after: InputCommand[] = [],
): ReduceResult {
  return result(idleState(policy, swallow), [...commitCommands(c, final, policy), ...after, RESET]);
}

const COMPOSITION_END_ECHO: readonly Swallow[] = [{ kind: "compositionend" }];

/** Moves a live region to deferred after a transport boundary; `delivered` is what is known sent. */
export function demoteToDeferred(state: MobileInputState, delivered: string): MobileInputState {
  if (state.kind !== "composing" || state.mode === "abandoned") return state;
  return { ...state, mode: "deferred", sent: asSentText(delivered) };
}

/** Session-class invalidation: the region emits nothing further. */
export function abandonRegion(state: MobileInputState): MobileInputState {
  return state.kind === "composing" ? { ...state, mode: "abandoned" } : state;
}

function applyRegion(c: Composing, next: string, policy: MobileInputPolicy): ReduceResult {
  const region: CompositionRegion = asRegion(next);
  if (c.mode !== "live") return result({ ...c, region });
  const edit = computeRewrite(c.sent, region, policy.maxRegion);
  if (edit.kind === "deferred") {
    return result({ ...c, region, mode: "deferred" }, [anomaly(edit.anomaly), { kind: "EnterDeferred" }]);
  }
  const bytes = DEL.repeat(edit.backspaces) + edit.insert;
  return result({ ...c, region, sent: asSentText(next) }, bytes ? [emit(bytes, policy)] : []);
}

function startComposition(t: number): Composing {
  return { kind: "composing", region: asRegion(""), sent: asSentText(""), mode: "live", lastT: t };
}

/** Bytes for a word-delete that the composition could not absorb. */
function wordDelete(e: Beforeinput, policy: MobileInputPolicy): string {
  if (policy.wordDeleteStrategy === "ctrl-w") return CTRL_W;
  const before = e.value.replace(/​/g, "").slice(0, e.selStart);
  const word = /\S+\s*$/u.exec(before)?.[0] ?? "";
  const count = Math.min(Math.max(graphemes(word).length, 1), policy.maxRegion);
  return DEL.repeat(count);
}

const isBackspaceKey = (e: Extract<MobileInputEvent, { kind: "keydown" }>) =>
  e.key === "Backspace" || e.keyCode === 8;
const isEnterKey = (e: Extract<MobileInputEvent, { kind: "keydown" }>) => e.key === "Enter" || e.keyCode === 13;

function onKeydown(
  s: Idle | Composing,
  e: Extract<MobileInputEvent, { kind: "keydown" }>,
  policy: MobileInputPolicy,
): ReduceResult {
  const imeKey = e.isComposing || e.keyCode === 229;
  if (s.kind === "composing") {
    if (imeKey) return result(s);
    if (isEnterKey(e)) return finish(s, s.region, policy, [{ kind: "lineBreak" }], [emit(CR, policy)]);
    if (isBackspaceKey(e)) {
      return finish(s, s.region, policy, [{ kind: "deleteBackward" }], [emit(DEL, policy)]);
    }
    return result(s);
  }
  if (imeKey) {
    // The only keydown-229 shape worth acting on: Backspace the browser will not report via beforeinput.
    if (policy.backspaceStrategy === "keydown8" && e.key === "Backspace") {
      return result(idleState(policy, [{ kind: "deleteBackward" }], false), [emit(DEL, policy)]);
    }
    return result(idleState(policy, [], false));
  }
  if (isEnterKey(e)) {
    return result(idleState(policy, [{ kind: "lineBreak" }], false), [emit(CR, policy)], true);
  }
  if (isBackspaceKey(e)) {
    return result(idleState(policy, [{ kind: "deleteBackward" }], false), [emit(DEL, policy)], true);
  }
  return result(idleState(policy, [], false));
}

const isMultiLine = (text: string) => /[\r\n]/.test(text);

function composingBeforeinput(c: Composing, e: Beforeinput, policy: MobileInputPolicy): ReduceResult {
  const data = e.data ?? "";
  switch (e.inputType) {
    case "insertCompositionText":
    case "insertReplacementText":
      return applyRegion(c, e.data ?? e.value, policy);
    case "insertText":
      return finish(c, data, policy, COMPOSITION_END_ECHO);
    case "insertFromPaste":
      return finish(c, c.region, policy, COMPOSITION_END_ECHO, data ? [emit(data, policy, true)] : []);
    case "insertLineBreak":
    case "insertParagraph":
      return finish(c, c.region, policy, COMPOSITION_END_ECHO, [emit(CR, policy)]);
    case "deleteContentBackward": {
      if (c.region === "") return finish(c, "", policy, COMPOSITION_END_ECHO, [emit(DEL, policy)]);
      return applyRegion(c, graphemes(c.region).slice(0, -1).join(""), policy);
    }
    case "deleteWordBackward": {
      if (c.region === "") return finish(c, "", policy, COMPOSITION_END_ECHO, [emit(wordDelete(e, policy), policy)]);
      return applyRegion(c, c.region.replace(/\S+\s*$/u, ""), policy);
    }
    default:
      return result(c, [anomaly("unrecognized_sequence")]);
  }
}

function idleBeforeinput(s: Idle, e: Beforeinput, policy: MobileInputPolicy): ReduceResult {
  const data = e.data ?? "";
  const next = (swallow: readonly Swallow[] = []) => idleState(policy, swallow, true);
  const suppressed = (rest: Swallow[]) => result(next(rest), [anomaly("duplicate_suppressed")]);
  switch (e.inputType) {
    case "insertCompositionText":
      return applyRegion(startComposition(e.t), e.data ?? e.value, policy);
    case "insertText": {
      const m = takeSwallow(s, (w) => w.kind === "insertText" && w.data === data);
      if (m.matched) return suppressed(m.rest);
      if (!data) return result(next());
      return result(next(), [emit(data, policy, isMultiLine(data)), RESET]);
    }
    case "insertFromPaste":
      return result(next(), data ? [emit(data, policy, true), RESET] : [RESET]);
    case "insertLineBreak":
    case "insertParagraph": {
      const m = takeSwallow(s, (w) => w.kind === "lineBreak");
      if (m.matched) return suppressed(m.rest);
      return result(next(), [emit(CR, policy), RESET]);
    }
    case "deleteContentBackward": {
      const m = takeSwallow(s, (w) => w.kind === "deleteBackward");
      if (m.matched) return suppressed(m.rest);
      return result(next(), [emit(DEL, policy), RESET]);
    }
    case "deleteWordBackward":
      return result(next(), [emit(wordDelete(e, policy), policy), RESET]);
    case "insertReplacementText":
    default:
      return result(next(), [anomaly("unrecognized_sequence"), RESET]);
  }
}

function onCompositionEnd(
  s: Idle | Composing,
  e: Extract<MobileInputEvent, { kind: "compositionend" }>,
  policy: MobileInputPolicy,
): ReduceResult {
  if (s.kind === "composing") {
    // An empty data string would make a rewrite erase text the user just typed; trust the region instead.
    const final = e.data !== "" ? e.data : s.region;
    const echo: Swallow[] = final ? [{ kind: "insertText", data: final }] : [];
    return finish(s, final, policy, echo);
  }
  const m = takeSwallow(s, (w) => w.kind === "compositionend");
  if (m.matched) return result(idleState(policy, m.rest, true), [RESET]);
  return result(idleState(policy, [], true), [anomaly("unrecognized_sequence")]);
}

/** Diffs the textarea value against the shadow when no beforeinput explained the change. */
function onInput(s: Idle, e: Extract<MobileInputEvent, { kind: "input" }>, policy: MobileInputPolicy): ReduceResult {
  if (s.inputExpected) return result({ ...s, inputExpected: false });
  const edit = computeRewrite(asSentText(s.shadow), asRegion(e.value), Number.MAX_SAFE_INTEGER);
  const degraded: MobileInputState = { kind: "degraded", shadow: resetValue(policy) };
  if (edit.kind !== "edit") return result(degraded, [anomaly("orphaned_input"), RESET]);
  const bytes = DEL.repeat(edit.backspaces) + edit.insert;
  if (!bytes) return result({ ...s });
  return result(degraded, [emit(bytes, policy), anomaly("orphaned_input"), RESET]);
}

function onSelectionChange(
  s: Idle | Composing,
  e: Extract<MobileInputEvent, { kind: "selectionchange" }>,
  policy: MobileInputPolicy,
): ReduceResult {
  if (s.kind !== "composing") return result(s);
  const insideRegion = e.selStart !== e.selEnd || e.selStart < s.region.length;
  if (!insideRegion) return result(s);
  return finish(s, s.region, policy, COMPOSITION_END_ECHO, [anomaly("selection_in_region")]);
}

/**
 * Pure reducer: one IME event in, bytes and next state out. Time arrives in events (`t`, `tick`);
 * the reducer never reads a clock. The exhaustive switches fail to compile when a variant is added.
 */
export function reduce(
  state: MobileInputState,
  event: MobileInputEvent,
  policy: MobileInputPolicy,
): ReduceResult {
  let s: Idle | Composing =
    state.kind === "degraded" ? { kind: "idle", shadow: state.shadow, swallow: [], inputExpected: false } : state;
  if (s.kind === "composing" && event.kind !== "tick") s = { ...s, lastT: event.t };

  switch (event.kind) {
    case "keydown":
      return onKeydown(s, event, policy);
    case "beforeinput":
      return s.kind === "composing" ? composingBeforeinput(s, event, policy) : idleBeforeinput(s, event, policy);
    case "input":
      return s.kind === "composing" ? result(s) : onInput(s, event, policy);
    case "compositionstart":
      return result(s.kind === "composing" ? s : startComposition(event.t));
    case "compositionupdate":
      return applyRegion(s.kind === "composing" ? s : startComposition(event.t), event.data, policy);
    case "compositionend":
      return onCompositionEnd(s, event, policy);
    case "selectionchange":
      return onSelectionChange(s, event, policy);
    case "blur":
      return s.kind === "composing" ? finish(s, s.region, policy, COMPOSITION_END_ECHO) : result(s);
    case "tick":
      if (s.kind === "composing" && event.t - s.lastT >= policy.stuckCompositionMs) {
        return finish(s, s.region, policy, COMPOSITION_END_ECHO, [anomaly("stuck_composition")]);
      }
      return result(s);
    default:
      return assertNever(event);
  }
}
