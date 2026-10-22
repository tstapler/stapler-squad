package unfinished_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/unfinished"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// benchRepo holds a controlled git repo used for all benchmarks.
// Created once per test binary run via TestMain or on first use.
type benchRepo struct {
	path string // absolute path (symlinks resolved)
	base string // name of the base branch (HEAD at commit 0)
}

// newBenchRepo creates a git repo with nCommits commits ahead of a base branch,
// plus one untracked file and one modified tracked file.
func newBenchRepo(b *testing.B, nCommits int) *benchRepo {
	b.Helper()
	repo, dir := testrepo.NewRepo(b)

	// Initial commit — becomes the base.
	initial := testrepo.Commit(b, repo, map[string]string{"README.md": "hello\n"}, "initial", testrepo.Epoch)
	testrepo.SetBranch(b, repo, "base", initial)

	// Add nCommits ahead of base.
	for i := range nCommits {
		name := fmt.Sprintf("file%d.txt", i)
		testrepo.Commit(b, repo, map[string]string{name: strconv.Itoa(i) + "\n"}, fmt.Sprintf("commit %d", i), testrepo.Epoch.Add(time.Duration(i+1)*time.Minute))
	}

	// Dirty working tree: modify a tracked file + leave one untracked.
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\nworld\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new\nfile\n"), 0644)

	return &benchRepo{path: dir, base: "base"}
}

type namedReader struct {
	name string
	r    unfinished.VCSReader
}

func readers() []namedReader {
	return []namedReader{
		{"GoGitVCSReader", &unfinished.GoGitVCSReader{}},
	}
}

// commitCounts to benchmark against — small (typical session), medium, large.
var commitCounts = []int{5, 20, 100}

func BenchmarkHasUncommitted(b *testing.B) {
	repo := newBenchRepo(b, 5)
	for _, nr := range readers() {
		b.Run(nr.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = nr.r.HasUncommitted(repo.path)
			}
		})
	}
}

func BenchmarkAheadBehind(b *testing.B) {
	for _, n := range commitCounts {
		repo := newBenchRepo(b, n)
		for _, nr := range readers() {
			b.Run(fmt.Sprintf("%s/%dcommits", nr.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _, _ = nr.r.AheadBehind(context.Background(), repo.path, repo.base)
				}
			})
		}
	}
}

func BenchmarkCommitMessages(b *testing.B) {
	for _, n := range commitCounts {
		repo := newBenchRepo(b, n)
		for _, nr := range readers() {
			b.Run(fmt.Sprintf("%s/%dcommits", nr.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _ = nr.r.CommitMessages(context.Background(), repo.path, repo.base, 20)
				}
			})
		}
	}
}

func BenchmarkDiffShortstat(b *testing.B) {
	repo := newBenchRepo(b, 5)
	for _, nr := range readers() {
		b.Run(nr.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = nr.r.DiffShortstat(repo.path)
			}
		})
	}
}

// BenchmarkDiffShortstatCached enforces that the TTL cache path in GoGitVCSReader
// is O(1) and allocation-free. Must be <1µs and 0 allocs/op on a warm cache.
// Failure means the fast path was broken (cache miss on every call).
func BenchmarkDiffShortstatCached(b *testing.B) {
	repo := newBenchRepo(b, 5)
	r := &unfinished.GoGitVCSReader{}
	// Warm the cache with one real call.
	if _, err := r.DiffShortstat(repo.path); err != nil {
		b.Fatalf("warm-up failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = r.DiffShortstat(repo.path)
	}
	// Post-bench assertion: cache hit must be allocation-free.
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = r.DiffShortstat(repo.path)
	})
	if allocs != 0 {
		b.Errorf("DiffShortstat cached path: got %.0f allocs, want 0", allocs)
	}
}

func BenchmarkListWorktrees(b *testing.B) {
	repo := newBenchRepo(b, 5)
	for _, nr := range readers() {
		b.Run(nr.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = nr.r.ListWorktrees(repo.path)
			}
		})
	}
}

func BenchmarkResolveDefaultBranch(b *testing.B) {
	repo := newBenchRepo(b, 5)
	for _, nr := range readers() {
		b.Run(nr.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = nr.r.ResolveDefaultBranch(repo.path)
			}
		})
	}
}

// BenchmarkFullScanCycle is the hot path the scanner runs every 30s per worktree.
func BenchmarkFullScanCycle(b *testing.B) {
	for _, n := range commitCounts {
		repo := newBenchRepo(b, n)
		for _, nr := range readers() {
			b.Run(fmt.Sprintf("%s/%dcommits", nr.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _ = nr.r.HasUncommitted(repo.path)
					_, _, _ = nr.r.AheadBehind(context.Background(), repo.path, repo.base)
					_, _ = nr.r.DiffShortstat(repo.path)
				}
			})
		}
	}
}

// BenchmarkCommitMessages_ColdThenWarm asserts that the second call to CommitMessages
// (warm reachableSetCache) is allocation-free compared to the cold path.
// This is the regression guard for the #1 mutex hotspot: 47.4B cycles, 38 events.
func BenchmarkCommitMessages_ColdThenWarm(b *testing.B) {
	for _, n := range commitCounts {
		repo := newBenchRepo(b, n)
		b.Run(fmt.Sprintf("GoGitVCSReader/%dcommits", n), func(b *testing.B) {
			r := &unfinished.GoGitVCSReader{}

			// Cold call: populates reachableSetCache and commitMessagesCache.
			if _, err := r.CommitMessages(context.Background(), repo.path, repo.base, 20); err != nil {
				b.Fatalf("cold call failed: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _ = r.CommitMessages(context.Background(), repo.path, repo.base, 20)
			}

			// Post-bench assertion: warm path (commitMessagesCache hit) must not
			// perform any graph-walk allocations. The only unavoidable allocs are:
			//   1. fmt.Sprintf to build the cache key (~1 alloc)
			//   2. sync.Map.Load boxing the key and value into interface{} (~2 allocs)
			// Any value >> 4 indicates the O(N) reachable-set walk is happening.
			allocs := testing.AllocsPerRun(100, func() {
				_, _ = r.CommitMessages(context.Background(), repo.path, repo.base, 20)
			})
			if allocs > 4 {
				b.Errorf("CommitMessages warm path: got %.0f allocs, want ≤4 (no graph walk)", allocs)
			}
		})
	}
}

// The benchmarks below measure the CURRENT code and report deterministic
// counters (decodes/op, stats/op, trees/op, blobs/op) next to the timings;
// the counters, not ns/op, are what later stories gate on (in Test*, never here).

func trackAll(b *testing.B, r *unfinished.CountingReader, paths []string) {
	b.Helper()
	for _, p := range paths {
		r.Track(b, p)
	}
}

// bulkRepo commits n files spread over 10 directories and returns its dir.
func bulkRepo(b *testing.B, n int) string {
	b.Helper()
	repo, dir := testrepo.NewRepo(b)
	files := make(map[string]string, n)
	for i := range n {
		files["d"+strconv.Itoa(i%10)+"/f"+strconv.Itoa(i)+".txt"] = strconv.Itoa(i) + "\n"
	}
	testrepo.CommitAll(b, repo, files, "bulk", testrepo.Epoch)
	return dir
}

// BenchmarkMultiWorktreeAheadBehind runs a cold AheadBehind from each of W
// worktrees (main plus W-1 linked) sharing one head and base.
func BenchmarkMultiWorktreeAheadBehind(b *testing.B) {
	for _, w := range []int{1, 3, 8} {
		b.Run(fmt.Sprintf("W=%d", w), func(b *testing.B) {
			paths := unfinished.DivergedWorktrees(b, w)
			r := unfinished.NewCountingReader()
			trackAll(b, r, paths)
			b.ReportAllocs()
			b.ResetTimer()
			before := r.DecodeCount()
			for b.Loop() {
				for _, p := range paths {
					r.InvalidateWorktree(p)
					if _, _, err := r.AheadBehind(context.Background(), p, "base"); err != nil {
						b.Fatal(err)
					}
				}
			}
			b.ReportMetric(float64(r.DecodeCount()-before)/float64(b.N), "decodes/op")
		})
	}
}

// BenchmarkReachableSetShared computes the cold reachable set of base from
// each of 3 worktrees of one repo (CommitMessages), the work an O(shared)
// implementation would do once.
func BenchmarkReachableSetShared(b *testing.B) {
	paths := unfinished.DivergedWorktrees(b, 3)
	r := unfinished.NewCountingReader()
	trackAll(b, r, paths)
	b.ReportAllocs()
	b.ResetTimer()
	before := r.DecodeCount()
	for b.Loop() {
		for _, p := range paths {
			r.ForgetWorktree(p)
			if _, err := r.CommitMessages(context.Background(), p, "base", 20); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportMetric(float64(r.DecodeCount()-before)/float64(b.N), "decodes/op")
}

// BenchmarkHasUncommittedClean scans a clean 2,000-file worktree cold.
func BenchmarkHasUncommittedClean(b *testing.B) {
	dir := bulkRepo(b, 2000)
	r := unfinished.NewCountingReader()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.InvalidateWorktree(dir)
		if dirty, err := r.HasUncommitted(dir); err != nil || dirty {
			b.Fatalf("HasUncommitted = %v, %v; want clean", dirty, err)
		}
	}
	b.ReportMetric(float64(r.StatCount())/float64(b.N), "stats/op")
}

// BenchmarkHeadTreeMapShared rebuilds the HEAD name->blob map cold for each of
// 3 worktrees of one 1,000-file repo.
func BenchmarkHeadTreeMapShared(b *testing.B) {
	dir := bulkRepo(b, 1000)
	paths := append([]string{dir}, testrepo.LinkedWorktrees(b, dir, 2)...)
	r := unfinished.NewCountingReader()
	trackAll(b, r, paths)
	b.ReportAllocs()
	b.ResetTimer()
	trees, commits := r.TreeReads(), r.DecodeCount()
	for b.Loop() {
		for _, p := range paths {
			r.ResolveHeadTreeCold(b, p)
		}
	}
	b.ReportMetric(float64(r.TreeReads()-trees)/float64(b.N), "trees/op")
	b.ReportMetric(float64(r.DecodeCount()-commits)/float64(b.N), "decodes/op")
}

// BenchmarkShortstatLines diffs a 2,000-line file with 15 changed lines
// against HEAD, cold, so the line-diff and blob-read cost is exercised.
func BenchmarkShortstatLines(b *testing.B) {
	repo, dir := testrepo.NewRepo(b)
	var orig, edited strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&orig, "line %d\n", i)
		if i%133 == 0 {
			fmt.Fprintf(&edited, "LINE %d changed\n", i)
		} else {
			fmt.Fprintf(&edited, "line %d\n", i)
		}
	}
	testrepo.Commit(b, repo, map[string]string{"big.txt": orig.String()}, "big", testrepo.Epoch)
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(edited.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	r := unfinished.NewCountingReader()
	r.Track(b, dir)
	b.ReportAllocs()
	b.ResetTimer()
	blobs, stats := r.BlobReads(), r.StatCount()
	for b.Loop() {
		r.InvalidateWorktree(dir)
		if st, err := r.DiffShortstat(dir); err != nil || st.Insertions == 0 {
			b.Fatalf("DiffShortstat = %+v, %v; want changes", st, err)
		}
	}
	b.ReportMetric(float64(r.BlobReads()-blobs)/float64(b.N), "blobs/op")
	b.ReportMetric(float64(r.StatCount()-stats)/float64(b.N), "stats/op")
}
