package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
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
