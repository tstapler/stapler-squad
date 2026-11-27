package mcp

import (
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/instance"
)

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
func stuckReportCheckerFactory(o WorktreeOpener) func(repoPath, prURL string) stuckReportPRChecker {
	if o == nil {
		return nil
	}
	return func(repoPath, prURL string) stuckReportPRChecker {
		wt := o(repoPath, repoPath, "", "", "")
		if ref, ok := instance.PRURLRepo(prURL); ok && wt != nil {
			wt.SetGHRepo(ref.GHArg())
		}
		return wt
	}
}
