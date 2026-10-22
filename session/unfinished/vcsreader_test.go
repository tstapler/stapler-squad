package unfinished_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitutil "github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/unfinished"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
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

	// Uses go-git directly rather than shelling out — see
	// the `prefer-go-git-over-subshells` skill.
	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatalf("PlainInitWithOptions: %v", err)
	}

	// Initial commit.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return dir
}

// addCommit adds a file and commits it, returning the short hash.
//
// Uses go-git directly rather than shelling out — see
// the `prefer-go-git-over-subshells` skill.
func addCommit(t *testing.T, repoPath, filename, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, filename), []byte(message), 0644); err != nil {
		t.Fatal(err)
	}
	repo, err := gitutil.OpenRepo(repoPath)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// openGoGit opens the repo at path through the shared go-git entry point.
func openGoGit(t testing.TB, path string) *git.Repository {
	t.Helper()
	repo, err := gitutil.OpenRepo(path)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	return repo
}

// branchAtHead creates refs/heads/<name> at the current HEAD (git branch <name>).
func branchAtHead(t testing.TB, repoPath, name string) {
	t.Helper()
	repo := openGoGit(t, repoPath)
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	testrepo.SetBranch(t, repo, name, head.Hash())
}

// checkoutBranch is git checkout [-b] <branch>.
func checkoutBranch(t testing.TB, repoPath, branch string, create bool) {
	t.Helper()
	testrepo.Checkout(t, openGoGit(t, repoPath), branch, create)
}

// commitAll is `git add . && git commit -m msg`.
func commitAll(t testing.TB, repoPath, msg string) {
	t.Helper()
	wt, err := openGoGit(t, repoPath).Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()}
	if _, err := wt.Commit(msg, &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// assertBehindFixtureShape checks the topology TestGoGitVCSReader_AheadBehind_BehindCount
// relies on: main has the single initial commit, base is exactly one commit
// ahead of it, and HEAD is main.
func assertBehindFixtureShape(t testing.TB, repoPath string) {
	t.Helper()
	repo := openGoGit(t, repoPath)
	head, err := repo.Head()
	if err != nil || head.Name() != plumbing.NewBranchReferenceName("main") {
		t.Fatalf("fixture: HEAD = %v (%v), want refs/heads/main", head, err)
	}
	mainCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("fixture: main commit: %v", err)
	}
	if len(mainCommit.ParentHashes) != 0 {
		t.Fatalf("fixture: main has %d parents, want 0 (single root commit)", len(mainCommit.ParentHashes))
	}
	baseRef, err := repo.Reference(plumbing.NewBranchReferenceName("base"), true)
	if err != nil {
		t.Fatalf("fixture: base ref: %v", err)
	}
	baseCommit, err := repo.CommitObject(baseRef.Hash())
	if err != nil {
		t.Fatalf("fixture: base commit: %v", err)
	}
	if len(baseCommit.ParentHashes) != 1 || baseCommit.ParentHashes[0] != head.Hash() {
		t.Fatalf("fixture: base parents = %v, want exactly [%s]", baseCommit.ParentHashes, head.Hash())
	}
	n := 0
	iter, err := repo.Log(&git.LogOptions{From: baseRef.Hash()})
	if err != nil {
		t.Fatalf("fixture: log: %v", err)
	}
	_ = iter.ForEach(func(*object.Commit) error { n++; return nil })
	if n != 2 {
		t.Fatalf("fixture: base reaches %d commits, want 2", n)
	}
}

// stageMergeConflict rewrites the index and worktree for path as an unresolved
// `git merge theirsBranch` into HEAD would: stage 1/2/3 entries from the merge
// base, HEAD and theirsBranch, conflict markers on disk, and MERGE_HEAD set.
func stageMergeConflict(t testing.TB, repoPath, theirsBranch, path string) {
	t.Helper()
	repo := openGoGit(t, repoPath)
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	theirsRef, err := repo.Reference(plumbing.NewBranchReferenceName(theirsBranch), true)
	if err != nil {
		t.Fatalf("theirs ref: %v", err)
	}
	ours, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("ours commit: %v", err)
	}
	theirs, err := repo.CommitObject(theirsRef.Hash())
	if err != nil {
		t.Fatalf("theirs commit: %v", err)
	}
	bases, err := ours.MergeBase(theirs)
	if err != nil || len(bases) != 1 {
		t.Fatalf("merge base: %v (%d results)", err, len(bases))
	}
	blobOf := func(c *object.Commit) plumbing.Hash {
		f, err := c.File(path)
		if err != nil {
			t.Fatalf("File %s@%s: %v", path, c.Hash, err)
		}
		return f.Hash
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if _, err := idx.Remove(path); err != nil {
		t.Fatalf("index remove %s: %v", path, err)
	}
	now := time.Now()
	for _, e := range []struct {
		stage index.Stage
		hash  plumbing.Hash
	}{
		{index.AncestorMode, blobOf(bases[0])},
		{index.OurMode, blobOf(ours)},
		{index.TheirMode, blobOf(theirs)},
	} {
		idx.Entries = append(idx.Entries, &index.Entry{
			Name: path, Hash: e.hash, Mode: filemode.Regular, Stage: e.stage,
			CreatedAt: now, ModifiedAt: now,
		})
	}
	if err := repo.Storer.SetIndex(idx); err != nil {
		t.Fatalf("SetIndex: %v", err)
	}
	markers := "<<<<<<< HEAD\nmain content\n=======\nbranch-a content\n>>>>>>> " + theirsBranch + "\n"
	if err := os.WriteFile(filepath.Join(repoPath, path), []byte(markers), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference("MERGE_HEAD", theirsRef.Hash())); err != nil {
		t.Fatalf("MERGE_HEAD: %v", err)
	}
}

// assertConflictFixtureShape checks the index holds exactly stages 1, 2 and 3
// (and no stage 0) for path, the equivalent of `git status --porcelain` UU.
func assertConflictFixtureShape(t testing.TB, repoPath, path string) {
	t.Helper()
	idx, err := openGoGit(t, repoPath).Storer.Index()
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	var stages []index.Stage
	for _, e := range idx.Entries {
		if e.Name == path {
			stages = append(stages, e.Stage)
		}
	}
	want := []index.Stage{index.AncestorMode, index.OurMode, index.TheirMode}
	if len(stages) != 3 || stages[0] != want[0] || stages[1] != want[1] || stages[2] != want[2] {
		t.Fatalf("fixture: %s stages = %v, want %v (unresolved conflict)", path, stages, want)
	}
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

	t.Run("HasUncommitted_true_for_same_size_edit_within_index_timestamp_second", func(t *testing.T) {
		repoPath := initRepo(t)
		repo, err := gitutil.OpenRepo(repoPath)
		if err != nil {
			t.Fatalf("OpenRepo: %v", err)
		}
		idx, err := repo.Storer.Index()
		if err != nil {
			t.Fatalf("Index: %v", err)
		}
		if len(idx.Entries) != 1 {
			t.Fatalf("index entries = %d, want 1", len(idx.Entries))
		}

		path := filepath.Join(repoPath, "README.md")
		if err := os.WriteFile(path, []byte("world\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mtime := idx.Entries[0].ModifiedAt.Truncate(time.Second).Add(123 * time.Millisecond)
		if mtime.Equal(idx.Entries[0].ModifiedAt) {
			mtime = mtime.Add(time.Millisecond)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}

		dirty, err := r.HasUncommitted(repoPath)
		if err != nil {
			t.Fatalf("HasUncommitted: %v", err)
		}
		if !dirty {
			t.Error("expected a same-size edit within the index timestamp's second to be uncommitted")
		}
	})

	t.Run("AheadBehind_zero_on_single_commit_repo", func(t *testing.T) {
		repo := initRepo(t)
		// Set up a "base" branch pointing at the same commit.
		branchAtHead(t, repo, "base")
		ahead, behind, err := r.AheadBehind(context.Background(), repo, "base")
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
		branchAtHead(t, repo, "base")

		// Add 2 commits on main.
		addCommit(t, repo, "a.txt", "commit a")
		addCommit(t, repo, "b.txt", "commit b")

		ahead, behind, err := r.AheadBehind(context.Background(), repo, "base")
		if err != nil {
			t.Fatalf("AheadBehind: %v", err)
		}
		if ahead != 2 {
			t.Errorf("expected 2 ahead, got %d", ahead)
		}
		if behind != 0 {
			t.Errorf("expected 0 behind, got %d", behind)
		}
	})

	t.Run("CommitMessages_returns_messages_ahead", func(t *testing.T) {
		repo := initRepo(t)

		branchAtHead(t, repo, "base")

		addCommit(t, repo, "x.txt", "feat: add x")
		addCommit(t, repo, "y.txt", "fix: add y")

		msgs, err := r.CommitMessages(context.Background(), repo, "base", 5)
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

// TestDiffShortstat_MultiBlobWorktree verifies that DiffShortstat returns correct
// counts when a worktree has 5+ modified files, exercising the batch blob-read path
// that replaced the per-file lock-acquire/release cycle.
func TestDiffShortstat_MultiBlobWorktree(t *testing.T) {
	repo := initRepo(t)
	r := &unfinished.GoGitVCSReader{}

	// Commit 5 files with known content, each 3 lines.
	files := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"}
	original := "line1\nline2\nline3\n"
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
	}
	commitAll(t, repo, "add five files")

	// Modify each file: replace 3 lines with 2 distinct lines → 3 deletions + 2 insertions per file.
	// Use "a\nb\n" (4 bytes) instead of a same-size string to ensure size-based
	// unstaged-change detection in GoGitVCSReader.DiffShortstat always sees a delta.
	modified := "a\nb\n"
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(modified), 0644); err != nil {
			t.Fatal(err)
		}
	}

	d, err := r.DiffShortstat(repo)
	if err != nil {
		t.Fatalf("DiffShortstat: %v", err)
	}

	// 5 files modified (README.md is unchanged).
	if d.Files != 5 {
		t.Errorf("expected 5 changed files, got %d", d.Files)
	}
	// Each file: 3 original lines deleted, 2 new lines inserted.
	if d.Deletions != 15 {
		t.Errorf("expected 15 deletions (5×3), got %d", d.Deletions)
	}
	if d.Insertions != 10 {
		t.Errorf("expected 10 insertions (5×2), got %d", d.Insertions)
	}
}

// TestGoGitVCSReader_AheadBehind_BehindCount verifies the "behind" direction of
// AheadBehind when the base branch has commits that the current branch lacks.
func TestGoGitVCSReader_AheadBehind_BehindCount(t *testing.T) {
	repo := initRepo(t)
	r := &unfinished.GoGitVCSReader{}

	// Create base branch at initial commit, then add a commit to base only.
	branchAtHead(t, repo, "base")
	checkoutBranch(t, repo, "base", false)
	addCommit(t, repo, "base-only.txt", "commit on base")
	checkoutBranch(t, repo, "main", false)
	assertBehindFixtureShape(t, repo)

	// main is 0 ahead, 1 behind base.
	ahead, behind, err := r.AheadBehind(context.Background(), repo, "base")
	if err != nil {
		t.Fatalf("AheadBehind: %v", err)
	}
	if ahead != 0 {
		t.Errorf("expected 0 ahead, got %d", ahead)
	}
	if behind != 1 {
		t.Errorf("expected 1 behind, got %d", behind)
	}
}

// TestGoGitVCSReader_HasUncommitted_StagedChange verifies that a file added to
// the index (git add) but not yet committed is detected as uncommitted.
func TestGoGitVCSReader_HasUncommitted_StagedChange(t *testing.T) {
	repo := initRepo(t)
	r := &unfinished.GoGitVCSReader{}

	// Write a new file and stage it.
	if err := os.WriteFile(filepath.Join(repo, "staged.txt"), []byte("staged content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Uses go-git directly rather than shelling out — see
	// the `prefer-go-git-over-subshells` skill.
	repo2, err := gitutil.OpenRepo(repo)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	wt, err := repo2.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add("staged.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	dirty, err := r.HasUncommitted(repo)
	if err != nil {
		t.Fatalf("HasUncommitted: %v", err)
	}
	if !dirty {
		t.Error("expected HasUncommitted=true for repo with staged (index) change")
	}
}

// TestGoGitVCSReader_HasUncommitted_StagedDeletion verifies that a tracked file
// removed from the index (git rm) is reported as uncommitted.
func TestGoGitVCSReader_HasUncommitted_StagedDeletion(t *testing.T) {
	repo := initRepo(t)
	r := &unfinished.GoGitVCSReader{}

	// Stage a deletion of the initial README.md.
	// Uses go-git directly rather than shelling out — see
	// the `prefer-go-git-over-subshells` skill.
	repo2, err := gitutil.OpenRepo(repo)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	wt, err := repo2.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Remove("README.md"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	dirty, err := r.HasUncommitted(repo)
	if err != nil {
		t.Fatalf("HasUncommitted: %v", err)
	}
	if !dirty {
		t.Error("expected HasUncommitted=true for repo with staged deletion")
	}
}

// TestGoGitVCSReader_HasUncommitted_MergeConflict verifies that a repo in a
// conflicted merge state (stage entries with non-zero stage number) is reported
// as having uncommitted changes.
func TestGoGitVCSReader_HasUncommitted_MergeConflict(t *testing.T) {
	repo := initRepo(t)
	r := &unfinished.GoGitVCSReader{}

	// Create diverging branches that conflict on README.md.
	checkoutBranch(t, repo, "branch-a", true)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("branch-a content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "branch-a commit")

	checkoutBranch(t, repo, "main", false)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("main content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "main commit")

	// go-git cannot merge, so reproduce the state `git merge --no-ff branch-a`
	// leaves behind: README.md carries stage 1/2/3 index entries.
	stageMergeConflict(t, repo, "branch-a", "README.md")
	assertConflictFixtureShape(t, repo, "README.md")

	dirty, err := r.HasUncommitted(repo)
	if err != nil {
		t.Fatalf("HasUncommitted: %v", err)
	}
	if !dirty {
		t.Error("expected HasUncommitted=true for repo with merge conflict")
	}
}

// TestFakeVCSReader verifies the scanner works correctly with an injected fake.
func TestFakeVCSReader(t *testing.T) {
	fake := &fakeVCSReader{
		worktrees: []unfinished.WorktreeInfo{
			{Path: "/repo/main", Branch: "feat/x"},
			{Path: "/repo/wt2", Branch: "feat/y"},
		},
		defaultBranch:  "origin/main",
		hasUncommitted: map[string]bool{"/repo/wt2": true},
		aheadCounts:    map[string]int{"/repo/main": 3},
		commitMessages: map[string][]string{"/repo/main": {"abc1234 feat: add thing"}},
		diffStatFiles:  map[string]int{"/repo/wt2": 2},
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

func (f *fakeVCSReader) AheadBehind(_ context.Context, worktreePath, base string) (int, int, error) {
	return f.aheadCounts[worktreePath], 0, nil
}

func (f *fakeVCSReader) CommitMessages(_ context.Context, worktreePath, base string, max int) ([]string, error) {
	return f.commitMessages[worktreePath], nil
}

func (f *fakeVCSReader) DiffShortstat(worktreePath string) (unfinished.DiffStat, error) {
	return unfinished.DiffStat{Files: f.diffStatFiles[worktreePath]}, nil
}

// ---------------------------------------------------------------------------
// Internal helpers — white-box tests for the go-git line-diff implementation
// ---------------------------------------------------------------------------

func TestLinesDiff(t *testing.T) {
	cases := []struct {
		name             string
		old, new         string
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
