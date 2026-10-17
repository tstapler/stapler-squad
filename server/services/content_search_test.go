package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// writeTree writes files (slash paths) under a fresh temp dir and returns the dir and a
// confined fs.FS over it, the same shape the production handler hands the searcher.
func writeTree(t testing.TB, files map[string]string) (string, fs.FS) {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return dir, openRootFS(t, dir)
}

func openRootFS(t testing.TB, dir string) fs.FS {
	t.Helper()
	r, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	return r.FS()
}

func literalOpts(pattern string) SearchOptions {
	return SearchOptions{Pattern: pattern, Match: MatchLiteral, Case: CaseSensitive, Word: WordAny}
}

func runSearch(t testing.TB, root fs.FS, opts SearchOptions) ([]SearchMatch, SearchStats) {
	t.Helper()
	return runSearcher(t, NewWalkSearcher(), root, opts)
}

func runSearcher(t testing.TB, s ContentSearcher, root fs.FS, opts SearchOptions) ([]SearchMatch, SearchStats) {
	t.Helper()
	var all []SearchMatch
	stats, err := s.Search(context.Background(), root, opts, func(m []SearchMatch) error {
		all = append(all, m...)
		return nil
	})
	require.NoError(t, err)
	return all, stats
}

func matchedPaths(ms []SearchMatch) []string {
	seen := map[string]bool{}
	for _, m := range ms {
		seen[m.Path] = true
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// utf16Slice returns text[start:end] addressed in UTF-16 code units.
func utf16Slice(text string, start, end int) string {
	u := utf16.Encode([]rune(text))
	return string(utf16.Decode(u[start:end]))
}

func TestContentSearch_ShouldEmitUTF16Ranges_WhenLiteralCaseInsensitiveWithEmoji(t *testing.T) {
	line := `x := "HÉLLO" // 😀 héllo`
	_, root := writeTree(t, map[string]string{"a.go": "package a\n\n" + line + "\n"})
	opts := literalOpts("héllo")
	opts.Case = CaseInsensitive

	got, _ := runSearch(t, root, opts)

	require.Len(t, got, 2)
	assert.Equal(t, 3, got[0].Line)
	assert.Equal(t, line, got[0].Text)
	assert.Equal(t, "HÉLLO", utf16Slice(got[0].Text, got[0].Start, got[0].End))
	assert.Equal(t, "héllo", utf16Slice(got[1].Text, got[1].Start, got[1].End))
	// `x := "HÉLLO" // ` is 16 units, the emoji adds 2 (not 1), then a space.
	assert.Equal(t, 19, got[1].Start, "second range must account for the 2-unit emoji")
	assert.Equal(t, 24, got[1].End)
}

func TestNewSearchOptions_ShouldReturnInvalidArgument_WhenRegexInvalidOrPatternTooShortOrTooLong(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		mode    sessionv1.SearchMatchMode
		wantMsg string
	}{
		{"unbalanced paren", "(", sessionv1.SearchMatchMode_SEARCH_MATCH_MODE_REGEX, "missing closing )"},
		{"one char", "a", sessionv1.SearchMatchMode_SEARCH_MATCH_MODE_LITERAL, "at least 2"},
		{"empty", "", sessionv1.SearchMatchMode_SEARCH_MATCH_MODE_LITERAL, "at least 2"},
		{"over 1 KB", strings.Repeat("x", 1025), sessionv1.SearchMatchMode_SEARCH_MATCH_MODE_LITERAL, "at most 1024"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewSearchOptions(&sessionv1.SearchOptions{Pattern: tc.pattern, MatchMode: tc.mode})
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}

	t.Run("parens are literal in literal mode", func(t *testing.T) {
		_, err := NewSearchOptions(&sessionv1.SearchOptions{Pattern: "f(x", MatchMode: sessionv1.SearchMatchMode_SEARCH_MATCH_MODE_LITERAL})
		assert.NoError(t, err)
	})
}

func TestNewSearchOptions_ShouldReturnInvalidArgument_When_IncludeOrExcludeGlobMalformed(t *testing.T) {
	bad := func(scope *sessionv1.SearchScope) error {
		_, err := NewSearchOptions(&sessionv1.SearchOptions{Pattern: "needle", Scope: scope})
		return err
	}
	for _, g := range []string{"[a", "{"} {
		err := bad(&sessionv1.SearchScope{IncludeGlobs: []string{g}})
		require.Error(t, err, g)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		err = bad(&sessionv1.SearchScope{ExcludeGlobs: []string{g}})
		require.Error(t, err, g)
	}
	require.NoError(t, bad(&sessionv1.SearchScope{IncludeGlobs: []string{"**/*.go"}, ExcludeGlobs: []string{"vendor/**"}}))

	_, root := writeTree(t, map[string]string{"a/b.go": "needle\n", "a/b.txt": "needle\n", "vendor/x.go": "needle\n"})
	opts := literalOpts("needle")
	opts.Scope = SearchScope{IncludeGlobs: []string{"**/*.go"}, ExcludeGlobs: []string{"vendor/**"}, IncludeIgnored: true}
	got, _ := runSearch(t, root, opts)
	assert.Equal(t, []string{"a/b.go"}, matchedPaths(got))
}

func TestContentSearch_ShouldCapResultsAndClampLines_WhenFiveThousandMatches(t *testing.T) {
	files := map[string]string{}
	long := strings.Repeat("é", 300) + " needle " + strings.Repeat("ü", 400) // 1.6 KB line, match past byte 512
	for i := 0; i < 200; i++ {
		var b strings.Builder
		for l := 0; l < 25; l++ {
			if l == 0 {
				b.WriteString(long + "\n")
				continue
			}
			fmt.Fprintf(&b, "line %d needle\n", l)
		}
		files[fmt.Sprintf("d%02d/f%03d.txt", i%20, i)] = b.String()
	}
	_, root := writeTree(t, files)
	opts := literalOpts("needle")
	opts.Limits.MaxResults = 1000

	got, stats := runSearch(t, root, opts)

	require.Len(t, got, 1000)
	assert.True(t, stats.Truncated)
	perFile := map[string]int{}
	for _, m := range got {
		perFile[m.Path]++
		assert.LessOrEqual(t, len(m.Text), searchLineClamp)
		assert.Equal(t, "needle", utf16Slice(m.Text, m.Start, m.End), "range must stay inside the clamped line")
	}
	for p, n := range perFile {
		assert.LessOrEqual(t, n, searchMaxPerFile, p)
	}
}

func TestContentSearch_ShouldNotTruncate_WhenResultsFitUnderCap(t *testing.T) {
	_, root := writeTree(t, map[string]string{"a.txt": "needle\n", "b.txt": "needle\n"})
	got, stats := runSearch(t, root, literalOpts("needle"))
	assert.Len(t, got, 2)
	assert.False(t, stats.Truncated)
}

func TestContentSearch_ShouldSupportRegexAndWholeWord(t *testing.T) {
	_, root := writeTree(t, map[string]string{"a.txt": "foo foobar foo_x foo\nbar42 bar7\n"})
	opts := literalOpts("foo")
	opts.Word = WordWhole
	got, _ := runSearch(t, root, opts)
	assert.Len(t, got, 2, "only the standalone foo tokens")

	opts = SearchOptions{Pattern: `bar\d+`, Match: MatchRegex, Case: CaseSensitive, Word: WordAny}
	got, _ = runSearch(t, root, opts)
	require.Len(t, got, 2)
	assert.Equal(t, "bar42", utf16Slice(got[0].Text, got[0].Start, got[0].End))
}

func TestContentSearch_ShouldSkipIgnoredAndBinary_WhenIncludeIgnoredFalse(t *testing.T) {
	_, root := writeTree(t, map[string]string{
		".gitignore": "dist/\n",
		"dist/x.js":  "needle\n",
		"src/x.js":   "needle\n",
		"bin.dat":    "needle\x00\x01 binary",
		".git/HEAD":  "needle\n",
	})

	got, stats := runSearch(t, root, literalOpts("needle"))
	assert.Equal(t, []string{"src/x.js"}, matchedPaths(got))
	assert.Equal(t, 1, stats.BinarySkipped)

	opts := literalOpts("needle")
	opts.Scope.IncludeIgnored = true
	got, _ = runSearch(t, root, opts)
	assert.Equal(t, []string{"dist/x.js", "src/x.js"}, matchedPaths(got), ".git never matches, dist does with IncludeIgnored")
}

func TestContentSearch_ShouldEmitRawByteOffsetOfMatchedLineStart_And_NeverLogIt(t *testing.T) {
	_, root := writeTree(t, map[string]string{"a.txt": "\xEF\xBB\xBFfirst\r\nsecond needle\r\nthird needle\n"})
	got, _ := runSearch(t, root, literalOpts("needle"))
	require.Len(t, got, 2)
	assert.EqualValues(t, 3+5+2, got[0].ByteOffset, "BOM and CRLF count toward the raw offset")
	assert.EqualValues(t, 3+5+2+len("second needle\r\n"), got[1].ByteOffset)
	// The never-logged half is asserted by TestSearchContentHandler_ShouldLogQueryLenNotText_When_SearchRuns.
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func TestScanFile_ShouldSkipBinaryAndStopAtTwoMegabyteCap_When_NulOrOversizeInput(t *testing.T) {
	re := mustCompile(t, literalOpts("needle"))

	t.Run("NUL in first 8 KB sets binary", func(t *testing.T) {
		in := append(bytes.Repeat([]byte("a"), 4000), 0)
		res, err := scanContent(context.Background(), bytes.NewReader(append(in, []byte(" needle\n")...)), "b.dat", re, searchFileCap)
		require.NoError(t, err)
		assert.True(t, res.Binary)
		assert.Empty(t, res.Matches)
	})

	t.Run("NUL after the sniff window is not binary", func(t *testing.T) {
		in := append(bytes.Repeat([]byte("a"), searchSniffBytes+10), 0)
		res, err := scanContent(context.Background(), bytes.NewReader(append(in, []byte("\nneedle\n")...)), "b.dat", re, searchFileCap)
		require.NoError(t, err)
		assert.False(t, res.Binary)
	})

	t.Run("nothing past the cap is read", func(t *testing.T) {
		endless := io.MultiReader(strings.NewReader("needle first\n"), repeatReader("filler line\n"))
		cr := &countingReader{r: endless}
		res, err := scanContent(context.Background(), cr, "big.txt", re, searchFileCap)
		require.NoError(t, err)
		assert.True(t, res.Capped)
		assert.LessOrEqual(t, cr.n, int64(searchFileCap), "bytes read from the source")
		require.NotEmpty(t, res.Matches)
	})

	t.Run("range stays inside a clamped 3000-byte line", func(t *testing.T) {
		line := strings.Repeat("x", 2000) + "needle" + strings.Repeat("y", 1000) + "\n"
		res, err := scanContent(context.Background(), strings.NewReader(line), "l.txt", re, searchFileCap)
		require.NoError(t, err)
		require.Len(t, res.Matches, 1)
		m := res.Matches[0]
		assert.LessOrEqual(t, len(m.Text), searchLineClamp)
		assert.Equal(t, "needle", m.Text[m.Start:m.End])
	})

	t.Run("invalid UTF-8 becomes replacement characters", func(t *testing.T) {
		res, err := scanContent(context.Background(), strings.NewReader("\xff\xfe needle\n"), "l.txt", re, searchFileCap)
		require.NoError(t, err)
		require.Len(t, res.Matches, 1)
		m := res.Matches[0]
		assert.Equal(t, "�� needle", m.Text)
		assert.Equal(t, "needle", utf16Slice(m.Text, m.Start, m.End))
	})
}

type repeatReader string

func (r repeatReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		n += copy(p[n:], string(r))
	}
	return n, nil
}

func mustCompile(t testing.TB, o SearchOptions) *regexp.Regexp {
	t.Helper()
	re, err := o.compile()
	require.NoError(t, err)
	return re
}

func TestIgnoreMatcher_ShouldLoadGitignoreOnDirectoryEntryThroughOsRoot_And_NotFollowEscapingSymlink(t *testing.T) {
	outer := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outer, "outside-ignore"), []byte("*.txt\n"), 0o644))
	dir := filepath.Join(outer, "repo")
	files := map[string]string{
		"a/.gitignore":   "*.gen\n",
		"a/b/.gitignore": "secret/\n",
		"a/x.gen":        "needle\n",
		"a/b/y.gen":      "needle\n",
		"a/b/secret/s":   "needle\n",
		"a/b/ok.txt":     "needle\n",
		"c/z.gen":        "needle\n",
		"esc/ok.txt":     "needle\n",
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	require.NoError(t, os.Symlink(filepath.Join(outer, "outside-ignore"), filepath.Join(dir, "esc", ".gitignore")))
	root := openRootFS(t, dir)

	got, _ := runSearch(t, root, literalOpts("needle"))

	assert.Equal(t, []string{"a/b/ok.txt", "c/z.gen", "esc/ok.txt"}, matchedPaths(got),
		"a/ rules cover a/ and a/b/ only; the escaping symlinked .gitignore is not followed")
}

type countingFS struct {
	fs.FS
	dirReads atomic.Int64
	onOpen   func(name string)
}

func (c *countingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	c.dirReads.Add(1)
	return fs.ReadDir(c.FS, name)
}

func (c *countingFS) Open(name string) (fs.File, error) {
	if c.onOpen != nil {
		c.onOpen(name)
	}
	return c.FS.Open(name)
}

func TestIgnoreMatcher_ShouldEmitFirstMatchBeforeTreeIsFullyEnumerated(t *testing.T) {
	files := map[string]string{".gitignore": "*.log\n"}
	for i := 0; i < 60; i++ {
		files[fmt.Sprintf("d%02d/f.txt", i)] = "needle\n"
	}
	_, root := writeTree(t, files)
	cfs := &countingFS{FS: root}

	var readsAtFirst int64 = -1
	_, err := walkSearcher{workers: 1}.Search(context.Background(), cfs, literalOpts("needle"), func([]SearchMatch) error {
		if readsAtFirst < 0 {
			readsAtFirst = cfs.dirReads.Load()
		}
		return nil
	})
	require.NoError(t, err)
	assert.Greater(t, readsAtFirst, int64(0))
	assert.Less(t, readsAtFirst, cfs.dirReads.Load()-20, "the first match must not wait for the walk to finish")
}

func TestIgnoreMatcher_ShouldSkipCommentsBlankLinesAndOverlongLines_And_LetDeeperGitignoreOverrideRoot(t *testing.T) {
	overlong := strings.Repeat("z", 70*1024)
	_, root := writeTree(t, map[string]string{
		".gitignore":    "# comment\n\n   \n" + overlong + "\n*.log\n",
		"a/.gitignore":  "!keep.log\n",
		"a/keep.log":    "needle\n",
		"a/drop.log":    "needle\n",
		"b/keep.log":    "needle\n",
		"root.log":      "needle\n",
		"plain.txt":     "needle\n",
		"#comment.here": "needle\n",
	})

	got, _ := runSearch(t, root, literalOpts("needle"))

	assert.Equal(t, []string{"#comment.here", "a/keep.log", "plain.txt"}, matchedPaths(got),
		"!keep.log re-includes only inside a/ and is popped on leaving; b/keep.log stays ignored")
}

// gitCheckIgnore asks real git which of paths a repo in dir ignores. A test-only
// subprocess, acceptable under prefer-go-git-over-subshells: git itself is the oracle.
func gitCheckIgnore(t *testing.T, dir string, paths []string) map[string]bool {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	run := func(stdin string, args ...string) []byte {
		cmd := exec.Command("git", append([]string{"-c", "core.excludesFile=/dev/null"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		cmd.Stdin = strings.NewReader(stdin)
		out, err := cmd.Output()
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return out // check-ignore exits 1 when nothing matched
		}
		require.NoError(t, err)
		return out
	}
	run("", "init", "-q")
	out := run(strings.Join(paths, "\x00")+"\x00", "check-ignore", "-z", "--stdin", "--no-index")
	ignored := map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			ignored[p] = true
		}
	}
	return ignored
}

func TestContentSearch_ShouldMatchGitCheckIgnoreOracle_WhenGeneratedRepo(t *testing.T) {
	files := map[string]string{
		".gitignore":          "*.log\n/root-only.txt\ntmp/\n**/cache/\ndocs/**/*.gen\nout[0-9].txt\n",
		"a/.gitignore":        "!keep.log\n*.tmp\n/only-a.txt\nsub/*.dat\n",
		"a/b/.gitignore":      "*.out\n!important.out\n",
		"root-only.txt":       "x",
		"a/root-only.txt":     "x",
		"a/only-a.txt":        "x",
		"a/b/only-a.txt":      "x",
		"a/keep.log":          "x",
		"a/drop.log":          "x",
		"a/b/keep.log":        "x",
		"a/x.tmp":             "x",
		"a/b/x.tmp":           "x",
		"a/b/run.out":         "x",
		"a/b/important.out":   "x",
		"a/sub/f.dat":         "x",
		"a/sub/deeper/f.dat":  "x",
		"tmp/inside.txt":      "x",
		"src/tmp/inside.txt":  "x",
		"src/cache/c.txt":     "x",
		"cache/c.txt":         "x",
		"docs/a/b/x.gen":      "x",
		"docs/x.gen":          "x",
		"other/x.gen":         "x",
		"out1.txt":            "x",
		"out10.txt":           "x",
		"src/main.go":         "x",
		"src/#hash.txt":       "x",
		"src/spaced name.txt": "x",
	}
	for p := range files {
		files[p] = "needle\n"
	}
	dir, root := writeTree(t, files)
	all := make([]string, 0, len(files))
	for p := range files {
		all = append(all, p)
	}
	sort.Strings(all)
	want := gitCheckIgnore(t, dir, all)

	got, _ := runSearch(t, root, literalOpts("needle"))
	searched := map[string]bool{}
	for _, p := range matchedPaths(got) {
		searched[p] = true
	}
	for _, p := range all {
		assert.Equal(t, !want[p], searched[p], "%s: git ignored=%v, searched=%v", p, want[p], searched[p])
	}
}

func TestContentSearch_ShouldReturnContextCanceled_WhenCancelledMidWalk(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	files := 50000
	if testing.Short() {
		files = 5000
	}
	root := openRootFS(t, genFixtureTree(t, files))
	assertCancelWithin200ms(t, root, 200)
}

func TestContentSearch_ShouldCancelWithin200ms_When_CancelledWhileLoadingIgnoreRules200Gitignores(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	files := 50000
	if testing.Short() {
		files = 5000
	}
	root := openRootFS(t, genFixtureTree(t, files, WithGitignores(200)))
	// Cancel on the 20th .gitignore open: rules for the rest of the tree are still unread.
	assertCancelWithin200ms(t, root, 0, 20)
}

// assertCancelWithin200ms cancels once afterFileOpens files (or afterIgnoreOpens
// .gitignore files) have been opened, so the cancel point does not depend on machine speed.
func assertCancelWithin200ms(t *testing.T, root fs.FS, afterFileOpens int, ignoreOpens ...int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var files, ignores atomic.Int64
	var cancelledAt atomic.Int64
	cfs := &countingFS{FS: root}
	cfs.onOpen = func(name string) {
		hit := false
		if filepath.Base(name) == gitignoreName {
			hit = len(ignoreOpens) > 0 && ignores.Add(1) == int64(ignoreOpens[0])
		} else {
			hit = afterFileOpens > 0 && files.Add(1) == int64(afterFileOpens)
		}
		if hit {
			cancelledAt.Store(time.Now().UnixNano())
			cancel()
		}
	}

	_, err := NewWalkSearcher().Search(ctx, cfs, literalOpts("zzzz-never-matches"), func([]SearchMatch) error { return nil })
	returned := time.Now().UnixNano()

	require.ErrorIs(t, err, context.Canceled)
	require.NotZero(t, cancelledAt.Load(), "the walk finished before the cancel point")
	assert.Less(t, time.Duration(returned-cancelledAt.Load()), 200*time.Millisecond)
}

func TestContentSearch_ShouldUseMinFourOrHalfGomaxprocsWorkers_When_Searching(t *testing.T) {
	for gmp, want := range map[int]int{1: 1, 2: 1, 3: 1, 4: 2, 8: 4, 24: 4} {
		assert.Equal(t, want, searchWorkerCount(gmp), "GOMAXPROCS=%d", gmp)
	}
	assert.Equal(t, searchWorkerCount(runtime.GOMAXPROCS(0)), NewWalkSearcher().(walkSearcher).workers)

	files := map[string]string{}
	for i := 0; i < 200; i++ {
		files[fmt.Sprintf("f%03d.txt", i)] = "needle\n"
	}
	_, root := writeTree(t, files)
	var inflight, peak atomic.Int64
	cfs := &concurrencyFS{FS: root, inflight: &inflight, peak: &peak}
	_, err := walkSearcher{workers: 2}.Search(context.Background(), cfs, literalOpts("zzzz"), func([]SearchMatch) error { return nil })
	require.NoError(t, err)
	assert.LessOrEqual(t, peak.Load(), int64(2), "no more concurrent file scans than workers")
}

type concurrencyFS struct {
	fs.FS
	inflight, peak *atomic.Int64
}

type trackedFile struct {
	fs.File
	inflight *atomic.Int64
}

func (f trackedFile) Close() error {
	f.inflight.Add(-1)
	return f.File.Close()
}

func (c *concurrencyFS) Open(name string) (fs.File, error) {
	f, err := c.FS.Open(name)
	if err != nil || filepath.Base(name) == gitignoreName {
		return f, err
	}
	if n := c.inflight.Add(1); n > c.peak.Load() {
		c.peak.Store(n)
	}
	return trackedFile{File: f, inflight: c.inflight}, nil
}

// peakHeapGrowth runs fn while sampling HeapAlloc every 10 ms and returns the peak
// growth over the pre-run baseline; each sample follows a GC so it reflects live data.
func peakHeapGrowth(fn func()) uint64 {
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	var peak atomic.Uint64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				runtime.GC() // measure live memory: uncollected garbage is relative to the whole test binary's heap
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > base.HeapAlloc && m.HeapAlloc-base.HeapAlloc > peak.Load() {
					peak.Store(m.HeapAlloc - base.HeapAlloc)
				}
			}
		}
	}()
	fn()
	close(stop)
	<-done
	return peak.Load()
}

func TestContentSearch_ShouldKeepPeakHeapGrowthUnder64MB_When_EveryFileMatches(t *testing.T) {
	ceiling := uint64(64 << 20)
	if raceDetectorOn() {
		// The race runtime slows the GC so allocate-black garbage from the sampler's own
		// cycle inflates HeapAlloc (observed 23-83 MiB on 5k files vs 2 MiB without -race),
		// so the strict 64 MiB ceiling is only asserted in the non-race run.
		ceiling = 192 << 20
	}
	sizes := []int{5000}
	if !testing.Short() {
		sizes = append(sizes, 50000)
	}
	peaks := map[int]uint64{}
	for _, n := range sizes {
		root := openRootFS(t, genFixtureTree(t, n))
		// A match-everything query stops at the 1000-result cap; the never-matching one
		// forces the whole tree through the scanners, which is the memory-relevant path.
		for _, q := range []string{"a ", "zzzz-never-matches"} {
			peak := peakHeapGrowth(func() {
				_, err := NewWalkSearcher().Search(context.Background(), root, literalOpts(q), func([]SearchMatch) error { return nil })
				require.NoError(t, err)
			})
			t.Logf("files=%d query=%q peak heap growth=%d KiB", n, q, peak/kibibyte)
			assert.Less(t, peak, ceiling, "files=%d query=%q", n, q)
			peaks[n] = max(peaks[n], peak)
		}
	}
	if big, ok := peaks[50000]; ok {
		assert.Less(t, big, 5*max(peaks[5000], 8<<20), "heap must not scale with file count (no retained path list)")
	}
}

func raceDetectorOn() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range info.Settings {
		if s.Key == "-race" {
			return s.Value == "true"
		}
	}
	return false
}
