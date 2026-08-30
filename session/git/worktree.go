package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
	"golang.org/x/sync/singleflight"
)

// getWorktreeDirectory returns the base directory fresh worktree paths are computed
// under. It must return a symlink-resolved path: git itself resolves symlinks when it
// records a worktree's path in .git/worktrees/<id>/gitdir, so findExistingWorktreeForBranch
// (which reads that path back via `git worktree list`) sees the resolved form. If this
// function returned an unresolved path (e.g. macOS's /tmp -> /private/tmp, or any
// symlinked/NFS-automounted config dir), a freshly-computed path and the same worktree's
// git-reported path would differ as strings despite naming the identical directory --
// exactly the mismatch that broke worktree-reuse identity checks (see
// TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession).
func getWorktreeDirectory() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(configDir, "worktrees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create worktree base directory %s: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve worktree base directory %s: %w", dir, err)
	}
	return resolved, nil
}

// IsDirtyCacheTTL is the duration for which a dirty (has changes) result is considered fresh.
// 30s keeps the review queue responsive when uncommitted changes are present.
// InvalidateDirtyCache() is called after commits/pushes so critical paths remain snappy.
const IsDirtyCacheTTL = 30 * time.Second

// IsDirtyCleanCacheTTL is the TTL when the worktree is known to be clean.
// Clean worktrees won't change unless Claude commits or a user modifies files;
// InvalidateDirtyCache() is called on those code paths, so 5 min is safe and
// cuts subprocess calls by ~10x vs dirty-path TTL for quiescent sessions.
const IsDirtyCleanCacheTTL = 5 * time.Minute

// IsDirtyErrorCacheTTL is the TTL applied when `git status` itself fails (e.g. the
// worktree directory is missing — a stale path left behind by a rework/reopen cycle).
// Without a backoff, a broken worktree gets re-checked on every poller tick (every few
// seconds), burning a subprocess spawn per tick indefinitely; 60s keeps failure visible
// in logs at a sane rate while still recovering quickly once the worktree is fixed.
const IsDirtyErrorCacheTTL = 60 * time.Second

// dirtyCacheState is the immutable snapshot stored in GitWorktree.isDirtyCache.
// atomic.Value replaces the previous sync.RWMutex + two fields; readers do a
// lock-free Load() on the hot per-tick path.
type dirtyCacheState struct {
	dirty bool
	time  time.Time
	err   error
}

// GitWorktree manages git worktree operations for a session
type GitWorktree struct {
	// Path to the repository
	repoPath string
	// Path to the worktree
	worktreePath string
	// Name of the session
	sessionName string
	// Branch name for the worktree
	branchName string
	// Base commit hash for the worktree
	baseCommitSHA string
	// runner is the CommandRunner execution seam (session/tmux, reused here
	// per ADR-002) every git/gh subprocess invocation in this package routes
	// through unconditionally. Defaults to tmux.LocalRunner{} via
	// commandRunner() below when unset, so every existing construction path
	// keeps working unchanged. Phase 2 (SSH remote workspaces) swaps this at
	// construction time for a remote-backed implementation; tests swap it
	// via WithCommandRunner for a spy. This package previously also carried
	// a cmdExec executor.Executor field (a plain, never circuit-breaker-
	// wrapped test-injection seam predating this one) — removed once
	// runGitCommand's last cmdExec-gated branch was migrated onto runner,
	// since nothing else in the package ever read it. See ADR-002's
	// addendum.
	runner tmux.CommandRunner

	// ponytail: atomic.Value replaces sync.RWMutex+bool+time — lock-free reads on the fast cache-hit path
	isDirtyCache atomic.Value // stores dirtyCacheState; zero value = cache invalid

	// isDirtySF coalesces concurrent dirty-checks on the same worktree so only
	// one in-process status check runs at a time.
	isDirtySF singleflight.Group //nolint:exhaustruct
}

// GitWorktreeOption is a functional option for GitWorktree construction,
// mirroring session/tmux's TmuxSessionOption. Trailing/variadic on every
// constructor that builds a *GitWorktree, so existing call sites are
// unaffected.
type GitWorktreeOption func(*GitWorktree)

// WithCommandRunner injects a CommandRunner, overriding the
// tmux.LocalRunner{} default every constructor otherwise applies. Used to
// swap in a remote-backed CommandRunner (Phase 2 of ssh-remote-workspaces)
// or a test spy that records/controls what the worktree's git/gh subprocess
// calls do.
func WithCommandRunner(r tmux.CommandRunner) GitWorktreeOption {
	return func(g *GitWorktree) {
		g.runner = r
	}
}

// NewGitWorktreeFromCommitSHA creates a new GitWorktree that will branch from the given
// commitSHA when Setup() is called, instead of branching from the current HEAD.
// This is used by ForkFromCheckpoint to recreate the exact git state at checkpoint time.
func NewGitWorktreeFromCommitSHA(repoPath, sessionName, branchName, commitSHA string, opts ...GitWorktreeOption) (*GitWorktree, string, error) {
	if commitSHA == "" {
		return nil, "", fmt.Errorf("commitSHA must not be empty")
	}
	if repoPath == "" {
		// filepath.Abs("") below would otherwise silently resolve to the
		// process's own cwd instead of erroring — reject up front so a caller
		// that failed to resolve a real path fails loudly here.
		return nil, "", fmt.Errorf("repoPath must not be empty")
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		log.Error("git worktree path abs error, falling back to repoPath", "path", repoPath, "err", err)
		absPath = repoPath
	}

	resolvedRepoPath, err := findGitRepoRoot(absPath)
	if err != nil {
		return nil, "", err
	}

	worktreeDir, err := getWorktreeDirectory()
	if err != nil {
		return nil, "", err
	}

	sanitizedName := sanitizeBranchName(sessionName)
	worktreePath, err := joinWithinDir(worktreeDir, sanitizedName)
	if err != nil {
		return nil, "", err
	}
	worktreePath = worktreePath + "_" + fmt.Sprintf("%x", time.Now().UnixNano())

	g := &GitWorktree{
		repoPath:      resolvedRepoPath,
		sessionName:   sessionName,
		branchName:    branchName,
		worktreePath:  worktreePath,
		baseCommitSHA: commitSHA,
		runner:        tmux.LocalRunner{},
	}
	for _, opt := range opts {
		opt(g)
	}
	return g, branchName, nil
}

func NewGitWorktreeFromStorage(repoPath string, worktreePath string, sessionName string, branchName string, baseCommitSHA string, opts ...GitWorktreeOption) *GitWorktree {
	return NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, sessionName, branchName, baseCommitSHA, opts...)
}

// NewGitWorktreeFromStorageWithExecutor creates a GitWorktree from stored data.
// The "WithExecutor" name predates CommandRunner (ADR-002): this used to also accept
// an optional executor.Executor parameter, removed once runGitCommand's last
// executor.Executor-gated branch was migrated onto CommandRunner and nothing else
// in the package read it (see ADR-002's addendum) — use WithCommandRunner instead
// to override how this worktree's subprocesses run.
func NewGitWorktreeFromStorageWithExecutor(repoPath string, worktreePath string, sessionName string, branchName string, baseCommitSHA string, opts ...GitWorktreeOption) *GitWorktree {
	// Return nil if the worktree has no actual paths (empty/invalid worktree)
	if repoPath == "" && worktreePath == "" && branchName == "" {
		return nil
	}

	// Rehydrated worktreePath values may predate this normalization (persisted
	// before CanonicalizeWorktreePath existed) or may already be canonical — either
	// way, normalize on read so in-memory comparisons stay consistent without a data
	// migration. Best-effort and non-fatal: if the directory no longer exists on
	// disk (deleted worktree, stale storage entry), skip normalization rather than
	// let EvalSymlinks's ENOENT propagate — CanonicalizeWorktreePath itself already
	// falls back to filepath.Clean on error, but we gate on os.Stat first so we
	// don't even attempt symlink resolution against a path known to be gone.
	if worktreePath != "" {
		if _, statErr := os.Stat(worktreePath); statErr == nil {
			worktreePath = CanonicalizeWorktreePath(worktreePath)
		}
	}

	g := &GitWorktree{
		repoPath:      repoPath,
		worktreePath:  worktreePath,
		sessionName:   sessionName,
		branchName:    branchName,
		baseCommitSHA: baseCommitSHA,
		runner:        tmux.LocalRunner{},
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// NewGitWorktree creates a new GitWorktree instance
func NewGitWorktree(repoPath string, sessionName string, opts ...GitWorktreeOption) (tree *GitWorktree, branchname string, err error) {
	return NewGitWorktreeWithBranchAndExecutor(repoPath, sessionName, "", opts...)
}

// NewGitWorktreeWithBranch creates a new GitWorktree instance with an optional custom branch name
func NewGitWorktreeWithBranch(repoPath string, sessionName string, customBranch string, opts ...GitWorktreeOption) (tree *GitWorktree, branchname string, err error) {
	return NewGitWorktreeWithBranchAndExecutor(repoPath, sessionName, customBranch, opts...)
}

// NewGitWorktreeWithBranchAndExecutor creates a new GitWorktree with an optional branch name.
// The "WithExecutor" name predates CommandRunner (ADR-002) — see
// NewGitWorktreeFromStorageWithExecutor's doc comment; use WithCommandRunner to
// override how this worktree's subprocesses run.
func NewGitWorktreeWithBranchAndExecutor(repoPath string, sessionName string, customBranch string, opts ...GitWorktreeOption) (tree *GitWorktree, branchname string, err error) {
	if repoPath == "" {
		// See NewGitWorktreeFromCommitSHA's identical check.
		return nil, "", fmt.Errorf("repoPath must not be empty")
	}

	cfg := config.LoadConfig()

	var branchName string
	if customBranch != "" {
		// Use the custom branch name directly
		branchName = customBranch
	} else {
		// Generate branch name from session name
		sanitizedName := sanitizeBranchName(sessionName)
		branchName = fmt.Sprintf("%s%s", cfg.BranchPrefix, sanitizedName)
	}

	// Convert repoPath to absolute path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		log.Error("git worktree path abs error, falling back to repoPath", "path", repoPath, "err", err)
		// If we can't get absolute path, use original path as fallback
		absPath = repoPath
	}

	repoPath, err = findGitRepoRoot(absPath)
	if err != nil {
		return nil, "", err
	}

	worktreeDir, err := getWorktreeDirectory()
	if err != nil {
		return nil, "", err
	}

	// First check if the branch is already checked out in an existing worktree
	existingWorktreePath, found := findExistingWorktreeForBranch(repoPath, branchName)
	if found {
		// git realpath's the path it reports in 'worktree list' output, so
		// canonicalize before storing to keep this consistent with the
		// already-resolved paths getWorktreeDirectory hands to fresh creates.
		existingWorktreePath = CanonicalizeWorktreePath(existingWorktreePath)
		log.Info("found existing worktree for branch, reusing it", "branch", branchName, "path", existingWorktreePath)
		g := &GitWorktree{
			repoPath:     repoPath,
			sessionName:  sessionName,
			branchName:   branchName,
			worktreePath: existingWorktreePath,
			runner:       tmux.LocalRunner{},
		}
		for _, opt := range opts {
			opt(g)
		}
		return g, branchName, nil
	}

	// No existing worktree found, create a new one with timestamp suffix
	sanitizedName := sanitizeBranchName(sessionName)
	worktreePath, err := joinWithinDir(worktreeDir, sanitizedName)
	if err != nil {
		return nil, "", err
	}
	worktreePath = worktreePath + "_" + fmt.Sprintf("%x", time.Now().UnixNano())

	g := &GitWorktree{
		repoPath:     repoPath,
		sessionName:  sessionName,
		branchName:   branchName,
		worktreePath: worktreePath,
		runner:       tmux.LocalRunner{},
	}
	for _, opt := range opts {
		opt(g)
	}
	return g, branchName, nil
}

// PreviewWorktreePath returns the directory PREFIX a new worktree would be created
// under - the same filepath.Join(worktreeDir, sanitizeBranchName(sessionName)) that
// NewGitWorktreeWithBranchAndExecutor computes, WITHOUT the "_<random-suffix>" it
// appends at actual creation time (that suffix can't be predicted ahead of the call).
// Performs no git subprocess calls (deliberately skips the existing-worktree-for-branch
// lookup, which shells out to "git worktree list") and, unlike findGitRepoRoot, never
// mutates the filesystem: it only walks up from repoPath looking for an existing git
// repo. A preview is called on every Omnibar keystroke, so it must be a pure read - it
// must not create directories, run `git init`, or create commits the way
// findGitRepoRoot's create-if-missing fallback does for the real creation path.
func PreviewWorktreePath(repoPath, sessionName string) (string, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}

	if _, err := findExistingGitRepoRootReadOnly(absPath); err != nil {
		return "", err
	}

	worktreeDir, err := getWorktreeDirectory()
	if err != nil {
		return "", err
	}

	sanitizedName := sanitizeBranchName(sessionName)
	return joinWithinDir(worktreeDir, sanitizedName)
}

// findExistingGitRepoRootReadOnly walks up from path looking for an existing git
// repository, without creating or modifying anything on disk. Unlike findGitRepoRoot,
// it errors on a missing directory and does not require the repo to have any commits.
func findExistingGitRepoRootReadOnly(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("path does not exist: %s", path)
	}

	currentPath := path
	for {
		if _, err := git.PlainOpen(currentPath); err == nil {
			return currentPath, nil
		}

		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			return "", fmt.Errorf("failed to find Git repository root from path: %s", path)
		}
		currentPath = parent
	}
}

// GetWorktreePath returns the path to the worktree
func (g *GitWorktree) GetWorktreePath() string {
	return g.worktreePath
}

// commandRunner returns g.runner, defaulting to tmux.LocalRunner{} when
// unset. All constructors set runner explicitly, but this lazy default also
// covers any GitWorktree built without going through them.
func (g *GitWorktree) commandRunner() tmux.CommandRunner {
	if g.runner == nil {
		return tmux.LocalRunner{}
	}
	return g.runner
}

// GetBranchName returns the name of the branch associated with this worktree
func (g *GitWorktree) GetBranchName() string {
	return g.branchName
}

// GetRepoPath returns the path to the repository
func (g *GitWorktree) GetRepoPath() string {
	return g.repoPath
}

// GetRepoName returns the name of the repository (last part of the repoPath).
func (g *GitWorktree) GetRepoName() string {
	return filepath.Base(g.repoPath)
}

// GetBaseCommitSHA returns the base commit SHA for the worktree
func (g *GitWorktree) GetBaseCommitSHA() string {
	return g.baseCommitSHA
}

// NewGitWorktreeFromExisting creates a GitWorktree from an existing worktree path
// This is used when connecting to worktrees that were created manually or by deleted sessions
func NewGitWorktreeFromExisting(existingWorktreePath string, sessionName string, opts ...GitWorktreeOption) (*GitWorktree, error) {
	return NewGitWorktreeFromExistingWithExecutor(existingWorktreePath, sessionName, opts...)
}

// NewGitWorktreeFromExistingWithExecutor creates a GitWorktree from an existing worktree path.
// The "WithExecutor" name predates CommandRunner (ADR-002) — see
// NewGitWorktreeFromStorageWithExecutor's doc comment; use WithCommandRunner to
// override how this worktree's subprocesses run.
func NewGitWorktreeFromExistingWithExecutor(existingWorktreePath string, sessionName string, opts ...GitWorktreeOption) (*GitWorktree, error) {
	// Ensure the path exists and is a valid git worktree
	if !IsGitRepo(existingWorktreePath) {
		return nil, fmt.Errorf("path '%s' is not a valid git repository or worktree", existingWorktreePath)
	}

	// Callers may hand in a raw, not-yet-canonicalized path (e.g. rehydrated from
	// storage written before this normalization existed); canonicalize it here so
	// worktreePath comparisons stay consistent with the resolved paths produced by
	// fresh-create and git-list-based reuse paths elsewhere in this package.
	existingWorktreePath = CanonicalizeWorktreePath(existingWorktreePath)

	// Find the repository root from the worktree path
	repoPath, err := findMainRepoPathForWorktree(existingWorktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to find repository root for worktree '%s': %w", existingWorktreePath, err)
	}

	// Detect the current branch name in the worktree
	branchName, err := getCurrentBranchName(existingWorktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect branch name for worktree '%s': %w", existingWorktreePath, err)
	}

	// Get the base commit SHA (HEAD commit)
	baseCommitSHA, err := getHeadCommitSHA(existingWorktreePath)
	if err != nil {
		// This is not critical - we can continue without it
		log.Warn("failed to get base commit SHA for worktree", "path", existingWorktreePath, "err", err)
		baseCommitSHA = ""
	}

	g := &GitWorktree{
		repoPath:      repoPath,
		worktreePath:  existingWorktreePath,
		sessionName:   sessionName,
		branchName:    branchName,
		baseCommitSHA: baseCommitSHA,
		runner:        tmux.LocalRunner{},
	}
	for _, opt := range opts {
		opt(g)
	}
	return g, nil
}

// findExistingWorktreeForBranch checks if the given branch is already checked out in an existing worktree
// Returns the path to the existing worktree and true if found, empty string and false otherwise
func findExistingWorktreeForBranch(repoPath, branchName string) (string, bool) {
	// Run git worktree list --porcelain to get detailed worktree information
	wtCtx, wtCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer wtCancel()
	cmd := safeexec.CommandContext(wtCtx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// If the command fails, assume no existing worktrees
		log.Info("failed to list worktrees for branch check", "err", err)
		return "", false
	}

	// Parse the porcelain output to find matching branch
	return parseWorktreeListForBranch(string(output), branchName)
}

// parseWorktreeListForBranch parses the output of 'git worktree list --porcelain'
// and returns the path of the worktree that has the specified branch checked out
func parseWorktreeListForBranch(porcelainOutput, targetBranch string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(porcelainOutput), "\n")
	var currentWorktreePath string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line separates worktree entries
			currentWorktreePath = ""
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			// Extract worktree path
			currentWorktreePath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") && currentWorktreePath != "" {
			// Extract branch name and check if it matches
			branchName := strings.TrimPrefix(line, "branch refs/heads/")
			if branchName == targetBranch {
				return currentWorktreePath, true
			}
		}
	}

	return "", false
}
