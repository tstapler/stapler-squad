package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

func TestWorktreeEntries_MarksFirstAsMainAndTrimsBranchPrefix(t *testing.T) {
	t.Parallel()
	got := worktreeEntries([]backend.WorktreeInfo{
		{Path: "/repo", Branch: "refs/heads/main"},
		{Path: "/repo-wt", Branch: "refs/heads/feat/x"},
		{Path: "/repo-detached", Detached: true},
	})

	assert.Len(t, got, 3)
	assert.True(t, got[0].IsMain)
	assert.Equal(t, "main", got[0].Branch)
	assert.False(t, got[1].IsMain)
	assert.Equal(t, "feat/x", got[1].Branch)
	assert.Equal(t, "/repo-detached", got[2].Path)
	assert.Empty(t, got[2].Branch)
}

func TestWorktreeEntries_EmptyInputYieldsEmptyNonNilList(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, worktreeEntries(nil))
}
