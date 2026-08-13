import { fuzzyMatchModels, getModelLabel } from "./modelFuzzyMatch";
import { MODEL_AUTOCOMPLETE_OPTIONS } from "./programs";

const ALL_VALUES = MODEL_AUTOCOMPLETE_OPTIONS.map((o) => o.value);

describe("fuzzyMatchModels", () => {
  it("is typo-tolerant: a misspelled query still surfaces the intended Sonnet entries", () => {
    // A plain substring filter (`"sonet".includes` against any suggestion) would
    // return zero results here — this is the exact gap the fuzzy filter fixes.
    const results = fuzzyMatchModels("sonet", ALL_VALUES);

    expect(results.length).toBeGreaterThan(0);
    expect(results).toContain("family:sonnet");
    // No unrelated Opus/Haiku entries should leak into a Sonnet-typo query.
    expect(results.some((v) => v.includes("opus"))).toBe(false);
    expect(results.some((v) => v.includes("haiku"))).toBe(false);
  });

  it("ranks results best-match-first rather than only filtering", () => {
    const results = fuzzyMatchModels("opus", ALL_VALUES);

    expect(results.length).toBeGreaterThan(0);
    // Fuse.js returns matches sorted by ascending score (best first); the
    // "family:opus" / "Claude Opus ..." entries should out-rank anything else.
    expect(results[0]).toMatch(/opus/);
  });

  it("lets a user find and select a model family without knowing the concrete version string", () => {
    const results = fuzzyMatchModels("sonnet", ALL_VALUES);
    expect(results).toContain("family:sonnet");
    expect(getModelLabel("family:sonnet")).toBe("Sonnet (latest)");
  });

  it("restricts results to the provided suggestions list", () => {
    const results = fuzzyMatchModels("opus", ["claude-opus-4-8"]);
    expect(results).toEqual(["claude-opus-4-8"]);
  });

  it("returns the full suggestions list unfiltered for an empty/whitespace query", () => {
    expect(fuzzyMatchModels("", ALL_VALUES)).toEqual(ALL_VALUES);
    expect(fuzzyMatchModels("   ", ALL_VALUES)).toEqual(ALL_VALUES);
  });
});

describe("getModelLabel", () => {
  it("returns the human label for a known value", () => {
    expect(getModelLabel("claude-sonnet-4-6")).toBe("Claude Sonnet 4.6");
  });

  it("falls back to the raw value for an unknown value", () => {
    expect(getModelLabel("some-custom-model-id")).toBe("some-custom-model-id");
  });
});
