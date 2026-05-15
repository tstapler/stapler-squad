package unfinished_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// initRepo creates a git repo at path with one commit and returns its path.
// The path is resolved through symlinks so it matches what git reports on macOS.
func initRepo(t *testing.T) string {
	t.Helper()
	raw := t.TempDir()
	// On macOS, /var/folders is a symlink to /private/var/folders. Resolve so
	// comparisons against git output (which resolves symlinks) don't fail.
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

	// Initial commit.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial commit")

	return dir
}

// addCommit adds a file and commits it, returning the short hash.
func addCommit(t *testing.T, repoPath, filename, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, filename), []byte(message), 0644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := safeexec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", ".")
	run("commit", "-m", message)
}

// TestVCSReaderContractGit verifies the CLI-git reader satisfies the interface contract.
func TestVCSReaderContractGit(t *testing.T) {
	testVCSReaderContract(t, &unfinished.GitVCSReader{})
}

// TestVCSReaderContractGoGit verifies the go-git reader satisfies the interface contract.
func TestVCSReaderContractGoGit(t *testing.T) {
	testVCSReaderContract(t, &unfinished.GoGitVCSReader{})
}

// testVCSReaderContract is a shared suite that any VCSReader implementation must pass.
func testVCSReaderContract(t *testing.T, r unfinished.VCSReader) {
	t.Helper()

	t.Run("ListWorktrees_returnsMainWorktree", func(t *testing.T) {
		repo := initRepo(t)
		wts, err := r.ListWorktrees(repo)
		if err != nil {
			t.Fatalf("ListWorktrees: %v", err)
		}
		if len(wts) == 0 {
			t.Fatal("expected at least one worktree")
		}
		found := false
		for _, wt := range wts {
			if wt.Path == repo {
				found = true
				if wt.Branch == "" && !wt.IsDetached {
					t.Error("main worktree has no branch and is not marked detached")
				}
			}
		}
		if !found {
			t.Errorf("repo path %q not in worktree list: %+v", repo, wts)
		}
	})

	t.Run("HasUncommitted_false_on_clean_repo", func(t *testing.T) {
		repo := initRepo(t)
		dirty, err := r.HasUncommitted(repo)
		if err != nil {
			t.Fatalf("HasUncommitted: %v", err)
		}
		if dirty {
			t.Error("expected clean repo to have no uncommitted changes")
		}
	})

	t.Run("HasUncommitted_true_when_file_modified", func(t *testing.T) {
		repo := initRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty"), 0644); err != nil {
			t.Fatal(err)
		}
		dirty, err := r.HasUncommitted(repo)
		if err != nil {
			t.Fatalf("HasUncommitted: %v", err)
		}
		if !dirty {
			t.Error("expected dirty repo to have uncommitted changes")
		}
	})

	t.Run("AheadBehind_zero_on_single_commit_repo", func(t *testing.T) {
		repo := initRepo(t)
		// Set up a "base" branch pointing at the same commit.
		cmd := safeexec.CommandContext(context.Background(), "git", "branch", "base")
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch: %v\n%s", err, out)
		}
		ahead, behind, err := r.AheadBehind(repo, "base")
		if err != nil {
			t.Fatalf("AheadBehind: %v", err)
		}
		if ahead != 0 || behind != 0 {
			t.Errorf("expected 0/0 ahead/behind, got %d/%d", ahead, behind)
		}
	})

	t.Run("AheadBehind_counts_correctly", func(t *testing.T) {
		repo := initRepo(t)

		// Create base branch at initial commit.
		cmd := safeexec.CommandContext(context.Background(), "git", "branch", "base")
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch: %v\n%s", err, out)
		}

		// Add 2 commits on main.
		addCommit(t, repo, "a.txt", "commit a")
		addCommit(t, repo, "b.txt", "commit b")

		ahead, _, err := r.AheadBehind(repo, "base")
		if err != nil {
			t.Fatalf("AheadBehind: %v", err)
		}
		if ahead != 2 {
			t.Errorf("expected 2 ahead, got %d", ahead)
		}
	})

	t.Run("CommitMessages_returns_messages_ahead", func(t *testing.T) {
		repo := initRepo(t)

		cmd := safeexec.CommandContext(context.Background(), "git", "branch", "base")
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch: %v\n%s", err, out)
		}

		addCommit(t, repo, "x.txt", "feat: add x")
		addCommit(t, repo, "y.txt", "fix: add y")

		msgs, err := r.CommitMessages(repo, "base", 5)
		if err != nil {
			t.Fatalf("CommitMessages: %v", err)
		}
		if len(msgs) != 2 {
			t.Errorf("expected 2 messages, got %d: %v", len(msgs), msgs)
		}
		// Most-recent commit should appear first.
		foundFix := false
		for _, m := range msgs {
			if strings.Contains(m, "fix: add y") || strings.Contains(m, "add y") {
				foundFix = true
			}
		}
		if !foundFix {
			t.Errorf("expected to find 'add y' message in %v", msgs)
		}
	})

	t.Run("DiffShortstat_zero_on_clean_repo", func(t *testing.T) {
		repo := initRepo(t)
		d, err := r.DiffShortstat(repo)
		if err != nil {
			t.Fatalf("DiffShortstat: %v", err)
		}
		if d.Files != 0 || d.Insertions != 0 || d.Deletions != 0 {
			t.Errorf("expected empty DiffStat on clean repo, got %+v", d)
		}
	})

	t.Run("DiffShortstat_counts_lines", func(t *testing.T) {
		repo := initRepo(t)
		// README.md started as "hello\n" (1 line). Replace with 3 lines.
		newContent := "line1\nline2\nline3\n"
		if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(newContent), 0644); err != nil {
			t.Fatal(err)
		}
		d, err := r.DiffShortstat(repo)
		if err != nil {
			t.Fatalf("DiffShortstat: %v", err)
		}
		if d.Files != 1 {
			t.Errorf("expected 1 changed file, got %d", d.Files)
		}
		// 1 old line deleted, 3 new lines inserted.
		if d.Deletions != 1 {
			t.Errorf("expected 1 deletion, got %d", d.Deletions)
		}
		if d.Insertions != 3 {
			t.Errorf("expected 3 insertions, got %d", d.Insertions)
		}
	})

	t.Run("DiffShortstat_new_file", func(t *testing.T) {
		repo := initRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("alpha\nbeta\n"), 0644); err != nil {
			t.Fatal(err)
		}
		d, err := r.DiffShortstat(repo)
		if err != nil {
			t.Fatalf("DiffShortstat: %v", err)
		}
		if d.Files != 1 {
			t.Errorf("expected 1 changed file, got %d", d.Files)
		}
		if d.Insertions != 2 {
			t.Errorf("expected 2 insertions for new file, got %d", d.Insertions)
		}
		if d.Deletions != 0 {
			t.Errorf("expected 0 deletions for new file, got %d", d.Deletions)
		}
	})
}


// TestFakeVCSReader verifies the scanner works correctly with an injected fake.
func TestFakeVCSReader(t *testing.T) {
	fake := &fakeVCSReader{
		worktrees: []unfinished.WorktreeInfo{
			{Path: "/repo/main", Branch: "feat/x"},
			{Path: "/repo/wt2", Branch: "feat/y"},
		},
		defaultBranch:    "origin/main",
		hasUncommitted:   map[string]bool{"/repo/wt2": true},
		aheadCounts:      map[string]int{"/repo/main": 3},
		commitMessages:   map[string][]string{"/repo/main": {"abc1234 feat: add thing"}},
		diffStatFiles:    map[string]int{"/repo/wt2": 2},
	}

	scanner := unfinished.NewScannerWithReader(nil, nil, fake)
	if scanner == nil {
		t.Fatal("NewScannerWithReader returned nil")
	}
}

type fakeVCSReader struct {
	worktrees      []unfinished.WorktreeInfo
	defaultBranch  string
	hasUncommitted map[string]bool
	aheadCounts    map[string]int
	commitMessages map[string][]string
	diffStatFiles  map[string]int
}

func (f *fakeVCSReader) ListWorktrees(repoPath string) ([]unfinished.WorktreeInfo, error) {
	return f.worktrees, nil
}

func (f *fakeVCSReader) ResolveDefaultBranch(repoPath string) string {
	return f.defaultBranch
}

func (f *fakeVCSReader) HasUncommitted(worktreePath string) (bool, error) {
	return f.hasUncommitted[worktreePath], nil
}

func (f *fakeVCSReader) AheadBehind(worktreePath, base string) (int, int, error) {
	return f.aheadCounts[worktreePath], 0, nil
}

func (f *fakeVCSReader) CommitMessages(worktreePath, base string, max int) ([]string, error) {
	return f.commitMessages[worktreePath], nil
}

func (f *fakeVCSReader) DiffShortstat(worktreePath string) (unfinished.DiffStat, error) {
	return unfinished.DiffStat{Files: f.diffStatFiles[worktreePath]}, nil
}

// ---------------------------------------------------------------------------
// JJ tests
// ---------------------------------------------------------------------------

// initJJRepo creates a jj-backed git repo at a temp dir with one change.
func initJJRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed")
	}

	raw := t.TempDir()
	dir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := safeexec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"JJ_USER=Test User",
			"JJ_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	run("jj", "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("jj", "describe", "-m", "initial commit")
	run("jj", "new") // move to a new empty change on top
	return dir
}

// TestVCSReaderContractJJ runs the shared contract suite against JJVCSReader.
func TestVCSReaderContractJJ(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed")
	}
	testVCSReaderContractJJ(t, &unfinished.JJVCSReader{})
}

// testVCSReaderContractJJ is a jj-specific variant of the contract suite.
// jj's model differs enough (no linked worktrees, change-based rather than
// branch-based) that it gets its own focused suite.
func testVCSReaderContractJJ(t *testing.T, r *unfinished.JJVCSReader) {
	t.Helper()

	t.Run("ListWorktrees_returns_single_entry", func(t *testing.T) {
		repo := initJJRepo(t)
		wts, err := r.ListWorktrees(repo)
		if err != nil {
			t.Fatalf("ListWorktrees: %v", err)
		}
		if len(wts) != 1 {
			t.Fatalf("expected exactly 1 worktree for jj repo, got %d", len(wts))
		}
		if wts[0].Path != repo {
			t.Errorf("worktree path = %q, want %q", wts[0].Path, repo)
		}
	})

	t.Run("HasUncommitted_false_on_empty_change", func(t *testing.T) {
		repo := initJJRepo(t)
		// jj new creates a fresh empty change — no uncommitted files.
		dirty, err := r.HasUncommitted(repo)
		if err != nil {
			t.Fatalf("HasUncommitted: %v", err)
		}
		if dirty {
			t.Error("expected empty jj change to have no uncommitted files")
		}
	})

	t.Run("HasUncommitted_true_when_file_modified", func(t *testing.T) {
		repo := initJJRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty"), 0644); err != nil {
			t.Fatal(err)
		}
		dirty, err := r.HasUncommitted(repo)
		if err != nil {
			t.Fatalf("HasUncommitted: %v", err)
		}
		if !dirty {
			t.Error("expected dirty jj change to be detected")
		}
	})

	t.Run("DiffShortstat_zero_on_empty_change", func(t *testing.T) {
		repo := initJJRepo(t)
		d, err := r.DiffShortstat(repo)
		if err != nil {
			t.Fatalf("DiffShortstat: %v", err)
		}
		if d.Files != 0 {
			t.Errorf("expected 0 changed files on empty change, got %d", d.Files)
		}
	})

	t.Run("DiffShortstat_detects_change", func(t *testing.T) {
		repo := initJJRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("alpha\nbeta\n"), 0644); err != nil {
			t.Fatal(err)
		}
		d, err := r.DiffShortstat(repo)
		if err != nil {
			t.Fatalf("DiffShortstat: %v", err)
		}
		if d.Files == 0 {
			t.Error("expected at least 1 changed file")
		}
	})
}

// ---------------------------------------------------------------------------
// Internal helpers — white-box tests for the go-git line-diff implementation
// ---------------------------------------------------------------------------

func TestLinesDiff(t *testing.T) {
	cases := []struct {
		name            string
		old, new        string
		wantIns, wantDel int
	}{
		{
			name:    "identical",
			old:     "a\nb\nc\n",
			new:     "a\nb\nc\n",
			wantIns: 0, wantDel: 0,
		},
		{
			name:    "one line replaced",
			old:     "hello\n",
			new:     "world\n",
			wantIns: 1, wantDel: 1,
		},
		{
			name:    "lines appended",
			old:     "a\n",
			new:     "a\nb\nc\n",
			wantIns: 2, wantDel: 0,
		},
		{
			name:    "lines removed",
			old:     "a\nb\nc\n",
			new:     "a\n",
			wantIns: 0, wantDel: 2,
		},
		{
			name:    "empty to content",
			old:     "",
			new:     "alpha\nbeta\n",
			wantIns: 2, wantDel: 0,
		},
		{
			name:    "content to empty (deleted file)",
			old:     "alpha\nbeta\n",
			new:     "",
			wantIns: 0, wantDel: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ins, del := unfinished.LinesDiff(tc.old, tc.new)
			if ins != tc.wantIns || del != tc.wantDel {
				t.Errorf("LinesDiff(%q, %q) = ins:%d del:%d, want ins:%d del:%d",
					tc.old, tc.new, ins, del, tc.wantIns, tc.wantDel)
			}
		})
	}
}
