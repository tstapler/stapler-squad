package session

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/linkdata/deadlock"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git"
)

// GitWorktreeManager owns the git worktree and diff-stats state that were
// previously bare fields on Instance.
//
// Instance keeps thin wrapper methods that delegate here. GitWorktreeManager
// itself has no knowledge of Instance lifecycle; it only manages the worktree
// and diff operations.
//
// worktree/diffStats are guarded by mu, not by Instance.stateMutex: setup
// (setupFirstTimeWorktree, called under Instance.startMu) and read-side
// callers (e.g. GetEffectiveRootDir) don't consistently hold stateMutex, so
// GitWorktreeManager protects its own fields directly.
type GitWorktreeManager struct {
	mu              deadlock.RWMutex
	worktree        *git.GitWorktree
	diffStats       *git.DiffStats
	diffStatsAt     time.Time // last UpdateDiffStats completion; see DiffStatsFresh
	dirBaseSHA      string
	hasCommitsAhead bool

	// lastRequestedAt is the last time either GetSessionDiff or GetVCSStatus
	// actually asked for this worktree's state (hit or miss) -- see
	// hasRecentActivity/touchRequestedAt.
	lastRequestedAt time.Time

	// changeDetector and changeDetectionActive back Epic 2.2's flag-gated
	// WorktreeChangeDetector wiring -- see startChangeDetector/stopChangeDetector.
	changeDetector        *WorktreeChangeDetector
	changeDetectionActive bool
}

// diffStatsCacheTTL bounds how often GetSessionDiff's RPC-triggered refresh
// (Instance.RefreshDiffStatsIfStale) recomputes the diff — matching
// server/services/workspace_service.go's vcsStatusCacheTTL, the same pattern
// already proven there for GetVCSStatus. Lifecycle-triggered callers
// (daemon.go's AutoYes loop, instance.go's exit/destroy paths) call
// UpdateDiffStats directly and are unaffected — they need a guaranteed-fresh
// read, not a cache.
const diffStatsCacheTTL = 15 * time.Second

// worktreeChangeDetectionFlagName must stay byte-identical to
// server/services/feature_flag_service.go's constant of the same name --
// session cannot import server/services (see session/instance_tmux.go:892-894
// for the established reason), so this is a deliberate duplication, not a typo.
const worktreeChangeDetectionFlagName = "vcs:worktree-change-detection"

// diffStatsCacheTTLWithChangeDetection is used instead of diffStatsCacheTTL
// when a WorktreeChangeDetector is active for this worktree (see
// DiffStatsFresh). 5 minutes matches session/unfinished.Scanner's own
// backstop-ticker interval -- the periodic stat-walk (15s) and .git watch
// are the real freshness mechanism; this is purely a backstop in case both
// somehow stop firing.
const diffStatsCacheTTLWithChangeDetection = 5 * time.Minute

// coldActivityThreshold bounds how long hasRecentActivity() keeps reporting
// true after the last GetSessionDiff/GetVCSStatus request -- deliberately
// matches diffStatsCacheTTLWithChangeDetection: if nobody has asked within
// one full widened-TTL window, there is by definition no live cache entry
// left to protect the freshness of.
const coldActivityThreshold = diffStatsCacheTTLWithChangeDetection

// DiffStatsFresh reports whether the cached diff stats were computed within
// the applicable TTL -- diffStatsCacheTTL normally, or the wider
// diffStatsCacheTTLWithChangeDetection when a WorktreeChangeDetector is
// actively invalidating this worktree's cache on real changes.
func (gm *GitWorktreeManager) DiffStatsFresh() bool {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	if gm.diffStatsAt.IsZero() {
		return false
	}
	ttl := diffStatsCacheTTL
	if gm.changeDetectionActive {
		ttl = diffStatsCacheTTLWithChangeDetection
	}
	return time.Since(gm.diffStatsAt) < ttl
}

// SetDirBaseSHA sets the base commit SHA for directory-mode diff computation.
func (gm *GitWorktreeManager) SetDirBaseSHA(sha string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.dirBaseSHA = sha
}

// GetDirBaseSHA returns the base commit SHA for directory-mode diff computation.
// Was unguarded until Instance.GetBaseCommitSHA() (added for the VCS-tab
// redesign, session/instance_worktree.go) started calling it on every
// WorkspaceService.GetVCSStatus RPC — a much hotter concurrent-read path than
// this field previously saw, turning a latent race into a realistic one.
func (gm *GitWorktreeManager) GetDirBaseSHA() string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.dirBaseSHA
}

// HasWorktree reports whether a git worktree has been initialized.
func (gm *GitWorktreeManager) HasWorktree() bool {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.worktree != nil
}

// GetWorktree returns the underlying GitWorktree (may be nil before Setup).
func (gm *GitWorktreeManager) GetWorktree() *git.GitWorktree {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.worktree
}

// SetWorktree replaces the underlying GitWorktree. Used during session start
// and by tests.
func (gm *GitWorktreeManager) SetWorktree(wt *git.GitWorktree) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.worktree = wt
}

// PrimeDirtyCacheJitter staggers the dirty-cache TTL by setting the cache
// timestamp to a random point in [now-IsDirtyCacheTTL, now). Call this when
// adding a session to the poller so sessions added in a burst don't all run
// git-status subprocesses simultaneously when their caches expire.
func (gm *GitWorktreeManager) PrimeDirtyCacheJitter() {
	wt := gm.GetWorktree()
	if wt == nil {
		return
	}
	jitter := time.Duration(rand.Int63n(int64(git.IsDirtyCacheTTL)))
	wt.PrimeDirtyCacheAt(time.Now().Add(-jitter))
}

// GetWorktreePath returns the worktree path or "" if no worktree.
func (gm *GitWorktreeManager) GetWorktreePath() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetWorktreePath()
}

// GetRepoPath returns the repo root path or "" if no worktree.
func (gm *GitWorktreeManager) GetRepoPath() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetRepoPath()
}

// GetRepoName returns the repository name or "" if no worktree.
func (gm *GitWorktreeManager) GetRepoName() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetRepoName()
}

// GetBranchName returns the branch name or "" if no worktree.
func (gm *GitWorktreeManager) GetBranchName() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetBranchName()
}

// GetBaseCommitSHA returns the base commit SHA or "" if no worktree.
func (gm *GitWorktreeManager) GetBaseCommitSHA() string {
	wt := gm.GetWorktree()
	if wt == nil {
		return ""
	}
	return wt.GetBaseCommitSHA()
}

// Setup prepares the worktree (creates directories, checks out branch, etc.),
// then -- when worktreeChangeDetectionFlagName is on -- starts a
// WorktreeChangeDetector for it (see startChangeDetector).
func (gm *GitWorktreeManager) Setup() error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	if err := wt.Setup(); err != nil {
		return err
	}
	if config.LoadConfig().GetFeatureFlagWithDefault(worktreeChangeDetectionFlagName, false) {
		gm.startChangeDetector(wt)
	}
	return nil
}

// startChangeDetector constructs and starts a WorktreeChangeDetector,
// registering GitWorktreeManager's own diff-stats-clearing callback before
// any external subscriber (e.g. WorkspaceService) gets a chance to register
// its own -- order doesn't affect correctness here (both callbacks are
// independent, idempotent invalidations) but matches architecture.md's
// documented ordering.
//
// Setup() (this method's only caller) is safe to call repeatedly on the same
// GitWorktreeManager -- see restartForRetry in retry_state.go -- so this must
// stop any already-running detector first, or a repeated Setup() call leaks
// the orphaned detector's goroutine and fsnotify watcher.
func (gm *GitWorktreeManager) startChangeDetector(wt *git.GitWorktree) {
	gm.stopChangeDetector()

	detector := NewWorktreeChangeDetector(wt.GetWorktreePath(), func() (bool, string, error) {
		dirty, err := wt.IsDirtyUncached()
		if err != nil {
			return false, "", err
		}
		sha, shaErr := gm.GetCurrentCommitSHA()
		if shaErr != nil {
			return false, "", shaErr
		}
		return dirty, sha, nil
	}, gm.hasRecentActivity) // activeFunc -- see Story 2.2.2
	detector.OnChange(func() {
		gm.mu.Lock()
		gm.diffStatsAt = time.Time{}
		gm.mu.Unlock()
	})
	detector.Start()

	gm.mu.Lock()
	gm.changeDetector = detector
	gm.changeDetectionActive = true
	gm.mu.Unlock()
}

// Cleanup removes the worktree from the filesystem and git metadata.
// Returns nil if no worktree is set.
func (gm *GitWorktreeManager) Cleanup() error {
	gm.stopChangeDetector()
	wt := gm.GetWorktree()
	if wt == nil {
		return nil
	}
	return wt.Cleanup()
}

// Remove removes the worktree from git without pruning.
func (gm *GitWorktreeManager) Remove() error {
	gm.stopChangeDetector()
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.Remove()
}

// stopChangeDetector is the single chokepoint for change-detector teardown,
// called from both Cleanup() and Remove() -- every existing session/instance.go
// call site already funnels through one of these two methods, so hooking in
// here covers every teardown path with no per-call-site duplication. No-op if
// no detector was ever started.
func (gm *GitWorktreeManager) stopChangeDetector() {
	gm.mu.Lock()
	detector := gm.changeDetector
	gm.changeDetector = nil
	gm.changeDetectionActive = false
	gm.mu.Unlock()
	if detector != nil {
		detector.Stop()
	}
}

// WorktreeChangeDetectionActive reports whether a WorktreeChangeDetector is
// currently running for this worktree (flag on and Setup() succeeded in
// starting one) -- the signal WorkspaceService needs to decide its own
// cache's TTL, and CreateDebugSnapshot surfaces for troubleshooting.
func (gm *GitWorktreeManager) WorktreeChangeDetectionActive() bool {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.changeDetectionActive
}

// GitWatchActive reports whether the .git fsnotify sub-watch specifically is
// running (observability-only -- see WorktreeChangeDetector.GitWatchActive's
// doc comment for why this doesn't gate TTL widening).
func (gm *GitWorktreeManager) GitWatchActive() bool {
	gm.mu.RLock()
	detector := gm.changeDetector
	gm.mu.RUnlock()
	return detector != nil && detector.GitWatchActive()
}

// OnChange registers fn to run whenever the active WorktreeChangeDetector
// fires. No-op if change detection isn't active for this worktree -- the
// caller (WorkspaceService) doesn't need to check WorktreeChangeDetectionActive
// itself first.
func (gm *GitWorktreeManager) OnChange(fn func()) {
	gm.mu.RLock()
	detector := gm.changeDetector
	gm.mu.RUnlock()
	if detector != nil {
		detector.OnChange(fn)
	}
}

// touchRequestedAt records that either GetSessionDiff or GetVCSStatus just
// asked for this worktree's state -- factored out so UpdateDiffStats and
// RecordRequest share the same mutex/time.Now() pair rather than duplicating
// it.
func (gm *GitWorktreeManager) touchRequestedAt() {
	gm.mu.Lock()
	gm.lastRequestedAt = time.Now()
	gm.mu.Unlock()
}

// RecordRequest marks this worktree as recently active for
// WorktreeChangeDetector's idle-gating (Story 2.2.2). Called by
// Instance.RecordVCSStatusRequest on every GetVCSStatus call (hit or miss),
// since that RPC's own cache lives in a different package (WorkspaceService)
// and never calls UpdateDiffStats itself.
func (gm *GitWorktreeManager) RecordRequest() {
	gm.touchRequestedAt()
}

// hasRecentActivity reports whether either GetSessionDiff or GetVCSStatus
// has asked for this worktree within coldActivityThreshold. Backs
// WorktreeChangeDetector's activeFunc (Story 2.1.2) -- a cheap mutex-guarded
// time.Time read, not a filesystem stat-walk, so calling it every periodic
// tick costs nothing. Distinct from diffStatsAt: a change-fire zeroes
// diffStatsAt (the cached *value* is stale) but must not zero
// lastRequestedAt (the fact that someone is *asking* is unrelated to
// whether the last answer given is still valid).
func (gm *GitWorktreeManager) hasRecentActivity() bool {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return !gm.lastRequestedAt.IsZero() && time.Since(gm.lastRequestedAt) < coldActivityThreshold
}

// Prune cleans up stale worktree references.
func (gm *GitWorktreeManager) Prune() error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.Prune()
}

// IsDirty reports whether the worktree has uncommitted changes.
func (gm *GitWorktreeManager) IsDirty() (bool, error) {
	wt := gm.GetWorktree()
	if wt == nil {
		return false, fmt.Errorf("git worktree not initialized")
	}
	return wt.IsDirty()
}

// InvalidateDirtyCache clears the IsDirty TTL cache so the next call re-runs git status.
// Call after transitions that may change worktree dirty state (Resume, Stop).
// No-op if no worktree is set.
func (gm *GitWorktreeManager) InvalidateDirtyCache() {
	wt := gm.GetWorktree()
	if wt == nil {
		return
	}
	wt.InvalidateDirtyCache()
}

// CommitChanges stages all changes and creates a commit.
func (gm *GitWorktreeManager) CommitChanges(commitMsg string) error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.CommitChanges(commitMsg)
}

// PushChanges commits and pushes the worktree branch.
func (gm *GitWorktreeManager) PushChanges(commitMsg string, open bool) error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.PushChanges(commitMsg, open)
}

// IsBranchCheckedOut reports whether the branch is currently checked out.
func (gm *GitWorktreeManager) IsBranchCheckedOut() (bool, error) {
	wt := gm.GetWorktree()
	if wt == nil {
		return false, fmt.Errorf("git worktree not initialized")
	}
	return wt.IsBranchCheckedOut()
}

// OpenBranchURL opens the branch URL in the browser.
func (gm *GitWorktreeManager) OpenBranchURL() error {
	wt := gm.GetWorktree()
	if wt == nil {
		return fmt.Errorf("git worktree not initialized")
	}
	return wt.OpenBranchURL()
}

// ComputeDiffIfReady checks if the worktree path exists and computes a new diff.
// Returns (stats, needsPause) where needsPause is true if the worktree directory is missing.
// This method performs I/O and should be called WITHOUT holding Instance.mu.
// Returns (nil, false) if no worktree is set.
func (gm *GitWorktreeManager) ComputeDiffIfReady() (stats *git.DiffStats, needsPause bool) {
	wt := gm.GetWorktree()
	if wt == nil {
		return nil, false
	}
	worktreePath := wt.GetWorktreePath()
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return nil, true
	}
	result := wt.Diff()
	return result, false
}

// ComputeDiff runs git diff and returns the result without storing it.
// Returns nil if no worktree is set.
func (gm *GitWorktreeManager) ComputeDiff() *git.DiffStats {
	wt := gm.GetWorktree()
	if wt == nil {
		return nil
	}
	return wt.Diff()
}

// UpdateDiffStats computes a new diff and stores it.
// Returns nil and clears stats if worktree is not ready.
func (gm *GitWorktreeManager) UpdateDiffStats() {
	gm.touchRequestedAt() // every call is evidence someone is polling, hit or miss
	wt := gm.GetWorktree()
	if wt == nil {
		gm.mu.Lock()
		gm.diffStats = nil
		gm.diffStatsAt = time.Time{}
		gm.mu.Unlock()
		return
	}
	stats := wt.Diff()
	gm.mu.Lock()
	gm.diffStats = stats
	gm.diffStatsAt = time.Now()
	gm.mu.Unlock()
}

// GetDiffStats returns the most recently computed diff stats (may be nil).
func (gm *GitWorktreeManager) GetDiffStats() *git.DiffStats {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.diffStats
}

// SetDiffStats directly replaces the diff stats — used both during
// deserialization and as the final step of Instance.UpdateDiffStats's own
// orchestration (worktree and directory-mode paths alike). Stamping
// diffStatsAt here, rather than only in GitWorktreeManager.UpdateDiffStats
// (which Instance's real hot path bypasses), is what makes DiffStatsFresh
// accurate for every caller that sets a genuinely fresh value — including a
// deserialized one, which is fresh as of process startup and no less valid
// than any other cache entry for diffStatsCacheTTL's short window.
func (gm *GitWorktreeManager) SetDiffStats(stats *git.DiffStats) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.diffStats = stats
	gm.diffStatsAt = time.Now()
}

// ClearDiffStats sets diffStats to nil.
func (gm *GitWorktreeManager) ClearDiffStats() {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.diffStats = nil
	gm.diffStatsAt = time.Time{} // nil stats are never "fresh" — see DiffStatsFresh
	gm.hasCommitsAhead = false
}

// GetHasCommitsAhead returns the most recently computed "has commits ahead of
// base" signal (see SetHasCommitsAhead).
func (gm *GitWorktreeManager) GetHasCommitsAhead() bool {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.hasCommitsAhead
}

// SetHasCommitsAhead directly replaces the cached "has commits ahead of base"
// signal, refreshed on the same cadence as diff stats (see UpdateDiffStats).
func (gm *GitWorktreeManager) SetHasCommitsAhead(v bool) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.hasCommitsAhead = v
}

// GitManager is the interface satisfied by *GitWorktreeManager.
// It covers all git worktree operations used by Instance and can be implemented
// by test doubles to avoid requiring a real git repository.
type GitManager interface {
	HasWorktree() bool
	GetWorktree() *git.GitWorktree
	SetWorktree(*git.GitWorktree)
	GetWorktreePath() string
	GetRepoPath() string
	GetRepoName() string
	GetBranchName() string
	GetBaseCommitSHA() string
	Setup() error
	Cleanup() error
	Remove() error
	Prune() error
	IsDirty() (bool, error)
	InvalidateDirtyCache()
	CommitChanges(commitMsg string) error
	PushChanges(commitMsg string, open bool) error
	IsBranchCheckedOut() (bool, error)
	OpenBranchURL() error
	ComputeDiffIfReady() (stats *git.DiffStats, needsPause bool)
	ComputeDiff() *git.DiffStats
	UpdateDiffStats()
	DiffStatsFresh() bool
	GetDiffStats() *git.DiffStats
	SetDiffStats(*git.DiffStats)
	ClearDiffStats()
	GetHasCommitsAhead() bool
	SetHasCommitsAhead(bool)
	GetCurrentCommitSHA() (string, error)
	PrimeDirtyCacheJitter()
	WorktreeChangeDetectionActive() bool
	GitWatchActive() bool
	OnChange(fn func())
	RecordRequest()
}

// compile-time check that *GitWorktreeManager satisfies GitManager.
var _ GitManager = (*GitWorktreeManager)(nil)

// GetCurrentCommitSHA returns the current HEAD commit SHA for the worktree.
// Returns an empty string (not an error) if no worktree is set or the repo
// has no commits yet — this is safe to use in checkpoint creation.
func (gm *GitWorktreeManager) GetCurrentCommitSHA() (string, error) {
	dir := gm.GetWorktreePath()
	if dir == "" {
		dir = gm.GetRepoPath()
	}
	if dir == "" {
		return "", nil
	}

	revCtx, revCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer revCancel()
	cmd := safeexec.CommandContext(revCtx, "git", "-C", dir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		// Not a fatal error — repo may have no commits yet.
		return "", nil
	}
	return strings.TrimSpace(string(output)), nil
}
