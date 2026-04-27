package unfinished

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ParseAllWorktrees ---------------------------------------------------

func TestParseAllWorktrees_Normal(t *testing.T) {
	input := `worktree /home/user/project
HEAD abc123
branch refs/heads/main

worktree /home/user/project-feature
HEAD def456
branch refs/heads/feature-auth

`
	results := ParseAllWorktrees(input)
	require.Len(t, results, 2)
	assert.Equal(t, "/home/user/project", results[0].Path)
	assert.Equal(t, "main", results[0].Branch)
	assert.Equal(t, "abc123", results[0].HEAD)
	assert.False(t, results[0].IsBare)
	assert.False(t, results[0].IsDetached)

	assert.Equal(t, "feature-auth", results[1].Branch)
}

func TestParseAllWorktrees_Bare(t *testing.T) {
	input := `worktree /srv/repo.git
HEAD 0000000000000000000000000000000000000000
bare

`
	results := ParseAllWorktrees(input)
	require.Len(t, results, 1)
	assert.True(t, results[0].IsBare)
	assert.Equal(t, "", results[0].Branch)
}

func TestParseAllWorktrees_Detached(t *testing.T) {
	input := `worktree /tmp/wt-detached
HEAD deadbeef
detached

`
	results := ParseAllWorktrees(input)
	require.Len(t, results, 1)
	assert.True(t, results[0].IsDetached)
	assert.Equal(t, "", results[0].Branch)
}

func TestParseAllWorktrees_Prunable(t *testing.T) {
	input := `worktree /tmp/wt-prunable
HEAD cafebabe
branch refs/heads/old-branch
prunable gitdir file points to non-existent location

`
	results := ParseAllWorktrees(input)
	require.Len(t, results, 1)
	assert.True(t, results[0].IsPrunable)
	assert.Equal(t, "old-branch", results[0].Branch)
}

func TestParseAllWorktrees_Locked(t *testing.T) {
	input := `worktree /tmp/wt-locked
HEAD 1234abcd
branch refs/heads/locked-branch
locked

`
	results := ParseAllWorktrees(input)
	require.Len(t, results, 1)
	assert.True(t, results[0].IsLocked)
}

func TestParseAllWorktrees_Empty(t *testing.T) {
	results := ParseAllWorktrees("")
	assert.Empty(t, results)
}

// ---- ScanResult.IsUnfinished --------------------------------------------

func TestIsUnfinished_Uncommitted(t *testing.T) {
	r := ScanResult{HasUncommitted: true}
	assert.True(t, r.IsUnfinished())
}

func TestIsUnfinished_Ahead(t *testing.T) {
	r := ScanResult{AheadCount: 3}
	assert.True(t, r.IsUnfinished())
}

func TestIsUnfinished_Behind(t *testing.T) {
	r := ScanResult{BehindCount: 5}
	assert.True(t, r.IsUnfinished())
}

func TestIsUnfinished_None(t *testing.T) {
	r := ScanResult{}
	assert.False(t, r.IsUnfinished())
}

func TestIsUnfinished_AllCriteria(t *testing.T) {
	table := []struct {
		name           string
		hasUncommitted bool
		ahead          int
		behind         int
		wantUnfinished bool
	}{
		{"uncommitted only", true, 0, 0, true},
		{"ahead only", false, 2, 0, true},
		{"behind only", false, 0, 3, true},
		{"all zero", false, 0, 0, false},
		{"all set", true, 1, 1, true},
	}
	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			r := ScanResult{
				HasUncommitted: tc.hasUncommitted,
				AheadCount:     tc.ahead,
				BehindCount:    tc.behind,
			}
			assert.Equal(t, tc.wantUnfinished, r.IsUnfinished())
		})
	}
}

// ---- parseDiffShortstat -------------------------------------------------

func TestParseDiffShortstat(t *testing.T) {
	t.Run("full stats", func(t *testing.T) {
		var r ScanResult
		parseDiffShortstat("3 files changed, 142 insertions(+), 28 deletions(-)", &r)
		assert.Equal(t, 3, r.ChangedFiles)
		assert.Equal(t, 142, r.LinesAdded)
		assert.Equal(t, 28, r.LinesRemoved)
	})

	t.Run("only insertions", func(t *testing.T) {
		var r ScanResult
		parseDiffShortstat("1 file changed, 10 insertions(+)", &r)
		assert.Equal(t, 1, r.ChangedFiles)
		assert.Equal(t, 10, r.LinesAdded)
		assert.Equal(t, 0, r.LinesRemoved)
	})

	t.Run("empty", func(t *testing.T) {
		var r ScanResult
		parseDiffShortstat("", &r)
		assert.Equal(t, 0, r.ChangedFiles)
		assert.Equal(t, 0, r.LinesAdded)
		assert.Equal(t, 0, r.LinesRemoved)
	})
}

// ---- SortByLastModified -------------------------------------------------

func TestSortByLastModified(t *testing.T) {
	now := time.Now()
	results := []ScanResult{
		{Branch: "a", LastModified: now.Add(-5 * time.Minute)},
		{Branch: "b", LastModified: now.Add(-1 * time.Minute)},
		{Branch: "c", LastModified: now.Add(-3 * time.Minute)},
	}
	SortByLastModified(results)
	assert.Equal(t, "b", results[0].Branch, "most recent first")
	assert.Equal(t, "c", results[1].Branch)
	assert.Equal(t, "a", results[2].Branch)
}

func TestSortByLastModified_Stable(t *testing.T) {
	now := time.Now()
	// Equal times — sort by RepoPath+Branch.
	results := []ScanResult{
		{RepoPath: "z", Branch: "a", LastModified: now},
		{RepoPath: "a", Branch: "b", LastModified: now},
	}
	SortByLastModified(results)
	// "a|b" < "z|a" lexicographically.
	assert.Equal(t, "b", results[0].Branch)
	assert.Equal(t, "a", results[1].Branch)
}

// ---- worktreeCache TTL --------------------------------------------------

func TestWorktreeCacheTTL(t *testing.T) {
	c := &worktreeCache{ttl: 30 * time.Millisecond}
	r := ScanResult{Branch: "main", HasUncommitted: true}
	c.Set(r)

	got, ok := c.Get()
	require.True(t, ok, "should have fresh entry")
	assert.Equal(t, "main", got.Branch)

	// Wait for TTL to expire.
	time.Sleep(40 * time.Millisecond)
	_, ok = c.Get()
	assert.False(t, ok, "should be expired")
}

func TestWorktreeCacheInvalidate(t *testing.T) {
	c := &worktreeCache{ttl: time.Minute}
	c.Set(ScanResult{Branch: "feature"})
	c.Invalidate()
	_, ok := c.Get()
	assert.False(t, ok, "should be cleared after invalidate")
}

// ---- Circuit breaker ----------------------------------------------------

func TestCircuitBreaker_BackoffAfterThreeTimeouts(t *testing.T) {
	s := &Scanner{}
	repoPath := "/tmp/test-repo"

	assert.True(t, s.shouldScan(repoPath))

	s.recordTimeout(repoPath)
	s.recordTimeout(repoPath)
	assert.True(t, s.shouldScan(repoPath), "two timeouts should not trigger backoff")

	s.recordTimeout(repoPath)
	assert.False(t, s.shouldScan(repoPath), "three timeouts should trigger backoff")
}

func TestCircuitBreaker_ResetOnSuccess(t *testing.T) {
	s := &Scanner{}
	repoPath := "/tmp/test-repo-2"

	s.recordTimeout(repoPath)
	s.recordTimeout(repoPath)
	s.recordTimeout(repoPath)
	require.False(t, s.shouldScan(repoPath))

	s.resetBreaker(repoPath)
	assert.True(t, s.shouldScan(repoPath), "should allow scan after reset")
}
