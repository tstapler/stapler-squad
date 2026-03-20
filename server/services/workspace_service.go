package services

import (
	"context"
	"fmt"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/vc"
	"github.com/tstapler/stapler-squad/session/vcs"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WorkspaceService handles all VCS/workspace RPC methods.
//
// These methods operate on session workspace state (git/jj status, branch
// switching, worktrees) and may emit events after state-modifying operations.
type WorkspaceService struct {
	storage  *session.Storage
	eventBus *events.EventBus
}

// NewWorkspaceService creates a WorkspaceService with the given dependencies.
func NewWorkspaceService(storage *session.Storage, eventBus *events.EventBus) *WorkspaceService {
	return &WorkspaceService{storage: storage, eventBus: eventBus}
}

// findInstance loads instances from storage and returns the one with the given title.
// Returns CodeNotFound if the session does not exist.
func (ws *WorkspaceService) findInstance(id string) ([]*session.Instance, *session.Instance, error) {
	instances, err := ws.storage.LoadInstances()
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}
	for _, inst := range instances {
		if inst.Title == id {
			return instances, inst, nil
		}
	}
	return instances, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", id))
}

// GetVCSStatus retrieves the current version control status for a session.
func (ws *WorkspaceService) GetVCSStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.GetVCSStatusRequest],
) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	_, instance, err := ws.findInstance(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	workDir := instance.Path
	if workDir == "" {
		return connect.NewResponse(&sessionv1.GetVCSStatusResponse{
			Error: "session has no working directory",
		}), nil
	}

	var provider vc.VCSProvider
	gitProvider, err := vc.NewGitProvider(workDir)
	if err != nil {
		jjProvider, jjErr := vc.NewJujutsuProvider(workDir)
		if jjErr != nil {
			return connect.NewResponse(&sessionv1.GetVCSStatusResponse{
				Error: fmt.Sprintf("not a version-controlled directory: %s", workDir),
			}), nil
		}
		provider = jjProvider
	} else {
		provider = gitProvider
	}

	status, err := provider.GetStatus()
	if err != nil {
		return connect.NewResponse(&sessionv1.GetVCSStatusResponse{
			Error: fmt.Sprintf("failed to get VCS status: %v", err),
		}), nil
	}

	return connect.NewResponse(&sessionv1.GetVCSStatusResponse{
		VcsStatus: vcsStatusToProto(status),
	}), nil
}

// GetWorkspaceInfo retrieves VCS and workspace information for a session.
func (ws *WorkspaceService) GetWorkspaceInfo(
	ctx context.Context,
	req *connect.Request[sessionv1.GetWorkspaceInfoRequest],
) (*connect.Response[sessionv1.GetWorkspaceInfoResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	_, instance, err := ws.findInstance(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	vcsInfo, err := instance.GetVCSInfo()
	if err != nil {
		return connect.NewResponse(&sessionv1.GetWorkspaceInfoResponse{
			Error: err.Error(),
		}), nil
	}

	protoVCSInfo := &sessionv1.VCSInfo{
		RepoPath:              vcsInfo.RepoPath,
		HasJj:                 vcsInfo.HasJJ,
		HasGit:                vcsInfo.HasGit,
		IsColocated:           vcsInfo.IsColocated,
		CurrentBookmark:       vcsInfo.CurrentBookmark,
		CurrentRevision:       vcsInfo.CurrentRevision,
		HasUncommittedChanges: vcsInfo.HasUncommittedChanges,
		ModifiedFileCount:     int32(vcsInfo.ModifiedFileCount),
	}

	switch vcsInfo.VCSType {
	case "jj":
		protoVCSInfo.VcsType = sessionv1.VCSType_VCS_TYPE_JUJUTSU
	case "git":
		protoVCSInfo.VcsType = sessionv1.VCSType_VCS_TYPE_GIT
	default:
		protoVCSInfo.VcsType = sessionv1.VCSType_VCS_TYPE_UNSPECIFIED
	}

	return connect.NewResponse(&sessionv1.GetWorkspaceInfoResponse{
		VcsInfo: protoVCSInfo,
	}), nil
}

// ListWorkspaceTargets returns available switch targets for a session.
func (ws *WorkspaceService) ListWorkspaceTargets(
	ctx context.Context,
	req *connect.Request[sessionv1.ListWorkspaceTargetsRequest],
) (*connect.Response[sessionv1.ListWorkspaceTargetsResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	_, instance, err := ws.findInstance(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	targets, err := instance.ListAvailableTargets()
	if err != nil {
		return connect.NewResponse(&sessionv1.ListWorkspaceTargetsResponse{
			Error: err.Error(),
		}), nil
	}

	protoTargets := &sessionv1.AvailableWorkspaceTargets{}

	switch targets.VCSType {
	case "jj":
		protoTargets.VcsType = sessionv1.VCSType_VCS_TYPE_JUJUTSU
	case "git":
		protoTargets.VcsType = sessionv1.VCSType_VCS_TYPE_GIT
	default:
		protoTargets.VcsType = sessionv1.VCSType_VCS_TYPE_UNSPECIFIED
	}

	for _, b := range targets.Bookmarks {
		protoTargets.Bookmarks = append(protoTargets.Bookmarks, &sessionv1.BookmarkTarget{
			Name:       b.Name,
			RevisionId: b.RevisionID,
			IsRemote:   b.IsRemote,
		})
	}

	for _, r := range targets.RecentRevisions {
		protoTargets.RecentRevisions = append(protoTargets.RecentRevisions, &sessionv1.RevisionTarget{
			Id:          r.ID,
			ShortId:     r.ShortID,
			Description: r.Description,
			Author:      r.Author,
			Timestamp:   timestamppb.New(r.Timestamp),
			IsCurrent:   r.IsCurrent,
		})
	}

	for _, wt := range targets.Worktrees {
		protoTargets.Worktrees = append(protoTargets.Worktrees, &sessionv1.WorktreeTarget{
			Name:       wt.Name,
			Path:       wt.Path,
			Bookmark:   wt.Bookmark,
			RevisionId: wt.RevisionID,
			IsCurrent:  wt.IsCurrent,
		})
	}

	return connect.NewResponse(&sessionv1.ListWorkspaceTargetsResponse{
		Targets: protoTargets,
	}), nil
}

// SwitchWorkspace switches a session's workspace to a different branch, revision, or worktree.
func (ws *WorkspaceService) SwitchWorkspace(
	ctx context.Context,
	req *connect.Request[sessionv1.SwitchWorkspaceRequest],
) (*connect.Response[sessionv1.SwitchWorkspaceResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}
	if req.Msg.Target == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("target is required"))
	}

	instances, instance, err := ws.findInstance(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	var switchType session.WorkspaceSwitchType
	switch req.Msg.SwitchType {
	case sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_DIRECTORY:
		switchType = session.SwitchTypeDirectory
	case sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_REVISION:
		switchType = session.SwitchTypeRevision
	case sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_WORKTREE:
		switchType = session.SwitchTypeWorktree
	default:
		switchType = session.SwitchTypeRevision
	}

	var changeStrategy vcs.ChangeStrategy
	switch req.Msg.ChangeStrategy {
	case sessionv1.ChangeStrategy_CHANGE_STRATEGY_KEEP_AS_WIP:
		changeStrategy = vcs.KeepAsWIP
	case sessionv1.ChangeStrategy_CHANGE_STRATEGY_BRING_ALONG:
		changeStrategy = vcs.BringAlong
	case sessionv1.ChangeStrategy_CHANGE_STRATEGY_ABANDON:
		changeStrategy = vcs.Abandon
	default:
		changeStrategy = vcs.KeepAsWIP
	}

	switchReq := session.WorkspaceSwitchRequest{
		Type:            switchType,
		Target:          req.Msg.Target,
		ChangeStrategy:  changeStrategy,
		CreateIfMissing: req.Msg.CreateIfMissing,
		BaseRevision:    req.Msg.BaseRevision,
	}

	result, err := instance.SwitchWorkspace(switchReq)
	if err != nil {
		return connect.NewResponse(&sessionv1.SwitchWorkspaceResponse{
			Success: false,
			Message: err.Error(),
		}), nil
	}

	var protoVCSType sessionv1.VCSType
	switch result.VCSType {
	case vcs.VCSTypeJJ:
		protoVCSType = sessionv1.VCSType_VCS_TYPE_JUJUTSU
	case vcs.VCSTypeGit:
		protoVCSType = sessionv1.VCSType_VCS_TYPE_GIT
	default:
		protoVCSType = sessionv1.VCSType_VCS_TYPE_UNSPECIFIED
	}

	if err := ws.storage.SaveInstances(instances); err != nil {
		log.WarningLog.Printf("Failed to save instances after workspace switch: %v", err)
	}

	if ws.eventBus != nil {
		ws.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"workspace", "branch"}))
	}

	return connect.NewResponse(&sessionv1.SwitchWorkspaceResponse{
		Success:          result.Success,
		Message:          "Workspace switched successfully",
		PreviousRevision: result.PreviousRevision,
		CurrentRevision:  result.CurrentRevision,
		VcsType:          protoVCSType,
		ChangesHandled:   result.ChangesHandled,
		Session:          adapters.InstanceToProto(instance),
	}), nil
}

// ---------------------------------------------------------------------------
// VCS conversion helpers
// ---------------------------------------------------------------------------

func vcsStatusToProto(status *vc.VCSStatus) *sessionv1.VCSStatus {
	if status == nil {
		return nil
	}

	protoStatus := &sessionv1.VCSStatus{
		Type:         vcsTypeToProto(status.Type),
		Branch:       status.Branch,
		HeadCommit:   status.HeadCommit,
		Description:  status.Description,
		AheadBy:      int32(status.AheadBy),
		BehindBy:     int32(status.BehindBy),
		Upstream:     status.Upstream,
		HasStaged:    status.HasStaged,
		HasUnstaged:  status.HasUnstaged,
		HasUntracked: status.HasUntracked,
		HasConflicts: status.HasConflicts,
		IsClean:      status.IsClean,
	}

	for _, f := range status.StagedFiles {
		protoStatus.StagedFiles = append(protoStatus.StagedFiles, fileChangeToProto(f))
	}
	for _, f := range status.UnstagedFiles {
		protoStatus.UnstagedFiles = append(protoStatus.UnstagedFiles, fileChangeToProto(f))
	}
	for _, f := range status.UntrackedFiles {
		protoStatus.UntrackedFiles = append(protoStatus.UntrackedFiles, fileChangeToProto(f))
	}
	for _, f := range status.ConflictFiles {
		protoStatus.ConflictFiles = append(protoStatus.ConflictFiles, fileChangeToProto(f))
	}

	return protoStatus
}

func vcsTypeToProto(t vc.VCSType) sessionv1.VCSType {
	switch t {
	case vc.VCSGit:
		return sessionv1.VCSType_VCS_TYPE_GIT
	case vc.VCSJujutsu:
		return sessionv1.VCSType_VCS_TYPE_JUJUTSU
	default:
		return sessionv1.VCSType_VCS_TYPE_UNSPECIFIED
	}
}

func fileStatusToProto(s vc.FileStatus) sessionv1.FileStatus {
	switch s {
	case vc.FileModified:
		return sessionv1.FileStatus_FILE_STATUS_MODIFIED
	case vc.FileAdded:
		return sessionv1.FileStatus_FILE_STATUS_ADDED
	case vc.FileDeleted:
		return sessionv1.FileStatus_FILE_STATUS_DELETED
	case vc.FileRenamed:
		return sessionv1.FileStatus_FILE_STATUS_RENAMED
	case vc.FileCopied:
		return sessionv1.FileStatus_FILE_STATUS_COPIED
	case vc.FileUntracked:
		return sessionv1.FileStatus_FILE_STATUS_UNTRACKED
	case vc.FileIgnored:
		return sessionv1.FileStatus_FILE_STATUS_IGNORED
	case vc.FileConflict:
		return sessionv1.FileStatus_FILE_STATUS_CONFLICT
	default:
		return sessionv1.FileStatus_FILE_STATUS_UNSPECIFIED
	}
}

func fileChangeToProto(f vc.FileChange) *sessionv1.FileChange {
	return &sessionv1.FileChange{
		Path:     f.Path,
		Status:   fileStatusToProto(f.Status),
		IsStaged: f.IsStaged,
		OldPath:  f.OldPath,
	}
}
