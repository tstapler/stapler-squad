import { makePr } from "./__testUtils__/pullRequests";
import { canStopTracking, itemPrSummary } from "./itemPullRequests";

const merged = (n: number) => makePr({ number: n, state: "merged" });
const closed = (n: number) => makePr({ number: n, state: "closed" });
const open = (n: number) => makePr({ number: n });

describe("itemPrSummary", () => {
  it("itemPrSummary_should_sayKofNmerged_AndNameTheBlockingPr", () => {
    const s = itemPrSummary([merged(21), merged(22), open(23)]);
    expect(s).toMatchObject({ text: "2 of 3 merged", total: 3, merged: 2, showBar: true, valueText: "2 of 3 pull requests merged" });
    expect(s.ruleLine).toBe("Done when at least one PR is merged and none are open. Waiting on #23 (open).");
  });

  it("itemPrSummary_should_haveNoBar_When_OnePr", () => {
    expect(itemPrSummary([merged(1)])).toMatchObject({ text: "1 of 1 merged", showBar: false });
    expect(itemPrSummary([open(1)]).text).toBe("0 of 1 merged");
  });

  it("itemPrSummary_should_countClosedUnmergedAsResolved_When_NothingIsOpen", () => {
    const s = itemPrSummary([merged(1), merged(2), closed(3)]);
    expect(s.text).toBe("2 merged, 1 closed");
    expect(s.ruleLine).toBe("Done when at least one PR is merged and none are open. Every PR is resolved.");
  });

  it("itemPrSummary_should_explainWhyItCannotComplete_When_AllClosedUnmerged", () => {
    const s = itemPrSummary([closed(1), closed(2), closed(3)]);
    expect(s.text).toBe("0 merged, 3 closed. No merged PR, so the item can't complete.");
  });

  it("itemPrSummary_should_listEveryBlockingPr_WhenUnknownOrUnverified", () => {
    const s = itemPrSummary([merged(1), makePr({ number: 2, state: "unknown" }), makePr({ number: 3, state: "closed", outcomeUnverified: true })]);
    expect(s.ruleLine).toContain("Waiting on #2 (unknown), #3 (unverified).");
  });
});

describe("canStopTracking", () => {
  it("canStopTracking_should_offerTheExitOnUnresolvedRowsOnly", () => {
    expect(canStopTracking(open(1))).toBe(true);
    expect(canStopTracking(makePr({ number: 2, state: "unknown" }))).toBe(true);
    expect(canStopTracking(makePr({ number: 3, state: "draft" }))).toBe(true);
    expect(canStopTracking(merged(4))).toBe(false);
    expect(canStopTracking(closed(5))).toBe(false);
  });
});
