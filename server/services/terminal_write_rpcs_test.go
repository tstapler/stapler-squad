//go:build sinkguard

package services

// Check (b) data (plan Story 5.1d, T-RO-17): every RPC of every service in the
// generated session.v1 package is classified here as a terminal write or not.
// A new RPC fails the descriptor test until it is added to one table. "Not a
// terminal write" is the explicit, listed default, so adding an RPC is a
// reviewed decision. Generated from the proto files at the time PR 5u merged.

// terminalWriteRPCs are the RPCs that can type into or restart a pane.
var terminalWriteRPCs = map[string]string{
	"SessionService.WriteToSession":       `types raw input into the agent pane (Story 5.2: refused for a hidden target)`,
	"SessionService.UpdateSession":        `classified by field, see updateSessionFieldClass: steer_message types into the pane, program and auto_approve restart it and type a marker`,
	"SessionService.SwitchWorkspace":      `directory switch types cd into the pane, revision and worktree switches restart the agent (refused for a hidden target)`,
	"SessionService.RestartSession":       `restarts the agent and types a marker into the new pane (refused for a hidden target)`,
	"SessionService.SpawnShell":           `runs an arbitrary command in a new sibling tmux session in the workspace; refused for a hidden target with RestartShell (T-RO-17)`,
	"SessionService.RestartShell":         `relaunches a session shell; refused for a hidden target with the other restart RPCs`,
	"GitHubUserService.NudgeSessionForPR": `types a prompt into the linked session through SteerInstanceGuarded`,
}

// notTerminalWriteReasons records why a lifecycle-looking RPC is not a terminal write.
var notTerminalWriteReasons = map[string]string{
	"SessionService.ResumeHibernatedSession":       `lifecycle transition; types no marker`,
	"SessionService.ResumeCrashedSession":          `lifecycle transition; types no marker`,
	"SessionService.RetrySession":                  `internal retry restart (Restart(false)): no marker typed`,
	"SessionService.RetrySessionCreation":          `re-runs creation of a session that never started`,
	"SessionService.ClearConversationState":        `clears a stored conversation id; types nothing`,
	"SessionService.ResolveApproval":               `resolves a pending hook decision; the answer is returned to the hook, not typed`,
	"SessionService.RunOneShot":                    `headless call, no pane`,
	"SessionService.RunWorkflow":                   `creates sessions through the workflow engine; types into no existing pane`,
	"SessionService.RestartTmuxServer":             `restarts the tmux server process; types into no pane`,
	"HandoffSummaryService.TriggerHandoffSummary":  `restart with a handoff summary goes through Instance.Restart, pinned by the lifecycle caller test`,
	"GuidanceRequestService.AnswerGuidanceRequest": `stores an answer for the triage pipeline; types into no pane`,
	"HeadlessService.RunHeadlessCall":              `headless call, no pane`,
}

// notTerminalWriteRPCs lists every other RPC, per service.
var notTerminalWriteRPCs = map[string][]string{
	"BacklogService": {
		"AddBacklogItemDependency", "ApprovePlan", "ArchiveBacklogItem", "AttachSessionToItem",
		"BulkResetStuckRemediation", "CancelTriage", "CheckCrossHostClaim", "CreateBacklogItem",
		"CreateBacklogItemFromChat", "CreateItemSource", "CreateLivenessDefinition", "CreatePipelineMode",
		"CreateStage", "CreateStageTransition", "CreateTransitionGate", "DeleteBacklogItem", "DeleteItemSource",
		"DeleteLivenessDefinition", "DeletePipelineMode", "DeleteStage", "DeleteStageTransition",
		"DeleteTransitionGate", "DispatchToJules", "GetBacklogItem", "GetBacklogItemCost", "GetBacklogItemDiff",
		"GetBacklogItemShipStatus", "GetLivenessDefinition", "GetPendingGates", "GetPipelineMode",
		"GetSessionBacklogIndex", "GetStage", "GetStageTransition", "GetSyncHistory", "GetTransitionGate",
		"ImportGitHubIssue", "ListBacklogItems", "ListForeignClaims", "ListGitHubIssues", "ListItemSources",
		"ListLivenessDefinitions", "ListPipelineModes", "ListStages", "ListStageTransitions",
		"ListStuckBacklogItems", "ListTransitionGates", "OverrideClaimBlock", "OverrideVerdict",
		"ParseBacklogItemIntent", "PreviewBackwardSyncImpact", "RecordClaimOverride", "RecordGateApproval",
		"RejectPlan", "ResetStuckRemediation", "ResolveClaimDispute", "SearchGitHubRepos", "SnoozeStuckItem",
		"SpawnSessionFromItem", "SubmitManualReview", "SuggestNextItem", "TransitionBacklogItemStatus",
		"TriggerRemediationNow", "TriggerReReview", "TriggerShipPR", "TriggerSync", "TriggerTriage",
		"UnarchiveBacklogItem", "UpdateBacklogItem", "UpdateItemSource", "UpdateLivenessDefinition",
		"UpdatePipelineMode", "UpdateStage", "UpdateStageTransition", "UpdateTransitionGate", "WatchBacklogItems",
	},
	"DiagnosticService": {
		"AssembleDiagnosticBundle", "DispatchDiagnose",
	},
	"GitHubUserService": {
		"AddGitHubAccountFromCLI", "AddGitHubAccountWithToken", "GetGitHubAuthState", "ListGitHubAccounts",
		"ListGitHubCLIHosts", "ListUserPRs", "PollGitHubDeviceAuth", "RevokeGitHubToken", "StartGitHubDeviceAuth",
		"WatchUserPRs",
	},
	"GuidanceRequestService": {
		"AnswerGuidanceRequest", "CreateGuidanceRequest", "GetGuidanceRequest", "ListAllPendingGuidanceRequests",
		"ListGuidanceRequests",
	},
	"HandoffSummaryService": {
		"GetHandoffSummary", "TriggerHandoffSummary",
	},
	"HeadlessService": {
		"RunHeadlessCall",
	},
	"ImportService": {
		"CancelPendingKill", "CommitImportExternalSession", "ConfirmKillExternalSession",
		"PreviewImportExternalSession",
	},
	"InsightsService": {
		"DismissFinding", "GetCompactionStats", "GetContextHistory", "GetInsightsSummary",
		"GetSessionTurnTimeline", "ListSessionsByCeilingTime", "ListSessionTokens", "WatchInsights",
	},
	"LLMBackendService": {
		"GetLLMBackendSettings", "UpdateLLMBackendSettings",
	},
	"RemoteService": {
		"CreateRemote", "DeleteRemote", "GenerateRemoteIdentity", "ListRemotes", "TestRemoteConnection",
		"TrustRemoteHostKey",
	},
	"SessionService": {
		"AcknowledgeError", "AcknowledgeSession", "ArchiveSession", "ArchiveWorkflowSessions",
		"AssignSessionsToProject", "BatchCreateSessions", "BulkUpsertRules", "CancelSessionCreation",
		"ClearConversationState", "ClearNotificationHistory", "ClosePR", "CompleteStreamHubRollbackRehearsal",
		"ConfirmEgressConsent", "CreateCheckpoint", "CreateDebugSnapshot", "CreateProject", "CreatePullRequest",
		"CreateSession", "CreateWorkflow", "DeleteAlias", "DeleteApprovalRule", "DeleteDirectoryRule",
		"DeleteProfile", "DeleteProgramConfig", "DeleteProject", "DeletePromptHistory", "DeleteSession",
		"DeleteShell", "DeleteTaggingRule", "DeleteWorkflow", "DeleteWorkflowFailedSessions", "DraftPullRequest",
		"ExportRules", "FocusWindow", "ForkSession", "GenerateSuggestedRule", "GetApprovalAnalytics",
		"GetBackgroundModels", "GetCallbackConfig", "GetCaptureTap", "GetClaudeConfig", "GetClaudeHistoryDetail",
		"GetClaudeHistoryMessages", "GetConfigFileRules", "GetCurrentDatabase", "GetDeliveryGateStats",
		"GetDetectionEvents", "GetEscapeAnalyticsGlobalSummary", "GetEscapeAnalyticsSummary", "GetFeatureFlags",
		"GetFileContent", "GetHookStatus", "GetJulesConfig", "GetLauncherPresets", "GetLogs",
		"GetNotificationHistory", "GetPRComments", "GetPRInfo", "GetProgramAnalytics", "GetProviderLimits",
		"GetReviewQueue", "GetSession", "GetSessionDefaults", "GetSessionDiff", "GetSlackConfig",
		"GetStreamHubRolloutStatus", "GetTaggingClassifierConfig", "GetTerminalSnapshot", "GetTmuxVersionStatus",
		"GetVCSStatus", "GetWorkspaceInfo", "HibernateSession", "InstallHooks", "ListAliases", "ListApprovalRules",
		"ListBranches", "ListCheckpoints", "ListClaudeConfigs", "ListClaudeHistory", "ListDatabases", "ListErrors",
		"ListFiles", "ListPathCompletions", "ListPendingApprovals", "ListProgramsConfig", "ListProjects",
		"ListPromptHistory", "ListSessions", "ListShells", "ListSlashCommands", "ListTaggingRules",
		"ListTriggerFireEvents", "ListWorkflows", "ListWorkspaceTargets", "ListWorktrees", "LogClientEvents",
		"LogUserInteraction", "MarkNotificationRead", "MergeDatabase", "MergePR", "PinSession", "PostPRComment",
		"PreviewDestinationPath", "ProbeProgram", "PruneHiddenSessionNotifications", "QueryEscapeAnalytics",
		"ReclassifySessionTags", "ReloadClaudeSettingsRules", "RenameSession", "ResolveApproval",
		"ResolveDefaults", "RestartTmuxServer", "ResumeCrashedSession", "ResumeHibernatedSession", "RetrySession",
		"RetrySessionCreation", "RevokeEgressConsent", "RunOneShot", "RunWorkflow", "SaveRulesToConfigFile",
		"SearchClaudeHistory", "SearchFiles", "SendNotification", "SetCaptureTap", "SetStreamHubGlobalOverride",
		"SetStreamHubSessionOverride", "StopShell", "StreamTerminal", "SwitchDatabase",
		"TestJulesConnection", "TestSlackWebhook", "UnarchiveSession", "UnpinSession", "UpdateBackgroundModels",
		"UpdateCallbackConfig", "UpdateClaudeConfig", "UpdateFeatureFlag", "UpdateGlobalDefaults",
		"UpdateJulesConfig", "UpdateProject", "UpdateSlackConfig", "UpdateTaggingClassifierConfig",
		"UpdateWorkflow", "UpsertAlias", "UpsertApprovalRule", "UpsertDirectoryRule", "UpsertProfile",
		"UpsertProgramConfig", "UpsertTaggingRule", "ValidateRules", "WatchReviewQueue", "WatchSessions",
		"WatchWorkflows",
	},
	"SessionSummaryService": {
		"GetSessionSummary", "RegenerateSessionSummary",
	},
	"TymuxRolloutService": {
		"CompleteTymuxRollbackRehearsal", "GetTymuxRolloutStatus", "SetTymuxGlobalOverride",
		"SetTymuxSessionOverride",
	},
	"UnfinishedWorkService": {
		"DismissWorktree", "GetUnfinishedWorkConfig", "GetWorktreeAISummary", "GetWorktreeDiff",
		"ListUnfinishedWork", "QuickCommitPush", "ScanUnfinishedWork", "SnoozeWorktree", "UndismissWorktree",
		"UpdateUnfinishedWorkConfig", "WatchUnfinishedWork",
	},
}

// updateSessionFieldClass classifies UpdateSessionRequest by field: the request
// is a terminal write only through these. A new field fails the test until it
// is classified.
var updateSessionFieldClass = map[string]string{
	"id":                 "not a write: the target",
	"status":             "not a write: pause, stop and resume are lifecycle transitions that type no marker",
	"category":           "not a write: metadata",
	"title":              "not a write: metadata",
	"program":            "WRITE: SwitchProgram restarts an Active pane and types a marker",
	"tags":               "not a write: metadata",
	"working_dir":        "not a write: metadata",
	"rate_limit_enabled": "not a write: a flag read by the controller",
	"pause_reason":       "not a write: metadata",
	"autonomous_mode":    "not a write: starts or stops the driver, which acquires the lease itself",
	"steer_message":      "WRITE: typed into the pane (visible, or the audited O7 path)",
	"note":               "not a write: metadata",
	"auto_approve":       "WRITE: SetAutoApprove restarts an Active pane and types a marker",
}
