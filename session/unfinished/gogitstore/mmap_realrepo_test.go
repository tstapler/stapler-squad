// mmap_realrepo_test.go measures the mmap-vs-copy-based .idx loader heap
// allocation delta against a REAL, large repository's actual .idx file,
// rather than only the synthetic fixtures used elsewhere in this package
// (buildPackedFixture, capped at a few hundred commits — see
// mmap_stage2_test.go's TestMmapIndex_HeapAllocation_LowerThanCopyBased).
//
// The design doc (session/unfinished/design/pluggable-gitstore.md §7)
// explicitly flags this as an open gap: "This fixture is deliberately small
// (a few hundred KB, not a multi-GB monorepo — this project's own .git is
// ~17GB and does not fit in the tmpfs-backed test temp directory available
// in this sandbox) — the *ratio* is the load-bearing claim, and it should
// only improve on a larger, more realistic repository". This test closes
// that gap using this repository's own .git directory in place, read-only —
// it never writes into or copies the real repo's object database; it only
// mmaps the existing on-disk .idx file(s) (a few MB) and parses them via
// both loader paths, exactly the same read-only operation the production
// scanner performs when it opens a worktree of this size of repo.
//
// This test intentionally does NOT touch the multi-GB .pack file itself —
// only the small, already-materialized .idx sidecar — so it stays fast
// (single-digit milliseconds to parse tens of thousands of index entries)
// despite the underlying repository being large.
package gogitstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/cache"
)

// realRepoRootForTest resolves the absolute path of the repository this test
// file itself lives in, by walking up from runtime.Caller(0)'s own file path
// (session/unfinished/gogitstore/mmap_realrepo_test.go is always exactly
// three directories below the repo root in this repository's own layout).
// This is deliberately NOT hardcoded to a specific machine-local absolute
// path — it works in any checkout of this repository, on any machine, as
// long as the source tree's relative layout is preserved (which `go test`
// always requires anyway, since it compiles from the checked-out source).
func realRepoRootForTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("could not determine this test file's path via runtime.Caller")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
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

// minRealisticIdxBytes is the threshold below which this test considers a
// checkout's .git too small/shallow to be a meaningful "real large repo"
// measurement (e.g. a shallow CI clone) and skips rather than reporting a
// number that would misrepresent the production case. This repository's own
// commondir's actual pack-*.idx is ~1.6MB (~57k objects) as of the
// measurement this test's own log captures — comfortably above this floor.
const minRealisticIdxBytes = 250 * 1024

// TestMmapIndex_HeapAllocation_RealRepo is gogitstore's real-repo companion
// to TestMmapIndex_HeapAllocation_LowerThanCopyBased (synthetic fixture).
// It measures runtime.MemStats.TotalAlloc deltas for SharedObjectStore.
// ensureIndex() under both loader modes against THIS repository's own,
// already-on-disk .idx file — read-only; nothing in the real repo's object
// database is written, copied, or modified. See package doc above for why
// this only touches the small .idx sidecar, not the multi-GB .pack file.
//
// Skips (rather than fails) when run against a checkout that doesn't have a
// large enough real pack to measure meaningfully (e.g. a shallow CI clone) —
// this is an environmental measurement, not a correctness proof, and a
// shallow checkout genuinely cannot answer the question this test asks.
func TestMmapIndex_HeapAllocation_RealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}

	repoRoot := realRepoRootForTest(t)
	gitDir := filepath.Join(repoRoot, ".git")
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		t.Skipf("no .git directory at resolved repo root %s — not running from a full checkout", repoRoot)
	}

	idxPath, idxSize := largestIdxFile(gitDir)
	if idxPath == "" {
		t.Skip("no packed .idx files found under .git/objects/pack — repo has no packs to measure (fully loose, or a bare fixture checkout)")
	}
	if idxSize < minRealisticIdxBytes {
		t.Skipf("largest .idx (%s, %d bytes) is below the %d-byte floor for a meaningful real-repo measurement — likely a shallow/small checkout", idxPath, idxSize, minRealisticIdxBytes)
	}

	_, commonFs, _, commonDirAbs, err := resolveGitFilesystems(repoRoot)
	if err != nil {
		t.Fatalf("resolveGitFilesystems(%s): %v", repoRoot, err)
	}

	measure := func(useMmap bool) uint64 {
		store := newSharedObjectStore(commonDirAbs, commonFs, cache.NewObjectLRU(cache.FileSize(1<<20)), 0, useMmap)
		// ensureIndex below starts a pack-watch goroutine (mmapwatch.go) when
		// useMmap is true; nothing else in this measurement helper would
		// otherwise ever stop it — see this task's report / the matching
		// cleanup comment in mmap_stage2_test.go's
		// TestRegistry_UseMmapIndex_True_EngagesMmapLoader for the goroutine
		// leak this exact omission caused when repeatedly stress-run.
		t.Cleanup(func() { store.stopPackWatch() })
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		if err := store.ensureIndex(); err != nil {
			t.Fatalf("ensureIndex(useMmap=%v): %v", useMmap, err)
		}
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(store)
		if after.TotalAlloc < before.TotalAlloc {
			return 0
		}
		return after.TotalAlloc - before.TotalAlloc
	}

	copyDelta := measure(false)
	mmapDelta := measure(true)

	t.Logf("real repo: %s", repoRoot)
	t.Logf("real repo .idx: %s (%d bytes on disk)", idxPath, idxSize)
	copyKB := float64(copyDelta) / 1024
	mmapKB := float64(mmapDelta) / 1024
	t.Logf("copy-based ensureIndex TotalAlloc delta: %d bytes (%.1f KiB)", copyDelta, copyKB)
	t.Logf("mmap-based ensureIndex TotalAlloc delta:  %d bytes (%.1f KiB)", mmapDelta, mmapKB)
	if copyDelta > 0 {
		reduction := 100 * (1 - float64(mmapDelta)/float64(copyDelta))
		t.Logf("mmap loader allocates %.1f%% less heap than the copy-based loader on this real repo's .idx", reduction)
	}

	if copyDelta == 0 {
		t.Fatal("copy-based loader allocated 0 bytes — measurement is broken")
	}
	if mmapDelta >= copyDelta {
		t.Errorf("mmap loader allocated %d bytes, want meaningfully less than copy-based loader's %d bytes", mmapDelta, copyDelta)
	}
}
