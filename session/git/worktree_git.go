package git

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tstapler/stapler-squad/log"
)

// prNumberFromURLRe extracts the trailing PR number from a GitHub PR URL,
// e.g. "https://github.com/owner/repo/pull/148" -> 148. Mirrors
// session/storage_backlog.go's identical pattern (BackfillMissingPRNumbers) —
// duplicated here rather than imported since session/git cannot import the
// parent session package without a cycle.
var prNumberFromURLRe = regexp.MustCompile(`/pull/(\d+)/?$`)

// runGitCommand executes a git command scoped to path and returns any error.
// Routes through g.commandRunner() unconditionally, exactly like every other
// call site in this file (PushChanges, PushBranch, OpenBranchURL, CreatePR,
// findExistingPR, GetPRStatus, EnablePRAutoMerge, RequestCopilotReview,
// ClosePR, IsPRMerged) — see ADR-002's addendum for the history here: this
// was previously the one call site with a g.cmdExec-gated branch (an
// executor.Executor test-injection seam, never circuit-breaker-wrapped
// anywhere in this package, unlike session/tmux's genuinely orthogonal
// cmdExec), which made it dead code for all ~25 production callers of this
// method (IsDirtyWithHint, RenameBranch, stageAndCommit,
// StageAllExceptScaffolding, HasStagedChanges, plus every worktree_ops.go
// worktree add/remove/prune/list call). IsDirtyWithHint's race/error-
// injection tests now inject a tmux.CommandRunner spy via WithCommandRunner
// instead of an executor.Executor mock.
func (g *GitWorktree) runGitCommand(path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := g.commandRunner().Run(ctx, path, "git", args...)
	if err != nil {
		return "", fmt.Errorf("git command failed: %s (%w)", output, err)
	}

	return string(output), nil
}

// PushChanges commits and pushes changes in the worktree to the remote branch
func (g *GitWorktree) PushChanges(commitMessage string, open bool) error {
	if err := checkGHCLI(); err != nil {
		return err
	}

	// Check if there are any changes to commit
	isDirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if isDirty {
		if err := g.stageAndCommit(commitMessage); err != nil {
			return err
		}
		g.InvalidateDirtyCache()
	}

	// First push the branch to remote to ensure it exists
	pushCtx, pushCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer pushCancel()
	if _, err := g.commandRunner().Run(pushCtx, g.worktreePath, "gh", "repo", "sync", "--source", "-b", g.branchName); err != nil {
		// If sync fails, try creating the branch on remote first
		gitPushCtx, gitPushCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer gitPushCancel()
		if pushOutput, pushErr := g.commandRunner().Run(gitPushCtx, g.worktreePath, "git", "push", "-u", "origin", g.branchName); pushErr != nil {
			log.Error("failed to push branch", "err", pushErr)
			return fmt.Errorf("failed to push branch: %s (%w)", pushOutput, pushErr)
		}
	}

	// Now sync with remote
	syncCtx, syncCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer syncCancel()
	if output, err := g.commandRunner().Run(syncCtx, g.worktreePath, "gh", "repo", "sync", "-b", g.branchName); err != nil {
		log.Error("failed to sync changes", "err", err)
		return fmt.Errorf("failed to sync changes: %s (%w)", output, err)
	}

	// Open the branch in the browser
	if open {
		if err := g.OpenBranchURL(); err != nil {
			// Just log the error but don't fail the push operation
			log.Error("failed to open branch URL", "err", err)
		}
	}

	return nil
}

// CommitChanges commits changes locally without pushing to remote
func (g *GitWorktree) CommitChanges(commitMessage string) error {
	// Check if there are any changes to commit
	isDirty, err := g.IsDirty()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if isDirty {
		if err := g.stageAndCommit(commitMessage); err != nil {
			return err
		}
		g.InvalidateDirtyCache()
	}

	return nil
}

// RenameBranch renames the worktree's current branch in place (git branch -m) and
// updates g.branchName to match. Used to move a worktree created under a
// provisional name onto the final branch name once it's known, without losing the
// worktree's existing content or commits — e.g. TriggerTriage names its worktree
// before the LLM call reveals the item's slug, then renames it afterward to the
// same "backlog/<item>" branch a later SpawnSessionFromItem will look for.
func (g *GitWorktree) RenameBranch(newBranchName string) error {
	if _, err := g.runGitCommand(g.worktreePath, "branch", "-m", newBranchName); err != nil {
		return fmt.Errorf("failed to rename branch to %q: %w", newBranchName, err)
	}
	g.branchName = newBranchName
	return nil
}

// stageAndCommit stages all changes (minus scaffolding), then commits — unless
// the only staged change was a scaffolding file that staging just untracked, in
// which case it skips the commit gracefully instead of failing on git's
// "nothing to commit". Shared by CommitChanges and PushChanges, whose stage/
// commit sequence is otherwise identical.
func (g *GitWorktree) stageAndCommit(commitMessage string) error {
	if err := g.StageAllExceptScaffolding(); err != nil {
		log.Error("failed to stage changes", "err", err)
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	hasStaged, err := g.HasStagedChanges()
	if err != nil {
		log.Error("failed to check staged changes", "err", err)
		return fmt.Errorf("failed to check staged changes: %w", err)
	}
	if !hasStaged {
		return nil
	}

	if _, err := g.runGitCommand(g.worktreePath, "commit", "-m", commitMessage, "--no-verify"); err != nil {
		log.Error("failed to commit changes", "err", err)
		return fmt.Errorf("failed to commit changes: %w", err)
	}
	return nil
}

// StageAllExceptScaffolding stages all worktree changes (`git add .`) and then
// untracks any staged path matching ScaffoldingExcludePatterns, so backlog
// automation scaffolding files (.backlog-context.md, .claude/commands/backlog/*)
// don't get (re)committed even if they were already tracked in this branch's
// history — gitignore/info-exclude rules only stop a NEW path from being
// staged, not one that's already in the index (see UntrackScaffolding's doc
// comment). Deliberately fails open on the untrack step (logs and continues
// rather than blocking a real commit) — the CI backstop workflow
// (.github/workflows/backlog-scaffolding-guard.yml) is the second, independent
// layer for the rare case where the untrack step itself errors.
func (g *GitWorktree) StageAllExceptScaffolding() error {
	if _, err := g.runGitCommand(g.worktreePath, "add", "."); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	removed, err := UntrackScaffolding(g.worktreePath, ScaffoldingExcludePatterns)
	if err != nil {
		log.Error("failed to untrack scaffolding files before commit", "worktree", g.worktreePath, "err", err)
	} else if len(removed) > 0 {
		log.Info("untracked scaffolding file(s) before commit", "worktree", g.worktreePath, "files", removed)
	}
	return nil
}

// HasStagedChanges reports whether the git index differs from HEAD — i.e.
// whether a commit right now would actually record anything. Used after
// StageAllExceptScaffolding so a commit whose only staged change was a
// just-untracked scaffolding file is skipped gracefully instead of failing on
// "nothing to commit".
func (g *GitWorktree) HasStagedChanges() (bool, error) {
	out, err := g.runGitCommand(g.worktreePath, "diff", "--cached", "--name-only")
	if err != nil {
		return false, fmt.Errorf("failed to check staged changes: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// PrimeDirtyCacheAt sets the dirty-cache timestamp to t without running git status.
// Use this to stagger per-session cache expiry so sessions added to the poller
// within a short window don't all expire simultaneously and burst-launch git subprocesses.
func (g *GitWorktree) PrimeDirtyCacheAt(t time.Time) {
	g.isDirtyCache.Store(dirtyCacheState{dirty: false, time: t})
}

// InvalidateDirtyCache clears the IsDirty cache so the next call re-runs git status.
// Call this whenever worktree state changes outside of Claude's control (e.g. after a
// manual commit, after running git operations, or in tests after writing files directly).
func (g *GitWorktree) InvalidateDirtyCache() {
	g.isDirtyCache.Store(dirtyCacheState{}) // zero time signals "cache invalid"
}

// IsDirty checks if the worktree has uncommitted changes.
// Results are cached for IsDirtyCacheTTL (dirty) or IsDirtyCleanCacheTTL (clean).
func (g *GitWorktree) IsDirty() (bool, error) {
	return g.IsDirtyWithHint(false)
}

// isDirtyCacheTTL returns the TTL to apply based on the current cached state.
// Clean worktrees use a longer TTL because they won't change while the session is idle,
// and InvalidateDirtyCache() fires on every code path that could make them dirty. A
// cached error (e.g. worktree directory missing) gets its own short backoff so a broken
// worktree isn't re-checked on every poller tick.
func isDirtyCacheTTL(state dirtyCacheState) time.Duration {
	switch {
	case state.err != nil:
		return IsDirtyErrorCacheTTL
	case state.dirty:
		return IsDirtyCacheTTL
	default:
		return IsDirtyCleanCacheTTL
	}
}

// IsDirtyWithHint checks if the worktree has uncommitted changes.
// When claudeActive is true the subprocess is skipped entirely and the cached value is returned
// (or false if no cached value is available yet), because Claude never modifies worktree state
// while it is actively generating output.
func (g *GitWorktree) IsDirtyWithHint(claudeActive bool) (bool, error) {
	// Fast path: lock-free atomic load; TTL varies by cached state (dirty/clean/error).
	// dirty → IsDirtyCacheTTL (30s); clean → IsDirtyCleanCacheTTL (5min); error → IsDirtyErrorCacheTTL (60s).
	if v := g.isDirtyCache.Load(); v != nil {
		state := v.(dirtyCacheState)
		if claudeActive || (!state.time.IsZero() && time.Since(state.time) < isDirtyCacheTTL(state)) {
			if state.err != nil {
				return false, state.err
			}
			return state.dirty, nil
		}
	} else if claudeActive {
		return false, nil
	}

	// Slow path: run git status --porcelain via subprocess, wrapped in singleflight
	// so concurrent callers coalesce onto a single status check rather than each
	// spawning their own git process.
	type dirtyResult struct {
		dirty bool
		err   error
	}
	v, _, _ := g.isDirtySF.Do(g.worktreePath, func() (interface{}, error) {
		out, subErr := g.runGitCommand(g.worktreePath, "status", "--porcelain")
		return dirtyResult{len(out) > 0, subErr}, nil
	})
	res := v.(dirtyResult)
	if res.err != nil {
		// Cache the failure with a backoff TTL (isDirtyCacheTTL routes err!=nil to
		// IsDirtyErrorCacheTTL) so a worktree with a stale/missing path — e.g. left
		// behind by a rework/reopen cycle — doesn't get re-checked on every poller
		// tick (previously: a fresh subprocess spawn roughly every few seconds,
		// indefinitely, for a directory that will never come back on its own).
		wrapped := fmt.Errorf("failed to check worktree status: %w", res.err)
		g.isDirtyCache.Store(dirtyCacheState{time: time.Now(), err: wrapped})
		return false, wrapped
	}
	dirty := res.dirty

	// Store the result. Return our own observation (`dirty`), not a re-read of
	// the slot: a lost write race (InvalidateDirtyCache after singleflight started)
	// is harmless — the next call will re-run git status when TTL expires.
	g.isDirtyCache.Store(dirtyCacheState{dirty: dirty, time: time.Now()})
	return dirty, nil
}

// IsBranchCheckedOut checks if the instance branch is currently checked out.
// Uses go-git to read HEAD directly (no subprocess).
func (g *GitWorktree) IsBranchCheckedOut() (bool, error) {
	current, err := getCurrentBranchName(g.repoPath)
	if err != nil {
		return false, fmt.Errorf("failed to get current branch: %w", err)
	}
	return current == g.branchName, nil
}

// OpenBranchURL opens the branch URL in the default browser
func (g *GitWorktree) OpenBranchURL() error {
	// Check if GitHub CLI is available
	if err := checkGHCLI(); err != nil {
		return err
	}

	browseCtx, browseCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer browseCancel()
	if _, err := g.commandRunner().Run(browseCtx, g.worktreePath, "gh", "browse", "--branch", g.branchName); err != nil {
		return fmt.Errorf("failed to open branch URL: %w", err)
	}
	return nil
}

func (g *GitWorktree) PushBranch() error {
	pushCtx, pushCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer pushCancel()
	if out, err := g.commandRunner().Run(pushCtx, g.worktreePath, "git", "push", "-u", "origin", g.branchName); err != nil {
		return fmt.Errorf("failed to push branch: %s (%w)", out, err)
	}
	return nil
}

// CreatePR creates a GitHub pull request for the current branch and returns the
// PR URL and number. Title defaults to the branch name if empty.
// If a PR already exists for the branch it is returned without creating a new one.
func (g *GitWorktree) CreatePR(title, body string) (prURL string, prNumber int, err error) {
	if err := checkGHCLI(); err != nil {
		return "", 0, err
	}
	if title == "" {
		title = strings.ReplaceAll(g.branchName, "-", " ")
	}

	// Check for an existing PR on this branch first.
	existingURL, existingNumber, existsErr := g.findExistingPR()
	if existsErr == nil && existingNumber > 0 {
		return existingURL, existingNumber, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := []string{"pr", "create", "--title", title, "--body", body, "--head", g.branchName}
	out, runErr := g.commandRunner().Run(ctx, g.worktreePath, "gh", args...)
	if runErr != nil {
		// A race: PR was created between our check and now. Re-check once.
		if u, n, err2 := g.findExistingPR(); err2 == nil && n > 0 {
			return u, n, nil
		}
		return "", 0, fmt.Errorf("gh pr create failed: %s (%w)", out, runErr)
	}

	// gh pr create prints the PR URL as the last line. Some gh versions treat
	// "PR already exists for this branch" as success rather than an error (the
	// findExistingPR race-check above already covers the common case, but not
	// every gh version/timing), so out may point at a pre-existing PR.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	prURL = strings.TrimSpace(lines[len(lines)-1])

	// Parse the number directly from the URL first — a plain string operation
	// that can't silently fail the way a second gh subprocess call can. Found
	// live: the separate `gh pr view --head` call below occasionally returned
	// empty/erroring output (its error was silently swallowed, leaving
	// prNumber at its zero value) even though prURL had already resolved
	// correctly — the resulting "PR #0" was then passed to EnablePRAutoMerge,
	// which predictably failed with "no pull requests found", so auto-merge
	// never got enabled for a PR that otherwise pushed and tracked correctly.
	if m := prNumberFromURLRe.FindStringSubmatch(prURL); m != nil {
		if n, convErr := strconv.Atoi(m[1]); convErr == nil && n > 0 {
			prNumber = n
		}
	}
	if prNumber == 0 {
		// Fallback: the URL didn't parse (unexpected format) — try the
		// original gh-view-based lookup as a last resort.
		numCtx, numCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer numCancel()
		numOut, numErr := g.commandRunner().Run(numCtx, g.worktreePath, "gh", "pr", "view", "--json", "number", "--jq", ".number", "--head", g.branchName)
		if numErr == nil {
			prNumber, _ = strconv.Atoi(strings.TrimSpace(string(numOut)))
		}
	}

	return prURL, prNumber, nil
}

// findExistingPR looks up an open PR for the branch. Returns (url, number, nil)
// if found, or ("", 0, err) if not found or on error.
func (g *GitWorktree) findExistingPR() (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := g.commandRunner().Run(ctx, g.worktreePath, "gh", "pr", "list", "--head", g.branchName,
		"--json", "number,url", "--jq", ".[0] | .number, .url")
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "", 0, fmt.Errorf("no existing PR")
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "", 0, fmt.Errorf("unexpected output")
	}
	num, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
	url := strings.TrimSpace(lines[1])
	if num == 0 {
		return "", 0, fmt.Errorf("no PR found")
	}
	return url, num, nil
}

// HasCommitsAheadOfMain reports whether this worktree's branch has at least
// one commit not present on mainBranch — i.e. whether there is genuinely
// anything to ship. Used as a pre-flight check before attempting CreatePR: a
// branch with zero commits ahead of main makes `gh pr create` fail with "No
// commits between X and Y", which is not a retryable push/PR failure (see
// BUG-063) but a signal that the item was already fully addressed elsewhere.
// Returns true (the safe, existing default: attempt PR creation as before) if
// the check itself is inconclusive — an error opening the repo, or the branch
// not existing locally — so a check failure never causes a caller to skip PR
// creation for a branch that may well need it.
func (g *GitWorktree) HasCommitsAheadOfMain(mainBranch string) (bool, error) {
	status, err := BranchAheadBehind(g.repoPath, g.branchName, mainBranch)
	if err != nil {
		return true, err
	}
	if !status.BranchExists {
		return true, nil
	}
	return status.AheadOfMain > 0, nil
}

// reviewInfo captures the blocking review that tripped HasBlockingReviews.
type reviewInfo struct{ author, body string }

// prFeedbackItem captures one piece of substantive PR feedback (a COMMENTED
// review or a plain comment) along with the GitHub-assigned timestamp it
// carries, so callers can compute a max-timestamp watermark for dedup.
type prFeedbackItem struct {
	author, body string
	at           time.Time
}

// substantiveFeedbackMinLen is the minimum trimmed-rune length a review/comment
// body must have to count as substantive feedback (filters bare "LGTM"-style
// noise out of the HasReviewFeedback signal).
const substantiveFeedbackMinLen = 10

// isSubstantiveFeedback reports whether body is long enough to be considered
// actionable feedback rather than noise (a bare "lgtm", empty, or whitespace-only
// body).
func isSubstantiveFeedback(body string) bool {
	return utf8.RuneCountInString(strings.TrimSpace(body)) >= substantiveFeedbackMinLen
}

// copilotReviewerLogin is the GitHub Copilot code-review bot account's login
// — the one "[bot]" account whose feedback IS meant to count toward
// HasReviewFeedback (it's this feature's motivating example). Mirrors the
// literal login RequestCopilotReview requests a review from.
const copilotReviewerLogin = "copilot-pull-request-reviewer[bot]"

// isExcludedBotAuthor reports whether login belongs to an automated bot
// account (GitHub's convention: a "[bot]" suffix, e.g. github-actions[bot],
// codecov[bot], dependabot[bot]) OTHER than Copilot's own review account.
// Without this exclusion, a long-enough recurring bot comment (a coverage
// report, a CI status summary) would pass isSubstantiveFeedback and
// repeatedly re-trigger a fix session on every push, burning the shared
// rework-cap budget on non-actionable text (pre-mortem.md #5).
func isExcludedBotAuthor(login string) bool {
	return login != copilotReviewerLogin && strings.HasSuffix(login, "[bot]")
}

// parseFeedbackTimestamp parses raw as an RFC3339 timestamp (GitHub's
// submittedAt/createdAt format), falling back to time.Now() on failure. A
// zero-valued fallback could lose to an already-persisted, later watermark
// and silently suppress detection of genuinely new feedback; time.Now() is
// guaranteed no earlier than any watermark this process could have already
// persisted, so it can only ever push LatestFeedbackAt later, never mask a
// real later item under an earlier one. fieldLabel names the source field,
// for the warning log.
func parseFeedbackTimestamp(raw, fieldLabel string) time.Time {
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		log.Warn("parsePRStatusPayload: failed to parse feedback timestamp", "field", fieldLabel, "value", raw, "err", err)
		return time.Now()
	}
	return at
}

// PRStatus holds the CI, review, and conflict state for a pull request.
type PRStatus struct {
	// CIFailing is true when at least one CI check has a terminal failure.
	CIFailing bool
	// HasBlockingReviews is true when a reviewer has requested changes.
	HasBlockingReviews bool
	// HasConflicts is true when GitHub reports mergeStateStatus == "DIRTY" or
	// mergeable == "CONFLICTING" — its branch cannot be merged as-is and needs
	// a rebase. Both fields are checked (see Task 1.1.1d) because gh's
	// mergeable field has been observed returning stale data (cli/cli#9583).
	HasConflicts bool
	// IsClosed is true when the PR's state is CLOSED (rejected by a human without
	// merging) rather than OPEN or MERGED. Callers must check this before treating
	// "not merged" as "still open and healthy" — a closed PR will never merge on
	// its own no matter how long ReconcilePRPending keeps polling it.
	IsClosed bool
	// IsDraft is true when the PR is still marked draft on GitHub. Captured from
	// the same gh pr view call as everything else on this struct (no second API
	// call) so callers such as the backlog stuck-item detector (prReadyToMergeSolo)
	// can gate on it without an extra fetch.
	IsDraft bool
	// Mergeable is the raw upper-cased GitHub `mergeable` field ("MERGEABLE",
	// "CONFLICTING", or "UNKNOWN"). HasConflicts is the belt-and-suspenders
	// bool derived from this plus mergeStateStatus (see above); Mergeable is
	// exposed separately for callers (prReadyToMergeSolo) that want the literal
	// "MERGEABLE" check called out in ADR-001 rather than the inverse-of-conflict
	// approximation.
	Mergeable string
	// ApprovedCount is the number of current non-dismissed APPROVED reviews.
	ApprovedCount int
	// ChangesRequestedCount is the number of current non-dismissed
	// CHANGES_REQUESTED reviews (equivalently, len of the reviews backing
	// HasBlockingReviews — exposed as a count so callers building a
	// github.PRInfo-shaped value don't need to re-derive it from the bool).
	ChangesRequestedCount int
	// HasReviewFeedback is true when at least one substantive COMMENTED-state
	// review or substantive plain PR comment exists (Copilot's typical review
	// posture is COMMENTED, not CHANGES_REQUESTED, so this is a distinct signal
	// from HasBlockingReviews). Non-substantive feedback (bare "lgtm", empty,
	// or whitespace-only bodies) never sets this.
	HasReviewFeedback bool
	// LatestFeedbackAt is the newest GitHub-assigned submittedAt/createdAt
	// timestamp among all substantive feedback captured this call; the zero
	// value when HasReviewFeedback is false. Callers use this as the dedup
	// watermark comparison point (see ReconcilePRPending's hasNewFeedback).
	LatestFeedbackAt time.Time
	// FeedbackText is a combined human-readable summary for the fix agent.
	FeedbackText string

	failedChecks    []string     // unexported; captured CI failures, consumed by render()
	blockingReviews []reviewInfo // unexported; one entry per CHANGES_REQUESTED review
	// conflictMergeStateStatus is the raw mergeStateStatus value that tripped
	// HasConflicts; only meaningful when HasConflicts is true. A plain string
	// rather than a nil-checked pointer, since HasConflicts is already the
	// single source of truth for "is there a conflict" — render() branches on
	// HasConflicts, not on this field's zero-ness.
	conflictMergeStateStatus string
	// commentReviews holds substantive COMMENTED-state reviews — today silently
	// dropped since only CHANGES_REQUESTED feeds blockingReviews.
	commentReviews  []prFeedbackItem
	generalComments []prFeedbackItem // unexported; existing "general comments" section content, retyped to carry timestamps
}

// render assembles FeedbackText from the fields captured during evaluation,
// in a fixed order (conflict first — features.md §2A), so FeedbackText can
// never drift from the bools it's derived from.
func (s *PRStatus) render() string {
	var sb strings.Builder

	if s.HasConflicts {
		sb.WriteString("## Merge conflict\n")
		fmt.Fprintf(&sb,
			"This PR's branch has merge conflicts against its base branch (mergeStateStatus=%s) "+
				"and cannot be merged as-is.\n"+
				"Rebase onto the base branch and resolve conflicts. This is not necessarily a "+
				"re-implementation of the original acceptance criteria — the PR's existing changes "+
				"are still correct, they just need to be replayed onto a moved base.\n\n"+
				"Follow these rules when resolving:\n"+
				"- Push with `git push --force-with-lease`, never `--force`. This fails safely if "+
				"the remote branch moved since you last fetched it, instead of silently discarding commits.\n"+
				"- If a conflicting file is a config file (for example `.gitignore`) and one side of "+
				"the conflict looks suspiciously short or placeholder-like compared to the other, prefer "+
				"the longer/more-complete side rather than guessing — this repo has hit real `.gitignore` "+
				"truncation incidents from automated rebases before.\n"+
				"- If you cannot confidently resolve a conflicting hunk, leave the conflict markers in "+
				"place, do not force-push, and say so clearly in your final message instead of guessing.\n"+
				"- Before finishing, run `git diff --stat` comparing your final branch against the base "+
				"branch's pre-rebase state, and paste that output verbatim into both your final report "+
				"and the PR description. Call out `.gitignore` or any other config/lockfile whose line-count "+
				"delta looks disproportionate. This rebase will force-push over the PR's existing diff, which "+
				"resets GitHub's review view — the diff-stat is the one artifact a human reviewer can check "+
				"against your summary instead of trusting it on faith.\n\n",
			s.conflictMergeStateStatus)
	}

	if len(s.failedChecks) > 0 {
		sb.WriteString("## Failing CI checks\n")
		for _, fc := range s.failedChecks {
			sb.WriteString("- " + fc + "\n")
		}
		sb.WriteString("\n")
	}

	for _, br := range s.blockingReviews {
		sb.WriteString("## Review: changes requested by @" + br.author + "\n")
		if br.body != "" {
			sb.WriteString(br.body + "\n\n")
		}
	}

	if len(s.commentReviews) > 0 {
		sb.WriteString("## Reviewer comments\n")
		for _, cr := range s.commentReviews {
			sb.WriteString("@" + cr.author + ": " + cr.body + "\n\n")
		}
	}

	if len(s.generalComments) > 0 {
		sb.WriteString("## PR comments\n")
		for _, c := range s.generalComments {
			sb.WriteString("@" + c.author + ": " + c.body + "\n\n")
		}
	}

	return sb.String()
}

// countableGeneralComments returns the subset of generalComments that count
// toward HasReviewFeedback/LatestFeedbackAt/FeedbackAuthors: substantive
// bodies from non-excluded-bot authors. commentReviews needs no equivalent
// filter here — it's already filtered to eligible entries at append time
// (see the COMMENTED case in parsePRStatusPayload).
func (s *PRStatus) countableGeneralComments() []prFeedbackItem {
	out := make([]prFeedbackItem, 0, len(s.generalComments))
	for _, gc := range s.generalComments {
		if !isSubstantiveFeedback(gc.body) || isExcludedBotAuthor(gc.author) {
			continue
		}
		out = append(out, gc)
	}
	return out
}

// FeedbackAuthors returns one author login per countable feedback item
// (COMMENTED reviews plus countableGeneralComments) captured this call — NOT
// deduplicated, so the same login appears once per item they authored.
// Callers use len() of this slice as an item count and the logins as an
// author list for a single hasNewFeedback-triggered dispatch, since a
// partially-addressed multi-item batch is otherwise silently unresolved
// forever once the dedup watermark advances past the whole batch.
func (s *PRStatus) FeedbackAuthors() []string {
	authors := make([]string, 0, len(s.commentReviews)+len(s.generalComments))
	for _, cr := range s.commentReviews {
		authors = append(authors, cr.author)
	}
	for _, gc := range s.countableGeneralComments() {
		authors = append(authors, gc.author)
	}
	return authors
}

// GetPRStatus fetches the combined CI check status, reviewer decisions,
// mergeability, and PR comments for the given pull request number.
func (g *GitWorktree) GetPRStatus(prNumber int) (*PRStatus, error) {
	if err := checkGHCLI(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	raw, err := g.commandRunner().Run(ctx, g.worktreePath, "gh", "pr", "view", strconv.Itoa(prNumber),
		"--json", "statusCheckRollup,reviews,comments,mergeable,mergeStateStatus,state,isDraft")
	if err != nil {
		return nil, fmt.Errorf("gh pr view failed: %s (%w)", raw, err)
	}

	return parsePRStatusPayload(raw)
}

// ParsePRStatusPayload parses gh pr view's combined JSON output into a
// PRStatus. Exported so callers outside this package (e.g.
// session/backlog_lifecycle_test.go's ReconcilePRPending fixtures) can build
// a *PRStatus with commentReviews/generalComments genuinely populated —
// FeedbackAuthors() depends on those unexported fields, which a struct
// literal from another package cannot set directly.
func ParsePRStatusPayload(raw []byte) (*PRStatus, error) {
	return parsePRStatusPayload(raw)
}

// parsePRStatusPayload parses gh pr view's combined JSON output, evaluates
// all PR-status signals into structured fields, and renders FeedbackText
// from them. It has no I/O dependency and is directly unit-testable.
func parsePRStatusPayload(raw []byte) (*PRStatus, error) {
	var payload struct {
		StatusCheckRollup []struct {
			Typename   string `json:"__typename"`
			Name       string `json:"name"`
			Context    string `json:"context"` // for StatusContext checks
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"` // for StatusContext checks
			DetailsURL string `json:"detailsUrl"`
			TargetURL  string `json:"targetUrl"`
		} `json:"statusCheckRollup"`
		Reviews []struct {
			State  string `json:"state"`
			Body   string `json:"body"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			SubmittedAt string `json:"submittedAt"`
		} `json:"reviews"`
		Comments []struct {
			Body   string `json:"body"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			CreatedAt string `json:"createdAt"`
		} `json:"comments"`
		Mergeable        string `json:"mergeable"`
		MergeStateStatus string `json:"mergeStateStatus"`
		State            string `json:"state"`
		IsDraft          bool   `json:"isDraft"`
	}
	if jsonErr := json.Unmarshal(raw, &payload); jsonErr != nil {
		return nil, fmt.Errorf("parse pr status: %w", jsonErr)
	}

	status := &PRStatus{}
	status.IsClosed = strings.ToUpper(payload.State) == "CLOSED"
	status.IsDraft = payload.IsDraft
	status.Mergeable = strings.ToUpper(payload.Mergeable)

	// Evaluate mergeability first — a PR that can't even be rebased makes
	// CI/review feedback moot until it's mergeable again. Check both fields:
	// cli/cli#9583 documents gh's `mergeable` field returning stale/incorrect
	// data vs. `mergeStateStatus` for the same PR (stack.md §3), so this is a
	// belt-and-suspenders OR, not a single-field check. UNKNOWN on both fields
	// falls through to "no signal this cycle" by construction — neither
	// comparison below matches "UNKNOWN".
	mss := strings.ToUpper(payload.MergeStateStatus)
	mg := strings.ToUpper(payload.Mergeable)
	if mss == "DIRTY" || mg == "CONFLICTING" {
		status.HasConflicts = true
		status.conflictMergeStateStatus = payload.MergeStateStatus
	}

	// Evaluate CI checks.
	for _, check := range payload.StatusCheckRollup {
		name := check.Name
		if name == "" {
			name = check.Context
		}
		// Terminal failures: FAILURE conclusion or failed state.
		conclusion := strings.ToUpper(check.Conclusion)
		state := strings.ToUpper(check.State)
		if conclusion == "FAILURE" || conclusion == "TIMED_OUT" || conclusion == "CANCELLED" ||
			state == "FAILURE" || state == "ERROR" {
			status.CIFailing = true
			url := check.DetailsURL
			if url == "" {
				url = check.TargetURL
			}
			entry := name + " FAILED"
			if url != "" {
				entry += " (" + url + ")"
			}
			status.failedChecks = append(status.failedChecks, entry)
		}
	}

	// Evaluate reviews.
	for _, r := range payload.Reviews {
		switch strings.ToUpper(r.State) {
		case "CHANGES_REQUESTED":
			status.HasBlockingReviews = true
			status.ChangesRequestedCount++
			status.blockingReviews = append(status.blockingReviews, reviewInfo{author: r.Author.Login, body: r.Body})
		case "APPROVED":
			status.ApprovedCount++
		case "COMMENTED":
			// Excluded-bot check alongside substantiveness: without it, a
			// long-enough recurring bot COMMENTED review would repeatedly
			// re-trigger a fix session, burning the shared rework-cap budget
			// on non-actionable text (pre-mortem.md #5). Copilot's own
			// review account is explicitly exempted — it's this feature's
			// motivating example.
			if !isSubstantiveFeedback(r.Body) || isExcludedBotAuthor(r.Author.Login) {
				continue
			}
			at := parseFeedbackTimestamp(r.SubmittedAt, "submittedAt")
			status.commentReviews = append(status.commentReviews, prFeedbackItem{author: r.Author.Login, body: r.Body, at: at})
		}
	}

	// Include general PR comments as context. Every comment is captured
	// unconditionally (unchanged behavior) — substantiveness/bot filtering
	// only affects what counts toward HasReviewFeedback (below), not what
	// renders in FeedbackText.
	for _, c := range payload.Comments {
		at := parseFeedbackTimestamp(c.CreatedAt, "createdAt")
		status.generalComments = append(status.generalComments, prFeedbackItem{author: c.Author.Login, body: c.Body, at: at})
	}

	for _, cr := range status.commentReviews {
		status.HasReviewFeedback = true
		if cr.at.After(status.LatestFeedbackAt) {
			status.LatestFeedbackAt = cr.at
		}
	}
	for _, gc := range status.countableGeneralComments() {
		status.HasReviewFeedback = true
		if gc.at.After(status.LatestFeedbackAt) {
			status.LatestFeedbackAt = gc.at
		}
	}

	status.FeedbackText = status.render()
	return status, nil
}

// EnablePRAutoMerge enables GitHub auto-merge on the given PR so it merges
// automatically once required CI checks pass. Best-effort: fails silently
// when the repo does not have auto-merge enabled in its branch protection rules.
func (g *GitWorktree) EnablePRAutoMerge(prNumber int) error {
	if err := checkGHCLI(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := g.commandRunner().Run(ctx, g.worktreePath, "gh", "pr", "merge", strconv.Itoa(prNumber), "--auto", "--squash")
	if err != nil {
		return fmt.Errorf("gh pr merge --auto failed: %s (%w)", out, err)
	}
	return nil
}

// RequestCopilotReview requests a GitHub Copilot code review on prNumber.
// Best-effort: fails when Copilot code review isn't enabled for the org/repo,
// or on any other gh error — callers must not fail PR creation on this error.
// Uses the legacy bot-login form (copilot-pull-request-reviewer[bot]) via
// --add-reviewer rather than the newer @copilot alias, since the literal
// login is accepted by every gh version this repo targets while the alias is
// version-gated (see plan.md's Pattern Decisions table).
func (g *GitWorktree) RequestCopilotReview(prNumber int) error {
	if err := checkGHCLI(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := g.commandRunner().Run(ctx, g.worktreePath, "gh", "pr", "edit", strconv.Itoa(prNumber), "--add-reviewer", copilotReviewerLogin)
	if err != nil {
		return fmt.Errorf("gh pr edit --add-reviewer copilot failed: %s (%w)", out, err)
	}
	return nil
}

// ClosePR closes prNumber without merging, posting comment as an explanatory
// PR comment first. Used when a PR is discovered to be superseded (its
// branch's work already landed on main through a different path) rather than
// genuinely broken — see BUG-032.
func (g *GitWorktree) ClosePR(prNumber int, comment string) error {
	if err := checkGHCLI(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := g.commandRunner().Run(ctx, g.worktreePath, "gh", "pr", "close", strconv.Itoa(prNumber), "--comment", comment)
	if err != nil {
		return fmt.Errorf("gh pr close failed: %s (%w)", out, err)
	}
	return nil
}

// IsPRMerged reports whether the given PR number has been merged.
func (g *GitWorktree) IsPRMerged(prNumber int) (bool, error) {
	if err := checkGHCLI(); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := g.commandRunner().Run(ctx, g.worktreePath, "gh", "pr", "view", strconv.Itoa(prNumber), "--json", "state", "--jq", ".state")
	if err != nil {
		return false, fmt.Errorf("gh pr view failed: %s (%w)", out, err)
	}
	return strings.TrimSpace(string(out)) == "MERGED", nil
}
