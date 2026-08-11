import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { StuckItemDetail } from "./StuckItemDetail";

function makeItem(overrides: Partial<StuckBacklogItem> = {}): StuckBacklogItem {
  return {
    itemId: "f9fcef32-c27e-434d-b23f-c873c18afa92",
    title: "fix: benchmark job CI",
    status: "pr_pending",
    reason: StuckReason.PR_READY_UNMERGED,
    firstDetectedAt: timestampFromDate(new Date(Date.now() - 3 * 24 * 60 * 60 * 1000)),
    lastCheckedAt: timestampFromDate(new Date(Date.now() - 47 * 1000)),
    prNumber: 148,
    prUrl: "https://github.com/tstapler/stapler-squad/pull/148",
    context: "PR #148 green & mergeable, unmerged for 3 days",
    planArtifactsPath: "",
    ...overrides,
  } as StuckBacklogItem;
}

describe("StuckItemDetail", () => {
  describe("StuckItemDetail_should_showAutoMergeUnknownAndRenderRest_When_SettingFetchFailed", () => {
    it("shows 'unknown' when allowAutoMerge is undefined, and still renders the rest of the detail", () => {
      render(<StuckItemDetail item={makeItem({ allowAutoMerge: undefined })} />);
      expect(screen.getByTestId("stuck-item-auto-merge").textContent).toMatch(
        /Repo auto-merge: unknown/
      );
      expect(screen.getByTestId("stuck-item-why")).toBeInTheDocument();
      expect(screen.getByTestId("stuck-item-last-check")).toBeInTheDocument();
      expect(screen.getByTestId("stuck-item-pr-link")).toBeInTheDocument();
    });

    it("shows 'off' when allowAutoMerge is explicitly false", () => {
      render(<StuckItemDetail item={makeItem({ allowAutoMerge: false })} />);
      expect(screen.getByTestId("stuck-item-auto-merge").textContent).toMatch(
        /Repo auto-merge: off/
      );
    });

    it("shows 'on' when allowAutoMerge is explicitly true", () => {
      render(<StuckItemDetail item={makeItem({ allowAutoMerge: true })} />);
      expect(screen.getByTestId("stuck-item-auto-merge").textContent).toMatch(
        /Repo auto-merge: on/
      );
    });
  });

  describe("StuckItemDetail_should_showExplicitMergeCopy_When_ReasonIsPrReadyUnmerged", () => {
    it("renders the literal 'This PR is ready — merge it on GitHub when you're ready.' line", () => {
      render(<StuckItemDetail item={makeItem()} />);
      expect(screen.getByTestId("stuck-item-action-copy").textContent).toBe(
        "This PR is ready — merge it on GitHub when you're ready."
      );
    });

    it("links to the PR with target=_blank rel=noreferrer", () => {
      render(<StuckItemDetail item={makeItem()} />);
      const link = screen.getByTestId("stuck-item-pr-link");
      expect(link).toHaveAttribute("target", "_blank");
      expect(link).toHaveAttribute("rel", "noreferrer");
      expect(link).toHaveAttribute("href", "https://github.com/tstapler/stapler-squad/pull/148");
    });
  });

  describe("pr_status_unknown detail copy", () => {
    it("renders the literal 'no action available' line when the PR status check is stale", () => {
      const item = makeItem({
        lastCheckedAt: timestampFromDate(new Date(Date.now() - 47 * 60 * 1000)),
      });
      render(<StuckItemDetail item={item} />);
      expect(screen.getByTestId("stuck-item-no-action-copy").textContent).toBe(
        "Couldn't check this PR's status — no action available."
      );
      expect(screen.queryByTestId("stuck-item-action-copy")).not.toBeInTheDocument();
    });
  });

  describe("context fallback", () => {
    it("falls back to a generic sentence when context is empty", () => {
      render(<StuckItemDetail item={makeItem({ context: "" })} />);
      expect(screen.getByTestId("stuck-item-why").textContent).toBe(
        "No additional context recorded"
      );
    });
  });

  describe("non-PR reasons", () => {
    it("omits the auto-merge line and PR link for a rework_cap item", () => {
      render(
        <StuckItemDetail
          item={makeItem({
            reason: StuckReason.REWORK_CAP,
            prNumber: 0,
            prUrl: "",
            context: "Auto-rework stopped after 3 failed review cycles (cap = 3 work sessions)",
          })}
        />
      );
      expect(screen.queryByTestId("stuck-item-auto-merge")).not.toBeInTheDocument();
      expect(screen.queryByTestId("stuck-item-pr-link")).not.toBeInTheDocument();
      expect(screen.queryByTestId("stuck-item-action-copy")).not.toBeInTheDocument();
    });
  });

  describe("StuckItemDetail_should_showFixGuidance_When_ReasonIsReworkCap", () => {
    it("renders how-to-fix copy pointing at Reopen for Revision and Settings", () => {
      render(
        <StuckItemDetail
          item={makeItem({
            reason: StuckReason.REWORK_CAP,
            prNumber: 0,
            prUrl: "",
            context: "hit the 3-iteration rework cap after a failed review verdict",
          })}
        />
      );
      const copy = screen.getByTestId("stuck-item-rework-cap-copy");
      expect(copy.textContent).toMatch(/Reopen for Revision/);
      expect(copy.textContent).toMatch(/cap/i);
    });

    it("does not render for a non-rework_cap reason", () => {
      render(<StuckItemDetail item={makeItem({ reason: StuckReason.PR_READY_UNMERGED })} />);
      expect(screen.queryByTestId("stuck-item-rework-cap-copy")).not.toBeInTheDocument();
    });
  });

  describe("StuckItemDetail_should_offerOverrideControl_When_ReasonIsReworkCapAndHandlerProvided", () => {
    it("does not render the override form when no handler is provided", () => {
      render(<StuckItemDetail item={makeItem({ reason: StuckReason.REWORK_CAP })} />);
      expect(screen.queryByTestId("stuck-item-rework-cap-override-form")).not.toBeInTheDocument();
    });

    it("calls onReworkCapOverride with the entered cap value when 'Set cap' is clicked", async () => {
      const onReworkCapOverride = jest.fn().mockResolvedValue(true);
      render(
        <StuckItemDetail
          item={makeItem({ reason: StuckReason.REWORK_CAP, itemId: "item-abc" })}
          onReworkCapOverride={onReworkCapOverride}
        />
      );
      const input = screen.getByTestId("stuck-item-rework-cap-rounds-input");
      fireEvent.change(input, { target: { value: "7" } });
      fireEvent.click(screen.getByTestId("stuck-item-rework-cap-allow-rounds"));

      await waitFor(() => expect(onReworkCapOverride).toHaveBeenCalledWith("item-abc", 7));
    });

    it("calls onReworkCapOverride with 0 (unlimited) when 'Remove cap' is clicked", async () => {
      const onReworkCapOverride = jest.fn().mockResolvedValue(true);
      render(
        <StuckItemDetail
          item={makeItem({ reason: StuckReason.REWORK_CAP, itemId: "item-xyz" })}
          onReworkCapOverride={onReworkCapOverride}
        />
      );
      fireEvent.click(screen.getByTestId("stuck-item-rework-cap-unlimited"));

      await waitFor(() => expect(onReworkCapOverride).toHaveBeenCalledWith("item-xyz", 0));
    });

    it("shows an error message when the override call fails", async () => {
      const onReworkCapOverride = jest.fn().mockResolvedValue(false);
      render(
        <StuckItemDetail
          item={makeItem({ reason: StuckReason.REWORK_CAP })}
          onReworkCapOverride={onReworkCapOverride}
        />
      );
      fireEvent.click(screen.getByTestId("stuck-item-rework-cap-unlimited"));

      await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    });
  });

  describe("StuckItemDetail_should_showAutonomousStuckGuidance_When_ReasonIsAutonomousStuck", () => {
    it("renders guidance copy for autonomous_stuck", () => {
      render(
        <StuckItemDetail
          item={makeItem({
            reason: StuckReason.AUTONOMOUS_STUCK,
            prNumber: 0,
            prUrl: "",
            context: "autonomous driver stopped after 20 turns without a DONE signal",
          })}
        />
      );
      expect(screen.getByTestId("stuck-item-autonomous-stuck-copy")).toBeInTheDocument();
    });

    it("does not render for a non-autonomous_stuck reason", () => {
      render(<StuckItemDetail item={makeItem({ reason: StuckReason.PR_READY_UNMERGED })} />);
      expect(screen.queryByTestId("stuck-item-autonomous-stuck-copy")).not.toBeInTheDocument();
    });
  });

  // review-gate-stale-session-rework Story 2.2.2 / Task 2.2.2a: the stuck-items
  // UI previously had no click-through path to the item's own detail page at
  // all (an inline accordion only, gap affecting all StuckReason values, not
  // just the new rework_blocked_stale one) — this asserts the link this
  // feature adds reaches the item detail route where GateVerdictBox's
  // "Reopen for Revision" button lives.
  describe("StuckItemDetail_should_linkToItemDetail_When_Rendered", () => {
    it("links to /backlog?item=<itemId> for a rework_blocked_stale item", () => {
      render(
        <StuckItemDetail
          item={makeItem({
            itemId: "rework-blocked-item-id",
            reason: StuckReason.REWORK_BLOCKED_STALE,
            prNumber: 0,
            prUrl: "",
            context: "active work session idle 20m0s since last meaningful output",
          })}
        />
      );
      const link = screen.getByTestId("stuck-item-open-detail-link");
      expect(link).toHaveAttribute("href", "/backlog?item=rework-blocked-item-id");
    });

    it("links to /backlog?item=<itemId> regardless of reason (generic fix, not reason-gated)", () => {
      render(<StuckItemDetail item={makeItem({ itemId: "f9fcef32-c27e-434d-b23f-c873c18afa92" })} />);
      const link = screen.getByTestId("stuck-item-open-detail-link");
      expect(link).toHaveAttribute(
        "href",
        "/backlog?item=f9fcef32-c27e-434d-b23f-c873c18afa92"
      );
    });
  });

  describe("StuckItemDetail_should_offerApprovePlanControl_When_ReasonIsPlanNotApprovedAndPlanExists", () => {
    it("renders explanatory copy for plan_not_approved when a plan exists", () => {
      render(
        <StuckItemDetail
          item={makeItem({
            reason: StuckReason.PLAN_NOT_APPROVED,
            prNumber: 0,
            prUrl: "",
            context: "queued item blocked by DequeueNextQueuedItems' planning gate",
            planArtifactsPath: "project_plans/foo/plan.md",
          })}
        />
      );
      expect(screen.getByTestId("stuck-item-plan-not-approved-copy")).toBeInTheDocument();
    });

    it("does not render for a non-plan_not_approved reason", () => {
      render(<StuckItemDetail item={makeItem({ reason: StuckReason.PR_READY_UNMERGED })} />);
      expect(screen.queryByTestId("stuck-item-plan-not-approved-copy")).not.toBeInTheDocument();
    });

    it("does not render the approve button when no handler is provided", () => {
      render(
        <StuckItemDetail
          item={makeItem({ reason: StuckReason.PLAN_NOT_APPROVED, planArtifactsPath: "plan.md" })}
        />
      );
      expect(screen.queryByTestId("stuck-item-approve-plan-form")).not.toBeInTheDocument();
    });

    it("calls onApprovePlan with the item id when 'Approve Plan' is clicked", async () => {
      const onApprovePlan = jest.fn().mockResolvedValue(undefined);
      render(
        <StuckItemDetail
          item={makeItem({
            reason: StuckReason.PLAN_NOT_APPROVED,
            itemId: "item-plan-1",
            planArtifactsPath: "plan.md",
          })}
          onApprovePlan={onApprovePlan}
        />
      );
      fireEvent.click(screen.getByTestId("stuck-item-approve-plan"));

      await waitFor(() => expect(onApprovePlan).toHaveBeenCalledWith("item-plan-1"));
    });

    it("shows the real backend error message when the approve call rejects", async () => {
      const onApprovePlan = jest
        .fn()
        .mockRejectedValue(new Error("no plan artifacts found — run TriggerTriage first"));
      render(
        <StuckItemDetail
          item={makeItem({ reason: StuckReason.PLAN_NOT_APPROVED, planArtifactsPath: "plan.md" })}
          onApprovePlan={onApprovePlan}
        />
      );
      fireEvent.click(screen.getByTestId("stuck-item-approve-plan"));

      await waitFor(() =>
        expect(screen.getByTestId("stuck-item-approve-plan-error").textContent).toBe(
          "no plan artifacts found — run TriggerTriage first"
        )
      );
    });
  });

  describe("StuckItemDetail_should_hideApproveAffordance_When_ReasonIsPlanNotApprovedButNoPlanExists", () => {
    it("does not render the Approve Plan button or form when planArtifactsPath is empty", () => {
      const onApprovePlan = jest.fn().mockResolvedValue(undefined);
      render(
        <StuckItemDetail
          item={makeItem({ reason: StuckReason.PLAN_NOT_APPROVED, planArtifactsPath: "" })}
          onApprovePlan={onApprovePlan}
        />
      );
      expect(screen.queryByTestId("stuck-item-approve-plan-form")).not.toBeInTheDocument();
      expect(screen.queryByTestId("stuck-item-plan-not-approved-copy")).not.toBeInTheDocument();
    });

    it("renders a non-actionable explanatory message instead", () => {
      render(
        <StuckItemDetail
          item={makeItem({ reason: StuckReason.PLAN_NOT_APPROVED, planArtifactsPath: "" })}
        />
      );
      expect(screen.getByTestId("stuck-item-no-action-copy")).toBeInTheDocument();
    });
  });
});
