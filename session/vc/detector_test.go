package vc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectVCS(t *testing.T) {
	// Test with non-existent path
	t.Run("non-existent path", func(t *testing.T) {
		result := DetectVCS("/non/existent/path/that/should/not/exist")
		if result != VCSUnknown {
			t.Errorf("DetectVCS() = %v, want %v for non-existent path", result, VCSUnknown)
		}
	})

	// Test with temporary directories
	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		result := DetectVCS(tmpDir)
		if result != VCSUnknown {
			t.Errorf("DetectVCS() = %v, want %v for empty directory", result, VCSUnknown)
		}
	})

	t.Run("git repository", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		if err := os.Mkdir(gitDir, 0755); err != nil {
			t.Fatalf("Failed to create .git directory: %v", err)
		}

		result := DetectVCS(tmpDir)
		if result != VCSGit {
			t.Errorf("DetectVCS() = %v, want %v for git repository", result, VCSGit)
		}
	})

	t.Run("jujutsu repository", func(t *testing.T) {
		tmpDir := t.TempDir()
		jjDir := filepath.Join(tmpDir, ".jj")
		if err := os.Mkdir(jjDir, 0755); err != nil {
			t.Fatalf("Failed to create .jj directory: %v", err)
		}

		result := DetectVCS(tmpDir)
		if result != VCSJujutsu {
			t.Errorf("DetectVCS() = %v, want %v for jujutsu repository", result, VCSJujutsu)
		}
	})

	t.Run("jujutsu takes precedence over git", func(t *testing.T) {
		// When both .jj and .git exist, jujutsu should be detected
		// (jj colocated repos have both)
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		jjDir := filepath.Join(tmpDir, ".jj")
		if err := os.Mkdir(gitDir, 0755); err != nil {
			t.Fatalf("Failed to create .git directory: %v", err)
		}
		if err := os.Mkdir(jjDir, 0755); err != nil {
			t.Fatalf("Failed to create .jj directory: %v", err)
		}

		result := DetectVCS(tmpDir)
		if result != VCSJujutsu {
			t.Errorf("DetectVCS() = %v, want %v when both .jj and .git exist", result, VCSJujutsu)
		}
	})
}

func TestIsGitRepo(t *testing.T) {
	t.Run("not a git repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		if isGitRepo(tmpDir) {
			t.Error("isGitRepo() = true, want false for empty directory")
		}
	})

	t.Run("is a git repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		if err := os.Mkdir(gitDir, 0755); err != nil {
			t.Fatalf("Failed to create .git directory: %v", err)
		}

		if !isGitRepo(tmpDir) {
			t.Error("isGitRepo() = false, want true for git repository")
		}
	})

	t.Run("git file reference", func(t *testing.T) {
		// Git worktrees have a .git file instead of directory
		tmpDir := t.TempDir()
		gitFile := filepath.Join(tmpDir, ".git")
		if err := os.WriteFile(gitFile, []byte("gitdir: /some/path/.git/worktrees/test"), 0644); err != nil {
			t.Fatalf("Failed to create .git file: %v", err)
		}

		if !isGitRepo(tmpDir) {
			t.Error("isGitRepo() = false, want true for git worktree (file)")
		}
	})
}

func TestIsJujutsuRepo(t *testing.T) {
	t.Run("not a jujutsu repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		if isJujutsuRepo(tmpDir) {
			t.Error("isJujutsuRepo() = true, want false for empty directory")
		}
	})

	t.Run("is a jujutsu repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		jjDir := filepath.Join(tmpDir, ".jj")
		if err := os.Mkdir(jjDir, 0755); err != nil {
			t.Fatalf("Failed to create .jj directory: %v", err)
		}

		if !isJujutsuRepo(tmpDir) {
			t.Error("isJujutsuRepo() = false, want true for jujutsu repository")
		}
	})
}

func TestFindVCSRoot(t *testing.T) {
	t.Run("find git root from subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		if err := os.Mkdir(gitDir, 0755); err != nil {
			t.Fatalf("Failed to create .git directory: %v", err)
		}

		// Create nested subdirectory
		subDir := filepath.Join(tmpDir, "src", "pkg", "module")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirectory: %v", err)
		}

		root, err := FindVCSRoot(subDir, VCSGit)
		if err != nil {
			t.Errorf("FindVCSRoot() error = %v, want nil", err)
		}
		if root != tmpDir {
			t.Errorf("FindVCSRoot() = %v, want %v", root, tmpDir)
		}
	})

	t.Run("find jujutsu root from subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		jjDir := filepath.Join(tmpDir, ".jj")
		if err := os.Mkdir(jjDir, 0755); err != nil {
			t.Fatalf("Failed to create .jj directory: %v", err)
		}

		// Create nested subdirectory
		subDir := filepath.Join(tmpDir, "src", "pkg")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirectory: %v", err)
		}

		root, err := FindVCSRoot(subDir, VCSJujutsu)
		if err != nil {
			t.Errorf("FindVCSRoot() error = %v, want nil", err)
		}
		if root != tmpDir {
			t.Errorf("FindVCSRoot() = %v, want %v", root, tmpDir)
		}
	})

	t.Run("no vcs root found", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := FindVCSRoot(tmpDir, VCSGit)
		if err == nil {
			t.Error("FindVCSRoot() error = nil, want error for non-repo directory")
		}
	})

	t.Run("unknown vcs type", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := FindVCSRoot(tmpDir, VCSUnknown)
		if err == nil {
			t.Error("FindVCSRoot() error = nil, want error for unknown VCS type")
		}
	})
}

func TestNewProvider(t *testing.T) {
	t.Run("no vcs detected", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := NewProvider(tmpDir)
		if err == nil {
			t.Error("NewProvider() error = nil, want error for non-repo directory")
		}
	})

	// Note: Full provider creation tests would require actual git/jj repos
	// which is tested in the respective provider test files
}

func TestIsToolAvailable(t *testing.T) {
	// Test with a command that should exist on most systems
	t.Run("existing tool", func(t *testing.T) {
		// 'ls' or 'echo' should exist on any Unix-like system
		if !IsToolAvailable("ls") && !IsToolAvailable("echo") {
			t.Error("IsToolAvailable() = false for common tools, expected at least one to exist")
		}
	})

	t.Run("non-existing tool", func(t *testing.T) {
		if IsToolAvailable("this-tool-definitely-does-not-exist-12345") {
			t.Error("IsToolAvailable() = true for non-existent tool")
		}
	})
}
