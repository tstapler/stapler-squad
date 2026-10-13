package git

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
)

// gitBackend is the backend git operations go through: the injected one, else the process-wide one.
func (g *GitWorktree) gitBackend() backend.Backend {
	if g.refBackend != nil {
		return g.refBackend
	}
	return gitprovider.Backend()
}

// locate is where path lives for this worktree: Remote carrying the worktree's own runner when
// that runner is remote, Local otherwise (so remote git can never run on this machine).
func (g *GitWorktree) locate(path string) backend.RepoLocation {
	return backend.Locate(g.commandRunner(), "", path)
}

// headMergeBase returns the merge-base of this worktree's HEAD and branch, or "" when there is
// none or the lookup failed. HEAD is resolved in the worktree itself, not the shared repoPath,
// whose ambient checked-out branch can be anything a concurrent process left it on.
func (g *GitWorktree) headMergeBase(branch string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sha, err := g.gitBackend().MergeBase(ctx, g.locate(g.worktreePath), backend.MergeBaseRequest{Left: "HEAD", Right: backend.RefName(branch)})
	if err != nil {
		return ""
	}
	return string(sha)
}
