package native

import (
	"os"
	"path/filepath"
	"testing"
)

// nestedDirsForBench creates n nested subdirectories, each holding a .gitignore file,
// so gitignore.ReadPatterns has real recursive work to do — this is what makes the
// cache-hit vs. cache-miss cost difference in BenchmarkWorktreeIsDirty_RepeatedCalls
// measurable rather than noise.
func nestedDirsForBench(b *testing.B, root string, n int) {
	b.Helper()
	dir := root
	for i := 0; i < n; i++ {
		dir = filepath.Join(dir, "pkg")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o600); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWorktreeIsDirty_RepeatedCalls_CachedFS is PerfFix-1's enforcement: repeated
// worktreeIsDirtyWithFS calls against the same GitignoreFSCache must not re-pay the
// full directory-listing/.gitignore-read cost on every call. Run with -benchmem and
// compare against BenchmarkWorktreeIsDirty_RepeatedCalls_Uncached via benchstat — a
// regression that drops the cache shows allocs/op collapsing back to the uncached
// baseline instead of staying far below it after the first call.
func BenchmarkWorktreeIsDirty_RepeatedCalls_CachedFS(b *testing.B) {
	repoDir := setupBenchRepo(b)
	nestedDirsForBench(b, repoDir, 25)

	var cache GitignoreFSCache
	// Warm the cache once outside the timed loop, matching production usage where
	// IsDirtyWithHint's 30s/5min TTL means most calls land on an already-warm cache.
	if _, err := worktreeIsDirtyWithFS(repoDir, &cache); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := worktreeIsDirtyWithFS(repoDir, &cache); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWorktreeIsDirty_RepeatedCalls_Uncached is the baseline: every call re-walks
// and re-parses the full gitignore pattern set from scratch, the behavior
// worktreeIsDirty (nil cache) still has for its direct callers.
func BenchmarkWorktreeIsDirty_RepeatedCalls_Uncached(b *testing.B) {
	repoDir := setupBenchRepo(b)
	nestedDirsForBench(b, repoDir, 25)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := worktreeIsDirty(repoDir); err != nil {
			b.Fatal(err)
		}
	}
}
