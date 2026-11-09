import fc from "fast-check";
import { delCount, ev, idle, run } from "./testHelpers";
import { graphemes } from "./rewrite";
import { MAX_REGION } from "./policy";
import type { MobileInputEvent } from "./types";

/** A typed composition: a sequence of region texts the keyboard reports, ending in the final text. */
const regionSeq = fc
  .array(fc.string({ unit: "grapheme", minLength: 0, maxLength: 12 }).filter((s) => !/[\r\n]/.test(s)), {
    minLength: 1,
    maxLength: 8,
  });

const updates = (regions: readonly string[]): MobileInputEvent[] =>
  regions.map((r, i) => ev.beforeinput("insertCompositionText", r, r, i));

/** Terminal model: DEL erases one grapheme of the line, text appends. */
function lineAfter(bytes: string): string {
  let line: string[] = [];
  let buf = "";
  const flush = () => {
    if (buf) line.push(...graphemes(buf));
    buf = "";
  };
  for (const ch of bytes) {
    if (ch === "\x7f") {
      flush();
      line = line.slice(0, -1);
    } else buf += ch;
  }
  flush();
  return line.join("");
}

describe("reduce properties", () => {
  it("reduce_should_YieldIdenticalBytesAndNeverOverDelete_When_CompositionEndAndBeforeInputPermuted", () => {
    fc.assert(
      fc.property(regionSeq, (regions) => {
        const final = regions[regions.length - 1];
        const base = updates(regions);
        const t = regions.length;
        // Order A: compositionend, then the duplicate commit beforeinput.
        const a = run(idle(), [...base, ev.compositionend(final, t), ev.beforeinput("insertText", final, final, t)]);
        // Order B: commit beforeinput first, then compositionend.
        const b = run(idle(), [...base, ev.beforeinput("insertText", final, final, t), ev.compositionend(final, t)]);
        // Order C: compositionend only.
        const c = run(idle(), [...base, ev.compositionend(final, t)]);
        const maxSent = Math.max(...regions.map((r) => graphemes(r).length));
        return (
          a.bytes === b.bytes &&
          b.bytes === c.bytes &&
          lineAfter(a.bytes) === final &&
          delCount(a.bytes) <= regions.reduce((n, r) => n + graphemes(r).length, 0) &&
          maxSent <= MAX_REGION
        );
      }),
    );
  });

  it("reduce_should_LeaveTerminalLineEqualToFinalText_When_AnyCompositionSequenceCommits", () => {
    fc.assert(
      fc.property(regionSeq, (regions) => {
        const final = regions[regions.length - 1];
        const r = run(idle(), [...updates(regions), ev.compositionend(final, regions.length)]);
        return lineAfter(r.bytes) === final;
      }),
    );
  });

  it("reduce_should_NeverEmitMoreDelsInOneStepThanGraphemesSent_When_RegionsRewrite", () => {
    fc.assert(
      fc.property(regionSeq, (regions) => {
        let sent: string = "";
        let state = idle();
        for (const [i, e] of updates(regions).entries()) {
          const out = run(state, [e]);
          if (delCount(out.bytes) > graphemes(sent).length) return false;
          sent = regions[i];
          state = out.state;
        }
        return true;
      }),
    );
  });

  it("reduce_should_NeverDeleteBelowZero_When_EnterFollowsAnyComposition", () => {
    fc.assert(
      fc.property(regionSeq, fc.boolean(), (regions, enterFirst) => {
        const final = regions[regions.length - 1];
        const t = regions.length;
        const tail = enterFirst
          ? [ev.beforeinput("insertLineBreak", null, "", t), ev.compositionend(final, t)]
          : [ev.compositionend(final, t), ev.beforeinput("insertLineBreak", null, "", t)];
        const r = run(idle(), [...updates(regions), ...tail]);
        return r.bytes.endsWith("\r") && r.bytes.split("\r").length === 2 && lineAfter(r.bytes.slice(0, -1)) === final;
      }),
    );
  });
});
