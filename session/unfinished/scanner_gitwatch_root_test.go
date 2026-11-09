package unfinished

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSharedWorktreesRoot(t *testing.T) {
	t.Parallel()
	repo := filepath.Join("/r", "repo", ".git")
	admin := filepath.Join(repo, "worktrees", "wt1")
	assert.Equal(t, filepath.Join(repo, "worktrees"), sharedWorktreesRoot(admin), "admin dirs share their repo's worktrees stream")
	assert.Equal(t, repo, sharedWorktreesRoot(repo), "a .git dir streams itself")
	other := filepath.Join("/r", "other", "worktrees", "x")
	assert.Equal(t, other, sharedWorktreesRoot(other), "only <repo>/.git/worktrees/* is shared")
}
