// Package workspacerpc serves the VCS and workspace RPCs of a session: status,
// workspace info, switch targets, switching and the branch list. It never imports
// server/services.
package workspacerpc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/fileops"
	"github.com/tstapler/stapler-squad/server/services/vcsstatus"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
	instancepkg "github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/vcs"
	"github.com/tstapler/stapler-squad/telemetry"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WorkspaceProvider resolves workspace path information for a session.
// Inject this interface into services that need path resolution instead of
// accessing session.Instance.Path directly.
type WorkspaceProvider interface {
	GetWorkspace(sessionID string) (domain.Workspace, error)
}

// FileService finds these by type assertion, so a signature drift would otherwise drop them silently.
var (
	_ fileops.WriteGuard        = (*WorkspaceService)(nil)
	_ fileops.StatusInvalidator = (*WorkspaceService)(nil)
)

// LiveInstanceFinder is satisfied by SessionService (server/services). It returns the live in-memory
// instance by scanning the poller's tracked sessions (O(N)) or nil if the session is
// not yet in the poller. WorkspaceService uses this as a fast path to avoid calling
// LoadInstances() — which re-hydrates all sessions from disk and spawns PTY/tmux
// subprocesses — on every read-only RPC call.
type LiveInstanceFinder interface {
	FindLiveInstance(id string) *instancepkg.Instance
}

// branchCacheEntry holds a cached branch list for a repository path.
// Moved from session_service.go together with ListBranches (ADR-001, Story 1.4).
type branchCacheEntry struct {
	branches []string
	cachedAt time.Time
}

const branchCacheTTL = 5 * time.Minute

// WorkspaceService handles all VCS/workspace RPC methods.
//
// These methods operate on session workspace state (git/jj status, branch
// switching, worktrees) and may emit events after state-modifying operations.
type WorkspaceService struct {
	storage    *session.Storage
	eventBus   *events.EventBus
	liveFinder LiveInstanceFinder
	// guards is the live hidden_session_readonly_guards value; nil means on.
	guards access.GuardsFlag
	// inFlightSwitches tracks session IDs currently undergoing a workspace switch.
	// Prevents concurrent SwitchWorkspace RPCs on the same session from corrupting state.
	inFlightSwitches sync.Map
	// branchCache caches git branch lists per repo path. ADR-002.
	// Moved here from SessionService (Story 1.4 — ListBranches was the odd one out
	// next to GetVCSStatus/GetWorkspaceInfo/ListWorkspaceTargets/SwitchWorkspace,
	// which all already lived in WorkspaceService).
	branchCache sync.Map // map[string]branchCacheEntry
	// vcs caches, coalesces and computes GetVCSStatus results per workdir.
	vcs vcsstatus.Cache
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

// SetGuardsFlag injects the live read-only guards flag.
func (ws *WorkspaceService) SetGuardsFlag(f access.GuardsFlag) {
	ws.guards = f
}

// findInstanceFast returns the live instance for the given id. It tries the
// live poller first (O(1) map lookup, no subprocess), falling back to
// LoadInstances only when the session is not yet tracked by the poller.
//
// All WorkspaceService RPCs use this path. SaveInstances accepts a slice
// with a single element, so the mutating SwitchWorkspace RPC also uses this
// rather than loading the full session list.
func (ws *WorkspaceService) findInstanceFast(id string) (*instancepkg.Instance, error) {
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
func (ws *WorkspaceService) GetWorkspace(sessionID string) (domain.Workspace, error) {
	inst, err := ws.findInstanceFast(sessionID)
	if err != nil {
		return domain.Workspace{}, err
	}
	return inst.Workspace(), nil
}

// CheckWriteAccess is the WriteGuard for file saves: archived sessions are refused (a
// save never revives one) and so are hidden sessions while the read-only guards are on.
func (ws *WorkspaceService) CheckWriteAccess(sessionID string) error {
	inst, err := ws.findInstanceFast(sessionID)
	if err != nil {
		return err
	}
	if inst.IsArchived() {
		return fileops.ArchivedSessionRefusal()
	}
	if _, err := access.AccessForUnary(inst, ws.guards).Writer(nil); err != nil {
		return fileops.HiddenSessionRefusal()
	}
	return nil
}

// InvalidateVCSStatus drops the cached status of workDir (keyed as GetVCSStatus keys it),
// so a file save shows in the next status poll instead of after the TTL.
func (ws *WorkspaceService) InvalidateVCSStatus(workDir string) {
	ws.vcs.Invalidate(workDir)
}

// GetVCSStatus retrieves the current version control status for a session.
func (ws *WorkspaceService) GetVCSStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.GetVCSStatusRequest],
) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	requestedAt := time.Now()
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	instance, err := ws.findInstanceFast(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	workDir := instance.Workspace().ExistingDir
	if workDir == "" {
		return connect.NewResponse(&sessionv1.GetVCSStatusResponse{
			Error: "session has no working directory",
		}), nil
	}

	ctx, span := telemetry.GetTracer().Start(ctx, "vcs.status.request",
		trace.WithAttributes(attribute.String("session.id", req.Msg.Id), attribute.String("workdir", workDir)))
	defer span.End()

	return ws.vcs.Serve(ctx, vcsstatus.Request{
		WorkDir:      workDir,
		WaitForFresh: req.Msg.WaitForFresh,
		RequestedAt:  requestedAt,
		BaseSHA:      instance.GetBaseCommitSHA,
	})
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
		// #nosec G115 -- ModifiedFileCount is a working-tree file count for one
		// local repo, far below int32 range.
		ModifiedFileCount: int32(vcsInfo.ModifiedFileCount),
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

	instance, err := ws.findInstanceFast(req.Msg.Id)
	if err != nil {
		return nil, err
	}

	// Every switch type types into or restarts the pane, so a hidden target is
	// refused before the in-flight entry, the checkpoint or any git-side change.
	if _, err := access.AccessForUnary(instance, ws.guards).Writer(nil); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %q is a background session and is read-only: %w", req.Msg.Id, err))
	}

	// Guard against concurrent switches on the same session.
	if _, loaded := ws.inFlightSwitches.LoadOrStore(req.Msg.Id, true); loaded {
		return nil, connect.NewError(connect.CodeUnavailable,
			fmt.Errorf("workspace switch already in progress for session '%s'", req.Msg.Id))
	}
	defer ws.inFlightSwitches.Delete(req.Msg.Id)

	// ExistingDir despite the "never compare it" rule on Workspace: this keys the
	// VCS status cache, and that status is genuinely a property of the directory
	// git actually ran in, not of which session asked.
	preWorkDir := instance.Workspace().ExistingDir

	var switchType instancepkg.WorkspaceSwitchType
	switch req.Msg.SwitchType {
	case sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_DIRECTORY:
		switchType = instancepkg.SwitchTypeDirectory
	case sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_REVISION:
		switchType = instancepkg.SwitchTypeRevision
	case sessionv1.WorkspaceSwitchType_WORKSPACE_SWITCH_TYPE_WORKTREE:
		switchType = instancepkg.SwitchTypeWorktree
	default:
		switchType = instancepkg.SwitchTypeRevision
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

	switchReq := instancepkg.WorkspaceSwitchRequest{
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
		if err := ws.storage.SaveInstances([]*instancepkg.Instance{instance}); err != nil {
			log.Warn("failed to save instances after workspace switch", "err", err)
		}
	}

	// Evict the status cache for the old path; the new path will be repopulated on next poll.
	ws.InvalidateVCSStatus(preWorkDir)

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
				Branches: branches,
				// #nosec G115 -- len(branches) is a git ref count for one local repo, far below int32 range.
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
	start := time.Now()
	refNames, runErr := gitprovider.Backend().ListRefs(cmdCtx, backend.Local{Root: backend.RepoRoot(absPath)}, backend.ListRefsRequest{Pattern: backend.RefPattern(refSpec)})
	latencyMs := time.Since(start).Milliseconds()
	log.Info("[ListBranches] branch list", "latency_ms", latencyMs, "repo", absPath)

	// A timeout returns an empty, truncated list: the backend yields no partial output.
	truncated := false
	if runErr != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
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

	all := make([]string, 0, len(refNames))
	for _, name := range refNames {
		if short := shortRefName(string(name)); short != "" {
			all = append(all, short)
		}
	}

	// Cache the full unfiltered list.
	ws.branchCache.Store(absPath, branchCacheEntry{branches: all, cachedAt: time.Now()})

	branches := filterBranches(all, filter, maxResults)
	return connect.NewResponse(&sessionv1.ListBranchesResponse{
		Branches: branches,
		// #nosec G115 -- len(branches) is a git ref count for one local repo, far below int32 range.
		TotalCount: int32(len(branches)),
		Truncated:  truncated,
	}), nil
}

// shortRefName reproduces `for-each-ref --format=%(refname:short)` for the ref kinds a branch
// picker lists. Git's extra disambiguation (a branch and tag sharing a name show as "heads/x"
// and "tags/x") is not reproduced; the Backend returns full names so the two stay distinct.
func shortRefName(full string) string {
	switch {
	case strings.HasPrefix(full, "refs/heads/"):
		return strings.TrimPrefix(full, "refs/heads/")
	case strings.HasPrefix(full, "refs/remotes/"):
		short := strings.TrimPrefix(full, "refs/remotes/")
		// git abbreviates a remote's HEAD symref to the bare remote name.
		if remote, ok := strings.CutSuffix(short, "/HEAD"); ok {
			return remote
		}
		return short
	case strings.HasPrefix(full, "refs/tags/"):
		return strings.TrimPrefix(full, "refs/tags/")
	default:
		return strings.TrimPrefix(full, "refs/")
	}
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

// VCSCacheForTest exposes the status cache so the host's wiring tests can seed
// and read it.
func (ws *WorkspaceService) VCSCacheForTest() *vcsstatus.Cache { return &ws.vcs }

// InFlightSwitchesForTest exposes the switch guard map to the host's tests.
func (ws *WorkspaceService) InFlightSwitchesForTest() *sync.Map { return &ws.inFlightSwitches }
