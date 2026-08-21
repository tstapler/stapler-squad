import {
  groupNavPages,
  NAV_PAGES,
  NAV_GROUP_LABELS,
  type NavGroup,
} from "@/lib/nav-pages";

describe("groupNavPages", () => {
  it("groupNavPages_should_placeItemsInCorrectBucket_When_givenFullNavPages", () => {
    const groups = groupNavPages(NAV_PAGES);
    const workItems = groups.get("work")!;
    const automationItems = groups.get("automation")!;
    expect(workItems.some((p) => p.label === "Sessions")).toBe(true);
    expect(automationItems.some((p) => p.label === "Workflows")).toBe(true);
    expect(automationItems.some((p) => p.label === "Rules")).toBe(true);
  });

  it("groupNavPages_should_preserveInsertionOrder_When_groupingItems", () => {
    const groups = groupNavPages(NAV_PAGES);
    const keys = Array.from(groups.keys());
    expect(keys[0]).toBe("work"); // work items appear first in NAV_PAGES
  });

  it("groupNavPages_should_returnEmptyMap_When_givenEmptyArray", () => {
    const groups = groupNavPages([]);
    expect(groups.size).toBe(0);
  });

  it("groupNavPages_should_accountForAllItems_When_givenFullNavPages", () => {
    const groups = groupNavPages(NAV_PAGES);
    const totalItems = Array.from(groups.values()).reduce(
      (sum, pages) => sum + pages.length,
      0
    );
    expect(totalItems).toBe(NAV_PAGES.length);
  });

  it("groupNavPages_should_haveLabelForAllFourGroups_When_checkingNAV_GROUP_LABELS", () => {
    const groups: NavGroup[] = ["work", "automation", "insights", "settings"];
    groups.forEach((g) => {
      expect(NAV_GROUP_LABELS[g]).toBeTruthy();
    });
  });
});
