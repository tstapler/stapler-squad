package adapters

import (
	"claude-squad/session"
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InstanceToProto converts a session.Instance to a proto Session message.
func InstanceToProto(inst *session.Instance) *sessionv1.Session {
	if inst == nil {
		return nil
	}

	protoSession := &sessionv1.Session{
		Id:          inst.Title, // Using Title as ID
		Title:       inst.Title,
		Path:        inst.Path,
		WorkingDir:  inst.WorkingDir,
		Branch:      inst.Branch,
		Status:      statusToProto(inst.Status),
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
			Content: stats.Content,
		}
	}

	// Convert Claude session data if available
	if inst.GetClaudeSession() != nil {
		cs := inst.GetClaudeSession()
		protoSession.ClaudeSession = &sessionv1.ClaudeSession{
			SessionId:      cs.SessionID,
			ConversationId: cs.ConversationID,
			ProjectName:    cs.ProjectName,
		}
	}

	return protoSession
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
	default:
		return sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
	}
}

// statusToProto is kept for backward compatibility
func statusToProto(status session.Status) sessionv1.SessionStatus {
	return StatusToProto(status)
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
