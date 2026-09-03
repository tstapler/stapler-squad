package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// worktreeAddRetryAttempts and worktreeAddRetryDelay bound Ground-Truth Re-Query (ADR-001):
// after a `worktree add` failure, both self-heal layers (setupNewWorktree,
// setupFromExistingBranch) re-verify actual git state instead of pattern-matching the
// failure's error text, retrying briefly in case the race winner's own `worktree
// add`/`worktree list` is still in flight.
//
// Deliberately NOT copied from headSHARetryAttempts/headSHARetryDelay (util.go:285-288,
// 3 attempts / 20ms total) — that pair bounds an in-process go-git torn-read against a
// local file write (microsecond-scale). This retry instead waits out a concurrent `git
// worktree add` *subprocess*, which can run up to runGitCommand's 30s timeout under CI
// load. No local repro captured real contended-completion timing, so this is sized as a
// materially larger bound than the unrelated precedent — a few seconds with backoff, not
// tens of milliseconds — rather than reusing 20ms unexamined.
const (
	worktreeAddRetryAttempts = 6
	worktreeAddRetryDelay    = 300 * time.Millisecond
)

// branchRefExists reports whether branchRef exists in repo, distinguishing a genuine
// "no such branch" (plumbing.ErrReferenceNotFound) from any other ref-read error (I/O,
// lock contention from a concurrent git worktree add/git branch, etc.) — the latter must
// never be treated as "branch does not exist".
func branchRefExists(repo *git.Repository, branchRef plumbing.ReferenceName) (bool, error) {
	_, err := repo.Reference(branchRef, false)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, plumbing.ErrReferenceNotFound):
		return false, nil
	default:
		return false, fmt.Errorf("failed to check branch reference %s: %w", branchRef, err)
	}
}

// branchExistsAfterAddFailure is setupNewWorktree's Ground-Truth Re-Query (ADR-001): after
// a `worktree add -b` failure, poll branchRefExists up to worktreeAddRetryAttempts times
// (sleeping worktreeAddRetryDelay between attempts) to give a concurrent race winner's own
// still-in-flight `worktree add` a chance to finish, rather than deciding the outcome from
// the failed command's error text. This supersedes an earlier, narrower fix (PR #595's
// retryBranchRefExists) that only retried on a "cannot lock ref" string match — this
// function's caller no longer inspects error text at all, so it already covers that case
// (and any other unrecognized error string) uniformly; see ADR-001 for why.
//
// Re-opens the repository fresh on every attempt, mirroring getHeadCommitSHA's precedent
// (util.go:310-329) — go-git has been observed to unreliably resolve state immediately
// after a concurrent git-CLI operation on the same repo, so a stale in-memory
// *git.Repository handle held across the whole loop is exactly the failure mode that
// precedent exists to avoid.
//
// A transient error from branchRefExists itself (e.g. the same subprocess/lock contention
// this retry is guarding against) consumes an attempt and the loop continues, rather than
// aborting immediately — a single transient re-query failure is not evidence the branch
// doesn't exist, and the loop is already bounded.
func (g *GitWorktree) branchExistsAfterAddFailure(branchRef plumbing.ReferenceName) bool {
	for attempt := 0; attempt < worktreeAddRetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(worktreeAddRetryDelay)
		}
		repo, err := OpenRepo(g.repoPath)
		if err != nil {
			log.Warn("branchExistsAfterAddFailure: failed to open repository, retrying", "repoPath", g.repoPath, "attempt", attempt, "err", err)
			continue
		}
		exists, err := branchRefExists(repo, branchRef)
		if err != nil {
			log.Warn("branchExistsAfterAddFailure: transient error re-checking branch reference, retrying", "branch", g.branchName, "attempt", attempt, "err", err)
			continue
		}
		if exists {
			return true
		}
	}
	return false
}

// Setup creates a new worktree for the session. The entire branch-check +
// add/reuse dispatch is serialized per-repoPath (across goroutines and OS
// processes) because git worktree add mutates shared .git/worktrees/
// administrative metadata that is not safe under concurrent access -- see
// WithRepoWorktreeLock.
func (g *GitWorktree) Setup() error {
	return WithRepoWorktreeLock(g.repoPath, g.setupLocked)
}

// SetupLocked runs the same setup logic as Setup but assumes the caller already holds
// repoPath's worktree lock (via WithRepoWorktreeLock) -- e.g. because the caller needs to run
// other repoPath-mutating work (like a corrupted-repo repair/re-clone) in the very same
// critical section, immediately before the worktree add. Calling this without already
// holding the lock defeats the cross-process guarantee Setup() normally provides. See
// session.CreateBacklogWorktree for the motivating caller and the race this closes: without
// it, one process's unlocked repo repair (os.RemoveAll + re-clone) could delete/recreate
// repoPath's working tree while a concurrent process was mid-way through the locked
// `git worktree add` for the same repoPath, plausibly surfacing as git's generic
// "fatal: failed to resolve HEAD as a valid ref".
func (g *GitWorktree) SetupLocked() error {
	return g.setupLocked()
}

func (g *GitWorktree) setupLocked() error {
	// Ensure worktrees directory exists early (can be done in parallel with branch check)
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	// Create directory and check branch existence in parallel
	errChan := make(chan error, 2)
	var branchExists bool

	// Goroutine for directory creation
	go func() {
		errChan <- os.MkdirAll(worktreesDir, 0755)
	}()

	// Goroutine for branch check
	go func() {
		repo, err := OpenRepo(g.repoPath)
		if err != nil {
			errChan <- fmt.Errorf("failed to open repository: %w", err)
			return
		}

		branchRef := plumbing.NewBranchReferenceName(g.branchName)
		exists, err := branchRefExists(repo, branchRef)
		if err != nil {
			log.Error("failed to check branch reference", "branch", g.branchName, "error", err)
			errChan <- err
			return
		}
		branchExists = exists
		errChan <- nil
	}()

	// Wait for both operations
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}

	if branchExists {
		return g.setupFromExistingBranch()
	}
	return g.setupNewWorktree()
}

// setupFromExistingBranch creates a worktree from an existing branch, reusing one
// already checked out at g.worktreePath in place rather than tearing it down and
// recreating it. Backlog rework/reopen spawns intentionally reuse the same
// "backlog/<item>" branch and worktree path across every revision (see
// SpawnSessionFromItem's reopen comment) — force-removing and re-adding the worktree
// here on every single reopen discarded whatever uncommitted state the worktree held
// and needlessly recreated the directory, which is exactly the behavior that left a
// still in_progress/review item with a missing worktree once anything else (a
// concurrent cleanup call, or simply a slow-running review) touched it mid-recreation.
//
// setupNewWorktree and setupFromExistingBranch are only ever reached, in production,
// through Setup()/SetupLocked() (worktree_ops.go:53-70), both of which serialize via
// WithRepoWorktreeLock before calling either. The specific two-unlocked-goroutines race
// TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate constructs (by
// calling setupNewWorktree directly, unlocked) cannot occur through any real caller —
// confirmed by TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo.
// The self-heal fallback below is still real defense-in-depth, though: a branch can exist
// for reasons other than a lost create race (a manual `git branch`, a prior partial run),
// and the same Ground-Truth Re-Query handles that case identically. See
// ADR-001-ground-truth-requery-over-stderr-matching.md for why the self-heal decision
// mechanism is a git-state re-query rather than matching the failed command's error text.
func (g *GitWorktree) setupFromExistingBranch() error {
	// Directory already created in Setup(), skip duplicate creation

	if g.worktreeAlreadyRegisteredForBranch() {
		log.Info("worktree already checked out for branch, reusing in place", "branch", g.branchName, "path", g.worktreePath)
		g.initBaseCommitSHA()
		return nil
	}

	// Clean up any existing worktree first. Unlock before removing: a worktree left
	// locked (initializing) by an interrupted `worktree add` — the exact state
	// worktreeAlreadyRegisteredForBranch just rejected above — otherwise refuses
	// `remove` regardless of -f, leaving the broken checkout stuck forever.
	_, _ = g.runGitCommand(g.repoPath, "worktree", "unlock", g.worktreePath)       // Ignore error if not locked
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Create a new worktree from the existing branch
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		// Ground-Truth Re-Query (ADR-001): rather than gating on specific error text
		// (git's "already checked out"/"already used by worktree" wording, which varies
		// by version and locale — and, per this fix's root-cause finding, doesn't cover
		// a timeout-killed subprocess's "signal: killed" either), unconditionally
		// re-check actual git state for any failure here. This is the second self-heal
		// layer for the same concurrent-spawn race setupNewWorktree's own Re-Query
		// handles: by the time this call runs, a race winner may have already checked
		// out the branch into its own worktree.
		log.Info("worktree add failed, checking whether the branch is already checked out elsewhere", "branch", g.branchName, "err", err)

		existingPath, found := g.findLiveWorktreeForBranch()
		if found {
			log.Info("found existing worktree for branch, using it instead", "branch", g.branchName, "path", existingPath)
			g.worktreePath = existingPath
			g.initBaseCommitSHA()
			return nil
		}

		return fmt.Errorf("failed to create worktree from branch %s: %w", g.branchName, err)
	}

	// Worktree created successfully — record the base commit for diff tracking.
	g.initBaseCommitSHA()

	return nil
}

// initBaseCommitSHA finds the merge-base of HEAD with common default branches and
// stores it in g.baseCommitSHA. Non-fatal: if no default branch is found the field
// remains empty and Diff() will fall back to its own resolution.
func (g *GitWorktree) initBaseCommitSHA() {
	for _, branch := range CandidateDefaultBranches {
		// cwd must be g.worktreePath, not g.repoPath: HEAD needs to resolve to this
		// worktree's own branch tip. g.repoPath is the shared parent checkout, whose
		// ambient checked-out branch can be anything a concurrent process left it on
		// (mirrors the fix in resolveBaseCommitSHA, session/git/diff.go, which already
		// does this correctly).
		output, err := g.runGitCommand(g.worktreePath, "merge-base", "HEAD", branch)
		if err == nil {
			if sha := strings.TrimSpace(output); sha != "" {
				g.baseCommitSHA = sha
				log.Info("set base commit SHA for branch to merge-base", "branch", g.branchName, "with_branch", branch, "sha", sha[:min(8, len(sha))])
				return
			}
		}
	}
	log.Warn("could not find merge-base for branch with any default branch (main/master/develop/trunk)", "branch", g.branchName)
}

// worktreeAlreadyRegisteredForBranch reports whether g.worktreePath is already a live,
// fully-set-up git worktree checked out to g.branchName — i.e. reused in place rather
// than removed and recreated. Requires:
//   - git registration for this exact path+branch (via 'worktree list --porcelain'),
//   - the directory's actual presence on disk ('git worktree list' still reports
//     prunable entries for directories deleted out from under git, e.g. by an external
//     rm -rf, and reusing one of those would hand back a path that doesn't exist), and
//   - NOT locked with git's "initializing" marker. `worktree add` briefly holds this
//     lock while it populates the checkout and clears it on success; a worktree still
//     locked here means an earlier `worktree add` was interrupted mid-checkout (e.g.
//     killed by runGitCommand's 30s timeout under load) and left a half-populated
//     directory — reusing that in place would silently hand a broken checkout to the
//     new session instead of self-healing via a fresh remove+add, exactly the failure
//     this reuse-in-place logic exists to prevent for the *good* case.
func (g *GitWorktree) worktreeAlreadyRegisteredForBranch() bool {
	if _, statErr := os.Stat(g.worktreePath); statErr != nil {
		return false
	}
	output, err := g.runGitCommand(g.repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	path, found := g.findWorktreeForBranch(output, g.branchName)
	// g.worktreePath may still be a raw, not-yet-migrated path (e.g. rehydrated
	// from storage written before this normalization existed), while path always
	// comes from git's realpath'd 'worktree list' output — canonicalize both
	// sides so the comparison isn't defeated by a stale spelling difference.
	if !found || CanonicalizeWorktreePath(path) != CanonicalizeWorktreePath(g.worktreePath) {
		return false
	}
	return !isWorktreeLocked(output, g.worktreePath)
}

// isWorktreeLocked reports whether the worktree at targetPath is marked "locked" in
// 'git worktree list --porcelain' output (with or without a reason).
func isWorktreeLocked(porcelainOutput, targetPath string) bool {
	lines := strings.Split(strings.TrimSpace(porcelainOutput), "\n")
	var currentWorktreePath string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			currentWorktreePath = ""
		case strings.HasPrefix(line, "worktree "):
			currentWorktreePath = strings.TrimPrefix(line, "worktree ")
		case currentWorktreePath == targetPath && (line == "locked" || strings.HasPrefix(line, "locked ")):
			return true
		}
	}
	return false
}

// findWorktreeForBranch parses the output of 'git worktree list --porcelain'
// and returns the path of the worktree that has the specified branch checked out
func (g *GitWorktree) findWorktreeForBranch(porcelainOutput, targetBranch string) (string, bool) {
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

// findLiveWorktreeForBranch is setupFromExistingBranch's Ground-Truth Re-Query (ADR-001):
// after a `worktree add` failure, poll `git worktree list --porcelain` up to
// worktreeAddRetryAttempts times (sleeping worktreeAddRetryDelay between attempts) looking
// for g.branchName registered to some other worktree path — giving a concurrent race
// winner's own still-in-flight worktree registration a chance to complete, symmetric with
// setupNewWorktree's branchExistsAfterAddFailure.
//
// Before returning a match, verifies the path is actually present on disk (os.Stat),
// mirroring worktreeAlreadyRegisteredForBranch's existing liveness check: a stale/prunable
// 'worktree list --porcelain' entry left by an earlier crashed run (registered in git's
// metadata but with no directory on disk) must not be silently adopted as a live worktree
// — that would mask a genuine, unrelated `worktree add` failure (disk full, permission
// denied) that happened to coincide with such a stale entry for the same branch name.
//
// A transient error from the `worktree list` subprocess itself consumes an attempt and the
// loop continues, rather than aborting immediately, for the same reason
// branchExistsAfterAddFailure does.
func (g *GitWorktree) findLiveWorktreeForBranch() (string, bool) {
	for attempt := 0; attempt < worktreeAddRetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(worktreeAddRetryDelay)
		}
		output, err := g.runGitCommand(g.repoPath, "worktree", "list", "--porcelain")
		if err != nil {
			log.Warn("findLiveWorktreeForBranch: transient error listing worktrees, retrying", "branch", g.branchName, "attempt", attempt, "err", err)
			continue
		}
		path, found := g.findWorktreeForBranch(output, g.branchName)
		if !found {
			continue
		}
		// git realpath's the path it reports in 'worktree list' output, so canonicalize
		// before storing to keep this consistent with the already-resolved paths
		// getWorktreeDirectory hands to fresh creates.
		path = CanonicalizeWorktreePath(path)
		if _, statErr := os.Stat(path); statErr != nil {
			log.Warn("findLiveWorktreeForBranch: found branch registered to a worktree path that doesn't exist on disk, treating as not found", "branch", g.branchName, "path", path, "err", statErr)
			continue
		}
		return path, true
	}
	return "", false
}

// setupNewWorktree creates a new worktree from HEAD.
//
// Like setupFromExistingBranch (see its doc comment), setupNewWorktree is only ever
// reached, in production, through Setup()/SetupLocked(), both of which serialize via
// WithRepoWorktreeLock — the two-unlocked-goroutines race
// TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate constructs cannot
// occur through any real caller. The self-heal fallback below is still real
// defense-in-depth: see ADR-001-ground-truth-requery-over-stderr-matching.md.
func (g *GitWorktree) setupNewWorktree() error {
	// Ensure worktrees directory exists
	worktreesDir := filepath.Join(g.repoPath, "worktrees")
	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		return fmt.Errorf("failed to create worktrees directory: %w", err)
	}

	// Clean up any existing worktree first
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Open the repository
	repo, err := OpenRepo(g.repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Check if the branch already exists - if so, use it instead of cleaning up
	branchRef := plumbing.NewBranchReferenceName(g.branchName)
	exists, err := branchRefExists(repo, branchRef)
	if err != nil {
		log.Error("failed to check branch reference", "branch", g.branchName, "error", err)
		return err
	}
	if exists {
		// Branch exists - use setupFromExistingBranch instead
		log.Info("branch already exists, using existing branch for worktree", "branch", g.branchName)
		return g.setupFromExistingBranch()
	}

	// Branch doesn't exist - clean up any orphaned references and create new branch
	if err := g.cleanupExistingBranch(repo); err != nil {
		return fmt.Errorf("failed to cleanup existing branch: %w", err)
	}

	// A caller that already knows the commit it wants (e.g. NewGitWorktreeFromCommitSHA,
	// or CreateBacklogWorktree resolving origin/main's true tip before branching) sets
	// baseCommitSHA ahead of time — respect it instead of clobbering it with whatever
	// repoPath's own checkout happens to have as HEAD right now. Only fall back to
	// rev-parse HEAD (branch from the current checkout, same as always) when no base
	// was pre-selected.
	headCommit := g.baseCommitSHA
	if headCommit == "" {
		output, err := g.runGitCommand(g.repoPath, "rev-parse", "HEAD")
		if err != nil {
			if strings.Contains(err.Error(), "fatal: ambiguous argument 'HEAD'") ||
				strings.Contains(err.Error(), "fatal: not a valid object name") ||
				strings.Contains(err.Error(), "fatal: HEAD: not a valid object name") {
				return fmt.Errorf("this appears to be a brand new repository: please create an initial commit before creating an instance")
			}
			return fmt.Errorf("failed to get HEAD commit hash: %w", err)
		}
		headCommit = strings.TrimSpace(string(output))
		g.baseCommitSHA = headCommit
	}

	// Create a new worktree from the resolved base commit.
	// Otherwise, we'll inherit uncommitted changes from the previous worktree.
	// This way, we can start the worktree with a clean slate.
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, headCommit); err != nil {
		// Two concurrent spawns for the same backlog item compute the identical
		// deterministic branch name (see backlogWorkBranchSlug) and can both pass the
		// branchRefExists check above before either creates the branch — the loser's
		// `worktree add -b` then fails, leaving a branch with no worktree. Ground-Truth
		// Re-Query (ADR-001): rather than inferring a lost race from err's text (which
		// misses a timeout-killed subprocess's "signal: killed", a lock-contended
		// "cannot lock ref", a locale-translated message, or a future git wording
		// change — see ADR-001), re-check actual git state directly. Any error here
		// still falls through to the hard error below if the branch genuinely never
		// appears.
		if g.branchExistsAfterAddFailure(branchRef) {
			log.Info("branch already exists (lost a concurrent create race), reusing it for worktree", "branch", g.branchName)
			return g.setupFromExistingBranch()
		}
		return fmt.Errorf("failed to create worktree from commit %s: %w", headCommit, err)
	}

	return nil
}

// Cleanup removes the worktree. It deliberately does NOT delete the branch: branch deletion
// via go-git's RemoveReference is not a "safe if merged" check, it is unconditional, and a
// branch can hold commits that exist nowhere else (never pushed, never merged). Silently
// destroying those on session teardown was a live bug — see
// docs/tasks/backlog-feature-improvement.md ("stop_session silently deletes the git branch").
// A leftover local branch ref costs nothing; a lost commit is not recoverable through this
// code path. Equivalent to Remove() — kept as a separate method so callers don't need to know
// the two used to differ.
func (g *GitWorktree) Cleanup() error {
	log.Info("starting cleanup for worktree", "path", g.worktreePath)
	if err := g.Remove(); err != nil {
		return err
	}
	return g.Prune()
}

// Remove removes the worktree but keeps the branch. Serialized per-repoPath like Setup —
// it prunes and removes shared .git/worktrees/ administrative metadata, the same resource
// Setup's branch-check + add dispatch touches.
func (g *GitWorktree) Remove() error {
	return WithRepoWorktreeLock(g.repoPath, g.removeLocked)
}

func (g *GitWorktree) removeLocked() error {
	log.Info("starting worktree removal", "path", g.worktreePath)

	// First, prune any stale worktree references
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		// Log the prune error but don't fail - continue with removal
		log.Warn("initial worktree prune failed (continuing with removal)", "err", err)
	} else {
		log.Info("initial worktree prune completed successfully")
	}

	// Check if worktree directory exists before attempting git removal
	worktreeExists := true
	if _, err := os.Stat(g.worktreePath); os.IsNotExist(err) {
		worktreeExists = false
		log.Info("worktree directory does not exist", "path", g.worktreePath)
	}

	// Remove the worktree using git command if directory exists
	if worktreeExists {
		if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
			// Check if this is the common "not a working tree" error - treat it as expected
			errStr := err.Error()
			isCorruptedWorktree := strings.Contains(errStr, "is not a working tree") ||
				strings.Contains(errStr, "not a git repository") ||
				strings.Contains(errStr, "worktree not found")

			if isCorruptedWorktree {
				log.Info("worktree is corrupted/invalid, cleaning up manually", "path", g.worktreePath)
			} else {
				log.Warn("git worktree remove failed", "path", g.worktreePath, "err", err)
			}

			// Try manual directory removal as fallback
			if rmErr := os.RemoveAll(g.worktreePath); rmErr != nil {
				log.Warn("manual directory removal also failed", "path", g.worktreePath, "err", rmErr)
				// Only return error if both git and manual removal fail for non-corrupted worktrees
				if !isCorruptedWorktree {
					return fmt.Errorf("failed to remove worktree: git remove failed (%v), manual remove failed (%v)", err, rmErr)
				} else {
					return fmt.Errorf("failed to remove corrupted worktree directory %s: %v", g.worktreePath, rmErr)
				}
			} else {
				if isCorruptedWorktree {
					log.Info("successfully cleaned up corrupted worktree directory", "path", g.worktreePath)
				} else {
					log.Info("successfully removed worktree directory manually", "path", g.worktreePath)
				}
			}
		} else {
			log.Info("successfully removed worktree with git command", "path", g.worktreePath)
		}
	}

	// Clean up any remaining administrative files
	if err := g.forceCleanupWorktree(); err != nil {
		log.Warn("administrative cleanup had some issues (not critical)", "err", err)
		// Don't fail the removal for admin cleanup issues
	}

	log.Info("worktree removal completed successfully", "path", g.worktreePath)
	return nil
}

// forceCleanupWorktree tries multiple strategies to clean up worktree admin files
func (g *GitWorktree) forceCleanupWorktree() error {
	var cleanupErrors []error

	// Strategy 1: Direct cleanup of git worktree admin files (most reliable)
	worktreesDir := filepath.Join(g.repoPath, ".git", "worktrees")

	// Ensure worktrees directory exists before attempting cleanup
	if _, err := os.Stat(worktreesDir); os.IsNotExist(err) {
		log.Info("git worktrees directory does not exist, no admin cleanup needed", "path", worktreesDir)
		return nil
	}

	worktreeName := filepath.Base(g.worktreePath)
	worktreeAdminDir := filepath.Join(worktreesDir, worktreeName)

	log.Info("attempting cleanup of worktree admin directory", "path", worktreeAdminDir)

	// Remove the exact match administrative directory
	if _, err := os.Stat(worktreeAdminDir); err == nil {
		if rmErr := os.RemoveAll(worktreeAdminDir); rmErr != nil {
			log.Warn("failed to remove worktree admin dir", "path", worktreeAdminDir, "err", rmErr)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to remove admin dir %s: %w", worktreeAdminDir, rmErr))
		} else {
			log.Info("successfully removed worktree admin directory", "path", worktreeAdminDir)
		}
	} else {
		log.Info("exact worktree admin directory does not exist", "path", worktreeAdminDir)
	}

	// Strategy 2: Try to find and remove any matching worktree admin directories
	// This handles cases where the directory name might be slightly different
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		log.Warn("could not read worktrees directory", "path", worktreesDir, "err", err)
		cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to read worktrees dir: %w", err))
	} else {
		baseWorktreeName := filepath.Base(g.worktreePath)
		for _, entry := range entries {
			if entry.IsDir() && strings.Contains(entry.Name(), baseWorktreeName) {
				adminPath := filepath.Join(worktreesDir, entry.Name())
				// Skip if this is the exact match we already processed
				if adminPath == worktreeAdminDir {
					continue
				}

				log.Info("found matching worktree admin directory", "path", adminPath)
				if rmErr := os.RemoveAll(adminPath); rmErr != nil {
					log.Warn("failed to remove matching admin dir", "path", adminPath, "err", rmErr)
					cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to remove matching admin dir %s: %w", adminPath, rmErr))
				} else {
					log.Info("successfully removed matching worktree admin directory", "path", adminPath)
				}
			}
		}
	}

	// Return combined errors, but don't treat cleanup failures as critical
	if len(cleanupErrors) > 0 {
		return fmt.Errorf("some cleanup operations failed: %v", cleanupErrors)
	}

	// Strategy 3: Try git commands only after manual cleanup
	log.Info("attempting git worktree prune after manual cleanup")
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune", "--verbose"); err != nil {
		// Prune failed, but we've done manual cleanup, so this is not critical
		log.Warn("git worktree prune failed after manual cleanup (not critical)", "err", err)
	} else {
		log.Info("git worktree prune completed successfully")
	}

	// Strategy 4: Try to list and remove specific worktrees that might still be registered
	if output, err := g.runGitCommand(g.repoPath, "worktree", "list", "--porcelain"); err == nil {
		// Parse the output to find any remaining references to our worktree
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "worktree ") && strings.Contains(line, filepath.Base(g.worktreePath)) {
				// Found our worktree still listed, try to remove it by path
				worktreePath := strings.TrimPrefix(line, "worktree ")
				log.Info("found worktree still listed, attempting removal", "path", worktreePath)
				if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "--force", worktreePath); err != nil {
					log.Warn("failed to remove listed worktree (not critical)", "err", err)
				} else {
					log.Info("successfully removed worktree from git registry", "path", worktreePath)
				}
			}
		}
	} else {
		log.Warn("failed to list worktrees for cleanup verification (not critical)", "err", err)
	}

	// Final prune to clean up any remaining stale references
	log.Info("performing final git worktree prune")
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		log.Warn("final git worktree prune failed (not critical)", "err", err)
	} else {
		log.Info("final worktree prune completed successfully")
	}

	// Always return success - we've done our best with comprehensive cleanup
	log.Info("worktree administrative cleanup completed successfully")
	return nil
}

// Prune removes all working tree administrative files and directories. Serialized
// per-repoPath like Setup/Remove — it rewrites the same shared .git/worktrees/ metadata.
func (g *GitWorktree) Prune() error {
	return WithRepoWorktreeLock(g.repoPath, func() error {
		if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
			return fmt.Errorf("failed to prune worktrees: %w", err)
		}
		return nil
	})
}

// CleanupWorktrees removes all worktree directories under the configured worktrees dir.
// It deliberately does NOT delete the associated branches: a branch can hold commits that
// exist nowhere else (never pushed, never merged), and this function has no way to know
// whether that's true for any given one. See GitWorktree.Cleanup's doc comment — same fix,
// same root cause (docs/tasks/backlog-feature-improvement.md).
func CleanupWorktrees() error {
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			os.RemoveAll(filepath.Join(worktreesDir, entry.Name()))
		}
	}

	pruneCtx, pruneCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pruneCancel()
	if _, err := safeexec.CommandContext(pruneCtx, "git", "worktree", "prune").Output(); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	return nil
}
