import fc from "fast-check";
import { computeRewrite, graphemes } from "./rewrite";
import { MAX_REGION } from "./policy";
import { asRegion, asSentText } from "./types";

const rewrite = (sent: string, region: string) => computeRewrite(asSentText(sent), asRegion(region));

/** Model of a terminal line: DEL removes the last grapheme, text appends. */
function applyToModel(buffer: string, backspaces: number, insert: string): string {
  const g = graphemes(buffer);
  return g.slice(0, g.length - backspaces).join("") + insert;
}

describe("computeRewrite", () => {
  it("computeRewrite_should_ReturnTwoBackspacesAndHe_When_TehBecomesThe", () => {
    expect(rewrite("teh", "the")).toEqual({ kind: "edit", backspaces: 2, insert: "he" });
  });

  it("computeRewrite_should_ReturnOnlyTail_When_RegionExtendsSent", () => {
    expect(rewrite("hel", "hello")).toEqual({ kind: "edit", backspaces: 0, insert: "lo" });
  });

  it("computeRewrite_should_ReturnNoBackspaces_When_SentIsEmpty", () => {
    expect(rewrite("", "hello")).toEqual({ kind: "edit", backspaces: 0, insert: "hello" });
  });

  it("computeRewrite_should_CountZwjSequenceAsOneGrapheme_When_EmojiIsReplaced", () => {
    expect(rewrite("a👨‍👩‍👧", "ab")).toEqual({ kind: "edit", backspaces: 1, insert: "b" });
  });

  it("computeRewrite_should_KeepEmojiPrefix_When_ZwjFamilyUnchanged", () => {
    expect(rewrite("👨‍👩‍👧x", "👨‍👩‍👧y")).toEqual({ kind: "edit", backspaces: 1, insert: "y" });
  });

  it("computeRewrite_should_TreatCombiningMarkAsPartOfGrapheme_When_AccentIsAdded", () => {
    // "e" -> "é" (e + U+0301) replaces one grapheme, not two code points.
    expect(rewrite("cafe", "café")).toEqual({ kind: "edit", backspaces: 1, insert: "é" });
  });

  it("computeRewrite_should_CountCjkCharactersIndividually_When_ComposingPinyin", () => {
    expect(rewrite("你好", "你们")).toEqual({ kind: "edit", backspaces: 1, insert: "们" });
  });

  it("computeRewrite_should_ReturnEmptyEdit_When_RegionEqualsSent", () => {
    expect(rewrite("same", "same")).toEqual({ kind: "edit", backspaces: 0, insert: "" });
  });

  it("computeRewrite_should_DeleteAllSent_When_RegionIsEmpty", () => {
    expect(rewrite("abc", "")).toEqual({ kind: "edit", backspaces: 3, insert: "" });
  });

  it("computeRewrite_should_ReportDeferredWithBoundExceeded_When_Region65GraphemesOrNewline", () => {
    expect(rewrite("", "a".repeat(MAX_REGION + 1))).toEqual({
      kind: "deferred",
      anomaly: "rewrite_bound_exceeded",
    });
    expect(rewrite("ab", "a\nb")).toEqual({ kind: "deferred", anomaly: "rewrite_bound_exceeded" });
    expect(rewrite("ab", "a\rb")).toEqual({ kind: "deferred", anomaly: "rewrite_bound_exceeded" });
  });

  it("computeRewrite_should_AcceptRegionOfExactlyMaxRegion_When_AtTheBound", () => {
    expect(rewrite("", "a".repeat(MAX_REGION))).toMatchObject({ kind: "edit" });
  });

  it("computeRewrite_should_CountGraphemesNotCodeUnitsForBound_When_RegionIsEmoji", () => {
    // 64 ZWJ families = 64 graphemes (but hundreds of UTF-16 units).
    expect(rewrite("", "👨‍👩‍👧".repeat(MAX_REGION))).toMatchObject({ kind: "edit" });
    expect(rewrite("", "👨‍👩‍👧".repeat(MAX_REGION + 1))).toMatchObject({ kind: "deferred" });
  });

  describe("properties", () => {
    const text = fc.string({ unit: "grapheme", maxLength: MAX_REGION }).filter((s) => !/[\r\n]/.test(s));

    it("never deletes more graphemes than SentText holds", () => {
      fc.assert(
        fc.property(text, text, (sent, region) => {
          const r = rewrite(sent, region);
          if (r.kind !== "edit") return true;
          return r.backspaces <= graphemes(sent).length && (sent !== "" || r.backspaces === 0);
        }),
      );
    });

    it("applying the edit to a model buffer holding SentText yields the Region", () => {
      fc.assert(
        fc.property(text, text, (sent, region) => {
          const r = rewrite(sent, region);
          if (r.kind !== "edit") return false;
          return applyToModel(sent, r.backspaces, r.insert) === region;
        }),
      );
    });

    it("keeps earlier text intact when it is prefixed to the model buffer", () => {
      fc.assert(
        fc.property(text, text, text, (before, sent, region) => {
          const r = rewrite(sent, region);
          if (r.kind !== "edit") return true;
          return applyToModel(before + sent, r.backspaces, r.insert).startsWith(before);
        }),
      );
    });

    it("handles ZWJ, emoji, CJK and combining marks as single graphemes", () => {
      const exotic = fc.array(
        fc.constantFrom("👨‍👩‍👧", "😀", "🇯🇵", "你", "好", "é", "a", "각", "👍🏽"),
        { maxLength: 20 },
      );
      fc.assert(
        fc.property(exotic, exotic, (sentParts, regionParts) => {
          const sent = sentParts.join("");
          const region = regionParts.join("");
          const r = rewrite(sent, region);
          if (r.kind !== "edit") return false;
          return (
            r.backspaces <= sentParts.length &&
            applyToModel(sent, r.backspaces, r.insert) === region
          );
        }),
      );
    });
  });
});
