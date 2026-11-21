package mcp

import "github.com/tstapler/stapler-squad/session/git"

// Worktree is the slice of *git.GitWorktree the VCS and stuck-report tools use.
// A fake lets those tools run without a real git checkout or the gh CLI.
type Worktree interface {
	Diff() *git.DiffStats
	GetBranchName() string
	stuckReportPRChecker
}

// WorktreeOpener rebuilds a worktree from stored paths. The composition root
// supplies it (WithWorktreeOpener); it returns nil when the paths are unusable.
type WorktreeOpener func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) Worktree

// WithWorktreeOpener supplies the worktree constructor behind get_session_diff,
// list_session_branches and get_backlog_stuck_report's live CI cross-check.
// Without it those tools report the worktree as unavailable.
func WithWorktreeOpener(o WorktreeOpener) CoreOption {
	return func(c *coreConfig) { c.openWorktree = o }
}

// WorktreeOpenerFromOptions returns the opener the options carry, or nil.
// It lets wiring tests prove each transport injects one.
func WorktreeOpenerFromOptions(opts ...CoreOption) WorktreeOpener {
	return newCoreConfig(opts).openWorktree
}

// stuckReportCheckerFactory adapts an opener to the stuck report's per-repo
// factory; nil when no opener is injected.
func stuckReportCheckerFactory(o WorktreeOpener) func(repoPath string) stuckReportPRChecker {
	if o == nil {
		return nil
	}
	return func(repoPath string) stuckReportPRChecker {
		return o(repoPath, repoPath, "", "", "")
	}
}
