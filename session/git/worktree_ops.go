package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/tstapler/stapler-squad/log"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// worktreeAddRetryAttempts and worktreeAddRetryDelay bound Ground-Truth Re-Query (ADR-001):
// after a `worktree add` failure, both self-heal layers (setupLocked's new-worktree
// branch, setupFromExistingBranch) re-verify actual git state instead of pattern-matching the
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

// branchExistsAfterAddFailure is setupLocked's Ground-Truth Re-Query (ADR-001): after
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
//
// TODO(validation.md P2): on retry exhaustion this returns a bare false, indistinguishable
// from "the branch genuinely doesn't exist" — no typed/distinguishable error surfaces to
// the caller. TestSetupNewWorktree_NativeFlag_should_ReturnDistinguishableError_When_RetriesExhausted
// (validation.md) covers this gap; deferred (Phase 6 verify pass, lower severity,
// pre-existing debt) rather than fixed here.
func (g *GitWorktree) branchExistsAfterAddFailure(branchRef plumbing.ReferenceName) bool {
	for attempt := 0; attempt < worktreeAddRetryAttempts; attempt++ {
		if attempt > 0 {
			worktreeRetryTotal.Add(context.Background(), 1)
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
	ctx := withOperationAttrs(context.Background(), attribute.String("session_name", g.sessionName))
	return withOperationSpan(ctx, "git.worktree.add", func() (string, string, error) {
		err := WithRepoWorktreeLock(g.repoPath, g.setupLocked)
		return implementationNative, spanOutcome(err), err
	})
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

// setupLocked creates a new worktree for g.branchName, reusing an existing checkout for
// that branch if one is already registered.
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
		errChan <- os.MkdirAll(worktreesDir, 0750)
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
	return g.nativeSetupNewWorktreeWithSelfHeal()
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
// setupLocked's new-worktree branch and setupFromExistingBranch are only ever
// reached, in production, through Setup()/SetupLocked() (worktree_ops.go:53-70), both of
// which serialize via WithRepoWorktreeLock before calling either. The specific
// two-unlocked-goroutines race TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate
// constructs (by calling setupLocked directly, unlocked) cannot occur through any
// real caller — confirmed by TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo.
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
	_ = g.unlockWorktree()                                                         // Ignore error if not locked
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Create a new worktree from the existing branch
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		// Ground-Truth Re-Query (ADR-001): rather than gating on specific error text
		// (git's "already checked out"/"already used by worktree" wording, which varies
		// by version and locale — and, per this fix's root-cause finding, doesn't cover
		// a timeout-killed subprocess's "signal: killed" either), unconditionally
		// re-check actual git state for any failure here. This is the second self-heal
		// layer for the same concurrent-spawn race setupLocked's own Re-Query
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

// unlockWorktree clears git's "initializing" lock on g.worktreePath left by an interrupted
// `worktree add`, via the native go-git implementation (Story 2.1.4, Task 2.1.4b).
func (g *GitWorktree) unlockWorktree() error {
	return nativeUnlockWorktree(g.repoPath, g.worktreePath)
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

// findLiveWorktreeForBranch looks up whether g.branchName is registered to a live worktree
// elsewhere, via the native go-git implementation (Epic 2.3, Task 2.3.2a).
func (g *GitWorktree) findLiveWorktreeForBranch() (string, bool) {
	var path string
	var found bool
	ctx := withOperationAttrs(context.Background(), attribute.String("session_name", g.sessionName))
	_ = withOperationSpan(ctx, "git.worktree.list", func() (string, string, error) {
		path, found = g.nativeFindLiveWorktreeForBranch()
		outcome := "not_found"
		if found {
			outcome = "found"
		}
		return implementationNative, outcome, nil
	})
	return path, found
}

// nativeFindLiveWorktreeForBranch is setupFromExistingBranch's Ground-Truth Re-Query
// (ADR-001): after a `worktree add` failure, poll nativeListWorktrees up to
// worktreeAddRetryAttempts times (sleeping worktreeAddRetryDelay between attempts) looking
// for g.branchName registered to some other worktree path — giving a concurrent race
// winner's own still-in-flight worktree registration a chance to complete, symmetric with
// branchExistsAfterAddFailure.
func (g *GitWorktree) nativeFindLiveWorktreeForBranch() (string, bool) {
	branchRef := plumbing.NewBranchReferenceName(g.branchName).String()
	for attempt := 0; attempt < worktreeAddRetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(worktreeAddRetryDelay)
		}
		entries, err := nativeListWorktrees(g.repoPath)
		if err != nil {
			log.Warn("nativeFindLiveWorktreeForBranch: transient error listing worktrees, retrying", "branch", g.branchName, "attempt", attempt, "err", err)
			continue
		}
		for _, entry := range entries {
			if entry.BranchRef != branchRef {
				continue
			}
			path := CanonicalizeWorktreePath(entry.WorktreePath)
			if _, statErr := os.Stat(path); statErr != nil {
				log.Warn("nativeFindLiveWorktreeForBranch: found branch registered to a worktree path that doesn't exist on disk, treating as not found", "branch", g.branchName, "path", path, "err", statErr)
				continue
			}
			return path, true
		}
	}
	return "", false
}

// nativeSetupNewWorktreeWithSelfHeal wraps nativeSetupNewWorktree with a Ground-Truth
// Re-Query (ADR-001, Story 2.5.2): two concurrent spawns computing the identical
// deterministic branch name can race, so the loser's nativeSetupNewWorktree
// (Worktree.Checkout with Create: true) fails.
//
// Task 2.5.2a's plan text names git.ErrBranchExists/plumbing.ErrReferenceNotFound as the
// errors to catch, framed against subprocess-stderr matching (which doesn't apply here at
// all). Reading the pinned v5.19.2 source directly (Worktree.createBranch,
// go-git/go-git/v5@v5.19.2/worktree.go:204-230) shows the actual collision error is
// neither of those: it's an unwrapped fmt.Errorf("a branch named %q already exists", ...),
// so errors.Is against either sentinel can never match it. Rather than pattern-matching
// error text instead (exactly what Ground-Truth Re-Query exists to avoid), any
// nativeSetupNewWorktree failure re-queries actual git state via
// branchExistsAfterAddFailure, falling through to the original error only if the branch
// genuinely never appears.
func (g *GitWorktree) nativeSetupNewWorktreeWithSelfHeal() error {
	err := g.nativeSetupNewWorktree()
	if err == nil {
		return nil
	}

	branchRef := plumbing.NewBranchReferenceName(g.branchName)
	if g.branchExistsAfterAddFailure(branchRef) {
		log.Info("native worktree add failed (lost a concurrent create race), reusing existing branch for worktree", "branch", g.branchName, "err", err)
		return g.setupFromExistingBranch()
	}

	return err
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

// removeLocked removes the worktree via the native go-git implementation (Epic 2.2, Task
// 2.2.2a).
func (g *GitWorktree) removeLocked() error {
	ctx := withOperationAttrs(context.Background(), attribute.String("session_name", g.sessionName))
	return withOperationSpan(ctx, "git.worktree.remove", func() (string, string, error) {
		err := nativeRemoveWorktree(g.repoPath, g.worktreePath)
		return implementationNative, spanOutcome(err), err
	})
}

// Prune removes all working tree administrative files and directories. Serialized
// per-repoPath like Setup/Remove — it rewrites the same shared .git/worktrees/ metadata.
func (g *GitWorktree) Prune() error {
	ctx := withOperationAttrs(context.Background(), attribute.String("session_name", g.sessionName))
	return withOperationSpan(ctx, "git.worktree.prune", func() (string, string, error) {
		err := WithRepoWorktreeLock(g.repoPath, func() error { return nativeWorktreePrune(g.repoPath) })
		return implementationNative, spanOutcome(err), err
	})
}

// CleanupWorktrees removes all worktree directories under the configured worktrees dir.
// It deliberately does NOT delete the associated branches: a branch can hold commits that
// exist nowhere else (never pushed, never merged), and this function has no way to know
// whether that's true for any given one. See GitWorktree.Cleanup's doc comment — same fix,
// same root cause (docs/tasks/backlog-feature-improvement.md).
//
// Task 2.2.2c disposition: a real, non-test caller exists (main.go's `reset` command),
// contradicting architecture.md §1a's "none found" — recorded here per the Unresolved
// Questions entry. Its per-directory removal below is already a direct os.RemoveAll (no
// subprocess to seam); only its trailing `git worktree prune` step dispatches natively via
// cleanupWorktreesPrune.
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
			dirPath := filepath.Join(worktreesDir, entry.Name())
			if err := os.RemoveAll(dirPath); err != nil {
				log.Warn("CleanupWorktrees: failed to remove worktree directory", "path", dirPath, "err", err)
			}
		}
	}

	return cleanupWorktreesPrune()
}

// cleanupWorktreesPrune runs CleanupWorktrees' trailing `git worktree prune` step via the
// native go-git implementation, against the process's current working directory (matching
// the old subprocess call's unset-Dir default) since CleanupWorktrees operates across every
// session's worktrees at once and has no single repoPath in scope.
func cleanupWorktreesPrune() error {
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve current directory for native worktree prune: %w", err)
	}
	return nativeWorktreePrune(repoPath)
}
