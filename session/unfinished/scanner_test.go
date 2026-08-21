package unfinished

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// ---- ParseAllWorktrees ---------------------------------------------------

func TestParseAllWorktrees_Normal(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	results := ParseAllWorktrees("")
	assert.Empty(t, results)
}

// ---- ScanResult.IsUnfinished --------------------------------------------

func TestIsUnfinished_Uncommitted(t *testing.T) {
	t.Parallel()
	r := ScanResult{HasUncommitted: true}
	assert.True(t, r.IsUnfinished())
}

func TestIsUnfinished_Ahead(t *testing.T) {
	t.Parallel()
	r := ScanResult{AheadCount: 3}
	assert.True(t, r.IsUnfinished())
}

func TestIsUnfinished_Behind(t *testing.T) {
	t.Parallel()
	r := ScanResult{BehindCount: 5}
	assert.True(t, r.IsUnfinished())
}

func TestIsUnfinished_None(t *testing.T) {
	t.Parallel()
	r := ScanResult{}
	assert.False(t, r.IsUnfinished())
}

func TestIsUnfinished_AllCriteria(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	t.Run("full stats", func(t *testing.T) {
		d := parseDiffShortstat("3 files changed, 142 insertions(+), 28 deletions(-)")
		assert.Equal(t, 3, d.Files)
		assert.Equal(t, 142, d.Insertions)
		assert.Equal(t, 28, d.Deletions)
	})

	t.Run("only insertions", func(t *testing.T) {
		d := parseDiffShortstat("1 file changed, 10 insertions(+)")
		assert.Equal(t, 1, d.Files)
		assert.Equal(t, 10, d.Insertions)
		assert.Equal(t, 0, d.Deletions)
	})

	t.Run("empty", func(t *testing.T) {
		d := parseDiffShortstat("")
		assert.Equal(t, 0, d.Files)
		assert.Equal(t, 0, d.Insertions)
		assert.Equal(t, 0, d.Deletions)
	})
}

// ---- SortByLastModified -------------------------------------------------

func TestSortByLastModified(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	c := &worktreeCache{ttl: 30 * time.Millisecond}
	r := ScanResult{Branch: "main", HasUncommitted: true}
	c.Set(r)

	got, ok := c.Get()
	require.True(t, ok, "should have fresh entry")
	assert.Equal(t, "main", got.Branch)

	// Wait for TTL to expire by polling until Get() returns false.
	if err := wait.WaitForCondition(func() bool {
		_, stillFresh := c.Get()
		return !stillFresh
	}, wait.WaitConfig{Timeout: 500 * time.Millisecond, PollInterval: 10 * time.Millisecond, Description: "cache TTL expiry"}); err != nil {
		t.Fatal("cache entry did not expire within timeout")
	}
	_, ok = c.Get()
	assert.False(t, ok, "should be expired")
}

func TestWorktreeCacheInvalidate(t *testing.T) {
	t.Parallel()
	c := &worktreeCache{ttl: time.Minute}
	c.Set(ScanResult{Branch: "feature"})
	c.Invalidate()
	_, ok := c.Get()
	assert.False(t, ok, "should be cleared after invalidate")
}

// ---- Circuit breaker ----------------------------------------------------

func TestCircuitBreaker_BackoffAfterThreeTimeouts(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	s := &Scanner{}
	repoPath := "/tmp/test-repo-2"

	s.recordTimeout(repoPath)
	s.recordTimeout(repoPath)
	s.recordTimeout(repoPath)
	require.False(t, s.shouldScan(repoPath))

	s.resetBreaker(repoPath)
	assert.True(t, s.shouldScan(repoPath), "should allow scan after reset")
}

// ---- removeStaleResult ---------------------------------------------------

// staleTestReader lets a single worktree flip from dirty to clean between
// two scanRepo calls, simulating "the only uncommitted file was deleted".
type staleTestReader struct {
	worktreePath   string
	hasUncommitted bool
}

func (r *staleTestReader) ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	return []WorktreeInfo{{Path: r.worktreePath, Branch: "feature-x"}}, nil
}
func (r *staleTestReader) ResolveDefaultBranch(repoPath string) string { return "" }
func (r *staleTestReader) HasUncommitted(worktreePath string) (bool, error) {
	return r.hasUncommitted, nil
}
func (r *staleTestReader) AheadBehind(worktreePath, base string) (int, int, error) {
	return 0, 0, nil
}
func (r *staleTestReader) CommitMessages(worktreePath, base string, max int) ([]string, error) {
	return nil, nil
}
func (r *staleTestReader) DiffShortstat(worktreePath string) (DiffStat, error) {
	return DiffStat{}, nil
}

func TestScanRepo_RemovesStaleResultWhenWorktreeBecomesClean(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()
	reader := &staleTestReader{worktreePath: repoPath, hasUncommitted: true}
	bus := pkgevents.NewEventBus(10)
	s := NewScannerWithReader(bus, nil, reader)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub, _ := bus.Subscribe(ctx)

	// First scan: dirty worktree is stored as an unfinished result.
	results := s.scanRepo(repoPath, false)
	require.Len(t, results, 1)
	s.publishResults(results)
	key := repoPath + "|feature-x"
	_, ok := s.resultStore.Load(key)
	require.True(t, ok, "dirty result should be stored")

	// Drain events from the first scan (work_updated + scan_completed) so
	// only the removal event is left to observe below.
	drain := true
	for drain {
		select {
		case <-sub:
		default:
			drain = false
		}
	}

	// The worktree goes clean (e.g. the uncommitted file was deleted).
	reader.hasUncommitted = false
	s.InvalidateCache(repoPath)

	results = s.scanRepo(repoPath, false)
	assert.Empty(t, results, "clean worktree should not be returned")

	_, ok = s.resultStore.Load(key)
	assert.False(t, ok, "stale result should be removed from resultStore")

	select {
	case evt := <-sub:
		assert.Equal(t, EventUnfinishedWorkRemoved, evt.Type)
		assert.Equal(t, key, evt.Context)
	case <-time.After(time.Second):
		t.Fatal("expected EventUnfinishedWorkRemoved to be published")
	}
}

// TestScanRepo_ForceBypassesPerWorktreeCache guards against a regression where
// force=true bypassed EnqueueRepo's repo-level TTL gate but scanWorktree kept
// reading its own independent per-worktree cache anyway — leaving a
// user-triggered scan (Refresh button) queued but still returning stale data.
func TestScanRepo_ForceBypassesPerWorktreeCache(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()
	reader := &staleTestReader{worktreePath: repoPath, hasUncommitted: true}
	s := NewScannerWithReader(pkgevents.NewEventBus(10), nil, reader)

	results := s.scanRepo(repoPath, false)
	require.Len(t, results, 1)
	require.True(t, results[0].HasUncommitted)

	// Underlying state changes, but the per-worktree cache is still warm —
	// an unforced scan must keep returning the stale (dirty) cached entry.
	reader.hasUncommitted = false
	results = s.scanRepo(repoPath, false)
	require.Len(t, results, 1, "unforced scan should return the stale cached (still-dirty) entry")
	assert.True(t, results[0].HasUncommitted)

	// A forced scan must bypass the cache and observe the real clean state
	// (and refresh the cache to match).
	results = s.scanRepo(repoPath, true)
	require.Empty(t, results, "forced scan should re-read live state, which is now clean")

	// Flip back to dirty: an unforced scan should still return the
	// now-stale clean cache entry the forced call above just wrote.
	reader.hasUncommitted = true
	results = s.scanRepo(repoPath, false)
	require.Empty(t, results, "unforced scan should still return the stale clean cache entry")

	results = s.scanRepo(repoPath, true)
	require.Len(t, results, 1, "forced scan must bypass the per-worktree cache and observe the new dirty state")
	assert.True(t, results[0].HasUncommitted)
}

// TestNewScannerWithReader_RegistersGoGitVCSReaderForDebugSnapshot asserts
// that constructing a Scanner with a real *GoGitVCSReader registers it as
// currentReader, so the process-wide /debug/blob-cache endpoint (see
// profiling.StartProfiling) can reach its BlobCacheStats without the
// profiling server needing a reference at startup (it starts before the
// scanner exists — see main.go).
func TestNewScannerWithReader_RegistersGoGitVCSReaderForDebugSnapshot(t *testing.T) {
	reader := &GoGitVCSReader{}
	bus := pkgevents.NewEventBus(10)
	NewScannerWithReader(bus, nil, reader)

	// Bump this specific reader's counters directly (white-box) and confirm
	// the package-level snapshot reflects THIS reader, not some other one a
	// prior test may have registered.
	reader.blobCacheHits = 3
	reader.blobCacheMisses = 1
	reader.blobCacheMissNanos = int64(10 * time.Millisecond)

	stats := BlobCacheStatsSnapshot()
	assert.Equal(t, int64(3), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, 10*time.Millisecond, stats.EstimatedTimeSaved/3)

	// A fake VCSReader (not *GoGitVCSReader) must NOT overwrite the
	// registration — the debug endpoint should keep pointing at a real
	// reader instead of silently going stale/zero.
	NewScannerWithReader(bus, nil, &staleTestReader{})
	stats = BlobCacheStatsSnapshot()
	assert.Equal(t, int64(3), stats.Hits, "registering a fake reader must not clear the real one's snapshot")
}
