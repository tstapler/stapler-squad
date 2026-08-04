import { getAvailableActions, KNOWN_STATUS_MEMBERSHIP } from "./itemActions";
import type { BacklogItem, KnownBacklogStatus } from "@/lib/hooks/useBacklogService";

function makeItem(overrides: Partial<BacklogItem> & { id: string }): BacklogItem {
  return {
    title: overrides.id,
    status: "ready",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

describe("getAvailableActions", () => {
  describe("idea", () => {
    it("exposes mark_ready and trigger_triage, no gate actions, no backward transitions", () => {
      const { actions, isGatedOnPlanApproval } = getAvailableActions(makeItem({ id: "a", status: "idea" }));
      expect(actions.has("mark_ready")).toBe(true);
      expect(actions.has("trigger_triage")).toBe(true);
      expect(actions.has("approve_plan")).toBe(false);
      expect(actions.has("retry_triage")).toBe(false);
      expect(actions.has("send_back_idea")).toBe(false);
      expect(isGatedOnPlanApproval).toBe(true);
    });
  });

  describe("refining", () => {
    it("exposes only the send_back_idea backward transition", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "refining" }));
      expect(actions.has("send_back_idea")).toBe(true);
      expect(actions.has("send_back_ready")).toBe(false);
      expect(actions.has("mark_ready")).toBe(false);
    });
  });

  describe("ready", () => {
    it("exposes trigger_triage, spawn_session, spawn_session_autonomous always", () => {
      const { actions } = getAvailableActions(
        makeItem({ id: "a", status: "ready", skipPlanning: true, planApproved: false })
      );
      expect(actions.has("trigger_triage")).toBe(true);
      expect(actions.has("spawn_session")).toBe(true);
      expect(actions.has("spawn_session_autonomous")).toBe(true);
      expect(actions.has("send_back_idea")).toBe(true);
      expect(actions.has("send_back_ready")).toBe(false); // ready has nothing earlier than idea to "back to ready" from
    });

    it("exposes approve_plan when gated and a plan exists — the pre-existing, correct case", () => {
      const { actions, isGatedOnPlanApproval, hasPlan } = getAvailableActions(
        makeItem({ id: "a", status: "ready", skipPlanning: false, planApproved: false, planArtifactsPath: "/plans/a" })
      );
      expect(isGatedOnPlanApproval).toBe(true);
      expect(hasPlan).toBe(true);
      expect(actions.has("approve_plan")).toBe(true);
      expect(actions.has("retry_triage")).toBe(false);
    });

    it("exposes retry_triage instead of approve_plan when gated, no plan exists, and the latest triage session failed — the be676dab dead-end fix", () => {
      const { actions, isGatedOnPlanApproval, hasPlan } = getAvailableActions(
        makeItem({
          id: "a",
          status: "ready",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: undefined,
          triageStatus: "failed",
        })
      );
      expect(isGatedOnPlanApproval).toBe(true);
      expect(hasPlan).toBe(false);
      expect(actions.has("retry_triage")).toBe(true);
      expect(actions.has("approve_plan")).toBe(false);
    });

    // MAJOR finding from PR #322 review: a ready item can be reached via "Mark
    // Ready" straight from idea with no plan and no triage session EVER having
    // run (triageStatus undefined, not "failed") — gated + no-plan alone is not
    // evidence of a failed retry. Before this fix, retry_triage rendered
    // unconditionally whenever gated+no-plan, duplicating the always-present
    // trigger_triage button and dispatching the identical underlying call for a
    // status where nothing had actually failed yet.
    it("does NOT expose retry_triage for a gated, no-plan ready item that was never triaged (only trigger_triage, no duplication)", () => {
      const { actions } = getAvailableActions(
        makeItem({
          id: "a",
          status: "ready",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: undefined,
          triageStatus: undefined,
        })
      );
      expect(actions.has("retry_triage")).toBe(false);
      expect(actions.has("approve_plan")).toBe(false);
      expect(actions.has("trigger_triage")).toBe(true);
    });

    it("does NOT expose retry_triage for a gated, no-plan ready item whose triage is still running or already completed", () => {
      for (const triageStatus of ["running", "completed"] as const) {
        const { actions } = getAvailableActions(
          makeItem({
            id: "a",
            status: "ready",
            skipPlanning: false,
            planApproved: false,
            planArtifactsPath: undefined,
            triageStatus,
          })
        );
        expect(actions.has("retry_triage")).toBe(false);
      }
    });

    it("exposes neither approve_plan nor retry_triage when skipPlanning is true, regardless of plan presence", () => {
      const { actions, isGatedOnPlanApproval } = getAvailableActions(
        makeItem({ id: "a", status: "ready", skipPlanning: true, planApproved: false, planArtifactsPath: undefined })
      );
      expect(isGatedOnPlanApproval).toBe(false);
      expect(actions.has("approve_plan")).toBe(false);
      expect(actions.has("retry_triage")).toBe(false);
    });

    it("exposes neither approve_plan nor retry_triage when planApproved is already true", () => {
      const { actions, isGatedOnPlanApproval } = getAvailableActions(
        makeItem({ id: "a", status: "ready", skipPlanning: false, planApproved: true, planArtifactsPath: undefined })
      );
      expect(isGatedOnPlanApproval).toBe(false);
      expect(actions.has("approve_plan")).toBe(false);
      expect(actions.has("retry_triage")).toBe(false);
    });
  });

  describe("queued", () => {
    it("exposes approve_plan when gated and a plan exists", () => {
      const { actions } = getAvailableActions(
        makeItem({ id: "a", status: "queued", skipPlanning: false, planApproved: false, planArtifactsPath: "/plans/a" })
      );
      expect(actions.has("approve_plan")).toBe(true);
      expect(actions.has("retry_triage")).toBe(false);
    });

    it("exposes retry_triage instead of approve_plan when gated, no plan exists, and the latest triage session failed — the be676dab dead-end fix", () => {
      const { actions } = getAvailableActions(
        makeItem({
          id: "a",
          status: "queued",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: undefined,
          triageStatus: "failed",
        })
      );
      expect(actions.has("retry_triage")).toBe(true);
      expect(actions.has("approve_plan")).toBe(false);
    });

    it("does NOT expose retry_triage for a gated, no-plan queued item with no failed-triage evidence", () => {
      const { actions } = getAvailableActions(
        makeItem({
          id: "a",
          status: "queued",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: undefined,
          triageStatus: undefined,
        })
      );
      expect(actions.has("retry_triage")).toBe(false);
      expect(actions.has("approve_plan")).toBe(false);
    });

    it("exposes no gate action when skipPlanning is true", () => {
      const { actions } = getAvailableActions(
        makeItem({ id: "a", status: "queued", skipPlanning: true, planApproved: false, planArtifactsPath: undefined })
      );
      expect(actions.has("approve_plan")).toBe(false);
      expect(actions.has("retry_triage")).toBe(false);
    });

    it("has no backward transitions today (queued is not in either send-back set)", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "queued" }));
      expect(actions.has("send_back_idea")).toBe(false);
      expect(actions.has("send_back_ready")).toBe(false);
    });
  });

  describe("in_progress", () => {
    it("exposes view_session and restart_session only when a linked session exists", () => {
      const withSession = getAvailableActions(
        makeItem({
          id: "a",
          status: "in_progress",
          linkedSessions: [{ entityId: "e1", sessionId: "s1", role: "work", estimatedCostUsd: 0 }],
        })
      );
      expect(withSession.actions.has("view_session")).toBe(true);
      expect(withSession.actions.has("restart_session")).toBe(true);

      const withoutSession = getAvailableActions(makeItem({ id: "a", status: "in_progress", linkedSessions: [] }));
      expect(withoutSession.actions.has("view_session")).toBe(false);
      expect(withoutSession.actions.has("restart_session")).toBe(false);
    });

    it("exposes both backward transitions", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "in_progress" }));
      expect(actions.has("send_back_idea")).toBe(true);
      expect(actions.has("send_back_ready")).toBe(true);
    });
  });

  describe("review", () => {
    it("exposes ship_pr only when there is no PR yet", () => {
      const noPr = getAvailableActions(makeItem({ id: "a", status: "review", prUrl: undefined }));
      expect(noPr.actions.has("ship_pr")).toBe(true);

      const withPr = getAvailableActions(makeItem({ id: "a", status: "review", prUrl: "https://github.com/x/y/pull/1" }));
      expect(withPr.actions.has("ship_pr")).toBe(false);
    });

    it("always exposes override_done, re_review, manual_review, restart_session", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "review" }));
      expect(actions.has("override_done")).toBe(true);
      expect(actions.has("re_review")).toBe(true);
      expect(actions.has("manual_review")).toBe(true);
      expect(actions.has("restart_session")).toBe(true);
    });

    it("exposes link_existing_pr only when there is no PR yet, mirroring ship_pr", () => {
      const noPr = getAvailableActions(makeItem({ id: "a", status: "review", prUrl: undefined }));
      expect(noPr.actions.has("link_existing_pr")).toBe(true);

      const withPr = getAvailableActions(makeItem({ id: "a", status: "review", prUrl: "https://github.com/x/y/pull/1" }));
      expect(withPr.actions.has("link_existing_pr")).toBe(false);
    });
  });

  describe("pr_pending", () => {
    it("has no status-specific primary action, only backward transitions", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "pr_pending" }));
      expect(actions.has("send_back_idea")).toBe(true);
      expect(actions.has("send_back_ready")).toBe(true);
    });
  });

  describe("done", () => {
    it("exposes archive and reopen, plus both backward transitions", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "done" }));
      expect(actions.has("archive")).toBe(true);
      expect(actions.has("reopen")).toBe(true);
      expect(actions.has("send_back_idea")).toBe(true);
      expect(actions.has("send_back_ready")).toBe(true);
    });
  });

  describe("archived", () => {
    it("exposes only delete and status_override", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "archived" }));
      expect(actions).toEqual(new Set(["delete", "status_override"]));
    });
  });

  describe("unknown status", () => {
    it("exposes only delete and status_override, defensively, for a forward-compatible unknown status string", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "some_future_status" }));
      expect(actions).toEqual(new Set(["delete", "status_override"]));
    });
  });

  it("always exposes delete and status_override regardless of status — the manual escape hatch must survive whatever state an item is stuck in", () => {
    const statuses: KnownBacklogStatus[] = [
      "idea",
      "refining",
      "ready",
      "queued",
      "in_progress",
      "review",
      "pr_pending",
      "done",
      "archived",
    ];
    for (const status of statuses) {
      const { actions } = getAvailableActions(makeItem({ id: "a", status }));
      expect(actions.has("delete")).toBe(true);
      expect(actions.has("status_override")).toBe(true);
    }
  });

  // MAJOR finding from PR #322 review: KNOWN_STATUS_MEMBERSHIP is a second,
  // independent source of truth alongside the KnownBacklogStatus union
  // (useBacklogService.ts) — the `satisfies Record<KnownBacklogStatus, true>`
  // annotation on its declaration is what actually closes the desync gap at
  // compile time (a missing OR extra key is a build error). This test is a
  // cheap runtime tripwire on top of that: it makes the invariant visible in
  // test output, not just in a type-checker error a reviewer might not re-run.
  it("KNOWN_STATUS_MEMBERSHIP's key set matches the full KnownBacklogStatus union with no extras or omissions", () => {
    const expected: KnownBacklogStatus[] = [
      "idea",
      "refining",
      "ready",
      "queued",
      "in_progress",
      "review",
      "pr_pending",
      "done",
      "archived",
    ];
    expect(Object.keys(KNOWN_STATUS_MEMBERSHIP).sort()).toEqual([...expected].sort());
  });
});
