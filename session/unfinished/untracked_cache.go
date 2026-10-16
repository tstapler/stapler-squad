package unfinished

import (
	"sync/atomic"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// untrackedCache is the per-worktree untracked-directory verdict cache
// (Story 4.2.3).
type untrackedCache struct {
	accessedAtNs atomic.Int64
}

// hasUntrackedSkip reports whether any non-ignored file is untracked.
func (g *GoGitVCSReader) hasUntrackedSkip(worktreePath string, indexed map[string]struct{}, matcher gitignore.Matcher, _ trackedGen, _ bool) (bool, error) {
	return hasUntrackedFiles(worktreePath, indexed, matcher)
}
