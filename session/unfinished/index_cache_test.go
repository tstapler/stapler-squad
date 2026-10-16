package unfinished

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	git "github.com/go-git/go-git/v5"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// newSkipReader returns a reader with scan_skip on whose clock runs a minute
// ahead of the file system, so index/dir timestamps backdated with
// testrepo.Age clear the 2 s racy guard without sleeping.
func newSkipReader() *GoGitVCSReader {
	g := &GoGitVCSReader{}
	g.scanSkip.flags = &Flags{ScanSkip: true}
	g.scanSkip.clock = func() time.Time { return time.Now().Add(time.Minute) }
	return g
}

// newLegacyReader is the retained oracle: scan_skip off.
func newLegacyReader() *GoGitVCSReader { return &GoGitVCSReader{} }

// trackedRepo commits files (path -> content) with one index rewrite and
// backdates the index (GA2).
func trackedRepo(t testing.TB, files map[string]string) (*git.Repository, string) {
	t.Helper()
	repo, dir := testrepo.NewRepo(t)
	if len(files) > 1000 {
		testrepo.CommitManyFast(t, repo, files, "init", testrepo.Epoch)
	} else {
		testrepo.CommitAll(t, repo, files, "init", testrepo.Epoch)
	}
	ageIndex(t, dir)
	return repo, dir
}

func ageIndex(t testing.TB, dir string) {
	t.Helper()
	testrepo.Age(t, filepath.Join(dir, ".git", "index"), time.Hour)
}

// rescan drops the 30 s verdict cache so the next call does real work, then
// asks HasUncommitted.
func rescan(t testing.TB, g *GoGitVCSReader, dir string) bool {
	t.Helper()
	g.InvalidateWorktree(dir)
	dirty, err := g.HasUncommitted(dir)
	require.NoError(t, err)
	return dirty
}

func manyFiles(n int) map[string]string {
	files := make(map[string]string, n)
	for i := range n {
		files[fmt.Sprintf("d%02d/f%05d.txt", i%50, i)] = "x\n"
	}
	return files
}

func TestParsedIndexCache_ShouldSkipDecode_WhenIndexStatSigUnchanged(t *testing.T) {
	t.Parallel()
	_, dir := trackedRepo(t, map[string]string{"a.txt": "a\n", "d/b.txt": "b\n"})
	g := newSkipReader()

	require.False(t, rescan(t, g, dir))
	require.False(t, rescan(t, g, dir))
	require.False(t, rescan(t, g, dir))

	s := g.ScanSkipStats()
	assert.Equal(t, int64(1), s.IndexDecodes, "index.Decode runs once; later scans reuse the parse")
	assert.Greater(t, s.IndexHits, int64(0), "engagement: the cache must be exercised, not vacuously idle")
	assert.Equal(t, int64(2), s.IndexHits)
}

func TestParsedIndexCache_ShouldReparse_WhenIndexRewrittenWithin2sOfParse(t *testing.T) {
	t.Parallel()
	_, dir := trackedRepo(t, map[string]string{"a.txt": "a\n"})
	sig, ok := statPathSig(filepath.Join(dir, ".git", "index"))
	require.True(t, ok)
	g := newSkipReader()
	// Parse time 0.5 s after the index's newest timestamp: inside the window.
	g.scanSkip.clock = func() time.Time { return time.Unix(0, sig.newest()).Add(500 * time.Millisecond) }

	rescan(t, g, dir)
	rescan(t, g, dir)

	s := g.ScanSkipStats()
	assert.Equal(t, int64(2), s.IndexDecodes, "racy signature must be re-parsed every scan")
	assert.Zero(t, s.IndexHits)
	assert.Greater(t, s.IndexRacySkips, int64(0))

	// Control: the same index observed 10 s later is trusted.
	g2 := newSkipReader()
	g2.scanSkip.clock = func() time.Time { return time.Unix(0, sig.newest()).Add(10 * time.Second) }
	rescan(t, g2, dir)
	rescan(t, g2, dir)
	assert.Equal(t, int64(1), g2.ScanSkipStats().IndexHits)
}

func TestParsedIndexCache_ShouldReparse_WhenIndexContentChanges(t *testing.T) {
	t.Parallel()
	repo, dir := trackedRepo(t, map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	g := newSkipReader()
	require.False(t, rescan(t, g, dir))
	require.False(t, rescan(t, g, dir))

	testrepo.WriteFiles(t, dir, map[string]string{"a.txt": "changed\n"})
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("a.txt")
	require.NoError(t, err)
	ageIndex(t, dir)

	assert.True(t, rescan(t, g, dir), "staged change must be seen after the index is rewritten")
	assert.Equal(t, int64(2), g.ScanSkipStats().IndexDecodes)
}

func TestHasUncommitted_ShouldReportDirty_WhenTrackedFileEditedInPlaceAndIndexCacheHit(t *testing.T) {
	t.Parallel()
	_, dir := trackedRepo(t, map[string]string{"a.txt": "a\n", "d/b.txt": "b\n"})
	g := newSkipReader()
	require.False(t, rescan(t, g, dir))
	require.False(t, rescan(t, g, dir))
	hitsBefore := g.ScanSkipStats().IndexHits
	require.Greater(t, hitsBefore, int64(0))

	// No .git event or index change is possible for an in-place edit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("edited in place\n"), 0o644))

	assert.True(t, rescan(t, g, dir), "the cache removes the decode, never the stat pass")
	assert.Greater(t, g.ScanSkipStats().IndexHits, hitsBefore, "the dirty scan itself ran on a cache hit")
}

func TestHasUncommitted_ShouldNotReportDeleted_WhenSkipWorktreeEntryAbsent(t *testing.T) {
	t.Parallel()
	repo, dir := trackedRepo(t, map[string]string{"keep.txt": "k\n", "sparse.txt": "s\n"})
	testrepo.SkipWorktree(t, repo, "sparse.txt")
	require.NoError(t, os.Remove(filepath.Join(dir, "sparse.txt")))
	ageIndex(t, dir)

	assert.True(t, rescan(t, newLegacyReader(), dir), "legacy path: today's behaviour, reported deleted")
	assert.False(t, rescan(t, newSkipReader(), dir), "deliberate fix under scan_skip: an absent sparse entry is not a deletion")
}

func TestHasUncommitted_ScanSkipShouldMatchLegacy_WhenWorktreeMutated(t *testing.T) {
	t.Parallel()
	base := map[string]string{"a.txt": "a\n", "d/b.txt": "b\n", ".gitignore": "ignored/\n", "keep.txt": "k\n"}
	cases := []struct {
		name   string
		mutate func(t *testing.T, repo *git.Repository, dir string)
		want   bool
	}{
		{"clean", func(*testing.T, *git.Repository, string) {}, false},
		{"edit tracked", func(t *testing.T, _ *git.Repository, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("edit\n"), 0o644))
		}, true},
		{"delete tracked", func(t *testing.T, _ *git.Repository, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, "d", "b.txt")))
		}, true},
		{"untracked file", func(t *testing.T, _ *git.Repository, dir string) {
			testrepo.WriteFiles(t, dir, map[string]string{"d/new.txt": "n\n"})
		}, true},
		{"ignored file only", func(t *testing.T, _ *git.Repository, dir string) {
			testrepo.WriteFiles(t, dir, map[string]string{"ignored/x.o": "n\n"})
		}, false},
		{"staged add", func(t *testing.T, repo *git.Repository, dir string) {
			testrepo.WriteFiles(t, dir, map[string]string{"s.txt": "s\n"})
			wt, err := repo.Worktree()
			require.NoError(t, err)
			_, err = wt.Add("s.txt")
			require.NoError(t, err)
		}, true},
		{"staged delete", func(t *testing.T, repo *git.Repository, dir string) {
			wt, err := repo.Worktree()
			require.NoError(t, err)
			_, err = wt.Remove("keep.txt")
			require.NoError(t, err)
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			repo, dir := trackedRepo(t, base)
			c.mutate(t, repo, dir)
			ageIndex(t, dir)

			legacy := rescan(t, newLegacyReader(), dir)
			g := newSkipReader()
			first := rescan(t, g, dir)
			second := rescan(t, g, dir)

			assert.Equal(t, c.want, legacy, "fixture expectation (legacy oracle)")
			assert.Equal(t, legacy, first, "cold scan_skip verdict")
			assert.Equal(t, legacy, second, "cache-hit scan_skip verdict")
			assert.Greater(t, g.ScanSkipStats().IndexHits, int64(0), "engagement: second scan must hit the index cache")
		})
	}
}
