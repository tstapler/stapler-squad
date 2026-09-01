package adapters

import (
	"fmt"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/cdp"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/ratelimit"
	"github.com/tstapler/stapler-squad/session/vnc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InstanceToProto converts a session.Instance to a proto Session message.
// workflowNames is an optional map from workflow UUID to workflow name; pass nil to omit workflow_name.
func InstanceToProto(inst *session.Instance, workflowNames map[string]string) *sessionv1.Session {
	if inst == nil {
		return nil
	}

	// Take a single lock-free snapshot so all field reads below are consistent and
	// race-free. Method calls (GetStableID, GetEffectiveStatus, etc.) have their own
	// synchronisation and are left as-is. Fields absent from InstanceSnapshot
	// (LaunchCommand, CreationProgress) remain as direct reads.
	snap := inst.Snapshot()

	protoSession := &sessionv1.Session{
		Id:                 inst.GetStableID(),
		Title:              snap.Title,
		Path:               inst.Workspace().EffectivePath,
		WorkingDir:         inst.GetWorkingDirectory(),
		Branch:             snap.Branch,
		Status:             statusToProto(inst.GetEffectiveStatus()),
		Program:            snap.Program,
		Height:             int32(snap.Height),
		Width:              int32(snap.Width),
		CreatedAt:          timestamppb.New(snap.CreatedAt),
		UpdatedAt:          timestamppb.New(snap.UpdatedAt),
		AutoYes:            snap.AutoYes,
		AutoApprove:        snap.AutoApprove,
		AutonomousMode:     snap.Autonomous.AutonomousMode,
		AutonomousTurn:     snap.Autonomous.AutonomousTurn,
		AutonomousMaxTurns: snap.Autonomous.AutonomousMaxTurns,
		AutonomousOutcome:  snap.Autonomous.AutonomousOutcome,
		Prompt:             snap.Prompt,
		InitialPrompt:      snap.InitialPrompt,
		Category:           snap.Category,
		Note:               snap.Note,
		IsExpanded:         snap.IsExpanded,
		SessionType:        sessionTypeToProto(snap.SessionType),
		TmuxPrefix:         snap.TmuxPrefix,
		Tags:               snap.Tags, // Tag-based organization
		// Terminal activity timestamps for staleness detection
		LastTerminalUpdate:   timestamppb.New(snap.LastTerminalUpdate),
		LastMeaningfulOutput: timestamppb.New(snap.LastMeaningfulOutput),
		// GitHub integration fields
		GithubPrNumber:  int32(snap.GitHub.GitHubPRNumber),
		GithubPrUrl:     snap.GitHub.GitHubPRURL,
		GithubOwner:     snap.GitHub.GitHubOwner,
		GithubRepo:      snap.GitHub.GitHubRepo,
		GithubSourceRef: snap.GitHub.GitHubSourceRef,
		ClonedRepoPath:  snap.GitHub.ClonedRepoPath,
		// Instance type and external metadata
		InstanceType:     instanceTypeToProto(snap.InstanceType),
		ExternalMetadata: externalMetadataToProto(snap.ExternalMetadata),
		// PR status fields (populated by PRStatusPoller)
		GithubPrState:         inst.GitHubPRState,
		GithubPrIsDraft:       inst.GitHubPRIsDraft,
		GithubPrPriority:      inst.GitHubPRPriority,
		GithubApprovedCount:   int32(inst.GitHubApprovedCount),
		GithubChangesReqCount: int32(inst.GitHubChangesReqCount),
		GithubCheckConclusion: inst.GitHubCheckConclusion,
		GithubChecks:          checksToProto(inst.GitHubChecks),
		GithubReviewFeedback:  reviewFeedbackToProto(inst.GitHubReviewFeedback),
		GithubMergeable:       inst.GitHubMergeable,
		LastPrStatusCheck:     timestamppb.New(inst.LastPRStatusCheck),
		WorkspaceKey:          inst.WorkspaceKey(),
		LaunchCommand:         inst.LaunchCommand,
		// Restart-from-session lineage (Story 2.3.1)
		RestartedFromSessionId: snap.RestartedFromSessionID,
	}

	// Retry state (session-retry-backoff), fields 76-80. Read directly from
	// the instance (not the InstanceSnapshot atomic pointer, which predates
	// this feature) via RetrySnapshot's race-free copy.
	rs := inst.RetrySnapshot()
	protoSession.RetryAttempt = int32(rs.RetryAttempt)
	protoSession.RetryMaxAttempts = int32(rs.RetryMaxAttempts)
	protoSession.LastFailureReason = rs.LastFailureReason
	if !rs.NextRetryAt.IsZero() {
		protoSession.NextRetryAt = timestamppb.New(rs.NextRetryAt)
	}
	if len(rs.RetryHistory) > 0 {
		protoSession.RetryHistory = make([]*sessionv1.RetryAttemptRecord, len(rs.RetryHistory))
		for i, rec := range rs.RetryHistory {
			protoSession.RetryHistory[i] = &sessionv1.RetryAttemptRecord{
				Attempt:   int32(rec.Attempt),
				Reason:    rec.Reason,
				Timestamp: timestamppb.New(rec.Timestamp),
			}
		}
	}

	// Convert artifact data if available
	if snap.Artifacts != nil {
		a := snap.Artifacts
		protoSession.Artifacts = &sessionv1.SessionArtifacts{
			PrUrls:        a.PRURLs,
			CommitShas:    a.CommitSHAs,
			ExternalUrls:  a.ExternalURLs,
			LastScannedAt: timestamppb.New(a.LastScannedAt),
		}
	}

	// Convert git worktree data if available
	wt, err := inst.GetGitWorktree()
	if err == nil && wt != nil {
		protoSession.GitWorktree = &sessionv1.GitWorktree{
			RepoPath:      wt.GetRepoPath(),
			WorktreePath:  wt.GetWorktreePath(),
			BranchName:    wt.GetBranchName(),
			BaseCommitSha: wt.GetBaseCommitSHA(),
		}
	}

	// Convert diff stats if available
	if inst.GetDiffStats() != nil {
		stats := inst.GetDiffStats()
		protoSession.DiffStats = &sessionv1.DiffStats{
			Added:   int32(stats.Added),
			Removed: int32(stats.Removed),
		}
	}

	// Cached "has commits ahead of base" signal (AC6) — see Instance.UpdateDiffStats.
	protoSession.HasCommitsAhead = inst.GetHasCommitsAhead()

	// Convert Claude session data if available
	if inst.GetClaudeSession() != nil {
		cs := inst.GetClaudeSession()
		protoSession.ClaudeSession = &sessionv1.ClaudeSession{
			SessionId:      cs.ConversationUUID,
			ConversationId: cs.SquadSessionID,
			ProjectName:    cs.ProjectName,
		}
	}

	// History file linkage — path to the Claude JSONL conversation file.
	protoSession.HistoryFilePath = snap.HistoryFilePath

	// Outcome of the most recent start/cold-restore decision.
	protoSession.ReviveOutcome = reviveOutcomeToProto(snap.LastReviveOutcome)

	// Creation progress message — only meaningful during Creating state.
	if inst.IsCreating() {
		if inst.CreationProgress != "" {
			protoSession.CreationProgress = inst.CreationProgress
		} else {
			protoSession.CreationProgress = "Starting session..."
		}
	}

	// Reason the async creation pipeline failed — only meaningful when the
	// session is Failed. See session.Instance.FailureReason's doc comment.
	protoSession.FailureReason = inst.FailureReason()

	// Rate limit state propagation.
	protoSession.RateLimitState = rateLimitStateToProto(ratelimit.RateLimitState(inst.GetRateLimitState()))
	if t := inst.GetRateLimitResetTime(); !t.IsZero() {
		protoSession.RateLimitResetTime = timestamppb.New(t)
	}
	protoSession.RateLimitEnabled = inst.IsRateLimitEnabled()

	// Pause reason — empty for sessions that have never been paused.
	protoSession.PauseReason = snap.PauseReason

	// Exit reason — only meaningful when status == SESSION_STATUS_CRASHED.
	protoSession.ExitReason = snap.ExitReason

	// VNC / browser-passthrough state.
	if vncMgr := inst.VNCManager(); vncMgr != nil {
		vncState := vncMgr.State()
		protoSession.VncState = &sessionv1.VNCState{
			Status:                mapVNCStatus(vncState.Status),
			DisplayNumber:         int32(vncState.DisplayNumber),
			BrowserWindowDetected: vncState.BrowserWindowDetected,
			// VncPassword intentionally omitted in list/watch paths — only exposed by GetSession.
		}
	}

	// CDP / browser-streaming state.
	if cdpMgr := inst.CDPManager(); cdpMgr != nil {
		cdpState := cdpMgr.State()
		protoSession.CdpState = &sessionv1.CDPState{
			Status: mapCDPStatus(cdpState.Status),
		}
	}

	// Compute status info once for SubStatus + DetectedStatus + DetectedContext.
	var statusInfo session.InstanceStatusInfo
	if snap.Status == session.Active {
		if mgr := inst.GetStatusManager(); mgr != nil {
			statusInfo = mgr.GetStatus(inst)
		}
	}

	// SubStatus: fine-grained activity state derived from terminal detection.
	// Only meaningful for Active sessions; non-Active sessions always return UNSPECIFIED.
	protoSession.SubStatus = toProtoSubStatusFromInfo(snap.Status, inst.GetRateLimitState(), statusInfo)

	// DetectedStatus / DetectedContext: typed detection fields (fields 68–69).
	if statusInfo.IsControllerActive && statusInfo.ClaudeStatus != detection.StatusUnknown {
		protoSession.DetectedStatus = detection.DetectedStatusToProto(statusInfo.ClaudeStatus)
		protoSession.DetectedContext = statusInfo.StatusContext
	}

	// SubagentCount (field 75): count of background agents/shells/monitors from the
	// WaitingForAgent detector. Set unconditionally — InstanceStatusInfo.SubagentCount is
	// already 0 by construction when the controller is inactive or status isn't
	// WaitingForAgent, so a guard here would be a no-op.
	protoSession.SubagentCount = int32(statusInfo.SubagentCount)

	// Hidden flag — system/background sessions excluded from default list/review queue.
	protoSession.Hidden = snap.Hidden

	// Workflow linkage, name, and archive state.
	protoSession.WorkflowId = snap.WorkflowID
	if snap.WorkflowID != "" && workflowNames != nil {
		protoSession.WorkflowName = workflowNames[snap.WorkflowID]
	}
	if snap.ArchivedAt != nil {
		protoSession.ArchivedAt = timestamppb.New(*snap.ArchivedAt)
	}

	// remote_name (field 76): host badge for a remote session (ssh-remote-workspaces
	// Epic 6.2). Derived live from ExecutionTarget rather than a persisted field --
	// see the proto field's doc comment for why (ExecutionTarget is `json:"-"`, not
	// reconstructed across a restart). Empty (the zero value) for a LocalTarget.
	if remoteTarget, ok := inst.GetExecutionTarget().(session.RemoteExecutionTarget); ok {
		protoSession.RemoteName = remoteTarget.Target().Name
	}

	// Session goal summary — populated when a goal has been set via set_session_goal MCP tool.
	if g := inst.GetSessionGoal(); g != nil {
		tasksJSON, _ := session.EncodeTasks(g.Tasks) // empty string on error is safe
		protoSession.Goal = &sessionv1.SessionGoalSummary{
			GoalText:   g.Goal,
			Status:     g.Status,
			TasksTotal: int32(g.TasksTotal()),
			TasksDone:  int32(g.TasksDone()),
			TasksJson:  tasksJSON,
			UpdatedAt:  timestamppb.New(g.UpdatedAt),
		}
	}

	return protoSession
}

// mapVNCStatus converts a vnc.VNCStatus to the proto VNCStatus enum.
func mapVNCStatus(status vnc.VNCStatus) sessionv1.VNCStatus {
	switch status {
	case vnc.VNCStatusStarting:
		return sessionv1.VNCStatus_VNC_STATUS_STARTING
	case vnc.VNCStatusReady:
		return sessionv1.VNCStatus_VNC_STATUS_READY
	case vnc.VNCStatusNoBrowser:
		return sessionv1.VNCStatus_VNC_STATUS_NO_BROWSER
	case vnc.VNCStatusPassthrough:
		return sessionv1.VNCStatus_VNC_STATUS_PASSTHROUGH
	case vnc.VNCStatusUnavailable:
		return sessionv1.VNCStatus_VNC_STATUS_UNAVAILABLE
	default:
		return sessionv1.VNCStatus_VNC_STATUS_UNSPECIFIED
	}
}

// mapCDPStatus converts a cdp.CDPStatus to the proto CDPStatus enum.
func mapCDPStatus(status cdp.CDPStatus) sessionv1.CDPStatus {
	switch status {
	case cdp.CDPStatusWaiting:
		return sessionv1.CDPStatus_CDP_STATUS_WAITING
	case cdp.CDPStatusStreaming:
		return sessionv1.CDPStatus_CDP_STATUS_STREAMING
	case cdp.CDPStatusNoBrowser:
		return sessionv1.CDPStatus_CDP_STATUS_NO_BROWSER
	case cdp.CDPStatusUnavailable:
		return sessionv1.CDPStatus_CDP_STATUS_UNAVAILABLE
	default:
		return sessionv1.CDPStatus_CDP_STATUS_UNSPECIFIED
	}
}

// toProtoSubStatusFromInfo derives the SubStatus proto enum from pre-computed status info.
// Returns SUB_STATUS_UNSPECIFIED for non-Active sessions or when no detection data is available.
// Rate limit state takes precedence over ClaudeController-detected sub-status.
func toProtoSubStatusFromInfo(basicStatus session.Status, rateLimitState int, info session.InstanceStatusInfo) sessionv1.SubStatus {
	if basicStatus != session.Active {
		return sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED
	}
	// Rate limit state takes precedence.
	if ratelimit.RateLimitState(rateLimitState) == ratelimit.StateWaiting {
		return sessionv1.SubStatus_SUB_STATUS_RATE_LIMITED
	}
	switch info.ClaudeStatus {
	case detection.StatusWaitingForAgent:
		return sessionv1.SubStatus_SUB_STATUS_WAITING_FOR_AGENT
	case detection.StatusCompacting:
		return sessionv1.SubStatus_SUB_STATUS_COMPACTING
	case detection.StatusProcessing, detection.StatusExecuting:
		return sessionv1.SubStatus_SUB_STATUS_PROCESSING
	case detection.StatusNeedsApproval:
		return sessionv1.SubStatus_SUB_STATUS_NEEDS_APPROVAL
	case detection.StatusInputRequired:
		return sessionv1.SubStatus_SUB_STATUS_INPUT_REQUIRED
	case detection.StatusError:
		return sessionv1.SubStatus_SUB_STATUS_ERROR
	case detection.StatusTestsFailing:
		return sessionv1.SubStatus_SUB_STATUS_TESTS_FAILING
	case detection.StatusReady:
		return sessionv1.SubStatus_SUB_STATUS_READY
	case detection.StatusIdle:
		return sessionv1.SubStatus_SUB_STATUS_IDLE
	case detection.StatusSuccess:
		return sessionv1.SubStatus_SUB_STATUS_SUCCESS
	case detection.StatusUnknown:
		// Unknown / undetected — don't show a chip
		return sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED
	}
	return sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED
}

// rateLimitStateToProto converts a ratelimit.RateLimitState to proto RateLimitState enum.
func rateLimitStateToProto(state ratelimit.RateLimitState) sessionv1.RateLimitState {
	switch state {
	case ratelimit.StateNone:
		return sessionv1.RateLimitState_RATE_LIMIT_STATE_NONE
	case ratelimit.StateWaiting:
		return sessionv1.RateLimitState_RATE_LIMIT_STATE_WAITING
	case ratelimit.StateRecovering:
		return sessionv1.RateLimitState_RATE_LIMIT_STATE_RECOVERING
	case ratelimit.StateRecovered:
		return sessionv1.RateLimitState_RATE_LIMIT_STATE_RECOVERED
	case ratelimit.StateFailed:
		return sessionv1.RateLimitState_RATE_LIMIT_STATE_FAILED
	default:
		return sessionv1.RateLimitState_RATE_LIMIT_STATE_NONE
	}
}

// StatusToProto converts session.Status to proto SessionStatus enum.
// StatusToProto converts a session.Status to its proto SessionStatus wire value.
// Returns an explicit error for an unrecognized status instead of silently
// falling back to UNSPECIFIED, so a future new session.Status value that isn't
// added here fails loudly (see .golangci.yml's exhaustive-linter exclusion
// comment for why this is enforced at the code level here rather than via the
// linter, which is deliberately disabled repo-wide for iota-typed switches with
// intentional default arms).
func StatusToProto(status session.Status) (sessionv1.SessionStatus, error) {
	switch status {
	case session.Active:
		return sessionv1.SessionStatus_SESSION_STATUS_ACTIVE, nil // wire value 1 (same as legacy RUNNING)
	case session.Paused:
		return sessionv1.SessionStatus_SESSION_STATUS_PAUSED, nil
	case session.Creating:
		return sessionv1.SessionStatus_SESSION_STATUS_CREATING, nil
	case session.Stopped:
		return sessionv1.SessionStatus_SESSION_STATUS_STOPPED, nil
	case session.Hibernated:
		return sessionv1.SessionStatus_SESSION_STATUS_HIBERNATED, nil
	case session.Restoring:
		return sessionv1.SessionStatus_SESSION_STATUS_RESTORING, nil
	case session.Crashed:
		return sessionv1.SessionStatus_SESSION_STATUS_CRASHED, nil
	case session.PermanentlyFailed:
		return sessionv1.SessionStatus_SESSION_STATUS_PERMANENTLY_FAILED, nil
	case session.Failed:
		return sessionv1.SessionStatus_SESSION_STATUS_FAILED, nil
	default:
		return sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED, fmt.Errorf("adapters.StatusToProto: unrecognized session.Status %d", int(status))
	}
}

// statusToProto is kept for backward compatibility with callers that predate
// StatusToProto's explicit-error signature. An unrecognized status is logged
// (it should never happen — every session.Status value is handled above) and
// mapped to UNSPECIFIED rather than propagated, matching this wrapper's
// original no-error contract.
func statusToProto(status session.Status) sessionv1.SessionStatus {
	proto, err := StatusToProto(status)
	if err != nil {
		log.Error("statusToProto: unrecognized session.Status", "status", int(status), "err", err)
		return sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
	}
	return proto
}

// StatusStringToProto converts a status string (from session.Status.String()) to proto SessionStatus.
// Used when the status is stored as a string in ReviewItem rather than session.Status.
func StatusStringToProto(status string) sessionv1.SessionStatus {
	switch status {
	case "Active", "Running", "Ready": // Running/Ready are deprecated aliases
		return sessionv1.SessionStatus_SESSION_STATUS_ACTIVE
	case "Paused":
		return sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	case "NeedsApproval": // deprecated — NeedsApproval is now a sub-status; sessions are Active
		return sessionv1.SessionStatus_SESSION_STATUS_ACTIVE
	case "Creating":
		return sessionv1.SessionStatus_SESSION_STATUS_CREATING
	case "Stopped":
		return sessionv1.SessionStatus_SESSION_STATUS_STOPPED
	case "Hibernated":
		return sessionv1.SessionStatus_SESSION_STATUS_HIBERNATED
	case "Crashed":
		return sessionv1.SessionStatus_SESSION_STATUS_CRASHED
	case "PermanentlyFailed":
		return sessionv1.SessionStatus_SESSION_STATUS_PERMANENTLY_FAILED
	default:
		return sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
	}
}

// sessionTypeToProto converts session.SessionType to proto SessionType enum.
func sessionTypeToProto(sessionType session.SessionType) sessionv1.SessionType {
	switch sessionType {
	case session.SessionTypeDirectory:
		return sessionv1.SessionType_SESSION_TYPE_DIRECTORY
	case session.SessionTypeNewWorktree:
		return sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE
	case session.SessionTypeExistingWorktree:
		return sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE
	default:
		return sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED
	}
}

// reviveOutcomeToProto converts session.ReviveOutcome to the proto ReviveOutcome enum.
func reviveOutcomeToProto(outcome session.ReviveOutcome) sessionv1.ReviveOutcome {
	switch outcome {
	case session.ReviveOutcomeResumeLive:
		return sessionv1.ReviveOutcome_REVIVE_OUTCOME_RESUME_LIVE
	case session.ReviveOutcomeResumeRecovered:
		return sessionv1.ReviveOutcome_REVIVE_OUTCOME_RESUME_RECOVERED
	case session.ReviveOutcomeFreshExpected:
		return sessionv1.ReviveOutcome_REVIVE_OUTCOME_FRESH_EXPECTED
	case session.ReviveOutcomeFreshLostHistory:
		return sessionv1.ReviveOutcome_REVIVE_OUTCOME_FRESH_LOST_HISTORY
	default:
		return sessionv1.ReviveOutcome_REVIVE_OUTCOME_UNSPECIFIED
	}
}

// ProtoToStatus converts proto SessionStatus enum to session.Status.
// Legacy wire values from older clients (READY=2, NEEDS_APPROVAL=5, LOADING=3) are
// mapped to the appropriate new lifecycle states.
func ProtoToStatus(status sessionv1.SessionStatus) session.Status {
	switch status {
	case sessionv1.SessionStatus_SESSION_STATUS_ACTIVE:
		// Also handles RUNNING(1) which shares the same integer wire value.
		return session.Active
	case 2: // SESSION_STATUS_READY — deprecated legacy wire value → Active
		return session.Active
	case 5: // SESSION_STATUS_NEEDS_APPROVAL — deprecated legacy wire value → Active (sub-status only now)
		return session.Active
	case 3: // SESSION_STATUS_LOADING — deprecated legacy wire value → Creating
		return session.Creating
	case sessionv1.SessionStatus_SESSION_STATUS_CREATING:
		return session.Creating
	case sessionv1.SessionStatus_SESSION_STATUS_PAUSED:
		return session.Paused
	case sessionv1.SessionStatus_SESSION_STATUS_STOPPED:
		return session.Stopped
	case sessionv1.SessionStatus_SESSION_STATUS_HIBERNATED:
		return session.Hibernated
	case sessionv1.SessionStatus_SESSION_STATUS_RESTORING:
		return session.Restoring
	case sessionv1.SessionStatus_SESSION_STATUS_CRASHED:
		// Collateral fix: this case was missing before session-retry-backoff
		// touched this switch — a round-tripped CRASHED session silently fell
		// to the default: below and reopened as Creating.
		return session.Crashed
	case sessionv1.SessionStatus_SESSION_STATUS_PERMANENTLY_FAILED:
		return session.PermanentlyFailed
	default:
		return session.Creating // Default to Creating for unknown statuses
	}
}

// ProtoToSessionType converts proto SessionType enum to session.SessionType.
func ProtoToSessionType(sessionType sessionv1.SessionType) session.SessionType {
	switch sessionType {
	case sessionv1.SessionType_SESSION_TYPE_DIRECTORY:
		return session.SessionTypeDirectory
	case sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE:
		return session.SessionTypeNewWorktree
	case sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE:
		return session.SessionTypeExistingWorktree
	default:
		return session.SessionTypeDirectory // Default to Directory for unknown types
	}
}

// instanceTypeToProto converts session.InstanceType to proto InstanceType enum.
func instanceTypeToProto(instanceType session.InstanceType) sessionv1.InstanceType {
	switch instanceType {
	case session.InstanceTypeManaged:
		return sessionv1.InstanceType_INSTANCE_TYPE_MANAGED
	case session.InstanceTypeExternal:
		return sessionv1.InstanceType_INSTANCE_TYPE_EXTERNAL
	default:
		return sessionv1.InstanceType_INSTANCE_TYPE_UNSPECIFIED
	}
}

// checksToProto converts the itemized GitHub statusCheckRollup into proto GithubCheckItem messages.
func checksToProto(checks []github.CheckItem) []*sessionv1.GithubCheckItem {
	out := make([]*sessionv1.GithubCheckItem, len(checks))
	for i, c := range checks {
		out[i] = &sessionv1.GithubCheckItem{Name: c.Name, Context: c.Context, State: c.State, Status: c.Status, Conclusion: c.Conclusion}
	}
	return out
}

// reviewFeedbackToProto converts the itemized GitHub PR review list into proto GithubReviewFeedback messages.
func reviewFeedbackToProto(reviews []github.ReviewItem) []*sessionv1.GithubReviewFeedback {
	out := make([]*sessionv1.GithubReviewFeedback, len(reviews))
	for i, r := range reviews {
		out[i] = &sessionv1.GithubReviewFeedback{Author: r.Author, State: r.State, Body: r.Body}
	}
	return out
}

// externalMetadataToProto converts session.ExternalInstanceMetadata to proto ExternalInstanceMetadata.
func externalMetadataToProto(metadata *session.ExternalInstanceMetadata) *sessionv1.ExternalInstanceMetadata {
	if metadata == nil {
		return nil
	}

	return &sessionv1.ExternalInstanceMetadata{
		TmuxSocket:      metadata.TmuxSocket,
		TmuxSessionName: metadata.TmuxSessionName,
		DiscoveredAt:    timestamppb.New(metadata.DiscoveredAt),
		LastSeen:        timestamppb.New(metadata.LastSeen),
		OriginalPid:     int32(metadata.OriginalPID),
		MuxSocketPath:   metadata.MuxSocketPath,
		MuxEnabled:      metadata.MuxEnabled,
		SourceTerminal:  metadata.SourceTerminal,
	}
}
