package git

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// BranchMergeResult describes MergeBranchIntoCheckedOut's outcome.
type BranchMergeResult struct {
	// UpToDate is true when branch was already merged into the checked-out branch.
	UpToDate bool
	// Conflicted is true when the merge hit conflicts. The merge is always aborted
	// before returning, so repoPath is left clean.
	Conflicted bool
	// ConflictedFiles lists the conflicting paths when Conflicted is true.
	ConflictedFiles []string
}

const branchMergeTimeout = 60 * time.Second

// MergeBranchIntoCheckedOut merges the local branch into whatever branch is currently
// checked out at repoPath (no fetch, no remote involved — unlike MergeMainIntoWorktree,
// which merges origin/<main>). It shells out because go-git cannot perform a three-way
// merge; on conflict it aborts so callers never see a half-merged tree.
// kibitzer:ignore primitive-obsession -- mirrors MergeMainIntoWorktree(worktreePath, mainBranch)
func MergeBranchIntoCheckedOut(repoPath, branch string) (*BranchMergeResult, error) {
	if branch == "" || strings.HasPrefix(branch, "-") {
		return nil, fmt.Errorf("invalid branch name %q", branch)
	}
	ctx, cancel := context.WithTimeout(context.Background(), branchMergeTimeout)
	defer cancel()

	out, mergeErr := runGitIn(ctx, repoPath, "merge", "--no-ff", "--no-edit", branch)
	if mergeErr == nil {
		return &BranchMergeResult{UpToDate: strings.Contains(out, "Already up to date")}, nil
	}

	conflicts, diffErr := runGitIn(ctx, repoPath, "diff", "--name-only", "--diff-filter=U")
	files := strings.Fields(conflicts)
	if diffErr != nil || len(files) == 0 {
		return nil, fmt.Errorf("git merge %s failed: %w: %s", branch, mergeErr, strings.TrimSpace(out))
	}
	if _, abortErr := runGitIn(ctx, repoPath, "merge", "--abort"); abortErr != nil {
		return nil, fmt.Errorf("git merge %s conflicted and abort failed: %w", branch, abortErr)
	}
	return &BranchMergeResult{Conflicted: true, ConflictedFiles: files}, nil
}

func runGitIn(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := safeexec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
