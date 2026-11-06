import { jumpToMatchNear } from "../jumpToMatchNear";

const doc = (n: number, at: Record<number, string> = {}) => Array.from({ length: n }, (_, i) => at[i + 1] ?? `line ${i + 1}`);

describe("jumpToMatchNear", () => {
  it("jumpToMatchNear_should_StayOnTheHitLine_When_TheTextIsStillThere", () => {
    expect(jumpToMatchNear(doc(50, { 20: "call needle()" }), 20, "needle", 200)).toEqual({ line: 20, col: 5 });
  });

  it("jumpToMatchNear_should_PickTheNearestLine_SearchingOutwardBothWays", () => {
    const lines = doc(500, { 100: "needle", 106: "needle" });
    expect(jumpToMatchNear(lines, 104, "needle", 200)).toEqual({ line: 106, col: 0 });
    expect(jumpToMatchNear(lines, 102, "needle", 200)).toEqual({ line: 100, col: 0 });
    // equidistant: the earlier line wins so the result does not depend on direction luck
    expect(jumpToMatchNear(doc(50, { 8: "needle", 12: "needle" }), 10, "needle", 200)).toEqual({ line: 8, col: 0 });
  });

  it("jumpToMatchNear_should_ReturnNull_When_NoLineWithinTheRadiusHasTheText", () => {
    const lines = doc(1000, { 500: "needle" });
    expect(jumpToMatchNear(lines, 200, "needle", 200)).toBeNull();
    expect(jumpToMatchNear(lines, 300, "needle", 200)).toEqual({ line: 500, col: 0 });
  });

  it("jumpToMatchNear_should_ClampAtDocumentEdges_And_RefuseAnEmptyNeedle", () => {
    expect(jumpToMatchNear(doc(5, { 5: "needle" }), 1, "needle", 200)).toEqual({ line: 5, col: 0 });
    expect(jumpToMatchNear(doc(5), 99, "line 5", 200)).toEqual({ line: 5, col: 0 });
    expect(jumpToMatchNear(doc(5), 2, "", 200)).toBeNull();
  });
});
