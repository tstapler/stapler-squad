package vc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

func TestNewGitProvider(t *testing.T) {
	t.Run("valid git repository", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Initialize a git repository
		cmd := safeexec.CommandContext(context.Background(), "git", "init")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to init git repo: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v, want nil", err)
		}

		if provider == nil {
			t.Fatal("NewGitProvider() returned nil provider")
		}

		if provider.workDir != tmpDir {
			t.Errorf("provider.workDir = %q, want %q", provider.workDir, tmpDir)
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		_, err := NewGitProvider("/non/existent/path/that/should/not/exist")
		if err == nil {
			t.Error("NewGitProvider() error = nil, want error for non-existent path")
		}
	})

	t.Run("not a git repository", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, err := NewGitProvider(tmpDir)
		if err == nil {
			t.Error("NewGitProvider() error = nil, want error for non-git directory")
		}
	})

	t.Run("subdirectory of git repository", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Initialize a git repository
		cmd := safeexec.CommandContext(context.Background(), "git", "init")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to init git repo: %v", err)
		}

		// Create a subdirectory
		subDir := filepath.Join(tmpDir, "src", "pkg")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirectory: %v", err)
		}

		provider, err := NewGitProvider(subDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v, want nil", err)
		}

		// repoRoot should be the top-level git directory
		if provider.repoRoot != tmpDir {
			t.Errorf("provider.repoRoot = %q, want %q", provider.repoRoot, tmpDir)
		}

		// workDir should be the subdirectory we passed in
		if provider.workDir != subDir {
			t.Errorf("provider.workDir = %q, want %q", provider.workDir, subDir)
		}
	})
}

func TestGitProviderType(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repository
	cmd := safeexec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	provider, err := NewGitProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewGitProvider() error = %v", err)
	}

	if provider.Type() != VCSGit {
		t.Errorf("Type() = %v, want %v", provider.Type(), VCSGit)
	}
}

func TestGitProviderName(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repository
	cmd := safeexec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	provider, err := NewGitProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewGitProvider() error = %v", err)
	}

	if provider.Name() != "Git" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "Git")
	}
}

func TestGitProviderWorkDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repository
	cmd := safeexec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	provider, err := NewGitProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewGitProvider() error = %v", err)
	}

	if provider.WorkDir() != tmpDir {
		t.Errorf("WorkDir() = %q, want %q", provider.WorkDir(), tmpDir)
	}
}

func TestParseGitStatusChar(t *testing.T) {
	tests := []struct {
		char     byte
		expected FileStatus
	}{
		{'M', FileModified},
		{'T', FileModified}, // Type change - implementation treats as modified via default
		{'A', FileAdded},
		{'D', FileDeleted},
		{'R', FileRenamed},
		{'C', FileCopied},
		{'U', FileConflict},
		{'?', FileUntracked},
		{'!', FileIgnored},
		{'X', FileModified}, // Unknown defaults to modified
		{'Z', FileModified}, // Unknown defaults to modified
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			result := parseGitStatusChar(tt.char)
			if result != tt.expected {
				t.Errorf("parseGitStatusChar(%q) = %v, want %v", tt.char, result, tt.expected)
			}
		})
	}
}

func TestGitProviderGetBranch(t *testing.T) {
	t.Run("normal branch", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Initialize a git repository
		cmd := safeexec.CommandContext(context.Background(), "git", "init", "-b", "main")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to init git repo: %v", err)
		}

		// Configure git user for commits
		configureGitUser(t, tmpDir)

		// Create an initial commit
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		cmd = safeexec.CommandContext(context.Background(), "git", "add", ".")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to add file: %v", err)
		}

		cmd = safeexec.CommandContext(context.Background(), "git", "commit", "-m", "initial commit")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		branch, err := provider.GetBranch()
		if err != nil {
			t.Fatalf("GetBranch() error = %v", err)
		}

		if branch != "main" {
			t.Errorf("GetBranch() = %q, want %q", branch, "main")
		}
	})

	t.Run("detached HEAD", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Initialize a git repository
		cmd := safeexec.CommandContext(context.Background(), "git", "init", "-b", "main")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to init git repo: %v", err)
		}

		// Configure git user for commits
		configureGitUser(t, tmpDir)

		// Create an initial commit
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		cmd = safeexec.CommandContext(context.Background(), "git", "add", ".")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to add file: %v", err)
		}

		cmd = safeexec.CommandContext(context.Background(), "git", "commit", "-m", "initial commit")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Get the commit hash
		cmd = safeexec.CommandContext(context.Background(), "git", "rev-parse", "HEAD")
		cmd.Dir = tmpDir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("Failed to get commit hash: %v", err)
		}
		commitHash := strings.TrimSpace(string(out))

		// Detach HEAD
		cmd = safeexec.CommandContext(context.Background(), "git", "checkout", "--detach", "HEAD")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to detach HEAD: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		branch, err := provider.GetBranch()
		if err != nil {
			t.Fatalf("GetBranch() error = %v", err)
		}

		// Should return "(detached: <short_hash>)" format
		expectedPrefix := "(detached: " + commitHash[:7] + ")"
		if branch != expectedPrefix {
			t.Errorf("GetBranch() = %q, want %q", branch, expectedPrefix)
		}
	})
}

func TestGitProviderGetChangedFiles(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Initialize a git repository with a commit
		initGitRepoWithCommit(t, tmpDir)

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 0 {
			t.Errorf("GetChangedFiles() returned %d files, want 0", len(files))
		}
	})

	t.Run("modified file", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Initialize a git repository with a commit
		initGitRepoWithCommit(t, tmpDir)

		// Modify the file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
			t.Fatalf("Failed to modify test file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 1 {
			t.Fatalf("GetChangedFiles() returned %d files, want 1", len(files))
		}

		if files[0].Path != "test.txt" {
			t.Errorf("files[0].Path = %q, want %q", files[0].Path, "test.txt")
		}

		if files[0].Status != FileModified {
			t.Errorf("files[0].Status = %v, want %v", files[0].Status, FileModified)
		}

		if files[0].IsStaged {
			t.Error("files[0].IsStaged = true, want false")
		}
	})

	t.Run("staged file", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Initialize a git repository with a commit
		initGitRepoWithCommit(t, tmpDir)

		// Add a new file and stage it
		newFile := filepath.Join(tmpDir, "new.txt")
		if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
			t.Fatalf("Failed to create new file: %v", err)
		}

		cmd := safeexec.CommandContext(context.Background(), "git", "add", "new.txt")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to stage file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 1 {
			t.Fatalf("GetChangedFiles() returned %d files, want 1", len(files))
		}

		if files[0].Path != "new.txt" {
			t.Errorf("files[0].Path = %q, want %q", files[0].Path, "new.txt")
		}

		if files[0].Status != FileAdded {
			t.Errorf("files[0].Status = %v, want %v", files[0].Status, FileAdded)
		}

		if !files[0].IsStaged {
			t.Error("files[0].IsStaged = false, want true")
		}
	})

	t.Run("untracked file", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Initialize a git repository with a commit
		initGitRepoWithCommit(t, tmpDir)

		// Add an untracked file
		untrackedFile := filepath.Join(tmpDir, "untracked.txt")
		if err := os.WriteFile(untrackedFile, []byte("untracked content"), 0644); err != nil {
			t.Fatalf("Failed to create untracked file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 1 {
			t.Fatalf("GetChangedFiles() returned %d files, want 1", len(files))
		}

		if files[0].Path != "untracked.txt" {
			t.Errorf("files[0].Path = %q, want %q", files[0].Path, "untracked.txt")
		}

		if files[0].Status != FileUntracked {
			t.Errorf("files[0].Status = %v, want %v", files[0].Status, FileUntracked)
		}
	})

	t.Run("renamed file reports new path, old path, and status", func(t *testing.T) {
		tmpDir := t.TempDir()

		initGitRepoWithCommit(t, tmpDir)

		// `git mv` stages a rename; porcelain v2 reports it as a single "2 "
		// entry with a tab-separated <origPath> suffix — the case that used to
		// be mis-parsed as Path="R100" (the similarity-score token) with no
		// OldPath at all.
		cmd := safeexec.CommandContext(context.Background(), "git", "mv", "test.txt", "renamed.txt")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to rename file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 1 {
			t.Fatalf("GetChangedFiles() returned %d files, want 1: %+v", len(files), files)
		}

		if files[0].Path != "renamed.txt" {
			t.Errorf("files[0].Path = %q, want %q", files[0].Path, "renamed.txt")
		}

		if files[0].OldPath != "test.txt" {
			t.Errorf("files[0].OldPath = %q, want %q", files[0].OldPath, "test.txt")
		}

		if files[0].Status != FileRenamed {
			t.Errorf("files[0].Status = %v, want %v", files[0].Status, FileRenamed)
		}

		if !files[0].IsStaged {
			t.Error("files[0].IsStaged = false, want true")
		}
	})
}

func TestGitProviderStageOperations(t *testing.T) {
	t.Run("Stage single file", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		// Add a new file
		newFile := filepath.Join(tmpDir, "new.txt")
		if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
			t.Fatalf("Failed to create new file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		// Stage the file
		if err := provider.Stage("new.txt"); err != nil {
			t.Fatalf("Stage() error = %v", err)
		}

		// Verify it's staged
		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 1 {
			t.Fatalf("Expected 1 file, got %d", len(files))
		}

		if !files[0].IsStaged {
			t.Error("File should be staged")
		}
	})

	t.Run("StageAll", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		// Add multiple new files
		for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
			path := filepath.Join(tmpDir, name)
			if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
				t.Fatalf("Failed to create file: %v", err)
			}
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		// Stage all files
		if err := provider.StageAll(); err != nil {
			t.Fatalf("StageAll() error = %v", err)
		}

		// Verify all are staged
		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 3 {
			t.Fatalf("Expected 3 files, got %d", len(files))
		}

		for _, f := range files {
			if !f.IsStaged {
				t.Errorf("File %s should be staged", f.Path)
			}
		}
	})

	t.Run("Unstage single file", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		// Add and stage a new file
		newFile := filepath.Join(tmpDir, "new.txt")
		if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
			t.Fatalf("Failed to create new file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		if err := provider.Stage("new.txt"); err != nil {
			t.Fatalf("Stage() error = %v", err)
		}

		// Unstage the file
		if err := provider.Unstage("new.txt"); err != nil {
			t.Fatalf("Unstage() error = %v", err)
		}

		// Verify it's unstaged (but still untracked)
		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 1 {
			t.Fatalf("Expected 1 file, got %d", len(files))
		}

		if files[0].IsStaged {
			t.Error("File should not be staged")
		}
	})

	t.Run("UnstageAll", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		// Add and stage multiple files
		for _, name := range []string{"a.txt", "b.txt"} {
			path := filepath.Join(tmpDir, name)
			if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
				t.Fatalf("Failed to create file: %v", err)
			}
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		if err := provider.StageAll(); err != nil {
			t.Fatalf("StageAll() error = %v", err)
		}

		// Unstage all
		if err := provider.UnstageAll(); err != nil {
			t.Fatalf("UnstageAll() error = %v", err)
		}

		// Verify none are staged
		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		for _, f := range files {
			if f.IsStaged {
				t.Errorf("File %s should not be staged", f.Path)
			}
		}
	})
}

func TestGitProviderCommit(t *testing.T) {
	t.Run("commit staged changes", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		// Add and stage a new file
		newFile := filepath.Join(tmpDir, "new.txt")
		if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
			t.Fatalf("Failed to create new file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		if err := provider.Stage("new.txt"); err != nil {
			t.Fatalf("Stage() error = %v", err)
		}

		// Commit
		if err := provider.Commit("test commit message"); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		// Verify no changes after commit
		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 0 {
			t.Errorf("Expected 0 files after commit, got %d", len(files))
		}

		// Verify commit message
		cmd := safeexec.CommandContext(context.Background(), "git", "log", "-1", "--pretty=%s")
		cmd.Dir = tmpDir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("Failed to get commit message: %v", err)
		}

		if strings.TrimSpace(string(out)) != "test commit message" {
			t.Errorf("Commit message = %q, want %q", strings.TrimSpace(string(out)), "test commit message")
		}
	})

	t.Run("commit with nothing staged", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		// Commit with nothing staged should fail
		err = provider.Commit("empty commit")
		if err == nil {
			t.Error("Commit() error = nil, want error for empty commit")
		}
	})
}

func TestGitProviderGetStatus(t *testing.T) {
	t.Run("clean repository", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		status, err := provider.GetStatus()
		if err != nil {
			t.Fatalf("GetStatus() error = %v", err)
		}

		if status.Type != VCSGit {
			t.Errorf("status.Type = %v, want %v", status.Type, VCSGit)
		}

		if status.Branch != "main" {
			t.Errorf("status.Branch = %q, want %q", status.Branch, "main")
		}

		if len(status.StagedFiles) != 0 {
			t.Errorf("status.StagedFiles has %d items, want 0", len(status.StagedFiles))
		}

		if len(status.UnstagedFiles) != 0 {
			t.Errorf("status.UnstagedFiles has %d items, want 0", len(status.UnstagedFiles))
		}

		if len(status.UntrackedFiles) != 0 {
			t.Errorf("status.UntrackedFiles has %d items, want 0", len(status.UntrackedFiles))
		}
	})

	t.Run("mixed status", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		// Add staged file
		stagedFile := filepath.Join(tmpDir, "staged.txt")
		if err := os.WriteFile(stagedFile, []byte("staged"), 0644); err != nil {
			t.Fatalf("Failed to create staged file: %v", err)
		}
		cmd := safeexec.CommandContext(context.Background(), "git", "add", "staged.txt")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to stage file: %v", err)
		}

		// Modify existing file (unstaged)
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		// Add untracked file
		untrackedFile := filepath.Join(tmpDir, "untracked.txt")
		if err := os.WriteFile(untrackedFile, []byte("untracked"), 0644); err != nil {
			t.Fatalf("Failed to create untracked file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		status, err := provider.GetStatus()
		if err != nil {
			t.Fatalf("GetStatus() error = %v", err)
		}

		if len(status.StagedFiles) != 1 {
			t.Errorf("status.StagedFiles has %d items, want 1", len(status.StagedFiles))
		}

		if len(status.UnstagedFiles) != 1 {
			t.Errorf("status.UnstagedFiles has %d items, want 1", len(status.UnstagedFiles))
		}

		if len(status.UntrackedFiles) != 1 {
			t.Errorf("status.UntrackedFiles has %d items, want 1", len(status.UntrackedFiles))
		}
	})
}

func TestGitProviderGetDiff(t *testing.T) {
	t.Run("diff of modified file", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		// Modify the file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		diff, err := provider.GetFileDiff("test.txt")
		if err != nil {
			t.Fatalf("GetFileDiff() error = %v", err)
		}

		if !strings.Contains(diff, "test.txt") {
			t.Error("Diff should contain filename")
		}

		if !strings.Contains(diff, "modified content") {
			t.Error("Diff should contain new content")
		}
	})

	t.Run("GetDiff for all changes", func(t *testing.T) {
		tmpDir := t.TempDir()
		initGitRepoWithCommit(t, tmpDir)

		// Modify the file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		// Add another file
		newFile := filepath.Join(tmpDir, "new.txt")
		if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
			t.Fatalf("Failed to create new file: %v", err)
		}

		provider, err := NewGitProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewGitProvider() error = %v", err)
		}

		diff, err := provider.GetDiff()
		if err != nil {
			t.Fatalf("GetDiff() error = %v", err)
		}

		if !strings.Contains(diff, "test.txt") {
			t.Error("Diff should contain modified file")
		}
	})
}

func TestGitProviderGetInteractiveCommand(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repository
	cmd := safeexec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	provider, err := NewGitProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewGitProvider() error = %v", err)
	}

	command := provider.GetInteractiveCommand()

	// Should return one of the known interactive tools
	validCommands := []string{"lazygit", "tig", "gitui", "git status"}
	valid := false
	for _, vc := range validCommands {
		if command == vc {
			valid = true
			break
		}
	}

	if !valid {
		t.Errorf("GetInteractiveCommand() = %q, want one of %v", command, validCommands)
	}
}

func TestGitProviderGetLogCommand(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repository
	cmd := safeexec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	provider, err := NewGitProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewGitProvider() error = %v", err)
	}

	command := provider.GetLogCommand()

	// Should return either tig or git log (implementation uses -20 limit)
	validCommands := []string{"tig", "git log --oneline --graph -20"}
	valid := false
	for _, vc := range validCommands {
		if command == vc {
			valid = true
			break
		}
	}

	if !valid {
		t.Errorf("GetLogCommand() = %q, want one of %v", command, validCommands)
	}
}

func TestNumstatPath(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "plain path",
			raw:  "main.go",
			want: "main.go",
		},
		{
			name: "plain path with leading/trailing whitespace",
			raw:  "  main.go  ",
			want: "main.go",
		},
		{
			name: "simple rename form",
			raw:  "old.txt => new.txt",
			want: "new.txt",
		},
		{
			name: "braced rename with common prefix and suffix",
			raw:  "common/{old => new}/tail",
			want: "common/new/tail",
		},
		{
			name: "braced rename with empty suffix",
			raw:  "src/{old.go => new.go}",
			want: "src/new.go",
		},
		{
			name: "braced rename with only a prefix, no suffix segment",
			raw:  "{old => new}/tail",
			want: "new/tail",
		},
		{
			name: "empty string",
			raw:  "",
			want: "",
		},
		{
			name: "path containing a space, no rename",
			raw:  "my file.txt",
			want: "my file.txt",
		},
		{
			name: "rename of a path containing spaces",
			raw:  "old file.txt => new file.txt",
			want: "new file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := numstatPath(tt.raw)
			if got != tt.want {
				t.Errorf("numstatPath(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParsePorcelainV2Z(t *testing.T) {
	// join builds a NUL-delimited -z record stream from individual tokens,
	// matching what `git status --porcelain=v2 -z` emits: one NUL after every
	// token, no separator required before the first or after the last.
	join := func(tokens ...string) string {
		return strings.Join(tokens, "\x00") + "\x00"
	}

	t.Run("ordinary modified entry, unstaged", func(t *testing.T) {
		output := join("1 .M N... 100644 100644 100644 " +
			"e69de29bb2d1d6434b8b29ae775ad8c2e48c5391 e69de29bb2d1d6434b8b29ae775ad8c2e48c5391 main.go")

		got := parsePorcelainV2Z(output)

		if len(got) != 1 {
			t.Fatalf("got %d files, want 1: %+v", len(got), got)
		}
		if got[0].Path != "main.go" {
			t.Errorf("Path = %q, want %q", got[0].Path, "main.go")
		}
		if got[0].Status != FileModified {
			t.Errorf("Status = %v, want %v", got[0].Status, FileModified)
		}
		if got[0].IsStaged {
			t.Error("IsStaged = true, want false")
		}
	})

	t.Run("ordinary added entry, staged", func(t *testing.T) {
		output := join("1 A. N... 000000 100644 100644 " +
			"0000000000000000000000000000000000000000 e69de29bb2d1d6434b8b29ae775ad8c2e48c5391 new.txt")

		got := parsePorcelainV2Z(output)

		if len(got) != 1 {
			t.Fatalf("got %d files, want 1: %+v", len(got), got)
		}
		if got[0].Path != "new.txt" {
			t.Errorf("Path = %q, want %q", got[0].Path, "new.txt")
		}
		if got[0].Status != FileAdded {
			t.Errorf("Status = %v, want %v", got[0].Status, FileAdded)
		}
		if !got[0].IsStaged {
			t.Error("IsStaged = false, want true")
		}
	})

	t.Run("ordinary deleted entry", func(t *testing.T) {
		output := join("1 .D N... 100644 100644 000000 " +
			"e69de29bb2d1d6434b8b29ae775ad8c2e48c5391 0000000000000000000000000000000000000000 gone.txt")

		got := parsePorcelainV2Z(output)

		if len(got) != 1 {
			t.Fatalf("got %d files, want 1: %+v", len(got), got)
		}
		if got[0].Path != "gone.txt" {
			t.Errorf("Path = %q, want %q", got[0].Path, "gone.txt")
		}
		if got[0].Status != FileDeleted {
			t.Errorf("Status = %v, want %v", got[0].Status, FileDeleted)
		}
	})

	t.Run("rename entry threads path and origPath", func(t *testing.T) {
		output := join(
			"2 R. N... 100644 100644 100644 "+
				"ce013625030ba8dba906f756967f9e9ca394464a ce013625030ba8dba906f756967f9e9ca394464a R100 renamed.txt",
			"old.txt",
		)

		got := parsePorcelainV2Z(output)

		if len(got) != 1 {
			t.Fatalf("got %d files, want 1: %+v", len(got), got)
		}
		if got[0].Path != "renamed.txt" {
			t.Errorf("Path = %q, want %q", got[0].Path, "renamed.txt")
		}
		if got[0].OldPath != "old.txt" {
			t.Errorf("OldPath = %q, want %q", got[0].OldPath, "old.txt")
		}
		if got[0].Status != FileRenamed {
			t.Errorf("Status = %v, want %v", got[0].Status, FileRenamed)
		}
		if !got[0].IsStaged {
			t.Error("IsStaged = false, want true")
		}
	})

	t.Run("untracked entry", func(t *testing.T) {
		output := join("? untracked.txt")

		got := parsePorcelainV2Z(output)

		if len(got) != 1 {
			t.Fatalf("got %d files, want 1: %+v", len(got), got)
		}
		if got[0].Path != "untracked.txt" {
			t.Errorf("Path = %q, want %q", got[0].Path, "untracked.txt")
		}
		if got[0].Status != FileUntracked {
			t.Errorf("Status = %v, want %v", got[0].Status, FileUntracked)
		}
	})

	t.Run("ignored entry is skipped", func(t *testing.T) {
		output := join("! ignored.txt")

		got := parsePorcelainV2Z(output)

		if len(got) != 0 {
			t.Fatalf("got %d files, want 0: %+v", len(got), got)
		}
	})

	// Regression test for the space-truncation bug this PR fixes: an ordinary
	// entry AND a rename entry whose path contains a literal space must not
	// be truncated by naive space-splitting.
	t.Run("ordinary entry and rename entry with spaces in path are not truncated", func(t *testing.T) {
		output := join(
			"1 .M N... 100644 100644 100644 "+
				"e69de29bb2d1d6434b8b29ae775ad8c2e48c5391 e69de29bb2d1d6434b8b29ae775ad8c2e48c5391 my file.txt",
			"2 R. N... 100644 100644 100644 "+
				"ce013625030ba8dba906f756967f9e9ca394464a ce013625030ba8dba906f756967f9e9ca394464a R100 renamed with space.txt",
			"old file.txt",
		)

		got := parsePorcelainV2Z(output)

		if len(got) != 2 {
			t.Fatalf("got %d files, want 2: %+v", len(got), got)
		}

		if got[0].Path != "my file.txt" {
			t.Errorf("files[0].Path = %q, want %q", got[0].Path, "my file.txt")
		}
		if got[0].Status != FileModified {
			t.Errorf("files[0].Status = %v, want %v", got[0].Status, FileModified)
		}

		if got[1].Path != "renamed with space.txt" {
			t.Errorf("files[1].Path = %q, want %q", got[1].Path, "renamed with space.txt")
		}
		if got[1].OldPath != "old file.txt" {
			t.Errorf("files[1].OldPath = %q, want %q", got[1].OldPath, "old file.txt")
		}
		if got[1].Status != FileRenamed {
			t.Errorf("files[1].Status = %v, want %v", got[1].Status, FileRenamed)
		}
	})

	t.Run("empty output yields no files", func(t *testing.T) {
		got := parsePorcelainV2Z("")
		if len(got) != 0 {
			t.Fatalf("got %d files, want 0: %+v", len(got), got)
		}
	})
}

func TestGitProviderGetChangedFiles_PathWithSpace(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepoWithCommit(t, tmpDir)

	// Create and commit a file with a literal space in its name, then modify
	// it — this is the regression case for the porcelain=v2 (non -z) bug
	// where strings.Fields() split the unquoted path on the embedded space
	// and truncated it.
	spacedFile := filepath.Join(tmpDir, "my file.txt")
	if err := os.WriteFile(spacedFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create spaced-name file: %v", err)
	}

	cmd := safeexec.CommandContext(context.Background(), "git", "add", "my file.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to stage spaced-name file: %v", err)
	}

	cmd = safeexec.CommandContext(context.Background(), "git", "commit", "-m", "add spaced file")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit spaced-name file: %v", err)
	}

	if err := os.WriteFile(spacedFile, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify spaced-name file: %v", err)
	}

	provider, err := NewGitProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewGitProvider() error = %v", err)
	}

	files, err := provider.GetChangedFiles()
	if err != nil {
		t.Fatalf("GetChangedFiles() error = %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("GetChangedFiles() returned %d files, want 1: %+v", len(files), files)
	}

	if files[0].Path != "my file.txt" {
		t.Errorf("files[0].Path = %q, want %q (full path, not truncated at the space)", files[0].Path, "my file.txt")
	}

	if files[0].Status != FileModified {
		t.Errorf("files[0].Status = %v, want %v", files[0].Status, FileModified)
	}
}

// Helper functions

func configureGitUser(t *testing.T, dir string) {
	t.Helper()

	cmd := safeexec.CommandContext(context.Background(), "git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to configure git email: %v", err)
	}

	cmd = safeexec.CommandContext(context.Background(), "git", "config", "user.name", "Test User")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to configure git name: %v", err)
	}
}

// initGitRepoWithCommit initialises a git repo on branch "main" with one commit.
// Uses go-git directly rather than shelling out — see
// .claude/rules/prefer-go-git-over-subshells.md.
func initGitRepoWithCommit(t *testing.T, dir string) {
	t.Helper()

	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}
	if _, err := wt.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}
	if _, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@test.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
}
