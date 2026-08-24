package backend

import (
	"bufio"
	"os"
	"regexp"
	"time"
)

// BackendFeature represents a single backend endpoint discovered from proto or marker scanning.
// Proto-derived entries have ProtoFile set; HTTP handler entries have HTTPMethod and HTTPPath set.
type BackendFeature struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Service      string    `json:"service"`
	Method       string    `json:"method"`
	ProtoFile    string    `json:"protoFile,omitempty"`
	HTTPMethod   string    `json:"httpMethod,omitempty"`
	HTTPPath     string    `json:"httpPath,omitempty"`
	MarkerFound  bool      `json:"markerFound"`
	HandlerFile  string    `json:"handlerFile,omitempty"`
	Tested       bool      `json:"tested"`
	TestIDs      []string  `json:"testIds"`
	LastModified time.Time `json:"lastModified"`
}

// methodToID maps proto RPC method names to their canonical feature IDs.
var methodToID = map[string]string{ //nolint:gochecknoglobals
	"CreateSession":             "session:create",
	"GetSession":                "session:get",
	"UpdateSession":             "session:update",
	"DeleteSession":             "session:delete",
	"ListSessions":              "session:list",
	"WatchSessions":             "session:watch",
	"StreamTerminal":            "session:stream-terminal",
	"GetSessionDiff":            "session:get-diff",
	"GetVCSStatus":              "session:get-vcs-status",
	"GetReviewQueue":            "review-queue:get",
	"AcknowledgeSession":        "session:acknowledge",
	"GetLogs":                   "logs:get",
	"WatchReviewQueue":          "review-queue:watch",
	"LogUserInteraction":        "interaction:log",
	"GetClaudeConfig":           "claude-config:get",
	"ListClaudeConfigs":         "claude-config:list",
	"UpdateClaudeConfig":        "claude-config:update",
	"ListClaudeHistory":         "history:list",
	"GetClaudeHistoryDetail":    "history:get-detail",
	"GetClaudeHistoryMessages":  "history:get-messages",
	"SearchClaudeHistory":       "history:search",
	"GetPRInfo":                 "pr:get-info",
	"GetPRComments":             "pr:get-comments",
	"PostPRComment":             "pr:post-comment",
	"MergePR":                   "pr:merge",
	"ClosePR":                   "pr:close",
	"SendNotification":          "notification:send",
	"FocusWindow":               "window:focus",
	"RenameSession":             "session:rename",
	"RestartSession":            "session:restart",
	"GetWorkspaceInfo":          "workspace:get-info",
	"ListWorkspaceTargets":      "workspace:list-targets",
	"SwitchWorkspace":           "workspace:switch",
	"ResolveApproval":           "approval:resolve",
	"ListPendingApprovals":      "approval:list-pending",
	"CreateDebugSnapshot":       "debug:create-snapshot",
	"GetNotificationHistory":    "notification:get-history",
	"MarkNotificationRead":      "notification:mark-read",
	"ClearNotificationHistory":  "notification:clear-history",
	"ListApprovalRules":         "approval:list-rules",
	"UpsertApprovalRule":        "approval:upsert-rule",
	"DeleteApprovalRule":        "approval:delete-rule",
	"ReloadClaudeSettingsRules": "approval:reload-claude-settings-rules",
	"GetApprovalAnalytics":      "approval:get-analytics",
	"ListDatabases":             "database:list",
	"GetCurrentDatabase":        "database:get-current",
	"SwitchDatabase":            "database:switch",
	"MergeDatabase":             "database:merge",
	"CreateCheckpoint":          "checkpoint:create",
	"ListCheckpoints":           "checkpoint:list",
	"ForkSession":               "session:fork",
	"ListFiles":                 "file:list",
	"GetFileContent":            "file:get-content",
	"SearchFiles":               "file:search",
	"ListPathCompletions":       "path:list-completions",
	"ListWorktrees":             "worktree:list",
	// Project management RPCs
	"CreateProject":           "project:create",
	"ListProjects":            "project:list",
	"UpdateProject":           "project:update",
	"DeleteProject":           "project:delete",
	"AssignSessionsToProject": "project:assign-sessions",
	// Prompt history RPCs
	"ListPromptHistory":   "session:list-prompt-history",
	"DeletePromptHistory": "session:delete-prompt-history",
	// Session execution RPCs
	"RunOneShot":          "session:run-one-shot",
	"BatchCreateSessions": "session:batch-create",
	"GetTerminalSnapshot": "session:get-terminal-snapshot",
	"ListBranches":        "session:list-branches",
	// Profile and defaults RPCs
	"UpsertProfile":          "profile:upsert",
	"DeleteProfile":          "profile:delete",
	"GetSessionDefaults":     "defaults:get",
	"UpdateGlobalDefaults":   "defaults:update-global",
	"ResolveDefaults":        "defaults:resolve",
	"PreviewDestinationPath": "session:preview-destination-path",
	// Alias RPCs
	"UpsertAlias": "alias:upsert",
	"DeleteAlias": "alias:delete",
	"ListAliases": "alias:list",
	// Directory rules RPCs
	"UpsertDirectoryRule": "directory-rule:upsert",
	"DeleteDirectoryRule": "directory-rule:delete",
	// Detection RPCs
	"GetDetectionEvents": "session:get-detection-events",
	// Workflow session management RPCs
	"ArchiveWorkflowSessions":      "session:archive-workflow",
	"DeleteWorkflowFailedSessions": "session:delete-workflow-failed",
	// Unfinished work RPCs (UnfinishedWorkService in unfinished.proto)
	"ListUnfinishedWork":         "unfinished:list",
	"WatchUnfinishedWork":        "unfinished:watch",
	"ScanUnfinishedWork":         "unfinished:scan",
	"DismissWorktree":            "unfinished:dismiss",
	"UndismissWorktree":          "unfinished:undismiss",
	"SnoozeWorktree":             "unfinished:snooze",
	"GetWorktreeAISummary":       "unfinished:get-ai-summary",
	"QuickCommitPush":            "unfinished:commit-push",
	"GetUnfinishedWorkConfig":    "unfinished:get-config",
	"UpdateUnfinishedWorkConfig": "unfinished:update-config",
	"GetWorktreeDiff":            "unfinished:get-worktree-diff",
	// Error tracking RPCs
	"LogClientEvents":  "client-event:log",
	"ListErrors":       "error:list",
	"AcknowledgeError": "error:acknowledge",
	// Conversation state RPCs
	"ClearConversationState": "session:clear-conversation-state",
	// Backlog RPCs (BacklogService in backlog.proto)
	"CreateBacklogItem":           "backlog:create-item",
	"CreateBacklogItemFromChat":   "backlog:create-item-from-chat",
	"GetBacklogItem":              "backlog:get-item",
	"ListBacklogItems":            "backlog:list-items",
	"UpdateBacklogItem":           "backlog:update-item",
	"ArchiveBacklogItem":          "backlog:archive-item",
	"UnarchiveBacklogItem":        "backlog:unarchive-item",
	"TransitionBacklogItemStatus": "backlog:transition-status",
	"SpawnSessionFromItem":        "backlog:spawn-session",
	"AttachSessionToItem":         "backlog:attach-session",
	"TriggerTriage":               "backlog:trigger-triage",
	"CancelTriage":                "backlog:cancel-triage",
	"ApprovePlan":                 "backlog:approve-plan",
	"RejectPlan":                  "backlog:reject-plan",
	"SuggestNextItem":             "backlog:suggest-next",
	"OverrideVerdict":             "backlog:override-verdict",
	"TriggerReReview":             "backlog:trigger-re-review",
	"TriggerShipPR":               "backlog:trigger-ship-pr",
	"TriggerSync":                 "backlog:trigger-sync",
	"CreateItemSource":            "backlog:create-source",
	"ListItemSources":             "backlog:list-sources",
	"UpdateItemSource":            "backlog:update-source",
	"DeleteItemSource":            "backlog:delete-source",
	"GetSyncHistory":              "backlog:get-sync-history",
	"PreviewBackwardSyncImpact":   "backlog:preview-backward-sync-impact",
	"GetBacklogItemDiff":          "backlog:get-item-diff",
	"GetBacklogItemCost":          "backlog:get-item-cost",
	"GetBacklogItemShipStatus":    "backlog:get-item-ship-status",
	"GetSessionBacklogIndex":      "backlog:get-session-index",
	"SubmitManualReview":          "backlog:submit-manual-review",
	"ListStuckBacklogItems":       "backlog:list-stuck",
	"SnoozeStuckItem":             "backlog:snooze-stuck",
	"ResetStuckRemediation":       "backlog:reset-stuck-remediation",
	"BulkResetStuckRemediation":   "backlog:bulk-reset-stuck-remediation",
	"TriggerRemediationNow":       "backlog:trigger-remediation-now",
	"CreatePipelineMode":          "backlog:create-pipeline-mode",
	"UpdatePipelineMode":          "backlog:update-pipeline-mode",
	"DeletePipelineMode":          "backlog:delete-pipeline-mode",
	"GetPipelineMode":             "backlog:get-pipeline-mode",
	"ListPipelineModes":           "backlog:list-pipeline-modes",
	"AddBacklogItemDependency":    "backlog:add-item-dependency",
	// GitHub issue import RPCs (BacklogService) - mapped to the method name
	// itself, not a kebab-case backlog:* id: origin/main already has
	// committed registry files under docs/registry/features/backend/{method
	// name}.json (id: "SearchGitHubRepos" etc.), produced by ScanProto's own
	// fallback (id = method) from before these existed in methodToID at all.
	// A kebab-case id here would relocate those files to a new path/id on
	// every PR that touches methodToID, which register-generate's own git
	// diff (and the "Check new RPCs have tests" CI gate, which diffs
	// registry files against origin/main) would then flag as a brand new,
	// untested RPC - even though SearchGitHubRepos/ListGitHubIssues already
	// have real tests committed upstream (server/services/backlog_github_rpc_test.go).
	// Matching the existing fallback id keeps the generated file identical.
	"SearchGitHubRepos": "SearchGitHubRepos",
	"ListGitHubIssues":  "ListGitHubIssues",
	"ImportGitHubIssue": "ImportGitHubIssue",
	// Launcher presets RPCs
	"GetLauncherPresets": "launcher_presets:get",
	// Session lifecycle RPCs
	"ArchiveSession":          "session:archive",
	"UnarchiveSession":        "session:unarchive",
	"HibernateSession":        "session:hibernate",
	"ResumeHibernatedSession": "session:resume-hibernated",
	"ResumeCrashedSession":    "session:resume-crashed",
	"WriteToSession":          "session:write",
	// Shell RPCs
	"SpawnShell":   "shell:spawn",
	"DeleteShell":  "shell:delete",
	"ListShells":   "shell:list",
	"RestartShell": "shell:restart",
	"StopShell":    "shell:stop",
	// Slash commands RPCs
	"ListSlashCommands": "slash-command:list",
	// Workflow RPCs
	"CreateWorkflow": "workflow:create",
	"DeleteWorkflow": "workflow:delete",
	"ListWorkflows":  "workflow:list",
	"UpdateWorkflow": "workflow:update",
	"RunWorkflow":    "workflow:run",
	// Trigger fire audit trail RPC (webhook-triggers Epic 1.2, Task 1.2.1d)
	"ListTriggerFireEvents": "workflow:list-trigger-fire-events",
	// Outbound callback config RPCs (webhook-triggers Phase 5, FR7)
	"GetCallbackConfig":    "callback-config:get",
	"UpdateCallbackConfig": "callback-config:update",
	// Stream Hub Rollout RPCs (terminal-multi-connection-streaming Story 3.3)
	"GetStreamHubRolloutStatus":          "stream-hub-rollout:get",
	"CompleteStreamHubRollbackRehearsal": "stream-hub-rollout:complete-rehearsal",
	"SetStreamHubSessionOverride":        "stream-hub-rollout:set-session-override",
	// Approval rules RPCs
	"BulkUpsertRules":       "approval:bulk-upsert-rules",
	"ExportRules":           "approval:export-rules",
	"GenerateSuggestedRule": "approval:generate-suggested-rule",
	"ValidateRules":         "approval:validate-rules",
	// Analytics RPCs
	"GetEscapeAnalyticsSummary":       "analytics:get-escape-summary",
	"GetEscapeAnalyticsGlobalSummary": "analytics:get-escape-global-summary",
	"GetProgramAnalytics":             "analytics:get-program",
	"QueryEscapeAnalytics":            "analytics:query-escape",
	// Feature flags RPCs
	"GetFeatureFlags":   "feature-flag:get",
	"UpdateFeatureFlag": "feature-flag:update",
	// Hooks RPCs
	"GetHookStatus": "hooks:status",
	"InstallHooks":  "hooks:install",
	// GitHub user RPCs
	"ListUserPRs":               "github-user:list-prs",
	"WatchUserPRs":              "github-user:watch-prs",
	"GetGitHubAuthState":        "github-user:get-auth-state",
	"AddGitHubAccountWithToken": "github-user:add-account-with-token",
	"ListGitHubCLIHosts":        "github-user:list-cli-hosts",
	"AddGitHubAccountFromCLI":   "github-user:add-account-from-cli",
	// Provider limits RPCs
	"GetProviderLimits": "session:get-provider-limits",
	// Config file rules RPCs (stub implementations in RulesService)
	"GetConfigFileRules":    "rules:get-config-file",
	"SaveRulesToConfigFile": "rules:save-to-config-file",
	// Backlog item lifecycle RPCs
	"DeleteBacklogItem": "backlog:delete-item",
	// Backlog real-time streaming RPC (backlog-event-driven-updates Epic 1.1/3.1)
	"WatchBacklogItems": "backlog:watch",
	// Import external session RPCs (ImportService in import.proto)
	"PreviewImportExternalSession": "import:preview",
	"CommitImportExternalSession":  "import:commit",
	"ConfirmKillExternalSession":   "import:confirm_kill",
	"CancelPendingKill":            "import:cancel_pending_kill",
	// Slack notification config RPCs
	"GetSlackConfig":    "slack-config:get",
	"UpdateSlackConfig": "slack-config:update",
	"TestSlackWebhook":  "slack-config:test-webhook",
	// PR creation RPCs
	"DraftPullRequest":  "session:draft-pull-request",
	"CreatePullRequest": "session:create-pull-request",
	// Remote (SSH remote workspaces) RPCs (RemoteService in remote.proto).
	// remote.proto was omitted from registry-generate-backend's explicit proto
	// enumeration until this mapping was added (ssh-remote-workspaces Phase 6
	// Epic 6.3, Story 6.3.1) -- these RPCs' // +api: markers in
	// remote_service.go already used these exact kebab-case ids, so the ids
	// here must match verbatim or ScanProto's method-name fallback would
	// produce a second, non-marker-matching id and file.
	"TestRemoteConnection":   "remote:test-connection",
	"TrustRemoteHostKey":     "remote:trust-host-key",
	"GenerateRemoteIdentity": "remote:generate-identity",
	"ListRemotes":            "remote:list",
	"CreateRemote":           "remote:create",
	"DeleteRemote":           "remote:delete",
	// Headless call RPC (HeadlessService in headless.proto). Found via
	// TestMethodToIDCompleteness after that test was switched from a hardcoded proto file
	// list to globbing proto/session/v1/*.proto -- headless.proto was invisible to every
	// hardcoded proto enumeration in this repo (Makefile's registry-generate-backend,
	// prune-stale-backend.sh, validate-registry.sh, AND this test's own old list), the same
	// bug class ssh-remote-workspaces Phase 6 Epic 6.3 found and fixed for remote.proto.
	// Only the methodToID mapping is added here (this map is what
	// TestMethodToIDCompleteness checks); wiring headless.proto into the Makefile/
	// prune-stale-backend.sh/validate-registry.sh's own scan enumerations so it actually
	// gets a generated per-feature file is a separate, larger followup (a new feature's
	// registry entries + testIds, not a completeness-test fix) -- out of scope here.
	"RunHeadlessCall": "headless:run-call",
	// Handoff summary RPCs (HandoffSummaryService in handoff_summary.proto). Added to fix
	// TestMethodToIDCompleteness, which was failing on main independent of any specific PR --
	// the handoff-summary feature (#612 and its stack) never ran make registry-generate.
	"GetHandoffSummary":     "handoff-summary:get",
	"TriggerHandoffSummary": "handoff-summary:trigger",
}

// rpcPattern matches lines like:   rpc MethodName(  (indented or not)
var rpcPattern = regexp.MustCompile(`^\s*rpc\s+(\w+)\s*\(`)

// servicePattern matches lines like:  service ServiceName {
var servicePattern = regexp.MustCompile(`^\s*service\s+(\w+)\s*\{`)

// ScanProto reads a proto file and returns BackendFeature entries for each RPC method found.
func ScanProto(protoFile string) ([]BackendFeature, error) {
	info, err := os.Stat(protoFile)
	if err != nil {
		return nil, err
	}
	lastMod := info.ModTime()

	f, err := os.Open(protoFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var features []BackendFeature
	currentService := "SessionService"
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if svcMatches := servicePattern.FindStringSubmatch(line); svcMatches != nil {
			currentService = svcMatches[1]
			continue
		}
		matches := rpcPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		method := matches[1]
		id, ok := methodToID[method]
		if !ok {
			// Fallback: use method name as-is
			id = method
		}
		features = append(features, BackendFeature{
			ID:           id,
			Type:         "backend",
			Service:      currentService,
			Method:       method,
			ProtoFile:    protoFile,
			MarkerFound:  false,
			Tested:       false,
			TestIDs:      []string{},
			LastModified: lastMod,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return features, nil
}
