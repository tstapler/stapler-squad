import {
  DEGRADE_NOTICE, DEGRADE_MIN_CHARS, FULL_PROFILE, LONG_LINE_CHARS, degradeProfile, longestLineLength,
} from "../degradeProfile";

const KB = 1000;
const text = (chars: number) => "a".repeat(chars);
const lines80 = (chars: number) => (text(79) + "\n").repeat(Math.floor(chars / 80));

describe("degradeProfile", () => {
  it("degradeProfile_should_TurnOffHighlightBracketsFold_When_600KbOnCoarsePointer", () => {
    // 600 KB in 80-char lines, the shape of a generated source file.
    const doc = (text(79) + "\n").repeat(Math.ceil((600 * KB) / 80));
    const p = degradeProfile({ doc, pointerCoarse: true });
    expect(p).toEqual({
      degraded: true, syntaxHighlight: false, bracketMatching: false, activeLine: false, foldGutter: false,
      wrap: true, notice: DEGRADE_NOTICE,
    });
    expect(DEGRADE_NOTICE.split("\n")).toHaveLength(1);
  });

  it("degradeProfile_should_KeepFullFeatures_When_FinePointerOrSmallFile", () => {
    const big = lines80(600 * KB);
    expect(degradeProfile({ doc: big, pointerCoarse: false })).toEqual(FULL_PROFILE);
    expect(degradeProfile({ doc: lines80(DEGRADE_MIN_CHARS - 80), pointerCoarse: true })).toEqual(FULL_PROFILE);
    expect(degradeProfile({ doc: "", pointerCoarse: true })).toEqual(FULL_PROFILE);
  });

  it("degradeProfile_should_TurnOffWrapping_When_4000CharSingleLineOnCoarsePointer", () => {
    const p = degradeProfile({ doc: text(LONG_LINE_CHARS), pointerCoarse: true });
    expect(p.wrap).toBe(false);
    expect(p.syntaxHighlight).toBe(true); // a small file keeps its features; only wrapping is dropped
    expect(degradeProfile({ doc: text(LONG_LINE_CHARS - 1), pointerCoarse: true }).wrap).toBe(true);
    expect(degradeProfile({ doc: text(LONG_LINE_CHARS), pointerCoarse: false }).wrap).toBe(true);
  });

  it("longestLineLength_should_HandleCrLfTrailingNewlineAndSingleLine", () => {
    expect(longestLineLength("ab\r\ncdef\r\ng")).toBe(4);
    expect(longestLineLength("abc\n")).toBe(3);
    expect(longestLineLength("one line")).toBe(8);
    expect(longestLineLength("")).toBe(0);
  });

  it("degradeProfile_should_KeepThresholdsAsNamedAdjustableConstants", () => {
    expect(DEGRADE_MIN_CHARS).toBeLessThan(600 * KB);
    expect(LONG_LINE_CHARS).toBe(4000);
  });
});
