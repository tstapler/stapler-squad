package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// FetchBranch fetches a specific branch from the origin remote.
func FetchBranch(repoPath, branchName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "origin", branchName)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to fetch branch: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to fetch branch: %w", err)
	}
	return nil
}

// CheckoutBranch checks out a branch in an existing repository.
func CheckoutBranch(repoPath, branchName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", repoPath, "checkout", branchName)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to checkout branch: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to checkout branch: %w", err)
	}
	return nil
}

// RemoteURL returns the URL of the named remote (usually "origin") for a local repo.
func RemoteURL(repoPath, remote string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", remote)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to get remote URL: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// MergeMainResult describes the outcome of MergeMainIntoWorktree.
type MergeMainResult struct {
	// UpToDate is true when the worktree's branch already contained everything
	// from mainBranch — nothing was merged in.
	UpToDate bool
	// Merged is true when the merge (including a fast-forward) brought in new
	// commits from mainBranch.
	Merged bool
	// Conflicted is true when merging mainBranch produced conflicts. The merge is
	// always aborted before returning, so the worktree is left clean either way —
	// callers never have to clean up a half-merged tree.
	Conflicted bool
	// ConflictedFiles lists the paths that conflicted. Populated only when
	// Conflicted is true.
	ConflictedFiles []string
}

// MergeMainIntoWorktree fetches mainBranch from origin and merges it into whatever
// branch is currently checked out in worktreePath. It never leaves the worktree in a
// conflicted state: on conflict it aborts the merge immediately (via `git merge
// --abort`) and reports the conflicting paths, so the caller can hand that context to
// whoever resolves it rather than leaving a half-merged working tree behind for the
// next thing that touches it.
func MergeMainIntoWorktree(worktreePath, mainBranch string) (*MergeMainResult, error) {
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer fetchCancel()
	fetchCmd := safeexec.CommandContext(fetchCtx, "git", "-C", worktreePath, "fetch", "origin", mainBranch)
	if out, err := fetchCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %s (%w)", mainBranch, out, err)
	}

	// Capture HEAD before the merge so up-to-date can be detected by comparing SHAs
	// rather than parsing merge output text ("Already up to date." is locale- and
	// git-version-dependent, e.g. older git prints "Already up-to-date.").
	beforeSHA, headErr := getHeadCommitSHA(worktreePath)
	if headErr != nil {
		return nil, fmt.Errorf("failed to resolve HEAD before merge: %w", headErr)
	}

	mergeCtx, mergeCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer mergeCancel()
	mergeCmd := safeexec.CommandContext(mergeCtx, "git", "-C", worktreePath, "merge", "--no-edit", "origin/"+mainBranch)
	mergeOut, mergeErr := mergeCmd.CombinedOutput()
	if mergeErr == nil {
		afterSHA, headErr := getHeadCommitSHA(worktreePath)
		if headErr != nil {
			return nil, fmt.Errorf("failed to resolve HEAD after merge: %w", headErr)
		}
		if afterSHA == beforeSHA {
			return &MergeMainResult{UpToDate: true}, nil
		}
		return &MergeMainResult{Merged: true}, nil
	}

	// The merge failed. Distinguish real conflicts (recoverable — abort and report)
	// from any other git failure (propagate as-is; aborting a non-conflict failure
	// could mask the real problem).
	conflictFiles, conflictErr := conflictedFiles(worktreePath)
	if conflictErr != nil || len(conflictFiles) == 0 {
		return nil, fmt.Errorf("failed to merge %s: %s (%w)", mainBranch, mergeOut, mergeErr)
	}

	abortCtx, abortCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer abortCancel()
	abortCmd := safeexec.CommandContext(abortCtx, "git", "-C", worktreePath, "merge", "--abort")
	if abortOut, abortErr := abortCmd.CombinedOutput(); abortErr != nil {
		return nil, fmt.Errorf("merge of %s conflicted in %v, and merge --abort failed: %s (%w)", mainBranch, conflictFiles, abortOut, abortErr)
	}

	return &MergeMainResult{Conflicted: true, ConflictedFiles: conflictFiles}, nil
}

// conflictedFiles returns the paths with unresolved merge conflicts in worktreePath.
func conflictedFiles(worktreePath string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", worktreePath, "diff", "--name-only", "--diff-filter=U")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}
