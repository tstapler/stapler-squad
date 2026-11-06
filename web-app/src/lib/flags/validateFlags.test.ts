import { validateFlags } from "./validateFlags";
import type { FlagOption } from "./flagTokens";

const flag = (over: Partial<FlagOption> & { name: string }): FlagOption => ({
  short: "",
  takesValue: false,
  description: "",
  aliases: [],
  ...over,
});

const KNOWN = [
  flag({ name: "--model", takesValue: true }),
  flag({ name: "--verbose", short: "-v" }),
  flag({ name: "--allowedTools", takesValue: true, aliases: ["--allowed-tools"] }),
];

describe("validateFlags", () => {
  it.each([
    ["spec example", "--model=x -v --no-verbose --verbos -abc -- --anything", ["--verbos"]],
    ["known long flag", "--model x --verbose", []],
    ["alias hit", "--allowed-tools Bash --allowedTools Read", []],
    ["value after takes_value flag is skipped", "--model --not-a-flag --verbos", ["--verbos"]],
    ["unknown flag does not flag its value", "--bogus value", ["--bogus"]],
    ["unknown with =value reports the name only", "--bogus=1", ["--bogus"]],
    ["negation of unknown is unknown", "--no-nothing", ["--no-nothing"]],
    ["quoted values are never flags", '--model "-x --y" --verbose', []],
    ["quoted token alone is a value", "\"--looks-like-a-flag\"", []],
    ["terminator stops checking", "-- --bogus", []],
    ["bundled and unknown shorts are not judged", "-abc -z", []],
    ["duplicates reported once", "--bogus --bogus", ["--bogus"]],
    ["empty input", "   ", []],
  ])("validateFlags_should_%s", (_name, input, expected) => {
    expect(validateFlags(input, KNOWN)).toEqual(expected);
  });

  it("validateFlags_should_ReturnEmpty_When_ZeroKnownFlags", () => {
    expect(validateFlags("--anything --at-all", [])).toEqual([]);
  });
});
