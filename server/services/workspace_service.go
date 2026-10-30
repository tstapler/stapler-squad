package services

import (
	"context"
	"errors"
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
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
	"github.com/tstapler/stapler-squad/session/vc"
	"github.com/tstapler/stapler-squad/session/vcs"
	"github.com/tstapler/stapler-squad/telemetry"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

// vcsStatusStaleTTL is how old a cached status may be and still be served instantly
// to a plain poll (while a background refresh runs). Beyond it, or when the request sets
// force_fresh, callers wait for a fresh computation. Matches the UI's 60s active poll.
const vcsStatusStaleTTL = time.Minute

// WorkspaceService handles all VCS/workspace RPC methods.
//
// These methods operate on session workspace state (git/jj status, branch
// switching, worktrees) and may emit events after state-modifying operations.
type WorkspaceService struct {
	storage    *session.Storage
	eventBus   *events.EventBus
	liveFinder LiveInstanceFinder
	// guards is the live hidden_session_readonly_guards value; nil means on.
	guards GuardsFlag
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
	// vcsFlights coalesces concurrent cache misses per workdir and bounds parallel computations.
	vcsFlights vcsFlights
	// vcsEpochs holds a per-workdir counter bumped by InvalidateVCSStatus so a computation that
	// straddles an invalidation does not re-cache a pre-change result. vcsMu makes the epoch
	// check and the cache store (and the bump and the delete) atomic with each other.
	vcsMu     sync.Mutex
	vcsEpochs map[string]uint64
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
func (ws *WorkspaceService) SetGuardsFlag(f GuardsFlag) {
	ws.guards = f
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

// CheckWriteAccess is the WriteGuard for file saves: archived sessions are refused (a
// save never revives one) and so are hidden sessions while the read-only guards are on.
func (ws *WorkspaceService) CheckWriteAccess(sessionID string) error {
	inst, err := ws.findInstanceFast(sessionID)
	if err != nil {
		return err
	}
	if inst.IsArchived() {
		return archivedSessionRefusal()
	}
	if _, err := AccessForUnary(inst, ws.guards).Writer(nil); err != nil {
		return hiddenSessionRefusal()
	}
	return nil
}

// InvalidateVCSStatus drops the cached status of workDir (keyed as GetVCSStatus keys it),
// so a file save shows in the next status poll instead of after the TTL.
func (ws *WorkspaceService) InvalidateVCSStatus(workDir string) {
	ws.vcsMu.Lock()
	if ws.vcsEpochs == nil {
		ws.vcsEpochs = make(map[string]uint64)
	}
	ws.vcsEpochs[workDir]++
	ws.vcsStatusCache.Delete(workDir)
	ws.vcsMu.Unlock()
	// A computation already running began before the change: new callers must not join it.
	ws.vcsFlights.forget(workDir)
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

	if cachedStatus, ok := ws.freshVCSStatus(workDir); ok {
		recordVCSOutcome(ctx, vcsOutcomeCacheHit)
		span.SetAttributes(attribute.String("cache", "hit"))
		return connect.NewResponse(&sessionv1.GetVCSStatusResponse{VcsStatus: vcsStatusToProto(cachedStatus)}), nil
	}
	// Stale-while-revalidate: a poller never waits on git for an entry that is merely
	// past its TTL. StatusAsOf tells the UI how old the data is.
	if stale, ok := ws.cachedVCSStatus(workDir, vcsStatusStaleTTL); ok && !req.Msg.WaitForFresh {
		ws.refreshVCSStatusInBackground(ctx, instance, workDir)
		recordVCSOutcome(ctx, vcsOutcomeStaleHit)
		span.SetAttributes(attribute.String("cache", "stale"))
		return connect.NewResponse(&sessionv1.GetVCSStatusResponse{VcsStatus: vcsStatusToProto(stale)}), nil
	}
	span.SetAttributes(attribute.String("cache", "miss"))

	// A caller waiting for fresh data must not join a computation that began before its request.
	var notBefore time.Time
	if req.Msg.WaitForFresh {
		notBefore = requestedAt
	}
	status, shared, err := ws.vcsFlights.do(ctx, workDir, notBefore, func(workCtx context.Context) (*vc.VCSStatus, error) {
		return ws.computeVCSStatus(workCtx, instance, workDir)
	})
	span.SetAttributes(attribute.Bool("coalesced", shared))
	if err != nil {
		return ws.vcsStatusFailure(ctx, span, err)
	}
	if shared {
		recordVCSOutcome(ctx, vcsOutcomeCoalesced)
	} else {
		recordVCSOutcome(ctx, vcsOutcomeComputed)
	}
	return connect.NewResponse(&sessionv1.GetVCSStatusResponse{VcsStatus: vcsStatusToProto(status)}), nil
}

// cachedAfter reports whether workDir has a cache entry computed after t.
func (ws *WorkspaceService) cachedAfter(workDir string, t time.Time) bool {
	cached, ok := ws.vcsStatusCache.Load(workDir)
	return ok && cached.(vcsStatusCacheEntry).cachedAt.After(t)
}

// refreshVCSStatusInBackground joins (or starts) the workdir's computation without
// blocking the caller. It is itself a waiter, so the work is not cancelled when the
// triggering request ends; vcsComputeTimeout bounds it.
func (ws *WorkspaceService) refreshVCSStatusInBackground(ctx context.Context, instance *session.Instance, workDir string) {
	// Background refreshes yield to interactive misses: skip when every slot is busy.
	if ws.vcsFlights.saturated() {
		return
	}
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), vcsComputeTimeout)
	go func() {
		defer cancel()
		_, _, _ = ws.vcsFlights.do(bgCtx, workDir, time.Time{}, func(workCtx context.Context) (*vc.VCSStatus, error) {
			return ws.computeVCSStatus(workCtx, instance, workDir)
		})
	}()
}

// freshVCSStatus returns a copy of the cached status for workDir if still within the TTL.
func (ws *WorkspaceService) freshVCSStatus(workDir string) (*vc.VCSStatus, bool) {
	return ws.cachedVCSStatus(workDir, vcsStatusCacheTTL)
}

// cachedVCSStatus returns a copy of the cached status for workDir if younger than maxAge.
func (ws *WorkspaceService) cachedVCSStatus(workDir string, maxAge time.Duration) (*vc.VCSStatus, bool) {
	cached, ok := ws.vcsStatusCache.Load(workDir)
	if !ok {
		return nil, false
	}
	entry := cached.(vcsStatusCacheEntry)
	if time.Since(entry.cachedAt) >= maxAge {
		return nil, false
	}
	// Shallow-copy before touching StatusAsOf — entry.status is a pointer shared
	// with every other concurrent cache hit (and with the map itself); writing
	// through it directly races with those readers.
	respStatus := *entry.status
	respStatus.StatusAsOf = entry.cachedAt
	return &respStatus, true
}

// vcsStatusFailure maps a failed flight to the RPC result: a caller that left
// gets a Connect cancel/deadline error (so it shows in the error-code series);
// a git failure keeps the long-standing in-band Error string.
func (ws *WorkspaceService) vcsStatusFailure(
	ctx context.Context, span trace.Span, err error,
) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	switch {
	case ctx.Err() != nil:
		outcome, code := vcsOutcomeCanceled, connect.CodeCanceled
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = connect.CodeDeadlineExceeded
		}
		recordVCSOutcome(context.WithoutCancel(ctx), outcome)
		span.AddEvent("caller_canceled", trace.WithAttributes(attribute.String("connect.code", code.String())))
		return nil, connect.NewError(code, ctx.Err())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// The shared computation was cut short (all other waiters left, or the 30s bound hit).
		recordVCSOutcome(ctx, vcsOutcomeError)
		span.SetStatus(codes.Error, err.Error())
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	recordVCSOutcome(ctx, vcsOutcomeError)
	span.SetStatus(codes.Error, err.Error())
	return connect.NewResponse(&sessionv1.GetVCSStatusResponse{Error: err.Error()}), nil
}

// computeVCSStatus does the expensive work for one workDir and fills the cache.
// Errors carry the user-facing message GetVCSStatus has always put in Error.
func (ws *WorkspaceService) computeVCSStatus(ctx context.Context, instance *session.Instance, workDir string) (*vc.VCSStatus, error) {
	ws.vcsMu.Lock()
	epochAtStart := ws.vcsEpochs[workDir]
	ws.vcsMu.Unlock()
	startedAt := time.Now()
	// A flight that finished between the caller's cache check and this start has filled it.
	if st, ok := ws.freshVCSStatus(workDir); ok {
		return st, nil
	}

	var provider vc.VCSProvider
	gitProvider, err := vc.NewGitProvider(workDir)
	if err != nil {
		jjProvider, jjErr := vc.NewJujutsuProvider(workDir)
		if jjErr != nil {
			return nil, fmt.Errorf("not a version-controlled directory: %s", workDir)
		}
		provider = jjProvider
	} else {
		provider = gitProvider
	}

	_, statusSpan := telemetry.GetTracer().Start(ctx, "vcs.status.git_status",
		trace.WithAttributes(attribute.String("vcs.type", provider.Name())))
	var status *vc.VCSStatus
	if gp, ok := provider.(*vc.GitProvider); ok {
		status, err = gp.GetStatusContext(ctx)
	} else {
		status, err = provider.GetStatus()
	}
	if err != nil {
		statusSpan.RecordError(err)
		statusSpan.End()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to get VCS status: %v", err)
	}
	statusSpan.SetAttributes(attribute.Int("files.changed", len(status.StagedFiles)+len(status.UnstagedFiles)+len(status.UntrackedFiles)+len(status.ConflictFiles)))
	statusSpan.End()

	if _, isGit := provider.(*vc.GitProvider); isGit {
		ws.addShippedCommits(ctx, instance, workDir, status)
	}
	// A cancelled or timed-out context makes the shipped-commit calls fail, which would mark
	// the status CommitsUnavailable: never cache or return such a partial result.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	now := time.Now()
	status.StatusAsOf = now
	ws.storeVCSStatusIfCurrent(workDir, epochAtStart, startedAt, status, now)
	return status, nil
}

// storeVCSStatusIfCurrent caches status unless an invalidation landed since epochAtStart (the
// result may predate it) or a computation that started later already cached a fresher entry.
// Check and store happen under vcsMu, which InvalidateVCSStatus also holds.
func (ws *WorkspaceService) storeVCSStatusIfCurrent(workDir string, epochAtStart uint64, startedAt time.Time, status *vc.VCSStatus, now time.Time) {
	ws.vcsMu.Lock()
	defer ws.vcsMu.Unlock()
	if ws.vcsEpochs[workDir] != epochAtStart || ws.cachedAfter(workDir, startedAt) {
		return
	}
	ws.vcsStatusCache.Store(workDir, vcsStatusCacheEntry{status: status, cachedAt: now})
}

// addShippedCommits fills Commits/AggregateDiffStat. They are computed here (not inside
// GitProvider.GetStatus) because they need the session's recorded base SHA, an
// Instance-level concept the vc.VCSProvider abstraction doesn't have. Jujutsu has no
// equivalent base SHA concept, so this only runs for git sessions.
func (ws *WorkspaceService) addShippedCommits(ctx context.Context, instance *session.Instance, workDir string, status *vc.VCSStatus) {
	baseSHA := instance.GetBaseCommitSHA()
	if baseSHA == "" {
		return
	}
	ctx, span := telemetry.GetTracer().Start(ctx, "vcs.status.shipped_commits")
	defer span.End()
	headSHA, headErr := git.GetHeadCommitSHA(workDir)
	if headErr != nil {
		log.Debug("GetVCSStatus: failed to resolve HEAD commit SHA", "workDir", workDir, "err", headErr)
		status.CommitsUnavailable = true
		return
	}
	if headSHA == baseSHA {
		return
	}
	commits, truncated, listErr := git.ListShippedCommits(ctx, workDir, baseSHA, headSHA)
	if listErr == nil {
		status.Commits = commits
		status.CommitsTruncated = truncated
	} else {
		log.Debug("GetVCSStatus: failed to list shipped commits", "workDir", workDir, "err", listErr)
		status.CommitsUnavailable = true
	}
	if diffStat, diffErr := git.DiffStatBetween(ctx, workDir, baseSHA, headSHA); diffErr == nil {
		status.AggregateDiffStat = &diffStat
	} else {
		log.Debug("GetVCSStatus: failed to compute aggregate diff stat", "workDir", workDir, "err", diffErr)
	}
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
	if _, err := AccessForUnary(instance, ws.guards).Writer(nil); err != nil {
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

// ---------------------------------------------------------------------------
// VCS conversion helpers
// ---------------------------------------------------------------------------

func vcsStatusToProto(status *vc.VCSStatus) *sessionv1.VCSStatus {
	if status == nil {
		return nil
	}

	protoStatus := &sessionv1.VCSStatus{
		Type:        vcsTypeToProto(status.Type),
		Branch:      status.Branch,
		HeadCommit:  status.HeadCommit,
		Description: status.Description,
		// #nosec G115 -- AheadBy/BehindBy are git/jj ahead-behind commit
		// counts for one local branch, far below int32 range.
		AheadBy: int32(status.AheadBy),
		// #nosec G115 -- see AheadBy above.
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

	for _, c := range status.Commits {
		commit := &sessionv1.ShippedCommit{Sha: c.SHA, Summary: c.Summary, AuthorName: c.AuthorName}
		if !c.AuthorAt.IsZero() {
			commit.AuthoredAt = timestamppb.New(c.AuthorAt)
		}
		protoStatus.Commits = append(protoStatus.Commits, commit)
	}
	protoStatus.CommitsTruncated = status.CommitsTruncated
	protoStatus.CommitsUnavailable = status.CommitsUnavailable
	if status.AggregateDiffStat != nil {
		protoStatus.AggregateDiffStat = &sessionv1.AggregateDiffStat{
			// #nosec G115 -- diff stats for one local commit range, far below int32 range.
			FilesChanged: int32(status.AggregateDiffStat.FilesChanged),
			// #nosec G115 -- see FilesChanged above.
			Additions: int32(status.AggregateDiffStat.Additions),
			// #nosec G115 -- see FilesChanged above.
			Deletions: int32(status.AggregateDiffStat.Deletions),
		}
	}
	if !status.StatusAsOf.IsZero() {
		protoStatus.StatusAsOf = timestamppb.New(status.StatusAsOf)
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
		// #nosec G115 -- per-file diff line counts, far below int32 range.
		Additions: int32(f.Additions),
		// #nosec G115 -- see Additions above.
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
