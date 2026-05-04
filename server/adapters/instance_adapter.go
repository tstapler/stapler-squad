package adapters

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/ratelimit"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InstanceToProto converts a session.Instance to a proto Session message.
func InstanceToProto(inst *session.Instance) *sessionv1.Session {
	if inst == nil {
		return nil
	}

	protoSession := &sessionv1.Session{
		Id:          inst.GetStableID(),
		Title:       inst.Title,
		Path:        inst.Workspace().EffectivePath,
		WorkingDir:  inst.GetWorkingDirectory(),
		Branch:      inst.Branch,
		Status:      statusToProto(inst.GetEffectiveStatus()),
		Program:     inst.Program,
		Height:      int32(inst.Height),
		Width:       int32(inst.Width),
		CreatedAt:   timestamppb.New(inst.CreatedAt),
		UpdatedAt:   timestamppb.New(inst.UpdatedAt),
		AutoYes:     inst.AutoYes,
		Prompt:      inst.Prompt,
		Category:    inst.Category,
		IsExpanded:  inst.IsExpanded,
		SessionType: sessionTypeToProto(inst.SessionType),
		TmuxPrefix:  inst.TmuxPrefix,
		Tags:        inst.Tags, // Tag-based organization
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

	// Rate limit state propagation.
	protoSession.RateLimitState = rateLimitStateToProto(ratelimit.RateLimitState(inst.GetRateLimitState()))
	if t := inst.GetRateLimitResetTime(); !t.IsZero() {
		protoSession.RateLimitResetTime = timestamppb.New(t)
	}
	protoSession.RateLimitEnabled = inst.IsRateLimitEnabled()

	return protoSession
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
	case session.Running:
		return sessionv1.SessionStatus_SESSION_STATUS_RUNNING
	case session.Ready:
		return sessionv1.SessionStatus_SESSION_STATUS_READY
	case session.Loading:
		return sessionv1.SessionStatus_SESSION_STATUS_LOADING
	case session.Paused:
		return sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	case session.NeedsApproval:
		return sessionv1.SessionStatus_SESSION_STATUS_NEEDS_APPROVAL
	case session.Creating:
		return sessionv1.SessionStatus_SESSION_STATUS_CREATING
	case session.Stopped:
		return sessionv1.SessionStatus_SESSION_STATUS_STOPPED
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
	case "Running":
		return sessionv1.SessionStatus_SESSION_STATUS_RUNNING
	case "Ready":
		return sessionv1.SessionStatus_SESSION_STATUS_READY
	case "Loading":
		return sessionv1.SessionStatus_SESSION_STATUS_LOADING
	case "Paused":
		return sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	case "NeedsApproval":
		return sessionv1.SessionStatus_SESSION_STATUS_NEEDS_APPROVAL
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
func ProtoToStatus(status sessionv1.SessionStatus) session.Status {
	switch status {
	case sessionv1.SessionStatus_SESSION_STATUS_RUNNING:
		return session.Running
	case sessionv1.SessionStatus_SESSION_STATUS_READY:
		return session.Ready
	case sessionv1.SessionStatus_SESSION_STATUS_LOADING:
		return session.Loading
	case sessionv1.SessionStatus_SESSION_STATUS_PAUSED:
		return session.Paused
	case sessionv1.SessionStatus_SESSION_STATUS_NEEDS_APPROVAL:
		return session.NeedsApproval
	case sessionv1.SessionStatus_SESSION_STATUS_CREATING:
		return session.Creating
	case sessionv1.SessionStatus_SESSION_STATUS_STOPPED:
		return session.Stopped
	default:
		return session.Loading // Default to Loading for unknown statuses
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

// MapIdleStateToWorkingState converts a detection.IdleState to the proto WorkingState enum.
// Prefer MapDetectedStatusToWorkingState when a ClaudeStatus is available, as it can
// produce WORKING_STATE_PROCESSING which IdleState alone cannot distinguish from ACTIVE.
func MapIdleStateToWorkingState(s detection.IdleState) sessionv1.WorkingState {
	switch s {
	case detection.IdleStateActive:
		return sessionv1.WorkingState_WORKING_STATE_ACTIVE
	case detection.IdleStateWaiting:
		return sessionv1.WorkingState_WORKING_STATE_IDLE
	case detection.IdleStateTimeout:
		return sessionv1.WorkingState_WORKING_STATE_WAITING
	default:
		return sessionv1.WorkingState_WORKING_STATE_UNSPECIFIED
	}
}

// MapDetectedStatusToWorkingState converts a detection.DetectedStatus to the proto
// WorkingState enum. It is more precise than MapIdleStateToWorkingState because
// DetectedStatus distinguishes StatusActive from StatusProcessing.
func MapDetectedStatusToWorkingState(s detection.DetectedStatus) sessionv1.WorkingState {
	switch s {
	case detection.StatusActive:
		return sessionv1.WorkingState_WORKING_STATE_ACTIVE
	case detection.StatusProcessing:
		return sessionv1.WorkingState_WORKING_STATE_PROCESSING
	case detection.StatusIdle, detection.StatusReady:
		return sessionv1.WorkingState_WORKING_STATE_IDLE
	case detection.StatusNeedsApproval, detection.StatusInputRequired:
		return sessionv1.WorkingState_WORKING_STATE_WAITING
	default:
		return sessionv1.WorkingState_WORKING_STATE_UNSPECIFIED
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
