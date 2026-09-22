import { applyCompletion, filterFlags, tokenAtCaret, type FlagOption, type FlagToken } from "./flagTokens";

const flag = (name: string, extra: Partial<FlagOption> = {}): FlagOption => ({
  name,
  short: "",
  takesValue: false,
  description: "",
  aliases: [],
  ...extra,
});

describe("flagTokens", () => {
  it("tokenAtCaret_should_ReturnTokenBounds_When_CaretMidString", () => {
    expect(tokenAtCaret("--yes --mo", 9)).toEqual({ start: 6, end: 10, text: "--mo" });
    expect(tokenAtCaret("--yes --mo", 0)).toEqual({ start: 0, end: 5, text: "--yes" });
    expect(tokenAtCaret("--yes --mo", 10)).toEqual({ start: 6, end: 10, text: "--mo" });
  });

  it.each([
    ["plain word", "foo", 3],
    ["empty value", "", 0],
    ["caret in whitespace", "--yes  ", 6],
    ["the -- terminator itself", "--", 2],
    ["after the -- terminator", "-- --mo", 7],
    ["inside an open quote", '--msg "a --mo', 13],
  ])("tokenAtCaret_should_ReturnNull_When_TokenNotDashOrAfterDoubleDash (%s)", (_n, value, caret) => {
    expect(tokenAtCaret(value, caret)).toBeNull();
  });

  it("applyCompletion_should_AppendTrailingSpaceNoEquals_When_FlagTakesValue", () => {
    const token = tokenAtCaret("--yes --mo", 9) as FlagToken;
    expect(applyCompletion("--yes --mo", token, flag("--model", { takesValue: true }))).toEqual({
      value: "--yes --model ",
      caret: 14,
    });
  });

  it("applyCompletion_should_NotDoubleSpace_When_SpaceFollowsToken", () => {
    const token = tokenAtCaret("--mo --yes", 4) as FlagToken;
    expect(applyCompletion("--mo --yes", token, flag("--model"))).toEqual({
      value: "--model --yes",
      caret: 8,
    });
  });

  it("filterFlags_should_MatchNameShortAndAliasesPrefixThenSubstring_When_TextGiven", () => {
    const flags = [
      flag("--no-model"),
      flag("--model", { short: "-m" }),
      flag("--verbose", { short: "-v" }),
      flag("--quiet", { aliases: ["--silent"] }),
    ];
    expect(filterFlags(flags, "-m").map((f) => f.name)).toEqual(["--model", "--no-model"]);
    expect(filterFlags(flags, "-v").map((f) => f.name)).toEqual(["--verbose"]);
    expect(filterFlags(flags, "--sil").map((f) => f.name)).toEqual(["--quiet"]);
    expect(filterFlags(flags, "--zzz")).toEqual([]);
  });
});
