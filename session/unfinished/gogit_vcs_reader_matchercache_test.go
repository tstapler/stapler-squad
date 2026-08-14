package unfinished

// White-box coverage for PerfFix-1: decoupling the compiled gitignore
// matcher's lifetime from cachedRepo's. pruneRepoCache evicts *cachedRepo
// entries purely for memory pressure (TTL/LRU), independent of whether HEAD
// or .gitignore actually moved; without matcherCache, every re-open after
// such an eviction paid gitignore.ReadPatterns' full recursive directory walk
// again even though the repo's ignore rules hadn't changed.

import (
	"sync/atomic"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func TestGetOrBuildUntrackedMatcher_SurvivesCachedRepoEviction_WhenHeadUnchanged(t *testing.T) {
	dir := newGitignoreBenchRepo(t, 1)
	g := &GoGitVCSReader{}
	headHash := plumbing.NewHash("0000000000000000000000000000000000000001")

	entry1 := &cachedRepo{headTreeHash: headHash}
	if m := g.getOrBuildUntrackedMatcher(entry1, dir); m == nil {
		t.Fatal("expected a non-nil matcher on first build")
	}
	if got := atomic.LoadInt64(&g.gitignoreMatcherRebuilds); got != 1 {
		t.Fatalf("got %d rebuilds after first build, want 1", got)
	}

	// Simulate pruneRepoCache evicting the cachedRepo for memory pressure and
	// the caller reopening it: a fresh *cachedRepo with the same HEAD, zero
	// -value untrackedMatcherBuilt/untrackedMatcher fields.
	entry2 := &cachedRepo{headTreeHash: headHash}
	if m := g.getOrBuildUntrackedMatcher(entry2, dir); m == nil {
		t.Fatal("expected a non-nil matcher reused from matcherCache")
	}
	if got := atomic.LoadInt64(&g.gitignoreMatcherRebuilds); got != 1 {
		t.Errorf("got %d rebuilds after eviction-driven reopen with unchanged HEAD, want still 1 (matcherCache should have served it)", got)
	}
	if !entry2.untrackedMatcherBuilt {
		t.Error("expected entry2.untrackedMatcherBuilt to be set from the cached matcher")
	}
}

func TestGetOrBuildUntrackedMatcher_Rebuilds_WhenHeadChanged(t *testing.T) {
	dir := newGitignoreBenchRepo(t, 1)
	g := &GoGitVCSReader{}

	entry1 := &cachedRepo{headTreeHash: plumbing.NewHash("0000000000000000000000000000000000000001")}
	g.getOrBuildUntrackedMatcher(entry1, dir)

	entry2 := &cachedRepo{headTreeHash: plumbing.NewHash("0000000000000000000000000000000000000002")}
	g.getOrBuildUntrackedMatcher(entry2, dir)

	if got := atomic.LoadInt64(&g.gitignoreMatcherRebuilds); got != 2 {
		t.Errorf("got %d rebuilds after a real HEAD change, want 2 (matcherCache must not serve a stale matcher across HEAD moves)", got)
	}
}

// BenchmarkGetOrBuildUntrackedMatcher_RepeatedAccessUnderCacheEviction
// demonstrates PerfFix-1's effect: each call simulates repoCache evicting and
// reopening cachedRepo (a fresh struct, same HEAD) — pre-fix, this paid
// gitignore.ReadPatterns' full directory walk every time; post-fix,
// matcherCache serves every call after the first.
func BenchmarkGetOrBuildUntrackedMatcher_RepeatedAccessUnderCacheEviction(b *testing.B) {
	dir := newGitignoreBenchRepo(b, 5_000)
	g := &GoGitVCSReader{}
	headHash := plumbing.NewHash("0000000000000000000000000000000000000001")

	// Warm the cache once, matching real usage where the first open pays the
	// unavoidable cost.
	g.getOrBuildUntrackedMatcher(&cachedRepo{headTreeHash: headHash}, dir)

	b.ResetTimer()
	allocs := testing.AllocsPerRun(20, func() {
		entry := &cachedRepo{headTreeHash: headHash}
		g.getOrBuildUntrackedMatcher(entry, dir)
	})
	b.ReportMetric(allocs, "allocs/op")
}
