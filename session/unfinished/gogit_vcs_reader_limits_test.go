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
	"testing"

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
