package adapters

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
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

	protoSession := &sessionv1.Session{
		Id:                 inst.GetStableID(),
		Title:              inst.Title,
		Path:               inst.Workspace().EffectivePath,
		WorkingDir:         inst.GetWorkingDirectory(),
		Branch:             inst.Branch,
		Status:             statusToProto(inst.GetEffectiveStatus()),
		Program:            inst.Program,
		Height:             int32(inst.Height),
		Width:              int32(inst.Width),
		CreatedAt:          timestamppb.New(inst.CreatedAt),
		UpdatedAt:          timestamppb.New(inst.UpdatedAt),
		AutoYes:            inst.AutoYes,
		AutonomousMode:     inst.AutonomousMode,
		AutonomousTurn:     inst.AutonomousTurn,
		AutonomousMaxTurns: inst.AutonomousMaxTurns,
		AutonomousOutcome:  inst.AutonomousOutcome,
		Prompt:             inst.Prompt,
		InitialPrompt:      inst.InitialPrompt,
		Category:           inst.Category,
		IsExpanded:         inst.IsExpanded,
		SessionType:        sessionTypeToProto(inst.SessionType),
		TmuxPrefix:         inst.TmuxPrefix,
		Tags:               inst.Tags, // Tag-based organization
		// Terminal activity timestamps for staleness detection
		LastTerminalUpdate:   timestamppb.New(inst.LastTerminalUpdate),
		LastMeaningfulOutput: timestamppb.New(inst.LastMeaningfulOutput),
		// GitHub integration fields
		GithubPrNumber:  int32(inst.GitHubPRNumber),
		GithubPrUrl:     inst.GitHubPRURL,
		GithubOwner:     inst.GitHubOwner,
		GithubRepo:      inst.GitHubRepo,
		GithubSourceRef: inst.GitHubSourceRef,
		ClonedRepoPath:  inst.ClonedRepoPath,
		// Instance type and external metadata
		InstanceType:     instanceTypeToProto(inst.InstanceType),
		ExternalMetadata: externalMetadataToProto(inst.ExternalMetadata),
		// PR status fields (populated by PRStatusPoller)
		GithubPrState:         inst.GitHubPRState,
		GithubPrIsDraft:       inst.GitHubPRIsDraft,
		GithubPrPriority:      inst.GitHubPRPriority,
		GithubApprovedCount:   int32(inst.GitHubApprovedCount),
		GithubChangesReqCount: int32(inst.GitHubChangesReqCount),
		GithubCheckConclusion: inst.GitHubCheckConclusion,
		LastPrStatusCheck:     timestamppb.New(inst.LastPRStatusCheck),
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
	protoSession.HistoryFilePath = inst.HistoryFilePath

	// Creation progress message — only meaningful during Creating state.
	if inst.IsCreating() {
		if inst.CreationProgress != "" {
			protoSession.CreationProgress = inst.CreationProgress
		} else {
			protoSession.CreationProgress = "Starting session..."
		}
	}

	// Rate limit state propagation.
	protoSession.RateLimitState = rateLimitStateToProto(ratelimit.RateLimitState(inst.GetRateLimitState()))
	if t := inst.GetRateLimitResetTime(); !t.IsZero() {
		protoSession.RateLimitResetTime = timestamppb.New(t)
	}
	protoSession.RateLimitEnabled = inst.IsRateLimitEnabled()

	// Pause reason — empty for sessions that have never been paused.
	protoSession.PauseReason = inst.PauseReason

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

	// SubStatus: fine-grained activity state derived from terminal detection.
	// Only meaningful for Active sessions; non-Active sessions always return UNSPECIFIED.
	protoSession.SubStatus = toProtoSubStatus(inst)

	// DetectedStatus / DetectedContext: typed detection fields (fields 68–69).
	// Only meaningful for Active sessions; non-Active sessions leave these at their
	// zero values (DETECTED_STATUS_UNSPECIFIED, empty string).
	if inst.Status == session.Active {
		detectedStatus := inst.GetDetectedStatus()
		if detectedStatus != detection.StatusUnknown {
			protoSession.DetectedStatus = detection.DetectedStatusToProto(detectedStatus)
			protoSession.DetectedContext = inst.GetDetectedContext()
		}
	}

	// Hidden flag — system/background sessions excluded from default list/review queue.
	protoSession.Hidden = inst.Hidden

	// Workflow linkage, name, and archive state.
	protoSession.WorkflowId = inst.WorkflowID
	if inst.WorkflowID != "" && workflowNames != nil {
		protoSession.WorkflowName = workflowNames[inst.WorkflowID]
	}
	if inst.ArchivedAt != nil {
		protoSession.ArchivedAt = timestamppb.New(*inst.ArchivedAt)
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

// toProtoSubStatus derives the SubStatus proto enum from an Instance's terminal detection state.
// Returns SUB_STATUS_UNSPECIFIED for non-Active sessions or when no detection data is available.
// Rate limit state takes precedence over ClaudeController-detected sub-status.
func toProtoSubStatus(inst *session.Instance) sessionv1.SubStatus {
	if inst.Status != session.Active {
		return sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED
	}
	// Rate limit state takes precedence.
	if ratelimit.RateLimitState(inst.GetRateLimitState()) == ratelimit.StateWaiting {
		return sessionv1.SubStatus_SUB_STATUS_RATE_LIMITED
	}
	switch inst.GetDetectedStatus() {
	case detection.StatusProcessing, detection.StatusExecuting, detection.StatusWaitingForAgent:
		// StatusWaitingForAgent maps to PROCESSING — no distinct proto value exists yet;
		// the UI shows the same "Thinking…" chip, which is correct while agents run.
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
func StatusToProto(status session.Status) sessionv1.SessionStatus {
	switch status {
	case session.Active:
		return sessionv1.SessionStatus_SESSION_STATUS_ACTIVE // wire value 1 (same as legacy RUNNING)
	case session.Paused:
		return sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	case session.Creating:
		return sessionv1.SessionStatus_SESSION_STATUS_CREATING
	case session.Stopped:
		return sessionv1.SessionStatus_SESSION_STATUS_STOPPED
	case session.Hibernated:
		return sessionv1.SessionStatus_SESSION_STATUS_HIBERNATED
	default:
		return sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
	}
}

// statusToProto is kept for backward compatibility
func statusToProto(status session.Status) sessionv1.SessionStatus {
	return StatusToProto(status)
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
