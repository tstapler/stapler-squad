package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestSanitizeBranchName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple lowercase string",
			input:    "feature",
			expected: "feature",
		},
		{
			name:     "string with spaces",
			input:    "new feature branch",
			expected: "new-feature-branch",
		},
		{
			name:     "mixed case string",
			input:    "FeAtUrE BrAnCh",
			expected: "feature-branch",
		},
		{
			name:     "string with special characters",
			input:    "feature!@#$%^&*()",
			expected: "feature",
		},
		{
			name:     "string with allowed special characters",
			input:    "feature/sub_branch.v1",
			expected: "feature/sub_branch.v1",
		},
		{
			name:     "string with multiple dashes",
			input:    "feature---branch",
			expected: "feature-branch",
		},
		{
			name:     "string with leading and trailing dashes",
			input:    "-feature-branch-",
			expected: "feature-branch",
		},
		{
			name:     "string with leading and trailing slashes",
			input:    "/feature/branch/",
			expected: "feature/branch",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "complex mixed case with special chars",
			input:    "USER/Feature Branch!@#$%^&*()/v1.0",
			expected: "user/feature-branch/v1.0",
		},
		{
			name:     "path traversal with multiple parent dirs is stripped",
			input:    "../../../etc",
			expected: "etc",
		},
		{
			name:     "path traversal mixed with valid segments is stripped",
			input:    "foo/../../bar",
			expected: "foo/bar",
		},
		{
			name:     "dot-prefixed segment is not traversal and is preserved",
			input:    "..hidden",
			expected: "..hidden",
		},
		{
			name:     "all-traversal input falls back to a safe non-empty name",
			input:    "..",
			expected: "session",
		},
		{
			name:     "triple-dot segment is not exact-match traversal and is preserved",
			input:    "...",
			expected: "...",
		},
		{
			name:     "dotted version-like name is preserved",
			input:    "release/v1.2.3",
			expected: "release/v1.2.3",
		},
		{
			name:     "double-dot in the middle of a segment is preserved",
			input:    "user..name",
			expected: "user..name",
		},
		{
			name:     "lone slash falls back to a safe non-empty name",
			input:    "/",
			expected: "session",
		},
		{
			name:     "all-dashes falls back to a safe non-empty name",
			input:    "---",
			expected: "session",
		},
		{
			name:     "all-disallowed-characters falls back to a safe non-empty name",
			input:    "!!!",
			expected: "session",
		},
		{
			name:     "triple-dot embedded in a segment is preserved",
			input:    "a...b",
			expected: "a...b",
		},
		{
			name:     "leading traversal segment with slash is stripped",
			input:    "/../repo",
			expected: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeBranchName(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeBranchName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestSanitizeBranchName_StaysWithinWorktreeDir verifies that
// filepath.Join(worktreeDir, sanitizeBranchName(input)) can never escape
// worktreeDir, for malicious inputs designed to traverse out of it.
func TestSanitizeBranchName_StaysWithinWorktreeDir(t *testing.T) {
	t.Parallel()
	worktreeDir := "/var/lib/stapler-squad/worktrees"

	maliciousInputs := []string{
		"../../../etc",
		"foo/../../bar",
		"..",
		"...",
		"../../../../../../etc/passwd",
		"/../../etc",
		"..hidden",
		"user..name",
	}

	for _, input := range maliciousInputs {
		t.Run(input, func(t *testing.T) {
			sanitized := sanitizeBranchName(input)
			joined := filepath.Join(worktreeDir, sanitized)

			rel, err := filepath.Rel(worktreeDir, joined)
			if err != nil {
				t.Fatalf("filepath.Rel(%q, %q) failed: %v", worktreeDir, joined, err)
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Errorf("sanitizeBranchName(%q) = %q; filepath.Join(%q, ...) = %q escapes worktreeDir (rel = %q)",
					input, sanitized, worktreeDir, joined, rel)
			}
		})
	}
}

// TestJoinWithinDir verifies the defense-in-depth boundary check used at every
// filepath.Join(worktreeDir, sanitizedName) call site: it must return an error
// (never a path) for any name that would resolve outside baseDir.
func TestJoinWithinDir(t *testing.T) {
	t.Parallel()
	baseDir := "/var/lib/stapler-squad/worktrees"

	t.Run("normal name stays within baseDir", func(t *testing.T) {
		got, err := joinWithinDir(baseDir, "my-feature")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(baseDir, "my-feature")
		if got != want {
			t.Errorf("joinWithinDir(%q, %q) = %q, want %q", baseDir, "my-feature", got, want)
		}
	})

	escapingNames := []string{
		"..",
		"../escaped",
		"../../etc/passwd",
	}
	for _, name := range escapingNames {
		t.Run(name, func(t *testing.T) {
			got, err := joinWithinDir(baseDir, name)
			if err == nil {
				t.Errorf("joinWithinDir(%q, %q) = (%q, nil), want an error", baseDir, name, got)
			}
		})
	}
}

// TestCanonicalizeWorktreePath_NonexistentPath verifies AC2: a path that doesn't
// exist on disk (the pre-`git worktree add` case for a freshly-computed
// worktreePath) must never error or panic — EvalSymlinks fails with ENOENT here,
// and the function must fall back to filepath.Clean rather than propagate that.
func TestCanonicalizeWorktreePath_NonexistentPath(t *testing.T) {
	t.Parallel()
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist", "leaf_1234")
	got := CanonicalizeWorktreePath(nonexistent)
	want := filepath.Clean(nonexistent)
	if got != want {
		t.Errorf("CanonicalizeWorktreePath(%q) = %q, want %q (filepath.Clean fallback)", nonexistent, got, want)
	}
}

// TestCanonicalizeWorktreePath_EmptyString verifies the empty-string short-circuit
// returns the input unchanged rather than resolving cwd (EvalSymlinks("") behavior
// is platform-dependent and not what any caller here wants).
func TestCanonicalizeWorktreePath_EmptyString(t *testing.T) {
	t.Parallel()
	if got := CanonicalizeWorktreePath(""); got != "" {
		t.Errorf("CanonicalizeWorktreePath(\"\") = %q, want empty string", got)
	}
}

// TestCanonicalizeWorktreePath_ResolvesSymlink verifies the real-path case: a
// symlinked directory canonicalizes to its resolved target, matching what git
// itself reports via `git worktree list --porcelain`.
func TestCanonicalizeWorktreePath_ResolvesSymlink(t *testing.T) {
	t.Parallel()
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link-to-real")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported on this platform: %v", err)
	}

	resolvedReal, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatalf("failed to resolve real dir: %v", err)
	}

	got := CanonicalizeWorktreePath(link)
	if got != resolvedReal {
		t.Errorf("CanonicalizeWorktreePath(%q) = %q, want %q", link, got, resolvedReal)
	}
}

// TestPreviewWorktreePath_RejectsTraversal exercises PreviewWorktreePath end-to-end
// against a real git repository, confirming a malicious sessionName can never
// produce a path outside the configured worktree directory (AC5).
func TestPreviewWorktreePath_RejectsTraversal(t *testing.T) {
	repoDir := t.TempDir()
	if _, err := git.PlainInit(repoDir, false); err != nil {
		t.Fatalf("failed to init test repo: %v", err)
	}

	configDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", configDir)
	worktreeDir := filepath.Join(configDir, "worktrees")
	// getWorktreeDirectory() creates this directory and resolves symlinks on it
	// (see worktree.go) so worktree-reuse identity checks aren't fooled by macOS's
	// /var/folders -> /private/var/folders symlink. Mirror that here or this
	// comparison compares an unresolved path against PreviewWorktreePath's
	// resolved return value and spuriously fails on macOS.
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatalf("failed to create worktree dir: %v", err)
	}
	resolvedWorktreeDir, err := filepath.EvalSymlinks(worktreeDir)
	if err != nil {
		t.Fatalf("failed to resolve worktree dir: %v", err)
	}
	worktreeDir = resolvedWorktreeDir

	maliciousInputs := []string{
		"../../../etc",
		"foo/../../bar",
		"..",
		"../../../../../../etc/passwd",
	}

	for _, input := range maliciousInputs {
		t.Run(input, func(t *testing.T) {
			path, err := PreviewWorktreePath(repoDir, input)
			// sanitizeBranchName is deterministic and already strips every traversal
			// segment (verified independently in TestSanitizeBranchName), so
			// joinWithinDir's escape check can never actually fire for these inputs —
			// assert the exact expected outcome rather than accepting either "errored"
			// or "resolved safely", so a regression in either function is caught.
			wantSanitized := sanitizeBranchName(input)
			wantPath := filepath.Join(worktreeDir, wantSanitized)
			if err != nil {
				t.Fatalf("PreviewWorktreePath(%q, %q) returned unexpected error: %v (want path %q)",
					repoDir, input, err, wantPath)
			}
			if path != wantPath {
				t.Errorf("PreviewWorktreePath(%q, %q) = %q, want %q", repoDir, input, path, wantPath)
			}

			rel, relErr := filepath.Rel(worktreeDir, path)
			if relErr != nil {
				t.Fatalf("filepath.Rel(%q, %q) failed: %v", worktreeDir, path, relErr)
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Errorf("PreviewWorktreePath(%q, %q) = %q, which escapes worktreeDir %q (rel = %q)",
					repoDir, input, path, worktreeDir, rel)
			}
		})
	}
}

// commitRealFile creates repoDir/name with content and commits it, returning the
// commit hash. Used to give a test repo genuine, distinguishable history (as
// opposed to createInitialCommit's own ".gitignore" content, which would make a
// test unable to tell "untouched" from "recreated identically" apart).
func commitRealFile(t *testing.T, repoDir, name, content, message string) string {
	t.Helper()
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		t.Fatalf("failed to open repo at %s: %v", repoDir, err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
	if _, err := worktree.Add(name); err != nil {
		t.Fatalf("failed to add %s: %v", name, err)
	}
	hash, err := worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@localhost", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
	return hash.String()
}

// breakHeadReference rewrites repoDir/.git/HEAD to point at a branch that does
// not exist, reproducing the go-git failure this bug hinged on: repo.Head()
// fails to resolve even though repo.References() still enumerates the repo's
// real refs — exactly what findGitRepoRoot previously misdiagnosed as unborn.
func breakHeadReference(t *testing.T, repoDir string) {
	t.Helper()
	headPath := filepath.Join(repoDir, ".git", "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/does-not-exist\n"), 0o644); err != nil {
		t.Fatalf("failed to corrupt HEAD: %v", err)
	}
}

func TestRepoHasAnyRef(t *testing.T) {
	t.Run("fresh repo has no refs", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := git.PlainInit(dir, false); err != nil {
			t.Fatalf("failed to init repo: %v", err)
		}
		hasRef, err := repoHasAnyRef(dir)
		if err != nil {
			t.Fatalf("repoHasAnyRef() error = %v", err)
		}
		if hasRef {
			t.Errorf("repoHasAnyRef() = true for a freshly-initialized repo, want false")
		}
	})

	t.Run("repo with a real commit has a ref", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := git.PlainInit(dir, false); err != nil {
			t.Fatalf("failed to init repo: %v", err)
		}
		commitRealFile(t, dir, "real.txt", "real content\n", "real commit")

		hasRef, err := repoHasAnyRef(dir)
		if err != nil {
			t.Fatalf("repoHasAnyRef() error = %v", err)
		}
		if !hasRef {
			t.Errorf("repoHasAnyRef() = false for a repo with a real commit, want true")
		}
	})

	t.Run("repo with real history is detected even when HEAD fails to resolve", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := git.PlainInit(dir, false); err != nil {
			t.Fatalf("failed to init repo: %v", err)
		}
		commitRealFile(t, dir, "real.txt", "real content\n", "real commit")
		breakHeadReference(t, dir)

		hasRef, err := repoHasAnyRef(dir)
		if err != nil {
			t.Fatalf("repoHasAnyRef() error = %v", err)
		}
		if !hasRef {
			t.Errorf("repoHasAnyRef() = false despite a real ref existing, want true")
		}
	})
}

// TestCreateInitialCommit_FreshRepo_Succeeds is the first direct test of
// createInitialCommit: the legitimate case (a freshly git.PlainInit'd repo,
// zero refs) must still succeed unchanged by the new repoHasAnyRef guard.
func TestCreateInitialCommit_FreshRepo_Succeeds(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	if err := createInitialCommit(repo, dir); err != nil {
		t.Fatalf("createInitialCommit() error = %v", err)
	}

	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(gitignore) != "# Project gitignore\n" {
		t.Errorf(".gitignore content = %q, want %q", gitignore, "# Project gitignore\n")
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("repo.Head() error = %v", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("repo.CommitObject() error = %v", err)
	}
	if commit.Message != "Initial commit" {
		t.Errorf("HEAD commit message = %q, want %q", commit.Message, "Initial commit")
	}
}

// TestCreateInitialCommit_RefusesWhenRepoAlreadyHasRefs is the regression test for
// the self-defending guard: a repo with real, pre-existing history must never
// have its .gitignore overwritten or a fabricated commit added, regardless of
// which caller reaches createInitialCommit.
func TestCreateInitialCommit_RefusesWhenRepoAlreadyHasRefs(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	commitRealFile(t, dir, "real.txt", "real content\n", "real commit")

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatalf("failed to seed .gitignore: %v", err)
	}

	err = createInitialCommit(repo, dir)
	if err == nil {
		t.Fatalf("createInitialCommit() error = nil, want an error refusing to run against a repo with existing refs")
	}

	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(gitignore) != "node_modules/\n" {
		t.Errorf(".gitignore content = %q, want unchanged %q", gitignore, "node_modules/\n")
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("repo.Head() error = %v", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("repo.CommitObject() error = %v", err)
	}
	if commit.Message != "real commit" {
		t.Errorf("HEAD commit message = %q, want unchanged %q (no fabricated commit should have been added)", commit.Message, "real commit")
	}
	if commit.NumParents() != 0 {
		t.Errorf("HEAD commit has %d parents, want 0 (no fabricated commit should have been added on top)", commit.NumParents())
	}
}

// TestFindGitRepoRoot_FreshRepo_CreatesInitialCommit is the happy-path
// regression test: a genuinely unborn repo must still get its initial commit.
func TestFindGitRepoRoot_FreshRepo_CreatesInitialCommit(t *testing.T) {
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	root, err := findGitRepoRoot(dir)
	if err != nil {
		t.Fatalf("findGitRepoRoot() error = %v", err)
	}
	if root != dir {
		t.Errorf("findGitRepoRoot() = %q, want %q", root, dir)
	}

	if _, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err != nil {
		t.Errorf("expected .gitignore to be created for an unborn repo: %v", err)
	}
}

// TestFindGitRepoRoot_ExistingRepoWithHeadResolutionFailure_DoesNotCorruptRepo is
// the direct regression test for this bug: a repo with real history whose
// repo.Head() transiently/incorrectly fails to resolve must NOT have its
// .gitignore overwritten or a fabricated "Initial commit" added on top of real
// history — findGitRepoRoot must instead trust repoHasAnyRef's ref-store check.
func TestFindGitRepoRoot_ExistingRepoWithHeadResolutionFailure_DoesNotCorruptRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	realCommit := commitRealFile(t, dir, "real.txt", "real content\n", "real commit")

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatalf("failed to seed .gitignore: %v", err)
	}
	breakHeadReference(t, dir)

	root, err := findGitRepoRoot(dir)
	if err != nil {
		t.Fatalf("findGitRepoRoot() error = %v, want success (repo has real history, should not be touched)", err)
	}
	if root != dir {
		t.Errorf("findGitRepoRoot() = %q, want %q", root, dir)
	}

	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(gitignore) != "node_modules/\n" {
		t.Errorf(".gitignore content = %q, want untouched %q", gitignore, "node_modules/\n")
	}

	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("failed to reopen repo: %v", err)
	}
	commit, err := repo.CommitObject(plumbing.NewHash(realCommit))
	if err != nil {
		t.Fatalf("original commit %s no longer resolves: %v", realCommit, err)
	}
	if commit.Message != "real commit" {
		t.Errorf("original commit message = %q, want %q", commit.Message, "real commit")
	}
}

// TestInitializeProjectDirectory_should_NotOverwriteGitignore_When_CalledTwice
// locks in InitializeProjectDirectory's existing PlainOpen short-circuit
// (util.go step 1) so it can't regress alongside the new repoHasAnyRef guard:
// calling it a second time against an already-initialized project must be a
// pure no-op, not a second createInitialCommit run.
func TestInitializeProjectDirectory_should_NotOverwriteGitignore_When_CalledTwice(t *testing.T) {
	dir := t.TempDir()

	if err := InitializeProjectDirectory(dir); err != nil {
		t.Fatalf("first InitializeProjectDirectory() error = %v", err)
	}

	gitignoreBefore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore after first call: %v", err)
	}
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("failed to open repo after first call: %v", err)
	}
	headBefore, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to resolve HEAD after first call: %v", err)
	}

	if err := InitializeProjectDirectory(dir); err != nil {
		t.Fatalf("second InitializeProjectDirectory() error = %v", err)
	}

	gitignoreAfter, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore after second call: %v", err)
	}
	if string(gitignoreAfter) != string(gitignoreBefore) {
		t.Errorf(".gitignore content changed after second call: got %q, want unchanged %q", gitignoreAfter, gitignoreBefore)
	}

	repo2, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("failed to reopen repo after second call: %v", err)
	}
	headAfter, err := repo2.Head()
	if err != nil {
		t.Fatalf("failed to resolve HEAD after second call: %v", err)
	}
	if headAfter.Hash() != headBefore.Hash() {
		t.Errorf("HEAD changed after second call: got %s, want unchanged %s", headAfter.Hash(), headBefore.Hash())
	}
}
