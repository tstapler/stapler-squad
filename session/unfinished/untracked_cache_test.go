package unfinished

// Story 4.2.3 tests: the per-directory untracked verdict cache. The fixture
// drives hasUntrackedSkip/walkUntrackedSkip directly over a plain directory
// tree with a synthetic tracked set, so every invalidator is isolated. Every
// test asserts engagement (DirsSkipped > 0) with timestamps backdated through
// testrepo.Age and an injected clock, so the 2 s racy guard cannot make a pass
// vacuous.

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// liveMatcher keeps one stable identity while its rules follow the files on
// disk. Production rebuilds the real matcher rarely; modelling a matcher that
// is always current with a stable identity isolates the cache's own key
// components (directory signature, .gitignore chain, excludes, tracked set).
type liveMatcher struct{ inner gitignore.Matcher }

func (m *liveMatcher) Match(path []string, isDir bool) bool { return m.inner.Match(path, isDir) }

type walkFixture struct {
	t       testing.TB
	root    string
	g       *GoGitVCSReader
	matcher *liveMatcher
	indexed map[string]struct{}
	tg      trackedGen
	offset  time.Duration
}

func newWalkFixture(t testing.TB) *walkFixture {
	t.Helper()
	f := &walkFixture{t: t, root: testrepo.TempDir(t), indexed: map[string]struct{}{}, matcher: &liveMatcher{}, offset: time.Minute}
	f.g = &GoGitVCSReader{}
	f.g.scanSkip.flags = &Flags{ScanSkip: true}
	f.g.scanSkip.clock = func() time.Time { return time.Now().Add(f.offset) }
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, ".git", "info"), 0o755))
	f.setIndex(1)
	f.refreshMatcher()
	return f
}

// setIndex models an index rewrite: a new signature (and the caller's tracked set).
func (f *walkFixture) setIndex(gen int64) {
	f.tg = trackedGen{sig: statSig{mtimeNs: gen, ctimeNs: gen, size: gen, ino: 1}, observedAt: time.Now().Add(f.offset), ok: true}
}

// refreshMatcher rebuilds the rules from the .gitignore files on disk. It does
// not use gitignore.ReadPatterns: that function appends to a shared slice while
// recursing, so sibling directories can overwrite each other's pattern domains
// and one .gitignore edit can change classification in unrelated directories
// (production is immune to the resulting non-locality because a rebuilt
// matcher has a new identity, which resets the whole cache). The model needs
// the locality git itself has, which is what the ancestor markers rely on.
func (f *walkFixture) refreshMatcher() {
	var patterns []gitignore.Pattern
	var read func(dir string, parts []string)
	read = func(dir string, parts []string) {
		if data, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					patterns = append(patterns, gitignore.ParsePattern(line, slices.Clone(parts)))
				}
			}
		}
		entries, err := os.ReadDir(dir)
		require.NoError(f.t, err)
		for _, e := range entries {
			if e.IsDir() && e.Name() != ".git" {
				read(filepath.Join(dir, e.Name()), append(slices.Clone(parts), e.Name()))
			}
		}
	}
	read(f.root, nil)
	f.matcher.inner = gitignore.NewMatcher(patterns)
}

func (f *walkFixture) path(rel string) string { return filepath.Join(f.root, filepath.FromSlash(rel)) }

func (f *walkFixture) write(rel, content string) {
	f.t.Helper()
	require.NoError(f.t, os.MkdirAll(filepath.Dir(f.path(rel)), 0o755))
	require.NoError(f.t, os.WriteFile(f.path(rel), []byte(content), 0o644))
}

func (f *walkFixture) track(rels ...string) {
	for _, r := range rels {
		f.indexed[r] = struct{}{}
	}
}

// ageAll backdates every directory and file mtime an hour (GA2).
func (f *walkFixture) ageAll() {
	f.t.Helper()
	require.NoError(f.t, filepath.WalkDir(f.root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink == 0 {
			testrepo.Age(f.t, p, time.Hour)
		}
		return nil
	}))
}

func (f *walkFixture) skipFiles() []string {
	f.t.Helper()
	var got []string
	require.NoError(f.t, f.g.walkUntrackedSkip(f.root, f.indexed, f.matcher, f.tg, func(abs string) { got = append(got, f.rel(abs)) }))
	sort.Strings(got)
	return got
}

func (f *walkFixture) fullFiles() []string {
	f.t.Helper()
	var got []string
	require.NoError(f.t, walkUntracked(f.root, f.indexed, f.matcher, func(abs string) { got = append(got, f.rel(abs)) }))
	sort.Strings(got)
	return got
}

func (f *walkFixture) rel(abs string) string {
	r, err := filepath.Rel(f.root, abs)
	require.NoError(f.t, err)
	return filepath.ToSlash(r)
}

// scanEqual runs the skip walk and the full walk and requires equal answers.
func (f *walkFixture) scanEqual() []string {
	f.t.Helper()
	skip, full := f.skipFiles(), f.fullFiles()
	require.Equal(f.t, full, skip, "scan_skip walk must equal the full walk")
	has, err := f.g.hasUntrackedSkip(f.root, f.indexed, f.matcher, f.tg, false)
	require.NoError(f.t, err)
	require.Equal(f.t, len(full) > 0, has)
	return skip
}

func (f *walkFixture) stats() ScanSkipStats { return f.g.ScanSkipStats() }

// threeLevel builds a/b/c plus a sibling, all tracked, backdated and warmed.
func threeLevel(t testing.TB) *walkFixture {
	f := newWalkFixture(t)
	for _, rel := range []string{"a/one.go", "a/b/two.go", "a/b/c/three.go", "z/four.go", "root.go"} {
		f.write(rel, "x\n")
		f.track(rel)
	}
	f.ageAll()
	require.Empty(t, f.scanEqual(), "cold scan")
	require.Empty(t, f.scanEqual(), "warm scan")
	require.Greater(t, f.stats().DirsSkipped, int64(0), "engagement: the warm scan must be served from the cache")
	return f
}

func TestUntrackedDirCache_ShouldRewalkDir_WhenNewFileCreatedAndMtimeChanges(t *testing.T) {
	t.Parallel()
	f := threeLevel(t)
	before := f.stats()

	f.write("a/b/new.txt", "n\n")

	assert.Equal(t, []string{"a/b/new.txt"}, f.scanEqual())
	after := f.stats()
	assert.Equal(t, before.DirsWalked+1, after.DirsWalked, "only the changed directory is re-read; the has query that follows hits the refreshed entry")
}

func TestUntrackedDirCache_ShouldRewalkOnlyLeaf_WhenOnlyNestedDirChanges(t *testing.T) {
	t.Parallel()
	f := threeLevel(t)
	before := f.stats()

	f.write("a/b/c/leaf.txt", "n\n")
	got := f.skipFiles() // one cached walk

	assert.Equal(t, []string{"a/b/c/leaf.txt"}, got)
	after := f.stats()
	assert.EqualValues(t, 1, after.DirsWalked-before.DirsWalked, "only c/ is re-read")
	assert.GreaterOrEqual(t, after.DirsSkipped-before.DirsSkipped, int64(3), "root, a/, a/b/ and z/ cost one stat each")
}

func TestUntrackedDirCache_ShouldReportUntracked_WhenIntermediateGitignoreAddedWithNegation(t *testing.T) {
	t.Parallel()
	f := newWalkFixture(t)
	f.write(".gitignore", "*.log\n")
	f.write("a/b/c/keep.log", "k\n")
	f.write("a/b/c/code.go", "c\n")
	f.track(".gitignore", "a/b/c/code.go")
	f.ageAll()
	f.refreshMatcher()
	require.Empty(t, f.scanEqual())
	require.Empty(t, f.scanEqual())
	require.Greater(t, f.stats().DirsSkipped, int64(0))

	// New .gitignore in the intermediate directory a/: a/b/c is untouched, yet a
	// negation now exposes keep.log below it.
	f.write("a/.gitignore", "!keep.log\n")
	f.track("a/.gitignore")
	f.ageAll()
	f.refreshMatcher()
	assert.Equal(t, []string{"a/b/c/keep.log"}, f.scanEqual(), "ancestor marker changed: a/'s whole subtree is re-walked")

	require.NoError(t, os.Remove(f.path("a/.gitignore")))
	f.refreshMatcher()
	assert.Empty(t, f.scanEqual(), "deleting it flips back")
}

func TestUntrackedDirCache_ShouldReportUntracked_WhenGitignoreEditedInPlaceToUnignore(t *testing.T) {
	t.Parallel()
	f := newWalkFixture(t)
	f.write(".gitignore", "build/\n")
	f.write("build/out.o", "o\n")
	f.write("main.go", "m\n")
	f.track(".gitignore", "main.go")
	f.ageAll()
	f.refreshMatcher()
	require.Empty(t, f.scanEqual())
	require.Empty(t, f.scanEqual())
	require.Greater(t, f.stats().DirsSkipped, int64(0))

	// In place: no directory entry changes, so no directory signature moves.
	f.write(".gitignore", "vendr/\n")
	f.ageAll()
	f.refreshMatcher()

	assert.Equal(t, []string{"build/out.o"}, f.scanEqual())
}

func TestUntrackedDirCache_ShouldReportUntracked_WhenInfoExcludeOrGlobalExcludesEdited(t *testing.T) {
	// Not parallel: redirects the global excludes location through the environment.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	f := newWalkFixture(t)
	f.write("a/one.go", "1\n")
	f.write("b/two.go", "2\n")
	f.track("a/one.go", "b/two.go")
	f.ageAll()
	require.Empty(t, f.scanEqual())
	require.Empty(t, f.scanEqual())
	require.Greater(t, f.stats().DirsSkipped, int64(0))

	edit := func(path, body string) int64 {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		testrepo.Age(t, path, time.Hour)
		before := f.stats().DirsWalked
		assert.Empty(t, f.scanEqual(), "verdict still equals a full walk")
		return f.stats().DirsWalked - before
	}
	// The current matcher ignores these files, so the verdict cannot move; the
	// fingerprint must still drop every cached directory.
	assert.Greater(t, edit(f.path(".git/info/exclude"), "*.tmp\n"), int64(0), "info/exclude edit re-walks")
	global, err := os.UserConfigDir()
	require.NoError(t, err)
	assert.Greater(t, edit(filepath.Join(global, "git", "ignore"), "*.bak\n"), int64(0), "global excludes edit re-walks")
}

func TestUntrackedDirCache_ShouldReportUntracked_WhenIndexEntryRemovedKeepingFile(t *testing.T) {
	t.Parallel()
	f := threeLevel(t)

	// go-git storer equivalent of `git rm --cached`: the index is rewritten
	// without the entry, the file stays on disk.
	delete(f.indexed, "a/b/two.go")
	f.setIndex(2)

	assert.Equal(t, []string{"a/b/two.go"}, f.scanEqual())
}

func TestUntrackedDirCache_ShouldRewalk_WhenCheckoutChangesTrackedSet(t *testing.T) {
	t.Parallel()
	f := threeLevel(t)
	before := f.stats()

	f.indexed = map[string]struct{}{"root.go": {}} // a checkout replaced the tracked set
	f.setIndex(2)
	got := f.scanEqual()

	assert.Len(t, got, 4)
	assert.GreaterOrEqual(t, f.stats().DirsWalked-before.DirsWalked, int64(5), "every directory re-listed")
}

func TestUntrackedDirCache_ShouldRewalk_WhenDirRemovedRenamedOrAged5Min(t *testing.T) {
	t.Parallel()
	f := threeLevel(t)

	require.NoError(t, os.Rename(f.path("a/b"), f.path("a/moved")))
	assert.ElementsMatch(t, []string{"a/moved/two.go", "a/moved/c/three.go"}, f.scanEqual(), "renamed directory is a new path")

	require.NoError(t, os.RemoveAll(f.path("z")))
	f.track("a/moved/two.go", "a/moved/c/three.go")
	assert.Empty(t, f.scanEqual(), "removed directory vanishes")
	require.Empty(t, f.scanEqual())

	before := f.stats()
	f.offset += scanSkipMaxAge
	require.Empty(t, f.scanEqual())
	assert.GreaterOrEqual(t, f.stats().DirsWalked-before.DirsWalked, int64(4), "5 minutes: every verdict is stale, full walk")
}

func TestUntrackedDirCache_ShouldRewalk_WhenDirMtimeWithin2sOfStore(t *testing.T) {
	t.Parallel()
	f := newWalkFixture(t)
	f.write("a/one.go", "1\n")
	f.track("a/one.go")
	dsig, ok := statPathSig(f.path("a"))
	require.True(t, ok)
	newest := max(dsig.newest(), mustSig(t, f.path(".")).newest())
	f.g.scanSkip.clock = func() time.Time { return time.Unix(0, newest).Add(500 * time.Millisecond) }
	f.tg.observedAt = time.Unix(0, newest).Add(time.Hour) // index signature trusted; only the dirs are racy

	f.skipFiles()
	f.skipFiles()

	s := f.stats()
	assert.Zero(t, s.DirsSkipped, "racy directories are never served from the cache")
	assert.Greater(t, s.DirsRacyRefused, int64(0))

	f.g.scanSkip.clock = func() time.Time { return time.Unix(0, newest).Add(10 * time.Second) }
	f.skipFiles()
	f.skipFiles()
	assert.Greater(t, f.stats().DirsSkipped, int64(0), "control: the same directories are trusted 10 s later")
}

func mustSig(t testing.TB, p string) statSig {
	t.Helper()
	sig, ok := statPathSig(p)
	require.True(t, ok)
	return sig
}

func TestUntrackedWalkSkip_ShouldPreserveWalkOrder_WhenDirsAndFilesInterleave(t *testing.T) {
	t.Parallel()
	f := newWalkFixture(t)
	for _, rel := range []string{"a.txt", "b/x.txt", "c.txt", "d/y.txt", "e.txt"} {
		f.write(rel, "x\n")
	}
	f.ageAll()
	var cold, warm []string
	require.NoError(t, f.g.walkUntrackedSkip(f.root, f.indexed, f.matcher, f.tg, func(p string) { cold = append(cold, f.rel(p)) }))
	require.NoError(t, f.g.walkUntrackedSkip(f.root, f.indexed, f.matcher, f.tg, func(p string) { warm = append(warm, f.rel(p)) }))

	assert.Equal(t, cold, warm, "same order from the cache as from disk (the file cap depends on it)")
	assert.Greater(t, f.stats().DirsSkipped, int64(0))
}

// Parity with the retained pre-optimisation walkers (walkers_oracle_test.go)
// on the shared walker fixture, scan_skip on (cold and warm) against the old
// recursive implementation. Not parallel: it lowers maxUntrackedFiles.
func TestUntrackedWalkSkip_ShouldMatchOldImplementation_WhenColdAndWarm(t *testing.T) {
	root, files := newWalkerFixture(t)
	matcher := buildTestMatcher(t, root)
	g := &GoGitVCSReader{}
	g.scanSkip.flags = &Flags{ScanSkip: true}
	g.scanSkip.clock = func() time.Time { return time.Now().Add(time.Minute) }
	tg := trackedGen{sig: statSig{mtimeNs: 1, ctimeNs: 1, size: 1, ino: 1}, observedAt: time.Now().Add(time.Minute), ok: true}

	collect := func(walk func(func(string)) error) []string {
		var out []string
		require.NoError(t, walk(func(abs string) {
			rel, _ := filepath.Rel(root, abs)
			out = append(out, filepath.ToSlash(rel))
		}))
		sort.Strings(out)
		return out
	}
	scenarios := map[string]map[string]struct{}{
		"everything tracked":      relNames(files, func(string) bool { return false }),
		"one deep file untracked": relNames(files, func(f string) bool { return f == "a/b/c/two.go" }),
		"nothing tracked":         {},
	}
	for name, indexed := range scenarios {
		for _, m := range []struct {
			label string
			m     gitignore.Matcher
		}{{"matcher", matcher}, {"nil matcher", nil}} {
			want := collect(func(fn func(string)) error { return oldWalkUntracked(root, indexed, m.m, fn) })
			for _, pass := range []string{"cold", "warm"} {
				got := collect(func(fn func(string)) error { return g.walkUntrackedSkip(root, indexed, m.m, tg, fn) })
				assert.Equal(t, want, got, "%s/%s/%s", name, m.label, pass)
			}
			wantHas, _ := oldHasUntrackedFilesRec(root, root, indexed, m.m)
			gotHas, err := g.hasUntrackedSkip(root, indexed, m.m, tg, false)
			require.NoError(t, err)
			assert.Equal(t, wantHas, gotHas, "%s/%s hasUntracked", name, m.label)
		}
	}
	assert.Greater(t, g.ScanSkipStats().DirsSkipped, int64(0), "engagement")

	orig := maxUntrackedFiles
	maxUntrackedFiles = 3
	defer func() { maxUntrackedFiles = orig }()
	var got, want int
	require.NoError(t, g.walkUntrackedSkip(root, map[string]struct{}{}, nil, tg, func(string) { got++ }))
	require.NoError(t, oldWalkUntracked(root, map[string]struct{}{}, nil, func(string) { want++ }))
	assert.Equal(t, want, got, "cap: both report the same number of files")
	assert.Equal(t, 3, got)
}
