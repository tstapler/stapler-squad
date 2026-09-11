package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderConflictHunk_MatchesGitDefaultStyle covers Story 3.3.1's acceptance
// criterion: 7-character markers, no ||||||| section, exact byte match.
func TestRenderConflictHunk_MatchesGitDefaultStyle(t *testing.T) {
	t.Parallel()
	hunk := MergeHunk{Kind: RegionConflict, Ours: []string{"B"}, Theirs: []string{"X"}}

	got, err := renderConflictHunk(hunk, "HEAD", "origin/main")

	require.NoError(t, err)
	assert.Equal(t, "<<<<<<< HEAD\nB\n=======\nX\n>>>>>>> origin/main\n", got)
}

func TestRenderConflictHunk_should_ReturnError_When_HunkKindIsNotConflict(t *testing.T) {
	t.Parallel()
	hunk := MergeHunk{Kind: RegionOursOnly, Ours: []string{"B"}}

	_, err := renderConflictHunk(hunk, "HEAD", "origin/main")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "RegionConflict")
}
