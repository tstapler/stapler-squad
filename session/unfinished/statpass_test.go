package unfinished

// Story 4.2.4 tests: the stat-pass duplication measurement (Task 4.2.4a).
// HasUncommitted and DiffShortstat each walk the tracked files with an lstat
// per file; the statFn seam counts them.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// statSplit is the per-call StatCount of one dirty scan.
type statSplit struct{ has, diff int64 }

// duplicate is the share of the scan's lstat calls spent on files the first
// pass had already classified: HasUncommitted's pass, which DiffShortstat
// repeats from the start.
func (s statSplit) duplicate() float64 {
	if s.has+s.diff == 0 {
		return 0
	}
	return float64(min(s.has, s.diff)) / float64(s.has+s.diff)
}

// measureStatSplit runs HasUncommitted then DiffShortstat cold on dir and
// returns the lstat count of each. dirty reports HasUncommitted's verdict
// (DiffShortstat runs regardless, as a measurement convenience; the scanner
// only calls it when dirty).
func measureStatSplit(t testing.TB, dir string) (split statSplit, dirty bool) {
	t.Helper()
	c := NewCountingReader()
	var err error
	dirty, err = c.HasUncommitted(dir)
	require.NoError(t, err)
	split.has = c.StatCount()
	if dirty {
		_, err = c.DiffShortstat(dir)
		require.NoError(t, err)
	}
	split.diff = c.StatCount() - split.has
	return split, dirty
}

func sequentialFiles(n int) map[string]string {
	files := make(map[string]string, n)
	for i := range n {
		files[fmt.Sprintf("f%05d.txt", i)] = "x\n"
	}
	return files
}

func TestDuplicateStatFraction_ShouldBeRecorded_BeforeBuilding(t *testing.T) {
	t.Parallel()
	const n = 5000
	cases := []struct {
		name    string
		mutate  func(t *testing.T, dir string)
		wantHas func(split statSplit) bool
	}{
		{"unstaged edit of the first indexed file", func(t *testing.T, dir string) {
			require.NoError(t, writeFile(dir, "f00000.txt", "edited\n"))
		}, func(s statSplit) bool { return s.has == 1 }},
		{"unstaged edit of the middle file", func(t *testing.T, dir string) {
			require.NoError(t, writeFile(dir, "f02500.txt", "edited\n"))
		}, func(s statSplit) bool { return s.has == n/2+1 }},
		{"unstaged edit of the last file", func(t *testing.T, dir string) {
			require.NoError(t, writeFile(dir, "f04999.txt", "edited\n"))
		}, func(s statSplit) bool { return s.has == n }},
		{"untracked file only", func(t *testing.T, dir string) {
			require.NoError(t, writeFile(dir, "new.txt", "new\n"))
		}, func(s statSplit) bool { return s.has == n }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, dir := trackedRepo(t, sequentialFiles(n))
			c.mutate(t, dir)

			split, dirty := measureStatSplit(t, dir)

			require.True(t, dirty, "fixture must be dirty")
			assert.True(t, c.wantHas(split), "has=%d diff=%d", split.has, split.diff)
			assert.EqualValues(t, n, split.diff, "DiffShortstat stats every unstaged tracked file")
			t.Logf("SG7 %-40s has=%d diff=%d duplicate=%.1f%% of total stats", c.name, split.has, split.diff, 100*split.duplicate())
		})
	}
}

// Optional measurement on the Story 0.1.5 workload corpus (read-only). Set
// SSQ_SG7_CORPUS to its manifest.json.
func TestDuplicateStatFraction_ShouldMeasureWorkloadCorpus_WhenManifestProvided(t *testing.T) {
	manifest := os.Getenv("SSQ_SG7_CORPUS")
	if manifest == "" {
		t.Skip("set SSQ_SG7_CORPUS to the corpus manifest.json to run")
	}
	raw, err := os.ReadFile(manifest)
	require.NoError(t, err)
	var m struct {
		Repos []struct {
			Name      string   `json:"name"`
			Path      string   `json:"path"`
			Worktrees []string `json:"worktrees"`
		} `json:"repos"`
	}
	require.NoError(t, json.Unmarshal(raw, &m))

	var total, dup, scans, dirtyScans int64
	for _, r := range m.Repos {
		for _, dir := range append([]string{r.Path}, r.Worktrees...) {
			split, dirty := measureStatSplit(t, dir)
			scans++
			total += split.has + split.diff
			if dirty {
				dirtyScans++
				dup += min(split.has, split.diff)
			}
			t.Logf("%-28s dirty=%-5v has=%-6d diff=%-6d", filepath.Base(dir), dirty, split.has, split.diff)
		}
	}
	require.NotZero(t, total)
	t.Logf("SG7 corpus: %d scans (%d dirty), %d lstat calls, %d duplicate = %.1f%% of total stats", scans, dirtyScans, total, dup, 100*float64(dup)/float64(total))
}

// scanOnce runs one forced scanWorktree through a Scanner over reader c and
// returns the result with the number of lstat calls it made.
func scanOnce(t testing.TB, c *CountingReader, dir string) (ScanResult, int64) {
	t.Helper()
	s := NewScannerWithReader(nil, nil, c)
	before := c.StatCount()
	res := s.scanWorktree(context.Background(), WorktreeInfo{Path: dir, Branch: "main"}, "", dir, true)
	require.Equal(t, ScanResultStatusOK, res.Status, res.ErrorMsg)
	return res, c.StatCount() - before
}

func newPassReader(skip bool) *CountingReader {
	c := NewCountingReader()
	c.scanSkip.flags = &Flags{ScanSkip: skip}
	c.scanSkip.clock = func() time.Time { return time.Now().Add(time.Minute) }
	return c
}

// G6: on a dirty worktree one scan stats each tracked file at most once, never
// more than the legacy path, and reports identical results.
func TestStatPassHandoff_ShouldNotExceedTrackedFilesOrLegacy_WhenDirtyWorktreeScanned(t *testing.T) {
	t.Parallel()
	const n = 400
	cases := []struct {
		name   string
		mutate func(t *testing.T, dir string)
	}{
		{"unstaged edit", func(t *testing.T, dir string) { require.NoError(t, writeFile(dir, "f00200.txt", "edited\nlines\n")) }},
		{"untracked only", func(t *testing.T, dir string) { require.NoError(t, writeFile(dir, "new.txt", "a\nb\nc\n")) }},
		{"deleted tracked file", func(t *testing.T, dir string) { require.NoError(t, os.Remove(filepath.Join(dir, "f00010.txt"))) }},
		{"sub-second mtime change only", func(t *testing.T, dir string) {
			p := filepath.Join(dir, "f00100.txt")
			fi, err := os.Stat(p)
			require.NoError(t, err)
			bumped := fi.ModTime().Add(300 * time.Millisecond)
			require.NoError(t, os.Chtimes(p, bumped, bumped))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			repo, dir := testrepo.NewRepo(t)
			testrepo.CommitAll(t, repo, sequentialFiles(n), "init", testrepo.Epoch)
			ageIndex(t, dir)
			c.mutate(t, dir)

			legacy, legacyStats := scanOnce(t, newPassReader(false), dir)
			passReader := newPassReader(true)
			got, gotStats := scanOnce(t, passReader, dir)

			require.True(t, legacy.HasUncommitted, "fixture must be dirty")
			assert.Equal(t, legacy.HasUncommitted, got.HasUncommitted)
			assert.Equal(t, legacy.ChangedFiles, got.ChangedFiles)
			assert.Equal(t, legacy.LinesAdded, got.LinesAdded)
			assert.Equal(t, legacy.LinesRemoved, got.LinesRemoved)
			assert.LessOrEqual(t, gotStats, int64(n), "at most one lstat per tracked file")
			assert.LessOrEqual(t, gotStats, legacyStats, "never more than the legacy path")
			t.Logf("%-30s legacy=%d stats, handoff=%d stats (tracked=%d)", c.name, legacyStats, gotStats, n)
		})
	}
}

func TestStatPassHandoff_ShouldUseConsistentVerdict_WhenFileEditedBetweenPasses(t *testing.T) {
	t.Parallel()
	repo, dir := testrepo.NewRepo(t)
	testrepo.CommitAll(t, repo, sequentialFiles(20), "init", testrepo.Epoch)
	// Index newer than every file: no file is racy, so the diff's verdict for a
	// stat-clean file comes from the stat alone, not from re-reading content.
	future := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, ".git", "index"), future, future))
	require.NoError(t, writeFile(dir, "f00001.txt", "first edit\n"))
	g := newPassReader(true)

	dirty, pass, err := g.HasUncommittedPass(dir)
	require.NoError(t, err)
	require.True(t, dirty)
	require.NotNil(t, pass)

	// Edited after the stat pass, before DiffShortstat: the pass's view wins.
	require.NoError(t, writeFile(dir, "f00002.txt", "edit between the passes\n"))
	d, err := g.DiffShortstatPass(dir, pass)
	require.NoError(t, err)
	assert.Equal(t, 1, d.Files, "the late edit is judged by the earlier stat: not counted yet")

	// The next scan sees it.
	g.InvalidateWorktree(dir)
	dirty, pass, err = g.HasUncommittedPass(dir)
	require.NoError(t, err)
	require.True(t, dirty)
	d, err = g.DiffShortstatPass(dir, pass)
	require.NoError(t, err)
	assert.Equal(t, 2, d.Files)
}

func TestStatCount_ShouldEqualTrackedFiles_WhenCleanScan(t *testing.T) {
	t.Parallel()
	const n = 300
	_, dir := trackedRepo(t, sequentialFiles(n))
	for _, skip := range []bool{false, true} {
		c := newPassReader(skip)
		_, stats := scanOnce(t, c, dir)
		assert.EqualValues(t, n, stats, "documented O(n) floor, scan_skip=%v", skip)
	}
}
