package git

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// isTraversalPathSegment reports whether s is "." or ".." — mirrors
// session.isTraversalSegment (session/repo_path.go), which can't be imported
// here without a package cycle (session imports git).
func isTraversalPathSegment(s string) bool {
	return s == "." || s == ".."
}

// defaultPlainOpenOptions is the single source of truth for how this codebase opens a
// git repository. In particular, EnableDotGitCommonDir must always be set: without it,
// go-git silently resolves objects/refs for a linked worktree (`git worktree add`)
// against the wrong gitdir — not an error, a real-but-wrong result (verified
// empirically: HEAD resolved to a stale SHA from before the worktree was created).
// Read-only after init, so sharing this one instance across every OpenRepo call is safe.
var defaultPlainOpenOptions = &git.PlainOpenOptions{ //nolint:gochecknoglobals shared read-only options, not mutable state
	DetectDotGit:          true,
	EnableDotGitCommonDir: true,
}

// OpenRepo opens the git repository or worktree at path using defaultPlainOpenOptions.
// Every git repository open in this codebase must go through this function rather than
// a bare git.PlainOpen/PlainOpenWithOptions call — see tools/lint's norawgitopen
// analyzer, which enforces this.
func OpenRepo(path string) (*git.Repository, error) {
	return git.PlainOpenWithOptions(path, defaultPlainOpenOptions) //nolint:norawgitopen this is the wrapper itself
}

// sanitizeBranchName transforms an arbitrary string into a Git branch name friendly string.
// Note: Git branch names have several rules, so this function uses a simple approach
// by allowing only a safe subset of characters.
func sanitizeBranchName(s string) string {
	original := s

	// Convert to lower-case
	s = strings.ToLower(s)

	// Replace spaces with a dash
	s = strings.ReplaceAll(s, " ", "-")

	// Remove any characters not allowed in our safe subset.
	// Here we allow: letters, digits, dash, underscore, slash, and dot.
	re := regexp.MustCompile(`[^a-z0-9\-_/.]+`)
	s = re.ReplaceAllString(s, "")

	// Replace multiple dashes with a single dash (optional cleanup)
	reDash := regexp.MustCompile(`-+`)
	s = reDash.ReplaceAllString(s, "-")

	// Trim leading and trailing dashes or slashes to avoid issues
	s = strings.Trim(s, "-/")

	// Strip "." and ".." path segments so a crafted name like "../../etc" can't
	// escape the directory it's later joined into
	// (filepath.Join(worktreeDir, sanitizedName) at the worktree-creation call
	// sites in worktree.go). Only exact segment matches are dropped — "..hidden",
	// "user..name", and "v1.2.3" are literal names, not traversal, and survive
	// untouched.
	segments := strings.Split(s, "/")
	kept := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg == "" || isTraversalPathSegment(seg) {
			continue
		}
		kept = append(kept, seg)
	}
	cleaned := strings.Join(kept, "/")

	// If the input was non-empty but every segment was traversal/empty, fall
	// back to a safe non-empty name — otherwise filepath.Join(worktreeDir, "")
	// collapses to worktreeDir itself, and a downstream os.RemoveAll on the
	// resulting "worktree" path (session/git/worktree_ops.go) would delete the
	// entire worktrees base directory. Compare against the ORIGINAL input, not
	// the post-trim/post-strip `s` — inputs like "/", "---", or "!!!" trim down
	// to "" before this point even though they were non-empty, and checking `s`
	// here would miss them, letting them fall through as "" unflagged. An
	// originally-empty input still yields "" here, preserving pre-existing
	// behavior for that case.
	if cleaned == "" && original != "" {
		return "session"
	}

	return cleaned
}

// SanitizeBranchName exports sanitizeBranchName for callers outside this
// package that need the same safe-subset transform for a remote worktree
// directory name (server/services/session_service.go's CreateSession
// mode-specific block, ssh-remote-workspaces Phase 4 Epic 4.2) as this
// package already applies to local branch/directory names -- including its
// path-traversal-segment stripping, which matters just as much for a remote
// `path.Join(base_path, name)` as it does for the local filepath.Join call
// sites in this file.
func SanitizeBranchName(s string) string {
	return sanitizeBranchName(s)
}

// joinWithinDir joins name onto baseDir and verifies the result is still a
// descendant of baseDir, returning an error instead of the escaped path if
// not. sanitizeBranchName already strips "." and ".." segments, so this
// should never trigger in practice — it exists as defense-in-depth against a
// future regression in the sanitizer at the actual filepath.Join call sites
// (worktree-creation and the PreviewWorktreePath RPC preview).
func joinWithinDir(baseDir, name string) (string, error) {
	joined := filepath.Join(baseDir, name)

	rel, err := filepath.Rel(baseDir, joined)
	if err != nil {
		return "", fmt.Errorf("failed to compute relative path from %q to %q: %w", baseDir, joined, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("sanitized name %q escapes base directory %q", name, baseDir)
	}

	return joined, nil
}

// CanonicalizeWorktreePath resolves path to its symlink-free (realpath'd) form,
// matching what `git worktree list --porcelain` reports and what
// getWorktreeDirectory already produces for freshly-created worktree parents.
// On macOS /var (and /tmp) is itself a symlink to /private/var, so two code
// paths that construct the "same" worktree path differently — one via
// filepath.Join on an unresolved parent, the other by reading git's
// already-resolved output — end up as different strings for the identical
// directory (see TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession).
// EvalSymlinks requires the path to exist, which doesn't hold for the
// pre-creation/rehydration cases this is also used in; falling back to
// filepath.Clean on ANY error (not just ENOENT) keeps this a pure, non-failing
// normalizer, matching the established pattern in session/history_detector.go,
// session/import_correlate.go, and session/unfinished/gogitstore/open.go.
func CanonicalizeWorktreePath(path string) string {
	if path == "" {
		return path
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return resolved
}

// checkGHCLI checks if GitHub CLI is installed and configured. The auth check
// runs through g.commandRunner() (the same CommandRunner execution seam every
// other gh/git call on GitWorktree uses, per ADR-002 -- see worktree.go's
// runner field doc comment) rather than shelling out directly, so tests that
// already inject a spy CommandRunner for gh pr create/list don't also need a
// real, authenticated `gh` binary on PATH just to get past this guard.
func (g *GitWorktree) checkGHCLI() error {
	// Check if gh is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is not installed. Please install it first")
	}

	// Check if gh is authenticated
	authCtx, authCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer authCancel()
	if _, err := g.commandRunner().Run(authCtx, g.worktreePath, "gh", "auth", "status"); err != nil {
		return fmt.Errorf("GitHub CLI is not configured. Please run 'gh auth login' first")
	}

	return nil
}

// IsGitRepo checks if the given path is within a git repository
func IsGitRepo(path string) bool {
	for {
		_, err := OpenRepo(path)
		if err == nil {
			return true
		}

		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}

// findGitRepoRoot walks up from path looking for an existing git repository and
// returns its root. It requires path to already exist: it must NOT fabricate a repo
// when the path is missing. An earlier version silently ran os.MkdirAll + git.PlainInit
// + createInitialCommit for a missing path, which was intended for genuine
// new-project bootstrapping but was unconditionally shared by every caller —
// including CreateBacklogWorktree's worktree constructors, which require repoPath to
// already be a real, previously-cloned repository. When that path went transiently
// missing (observed live: deleted mid-repair while the backlog automation loop kept
// retrying), the fallback masked the real "repo is missing" error by fabricating a
// disconnected repo with a single placeholder commit, which downstream worktree
// creation then treated as real — producing broken sessions instead of a clear
// failure. Genuine new-project initialization has its own dedicated function,
// InitializeProjectDirectory (used by SessionTypeNewProject), which never routes
// through here — so no caller needs findGitRepoRoot to create anything from scratch.
func findGitRepoRoot(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("repository path does not exist: %s (expected an existing git repository; use InitializeProjectDirectory to bootstrap a new one)", path)
	}

	// Directory exists - find the git repo root
	currentPath := path
	for {
		repo, err := OpenRepo(currentPath)
		if err == nil {
			// Found the repository root. A bare repo.Head() error is not reliable
			// evidence the repo is unborn — go-git can transiently fail to resolve
			// HEAD for an otherwise-real repo (see getHeadCommitSHA's doc comment) —
			// so check the ref store directly before concluding it has zero history.
			hasRef, err := repoHasAnyRef(currentPath)
			if err != nil {
				return "", fmt.Errorf("failed to check for existing refs at '%s': %w", currentPath, err)
			}
			if !hasRef {
				log.Info("repository has no commits, creating initial commit", "path", currentPath)
				if err := createInitialCommit(repo, currentPath); err != nil {
					return "", fmt.Errorf("failed to create initial commit at '%s': %w", currentPath, err)
				}
				log.Info("successfully created initial commit", "path", currentPath)
			}
			return currentPath, nil
		}

		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			// Reached the filesystem root without finding a repository
			return "", fmt.Errorf("failed to find Git repository root from path: %s", path)
		}
		currentPath = parent
	}
}

// findMainRepoPathForWorktree returns the main repository root for an existing git
// worktree at worktreePath.
//
// This must NOT reuse findGitRepoRoot: that function's "no commits found → create an
// initial commit here" fallback exists for locating or bootstrapping a fresh repo root
// by walking up parent directories, and is actively harmful for a worktree — a
// worktree's `.git` is a redirect FILE ("gitdir: /path/to/main/.git/worktrees/<name>"),
// not a directory to walk up from. This was observed in production and reproduced in a
// unit test: git.PlainOpen on a worktree path can succeed while the subsequent
// repo.Head() call errors (go-git struggling to resolve HEAD for a git-CLI-created
// worktree), which findGitRepoRoot misreads as "no commits" and "fixes" by creating a
// brand-new, disconnected initial commit directly inside the worktree directory and
// returning the worktree's own path as if it were the repo root — corrupting the
// worktree's link to its real branch/history entirely.
//
// Resolves via `git rev-parse --git-common-dir`, which is the shared .git directory a
// worktree and its main repo both point to; the main repo root is its parent.
func findMainRepoPathForWorktree(worktreePath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve git-common-dir for worktree %s: %w", worktreePath, err)
	}
	commonDir := strings.TrimSpace(string(out))
	return filepath.Dir(commonDir), nil
}

// GetCurrentBranchName returns the current branch name for a git repository or worktree.
// Returns an error if the repo is in detached HEAD state.
func GetCurrentBranchName(path string) (string, error) {
	return getCurrentBranchName(path)
}

// getCurrentBranchName returns the current branch name for a git repository or worktree.
// Uses go-git to read HEAD directly (file read, no subprocess).
func getCurrentBranchName(path string) (string, error) {
	repo, err := OpenRepo(path)
	if err != nil {
		return "", fmt.Errorf("failed to open git repo at %s: %w", path, err)
	}
	// Don't resolve the symbolic ref — we want the branch name, not the commit hash.
	ref, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return "", fmt.Errorf("failed to read HEAD at %s: %w", path, err)
	}
	if ref.Type() != plumbing.SymbolicReference {
		return "", fmt.Errorf("repository at '%s' is in detached HEAD state", path)
	}
	// ref.Target() is e.g. "refs/heads/main"; Short() trims to "main".
	return ref.Target().Short(), nil
}

// GetHeadCommitSHA returns the SHA of the HEAD commit for a git repository or worktree.
func GetHeadCommitSHA(path string) (string, error) { return getHeadCommitSHA(path) }

// headSHARetryAttempts and headSHARetryDelay bound the go-git torn-read mitigation in
// getHeadCommitSHA — see its doc comment.
const (
	headSHARetryAttempts = 3
	headSHARetryDelay    = 20 * time.Millisecond
)

// getHeadCommitSHA returns the SHA of the HEAD commit for a git repository or worktree.
// Reads via go-git (no subprocess) in the common case for speed — this repo can have
// many concurrent sessions, and shelling out to git for every SHA lookup adds real
// process overhead at that scale.
//
// go-git was observed to unreliably resolve HEAD for a worktree created via the git
// CLI's `git worktree add`, in two distinct ways: (1) repo.Head() erroring outright
// immediately after worktree creation (reproduced deterministically in a unit test,
// single-threaded, no concurrency involved — a go-git worktree-gitdir-parsing gap, not
// a race), and (2) in production, repo.Head() returning a syntactically-valid SHA that
// did not correspond to any real object in the repository at all (failed git cat-file
// -t, absent from git rev-list --all, git reflog show --all, and git fsck
// --unreachable) — consistent with go-git's unlocked, direct ref-file read racing a
// concurrent git operation against the same parent repo, unprotected by git's own
// atomic-rename ref-update guarantees.
//
// So every go-git read here is verified against go-git's own object store
// (repo.CommitObject, still no subprocess) before being trusted, and a Head() error is
// treated the same as an invalid result; both retry briefly, and only after repeated
// failures do we fall back to the git CLI, which is immune to both failure modes.
//
// Every PlainOpenWithOptions call in this package now also sets EnableDotGitCommonDir
// (2026-09-02), fixing a third, distinct worktree bug: without it, go-git silently
// resolves HEAD to a real-but-wrong commit object for a linked worktree (verified
// against this repo's own worktree: HEAD read without the flag returned a stale SHA
// from before the worktree was created, not a not-found or malformed result at all) —
// see session/git/ops.go's diffPatchBetween for the code-review finding that surfaced
// it. That fix is orthogonal to the retry/CLI-fallback above, which guards against the
// two races described above and stays in place as defense-in-depth.
func getHeadCommitSHA(path string) (string, error) {
	// A failed open (path doesn't exist / isn't a git repo at all) is not retryable —
	// only the subsequent Head() resolution and object-existence check are, since those
	// are what were observed to fail transiently for worktrees.
	if _, err := OpenRepo(path); err != nil {
		return "", fmt.Errorf("failed to open git repo at %s: %w", path, err)
	}

	var lastErr error
	for attempt := 0; attempt < headSHARetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(headSHARetryDelay)
		}
		// Re-open fresh each attempt in case go-git's failure originates in gitdir/ref
		// parsing at open time, not just in the Repository object's later reads.
		repo, err := OpenRepo(path)
		if err != nil {
			lastErr = fmt.Errorf("failed to open git repo at %s: %w", path, err)
			continue
		}
		ref, err := repo.Head()
		if err != nil {
			lastErr = fmt.Errorf("failed to read HEAD at %s: %w", path, err)
			continue
		}
		hash := ref.Hash()
		if _, commitErr := repo.CommitObject(hash); commitErr == nil {
			return hash.String(), nil
		}
		lastErr = fmt.Errorf("go-git resolved HEAD to %s at %s, which is not a real commit object", hash, path)
	}
	log.Warn("getHeadCommitSHA: go-git repeatedly resolved HEAD to a nonexistent object, falling back to git CLI", "path", path, "err", lastErr)
	return getHeadCommitSHAViaCLI(path)
}

// repoHasAnyRef reports whether the git repository at path has at least one
// hash reference — a reliable signal of real history, unlike repo.Head() (see
// getHeadCommitSHA's doc comment). Only HashReferences count: git.PlainInit's
// symbolic "HEAD -> refs/heads/master" always exists even in a zero-commit
// repo, so counting it would always report true. Retry/CLI-fallback shape
// mirrors getHeadCommitSHA (same underlying go-git flakiness class).
func repoHasAnyRef(path string) (bool, error) {
	if _, err := OpenRepo(path); err != nil {
		return false, fmt.Errorf("failed to open git repo at %s: %w", path, err)
	}

	var lastErr error
	for attempt := 0; attempt < headSHARetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(headSHARetryDelay)
		}
		repo, err := OpenRepo(path)
		if err != nil {
			lastErr = fmt.Errorf("failed to open git repo at %s: %w", path, err)
			continue
		}
		refs, err := repo.References()
		if err != nil {
			lastErr = fmt.Errorf("failed to list refs at %s: %w", path, err)
			continue
		}
		found := false
		iterErr := refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Type() != plumbing.HashReference {
				return nil
			}
			found = true
			return storer.ErrStop
		})
		refs.Close()
		if iterErr != nil {
			lastErr = fmt.Errorf("failed to iterate refs at %s: %w", path, iterErr)
			continue
		}
		return found, nil
	}
	log.Warn("repoHasAnyRef: go-git repeatedly failed to read refs, falling back to git CLI", "path", path, "err", lastErr)
	return repoHasAnyRefViaCLI(path)
}

// repoHasAnyRefViaCLI is repoHasAnyRef's fallback when go-git repeatedly fails to read
// the ref store — see getHeadCommitSHAViaCLI's identical rationale (atomic-rename ref
// updates mean the CLI doesn't observe the torn-read race go-git is susceptible to).
// `git for-each-ref` does not enumerate the symbolic HEAD pseudo-ref, matching
// repoHasAnyRef's own HashReference-only filter.
func repoHasAnyRefViaCLI(path string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "for-each-ref", "--count=1")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to list refs at %s: %w", path, err)
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// getHeadCommitSHAViaCLI is the fallback path for getHeadCommitSHA when go-git
// repeatedly fails to resolve HEAD to a real object. The git CLI relies on atomic-rename
// ref updates, so it doesn't observe the torn-read race go-git is susceptible to.
func getHeadCommitSHAViaCLI(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read HEAD at %s: %w", path, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// InitializeProjectDirectory creates a directory and initializes it as a git repository.
// Behavior by pre-existing state:
//   - Path does not exist: creates with os.MkdirAll(path, 0755), runs git init, commits.
//   - Path exists, no .git: runs git init in place, commits.
//   - Path exists, already a git repo: no-op, returns nil.
//   - Path exists but is a regular file: returns an error.
//
// On partial failure (dir created, git init failed): attempts os.RemoveAll to roll back
// the newly created directory. Logs a warning if rollback also fails.
func InitializeProjectDirectory(path string) error {
	// 1. Check if already a git repo (open succeeds) → no-op
	if _, err := OpenRepo(path); err == nil {
		return nil
	}

	// 2. Check for file collision
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return fmt.Errorf("path exists and is not a directory: %s", path)
	}

	// 3. Track whether we created the directory so we can roll back on failure
	dirCreated := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0750); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		dirCreated = true
	}

	// 4. git init
	repo, err := git.PlainInit(path, false)
	if err != nil {
		if dirCreated {
			if rmErr := os.RemoveAll(path); rmErr != nil {
				log.Error("InitializeProjectDirectory: rollback failed", "path", path, "err", rmErr)
			}
		}
		return fmt.Errorf("failed to init git repo: %w", err)
	}

	// 5. Initial commit (reuses the existing createInitialCommit helper)
	if err := createInitialCommit(repo, path); err != nil {
		if dirCreated {
			if rmErr := os.RemoveAll(path); rmErr != nil {
				log.Error("InitializeProjectDirectory: rollback failed", "path", path, "err", rmErr)
			}
		}
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	return nil
}

// createInitialCommit creates an initial commit in a new git repository
// This is required because git worktrees need at least one commit to exist
func createInitialCommit(repo *git.Repository, repoPath string) error {
	// Self-defending guard: refuse to run against a repo that already has any
	// ref, regardless of caller. A fresh git.PlainInit repo has zero refs by
	// construction, so this is a no-op for every legitimate caller — it only
	// fires when something upstream misdiagnosed a real repo as unborn.
	if hasRef, err := repoHasAnyRef(repoPath); err != nil {
		return fmt.Errorf("failed to check for existing refs at '%s': %w", repoPath, err)
	} else if hasRef {
		return fmt.Errorf("refusing to create initial commit at '%s': repository already has existing refs", repoPath)
	}

	// Get the worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Create a .gitignore file as the initial commit content
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	gitignoreContent := []byte("# Project gitignore\n")
	if err := os.WriteFile(gitignorePath, gitignoreContent, 0600); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}

	// Add .gitignore to staging
	if _, err := worktree.Add(".gitignore"); err != nil {
		return fmt.Errorf("failed to add .gitignore: %w", err)
	}

	// Create the initial commit
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Stapler Squad",
			Email: "stapler-squad@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	return nil
}
