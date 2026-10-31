package services

import (
	"strings"
	"testing"
)

// configSurfaceProofs maps a surface to the test proving it changes runtime
// behavior, or a written exemption. Stories 5.1.2 and 5.1.3 add the remaining
// entries and delete the matching pendingSurfaces line in the same change. The
// four workflow-engine mode flags (Task 1.5.9c) are proved through the
// production per-class reader in backlog_gate_classes_test.go.
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
	"quota.pause_below_headroom_pct": {
		testName:   "TestConfigSurfaceProof_QuotaPauseBelowHeadroomPct_ChangesPauseDecision",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"quota.consecutive_ticks_to_pause": {
		testName:   "TestConfigSurfaceProof_QuotaConsecutiveTicksToPause_ChangesPauseTiming",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"quota.consecutive_ticks_to_resume": {
		testName:   "TestConfigSurfaceProof_QuotaConsecutiveTicksToResume_ChangesResumeTiming",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"quota.resume_margin_pct": {
		testName:   "TestConfigSurfaceProof_QuotaResumeMarginPct_ChangesResumeDecision",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"quota.rate_limit_window_minutes": {
		testName:   "TestConfigSurfaceProof_QuotaRateLimitWindowMinutes_ChangesHardOverride",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"quota.foreground_throttle_delay_seconds": {
		testName:   "TestConfigSurfaceProof_QuotaForegroundThrottleDelaySeconds_ChangesThrottleDeadline",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"quota.manual_override_grace_minutes": {
		testName:   "TestConfigSurfaceProof_QuotaManualOverrideGraceMinutes_ChangesNotifyCooldownBypass",
		entrypoint: "QuotaSettingsService.UpdateQuotaConfig",
	},
	"proto.StuckBacklogItem.allow_auto_merge": {
		testName:   "TestConfigSurfaceProof_StuckAllowAutoMerge_ChangesListedField",
		entrypoint: "BacklogService.ListStuckBacklogItems",
	},
	"flag:backlog:workflow-engine-shadow": {
		testName:   "TestConfigSurfaceProof_WorkflowEngineShadowFlag_ChangesShadowCallsForEveryClass",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:backlog:workflow-engine-enforce": {
		testName:   "TestConfigSurfaceProof_WorkflowEngineEnforceFlag_ChangesOperatorAnswerAndNoOtherClass",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:backlog:workflow-engine-enforce-agent": {
		testName:   "TestConfigSurfaceProof_WorkflowEngineEnforceAgent_ChangesAgentTransitionAnswer",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:backlog:workflow-engine-enforce-system": {
		testName:   "TestConfigSurfaceProof_WorkflowEngineEnforceSystem_ChangesSystemTransitionAnswer",
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
	"proto.CreateTransitionGateRequest.stateful": {
		testName:   "TestConfigSurfaceProof_CreateTransitionGate_Stateful_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateTransitionGate",
	},
	"proto.UpdateTransitionGateRequest.stateful": {
		testName:   "TestConfigSurfaceProof_UpdateTransitionGate_Stateful_ChangesHandlerAnswer",
		entrypoint: "BacklogService.UpdateTransitionGate",
	},
	"proto.CreateTransitionGateRequest.transition_id": {
		testName:   "TestConfigSurfaceProof_CreateTransitionGate_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateTransitionGate",
	},
	"proto.CreateTransitionGateRequest.enabled": {
		testName:   "TestConfigSurfaceProof_CreateTransitionGate_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateTransitionGate",
	},
	"proto.CreateStageRequest.description": {
		exempt: "display-only: stored and echoed by GetStage/ListStages (stageToProto) and shown in Stages settings; no Go code outside the CRUD handlers reads it (grep of non-test session/ and server/ finds only the repository copy and stageToProto)",
	},
	"proto.CreateStageRequest.enabled": {
		testName:   "TestConfigSurfaceProof_CreateStage_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateStage",
	},
	"proto.CreateStageRequest.is_entry": {
		testName:   "TestConfigSurfaceProof_CreateStage_IsEntryAndIsTerminal_ChangeGraphValidation",
		entrypoint: "BacklogService.CreateStage",
	},
	"proto.CreateStageRequest.is_terminal": {
		testName:   "TestConfigSurfaceProof_CreateStage_IsEntryAndIsTerminal_ChangeGraphValidation",
		entrypoint: "BacklogService.CreateStage",
	},
	"proto.CreateStageRequest.name": {
		exempt: "display-only: stored and echoed by GetStage/ListStages (stageToProto) and shown in Stages settings; no Go code outside the CRUD handlers reads it (grep of non-test session/ and server/ finds only the repository copy and stageToProto)",
	},
	"proto.CreateStageRequest.slug": {
		testName:   "TestConfigSurfaceProof_CreateStage_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateStage",
	},
	"proto.CreateStageTransitionRequest.enabled": {
		testName:   "TestConfigSurfaceProof_CreateStageTransition_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateStageTransition",
	},
	"proto.CreateStageTransitionRequest.from_stage_slug": {
		testName:   "TestConfigSurfaceProof_CreateStageTransition_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateStageTransition",
	},
	"proto.CreateStageTransitionRequest.to_stage_slug": {
		testName:   "TestConfigSurfaceProof_CreateStageTransition_ChangesHandlerAnswer",
		entrypoint: "BacklogService.CreateStageTransition",
	},
	"proto.CreateTransitionGateRequest.config": {
		testName:   "TestConfigSurfaceProof_CreateTransitionGate_KindConfigAndOrder_ChangeHandlerAnswer",
		entrypoint: "BacklogService.CreateTransitionGate",
	},
	"proto.CreateTransitionGateRequest.kind": {
		testName:   "TestConfigSurfaceProof_CreateTransitionGate_KindConfigAndOrder_ChangeHandlerAnswer",
		entrypoint: "BacklogService.CreateTransitionGate",
	},
	"proto.CreateTransitionGateRequest.order_index": {
		testName:   "TestConfigSurfaceProof_CreateTransitionGate_KindConfigAndOrder_ChangeHandlerAnswer",
		entrypoint: "BacklogService.CreateTransitionGate",
	},
	"proto.UpdateStageRequest.description": {
		exempt: "display-only: stored and echoed by GetStage/ListStages (stageToProto) and shown in Stages settings; no Go code outside the CRUD handlers reads it (grep of non-test session/ and server/ finds only the repository copy and stageToProto)",
	},
	"proto.UpdateStageRequest.enabled": {
		testName:   "TestConfigSurfaceProof_UpdateStage_ChangesHandlerAnswer",
		entrypoint: "BacklogService.UpdateStage",
	},
	"proto.UpdateStageRequest.id": {
		testName:   "TestConfigSurfaceProof_UpdateStage_ChangesHandlerAnswer",
		entrypoint: "BacklogService.UpdateStage",
	},
	"proto.UpdateStageRequest.is_entry": {
		testName:   "TestConfigSurfaceProof_UpdateStage_IsEntryAndIsTerminal_ChangeGraphValidation",
		entrypoint: "BacklogService.UpdateStage",
	},
	"proto.UpdateStageRequest.is_terminal": {
		testName:   "TestConfigSurfaceProof_UpdateStage_IsEntryAndIsTerminal_ChangeGraphValidation",
		entrypoint: "BacklogService.UpdateStage",
	},
	"proto.UpdateStageRequest.name": {
		exempt: "display-only: stored and echoed by GetStage/ListStages (stageToProto) and shown in Stages settings; no Go code outside the CRUD handlers reads it (grep of non-test session/ and server/ finds only the repository copy and stageToProto)",
	},
	"proto.UpdateTransitionGateRequest.config": {
		testName:   "TestConfigSurfaceProof_UpdateTransitionGate_KindConfigAndOrder_ChangeHandlerAnswer",
		entrypoint: "BacklogService.UpdateTransitionGate",
	},
	"proto.UpdateTransitionGateRequest.enabled": {
		testName:   "TestConfigSurfaceProof_UpdateTransitionGate_EnabledAndID_ChangeHandlerAnswer",
		entrypoint: "BacklogService.UpdateTransitionGate",
	},
	"proto.UpdateTransitionGateRequest.id": {
		testName:   "TestConfigSurfaceProof_UpdateTransitionGate_EnabledAndID_ChangeHandlerAnswer",
		entrypoint: "BacklogService.UpdateTransitionGate",
	},
	"proto.UpdateTransitionGateRequest.kind": {
		testName:   "TestConfigSurfaceProof_UpdateTransitionGate_KindConfigAndOrder_ChangeHandlerAnswer",
		entrypoint: "BacklogService.UpdateTransitionGate",
	},
	"proto.UpdateTransitionGateRequest.order_index": {
		testName:   "TestConfigSurfaceProof_UpdateTransitionGate_KindConfigAndOrder_ChangeHandlerAnswer",
		entrypoint: "BacklogService.UpdateTransitionGate",
	},
	"flag:agent_wake_triggers": {
		testName:   "TestConfigSurfaceProof_NotificationActionFlags_ChangeEffectiveFlag",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:askuserquestion_structured": {
		testName:   "TestConfigSurfaceProof_NotificationActionFlags_ChangeEffectiveFlag",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:attention_ack_bypass_fix": {
		testName:   "TestConfigSurfaceProof_Should_IncludeAllSixAttentionFlags_When_Run",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:attention_idle_clock_output": {
		testName:   "TestConfigSurfaceProof_Should_IncludeAllSixAttentionFlags_When_Run",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:attention_inbox_consolidation": {
		testName:   "TestConfigSurfaceProof_Should_IncludeAllSixAttentionFlags_When_Run",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:attention_prose_question_detection": {
		testName:   "TestConfigSurfaceProof_Should_IncludeAllSixAttentionFlags_When_Run",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:attention_prose_question_llm_tiebreak": {
		testName:   "TestConfigSurfaceProof_Should_IncludeAllSixAttentionFlags_When_Run",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:autonomous_stall_nudge_enabled": {
		testName:   "TestConfigSurfaceProof_AutonomyNudgeFlags_ChangeEffectiveReaders",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:background_activity_feed": {
		testName:   "TestConfigSurfaceProof_NotificationActionFlags_ChangeEffectiveFlag",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:backlog": {
		testName:   "TestConfigSurfaceProof_BacklogFlag_ChangesBacklogRPCAnswer",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:backlog:conversation-view": {
		exempt: "client-only: no Go reader; the web client gates the session monitor's conversation view on it (web-app/src/components/backlog/SessionMonitor.tsx:32, pinned by SessionMonitor_should_notCallConversationRpcOrShowHistory_When_conversationViewFlagOff in web-app/src/components/backlog/__tests__/SessionMonitor.test.tsx), and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:backlog:completion-reviewer": {
		testName:   "TestConfigSurfaceProof_CompletionReviewer_ChangesSubscriberGate",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:backlog-review-results-panel": {
		exempt: "client-only: no Go reader; the web client gates the formatted review verdict panel on it (web-app/src/components/backlog/ReviewVerdictPanel.tsx:10), and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:backlog-triage-results-panel": {
		exempt: "client-only: no Go reader; the web client gates the formatted triage results panel on it (web-app/src/components/backlog/TriageResultPanel.tsx:10, web-app/src/components/backlog/detail/PlanningSection.tsx:19), and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:backlog:sdd-default-pipeline": {
		testName:   "TestConfigSurfaceProof_SDDDefaultPipeline_ChangesNewItemDefault",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:browser-passthrough": {
		testName:   "TestConfigSurfaceProof_BrowserPassthrough_ChangesInstanceManagerGate",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:changelog": {
		testName:   "TestConfigSurfaceProof_ChangelogFlags_ChangeReaders",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:changelog:update-check": {
		testName:   "TestConfigSurfaceProof_ChangelogFlags_ChangeReaders",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:cross_host_claim_dedup": {
		testName:   "TestConfigSurfaceProof_CrossHostClaimDedup_ChangesClaimLookup",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:diagnose_nudge_enabled": {
		testName:   "TestConfigSurfaceProof_AutonomyNudgeFlags_ChangeEffectiveReaders",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:files:delta-write": {
		testName:   "TestConfigSurfaceProof_FilesFlags_ChangeServerSideGates",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:files:edit-large": {
		testName:   "TestConfigSurfaceProof_FilesFlags_ChangeServerSideGates",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:files:editing": {
		testName:   "TestConfigSurfaceProof_FilesFlags_ChangeServerSideGates",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:files:merge-phone": {
		testName:   "TestConfigSurfaceProof_FilesFlags_ChangeServerSideGates",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:files:range-read": {
		testName:   "TestConfigSurfaceProof_FilesFlags_ChangeServerSideGates",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:files:viewer-v2": {
		testName:   "TestConfigSurfaceProof_FilesFlags_ChangeServerSideGates",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:github:graphql-pr-info": {
		testName:   "TestConfigSurfaceProof_GitHubGraphQLPRInfo_ChangesPRInfoDispatch",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:github:priority-admission-control": {
		testName:   "TestConfigSurfaceProof_GitHubPriorityAdmission_ChangesBackgroundAdmission",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:handoff-summary": {
		exempt: "read-only mirror: HandoffSummaryFeatureController.Enable and Disable refuse (feature_flag_service.go), so UpdateFeatureFlag cannot flip it; the real setting is config.json handoff_summary.enabled, read via HandoffSummaryConfig.EnabledOrDefault, which is not a feature_flags surface",
	},
	"flag:hidden_session_gate": {
		testName:   "TestConfigSurfaceProof_HiddenSessionGate_ChangesDeliveryGateCache",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:hidden_session_readonly_guards": {
		testName:   "TestConfigSurfaceProof_HiddenSessionReadonlyGuards_ChangesWriteToHiddenSession",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:hidden_session_reply": {
		testName:   "TestConfigSurfaceProof_HiddenSessionReply_ChangesReplyOutcome",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:notification_inline_actions": {
		testName:   "TestConfigSurfaceProof_NotificationActionFlags_ChangeEffectiveFlag",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:pi-support": {
		testName:   "TestConfigSurfaceProof_PiSupport_ChangesPiExtensionHook",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:github_issue_progress_comments": {
		testName:   "TestConfigSurfaceProof_GitHubIssueProgressComments_ChangesGate",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:programs:cli-flag-probe": {
		testName:   "TestConfigSurfaceProof_ProgramCLIFlagProbe_ChangesProbeGate",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:request_coalescing": {
		exempt: "client-only: no Go reader; the web client reads it through flagStore (web-app/src/lib/features/flagStore.ts, preloadFlagValue pins the pre-load default to ON), covered by web-app/src/lib/features/flagStore.test.ts `flagStore_should_UseFixedOnConstantForRequestCoalescing_And_UndefinedForUnlistedFlags` and `flagStore_should_IssueOneGetFeatureFlags_When_ProviderAnd5HookConsumersMountTogether`, and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:unary_over_ws": {
		exempt: "client-only: no Go reader; the web client gates unary RPCs over WebSocket on it (client reader lands with the unary-over-WS transport story via flagStore), and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:unary_over_ws_remote": {
		exempt: "client-only: no Go reader; the web client gates unary RPCs over WebSocket on remote-access origins on it (client reader lands with the unary-over-WS transport story via flagStore), and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:terminal_ws_pool": {
		exempt: "client-only: no Go reader; the web client gates the shared terminal socket pool on it (web-app/src/lib/transport/terminal-socket.ts via flagStore), and the server only persists and echoes the value through GetFeatureFlags; the /api/ws/terminal-mux endpoint is always mounted (advertised size 0 makes it answer 404) and covered by ws_terminal_mux_test.go",
	},
	"flag:batch_unary": {
		exempt: "client-only: no Go reader; the web client gates request batching on it (web-app/src/lib/api/batchTransport.ts via flagStore), and the server only persists and echoes the value through GetFeatureFlags; the batch endpoint itself is always mounted and deny-by-default per call",
	},
	"flag:window_keep_alive": {
		exempt: "client-only: no Go reader; the web client gates window keep-alive on it (client reader lands with the window keep-alive story via flagStore), and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:review:block-approval-on-ci-failure": {
		testName:   "TestConfigSurfaceProof_BlockApprovalOnCIFailure_ChangesResolveApproval",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:session_event_log": {
		testName:   "TestConfigSurfaceProof_Should_IncludeAllSixAttentionFlags_When_Run",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:session:workspace-peers-nudge": {
		testName:   "TestConfigSurfaceProof_WorkspacePeersNudge_ChangesPromptBlock",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:terminal:app-scrollback-forwarding:claude": {
		testName:   "TestConfigSurfaceProof_TerminalAppScrollbackForwardingClaude_ChangesScrollbackPath",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:terminal:resync-batching": {
		exempt: "client-only: no Go reader; the web client gates batched resync on it (web-app/src/components/sessions/SessionDetailView.tsx:399), and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:terminal:resync-compression": {
		testName:   "TestConfigSurfaceProof_TerminalResyncCompression_ChangesEnvelopeFlag",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:terminal:resync-correlation-id": {
		testName:   "TestConfigSurfaceProof_TerminalResyncFlags_ChangeResyncOptions",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:terminal:resync-exec-gate-fast-lane": {
		testName:   "TestConfigSurfaceProof_TerminalResyncFlags_ChangeResyncOptions",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:terminal:resync-skip-stale-dimension-slowpath": {
		testName:   "TestConfigSurfaceProof_TerminalResyncFlags_ChangeResyncOptions",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:terminal:resync-stagger": {
		exempt: "client-only: no Go reader; the web client gates resync staggering on it (web-app/src/components/sessions/SessionDetailView.tsx:398), and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:terminal:resync-visibility-scope": {
		exempt: "client-only: no Go reader; the web client gates resync on it (web-app/src/components/sessions/useVisibilityResync.ts:107 and :273), and the server only persists and echoes the value through GetFeatureFlags",
	},
	"flag:terminal_write_lease": {
		testName:   "TestConfigSurfaceProof_TerminalWriteLease_ChangesLeaseExclusivity",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:triage_guidance_halt": {
		testName:   "TestConfigSurfaceProof_AutonomyNudgeFlags_ChangeEffectiveReaders",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:unfinished:mmap-index": {
		testName:   "TestConfigSurfaceProof_UnfinishedMmapIndex_ChangesLoaderChoice",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
	"flag:vcs:worktree-change-detection": {
		testName:   "TestConfigSurfaceProof_WorktreeChangeDetection_ChangesWorktreeSetup",
		entrypoint: "FeatureFlagService.UpdateFeatureFlag",
	},
}

// pendingCeiling is len(pendingSurfaces). It only shrinks: lower it in the same
// change that deletes pending lines, so the list cannot grow without a visible
// edit here. It reached 0 at Story 5.1.3 and must stay there.
const pendingCeiling = 0

// pendingSurfaces lists every surface without a proof yet (ADR-006 ratchet).
// planned:true marks a surface absent from the code until its story lands; the
// entry then keeps failing the "neither registered nor pending" rule only if it
// is deleted before a proof exists.
var pendingSurfaces = buildPendingSurfaces()

func buildPendingSurfaces() map[surfaceID]pendingSurface {
	return map[surfaceID]pendingSurface{}
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
