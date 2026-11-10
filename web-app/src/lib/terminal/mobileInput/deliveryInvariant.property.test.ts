import fc from "fast-check";
import { InputEngine } from "./inputEngine";
import { NO_MODIFIERS } from "./keymap";
import { DEFAULT_POLICY } from "./policy";
import { DelBoundaryOracle, ModelTerminal } from "./replayHarness";
import { ev } from "./testHelpers";
import type { SendResult } from "./types";

type Op =
  | { op: "update"; text: string }
  | { op: "end"; text: string }
  | { op: "linkDown" }
  | { op: "linkUp" }
  | { op: "sessionSwitch" }
  | { op: "dropNext" };

const text = fc.string({ unit: "grapheme", minLength: 0, maxLength: 8 }).filter((s) => !/[\r\n\x7f\x1b]/.test(s));
const ops: fc.Arbitrary<Op[]> = fc.array(
  fc.oneof(
    { weight: 5, arbitrary: text.map((t): Op => ({ op: "update", text: t })) },
    { weight: 2, arbitrary: text.map((t): Op => ({ op: "end", text: t })) },
    { weight: 1, arbitrary: fc.constant<Op>({ op: "linkDown" }) },
    { weight: 1, arbitrary: fc.constant<Op>({ op: "linkUp" }) },
    { weight: 1, arbitrary: fc.constant<Op>({ op: "sessionSwitch" }) },
    { weight: 1, arbitrary: fc.constant<Op>({ op: "dropNext" }) },
  ),
  { minLength: 1, maxLength: 40 },
);

/** Replays an interleaving and returns the first invariant violation, or null. */
function violation(sequence: readonly Op[]): string | null {
  let link = true;
  let dropNext = 0;
  const oracle = new DelBoundaryOracle();
  const term = new ModelTerminal();

  const engine = new InputEngine({
    policy: DEFAULT_POLICY,
    deps: { getModifiers: () => NO_MODIFIERS, consumeModifiers: () => undefined, bracketedPasteEnabled: () => false },
    isReadOnly: () => false,
    onAnomaly: () => undefined,
    onData: (bytes): SendResult => {
      const dropped = !link || dropNext > 0;
      if (dropNext > 0) dropNext--;
      oracle.onUnit(bytes, !dropped);
      if (!dropped) term.apply(bytes);
      return dropped ? "dropped" : "sent";
    },
  });

  let t = 0;
  for (const step of sequence) {
    t++;
    switch (step.op) {
      case "update":
        engine.dispatch(ev.beforeinput("insertCompositionText", step.text, step.text, t));
        break;
      case "end":
        engine.dispatch(ev.compositionend(step.text, t));
        oracle.compositionEnded();
        break;
      case "linkDown":
        link = false;
        oracle.boundary();
        engine.invalidate("transport");
        break;
      case "linkUp":
        link = true;
        oracle.boundary();
        engine.invalidate("transport");
        break;
      case "sessionSwitch":
        oracle.sessionSwitch(engine.state.kind === "composing");
        engine.invalidate("session");
        break;
      case "dropNext":
        dropNext++;
        break;
    }
    if (oracle.violations.length) return oracle.violations[0];
  }
  return term.overDeletes > 0 ? `${term.overDeletes} DEL(s) ran past the start of the line` : null;
}

describe("no-DEL-across-boundary", () => {
  it("deleteCount_should_NeverExceedCurrentEpochGraphemes_When_RandomLinkSessionImeInterleavings", () => {
    fc.assert(
      fc.property(ops, (sequence) => violation(sequence) === null),
      { seed: 20261108, numRuns: 200 },
    );
  });

  it("violation_should_BeNull_When_UnitDroppedThenRegionAutocorrected", () => {
    const seq: Op[] = [
      { op: "update", text: "he" },
      { op: "dropNext" },
      { op: "update", text: "hel" },
      { op: "update", text: "yo" },
      { op: "end", text: "yo" },
    ];
    expect(violation(seq)).toBeNull();
  });

  it("oracle_should_FlagDelAndSessionBytes_When_LayerWereToEmitThemAcrossBoundary", () => {
    const o = new DelBoundaryOracle();
    o.onUnit("hel", true);
    o.boundary();
    o.onUnit("\x7f\x7f", true);
    expect(o.violations).toHaveLength(1);
    const s = new DelBoundaryOracle();
    s.onUnit("x", true);
    s.sessionSwitch(true);
    s.onUnit("y", true);
    expect(s.violations[0]).toMatch(/session/);
    s.compositionEnded();
    s.onUnit("z", true);
    expect(s.violations).toHaveLength(1);
  });
});
