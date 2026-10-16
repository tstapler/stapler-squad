//go:build !race

package unfinished

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gateRuns = 5

// scanAllocs is the mean allocations of one uncached HasUncommitted.
func scanAllocs(t testing.TB, g *GoGitVCSReader, dir string) float64 {
	t.Helper()
	require.False(t, rescan(t, g, dir)) // warm: open the repo, build the matcher, fill caches
	return testing.AllocsPerRun(gateRuns, func() {
		g.InvalidateWorktree(dir)
		_, _ = g.HasUncommitted(dir)
	})
}

// G8: with the index signature unchanged the clean scan allocates at most 60%
// of the legacy path. The index is backdated (GA2) so the racy guard lets the
// cache engage; the engagement counter proves it did.
func TestHasUncommittedClean_ShouldAllocAtMost60PctOfLegacy_WhenIndexSigUnchanged(t *testing.T) {
	_, dir := trackedRepo(t, manyFiles(20000))
	legacy := scanAllocs(t, newLegacyReader(), dir)
	g := newSkipReader()
	skip := scanAllocs(t, g, dir)

	require.Greater(t, g.ScanSkipStats().IndexHits, int64(0), "engagement: gate would pass vacuously otherwise")
	assert.LessOrEqual(t, skip, 0.60*legacy, "legacy=%.0f skip=%.0f allocs/op", legacy, skip)
	t.Logf("20,000 tracked files, clean: legacy=%.0f allocs/op, scan_skip=%.0f allocs/op (%.1f%%)", legacy, skip, 100*skip/legacy)
}

// Error path: when the racy guard denies the cached path, G8 must not hold.
func TestHasUncommittedClean_ShouldReparse_WhenIndexMtimeWithin2sOfParse(t *testing.T) {
	_, dir := trackedRepo(t, manyFiles(5000))
	sig, ok := statPathSig(filepath.Join(dir, ".git", "index"))
	require.True(t, ok)
	legacy := scanAllocs(t, newLegacyReader(), dir)
	g := newSkipReader()
	g.scanSkip.clock = func() time.Time { return time.Unix(0, sig.newest()).Add(500 * time.Millisecond) }
	skip := scanAllocs(t, g, dir)

	assert.Zero(t, g.ScanSkipStats().IndexHits)
	assert.Greater(t, skip, 0.60*legacy, "a racy index is re-parsed, so the saving must vanish (legacy=%.0f skip=%.0f)", legacy, skip)
}

// GA7, the one deterministic "scales with changes, not files" shape gate. The
// per-file lstat pass stays O(files) by the ADR-006 hard rule and costs exactly
// one heap allocation per call (x/sys/unix.Fstatat copies the name into a
// NUL-terminated buffer), so total allocs/op cannot be sublinear; the gate
// bounds everything else: the allocations beyond one per tracked file must not
// grow with the file count once the index and directory caches hit.
func TestHasUncommittedAllocs_ShouldGrowSublinearlyInFiles_WhenIndexCacheHits(t *testing.T) {
	residual := func(n int) float64 {
		_, dir := trackedRepo(t, manyFiles(n))
		g := newSkipReader()
		allocs := scanAllocs(t, g, dir)
		s := g.ScanSkipStats()
		require.Greater(t, s.IndexHits, int64(0), "engagement: index cache (n=%d)", n)
		require.Greater(t, s.DirsSkipped, int64(0), "engagement: untracked directory cache (n=%d)", n)
		return allocs - float64(n)
	}
	small, large := residual(5000), residual(20000)
	t.Logf("allocs beyond one lstat per file: 5,000 files=%.0f, 20,000 files=%.0f (ratio %.2f)", small, large, large/small)
	assert.LessOrEqual(t, large/small, 1.5)
}

// The untracked walk is 34.1% of diffShortstatUncached CPU in the S1 profile
// (baseline.md). Warm, the verdict cache replaces per-directory ReadDir and
// per-entry matcher/lookup work with one stat per directory.
func TestUntrackedWalkSkip_ShouldAllocAtMost25PctOfLegacy_WhenWarm(t *testing.T) {
	f := newWalkFixture(t)
	f.write(".gitignore", "*.log\n")
	f.track(".gitignore")
	for i := range 20000 {
		rel := fmt.Sprintf("d%02d/s%d/f%05d.go", i%50, i%3, i)
		f.write(rel, "x\n")
		f.track(rel)
	}
	f.write("d07/untracked.txt", "u\n")
	f.refreshMatcher()
	noop := func(string) {}
	require.NoError(t, f.g.walkUntrackedSkip(f.root, f.indexed, f.matcher, f.tg, noop)) // cold fill

	legacy := testing.AllocsPerRun(5, func() { _ = walkUntracked(f.root, f.indexed, f.matcher, noop) })
	skip := testing.AllocsPerRun(5, func() { _ = f.g.walkUntrackedSkip(f.root, f.indexed, f.matcher, f.tg, noop) })

	require.Greater(t, f.stats().DirsSkipped, int64(0), "engagement")
	t.Logf("untracked walk, 20,000 files / 200 dirs: legacy=%.0f allocs/op, cached=%.0f allocs/op (%.1f%%)", legacy, skip, 100*skip/legacy)
	assert.LessOrEqual(t, skip, 0.25*legacy)
}
