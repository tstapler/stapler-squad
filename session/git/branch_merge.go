package git

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/tstapler/stapler-squad/session/git/native"
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

// MergeBranchIntoCheckedOut merges the local branch into whatever branch is currently
// checked out at repoPath (no fetch, no remote involved). It reuses the native merge
// pipeline MergeMainIntoWorktree runs, which only knows how to merge a remote-tracking
// ref: the branch tip is exposed as a temporary refs/remotes/origin/<branch> (restoring any
// real one afterwards) and the fetch step is skipped. On conflict the merge is aborted, so
// callers never see a half-merged tree.
// kibitzer:ignore primitive-obsession -- mirrors MergeMainIntoWorktree(worktreePath, mainBranch)
func MergeBranchIntoCheckedOut(repoPath, branch string) (*BranchMergeResult, error) {
	if branch == "" || strings.HasPrefix(branch, "-") {
		return nil, fmt.Errorf("invalid branch name %q", branch)
	}
	repo, err := OpenRepo(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", repoPath, err)
	}
	tip, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return nil, fmt.Errorf("resolve branch %q in %s: %w", branch, repoPath, err)
	}

	trackingName := plumbing.NewRemoteReferenceName("origin", branch)
	prior, priorErr := repo.Storer.Reference(trackingName)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(trackingName, tip.Hash())); err != nil {
		return nil, fmt.Errorf("expose %q for merge: %w", branch, err)
	}
	defer func() {
		if priorErr == nil {
			_ = repo.Storer.SetReference(prior)
		} else {
			_ = repo.Storer.RemoveReference(trackingName)
		}
	}()

	deps := nativeMergeDeps
	deps.Fetch = func(string, string) error { return nil }
	res, err := native.MergeMainIntoWorktree(deps, repoPath, branch)
	if err != nil {
		return nil, fmt.Errorf("merge %q into %s: %w", branch, repoPath, err)
	}
	if res == nil {
		return nil, errors.New("merge returned no result")
	}
	return &BranchMergeResult{UpToDate: res.UpToDate, Conflicted: res.Conflicted, ConflictedFiles: res.ConflictedFiles}, nil
}

// RepoHasUncommittedChanges reports whether repoPath's working tree has uncommitted
// changes, which a merge there would mix in or trip over.
func RepoHasUncommittedChanges(repoPath string) (bool, error) {
	var ignoreCache gitignoreFSCache
	var headCache headTreeHashCache
	return worktreeIsDirtyFast(repoPath, &ignoreCache, &headCache)
}
