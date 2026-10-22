import { canonicalValue, compareCensus, parseRules, reorderedSelectors, selectorSequence } from "../oracle/css-census";

const CLASSES = { clean: "c1", dark: "d1" };

describe("canonicalValue", () => {
  it("treats the notations Bun's minifier rewrites as equal", () => {
    expect(canonicalValue("#00cc33")).toBe(canonicalValue("#0c3"));
    expect(canonicalValue("rgba(0, 0, 0, 0.85)")).toBe(canonicalValue("#000000d9"));
    expect(canonicalValue("rgba(255,215,0,1)")).toBe(canonicalValue("gold"));
    expect(canonicalValue("100ms")).toBe(canonicalValue(".1s"));
    expect(canonicalValue("0.5")).toBe(canonicalValue(".5"));
  });

  it("keeps genuinely different values different", () => {
    expect(canonicalValue("#0c3")).not.toBe(canonicalValue("#0c4"));
    expect(canonicalValue("100ms")).not.toBe(canonicalValue("200ms"));
  });
});

describe("parseRules", () => {
  it("reads selectors lists, declarations and nested at-rules in order", () => {
    const rules = parseRules(".a,.b{color:red;--x:1}@media (min-width:1px){.c{margin:0}}@keyframes k{from{opacity:0}}");
    expect(rules[0]).toEqual({ at: [], selectors: [".a", ".b"], decls: [["color", "red"], ["--x", "1"]] });
    expect(rules[1]).toMatchObject({ at: ["@media (min-width:1px)"], selectors: [".c"] });
    expect(rules).toHaveLength(3);
  });
});

describe("compareCensus", () => {
  const next = ".c1{--bg:#00cc33;--t:100ms}.d1{--bg:#000}.a{x:1}.b{x:2}.z{x:3}";

  it("passes when the only differences are notation and rule merging", () => {
    const bun = ".c1{--bg:#0c3;--t:.1s}.d1{--bg:#000000}.a,.b{x:1}.z{x:3}";
    const r = compareCensus(next, bun, CLASSES);
    expect(r.themeDiffs).toEqual([]);
    expect(r.cascadeDiffs).toEqual([]);
  });

  it("fails a resolved custom-property difference", () => {
    const r = compareCensus(next, ".c1{--bg:#0c4;--t:.1s}.d1{--bg:#000}.a{x:1}.b{x:2}.z{x:3}", CLASSES);
    expect(r.themeDiffs).toEqual([expect.stringContaining("clean --bg")]);
  });

  it("fails a reordering that flips the winner of a repeated write, though counts stay equal", () => {
    const r = compareCensus(".a{color:red}.a{color:blue}", ".a{color:blue}.a{color:red}", CLASSES);
    expect(r.counts.bun).toBe(r.counts.next);
    expect(r.cascadeDiffs).toEqual([expect.stringContaining(".a | color")]);
  });

  it("reports but does not fail wholesale module reordering where no property is written twice", () => {
    const r = compareCensus(".a{x:1}.b{x:2}.z{x:3}", ".z{x:3}.a{x:1}.b{x:2}", CLASSES);
    expect(r.cascadeDiffs).toEqual([]);
    expect(r.movedSelectors).toBeGreaterThan(0);
  });

  it("ignores selectors only one build has", () => {
    expect(reorderedSelectors(selectorSequence(parseRules(".a{x:1}.b{x:1}")), selectorSequence(parseRules(".lazy{x:1}.a{x:1}.b{x:1}")))).toEqual([]);
  });
});
