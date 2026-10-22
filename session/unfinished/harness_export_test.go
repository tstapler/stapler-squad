package unfinished

// Test-only exports for the external-package benchmarks (vcsreader_bench_test.go):
// a GoGitVCSReader whose repo handles read objects through countingStorer and
// whose tracked-file stat passes are counted via the statFn seam.

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
)

// CountingReader is a GoGitVCSReader with DecodeCount / tree / blob / StatCount
// counters. Track each worktree path once, before measuring.
type CountingReader struct {
	*GoGitVCSReader
	mu     sync.Mutex
	stores []*countingStorer
	stats  atomic.Int64
}

func NewCountingReader(flags ...Flags) *CountingReader {
	c := &CountingReader{GoGitVCSReader: &GoGitVCSReader{}}
	if len(flags) > 0 {
		c.setFlags(flags[0])
	}
	c.statFn = func(string) { c.stats.Add(1) }
	return c
}

// Track opens path through the reader's normal cache and swaps the cached repo
// handle for one whose storer counts object reads. Product code is unchanged:
// only the cachedRepo's *git.Repository is replaced, in the test.
func (c *CountingReader) Track(tb testing.TB, path string) {
	tb.Helper()
	entry, err := c.openRepoEntry(path)
	if err != nil {
		tb.Fatalf("Track %s: %v", path, err)
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	cs := &countingStorer{Storer: entry.repo.Storer}
	repo, err := git.Open(cs, osfs.New(path))
	if err != nil {
		tb.Fatalf("Track %s: reopen through counting storer: %v", path, err)
	}
	entry.repo = repo
	c.mu.Lock()
	c.stores = append(c.stores, cs)
	c.mu.Unlock()
}

func (c *CountingReader) sum(f func(*countingStorer) int64) (n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.stores {
		n += f(s)
	}
	return n
}

// DecodeCount is the total commit-object reads across tracked worktrees.
func (c *CountingReader) DecodeCount() int64 {
	return c.sum(func(s *countingStorer) int64 { return s.DecodeCount() })
}

// TreeReads is the total tree-object reads across tracked worktrees.
func (c *CountingReader) TreeReads() int64 {
	return c.sum(func(s *countingStorer) int64 { return s.TreeReads() })
}

// BlobReads is the total blob-object reads across tracked worktrees.
func (c *CountingReader) BlobReads() int64 {
	return c.sum(func(s *countingStorer) int64 { return s.BlobReads() })
}

// StatCount is the number of per-file lstat calls made by the tracked-file stat passes.
func (c *CountingReader) StatCount() int64 { return c.stats.Load() }

// ResolveHeadTreeCold forgets the cached HEAD tree map of path's repo entry
// and rebuilds it, so each call pays the cold cost resolveHeadTreeHashes has today.
func (c *CountingReader) ResolveHeadTreeCold(tb testing.TB, path string) {
	tb.Helper()
	entry, err := c.openRepoEntry(path)
	if err != nil {
		tb.Fatalf("ResolveHeadTreeCold %s: %v", path, err)
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.headTreeHash, entry.headTreeCache = [20]byte{}, nil
	if _, err := resolveHeadTreeHashes(c.GoGitVCSReader, entry, entry.repo); err != nil {
		tb.Fatalf("ResolveHeadTreeCold %s: %v", path, err)
	}
}

// DivergedWorktrees is divergedWorktrees for external-package benchmarks.
func DivergedWorktrees(tb testing.TB, w int) []string { return divergedWorktrees(tb, w) }

// readerWithFlags gives g the rollback-unit flags f and returns it.
func readerWithFlags(f Flags, g *GoGitVCSReader) *GoGitVCSReader {
	g.setFlags(f)
	return g
}
