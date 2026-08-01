import { derivePlanReviewStatus } from "./planReviewStatus";

describe("derivePlanReviewStatus", () => {
  it("derivePlanReviewStatus_should_ReturnSkipped_When_SkipPlanningIsTrue", () => {
    expect(
      derivePlanReviewStatus({
        skipPlanning: true,
        planApproved: false,
        planArtifactsPath: undefined,
        planRejectionReason: undefined,
      })
    ).toBe("skipped");
  });

  it("derivePlanReviewStatus_should_ReturnSkipped_When_SkipPlanningIsTrueEvenWithPlanArtifacts", () => {
    // skip-planning means "no plan ever required" — a different meaning than
    // "no plan yet" — and must win over every other signal.
    expect(
      derivePlanReviewStatus({
        skipPlanning: true,
        planApproved: true,
        planArtifactsPath: "some/path",
        planRejectionReason: "stale reason",
      })
    ).toBe("skipped");
  });

  it("derivePlanReviewStatus_should_ReturnChangesRequested_When_RejectionReasonIsSet", () => {
    expect(
      derivePlanReviewStatus({
        skipPlanning: false,
        planApproved: false,
        planArtifactsPath: "some/path",
        planRejectionReason: "missing caching plan",
      })
    ).toBe("changes_requested");
  });

  it("derivePlanReviewStatus_should_ReturnChangesRequested_When_RejectionReasonSetEvenIfPlanApprovedIsStaleTrue", () => {
    // Defensive: this combination shouldn't occur post-RejectPlan (which clears
    // plan_approved), but the derivation must still resolve deterministically.
    expect(
      derivePlanReviewStatus({
        skipPlanning: false,
        planApproved: true,
        planArtifactsPath: "some/path",
        planRejectionReason: "missing caching plan",
      })
    ).toBe("changes_requested");
  });

  it("derivePlanReviewStatus_should_ReturnApproved_When_PlanApprovedIsTrue", () => {
    expect(
      derivePlanReviewStatus({
        skipPlanning: false,
        planApproved: true,
        planArtifactsPath: "some/path",
        planRejectionReason: undefined,
      })
    ).toBe("approved");
  });

  it("derivePlanReviewStatus_should_ReturnPendingReview_When_PlanArtifactsPathSetButNotApprovedOrRejected", () => {
    expect(
      derivePlanReviewStatus({
        skipPlanning: false,
        planApproved: false,
        planArtifactsPath: "some/path",
        planRejectionReason: undefined,
      })
    ).toBe("pending_review");
  });

  it("derivePlanReviewStatus_should_ReturnNoPlan_When_NoPlanArtifactsPathAndNotSkipped", () => {
    expect(
      derivePlanReviewStatus({
        skipPlanning: false,
        planApproved: false,
        planArtifactsPath: undefined,
        planRejectionReason: undefined,
      })
    ).toBe("no_plan");
  });
});
