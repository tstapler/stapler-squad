import { reduce } from "./composition";
import { DEFAULT_POLICY, type MobileInputPolicy } from "./policy";
import {
  asRegion,
  asSentText,
  type InputCommand,
  type MobileInputEvent,
  type MobileInputState,
  type ReduceResult,
} from "./types";

export const idle = (shadow = ""): MobileInputState => ({
  kind: "idle",
  shadow,
  swallow: [],
  inputExpected: false,
});

export const composing = (
  region: string,
  sent: string,
  mode: "live" | "deferred" = "live",
  lastT = 0,
): MobileInputState => ({
  kind: "composing",
  region: asRegion(region),
  sent: asSentText(sent),
  mode,
  lastT,
});

export const ev = {
  keydown: (key: string, keyCode: number, isComposing = false, t = 0): MobileInputEvent => ({
    kind: "keydown",
    key,
    keyCode,
    isComposing,
    t,
  }),
  beforeinput: (inputType: string, data: string | null, value = data ?? "", t = 0): MobileInputEvent => ({
    kind: "beforeinput",
    inputType,
    data,
    value,
    selStart: value.length,
    selEnd: value.length,
    t,
  }),
  input: (value: string, t = 0): MobileInputEvent => ({
    kind: "input",
    value,
    selStart: value.length,
    selEnd: value.length,
    t,
  }),
  compositionstart: (t = 0): MobileInputEvent => ({ kind: "compositionstart", t }),
  compositionupdate: (data: string, t = 0): MobileInputEvent => ({ kind: "compositionupdate", data, t }),
  compositionend: (data: string, t = 0): MobileInputEvent => ({ kind: "compositionend", data, t }),
  selectionchange: (selStart: number, selEnd = selStart, t = 0): MobileInputEvent => ({
    kind: "selectionchange",
    selStart,
    selEnd,
    t,
  }),
  blur: (t = 0): MobileInputEvent => ({ kind: "blur", t }),
  tick: (t: number): MobileInputEvent => ({ kind: "tick", t }),
};

export const emitted = (commands: readonly InputCommand[]): string[] =>
  commands.flatMap((c) => (c.kind === "Emit" ? [c.bytes as string] : []));

export const anomalies = (commands: readonly InputCommand[]): string[] =>
  commands.flatMap((c) => (c.kind === "Anomaly" ? [c.anomaly] : []));

export interface Run {
  state: MobileInputState;
  commands: InputCommand[];
  /** All emitted bytes joined. */
  bytes: string;
  results: ReduceResult[];
}

export function run(
  start: MobileInputState,
  events: readonly MobileInputEvent[],
  policy: MobileInputPolicy = DEFAULT_POLICY,
): Run {
  let state = start;
  const commands: InputCommand[] = [];
  const results: ReduceResult[] = [];
  for (const e of events) {
    const r = reduce(state, e, policy);
    state = r.state;
    commands.push(...r.commands);
    results.push(r);
  }
  return { state, commands, bytes: emitted(commands).join(""), results };
}

/** Count of DEL (0x7f) characters in a byte string. */
export const delCount = (bytes: string): number => [...bytes].filter((c) => c === "\x7f").length;
