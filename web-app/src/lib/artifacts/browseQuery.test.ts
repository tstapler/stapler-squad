import { activeFilterCount, checkQuery, modeOf, parseBrowseParams, parsePrParam, toSearch, EMPTY_PARAMS } from "./browseQuery";

describe("checkQuery", () => {
  it.each([
    ["", { kind: "empty" }],
    ["   ", { kind: "empty" }],
    ["a", { kind: "too_short", message: "Enter at least 2 characters" }],
    ["ab", { kind: "ok", shaHint: true }],
    ["1a2b3c", { kind: "ok", shaHint: true }],
    ["1a2b3c4", { kind: "ok", shaHint: false }],
    ["zz", { kind: "ok", shaHint: false }],
    ["token refresh", { kind: "ok", shaHint: false }],
  ])("checkQuery_should_classify_%j", (raw, want) => {
    expect(checkQuery(raw)).toEqual(want);
  });
});

describe("parseBrowseParams / toSearch", () => {
  it("round-trips the five URL filters and drops unknown enum values", () => {
    const sp = new URLSearchParams("type=pr&repo=acme/app&state=merged&q=zebra&pr=acme/app%2312");
    const p = parseBrowseParams(sp);
    expect(p).toEqual({ ...EMPTY_PARAMS, type: "pr", repo: "acme/app", state: "merged", q: "zebra", pr: "acme/app#12" });
    expect(parseBrowseParams(new URLSearchParams(toSearch(p)))).toEqual(p);
    expect(parseBrowseParams(new URLSearchParams("type=bogus&state=nope")).type).toBe("");
    expect(parseBrowseParams(new URLSearchParams("type=bogus&state=nope")).state).toBe("");
  });

  it("counts only the three filters, not q or pr", () => {
    expect(activeFilterCount({ ...EMPTY_PARAMS, type: "pr", repo: "a/b", q: "x", pr: "a/b#1" })).toBe(2);
  });
});

describe("parsePrParam / modeOf", () => {
  it("parses owner/repo#N and PR URLs, rejects the rest", () => {
    expect(parsePrParam("acme/app#12")).toEqual({ host: "", owner: "acme", repo: "app", number: 12 });
    expect(parsePrParam("https://github.com/acme/app/pull/12")).toEqual({ host: "", owner: "acme", repo: "app", number: 12 });
    expect(parsePrParam("https://ghe.example.com/acme/app/pull/12")?.host).toBe("ghe.example.com");
    expect(parsePrParam("acme/app")).toBeNull();
    expect(parsePrParam("#12")).toBeNull();
  });

  it("picks search over the PR lookup and browse by default", () => {
    expect(modeOf(EMPTY_PARAMS)).toBe("browse");
    expect(modeOf({ ...EMPTY_PARAMS, pr: "a/b#1" })).toBe("pr");
    expect(modeOf({ ...EMPTY_PARAMS, pr: "a/b#1", q: "x" })).toBe("search");
  });
});
