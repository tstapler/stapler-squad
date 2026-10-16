//go:build !race

package unfinished

import (
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
