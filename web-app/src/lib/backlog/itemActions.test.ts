import { getAvailableActions } from "./itemActions";
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

    it("exposes retry_triage instead of approve_plan when gated but no plan exists — the be676dab dead-end fix", () => {
      const { actions, isGatedOnPlanApproval, hasPlan } = getAvailableActions(
        makeItem({ id: "a", status: "ready", skipPlanning: false, planApproved: false, planArtifactsPath: undefined })
      );
      expect(isGatedOnPlanApproval).toBe(true);
      expect(hasPlan).toBe(false);
      expect(actions.has("retry_triage")).toBe(true);
      expect(actions.has("approve_plan")).toBe(false);
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

    it("exposes retry_triage instead of approve_plan when gated but no plan exists — the be676dab dead-end fix", () => {
      const { actions } = getAvailableActions(
        makeItem({ id: "a", status: "queued", skipPlanning: false, planApproved: false, planArtifactsPath: undefined })
      );
      expect(actions.has("retry_triage")).toBe(true);
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
    it("exposes only delete", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "archived" }));
      expect(actions).toEqual(new Set(["delete"]));
    });
  });

  describe("unknown status", () => {
    it("exposes only delete, defensively, for a forward-compatible unknown status string", () => {
      const { actions } = getAvailableActions(makeItem({ id: "a", status: "some_future_status" }));
      expect(actions).toEqual(new Set(["delete"]));
    });
  });

  it("always exposes delete regardless of status", () => {
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
      expect(getAvailableActions(makeItem({ id: "a", status })).actions.has("delete")).toBe(true);
    }
  });
});
