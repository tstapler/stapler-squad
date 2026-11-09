import { buildFlowSearch, isKnownDrillKey, parseFlowUrlState } from "../flowUrlState";

const parse = (query: string) => parseFlowUrlState(new URLSearchParams(query));

describe("parseFlowUrlState", () => {
  it.each([
    ["", 30, "none", null, false],
    ["window=7", 7, "none", null, false],
    ["window=all", "all", "none", null, false],
    ["window=30&compare=prev&drill=rework.review_fail", 30, "prev", "rework.review_fail", false],
    ["window=all&compare=prev", "all", "none", null, true],
    ["compare=sideways", 30, "none", null, true],
  ])("parseFlowUrlState_should_Resolve_When_Query_%s", (query, window, compare, drill, needsRewrite) => {
    expect(parse(query)).toMatchObject({ window, compare, drill, needsRewrite });
  });

  it("parseFlowUrlState_should_DropBadParamsAndKeepOthers_When_WindowAndDrillInvalid", () => {
    const state = parse("window=13&drill=bogus&item=abc");
    expect(state.window).toBe(30);
    expect(state.notices).toEqual(["unsupported-window", "unknown-drill"]);
    expect(state.cleanSearch).toBe("item=abc");
  });
});

describe("isKnownDrillKey", () => {
  it.each([
    ["rework.review_fail", true],
    ["funnel.stage.in_progress", true],
    ["review.class.judgment", true],
    ["stuck.pr_checks", true],
    ["funnel.stage.", false],
    ["bogus", false],
    ["", false],
  ])("isKnownDrillKey_should_Match_%s", (key, expected) => {
    expect(isKnownDrillKey(key)).toBe(expected);
  });
});

describe("buildFlowSearch", () => {
  it("buildFlowSearch_should_KeepOtherParamsAndDropCompare_When_AllTimeChosen", () => {
    expect(buildFlowSearch(new URLSearchParams("window=30&compare=prev&item=abc"), { window: "all" })).toBe("?window=all&item=abc");
  });

  it("buildFlowSearch_should_ReturnEmptyString_When_NothingLeft", () => {
    expect(buildFlowSearch(new URLSearchParams("drill=rework.bounce"), { drill: null })).toBe("");
  });
});
