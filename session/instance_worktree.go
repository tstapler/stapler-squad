package session

// instance_worktree.go contains git/worktree-related methods for Instance.
// Workspace switching is in instance_workspace.go (already extracted).
// This file covers git worktree lifecycle, diff stats, and path resolution.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
)

// RepoName returns the name of the git repository.
// Returns an error if the instance has not been started or has no worktree.
func (i *Instance) RepoName() (string, error) {
	if !i.started.Load() {
		return "", fmt.Errorf("cannot get repo name for instance that has not been started")
	}
	if i.Status == Paused {
		return "", fmt.Errorf("cannot get repo name for paused instance")
	}
	if !i.gitManager.HasWorktree() {
		return "", fmt.Errorf("gitWorktree is nil")
	}
	return i.gitManager.GetRepoName(), nil
}

// setupFirstTimeWorktree creates or attaches to the git worktree based on session type.
//
// A remote ExecutionTarget (ssh-remote-workspaces Phase 4 Epic 4.2) only supports
// SessionTypeExistingWorktree, SessionTypeDirectory, and SessionTypeNewProject:
// CreateSession's mode-specific block (server/services/session_service.go) already
// creates/resolves/git-initializes the remote working path synchronously -- via
// RemoteWorktreeOps.CreateWorktree for a real worktree, RemoteWorktreeOps.
// InitializeProjectDirectory for a new project, or trivially for a plain directory
// -- before this method ever runs, precisely so the git.NewGitWorktreeWithBranch
// path below (which does local-filesystem repo discovery: findGitRepoRoot,
// getWorktreeDirectory, findExistingWorktreeForBranch) is never reached against a
// path that only exists on the remote host. session_type is remapped to
// SessionTypeExistingWorktree with ExistingWorktree set to the remote path ONLY
// when the request originated as NewWorktree/ExistingWorktree (a real worktree); a
// Directory- or NewProject-originated remote session deliberately keeps its
// original session_type so it falls through to the default/NewProject case below
// (no worktree persisted) exactly like its local counterpart -- see those cases'
// own comments for why. Any other combination is rejected up front rather than
// silently attempting local discovery against a remote path.
func (i *Instance) setupFirstTimeWorktree() error {
	if i.executionTarget().IsRemote() && i.SessionType != SessionTypeExistingWorktree &&
		i.SessionType != SessionTypeDirectory && i.SessionType != SessionTypeNewProject {
		return fmt.Errorf("remote execution target only supports session_type=existing_worktree, session_type=directory, "+
			"or session_type=new_project (worktree/path must be pre-resolved by CreateSession's mode-specific block); got %v", i.SessionType)
	}

	switch i.SessionType {
	case SessionTypeNewWorktree:
		// This case's contract is deliberately strict and unconditional: i.Path
		// must already be an existing repo. It never bootstraps one, with no
		// CreateIfMissing exception -- see findGitRepoRoot's doc comment for why
		// that was a deliberate hardening fix (9037ed5e0), not an oversight. A
		// genuinely new project that also wants an isolated worktree goes
		// through SessionTypeNewProject below (i.Branch set), which always
		// bootstraps and then optionally worktrees off the fresh repo -- so
		// this case's contract never needs a second, parallel bootstrap path.
		log.Info("creating git worktree for instance", "session", i.Title, "path", i.Path)
		gitWorktree, branchName, err := git.NewGitWorktreeWithBranch(i.Path, i.Title, i.Branch, git.WithCommandRunner(i.executionTarget().Runner()))
		if err != nil {
			return fmt.Errorf("failed to create git worktree: %w", err)
		}
		i.gitManager.SetWorktree(gitWorktree)
		if i.Branch == "" {
			i.Branch = branchName
		}
		log.Info("git worktree created", "session", i.Title, "branch", i.Branch)
	case SessionTypeExistingWorktree:
		if i.ExistingWorktree == "" {
			return fmt.Errorf("existing worktree path required for SessionTypeExistingWorktree")
		}
		runner := i.executionTarget().Runner()
		if i.executionTarget().IsRemote() {
			// Attach to the already-created remote worktree using the field-setting
			// constructor -- NOT NewGitWorktreeFromExisting, whose IsGitRepo/
			// findMainRepoPathForWorktree/getCurrentBranchName discovery reads the
			// local filesystem and would resolve against this process's own disk,
			// not the remote host's.
			//
			// base_commit_sha must be resolved via a real remote `git rev-parse
			// HEAD` (not left "") -- the ent schema's Worktree.base_commit_sha
			// field is NotEmpty (session/ent/schema/worktree.go), so a blank value
			// fails persistence (Storage.SaveInstances) the first time this
			// instance is saved, discovered via
			// TestCreateSession_RemoteTarget_CreatesRemoteWorktreeAndTmuxSession.
			// Best-effort: mirrors NewGitWorktreeFromCommitSHA's local
			// counterparts, which likewise tolerate a lookup failure by falling
			// back to a placeholder rather than failing worktree attachment
			// entirely over a cosmetic diff-stats field.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			baseCommitSHA := "unknown"
			if out, shaErr := runner.Run(ctx, i.ExistingWorktree, "git", "rev-parse", "HEAD"); shaErr == nil {
				if sha := strings.TrimSpace(string(out)); sha != "" {
					baseCommitSHA = sha
				}
			} else {
				log.Warn("failed to resolve remote worktree base commit SHA", "session", i.Title, "path", i.ExistingWorktree, "err", shaErr)
			}
			// i.Branch is empty for a remote session composed from
			// SessionTypeDirectory/SessionTypeExistingWorktree (post-review
			// composability with ADR-001's "remote as an orthogonal flag" --
			// neither type carries an explicit branch the way NewWorktree
			// does), and Worktree.branch_name is NotEmpty just like
			// base_commit_sha above -- same best-effort resolve-from-remote
			// pattern, so persistence doesn't silently fail
			// (Storage.SaveInstances) the first time this instance is saved.
			// "HEAD" (git's own answer for a detached checkout) is treated
			// the same as a lookup failure -- it's a real string but not a
			// meaningful branch name to persist or display.
			if i.Branch == "" {
				i.Branch = "unknown"
				if out, brErr := runner.Run(ctx, i.ExistingWorktree, "git", "rev-parse", "--abbrev-ref", "HEAD"); brErr == nil {
					if br := strings.TrimSpace(string(out)); br != "" && br != "HEAD" {
						i.Branch = br
					}
				} else {
					log.Warn("failed to resolve remote worktree branch name", "session", i.Title, "path", i.ExistingWorktree, "err", brErr)
				}
			}
			cancel()
			gitWorktree := git.NewGitWorktreeFromStorage(i.Path, i.ExistingWorktree, i.Title, i.Branch, baseCommitSHA, git.WithCommandRunner(runner))
			i.gitManager.SetWorktree(gitWorktree)
			log.Info("attached to remote git worktree", "session", i.Title, "path", i.ExistingWorktree, "branch", i.Branch)
			break
		}
		log.Info("connecting to existing worktree", "session", i.Title, "path", i.ExistingWorktree)
		gitWorktree, err := git.NewGitWorktreeFromExisting(i.ExistingWorktree, i.Title, git.WithCommandRunner(runner))
		if err != nil {
			return fmt.Errorf("failed to connect to existing worktree: %w", err)
		}
		i.gitManager.SetWorktree(gitWorktree)
		i.Branch = gitWorktree.GetBranchName()
		log.Info("connected to existing worktree", "session", i.Title, "branch", i.Branch)
	case SessionTypeNewProject:
		log.Info("new project session, initializing git repo", "session", i.Title, "path", i.Path)
		// A remote instance's project directory was already git-initialized on the
		// remote host by CreateSession's mode-specific block (git.RemoteWorktreeOps.
		// InitializeProjectDirectory) -- git.InitializeProjectDirectory below is the
		// LOCAL-filesystem path (go-git PlainInit against this process's own disk)
		// and must not run a second time against i.Path, which names a remote host
		// path in that case.
		if !i.executionTarget().IsRemote() {
			if err := git.InitializeProjectDirectory(i.Path); err != nil {
				return fmt.Errorf("new_project initialization failed: %w", err)
			}
		}
		// web-app's "New Project, opened as New Worktree" (Omnibar) sends an
		// explicit i.Branch alongside SessionTypeNewProject rather than
		// SessionTypeNewWorktree -- i.Path is guaranteed to be a real repo from
		// InitializeProjectDirectory above, so a worktree can be created on it
		// the same way SessionTypeNewWorktree does for an already-existing repo.
		// "Open as: Directory" leaves i.Branch empty and falls through to the
		// no-worktree branch below, unchanged from before this bootstrap
		// capability existed. Remote targets don't support this combination yet
		// (CreateSession's remote mode-specific block has no worktree-creation
		// case for SessionTypeNewProject) -- a real, pre-existing limitation,
		// not one introduced here; i.Branch is simply ignored for remote.
		if i.Branch != "" && !i.executionTarget().IsRemote() {
			gitWorktree, branchName, err := git.NewGitWorktreeWithBranch(i.Path, i.Title, i.Branch, git.WithCommandRunner(i.executionTarget().Runner()))
			if err != nil {
				return fmt.Errorf("new_project worktree creation failed: %w", err)
			}
			i.gitManager.SetWorktree(gitWorktree)
			i.Branch = branchName
			log.Info("new project initialized with worktree", "path", i.Path, "branch", i.Branch)
			return nil
		}
		i.gitManager.SetWorktree(nil)
		i.Branch = ""
		log.Info("new project initialized", "path", i.Path)
	default: // SessionTypeDirectory and unknown types → no worktree
		log.Info("directory session, no git worktree", "session", i.Title, "path", i.Path)
		// EnsureDirectorySessionPath does local-filesystem os.Stat/git-init -- correct for
		// a local Directory session, but i.Path names a REMOTE host path for a remote one,
		// so running it here would silently create/git-init the wrong directory on this
		// server's own disk. create_if_missing is not yet supported for remote Directory
		// sessions (CreateSession's own request-validation layer already skips its
		// existence check for this combination -- see session_service.go's CreateSession),
		// so this just no-ops rather than acting on a path it cannot correctly resolve.
		if i.CreateIfMissing && !i.executionTarget().IsRemote() {
			if err := EnsureDirectorySessionPath(i.Path); err != nil {
				return fmt.Errorf("failed to create directory for session: %w", err)
			}
		}
		i.gitManager.SetWorktree(nil)
		i.Branch = ""
	}
	return nil
}

// EnsureDirectorySessionPath creates and git-inits path if it does not already exist —
// the same directory-creation step SessionTypeDirectory takes when CreateIfMissing is set.
// Callers that need path to exist before spawning a directory session (e.g. to write files
// into the worktree ahead of the claude process starting) should call this first so the
// spawn's own CreateIfMissing check finds the directory already present and correctly
// git-initialized, rather than skipping git-init because the path merely exists.
func EnsureDirectorySessionPath(path string) error {
	_, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		return git.InitializeProjectDirectory(path)
	case err != nil:
		return fmt.Errorf("failed to stat session path %q: %w", path, err)
	default:
		return nil
	}
}

// BacklogBranchPrefix is the git branch prefix every backlog work session's
// branch is created under — the one place this literal is defined. Anything
// that needs to independently predict or recreate a backlog work session's
// branch name (e.g. server/services/backlog_service_triage.go's
// retitleTriageWorktreeToFinalBranch, which renames a triage worktree onto the
// exact branch a later real spawn will look for) must reference this constant
// rather than hardcoding "backlog/" — see that function's doc comment for the
// bug this once caused when the two sides used independently-duplicated logic.
const BacklogBranchPrefix = "backlog/"

// CreateBacklogWorktree creates a git worktree for a backlog work session.
// It creates a branch named BacklogBranchPrefix+branchSuffix and returns the
// on-disk worktree path. The caller is responsible for writing files to the
// path before spawning the session.
//
// A brand-new branch is based on the repo's real default branch's freshly-fetched origin
// tip (git.ResolveDefaultBranchSHA — tries "main", "master", "develop", "trunk" in turn,
// since a repo's default branch can be named anything: this repo's own sibling dotfiles
// project uses "master", not "main"), not repoPath's ambient HEAD
// (git.NewGitWorktreeWithBranch's default, correct for an interactive "branch off my
// current checkout" ad-hoc worktree, but wrong here: a queue-driven backlog spawn has no
// human on a particular branch, and repoPath's own checkout can sit unfetched for days,
// or be checked out to whatever branch a concurrent process — including another
// triaging/reporting session sharing this same repoPath — last left it on). A stale
// ambient HEAD as the recorded base_commit_sha meant every commit that later landed on
// the default branch between that stale point and whatever this session's HEAD later
// resolved to got misattributed to the session as its own work (surfaced as
// inflated/wrong commit_count_since_spawn — see resolveLatestWorkCommit's doc comment
// for the sibling bug this was found alongside). If every candidate's origin fetch fails
// (offline, no origin remote), falls back to a local candidate branch's tip — still
// guaranteed to be the real default branch, just not guaranteed fresh — and only errors
// out if even that doesn't exist. Never falls back to ambient HEAD: a wrong-but-loud
// failure beats a spawn silently branching from an unrelated branch.
//
// The repair, branch resolution, worktree construction, and setup all run inside a single
// git.WithRepoWorktreeLock critical section for resolvedRepo. RepairCorruptedGitRepo's
// os.RemoveAll+re-clone used to run unlocked, racing a concurrent spawn's locked
// `git worktree add` on the same repo — one process's repair could delete/recreate the repo
// out from under another process mid-add, surfacing as git's generic "fatal: failed to
// resolve HEAD as a valid ref". Setup runs via wt.SetupLocked() rather than wt.Setup() here
// because this goroutine already holds the (non-reentrant) lock.
// baseBranch, when non-empty, is an explicit opt-in override (BacklogItem.
// BaseBranch): the branch is resolved by name (origin, falling back to
// local) instead of the repo's default branch, and any resolution failure
// (including "branch doesn't exist anywhere") is a hard error — never a
// silent fallback to the default branch or ambient HEAD, since the caller
// asked for this branch specifically.
func CreateBacklogWorktree(repoPath, branchSuffix, baseBranch string) (string, error) {
	if repoPath == "" {
		// ResolveSessionPath("") silently resolves to the process's own cwd
		// (the live server's own checkout) instead of erroring — a BacklogItem
		// can reach here with RepoPath still "" (nothing upstream requires it;
		// see SpawnSessionFromItem's own guard), so reject before that happens.
		return "", fmt.Errorf("CreateBacklogWorktree: repoPath must not be empty")
	}
	// ResolveMainRepoRoot anchors everything below (repair, lock, fetch, worktree
	// add) to the real repo root, not to whatever worktree path repoPath happened
	// to be stored as — see its doc comment for why that matters (WithRepoWorktreeLock's
	// lock key and RemoveWorktree's cleanup both key off this path).
	resolvedRepo, err := ResolveMainRepoRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("CreateBacklogWorktree: %w", err)
	}

	var worktreePath string
	err = git.WithRepoWorktreeLock(resolvedRepo, func() error {
		if err := RepairCorruptedGitRepo(resolvedRepo); err != nil {
			return err
		}
		branchName := BacklogBranchPrefix + branchSuffix
		var wt *git.GitWorktree
		var err error
		var defaultBranch, baseSHA string
		var resolveErr error
		if baseBranch != "" {
			defaultBranch = baseBranch
			baseSHA, resolveErr = git.ResolveExplicitBranchSHA(resolvedRepo, baseBranch)
		} else {
			defaultBranch, baseSHA, resolveErr = git.ResolveWorktreeBaseCommit(resolvedRepo)
		}
		switch {
		case resolveErr != nil && baseBranch != "":
			return fmt.Errorf("resolve base_branch %q: %w", baseBranch, resolveErr)
		case resolveErr != nil:
			return fmt.Errorf("resolve default branch: %w", resolveErr)
		case baseSHA != "":
			if baseBranch != "" {
				log.Debug("CreateBacklogWorktree: branching from explicit base_branch override", "repoPath", resolvedRepo, "baseBranch", baseBranch, "baseSHA", baseSHA)
			} else {
				log.Debug("CreateBacklogWorktree: branching from default branch", "repoPath", resolvedRepo, "defaultBranch", defaultBranch, "baseSHA", baseSHA)
			}
			wt, _, err = git.NewGitWorktreeFromCommitSHA(resolvedRepo, branchSuffix, branchName, baseSHA)
		default:
			// No commits exist anywhere in resolvedRepo yet (IsUnbornRepo), so ambient
			// HEAD carries no risk of branching from an unrelated branch's work — there
			// is no other branch. Preserves the auto-create-initial-commit behavior
			// findGitRepoRoot already provides for a brand-new, never-committed-to repo.
			log.Warn("no default branch found and repo has no commits yet, branching from empty ambient HEAD", "repoPath", resolvedRepo)
			wt, _, err = git.NewGitWorktreeWithBranch(resolvedRepo, branchSuffix, branchName)
		}
		if err != nil {
			return err
		}
		if err := wt.SetupLocked(); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		worktreePath = wt.GetWorktreePath()
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("CreateBacklogWorktree: %w", err)
	}
	return worktreePath, nil
}

// resolveStartPath returns the effective start directory, applying WorkingDir on top of basePath.
// Falls back to basePath if the resolved directory does not exist.
// For worktree sessions, absolute WorkingDir paths outside the worktree are ignored to prevent
// stale CWD snapshots (from CaptureCurrentState) from overriding the worktree root.
func (i *Instance) resolveStartPath(basePath string) string {
	if i.WorkingDir == "" {
		return basePath
	}
	startPath := i.WorkingDir
	if !filepath.IsAbs(i.WorkingDir) {
		startPath = filepath.Join(basePath, i.WorkingDir)
	} else if i.gitManager.HasWorktree() && pathEscapesRoot(basePath, startPath) {
		// For worktree sessions, an absolute WorkingDir must be within the worktree.
		// CaptureCurrentState() can persist the process CWD (e.g. the main repo path
		// when Claude cd's there) — this is a read-side backstop for that; see
		// pathEscapesRoot's doc comment for the write-side gate that now also exists
		// (BUG-033) and for why this check alone isn't sufficient on its own: it only
		// fires when i.gitManager.HasWorktree() is already true at the moment a
		// session (re)starts, which is not guaranteed on every restart ordering.
		log.Warn("working dir is outside worktree, using worktree path", "path", startPath, "worktree", basePath)
		return basePath
	}
	if _, err := os.Stat(startPath); os.IsNotExist(err) {
		log.Warn("working directory doesn't exist, using fallback", "path", startPath, "fallback", basePath)
		return basePath
	}
	return startPath
}

// pathEscapesRoot reports whether candidate is outside root (or root/candidate
// can't be compared at all, treated as escaped — fail closed). Shared by
// resolveStartPath (read-side backstop) and CaptureCurrentState (write-side
// gate, BUG-033) so a worktree session's persisted WorkingDir can never
// silently point outside its own isolated worktree in the first place —
// see CaptureCurrentState's doc comment for the live incident this closes:
// an agent that `cd`s into the parent repo (e.g. to "mirror" a branch) had
// that path captured and persisted with no validation, and depending on
// gitManager's re-initialization ordering on the next restart, resolveStartPath's
// own guard could be bypassed, starting the session's next run directly in the
// shared parent checkout instead of its worktree.
func pathEscapesRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err != nil || strings.HasPrefix(rel, "..")
}

// GetEffectiveRootDir returns the root directory where this session operates.
// For worktree sessions, this is the worktree path. For directory sessions, this is Path.
// Used for injecting configuration files (e.g., .claude/settings.local.json).
//
// This returns the worktree path as recorded, without checking whether it
// still exists on disk — callers that do path-string correlation (e.g.
// HistoryLinker matching against ~/.claude/projects/<hashed-path>) need the
// nominal path regardless of whether the directory is currently present. For
// callers that need to actually read from the filesystem, use Workspace(),
// which falls back to the repo root when the worktree is gone.
func (i *Instance) GetEffectiveRootDir() string {
	if i.gitManager.HasWorktree() {
		if p := i.gitManager.GetWorktreePath(); p != "" {
			return p
		}
	}
	return i.GetPath()
}

// GetPath returns the instance's repository root path. Written under
// i.mu by setGitHubResolutionLocked once deferred GitHub URL resolution
// completes in the background, so reads must go through this lock-free
// accessor (the published atomic Snapshot) rather than the bare i.Path field.
func (i *Instance) GetPath() string {
	return i.Snapshot().Path
}

// Workspace returns where this session is operating.
// Use this as the single source of truth for path resolution instead of
// accessing inst.Path directly, which is wrong for worktree sessions.
//
// Falls back to RepoRoot if the worktree path no longer exists on disk (e.g.
// a paused session's worktree was removed while its branch/metadata
// persisted) — otherwise filesystem-reading callers like ListFiles would try
// to read a directory that's gone and surface a bare "directory not found: ."
// with no indication why.
func (i *Instance) Workspace() Workspace {
	repoRoot := i.GetPath()
	effectivePath := i.GetEffectiveRootDir()
	if effectivePath != repoRoot {
		if _, err := os.Stat(effectivePath); err != nil {
			log.Warn("worktree path no longer exists on disk, falling back to repo path", "session", i.Title, "worktreePath", effectivePath)
			effectivePath = repoRoot
		}
	}
	return Workspace{
		EffectivePath: effectivePath,
		RepoRoot:      repoRoot,
	}
}

// CleanupWorktree removes the git worktree, keeping session intact.
func (i *Instance) CleanupWorktree() error {
	if i.gitManager.HasWorktree() {
		if err := i.gitManager.Cleanup(); err != nil {
			return fmt.Errorf("failed to cleanup git worktree: %w", err)
		}
	}
	return nil
}

// GetGitWorktree returns the git worktree for the instance.
func (i *Instance) GetGitWorktree() (*git.GitWorktree, error) {
	if !i.started.Load() {
		return nil, fmt.Errorf("cannot get git worktree for instance that has not been started")
	}
	return i.gitManager.GetWorktree(), nil
}

// HasGitWorktree returns true if the instance has a git worktree.
func (i *Instance) HasGitWorktree() bool {
	return i.gitManager.HasWorktree()
}

// GetBaseCommitSHA returns the commit SHA this session's branch diverged
// from, falling back to the directory-mode base SHA when there's no worktree
// (SessionTypeDirectory, the default session type) — mirrors
// computeDirDiffStats's HasWorktree()-gated pattern. Returns "" if neither is
// set yet.
func (i *Instance) GetBaseCommitSHA() string {
	if i.gitManager.HasWorktree() {
		return i.gitManager.GetBaseCommitSHA()
	}
	return i.gitManager.GetDirBaseSHA()
}

// SetGitWorktree sets the git worktree for testing purposes.
func (i *Instance) SetGitWorktree(worktree *git.GitWorktree) {
	i.gitManager.SetWorktree(worktree)
	i.started.Store(worktree != nil)
}

// UpdateDiffStats updates the git diff statistics for this instance.
// Performs I/O (git diff) outside the lock, then updates state under the write lock.
func (i *Instance) UpdateDiffStats() error {
	// Read lock for initial state checks
	i.mu.RLock()
	if !i.started.Load() {
		i.gitManager.ClearDiffStats()
		i.mu.RUnlock()
		return nil
	}
	if i.Status == Paused {
		i.mu.RUnlock()
		return nil
	}
	if !i.gitManager.HasWorktree() {
		dirBase := i.gitManager.GetDirBaseSHA()
		workingDir := i.WorkingDir
		if workingDir == "" {
			workingDir = i.Path
		}
		i.mu.RUnlock()
		if dirBase == "" {
			return nil
		}
		// Directory session with a known base SHA — compute diff outside the lock.
		stats := computeDirDiffStats(workingDir, dirBase)
		i.mu.Lock()
		i.gitManager.SetDiffStats(stats)
		i.mu.Unlock()
		return nil
	}
	i.mu.RUnlock()

	// I/O outside lock: check worktree existence and compute diff
	stats, needsPause := i.gitManager.ComputeDiffIfReady()

	// Compute "has commits ahead of base" alongside the diff stats, also outside
	// the lock — AC6 needs this cached on Session so the frontend can disable the
	// "Create PR" trigger before the user clicks, without a synchronous
	// DraftPullRequest round trip. Skipped when the worktree needs pausing, same
	// as diff stats (cleared below in that case).
	var hasCommits bool
	if !needsPause {
		if wt := i.gitManager.GetWorktree(); wt != nil {
			// bounceMainBranch, not a dynamically-resolved default branch: mirrors
			// pushAndCreatePR's own HasCommitsAheadOfMain call
			// (session/backlog_lifecycle_pr.go:564). Resolving the actual default
			// branch the way DraftPullRequest does
			// (unfinished.GoGitVCSReader.ResolveDefaultBranch, server/services/pr_creation_service.go:139)
			// isn't available from this package: session/unfinished imports
			// pkg/events, which imports session — an import cycle back into this
			// file (documented in session/worktree_pr_poller.go). Fails open
			// (HasCommitsAheadOfMain returns true on error by contract) — ignore
			// the error and trust the bool.
			hasCommits, _ = wt.HasCommitsAheadOfMain(bounceMainBranch)
		}
	}

	// Write lock to update state — keep non-logging work only to minimise hold time.
	i.mu.Lock()
	var transitionErr error
	var didTransitionToPaused bool
	if needsPause {
		if i.Status != Paused {
			didTransitionToPaused = true
			transitionErr = i.transitionTo(context.Background(), Paused)
		}
		i.gitManager.ClearDiffStats()
		i.mu.Unlock()
		if didTransitionToPaused {
			log.Warn("worktree directory doesn't exist, marking as paused", "session", i.Title)
		}
		if transitionErr != nil {
			log.Warn("failed to transition to paused", "session", i.Title, "err", transitionErr)
		}
		return nil
	}
	if stats != nil && stats.Error != nil {
		if strings.Contains(stats.Error.Error(), "base commit SHA not set") {
			i.gitManager.ClearDiffStats()
			i.mu.Unlock()
			return nil
		}
		i.mu.Unlock()
		return fmt.Errorf("failed to get diff stats: %w", stats.Error)
	}
	i.gitManager.SetDiffStats(stats)
	i.gitManager.SetHasCommitsAhead(hasCommits)
	i.mu.Unlock()
	return nil
}

// GetDiffStats returns the current git diff statistics.
func (i *Instance) GetDiffStats() *git.DiffStats {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.gitManager.GetDiffStats()
}

// GetHasCommitsAhead returns the cached signal for whether the session's
// branch currently has commits ahead of its base branch (see UpdateDiffStats,
// which refreshes it on the same cadence as diff stats).
func (i *Instance) GetHasCommitsAhead() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.gitManager.GetHasCommitsAhead()
}

// SetDirBaseSHA sets the base commit SHA used to compute diff stats for
// directory-mode sessions (sessions without an isolated git worktree).
func (i *Instance) SetDirBaseSHA(sha string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.gitManager.SetDirBaseSHA(sha)
}

// computeDirDiffStats computes diff stats for a directory session — the
// committed-range equivalent of `git diff baseSHA..HEAD` — via go-git
// (session/git.DiffContentBetween), avoiding a subshell on this poll-cadence
// path (see the `prefer-go-git-over-subshells` skill; profiling on
// 2026-09-02 found subshell ForkLock contention was 77% of all mutex-profile
// delay). Returns nil on any error (diff is optional/cosmetic).
func computeDirDiffStats(repoPath, baseSHA string) *git.DiffStats {
	headSHA, err := git.GetHeadCommitSHA(repoPath)
	if err != nil {
		return nil
	}
	stats, err := git.DiffContentBetween(repoPath, baseSHA, headSHA)
	if err != nil {
		return nil
	}
	return stats
}

// GetWorkingDirectory returns the working directory for this instance.
func (i *Instance) GetWorkingDirectory() string {
	if i.gitManager.HasWorktree() {
		return i.gitManager.GetWorktreePath()
	}
	return i.GetPath()
}

// DetectAndPopulateWorktreeInfo detects if the instance path is a worktree
// and populates the IsWorktree, MainRepoPath, GitHubOwner, and GitHubRepo fields.
//
// The writes are routed through applyWorktreeDetectionLocked (instance_actor_setters.go),
// which takes i.mu.Lock() and republishes the snapshot exactly like
// setGitHubResolutionLocked -- this closes the write/write race backlog item
// 10fc3913 found between the two (reproduced with `go test -race` racing this
// method's writes against a concurrent setGitHubResolutionLocked call).
//
// The i.Path *read* below deliberately stays the raw field, not i.GetPath():
// at least one caller in the session-creation/retry/trigger pipeline sets
// i.Path via a raw assignment without republishing the atomic snapshot before
// this method runs, so GetPath() would observe a stale cached value instead
// of the fresh field -- confirmed by `go test ./server/services/...`
// regressing (TestBackgroundResolutionPipeline_*, TestSessionService_RetrySession_*,
// TestTriggerTriage_*) when this was tried. Both current call sites
// (NewInstance-style construction in instance.go, and deserialization in
// instance_serialization.go) run before the instance is shared with any
// other goroutine, so this read isn't itself a reachable race today.
// This is useful for sessions created from existing worktrees where we want to
// display the actual repository information in the UI.
//
// IMPORTANT: For sessions with git worktrees, we check BOTH paths:
// 1. The worktree path (gitWorktree.GetWorktreePath()) - to detect IsWorktree and MainRepoPath
// 2. The original path (i.Path) - as fallback for GitHub owner/repo if worktree detection fails
//
// This is necessary because:
// - i.Path is the main repository path (e.g., ~/Documents/personal-wiki)
// - gitWorktree.GetWorktreePath() is the actual worktree (e.g., ~/.stapler-squad/worktrees/...)
// - The main repo has .git as a directory; the worktree has .git as a file pointing to the main repo
func (i *Instance) DetectAndPopulateWorktreeInfo() error {
	// Determine the path to use for detection
	// For worktree sessions, use the worktree path; otherwise use i.Path
	// (raw field, deliberately not GetPath() -- see this function's doc
	// comment for why: GetPath() regressed real session-creation tests here).
	detectPath := i.Path
	if i.gitManager.HasWorktree() {
		worktreePath := i.gitManager.GetWorktreePath()
		if worktreePath != "" {
			detectPath = worktreePath
		}
	}

	if detectPath == "" {
		return nil
	}

	info, err := DetectWorktree(detectPath)
	if err != nil {
		return err
	}

	return i.sendSyncErr(func(s *instanceState) error {
		applyWorktreeDetectionLocked(s, info)
		return nil
	})
}
