import { derivePlanReviewStatus } from "./planReviewStatus";

type Input = Parameters<typeof derivePlanReviewStatus>[0];

function makeInput(overrides: Partial<Input> = {}): Input {
  return {
    skipPlanning: false,
    planApproved: false,
    planArtifactsPath: undefined,
    planRejectionReason: undefined,
    ...overrides,
  };
}

describe("derivePlanReviewStatus", () => {
  it("derivePlanReviewStatus_should_ReturnNoPlan_When_NoFieldsSet", () => {
    expect(derivePlanReviewStatus(makeInput())).toBe("no_plan");
  });

  it("derivePlanReviewStatus_should_ReturnPendingReview_When_PlanArtifactsPathSetAndNotApproved", () => {
    expect(
      derivePlanReviewStatus(makeInput({ planArtifactsPath: "/tmp/plans/item-1.md" })),
    ).toBe("pending_review");
  });

  it("derivePlanReviewStatus_should_ReturnApproved_When_PlanApprovedTrue", () => {
    expect(
      derivePlanReviewStatus(
        makeInput({ planApproved: true, planArtifactsPath: "/tmp/plans/item-1.md" }),
      ),
    ).toBe("approved");
  });

  it("derivePlanReviewStatus_should_ReturnChangesRequested_When_RejectionReasonSet", () => {
    expect(
      derivePlanReviewStatus(
        makeInput({ planRejectionReason: "Missing error handling for the retry path." }),
      ),
    ).toBe("changes_requested");
  });

  it("derivePlanReviewStatus_should_ReturnChangesRequested_When_PlanApprovedAndRejectionReasonBothSet", () => {
    // Defensive case: should never coexist post-symmetry-fix (RejectPlan
    // clears planApproved, ApprovePlan clears planRejectionReason), but the
    // derivation must still resolve deterministically rather than throwing
    // or silently picking "approved".
    expect(
      derivePlanReviewStatus(
        makeInput({
          planApproved: true,
          planRejectionReason: "Needs a different approach.",
          planArtifactsPath: "/tmp/plans/item-1.md",
        }),
      ),
    ).toBe("changes_requested");
  });

  it("derivePlanReviewStatus_should_ReturnSkipped_When_SkipPlanningTrueAndNoPlanArtifacts", () => {
    expect(derivePlanReviewStatus(makeInput({ skipPlanning: true }))).toBe("skipped");
  });

  it("derivePlanReviewStatus_should_ReturnSkipped_When_SkipPlanningTrueEvenWithRejectionReason", () => {
    // skipped wins over everything, including a non-empty rejection reason —
    // "planning intentionally bypassed" must never be confused with "plan
    // rejected" or "plan never reviewed".
    expect(
      derivePlanReviewStatus(
        makeInput({ skipPlanning: true, planRejectionReason: "stale reason from before skip" }),
      ),
    ).toBe("skipped");
  });
});
