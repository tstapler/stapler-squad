package unfinished

// White-box tests for the OOM-prevention limits in gogit_vcs_reader.go.
//
// These tests must FAIL against pre-fix code (before maxUntrackedFiles,
// maxUntrackedFileSize, and the blob-size check in readBlobUnderLock were
// introduced). They assert the exact behaviours that prevented the OOM:
//
//  1. Large untracked files are counted but not read for line-counting.
//  2. The untracked walk stops after maxUntrackedFiles files.
//  3. Large staged/unstaged tracked files are counted but not diff'd.
//
// Why white-box (package unfinished, not unfinished_test)?
// To override maxUntrackedFiles / maxUntrackedFileSize with small values
// so tests complete quickly without creating 10 000 files.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// initRepoInternal mirrors initRepo from vcsreader_test.go for white-box tests.
func initRepoInternal(t *testing.T) string {
	t.Helper()
	raw := t.TempDir()
	dir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := safeexec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial commit")
	return dir
}

func gitRunInternal(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestGoGitVCSReader_DiffShortstat_largeUntrackedFile_countedButNotRead asserts
// that a large untracked file is counted in Files but its lines are not read,
// so Insertions stays 0.
//
// Pre-fix behaviour: the entire file would be read via os.ReadFile, Insertions
// would equal the line count (~262 144 for a 512 KB "a\n" file).
func TestGoGitVCSReader_DiffShortstat_largeUntrackedFile_countedButNotRead(t *testing.T) {
	repo := initRepoInternal(t)

	// Override the size limit so we can use a small test file.
	origSize := maxUntrackedFileSize
	maxUntrackedFileSize = 1024 // 1 KB for this test
	defer func() { maxUntrackedFileSize = origSize }()

	// Create an untracked file just over the 1 KB limit with ~600 lines.
	content := strings.Repeat("abcdefghijklmnopqrstuvwxyz\n", 60) // 60×27 = 1620 bytes
	if err := os.WriteFile(filepath.Join(repo, "big.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r := &GoGitVCSReader{}
	d, err := r.DiffShortstat(repo)
	if err != nil {
		t.Fatalf("DiffShortstat: %v", err)
	}

	// File must be counted.
	if d.Files != 1 {
		t.Errorf("Files = %d, want 1: large untracked file should be counted", d.Files)
	}
	// Lines must NOT be counted (file was skipped).
	if d.Insertions != 0 {
		t.Errorf("Insertions = %d, want 0: large untracked file should not contribute line count", d.Insertions)
	}
}

// TestGoGitVCSReader_DiffShortstat_smallUntrackedFile_linesAreCounted asserts that
// a small untracked file below the size threshold DOES have its lines counted.
// This is the complement of the large-file test — guards against over-capping.
func TestGoGitVCSReader_DiffShortstat_smallUntrackedFile_linesAreCounted(t *testing.T) {
	repo := initRepoInternal(t)

	origSize := maxUntrackedFileSize
	maxUntrackedFileSize = 1024
	defer func() { maxUntrackedFileSize = origSize }()

	// Create a small untracked file with 3 lines (well under 1 KB).
	if err := os.WriteFile(filepath.Join(repo, "small.txt"), []byte("a\nb\nc\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &GoGitVCSReader{}
	d, err := r.DiffShortstat(repo)
	if err != nil {
		t.Fatalf("DiffShortstat: %v", err)
	}

	if d.Files != 1 {
		t.Errorf("Files = %d, want 1", d.Files)
	}
	if d.Insertions != 3 {
		t.Errorf("Insertions = %d, want 3: small untracked file lines should be counted", d.Insertions)
	}
}

// TestGoGitVCSReader_DiffShortstat_untrackedFilesCapStopsWalk asserts that when
// the number of untracked files exceeds maxUntrackedFiles, the walk stops and
// only maxUntrackedFiles entries are counted.
//
// Pre-fix behaviour: the walk would never stop; all N+1 files would be visited.
func TestGoGitVCSReader_DiffShortstat_untrackedFilesCapStopsWalk(t *testing.T) {
	repo := initRepoInternal(t)

	origMax := maxUntrackedFiles
	maxUntrackedFiles = 5
	defer func() { maxUntrackedFiles = origMax }()

	// Create maxUntrackedFiles+1 small untracked files.
	for i := 0; i <= maxUntrackedFiles; i++ {
		name := filepath.Join(repo, fmt.Sprintf("file%03d.txt", i))
		if err := os.WriteFile(name, []byte("line\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	r := &GoGitVCSReader{}
	d, err := r.DiffShortstat(repo)
	if err != nil {
		t.Fatalf("DiffShortstat: %v", err)
	}

	// Exactly maxUntrackedFiles files should be counted; the walk stops early.
	if d.Files != maxUntrackedFiles {
		t.Errorf("Files = %d, want %d: walk should stop at the cap",
			d.Files, maxUntrackedFiles)
	}
}

// TestGoGitVCSReader_DiffShortstat_largeModifiedTrackedFile_countedButNotDiffed
// asserts that a tracked file that exceeds the size threshold is counted in Files
// but produces no Insertions/Deletions — preventing OOM from blob reads and LCS.
//
// Pre-fix behaviour: both the HEAD blob and the working-tree file would be fully
// read into memory; linesDiffBytes (LCS) would be run on them.
func TestGoGitVCSReader_DiffShortstat_largeModifiedTrackedFile_countedButNotDiffed(t *testing.T) {
	repo := initRepoInternal(t)

	origSize := maxUntrackedFileSize
	maxUntrackedFileSize = 1024
	defer func() { maxUntrackedFileSize = origSize }()

	// Commit a large tracked file (just over the 1 KB threshold).
	// 600 × "x\n" = 1200 bytes > 1024 byte test limit.
	largeContent := strings.Repeat("x\n", 600)
	if err := os.WriteFile(filepath.Join(repo, "large.txt"), []byte(largeContent), 0644); err != nil {
		t.Fatal(err)
	}
	gitRunInternal(t, repo, "add", "large.txt")
	gitRunInternal(t, repo, "commit", "-m", "add large file")

	// Modify it (unstaged change): append more lines so both HEAD and WT exceed the limit.
	modifiedContent := largeContent + strings.Repeat("y\n", 600)
	if err := os.WriteFile(filepath.Join(repo, "large.txt"), []byte(modifiedContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := &GoGitVCSReader{}
	d, err := r.DiffShortstat(repo)
	if err != nil {
		t.Fatalf("DiffShortstat: %v", err)
	}

	// File is changed → should be counted.
	if d.Files != 1 {
		t.Errorf("Files = %d, want 1: large modified file should be counted", d.Files)
	}
	// Lines must NOT be counted because both HEAD blob and WT file exceed the limit.
	if d.Insertions != 0 {
		t.Errorf("Insertions = %d, want 0: large modified file should skip line diff", d.Insertions)
	}
}

// TestReadFileIfSmall_belowLimit reads a small file successfully.
func TestReadFileIfSmall_belowLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	origSize := maxUntrackedFileSize
	maxUntrackedFileSize = 100
	defer func() { maxUntrackedFileSize = origSize }()

	data, ok := readFileIfSmall(path)
	if !ok {
		t.Fatal("readFileIfSmall returned false for file under limit")
	}
	if string(data) != "hello\n" {
		t.Errorf("data = %q, want %q", data, "hello\n")
	}
}

// TestReadFileIfSmall_atOrAboveLimit returns false without reading.
func TestReadFileIfSmall_atOrAboveLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	content := strings.Repeat("a", 101)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	origSize := maxUntrackedFileSize
	maxUntrackedFileSize = 100
	defer func() { maxUntrackedFileSize = origSize }()

	data, ok := readFileIfSmall(path)
	if ok {
		t.Errorf("readFileIfSmall returned true for file at/above limit (data len=%d)", len(data))
	}
}

// TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers verifies
// result consistency and absence of data races when 4 goroutines call AheadBehind
// concurrently with a cold cache. It does not assert call-count deduplication
// (GoGitVCSReader has no invocation counter hook); race-detector clean is the
// correctness gate.
func TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers(t *testing.T) {
	dir := initRepoInternal(t)

	r := &GoGitVCSReader{}
	// Ensure cache is cold (zero value = expired)
	const workers = 4
	type result struct {
		ahead, behind int
		err           error
	}
	results := make([]result, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		go func(idx int) {
			defer wg.Done()
			a, b, err := r.AheadBehind(dir, "main")
			results[idx] = result{a, b, err}
		}(i)
	}
	wg.Wait()
	for i, res := range results {
		if res.err != nil {
			t.Errorf("worker %d: unexpected error: %v", i, res.err)
		}
		if res.ahead != results[0].ahead || res.behind != results[0].behind {
			t.Errorf("worker %d: got (%d, %d), want (%d, %d)",
				i, res.ahead, res.behind, results[0].ahead, results[0].behind)
		}
	}
}

// TestGoGitVCSReader_AheadBehind_InvalidPath_ReturnsError verifies that
// openRepoEntry failure inside the singleflight Do body is returned as an
// error to the caller without panicking. True panic injection requires a
// hook into the Do body which GoGitVCSReader does not expose.
func TestGoGitVCSReader_AheadBehind_InvalidPath_ReturnsError(t *testing.T) {
	// Use a non-existent path — openRepoEntry returns error, not panic,
	// but confirms the caller handles Do errors without crashing.
	r := &GoGitVCSReader{}
	_, _, err := r.AheadBehind("/nonexistent/path/guaranteed-missing", "main")
	if err == nil {
		t.Fatal("expected error from non-existent repo, got nil")
	}
	// Reaching here proves no panic escaped to the caller
}

// TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue verifies
// that a warm hasUncommittedCache entry is returned without re-scanning.
func TestGoGitVCSReader_HasUncommitted_CacheHitReturnsCachedValue(t *testing.T) {
	dir := initRepoInternal(t)
	r := &GoGitVCSReader{}
	// Cold call — establishes the actual value
	got, err := r.HasUncommitted(dir)
	if err != nil {
		t.Fatalf("cold call: %v", err)
	}
	// Pre-populate cache with inverted value to detect cache bypass
	r.hasUncommittedCache.Store(dir, hasUncommittedEntry{
		result: !got,
		expiry: time.Now().Add(30 * time.Second),
	})
	cached, err := r.HasUncommitted(dir)
	if err != nil {
		t.Fatalf("warm call: %v", err)
	}
	if cached != !got {
		t.Errorf("warm call: got %v, want %v (cache was not used)", cached, !got)
	}
}

// TestFileNeedsContentCheck is a table-driven unit test for the racy-git
// window gate: a stat-clean file should only need a content-hash fallback
// when its mtime is not clearly before the index was last written.
func TestFileNeedsContentCheck(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		fileMtime  time.Time
		indexMtime time.Time
		want       bool
	}{
		{"file clearly older than index: safe to trust stat", base.Add(-time.Hour), base, false},
		{"file mtime equals index mtime: racy, needs hash", base, base, true},
		{"file mtime after index mtime: racy, needs hash", base.Add(time.Second), base, true},
		{"unknown index mtime: conservative, needs hash", base, time.Time{}, true},
		{"file one second before index: safe (past the racy window)", base.Add(-time.Second), base, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fileNeedsContentCheck(tt.fileMtime, tt.indexMtime); got != tt.want {
				t.Errorf("fileNeedsContentCheck(%v, %v) = %v, want %v", tt.fileMtime, tt.indexMtime, got, tt.want)
			}
		})
	}
}

// TestGoGitVCSReader_DiffShortstat_gitignoredUntrackedFile_notCounted verifies
// that walkUntracked's gitignore matcher excludes ignored paths entirely —
// both from the file count and from line-counting. Pre-fix behaviour: the
// walk had no gitignore awareness, so an ignored directory the size of
// node_modules would be walked and read just like any other untracked file.
func TestGoGitVCSReader_DiffShortstat_gitignoredUntrackedFile_notCounted(t *testing.T) {
	repo := initRepoInternal(t)

	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "visible.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// A large ignored file — if it leaked past the matcher, its 500 lines
	// would show up in Insertions and its 1 file would inflate Files.
	ignoredContent := strings.Repeat("line\n", 500)
	if err := os.WriteFile(filepath.Join(repo, "ignored.txt"), []byte(ignoredContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := &GoGitVCSReader{}
	d, err := r.DiffShortstat(repo)
	if err != nil {
		t.Fatalf("DiffShortstat: %v", err)
	}

	// Only .gitignore (1 line) and visible.txt (1 line) should be counted.
	if d.Files != 2 {
		t.Errorf("Files = %d, want 2 (.gitignore + visible.txt; ignored.txt must be excluded)", d.Files)
	}
	if d.Insertions != 2 {
		t.Errorf("Insertions = %d, want 2: ignored.txt's 500 lines must not be counted", d.Insertions)
	}
}

// TestGoGitVCSReader_DiffShortstat_headTreeCache_invalidatesOnNewCommit
// verifies the headTreeCache is keyed by HEAD commit hash and is NOT reused
// once HEAD moves. A stale cache here would silently compare the working
// tree against the wrong (old) HEAD blob, producing incorrect diff stats.
func TestGoGitVCSReader_DiffShortstat_headTreeCache_invalidatesOnNewCommit(t *testing.T) {
	repo := initRepoInternal(t)
	readme := filepath.Join(repo, "README.md")

	r := &GoGitVCSReader{}
	d, err := r.diffShortstatUncached(repo)
	if err != nil {
		t.Fatalf("diffShortstatUncached (cold): %v", err)
	}
	if d.Files != 0 {
		t.Fatalf("Files = %d, want 0 on a clean repo", d.Files)
	}

	// Populate entry.headTreeCache for the initial commit's HEAD.
	entryVal, ok := r.repoCache.Load(repo)
	if !ok {
		t.Fatal("expected a cached repo entry after diffShortstatUncached")
	}
	entry := entryVal.(*cachedRepo)
	firstHeadHash := entry.headTreeHash
	if firstHeadHash.IsZero() {
		t.Fatal("expected headTreeHash to be populated after first call")
	}

	// Move HEAD: commit a change to README.md ("hello\n" -> "hello\nworld\n").
	if err := os.WriteFile(readme, []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRunInternal(t, repo, "add", "README.md")
	gitRunInternal(t, repo, "commit", "-m", "add world line")

	// Revert the working tree to the OLD content — relative to the NEW HEAD,
	// this is a 1-line deletion. A stale cache stuck on the old HEAD (where
	// README.md's blob hash matched "hello\n") would wrongly report no change.
	if err := os.WriteFile(readme, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	d, err = r.diffShortstatUncached(repo)
	if err != nil {
		t.Fatalf("diffShortstatUncached (after HEAD move): %v", err)
	}
	if d.Files != 1 {
		t.Errorf("Files = %d, want 1: cache must invalidate when HEAD moves", d.Files)
	}
	if d.Deletions != 1 {
		t.Errorf("Deletions = %d, want 1 (the reverted 'world' line)", d.Deletions)
	}

	entryVal, _ = r.repoCache.Load(repo)
	entry = entryVal.(*cachedRepo)
	if entry.headTreeHash == firstHeadHash {
		t.Error("headTreeHash was not updated after HEAD moved")
	}
}

// TestGoGitVCSReader_DiffShortstat_blobCache_populatedWithCorrectContent
// verifies that after diffing a changed tracked file, entry.blobCache holds
// the file's HEAD-version bytes (not the working-tree bytes) keyed by the
// HEAD blob hash — guarding against accidentally caching the wrong side of
// the diff, which repeated-poll callers would silently reuse.
func TestGoGitVCSReader_DiffShortstat_blobCache_populatedWithCorrectContent(t *testing.T) {
	repo := initRepoInternal(t)
	readme := filepath.Join(repo, "README.md")

	if err := os.WriteFile(readme, []byte("hello\nchanged\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &GoGitVCSReader{}
	d, err := r.diffShortstatUncached(repo)
	if err != nil {
		t.Fatalf("diffShortstatUncached: %v", err)
	}
	if d.Files != 1 {
		t.Fatalf("Files = %d, want 1", d.Files)
	}

	entryVal, ok := r.repoCache.Load(repo)
	if !ok {
		t.Fatal("expected a cached repo entry")
	}
	entry := entryVal.(*cachedRepo)

	found := false
	entry.blobCache.Range(func(_, v any) bool {
		if string(v.([]byte)) == "hello\n" {
			found = true
			return false
		}
		return true
	})
	if !found {
		t.Error("blobCache does not contain the HEAD blob content (\"hello\\n\") for the changed file")
	}
}

// TestGoGitVCSReader_HasUncommitted_headTreeCache_invalidatesOnNewCommit
// verifies HasUncommitted's staged-change detection uses entry.headTreeCache
// correctly across a HEAD move — the same cache diffShortstatUncached
// populates and consumes. A stale cache here would miss a staged revert
// whose content happens to match an OLD HEAD version of the file, reporting
// a dirty repo as clean.
func TestGoGitVCSReader_HasUncommitted_headTreeCache_invalidatesOnNewCommit(t *testing.T) {
	repo := initRepoInternal(t)
	readme := filepath.Join(repo, "README.md")

	// commit1: README.md = "AAA\n"
	if err := os.WriteFile(readme, []byte("AAA\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRunInternal(t, repo, "add", "README.md")
	gitRunInternal(t, repo, "commit", "-m", "AAA")

	r := &GoGitVCSReader{}
	dirty, err := r.HasUncommitted(repo)
	if err != nil {
		t.Fatalf("HasUncommitted (cold): %v", err)
	}
	if dirty {
		t.Fatal("expected clean repo after commit1")
	}

	entryVal, ok := r.repoCache.Load(repo)
	if !ok {
		t.Fatal("expected a cached repo entry after HasUncommitted")
	}
	entry := entryVal.(*cachedRepo)
	firstHeadHash := entry.headTreeHash
	if firstHeadHash.IsZero() {
		t.Fatal("expected headTreeHash to be populated after first call")
	}

	// commit2: moves HEAD — README.md = "BBB\n"
	if err := os.WriteFile(readme, []byte("BBB\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRunInternal(t, repo, "add", "README.md")
	gitRunInternal(t, repo, "commit", "-m", "BBB")

	// Stage a revert to the OLD content ("AAA\n") — a real staged change
	// relative to the NEW HEAD ("BBB\n"), but one a stale commit1-era
	// headTreeCache would miss entirely, since "AAA\n" was exactly what
	// commit1's tree already recorded for this file.
	if err := os.WriteFile(readme, []byte("AAA\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRunInternal(t, repo, "add", "README.md")

	r.hasUncommittedCache.Delete(repo) // bypass HasUncommitted's own 30s TTL cache

	dirty, err = r.HasUncommitted(repo)
	if err != nil {
		t.Fatalf("HasUncommitted (after HEAD move): %v", err)
	}
	if !dirty {
		t.Error("expected dirty=true: a stale headTreeCache would miss this staged revert")
	}

	entryVal, _ = r.repoCache.Load(repo)
	entry = entryVal.(*cachedRepo)
	if entry.headTreeHash == firstHeadHash {
		t.Error("headTreeHash was not updated after HEAD moved")
	}
}

// TestReachableSet_CapsAtLimit verifies reachableSet stops walking history
// once reachableSetLimit commits have been visited, rather than walking the
// full history unboundedly (the same class of bound findMergeBase already
// applies via mergeBaseBFSLimit).
func TestReachableSet_CapsAtLimit(t *testing.T) {
	repo := initRepoInternal(t) // 1 commit already (the initial commit)

	for i := range 5 {
		name := filepath.Join(repo, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(name, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		gitRunInternal(t, repo, "add", ".")
		gitRunInternal(t, repo, "commit", "-m", fmt.Sprintf("commit %d", i))
	}
	// Total history: 6 commits.

	gitRepo, err := git.PlainOpen(repo)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := gitRepo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	origLimit := reachableSetLimit
	reachableSetLimit = 3
	defer func() { reachableSetLimit = origLimit }()

	set, err := reachableSet(gitRepo, head.Hash())
	if err != nil {
		t.Fatalf("reachableSet: %v", err)
	}
	if len(set) != 3 {
		t.Errorf("len(set) = %d, want 3 (capped by reachableSetLimit; history has 6 commits)", len(set))
	}
}

// TestLcsLength_CapsAtMaxLines verifies both lcsLength and lcsLengthBytes
// skip the O(n*m) DP entirely once either input exceeds maxLCSLines,
// returning 0 (which callers' len(new)-lcs / len(old)-lcs formulas turn into
// a bounded "fully replaced" approximation) instead of running the full DP.
func TestLcsLength_CapsAtMaxLines(t *testing.T) {
	origLimit := maxLCSLines
	maxLCSLines = 3
	defer func() { maxLCSLines = origLimit }()

	t.Run("string lines", func(t *testing.T) {
		linesA := []string{"a", "b", "c", "d", "e"} // len=5 > cap=3, identical on both sides
		linesB := []string{"a", "b", "c", "d", "e"}
		if got := lcsLength(linesA, linesB); got != 0 {
			t.Errorf("lcsLength = %d, want 0 (over the cap, even though inputs are identical)", got)
		}
		// Sanity: at/under the cap, the real DP still runs and computes correctly.
		small := []string{"a", "b"}
		if got := lcsLength(small, small); got != 2 {
			t.Errorf("lcsLength = %d, want 2 for input at/under the cap", got)
		}
	})

	t.Run("byte lines", func(t *testing.T) {
		linesA := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d"), []byte("e")}
		linesB := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d"), []byte("e")}
		if got := lcsLengthBytes(linesA, linesB); got != 0 {
			t.Errorf("lcsLengthBytes = %d, want 0 (over the cap, even though inputs are identical)", got)
		}
		small := [][]byte{[]byte("a"), []byte("b")}
		if got := lcsLengthBytes(small, small); got != 2 {
			t.Errorf("lcsLengthBytes = %d, want 2 for input at/under the cap", got)
		}
	})
}
