package services

import (
	"strings"
	"testing"
)

// configSurfaceProofs maps a surface to the test proving it changes runtime
// behavior, or a written exemption. Stories 2.1.1b, 1.2.4a, 1.5.3c, 1.5.8g,
// 1.5.9c, 5.1.2 and 5.1.3 add entries and delete the matching pendingSurfaces
// line in the same change.
var configSurfaceProofs = map[surfaceID]proofEntry{
	"quota.enabled": {
		testName:   "TestConfigSurfaceProof_QuotaEnabled_ChangesQuotaMode",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"quota.assumed_window_token_budget": {
		testName:   "TestConfigSurfaceProof_QuotaAssumedWindowTokenBudget_ChangesQuotaMode",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"quota.default_cost_budget_threshold_usd": {
		testName:   "TestConfigSurfaceProof_QuotaDefaultCostBudgetThresholdUSD_ChangesBudgetWarning",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"flag:backlog:workflow-engine-shadow": {
		testName:   "TestConfigSurfaceProof_WorkflowEngineShadow_ChangesShadowCallsRecorded",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:backlog:workflow-engine-enforce": {
		testName:   "TestConfigSurfaceProof_WorkflowEngineEnforce_ChangesTransitionAnswer",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:backlog:workflow-engine-enforce-agent": {
		testName:   "TestConfigSurfaceProof_WorkflowEngineEnforceAgent_ChangesAgentTransitionAnswer",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"proto.UpdateStageTransitionRequest.id": {
		testName:   "TestConfigSurfaceProof_UpdateStageTransition_ChangesHandlerAnswer",
		entrypoint: "BacklogService.UpdateStageTransition",
	},
	"proto.UpdateStageTransitionRequest.enabled": {
		testName:   "TestConfigSurfaceProof_UpdateStageTransition_ChangesHandlerAnswer",
		entrypoint: "BacklogService.UpdateStageTransition",
	},
	"proto.CreateTransitionGateRequest.transition_id": {
		testName:   "TestConfigSurfaceProof_CreateTransitionGate_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateTransitionGate",
	},
	"proto.CreateTransitionGateRequest.enabled": {
		testName:   "TestConfigSurfaceProof_CreateTransitionGate_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateTransitionGate",
	},
}

// pendingCeiling is len(pendingSurfaces). It only shrinks: lower it in the same
// change that deletes pending lines, so the list cannot grow without a visible
// edit here. It must reach 0 at Story 5.1.3.
const pendingCeiling = 76

// pendingSurfaces lists every surface without a proof yet (ADR-006 ratchet).
// planned:true marks a surface absent from the code until its story lands; the
// entry then keeps failing the "neither registered nor pending" rule only if it
// is deleted before a proof exists.
var pendingSurfaces = buildPendingSurfaces()

func buildPendingSurfaces() map[surfaceID]pendingSurface {
	p := map[surfaceID]pendingSurface{}
	add := func(story string, planned bool, ids ...surfaceID) {
		for _, id := range ids {
			p[id] = pendingSurface{story: story, planned: planned}
		}
	}
	add("5.1.2a", false,
		"quota.consecutive_ticks_to_pause", "quota.consecutive_ticks_to_resume",
		"quota.foreground_throttle_delay_seconds", "quota.manual_override_grace_minutes",
		"quota.pause_below_headroom_pct", "quota.rate_limit_window_minutes",
		"quota.resume_margin_pct")
	add("5.1.2b", true, "flag:backlog:workflow-engine-enforce-system") // Story 1.5.8
	add("5.1.2b", false, "proto.StuckBacklogItem.allow_auto_merge")
	add("5.1.3", false,
		"flag:agent_wake_triggers", "flag:askuserquestion_structured", "flag:background_activity_feed",
		"flag:notification_inline_actions",                                      // arrived from main after the ratchet was set; backfilled with the rest
		"flag:files:delta-write", "flag:files:edit-large", "flag:files:editing", // arrived from main (#1107); backfilled with the rest
		"flag:files:merge-phone", "flag:files:range-read", "flag:files:viewer-v2",
		"flag:autonomous_stall_nudge_enabled", "flag:backlog", "flag:backlog:completion-reviewer",
		"flag:backlog:conversation-view", "flag:backlog:sdd-default-pipeline", "flag:browser-passthrough",
		"flag:changelog", "flag:changelog:update-check", "flag:cross_host_claim_dedup",
		"flag:diagnose_nudge_enabled", "flag:github:graphql-pr-info", "flag:github:priority-admission-control",
		"flag:handoff-summary", "flag:hidden_session_gate", "flag:hidden_session_readonly_guards",
		"flag:hidden_session_reply", "flag:pi-support", "flag:programs:cli-flag-probe",
		"flag:review:block-approval-on-ci-failure", "flag:session:workspace-peers-nudge",
		"flag:terminal:app-scrollback-forwarding:claude", "flag:terminal:resync-batching",
		"flag:terminal:resync-compression", "flag:terminal:resync-correlation-id",
		"flag:terminal:resync-exec-gate-fast-lane", "flag:terminal:resync-skip-stale-dimension-slowpath",
		"flag:terminal:resync-stagger", "flag:terminal:resync-visibility-scope", "flag:terminal_write_lease",
		"flag:triage_guidance_halt", "flag:unfinished:mmap-index", "flag:vcs:worktree-change-detection",
		"proto.CreateStageRequest.description", "proto.CreateStageRequest.enabled",
		"proto.CreateStageRequest.is_entry", "proto.CreateStageRequest.is_terminal",
		"proto.CreateStageRequest.name", "proto.CreateStageRequest.slug",
		"proto.CreateStageTransitionRequest.enabled", "proto.CreateStageTransitionRequest.from_stage_slug",
		"proto.CreateStageTransitionRequest.to_stage_slug", "proto.CreateTransitionGateRequest.config",
		"proto.CreateTransitionGateRequest.kind",
		"proto.CreateTransitionGateRequest.order_index", "proto.CreateTransitionGateRequest.stateful",
		"proto.UpdateStageRequest.description",
		"proto.UpdateStageRequest.enabled", "proto.UpdateStageRequest.id", "proto.UpdateStageRequest.is_entry",
		"proto.UpdateStageRequest.is_terminal", "proto.UpdateStageRequest.name",
		"proto.UpdateTransitionGateRequest.config", "proto.UpdateTransitionGateRequest.enabled",
		"proto.UpdateTransitionGateRequest.id", "proto.UpdateTransitionGateRequest.kind",
		"proto.UpdateTransitionGateRequest.order_index", "proto.UpdateTransitionGateRequest.stateful")
	return p
}

func TestConfigSurfaceProofs_EveryRegisteredSurfaceHasEntry_WhenFieldsEnumerated(t *testing.T) {
	t.Parallel()
	if got := len(pendingSurfaces); got != pendingCeiling {
		t.Errorf("pendingSurfaces has %d entries, pendingCeiling is %d: delete pending lines and lower the ceiling together; never add", got, pendingCeiling)
	}
	errs := validateConfigSurfaces(allConfigSurfaces(), configSurfaceProofs, pendingSurfaces, testFuncBodies(t, "."))
	if len(errs) > 0 {
		t.Fatalf("config surface proof violations:\n%s", strings.Join(errs, "\n"))
	}
}
