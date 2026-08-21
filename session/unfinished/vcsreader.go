package unfinished

// VCSReader is the read-only interface the Scanner uses to interrogate
// repositories. All methods must be safe to call concurrently.
// The interface is intentionally narrow — only what the scanner needs.
type VCSReader interface {
	// ListWorktrees returns all worktrees registered in the repo at repoPath.
	ListWorktrees(repoPath string) ([]WorktreeInfo, error)

	// ResolveDefaultBranch returns the ref to compare unfinished work against
	// (e.g. "origin/main"). Returns "" if no default branch can be determined.
	ResolveDefaultBranch(repoPath string) string

	// HasUncommitted reports whether worktreePath has uncommitted changes.
	HasUncommitted(worktreePath string) (bool, error)

	// AheadBehind returns how many commits worktreePath is ahead of and behind base.
	AheadBehind(worktreePath, base string) (ahead, behind int, err error)

	// CommitMessages returns up to max one-line commit messages that are in
	// worktreePath but not in base.
	CommitMessages(worktreePath, base string, max int) ([]string, error)

	// DiffShortstat returns a summary of the diff between HEAD and the working tree.
	DiffShortstat(worktreePath string) (DiffStat, error)
}

// DiffStat holds a summary of changed lines.
type DiffStat struct {
	Files, Insertions, Deletions int
}
