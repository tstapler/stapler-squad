package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// LogUserInteraction logs a user interaction event for audit trail and analytics.
func (s *SessionService) LogUserInteraction(
	ctx context.Context,
	req *connect.Request[sessionv1.LogUserInteractionRequest],
) (*connect.Response[sessionv1.LogUserInteractionResponse], error) {
	return s.reviewQueueSvc.LogUserInteraction(ctx, req)
}

// GetClaudeConfig retrieves a Claude configuration file by name.
func (s *SessionService) GetClaudeConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeConfigRequest],
) (*connect.Response[sessionv1.GetClaudeConfigResponse], error) {
	return s.configSvc.GetClaudeConfig(ctx, req)
}

// ListClaudeConfigs returns all configuration files in the ~/.claude directory.
func (s *SessionService) ListClaudeConfigs(
	ctx context.Context,
	req *connect.Request[sessionv1.ListClaudeConfigsRequest],
) (*connect.Response[sessionv1.ListClaudeConfigsResponse], error) {
	return s.configSvc.ListClaudeConfigs(ctx, req)
}

// UpdateClaudeConfig updates a Claude configuration file with atomic write and backup.
func (s *SessionService) UpdateClaudeConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateClaudeConfigRequest],
) (*connect.Response[sessionv1.UpdateClaudeConfigResponse], error) {
	return s.configSvc.UpdateClaudeConfig(ctx, req)
}

// ListClaudeHistory returns Claude session history entries with optional filtering.
func (s *SessionService) ListClaudeHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ListClaudeHistoryRequest],
) (*connect.Response[sessionv1.ListClaudeHistoryResponse], error) {
	return s.searchSvc.ListClaudeHistory(ctx, req)
}

// GetClaudeHistoryDetail retrieves detailed information for a specific history entry.
func (s *SessionService) GetClaudeHistoryDetail(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeHistoryDetailRequest],
) (*connect.Response[sessionv1.GetClaudeHistoryDetailResponse], error) {
	return s.searchSvc.GetClaudeHistoryDetail(ctx, req)
}

// GetClaudeHistoryMessages retrieves messages from a specific conversation.
func (s *SessionService) GetClaudeHistoryMessages(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeHistoryMessagesRequest],
) (*connect.Response[sessionv1.GetClaudeHistoryMessagesResponse], error) {
	return s.searchSvc.GetClaudeHistoryMessages(ctx, req)
}

// SearchClaudeHistory performs full-text search across Claude conversation history.
// +api: history:search
func (s *SessionService) SearchClaudeHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchClaudeHistoryRequest],
) (*connect.Response[sessionv1.SearchClaudeHistoryResponse], error) {
	return s.searchSvc.SearchClaudeHistory(ctx, req)
}

// GetPRInfo retrieves the latest PR information for a session.
func (s *SessionService) GetPRInfo(
	ctx context.Context,
	req *connect.Request[sessionv1.GetPRInfoRequest],
) (*connect.Response[sessionv1.GetPRInfoResponse], error) {
	return s.githubSvc.GetPRInfo(ctx, req)
}

// DraftPullRequest delegates to prCreationSvc. See PRCreationService for the
// handler implementation.
func (s *SessionService) DraftPullRequest(ctx context.Context, req *connect.Request[sessionv1.DraftPullRequestRequest]) (*connect.Response[sessionv1.DraftPullRequestResponse], error) {
	return s.prCreationSvc.DraftPullRequest(ctx, req)
}

// CreatePullRequest delegates to prCreationSvc. See PRCreationService for the
// handler implementation.
func (s *SessionService) CreatePullRequest(ctx context.Context, req *connect.Request[sessionv1.CreatePullRequestRequest]) (*connect.Response[sessionv1.CreatePullRequestResponse], error) {
	return s.prCreationSvc.CreatePullRequest(ctx, req)
}

// GetPRComments retrieves all comments on the PR for a session.
func (s *SessionService) GetPRComments(
	ctx context.Context,
	req *connect.Request[sessionv1.GetPRCommentsRequest],
) (*connect.Response[sessionv1.GetPRCommentsResponse], error) {
	return s.githubSvc.GetPRComments(ctx, req)
}

// PostPRComment posts a new comment to the PR for a session.
func (s *SessionService) PostPRComment(
	ctx context.Context,
	req *connect.Request[sessionv1.PostPRCommentRequest],
) (*connect.Response[sessionv1.PostPRCommentResponse], error) {
	return s.githubSvc.PostPRComment(ctx, req)
}

// MergePR merges the PR for a session using the specified merge method.
func (s *SessionService) MergePR(
	ctx context.Context,
	req *connect.Request[sessionv1.MergePRRequest],
) (*connect.Response[sessionv1.MergePRResponse], error) {
	return s.githubSvc.MergePR(ctx, req)
}

// ClosePR closes the PR without merging for a session.
func (s *SessionService) ClosePR(
	ctx context.Context,
	req *connect.Request[sessionv1.ClosePRRequest],
) (*connect.Response[sessionv1.ClosePRResponse], error) {
	return s.githubSvc.ClosePR(ctx, req)
}

// SendNotification allows tmux sessions and external Claude processes to send notifications.
func (s *SessionService) SendNotification(
	ctx context.Context,
	req *connect.Request[sessionv1.SendNotificationRequest],
) (*connect.Response[sessionv1.SendNotificationResponse], error) {
	return s.notificationSvc.SendNotification(ctx, req)
}

// FocusWindow activates a window for the specified application.
func (s *SessionService) FocusWindow(
	ctx context.Context,
	req *connect.Request[sessionv1.FocusWindowRequest],
) (*connect.Response[sessionv1.FocusWindowResponse], error) {
	return s.utilitySvc.FocusWindow(ctx, req)
}

// RenameSession changes the title of an existing session.
// Validates that the new title doesn't conflict with existing sessions.
func (s *SessionService) RenameSession(
	ctx context.Context,
	req *connect.Request[sessionv1.RenameSessionRequest],
) (*connect.Response[sessionv1.RenameSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	if req.Msg.NewTitle == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("new title is required"))
	}

	// Use the live poller list for the same reason as UpdateSession: avoid LoadInstances
	// side-effects that can drop sessions and clobber the poller list via SetInstances.
	var instances []*session.Instance
	if s.reviewQueuePoller != nil {
		instances = s.reviewQueuePoller.GetInstances()
	} else {
		var loadErr error
		instances, loadErr = s.loadInstancesWithWiring()
		if loadErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", loadErr))
		}
	}

	// Find the instance to rename
	instance := findInstanceByID(instances, req.Msg.Id)
	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	// Check if new title already exists (if different from current)
	if req.Msg.NewTitle != instance.Title {
		for _, inst := range instances {
			if inst.Title == req.Msg.NewTitle {
				return nil, connect.NewError(connect.CodeAlreadyExists,
					fmt.Errorf("session with title '%s' already exists", req.Msg.NewTitle))
			}
		}
	}

	// Rename the instance
	oldTitle := instance.Title
	if err := instance.Rename(req.Msg.NewTitle); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to rename session: %w", err))
	}

	// Narrow single-row rename, keyed on the pre-mutation title. The generic
	// SaveInstances path looks the DB row up by the (already renamed) in-memory
	// title, misses the still-old-titled row, and falls into a Create fallback that
	// orphans it — using oldTitle as the WHERE key avoids that.
	if err := s.storage.UpdateInstanceMetadata(oldTitle, &req.Msg.NewTitle, nil, nil, nil); err != nil {
		// Try to rollback the rename
		instance.SetTitleDirect(oldTitle)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save renamed instance: %w", err))
	}

	// Publish SessionUpdated event
	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"title"}))

	log.Info("successfully renamed session", "from", oldTitle, "to", req.Msg.NewTitle)

	return connect.NewResponse(&sessionv1.RenameSessionResponse{
		Session: adapters.InstanceToProto(instance, s.workflowNames()),
	}), nil
}

// GetVCSStatus retrieves the current version control status for a session.
func (s *SessionService) GetVCSStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.GetVCSStatusRequest],
) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	return s.workspaceSvc.GetVCSStatus(ctx, req)
}

// GetWorkspaceInfo retrieves VCS and workspace information for a session.
func (s *SessionService) GetWorkspaceInfo(
	ctx context.Context,
	req *connect.Request[sessionv1.GetWorkspaceInfoRequest],
) (*connect.Response[sessionv1.GetWorkspaceInfoResponse], error) {
	return s.workspaceSvc.GetWorkspaceInfo(ctx, req)
}

// ListWorkspaceTargets returns available switch targets for a session.
// +api: workspace:list-targets
func (s *SessionService) ListWorkspaceTargets(
	ctx context.Context,
	req *connect.Request[sessionv1.ListWorkspaceTargetsRequest],
) (*connect.Response[sessionv1.ListWorkspaceTargetsResponse], error) {
	return s.workspaceSvc.ListWorkspaceTargets(ctx, req)
}

// SwitchWorkspace switches a session's workspace to a different branch, revision, or worktree.
// +api: workspace:switch
func (s *SessionService) SwitchWorkspace(
	ctx context.Context,
	req *connect.Request[sessionv1.SwitchWorkspaceRequest],
) (*connect.Response[sessionv1.SwitchWorkspaceResponse], error) {
	return s.workspaceSvc.SwitchWorkspace(ctx, req)
}

// CreateDebugSnapshot captures diagnostic information and writes a JSON file to the log directory.
func (s *SessionService) CreateDebugSnapshot(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateDebugSnapshotRequest],
) (*connect.Response[sessionv1.CreateDebugSnapshotResponse], error) {
	return s.utilitySvc.CreateDebugSnapshot(ctx, req)
}

// GetNotificationHistory returns persisted notification history with optional filtering.
func (s *SessionService) GetNotificationHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.GetNotificationHistoryRequest],
) (*connect.Response[sessionv1.GetNotificationHistoryResponse], error) {
	return s.notificationSvc.GetNotificationHistory(ctx, req)
}

// MarkNotificationRead marks specific notifications as read.
func (s *SessionService) MarkNotificationRead(
	ctx context.Context,
	req *connect.Request[sessionv1.MarkNotificationReadRequest],
) (*connect.Response[sessionv1.MarkNotificationReadResponse], error) {
	return s.notificationSvc.MarkNotificationRead(ctx, req)
}

// ClearNotificationHistory removes notifications from the history.
func (s *SessionService) ClearNotificationHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ClearNotificationHistoryRequest],
) (*connect.Response[sessionv1.ClearNotificationHistoryResponse], error) {
	return s.notificationSvc.ClearNotificationHistory(ctx, req)
}

// ResolveApproval allows the web UI to approve or deny a pending Claude Code tool use request.
func (s *SessionService) ResolveApproval(
	ctx context.Context,
	req *connect.Request[sessionv1.ResolveApprovalRequest],
) (*connect.Response[sessionv1.ResolveApprovalResponse], error) {
	return s.approvalSvc.ResolveApproval(ctx, req)
}

// ListPendingApprovals returns all pending Claude Code tool approval requests.
func (s *SessionService) ListPendingApprovals(
	ctx context.Context,
	req *connect.Request[sessionv1.ListPendingApprovalsRequest],
) (*connect.Response[sessionv1.ListPendingApprovalsResponse], error) {
	return s.approvalSvc.ListPendingApprovals(ctx, req)
}

// ListApprovalRules returns all auto-approval rules (user, seed, and claude-settings).
func (s *SessionService) ListApprovalRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ListApprovalRulesRequest],
) (*connect.Response[sessionv1.ListApprovalRulesResponse], error) {
	return s.rulesSvc.ListApprovalRules(ctx, req)
}

// UpsertApprovalRule creates or updates a user-defined auto-approval rule.
func (s *SessionService) UpsertApprovalRule(
	ctx context.Context,
	req *connect.Request[sessionv1.UpsertApprovalRuleRequest],
) (*connect.Response[sessionv1.UpsertApprovalRuleResponse], error) {
	return s.rulesSvc.UpsertApprovalRule(ctx, req)
}

// DeleteApprovalRule removes a user-defined auto-approval rule by ID.
func (s *SessionService) DeleteApprovalRule(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteApprovalRuleRequest],
) (*connect.Response[sessionv1.DeleteApprovalRuleResponse], error) {
	return s.rulesSvc.DeleteApprovalRule(ctx, req)
}

func (s *SessionService) ReloadClaudeSettingsRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ReloadClaudeSettingsRulesRequest],
) (*connect.Response[sessionv1.ReloadClaudeSettingsRulesResponse], error) {
	return s.rulesSvc.ReloadClaudeSettingsRules(ctx, req)
}

// ListTaggingRules returns all session-tagging rules (user and seed), each with its 7-day
// fire count.
func (s *SessionService) ListTaggingRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ListTaggingRulesRequest],
) (*connect.Response[sessionv1.ListTaggingRulesResponse], error) {
	if s.taggingRulesSvc == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("tagging rules are not available on this server"))
	}
	return s.taggingRulesSvc.ListTaggingRulesRPC(ctx, req)
}

// UpsertTaggingRule creates or updates a user-defined session-tagging rule.
func (s *SessionService) UpsertTaggingRule(
	ctx context.Context,
	req *connect.Request[sessionv1.UpsertTaggingRuleRequest],
) (*connect.Response[sessionv1.UpsertTaggingRuleResponse], error) {
	if s.taggingRulesSvc == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("tagging rules are not available on this server"))
	}
	return s.taggingRulesSvc.UpsertTaggingRuleRPC(ctx, req)
}

// DeleteTaggingRule removes a user-defined session-tagging rule by ID.
func (s *SessionService) DeleteTaggingRule(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteTaggingRuleRequest],
) (*connect.Response[sessionv1.DeleteTaggingRuleResponse], error) {
	if s.taggingRulesSvc == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("tagging rules are not available on this server"))
	}
	return s.taggingRulesSvc.DeleteTaggingRuleRPC(ctx, req)
}

// GetApprovalAnalytics returns aggregated analytics for classification decisions.
func (s *SessionService) GetApprovalAnalytics(
	ctx context.Context,
	req *connect.Request[sessionv1.GetApprovalAnalyticsRequest],
) (*connect.Response[sessionv1.GetApprovalAnalyticsResponse], error) {
	return s.rulesSvc.GetApprovalAnalytics(ctx, req)
}

// GetProgramAnalytics returns drill-down analytics for a single command program.
func (s *SessionService) GetProgramAnalytics(
	ctx context.Context,
	req *connect.Request[sessionv1.GetProgramAnalyticsRequest],
) (*connect.Response[sessionv1.GetProgramAnalyticsResponse], error) {
	return s.rulesSvc.GetProgramAnalytics(ctx, req)
}

// GenerateSuggestedRule asks an AI to propose new auto-approval rules.
func (s *SessionService) GenerateSuggestedRule(
	ctx context.Context,
	req *connect.Request[sessionv1.GenerateSuggestedRuleRequest],
) (*connect.Response[sessionv1.GenerateSuggestedRuleResponse], error) {
	return s.rulesSvc.GenerateSuggestedRule(ctx, req)
}

// ValidateRules parses and validates a YAML rules file without applying it.
func (s *SessionService) ValidateRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ValidateRulesRequest],
) (*connect.Response[sessionv1.ValidateRulesResponse], error) {
	return s.rulesSvc.ValidateRules(ctx, req)
}

// ExportRules serializes user-authored rules to YAML format for download.
func (s *SessionService) ExportRules(
	ctx context.Context,
	req *connect.Request[sessionv1.ExportRulesRequest],
) (*connect.Response[sessionv1.ExportRulesResponse], error) {
	return s.rulesSvc.ExportRules(ctx, req)
}

// BulkUpsertRules creates or updates multiple user-defined rules in one call.
func (s *SessionService) BulkUpsertRules(
	ctx context.Context,
	req *connect.Request[sessionv1.BulkUpsertRulesRequest],
) (*connect.Response[sessionv1.BulkUpsertRulesResponse], error) {
	return s.rulesSvc.BulkUpsertRules(ctx, req)
}

// ListDatabases returns all discovered workspace databases with metadata.
func (s *SessionService) ListDatabases(
	ctx context.Context,
	req *connect.Request[sessionv1.ListDatabasesRequest],
) (*connect.Response[sessionv1.ListDatabasesResponse], error) {
	return s.databaseSvc.ListDatabases(ctx, req)
}

// GetCurrentDatabase returns metadata for the currently active workspace database.
func (s *SessionService) GetCurrentDatabase(
	ctx context.Context,
	req *connect.Request[sessionv1.GetCurrentDatabaseRequest],
) (*connect.Response[sessionv1.GetCurrentDatabaseResponse], error) {
	return s.databaseSvc.GetCurrentDatabase(ctx, req)
}

// SwitchDatabase switches to a different workspace database and restarts the server.
func (s *SessionService) SwitchDatabase(
	ctx context.Context,
	req *connect.Request[sessionv1.SwitchDatabaseRequest],
) (*connect.Response[sessionv1.SwitchDatabaseResponse], error) {
	return s.databaseSvc.SwitchDatabase(ctx, req)
}

// MergeDatabase copies sessions from a source workspace into the current database.
func (s *SessionService) MergeDatabase(
	ctx context.Context,
	req *connect.Request[sessionv1.MergeDatabaseRequest],
) (*connect.Response[sessionv1.MergeDatabaseResponse], error) {
	return s.databaseSvc.MergeDatabase(ctx, req)
}

// SetScrollbackManager wires a scrollback sequence provider for checkpoint creation.
func (s *SessionService) SetScrollbackManager(mgr ScrollbackSequencer) {
	s.scrollbackMgr = mgr
	s.checkpointSvc.SetScrollbackMgr(mgr)
}

// CreateCheckpoint captures the current state of a session as a named bookmark.
func (s *SessionService) CreateCheckpoint(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateCheckpointRequest],
) (*connect.Response[sessionv1.CreateCheckpointResponse], error) {
	return s.checkpointSvc.CreateCheckpoint(ctx, req)
}

// ListCheckpoints returns all checkpoints for the specified session.
func (s *SessionService) ListCheckpoints(
	ctx context.Context,
	req *connect.Request[sessionv1.ListCheckpointsRequest],
) (*connect.Response[sessionv1.ListCheckpointsResponse], error) {
	return s.checkpointSvc.ListCheckpoints(ctx, req)
}

// ForkSession creates a new independent session branched from a checkpoint on an existing session.
func (s *SessionService) ForkSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ForkSessionRequest],
) (*connect.Response[sessionv1.ForkSessionResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.CheckpointId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("checkpoint_id is required"))
	}
	if req.Msg.NewTitle == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("new_title is required"))
	}
	if strings.Contains(req.Msg.NewTitle, "..") || strings.ContainsRune(req.Msg.NewTitle, '/') || strings.ContainsRune(req.Msg.NewTitle, os.PathSeparator) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("new_title must not contain path separators or '..'"))
	}

	src := s.findInstance(req.Msg.SessionId)
	if src == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	if s.findInstance(req.Msg.NewTitle) != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("session with title %q already exists", req.Msg.NewTitle))
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get config dir: %w", err))
	}

	newInst, err := src.ForkFromCheckpoint(req.Msg.CheckpointId, req.Msg.NewTitle, configDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	// ForkFromCheckpoint builds newInst via NewInstance, which never sets
	// MCPServerURL or a provider (unlike CreateWorktreeSession/CreateDirectorySession,
	// which pass MCPServerURL in InstanceOptions) -- without this, a forked
	// session would launch with no --mcp-config at all, same failure class as
	// backlog e6c2a88e.
	newInst.SetMCPServerURLProvider(s.resolveMCPServerURL)

	if err := s.storage.AddInstance(newInst); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("persist forked session: %w", err))
	}

	if s.reviewQueuePoller != nil {
		updatedInstances := append(s.reviewQueuePoller.GetInstances(), newInst)
		s.reviewQueuePoller.SetInstances(updatedInstances)
		log.Info("[ReviewQueue] updated poller instance references after ForkSession", "session", newInst.Title)
	}
	if s.sessionTagPoller != nil {
		s.sessionTagPoller.AddInstance(newInst)
	}

	respProto := adapters.InstanceToProto(newInst, s.workflowNames())

	go func() {
		if startErr := newInst.Start(true); startErr != nil {
			log.Warn("ForkSession: failed to start forked session", "session", newInst.Title, "err", startErr)
			newInst.Status = session.Stopped
			if saveErr := s.storage.SaveInstances(s.allInstances()); saveErr != nil {
				log.Warn("ForkSession: failed to persist Stopped status", "session", newInst.Title, "err", saveErr)
			}
		}
	}()

	s.eventBus.Publish(events.NewSessionCreatedEvent(newInst))

	return connect.NewResponse(&sessionv1.ForkSessionResponse{
		Session: respProto,
	}), nil
}

// ClearConversationState removes the stored Claude conversation UUID from a session
// so that the next Resume starts a fresh conversation instead of attempting --resume
// with a stale or path-mismatched UUID.
func (s *SessionService) ClearConversationState(
	ctx context.Context,
	req *connect.Request[sessionv1.ClearConversationStateRequest],
) (*connect.Response[sessionv1.ClearConversationStateResponse], error) {
	return s.checkpointSvc.ClearConversationState(ctx, req)
}

// ListPathCompletions returns filesystem entries matching the given path prefix.
func (s *SessionService) ListPathCompletions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListPathCompletionsRequest],
) (*connect.Response[sessionv1.ListPathCompletionsResponse], error) {
	return s.pathCompletionSvc.ListPathCompletions(ctx, req)
}

func (s *SessionService) ListSlashCommands(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSlashCommandsRequest],
) (*connect.Response[sessionv1.ListSlashCommandsResponse], error) {
	return s.slashCommandSvc.ListSlashCommands(ctx, req)
}

// ListWorktrees returns the git worktrees for a given repository path.
func (s *SessionService) ListWorktrees(
	ctx context.Context,
	req *connect.Request[sessionv1.ListWorktreesRequest],
) (*connect.Response[sessionv1.ListWorktreesResponse], error) {
	return s.pathCompletionSvc.ListWorktrees(ctx, req)
}

// ListBranches returns the git branches for a given repository path.
// Results are cached per repo path with a 5-minute TTL. ADR-002.
// Delegates to WorkspaceService (Story 1.4).
func (s *SessionService) ListBranches(
	ctx context.Context,
	req *connect.Request[sessionv1.ListBranchesRequest],
) (*connect.Response[sessionv1.ListBranchesResponse], error) {
	return s.workspaceSvc.ListBranches(ctx, req)
}

// findInstance finds an instance by title using the live in-memory poller.
func (s *SessionService) findInstance(id string) *session.Instance {
	if s.reviewQueuePoller != nil {
		if inst := s.reviewQueuePoller.FindInstance(id); inst != nil {
			return inst
		}
	}
	if s.externalDiscovery != nil {
		if inst := s.externalDiscovery.GetSession(id); inst != nil {
			return inst
		}
	}
	return nil
}

// allInstances returns all managed (poller-tracked) live instances.
// External discovery sessions are intentionally excluded: they are persisted via
// the OnSessionAdded/OnSessionRemoved callbacks in dependencies.go, and including
// them here would re-persist deleted external sessions during unrelated operations
// such as CreateCheckpoint or ForkSession.
func (s *SessionService) allInstances() []*session.Instance {
	if s.reviewQueuePoller != nil {
		return s.reviewQueuePoller.GetInstances()
	}
	return nil
}

// ListFiles returns the immediate children of a directory in a session's worktree.
func (s *SessionService) ListFiles(
	ctx context.Context,
	req *connect.Request[sessionv1.ListFilesRequest],
) (*connect.Response[sessionv1.ListFilesResponse], error) {
	return s.fileSvc.ListFiles(ctx, req)
}

// GetFileContent retrieves the text content of a file in a session's worktree.
func (s *SessionService) GetFileContent(
	ctx context.Context,
	req *connect.Request[sessionv1.GetFileContentRequest],
) (*connect.Response[sessionv1.GetFileContentResponse], error) {
	return s.fileSvc.GetFileContent(ctx, req)
}

// ─── Session Defaults delegates ──────────────────────────────────────────────

// GetSessionDefaults returns the full session defaults configuration.
func (s *SessionService) GetSessionDefaults(ctx context.Context, req *connect.Request[sessionv1.GetSessionDefaultsRequest]) (*connect.Response[sessionv1.GetSessionDefaultsResponse], error) {
	return s.defaultsSvc.GetSessionDefaults(ctx, req)
}

// GetLauncherPresets returns the hand-authored launcher presets, freshly read on every call.
func (s *SessionService) GetLauncherPresets(ctx context.Context, req *connect.Request[sessionv1.GetLauncherPresetsRequest]) (*connect.Response[sessionv1.GetLauncherPresetsResponse], error) {
	return s.launcherPresetsSvc.GetLauncherPresets(ctx, req)
}

// ResolveDefaults merges all default layers for the given working directory and profile.
func (s *SessionService) ResolveDefaults(ctx context.Context, req *connect.Request[sessionv1.ResolveDefaultsRequest]) (*connect.Response[sessionv1.ResolveDefaultsResponse], error) {
	return s.defaultsSvc.ResolveDefaults(ctx, req)
}

// UpdateGlobalDefaults replaces the global default fields.
func (s *SessionService) UpdateGlobalDefaults(ctx context.Context, req *connect.Request[sessionv1.UpdateGlobalDefaultsRequest]) (*connect.Response[sessionv1.UpdateGlobalDefaultsResponse], error) {
	return s.defaultsSvc.UpdateGlobalDefaults(ctx, req)
}

// GetSlackConfig returns the current Slack notification configuration.
func (s *SessionService) GetSlackConfig(ctx context.Context, req *connect.Request[sessionv1.GetSlackConfigRequest]) (*connect.Response[sessionv1.GetSlackConfigResponse], error) {
	return s.slackConfigSvc.GetSlackConfig(ctx, req)
}

// UpdateSlackConfig updates the Slack notification configuration.
func (s *SessionService) UpdateSlackConfig(ctx context.Context, req *connect.Request[sessionv1.UpdateSlackConfigRequest]) (*connect.Response[sessionv1.UpdateSlackConfigResponse], error) {
	return s.slackConfigSvc.UpdateSlackConfig(ctx, req)
}

// TestSlackWebhook sends a synchronous test message and reports the outcome.
func (s *SessionService) TestSlackWebhook(ctx context.Context, req *connect.Request[sessionv1.TestSlackWebhookRequest]) (*connect.Response[sessionv1.TestSlackWebhookResponse], error) {
	return s.slackConfigSvc.TestSlackWebhook(ctx, req)
}

// GetJulesConfig returns the current Jules dispatch-and-poll configuration.
func (s *SessionService) GetJulesConfig(ctx context.Context, req *connect.Request[sessionv1.GetJulesConfigRequest]) (*connect.Response[sessionv1.GetJulesConfigResponse], error) {
	return s.julesConfigSvc.GetJulesConfig(ctx, req)
}

// UpdateJulesConfig updates the Jules configuration.
func (s *SessionService) UpdateJulesConfig(ctx context.Context, req *connect.Request[sessionv1.UpdateJulesConfigRequest]) (*connect.Response[sessionv1.UpdateJulesConfigResponse], error) {
	return s.julesConfigSvc.UpdateJulesConfig(ctx, req)
}

// TestJulesConnection checks whether a repo is registered as a Jules source.
func (s *SessionService) TestJulesConnection(ctx context.Context, req *connect.Request[sessionv1.TestJulesConnectionRequest]) (*connect.Response[sessionv1.TestJulesConnectionResponse], error) {
	return s.julesConfigSvc.TestJulesConnection(ctx, req)
}

// ConfirmEgressConsent is the only RPC that may grant Jules cloud-egress
// consent for a repo — see JulesConfigService.ConfirmEgressConsent's doc
// comment.
func (s *SessionService) ConfirmEgressConsent(ctx context.Context, req *connect.Request[sessionv1.ConfirmEgressConsentRequest]) (*connect.Response[sessionv1.ConfirmEgressConsentResponse], error) {
	return s.julesConfigSvc.ConfirmEgressConsent(ctx, req)
}

// RevokeEgressConsent is the only RPC that may remove Jules cloud-egress
// consent for a repo — see JulesConfigService.RevokeEgressConsent's doc
// comment.
func (s *SessionService) RevokeEgressConsent(ctx context.Context, req *connect.Request[sessionv1.RevokeEgressConsentRequest]) (*connect.Response[sessionv1.RevokeEgressConsentResponse], error) {
	return s.julesConfigSvc.RevokeEgressConsent(ctx, req)
}

// GetStreamHubRolloutStatus returns the current stream-hub rollout status.
func (s *SessionService) GetStreamHubRolloutStatus(ctx context.Context, req *connect.Request[sessionv1.GetStreamHubRolloutStatusRequest]) (*connect.Response[sessionv1.StreamHubRolloutStatus], error) {
	return s.streamHubRolloutSvc.GetStreamHubRolloutStatus(ctx, req)
}

// CompleteStreamHubRollbackRehearsal records the rollback rehearsal as completed.
func (s *SessionService) CompleteStreamHubRollbackRehearsal(ctx context.Context, req *connect.Request[sessionv1.CompleteStreamHubRollbackRehearsalRequest]) (*connect.Response[sessionv1.StreamHubRolloutStatus], error) {
	return s.streamHubRolloutSvc.CompleteStreamHubRollbackRehearsal(ctx, req)
}

// SetStreamHubSessionOverride sets or clears a per-session stream-hub canary override.
func (s *SessionService) SetStreamHubSessionOverride(ctx context.Context, req *connect.Request[sessionv1.SetStreamHubSessionOverrideRequest]) (*connect.Response[sessionv1.StreamHubRolloutStatus], error) {
	return s.streamHubRolloutSvc.SetStreamHubSessionOverride(ctx, req)
}

// SetStreamHubGlobalOverride sets or clears the live global stream-hub override.
func (s *SessionService) SetStreamHubGlobalOverride(ctx context.Context, req *connect.Request[sessionv1.SetStreamHubGlobalOverrideRequest]) (*connect.Response[sessionv1.StreamHubRolloutStatus], error) {
	return s.streamHubRolloutSvc.SetStreamHubGlobalOverride(ctx, req)
}

// SetOnGlobalDefaultsUpdated wires in the callback invoked after every
// successful UpdateGlobalDefaults save (server/dependencies.go uses this to
// trigger an immediate backlog-queue dequeue sweep when the concurrency limit
// is raised).
func (s *SessionService) SetOnGlobalDefaultsUpdated(fn func()) {
	s.defaultsSvc.SetOnGlobalDefaultsUpdated(fn)
}

// SetSharedBacklogConfig wires the *config.Config instance (and its guarding
// mutex) BacklogService reads its concurrency fields from into this
// SessionService's DefaultsService, so UpdateGlobalDefaults can propagate a
// Settings change into BacklogService's live view without a process restart
// (PR #199 review F1). See DefaultsService.SetSharedBacklogConfig.
func (s *SessionService) SetSharedBacklogConfig(cfg *config.Config, mu *sync.RWMutex) {
	s.defaultsSvc.SetSharedBacklogConfig(cfg, mu)
}

// SetSharedCallbackConfig wires the *config.Config instance (and its guarding
// mutex) CallbackDispatcher reads callback URLs from into this SessionService's
// CallbackConfigService, so UpdateCallbackConfig can propagate a saved URL into
// CallbackDispatcher's live view without a process restart. See
// CallbackConfigService.SetSharedCallbackConfig.
func (s *SessionService) SetSharedCallbackConfig(cfg *config.Config, mu *sync.RWMutex) {
	s.callbackConfigSvc.SetSharedCallbackConfig(cfg, mu)
}

// UpsertProfile creates or updates a named profile.
func (s *SessionService) UpsertProfile(ctx context.Context, req *connect.Request[sessionv1.UpsertProfileRequest]) (*connect.Response[sessionv1.UpsertProfileResponse], error) {
	return s.defaultsSvc.UpsertProfile(ctx, req)
}

// DeleteProfile removes a named profile by name.
func (s *SessionService) DeleteProfile(ctx context.Context, req *connect.Request[sessionv1.DeleteProfileRequest]) (*connect.Response[sessionv1.DeleteProfileResponse], error) {
	return s.defaultsSvc.DeleteProfile(ctx, req)
}

// UpsertDirectoryRule creates or updates a directory rule.
func (s *SessionService) UpsertDirectoryRule(ctx context.Context, req *connect.Request[sessionv1.UpsertDirectoryRuleRequest]) (*connect.Response[sessionv1.UpsertDirectoryRuleResponse], error) {
	return s.defaultsSvc.UpsertDirectoryRule(ctx, req)
}

// DeleteDirectoryRule removes a directory rule by path.
func (s *SessionService) DeleteDirectoryRule(ctx context.Context, req *connect.Request[sessionv1.DeleteDirectoryRuleRequest]) (*connect.Response[sessionv1.DeleteDirectoryRuleResponse], error) {
	return s.defaultsSvc.DeleteDirectoryRule(ctx, req)
}

// ─── Callback Config delegates (webhook-triggers Phase 5, FR7) ──────────────

// +api: callback-config:get
// GetCallbackConfig reports which outbound-callback URLs are configured.
func (s *SessionService) GetCallbackConfig(ctx context.Context, req *connect.Request[sessionv1.GetCallbackConfigRequest]) (*connect.Response[sessionv1.GetCallbackConfigResponse], error) {
	return s.callbackConfigSvc.GetCallbackConfig(ctx, req)
}

// +api: callback-config:update
// UpdateCallbackConfig sets one or more outbound-callback URLs.
func (s *SessionService) UpdateCallbackConfig(ctx context.Context, req *connect.Request[sessionv1.UpdateCallbackConfigRequest]) (*connect.Response[sessionv1.UpdateCallbackConfigResponse], error) {
	return s.callbackConfigSvc.UpdateCallbackConfig(ctx, req)
}

// ListAliases returns all configured alias presets.
func (s *SessionService) ListAliases(ctx context.Context, req *connect.Request[sessionv1.ListAliasesRequest]) (*connect.Response[sessionv1.ListAliasesResponse], error) {
	return s.defaultsSvc.ListAliases(ctx, req)
}

// UpsertAlias creates or updates a named alias preset.
func (s *SessionService) UpsertAlias(ctx context.Context, req *connect.Request[sessionv1.UpsertAliasRequest]) (*connect.Response[sessionv1.UpsertAliasResponse], error) {
	return s.defaultsSvc.UpsertAlias(ctx, req)
}

// DeleteAlias removes an alias preset by name.
func (s *SessionService) DeleteAlias(ctx context.Context, req *connect.Request[sessionv1.DeleteAliasRequest]) (*connect.Response[sessionv1.DeleteAliasResponse], error) {
	return s.defaultsSvc.DeleteAlias(ctx, req)
}

// ListProgramsConfig returns all program configurations (built-in and custom).
func (s *SessionService) ListProgramsConfig(ctx context.Context, req *connect.Request[sessionv1.ListProgramsConfigRequest]) (*connect.Response[sessionv1.ListProgramsConfigResponse], error) {
	return s.defaultsSvc.ListProgramsConfig(ctx, req)
}

// UpsertProgramConfig creates or updates a custom program configuration.
func (s *SessionService) UpsertProgramConfig(ctx context.Context, req *connect.Request[sessionv1.UpsertProgramConfigRequest]) (*connect.Response[sessionv1.UpsertProgramConfigResponse], error) {
	return s.defaultsSvc.UpsertProgramConfig(ctx, req)
}

// ProbeProgram checks whether a program command resolves to a usable executable.
func (s *SessionService) ProbeProgram(ctx context.Context, req *connect.Request[sessionv1.ProbeProgramRequest]) (*connect.Response[sessionv1.ProbeProgramResponse], error) {
	return s.defaultsSvc.ProbeProgram(ctx, req)
}

// StartProgramProbeLoginPath starts login-shell PATH derivation for ProbeProgram.
func (s *SessionService) StartProgramProbeLoginPath() {
	s.defaultsSvc.StartProgramProbeLoginPath()
}

// DeleteProgramConfig removes a custom program configuration by ID.
func (s *SessionService) DeleteProgramConfig(ctx context.Context, req *connect.Request[sessionv1.DeleteProgramConfigRequest]) (*connect.Response[sessionv1.DeleteProgramConfigResponse], error) {
	return s.defaultsSvc.DeleteProgramConfig(ctx, req)
}

// SearchFiles performs a recursive name-substring search in a session's worktree.
func (s *SessionService) SearchFiles(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchFilesRequest],
) (*connect.Response[sessionv1.SearchFilesResponse], error) {
	return s.fileSvc.SearchFiles(ctx, req)
}

// GetFileService returns the underlying FileService so callers can register
// additional HTTP handlers (e.g. the raw file download endpoint).
func (s *SessionService) GetFileService() *FileService {
	return s.fileSvc
}

// ─── Prompt History ───────────────────────────────────────────────────────────

// +api: session:list-prompt-history
// ListPromptHistory returns saved prompt history entries.
func (s *SessionService) ListPromptHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ListPromptHistoryRequest],
) (*connect.Response[sessionv1.ListPromptHistoryResponse], error) {
	entries, err := s.promptStore.Load()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load prompt history: %w", err))
	}
	protos := make([]*sessionv1.PromptHistoryEntry, len(entries))
	for i, e := range entries {
		protos[i] = &sessionv1.PromptHistoryEntry{
			Id:        e.ID,
			Text:      e.Text,
			Label:     e.Label,
			UsedCount: int32(e.UsedCount), //#nosec G115 -- prompt reuse count, bounded well under int32 max
			LastUsed:  timestamppb.New(e.LastUsed),
		}
	}
	return connect.NewResponse(&sessionv1.ListPromptHistoryResponse{Entries: protos}), nil
}

// +api: session:delete-prompt-history
// DeletePromptHistory removes a saved prompt from history.
func (s *SessionService) DeletePromptHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.DeletePromptHistoryRequest],
) (*connect.Response[sessionv1.DeletePromptHistoryResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	if err := s.promptStore.Delete(req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete prompt history entry: %w", err))
	}
	return connect.NewResponse(&sessionv1.DeletePromptHistoryResponse{}), nil
}

// ─── Projects ─────────────────────────────────────────────────────────────────

// +api: project:create
// CreateProject creates a new project for grouping sessions.
func (s *SessionService) CreateProject(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateProjectRequest],
) (*connect.Response[sessionv1.CreateProjectResponse], error) {
	return s.projectSvc.CreateProject(ctx, req)
}

// +api: project:list
// ListProjects returns all projects.
func (s *SessionService) ListProjects(
	ctx context.Context,
	req *connect.Request[sessionv1.ListProjectsRequest],
) (*connect.Response[sessionv1.ListProjectsResponse], error) {
	return s.projectSvc.ListProjects(ctx, req)
}

// +api: project:update
// UpdateProject updates an existing project's metadata.
func (s *SessionService) UpdateProject(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateProjectRequest],
) (*connect.Response[sessionv1.UpdateProjectResponse], error) {
	return s.projectSvc.UpdateProject(ctx, req)
}

// +api: project:delete
// DeleteProject removes a project (sessions are unassigned, not deleted).
func (s *SessionService) DeleteProject(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteProjectRequest],
) (*connect.Response[sessionv1.DeleteProjectResponse], error) {
	return s.projectSvc.DeleteProject(ctx, req)
}

// +api: project:assign-sessions
// AssignSessionsToProject assigns one or more sessions to a project.
func (s *SessionService) AssignSessionsToProject(
	ctx context.Context,
	req *connect.Request[sessionv1.AssignSessionsToProjectRequest],
) (*connect.Response[sessionv1.AssignSessionsToProjectResponse], error) {
	return s.projectSvc.AssignSessionsToProject(ctx, req)
}

// GetTerminalSnapshot returns the last N lines of terminal output for a session.
// Uses inst.Preview() for a read-only snapshot without requiring an active stream.
func (s *SessionService) GetTerminalSnapshot(
	ctx context.Context,
	req *connect.Request[sessionv1.GetTerminalSnapshotRequest],
) (*connect.Response[sessionv1.GetTerminalSnapshotResponse], error) {
	return s.terminalSvc.GetTerminalSnapshot(ctx, req)
}

// +api: session:log-client-events
// +api: session:write-to-session
// WriteToSession sends raw text input to a running session's PTY.
func (s *SessionService) WriteToSession(
	ctx context.Context,
	req *connect.Request[sessionv1.WriteToSessionRequest],
) (*connect.Response[sessionv1.WriteToSessionResponse], error) {
	return s.terminalSvc.WriteToSession(ctx, req)
}

// LogClientEvents receives batched browser console log entries from the web UI.
// Used for remote debugging of mobile browser sessions where DevTools are unavailable.
// Never returns an error — malformed or oversized entries are silently discarded.
func (s *SessionService) LogClientEvents(
	_ context.Context,
	req *connect.Request[sessionv1.LogClientEventsRequest],
) (*connect.Response[sessionv1.LogClientEventsResponse], error) {
	for _, entry := range req.Msg.GetEntries() {
		logClientEntry(entry)
	}
	return connect.NewResponse(&sessionv1.LogClientEventsResponse{}), nil
}
