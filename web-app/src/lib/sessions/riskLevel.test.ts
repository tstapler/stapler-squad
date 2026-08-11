import { riskLevelRank, RISK_LEVEL_LABELS } from "./riskLevel";

// Pins the exact rank values so a one-sided edit to either this table or
// session/review_queue_poller.go's riskLevelRank map fails loudly here instead of silently
// desyncing default sort order between the Go-side GAP-004 tie-break and the frontend's
// default severity sort (sdd:6-verify Layer 2 finding, review-queue-severity). If this test
// ever needs to change, the Go mirror (TestRiskLevelRank_MatchesTypeScriptMirror) must change too.
describe("riskLevelRank", () => {
  it("matches the Go-side riskLevelRank mirror", () => {
    expect(riskLevelRank("critical")).toBe(4);
    expect(riskLevelRank("high")).toBe(3);
    expect(riskLevelRank("")).toBe(3);
    expect(riskLevelRank("medium")).toBe(2);
    expect(riskLevelRank("low")).toBe(1);
  });

  it("falls back to the unrecorded rank for an unrecognized value", () => {
    expect(riskLevelRank("garbage")).toBe(riskLevelRank(""));
  });
});

describe("RISK_LEVEL_LABELS", () => {
  it("has an entry for every known RiskLevel", () => {
    expect(RISK_LEVEL_LABELS).toEqual({
      critical: "Critical",
      high: "High",
      medium: "Medium",
      low: "Low",
    });
  });
});
