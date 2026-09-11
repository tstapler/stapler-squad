// mmap_realrepo_test.go measures the mmap-vs-copy-based .idx loader heap
// allocation delta against a REAL, large repository's actual .idx file,
// rather than only the synthetic fixtures used elsewhere in this package
// (buildPackedFixture, capped at a few hundred commits — see
// mmap_stage2_test.go's BenchmarkMmapIndex_HeapAllocation_CopyVsMmap).
//
// The design doc (session/unfinished/design/pluggable-gitstore.md §7)
// explicitly flags this as an open gap: "This fixture is deliberately small
// (a few hundred KB, not a multi-GB monorepo — this project's own .git is
// ~17GB and does not fit in the tmpfs-backed test temp directory available
// in this sandbox) — the *ratio* is the load-bearing claim, and it should
// only improve on a larger, more realistic repository". This benchmark closes
// that gap using this repository's own .git directory in place, read-only —
// it never writes into or copies the real repo's object database; it only
// mmaps the existing on-disk .idx file(s) (a few MB) and parses them via
// both loader paths, exactly the same read-only operation the production
// scanner performs when it opens a worktree of this size of repo.
//
// This intentionally does NOT touch the multi-GB .pack file itself — only the
// small, already-materialized .idx sidecar — so it stays fast (single-digit
// milliseconds to parse tens of thousands of index entries) despite the
// underlying repository being large.
package gogitstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/cache"
)

// realRepoRootForBench resolves the absolute path of the repository this test
// file itself lives in, by walking up from runtime.Caller(0)'s own file path
// (session/unfinished/gogitstore/mmap_realrepo_test.go is always exactly
// three directories below the repo root in this repository's own layout).
// This is deliberately NOT hardcoded to a specific machine-local absolute
// path — it works in any checkout of this repository, on any machine, as
// long as the source tree's relative layout is preserved (which `go test`
// always requires anyway, since it compiles from the checked-out source).
// Takes testing.TB so it works from both t *testing.T and b *testing.B callers.
func realRepoRootForBench(tb testing.TB) string {
	tb.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		tb.Skip("could not determine this test file's path via runtime.Caller")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	if err != nil {
		tb.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// largestIdxFile returns the largest pack-*.idx file under gitDir/objects/pack
// (as a proxy for "most representative of this repo's real object count" —
// picking the largest avoids accidentally measuring a small leftover pack
// from an old gc cycle if more than one happens to be present) along with its
// size in bytes. Returns ("", 0) if none are found.
func largestIdxFile(gitDir string) (string, int64) {
	matches, err := filepath.Glob(filepath.Join(gitDir, "objects", "pack", "*.idx"))
	if err != nil || len(matches) == 0 {
		return "", 0
	}
	var best string
	var bestSize int64
	for _, m := range matches {
		fi, serr := os.Stat(m)
		if serr != nil {
			continue
		}
		if fi.Size() > bestSize {
			best = m
			bestSize = fi.Size()
		}
	}
	return best, bestSize
}

// minRealisticIdxBytes is the threshold below which this benchmark considers
// a checkout's .git too small/shallow to be a meaningful "real large repo"
// measurement (e.g. a shallow CI clone) and skips rather than reporting a
// number that would misrepresent the production case. This repository's own
// commondir's actual pack-*.idx is ~1.6MB (~57k objects) as of the
// measurement this benchmark's own log captures — comfortably above this floor.
const minRealisticIdxBytes = 250 * 1024

// BenchmarkMmapIndex_HeapAllocation_RealRepo is gogitstore's real-repo
// companion to mmap_stage2_test.go's BenchmarkMmapIndex_HeapAllocation_CopyVsMmap
// (synthetic fixture). It measures SharedObjectStore.ensureIndex()'s
// allocation cost under both loader modes against THIS repository's own,
// already-on-disk .idx file — read-only; nothing in the real repo's object
// database is written, copied, or modified. See package doc above for why
// this only touches the small .idx sidecar, not the multi-GB .pack file.
// b.ReportAllocs() reports allocs/op and B/op for each arm; compare via
// benchstat rather than a hand-rolled ratio assertion — see
// BenchmarkMmapIndex_HeapAllocation_CopyVsMmap's doc comment for why this
// class of "is A meaningfully cheaper than B" question belongs in -bench,
// not a pass/fail Test.
//
// Skips (rather than failing) when run against a checkout that doesn't have a
// large enough real pack to measure meaningfully (e.g. a shallow CI clone) —
// this is an environmental measurement, not a correctness proof, and a
// shallow checkout genuinely cannot answer the question this benchmark asks.
//
// Run explicitly with `go test -run '^$' -bench BenchmarkMmapIndex_HeapAllocation_RealRepo
// -benchmem ./session/unfinished/gogitstore/...`.
// realRepoIdxOrSkip resolves this checkout's largest real .idx file, skipping
// b (not failing it) when the environment can't provide a meaningful one —
// see largestIdxFile/minRealisticIdxBytes's doc comments for why a skip, not
// a failure, is correct here.
func realRepoIdxOrSkip(b *testing.B) (repoRoot, idxPath string, idxSize int64) {
	b.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		b.Skip("git binary not available")
	}

	repoRoot = realRepoRootForBench(b)
	gitDir := filepath.Join(repoRoot, ".git")
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		b.Skipf("no .git directory at resolved repo root %s — not running from a full checkout", repoRoot)
	}

	idxPath, idxSize = largestIdxFile(gitDir)
	if idxPath == "" {
		b.Skip("no packed .idx files found under .git/objects/pack — repo has no packs to measure (fully loose, or a bare fixture checkout)")
	}
	if idxSize < minRealisticIdxBytes {
		b.Skipf("largest .idx (%s, %d bytes) is below the %d-byte floor for a meaningful real-repo measurement — likely a shallow/small checkout", idxPath, idxSize, minRealisticIdxBytes)
	}
	return repoRoot, idxPath, idxSize
}

func BenchmarkMmapIndex_HeapAllocation_RealRepo(b *testing.B) {
	repoRoot, idxPath, idxSize := realRepoIdxOrSkip(b)

	_, commonFs, _, commonDirAbs, err := resolveGitFilesystems(repoRoot)
	if err != nil {
		b.Fatalf("resolveGitFilesystems(%s): %v", repoRoot, err)
	}
	b.Logf("real repo: %s", repoRoot)
	b.Logf("real repo .idx: %s (%d bytes on disk)", idxPath, idxSize)

	run := func(b *testing.B, useMmap bool) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			store := newSharedObjectStore(commonDirAbs, commonFs, cache.NewObjectLRU(cache.FileSize(1<<20)), 0, useMmap)
			if err := store.ensureIndex(); err != nil {
				b.Fatalf("ensureIndex(useMmap=%v): %v", useMmap, err)
			}
			// See TestRegistry_UseMmapIndex_True_EngagesMmapLoader's cleanup
			// comment (mmap_stage2_test.go): ensureIndex starts a pack-watch
			// goroutine per store when useMmap is true, which must be stopped
			// every iteration rather than only once at the end.
			store.stopPackWatch()
		}
	}

	b.Run("copy", func(b *testing.B) { run(b, false) })
	b.Run("mmap", func(b *testing.B) { run(b, true) })
}
