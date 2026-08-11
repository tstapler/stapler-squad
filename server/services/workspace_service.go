package services

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
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

// WorkspaceProvider resolves workspace path information for a session.
// Inject this interface into services that need path resolution instead of
// accessing session.Instance.Path directly.
type WorkspaceProvider interface {
	GetWorkspace(sessionID string) (session.Workspace, error)
}

// LiveInstanceFinder is satisfied by SessionService. It returns the live in-memory
// instance by scanning the poller's tracked sessions (O(N)) or nil if the session is
// not yet in the poller. WorkspaceService uses this as a fast path to avoid calling
// LoadInstances() — which re-hydrates all sessions from disk and spawns PTY/tmux
// subprocesses — on every read-only RPC call.
type LiveInstanceFinder interface {
	FindLiveInstance(id string) *session.Instance
}

// branchCacheEntry holds a cached branch list for a repository path.
// Moved from session_service.go together with ListBranches (ADR-001, Story 1.4).
type branchCacheEntry struct {
	branches []string
	cachedAt time.Time
}

// vcsStatusCacheEntry caches the full VCSStatus result for a working directory.
type vcsStatusCacheEntry struct {
	status   *vc.VCSStatus
	cachedAt time.Time
}

const branchCacheTTL = 5 * time.Minute

// vcsStatusCacheTTL caps repeated git subprocess overhead for frequent GetVCSStatus
// polls. 15 s matches the GitProvider.branchCacheTTL and keeps the UI feeling fresh.
const vcsStatusCacheTTL = 15 * time.Second

// WorkspaceService handles all VCS/workspace RPC methods.
//
// These methods operate on session workspace state (git/jj status, branch
// switching, worktrees) and may emit events after state-modifying operations.
type WorkspaceService struct {
	storage    *session.Storage
	eventBus   *events.EventBus
	liveFinder LiveInstanceFinder
	// inFlightSwitches tracks session IDs currently undergoing a workspace switch.
	// Prevents concurrent SwitchWorkspace RPCs on the same session from corrupting state.
	inFlightSwitches sync.Map
	// branchCache caches git branch lists per repo path. ADR-002.
	// Moved here from SessionService (Story 1.4 — ListBranches was the odd one out
	// next to GetVCSStatus/GetWorkspaceInfo/ListWorkspaceTargets/SwitchWorkspace,
	// which all already lived in WorkspaceService).
	branchCache sync.Map // map[string]branchCacheEntry
	// vcsStatusCache caches full VCSStatus results per workdir to avoid spawning 6+
	// git subprocesses on every poll. Keyed by workdir path.
	vcsStatusCache sync.Map // map[string]vcsStatusCacheEntry
}

// NewWorkspaceService creates a WorkspaceService with the given dependencies.
func NewWorkspaceService(storage *session.Storage, eventBus *events.EventBus) *WorkspaceService {
	return &WorkspaceService{storage: storage, eventBus: eventBus}
}

// SetLiveFinder wires the fast-path instance lookup. Call this after constructing
// SessionService so that read-only RPCs bypass LoadInstances().
func (ws *WorkspaceService) SetLiveFinder(f LiveInstanceFinder) {
	ws.liveFinder = f
}

// findInstanceFast returns the live instance for the given id. It tries the
// live poller first (O(1) map lookup, no subprocess), falling back to
// LoadInstances only when the session is not yet tracked by the poller.
//
// All WorkspaceService RPCs use this path. SaveInstances accepts a slice
// with a single element, so the mutating SwitchWorkspace RPC also uses this
// rather than loading the full session list.
func (ws *WorkspaceService) findInstanceFast(id string) (*session.Instance, error) {
	if ws.liveFinder != nil {
		if inst := ws.liveFinder.FindLiveInstance(id); inst != nil {
			return inst, nil
		}
	}
	if ws.storage == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", id))
	}
	instances, err := ws.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}
	for _, inst := range instances {
		if inst.MatchesID(id) {
			return inst, nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", id))
}

// GetWorkspace implements WorkspaceProvider.
func (ws *WorkspaceService) GetWorkspace(sessionID string) (session.Workspace, error) {
	inst, err := ws.findInstanceFast(sessionID)
	if err != nil {
		return session.Workspace{}, err
	}
	return inst.Workspace(), nil
}

// GetVCSStatus retrieves the current version control status for a session.
func (ws *WorkspaceService) GetVCSStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.GetVCSStatusRequest],
) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instance, err := ws.findInstanceFast(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	workDir := instance.Workspace().EffectivePath
	if workDir == "" {
		return connect.NewResponse(&sessionv1.GetVCSStatusResponse{
			Error: "session has no working directory",
		}), nil
	}

	// Fast path: return cached status if still fresh.
	if cached, ok := ws.vcsStatusCache.Load(workDir); ok {
		entry := cached.(vcsStatusCacheEntry)
		if time.Since(entry.cachedAt) < vcsStatusCacheTTL {
			return connect.NewResponse(&sessionv1.GetVCSStatusResponse{
				VcsStatus: vcsStatusToProto(entry.status),
			}), nil
		}
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

	ws.vcsStatusCache.Store(workDir, vcsStatusCacheEntry{status: status, cachedAt: time.Now()})

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

	instance, err := ws.findInstanceFast(req.Msg.Id)
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

	instance, err := ws.findInstanceFast(req.Msg.Id)
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

	// Guard against concurrent switches on the same session.
	if _, loaded := ws.inFlightSwitches.LoadOrStore(req.Msg.Id, true); loaded {
		return nil, connect.NewError(connect.CodeUnavailable,
			fmt.Errorf("workspace switch already in progress for session '%s'", req.Msg.Id))
	}
	defer ws.inFlightSwitches.Delete(req.Msg.Id)

	instance, err := ws.findInstanceFast(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	preWorkDir := instance.Workspace().EffectivePath

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

	// Create a named checkpoint before switching so the state is recoverable.
	if instance.Started() && !instance.Paused() {
		label := "pre-switch: " + req.Msg.Target
		if _, cpErr := instance.CreateCheckpoint(label, 0); cpErr != nil {
			log.Warn("SwitchWorkspace: pre-switch checkpoint failed (non-fatal)", "session", instance.Title, "err", cpErr)
		}
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

	if ws.storage != nil {
		if err := ws.storage.SaveInstances([]*session.Instance{instance}); err != nil {
			log.Warn("failed to save instances after workspace switch", "err", err)
		}
	}

	// Evict the status cache for the old path; the new path will be repopulated on next poll.
	ws.vcsStatusCache.Delete(preWorkDir)

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
		Session:          adapters.InstanceToProto(instance, nil),
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
		Path:      f.Path,
		Status:    fileStatusToProto(f.Status),
		IsStaged:  f.IsStaged,
		OldPath:   f.OldPath,
		Additions: int32(f.Additions),
		Deletions: int32(f.Deletions),
	}
}

// ListBranches returns the git branches for a given repository path.
// Results are cached per repo path with a 5-minute TTL. ADR-002.
// Moved from SessionService (Story 1.4 — was the odd one out next to the four
// workspace methods that already delegated here).
func (ws *WorkspaceService) ListBranches(
	ctx context.Context,
	req *connect.Request[sessionv1.ListBranchesRequest],
) (*connect.Response[sessionv1.ListBranchesResponse], error) {
	repoPath := req.Msg.GetRepoPath()
	if repoPath == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo_path is required"))
	}

	// Normalize and validate the path: must resolve within the user's home directory.
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid repo_path: %w", err))
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("cannot determine home directory: %w", err))
	}
	if !strings.HasPrefix(absPath, homeDir+string(filepath.Separator)) && absPath != homeDir {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo_path must be within the user home directory"))
	}
	if _, err := os.Stat(absPath); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo_path does not exist: %w", err))
	}

	maxResults := int(req.Msg.GetMaxResults())
	if maxResults <= 0 {
		maxResults = 200
	}
	filter := req.Msg.GetFilter()

	// Serve from cache if still fresh.
	if entry, ok := ws.branchCache.Load(absPath); ok {
		cached := entry.(branchCacheEntry)
		if time.Since(cached.cachedAt) < branchCacheTTL {
			branches := filterBranches(cached.branches, filter, maxResults)
			return connect.NewResponse(&sessionv1.ListBranchesResponse{
				Branches:   branches,
				TotalCount: int32(len(branches)),
				Truncated:  false,
			}), nil
		}
	}

	// Run git for-each-ref with a 2-second timeout.
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	refSpec := "refs/heads"
	if req.Msg.GetIncludeRemote() {
		refSpec = "refs/"
	}
	cmd := safeexec.CommandContext(cmdCtx, "git", "-C", absPath, "for-each-ref", refSpec, "--format=%(refname:short)")

	var out bytes.Buffer
	cmd.Stdout = &out

	start := time.Now()
	runErr := cmd.Run()
	latencyMs := time.Since(start).Milliseconds()
	log.Info("[ListBranches] branch list", "latency_ms", latencyMs, "repo", absPath)

	truncated := false
	if runErr != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			// Timeout: return whatever partial output was collected.
			truncated = true
		} else {
			// git failed (not a git repo, etc.): return empty list, not an error.
			log.Warn("[ListBranches] git for-each-ref failed", "repo", absPath, "err", runErr)
			return connect.NewResponse(&sessionv1.ListBranchesResponse{
				Branches:   []string{},
				TotalCount: 0,
				Truncated:  false,
			}), nil
		}
	}

	var all []string
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			all = append(all, line)
		}
	}

	// Cache the full unfiltered list.
	ws.branchCache.Store(absPath, branchCacheEntry{branches: all, cachedAt: time.Now()})

	branches := filterBranches(all, filter, maxResults)
	return connect.NewResponse(&sessionv1.ListBranchesResponse{
		Branches:   branches,
		TotalCount: int32(len(branches)),
		Truncated:  truncated,
	}), nil
}

// filterBranches applies a case-insensitive substring filter and caps results at maxResults.
// Moved from session_service.go together with ListBranches (Story 1.4).
func filterBranches(all []string, filter string, maxResults int) []string {
	lowerFilter := strings.ToLower(filter)
	var result []string
	for _, b := range all {
		if filter == "" || strings.Contains(strings.ToLower(b), lowerFilter) {
			result = append(result, b)
			if len(result) >= maxResults {
				break
			}
		}
	}
	return result
}
