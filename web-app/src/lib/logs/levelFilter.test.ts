import { nextLevelFilters } from "./levelFilter";

describe("nextLevelFilters", () => {
  it("sets_only_the_tapped_level_when_no_filter_is_active", () => {
    expect(nextLevelFilters([], "ERROR")).toEqual(["ERROR"]);
    expect(nextLevelFilters(["ALL"], "ERROR")).toEqual(["ERROR"]);
  });
  it("resets_to_ALL_when_the_tapped_level_is_the_sole_filter", () => {
    expect(nextLevelFilters(["ERROR"], "ERROR")).toEqual(["ALL"]);
  });
  it("replaces_a_different_or_multi_level_filter", () => {
    expect(nextLevelFilters(["WARN"], "ERROR")).toEqual(["ERROR"]);
    expect(nextLevelFilters(["ERROR", "WARN"], "ERROR")).toEqual(["ERROR"]);
  });
});
