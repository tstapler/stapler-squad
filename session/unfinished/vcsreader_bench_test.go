package unfinished_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// benchRepo holds a controlled git repo used for all benchmarks.
// Created once per test binary run via TestMain or on first use.
type benchRepo struct {
	path string // absolute path (symlinks resolved)
	base string // name of the base branch (HEAD at commit 0)
}

// newBenchRepo creates a git repo with nCommits commits ahead of a base branch,
// plus one untracked file and one modified tracked file.
func newBenchRepo(b *testing.B, nCommits int) *benchRepo {
	b.Helper()
	raw := b.TempDir()
	dir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		b.Fatalf("EvalSymlinks: %v", err)
	}

	run := func(args ...string) string {
		b.Helper()
		cmd := safeexec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Bench", "GIT_AUTHOR_EMAIL=bench@test.com",
			"GIT_COMMITTER_NAME=Bench", "GIT_COMMITTER_EMAIL=bench@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "bench@test.com")
	run("git", "config", "user.name", "Bench")

	// Initial commit — becomes the base.
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0644)
	run("git", "add", ".")
	run("git", "commit", "-m", "initial")
	run("git", "branch", "base")

	// Add nCommits ahead of base.
	for i := range nCommits {
		name := fmt.Sprintf("file%d.txt", i)
		_ = os.WriteFile(filepath.Join(dir, name), []byte(strconv.Itoa(i)+"\n"), 0644)
		run("git", "add", ".")
		run("git", "commit", "-m", fmt.Sprintf("commit %d", i))
	}

	// Dirty working tree: modify a tracked file + leave one untracked.
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\nworld\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new\nfile\n"), 0644)

	return &benchRepo{path: dir, base: "base"}
}

type namedReader struct {
	name string
	r    unfinished.VCSReader
}

func readers() []namedReader {
	return []namedReader{
		{"GitVCSReader", &unfinished.GitVCSReader{}},
		{"GoGitVCSReader", &unfinished.GoGitVCSReader{}},
	}
}

// commitCounts to benchmark against — small (typical session), medium, large.
var commitCounts = []int{5, 20, 100}

func BenchmarkHasUncommitted(b *testing.B) {
	repo := newBenchRepo(b, 5)
	for _, nr := range readers() {
		b.Run(nr.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = nr.r.HasUncommitted(repo.path)
			}
		})
	}
}

func BenchmarkAheadBehind(b *testing.B) {
	for _, n := range commitCounts {
		repo := newBenchRepo(b, n)
		for _, nr := range readers() {
			b.Run(fmt.Sprintf("%s/%dcommits", nr.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _, _ = nr.r.AheadBehind(repo.path, repo.base)
				}
			})
		}
	}
}

func BenchmarkCommitMessages(b *testing.B) {
	for _, n := range commitCounts {
		repo := newBenchRepo(b, n)
		for _, nr := range readers() {
			b.Run(fmt.Sprintf("%s/%dcommits", nr.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _ = nr.r.CommitMessages(repo.path, repo.base, 20)
				}
			})
		}
	}
}

func BenchmarkDiffShortstat(b *testing.B) {
	repo := newBenchRepo(b, 5)
	for _, nr := range readers() {
		b.Run(nr.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = nr.r.DiffShortstat(repo.path)
			}
		})
	}
}

func BenchmarkListWorktrees(b *testing.B) {
	repo := newBenchRepo(b, 5)
	for _, nr := range readers() {
		b.Run(nr.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = nr.r.ListWorktrees(repo.path)
			}
		})
	}
}

func BenchmarkResolveDefaultBranch(b *testing.B) {
	repo := newBenchRepo(b, 5)
	for _, nr := range readers() {
		b.Run(nr.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = nr.r.ResolveDefaultBranch(repo.path)
			}
		})
	}
}

// BenchmarkFullScanCycle is the hot path the scanner runs every 30s per worktree.
func BenchmarkFullScanCycle(b *testing.B) {
	for _, n := range commitCounts {
		repo := newBenchRepo(b, n)
		for _, nr := range readers() {
			b.Run(fmt.Sprintf("%s/%dcommits", nr.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _ = nr.r.HasUncommitted(repo.path)
					_, _, _ = nr.r.AheadBehind(repo.path, repo.base)
					_, _ = nr.r.DiffShortstat(repo.path)
				}
			})
		}
	}
}
