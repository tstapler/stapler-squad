package vc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// skipIfNoJJ skips the test if jj is not installed
func skipIfNoJJ(t *testing.T) {
	t.Helper()
	if !IsToolAvailable("jj") {
		t.Skip("jj (Jujutsu) not installed, skipping test")
	}
}

func TestNewJujutsuProvider(t *testing.T) {
	t.Run("valid jujutsu repository", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()

		// Initialize a jujutsu repository
		initJJRepo(t, tmpDir)

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v, want nil", err)
		}

		if provider == nil {
			t.Fatal("NewJujutsuProvider() returned nil provider")
		}

		if provider.workDir != tmpDir {
			t.Errorf("provider.workDir = %q, want %q", provider.workDir, tmpDir)
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		skipIfNoJJ(t)
		_, err := NewJujutsuProvider("/non/existent/path/that/should/not/exist")
		if err == nil {
			t.Error("NewJujutsuProvider() error = nil, want error for non-existent path")
		}
	})

	t.Run("not a jujutsu repository", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()

		_, err := NewJujutsuProvider(tmpDir)
		if err == nil {
			t.Error("NewJujutsuProvider() error = nil, want error for non-jj directory")
		}
	})

	t.Run("subdirectory of jujutsu repository", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()

		// Initialize a jujutsu repository
		initJJRepo(t, tmpDir)

		// Create a subdirectory
		subDir := filepath.Join(tmpDir, "src", "pkg")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create subdirectory: %v", err)
		}

		provider, err := NewJujutsuProvider(subDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v, want nil", err)
		}

		// repoRoot should be the top-level jj directory
		if provider.repoRoot != tmpDir {
			t.Errorf("provider.repoRoot = %q, want %q", provider.repoRoot, tmpDir)
		}

		// workDir should be the subdirectory we passed in
		if provider.workDir != subDir {
			t.Errorf("provider.workDir = %q, want %q", provider.workDir, subDir)
		}
	})
}

func TestJujutsuProviderType(t *testing.T) {
	skipIfNoJJ(t)
	tmpDir := t.TempDir()

	// Initialize a jujutsu repository
	initJJRepo(t, tmpDir)

	provider, err := NewJujutsuProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewJujutsuProvider() error = %v", err)
	}

	if provider.Type() != VCSJujutsu {
		t.Errorf("Type() = %v, want %v", provider.Type(), VCSJujutsu)
	}
}

func TestJujutsuProviderName(t *testing.T) {
	skipIfNoJJ(t)
	tmpDir := t.TempDir()

	// Initialize a jujutsu repository
	initJJRepo(t, tmpDir)

	provider, err := NewJujutsuProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewJujutsuProvider() error = %v", err)
	}

	if provider.Name() != "Jujutsu" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "Jujutsu")
	}
}

func TestJujutsuProviderWorkDir(t *testing.T) {
	skipIfNoJJ(t)
	tmpDir := t.TempDir()

	// Initialize a jujutsu repository
	initJJRepo(t, tmpDir)

	provider, err := NewJujutsuProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewJujutsuProvider() error = %v", err)
	}

	if provider.WorkDir() != tmpDir {
		t.Errorf("WorkDir() = %q, want %q", provider.WorkDir(), tmpDir)
	}
}

func TestParseJJStatusChar(t *testing.T) {
	tests := []struct {
		char     byte
		expected FileStatus
	}{
		{'M', FileModified},
		{'A', FileAdded},
		{'D', FileDeleted},
		{'R', FileRenamed},
		{'C', FileCopied},
		{'?', FileUntracked},
		{'X', FileModified}, // Unknown defaults to modified
		{'Z', FileModified}, // Unknown defaults to modified
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			result := parseJJStatusChar(tt.char)
			if result != tt.expected {
				t.Errorf("parseJJStatusChar(%q) = %v, want %v", tt.char, result, tt.expected)
			}
		})
	}
}

func TestJujutsuProviderGetBranch(t *testing.T) {
	t.Run("no bookmark (returns change ID or @)", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()

		// Initialize a jujutsu repository
		initJJRepo(t, tmpDir)

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
		}

		branch, err := provider.GetBranch()
		if err != nil {
			t.Fatalf("GetBranch() error = %v", err)
		}

		// Should return a change ID (short hash) or @ when no bookmark
		if branch == "" {
			t.Error("GetBranch() returned empty string, want change ID or @")
		}
	})

	t.Run("with bookmark", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()

		// Initialize a jujutsu repository
		initJJRepo(t, tmpDir)

		// Create a bookmark
		cmd := exec.Command("jj", "bookmark", "create", "test-bookmark")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to create bookmark: %v", err)
		}

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
		}

		branch, err := provider.GetBranch()
		if err != nil {
			t.Fatalf("GetBranch() error = %v", err)
		}

		if !strings.Contains(branch, "test-bookmark") {
			t.Errorf("GetBranch() = %q, want to contain %q", branch, "test-bookmark")
		}
	})
}

func TestJujutsuProviderGetChangedFiles(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()

		// Initialize a jujutsu repository
		initJJRepo(t, tmpDir)

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
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
		skipIfNoJJ(t)
		tmpDir := t.TempDir()

		// Initialize a jujutsu repository with initial content
		initJJRepoWithFile(t, tmpDir)

		// Modify the file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
			t.Fatalf("Failed to modify test file: %v", err)
		}

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
		}

		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 1 {
			t.Fatalf("GetChangedFiles() returned %d files, want 1", len(files))
		}

		// jj may return full paths or relative paths depending on working directory
		// Just check that the path ends with test.txt
		if !strings.HasSuffix(files[0].Path, "test.txt") {
			t.Errorf("files[0].Path = %q, want to end with %q", files[0].Path, "test.txt")
		}

		if files[0].Status != FileModified {
			t.Errorf("files[0].Status = %v, want %v", files[0].Status, FileModified)
		}

		// In jj, all changes are "staged" in the working copy commit
		if !files[0].IsStaged {
			t.Error("files[0].IsStaged = false, want true")
		}
	})

	t.Run("added file", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()

		// Initialize a jujutsu repository
		initJJRepo(t, tmpDir)

		// Add a new file
		newFile := filepath.Join(tmpDir, "new.txt")
		if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
			t.Fatalf("Failed to create new file: %v", err)
		}

		// Track the file
		cmd := exec.Command("jj", "file", "track", "new.txt")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to track file: %v", err)
		}

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
		}

		files, err := provider.GetChangedFiles()
		if err != nil {
			t.Fatalf("GetChangedFiles() error = %v", err)
		}

		if len(files) != 1 {
			t.Fatalf("GetChangedFiles() returned %d files, want 1", len(files))
		}

		// jj may return full paths or relative paths depending on working directory
		// Just check that the path ends with new.txt
		if !strings.HasSuffix(files[0].Path, "new.txt") {
			t.Errorf("files[0].Path = %q, want to end with %q", files[0].Path, "new.txt")
		}

		if files[0].Status != FileAdded {
			t.Errorf("files[0].Status = %v, want %v", files[0].Status, FileAdded)
		}
	})
}

func TestJujutsuProviderGetStatus(t *testing.T) {
	t.Run("clean repository", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()
		initJJRepo(t, tmpDir)

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
		}

		status, err := provider.GetStatus()
		if err != nil {
			t.Fatalf("GetStatus() error = %v", err)
		}

		if status.Type != VCSJujutsu {
			t.Errorf("status.Type = %v, want %v", status.Type, VCSJujutsu)
		}

		if len(status.StagedFiles) != 0 {
			t.Errorf("status.StagedFiles has %d items, want 0", len(status.StagedFiles))
		}

		if len(status.UntrackedFiles) != 0 {
			t.Errorf("status.UntrackedFiles has %d items, want 0", len(status.UntrackedFiles))
		}
	})

	t.Run("with changes", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()
		initJJRepoWithFile(t, tmpDir)

		// Modify the file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
		}

		status, err := provider.GetStatus()
		if err != nil {
			t.Fatalf("GetStatus() error = %v", err)
		}

		// In jj, working copy changes are "staged" in @
		if len(status.StagedFiles) != 1 {
			t.Errorf("status.StagedFiles has %d items, want 1", len(status.StagedFiles))
		}

		if !status.HasStaged {
			t.Error("status.HasStaged = false, want true")
		}
	})
}

func TestJujutsuProviderCommit(t *testing.T) {
	t.Run("commit with message (describe + new)", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()
		initJJRepoWithFile(t, tmpDir)

		// Modify the file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
			t.Fatalf("Failed to modify test file: %v", err)
		}

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
		}

		// Get change ID before commit
		changeIDBefore, _ := provider.runJJ("log", "-r", "@", "--no-graph", "-T", `change_id.short()`)

		// Commit (describe + new)
		if err := provider.Commit("test commit message"); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		// Change ID should be different after 'jj new' creates new change
		changeIDAfter, _ := provider.runJJ("log", "-r", "@", "--no-graph", "-T", `change_id.short()`)

		if changeIDBefore == changeIDAfter {
			t.Error("Change ID should be different after commit (jj new creates new change)")
		}

		// Check that the previous change has our description
		desc, _ := provider.runJJ("log", "-r", "@-", "--no-graph", "-T", `description.first_line()`)
		if !strings.Contains(desc, "test commit message") {
			t.Errorf("Description = %q, want to contain %q", desc, "test commit message")
		}
	})
}

func TestJujutsuProviderGetDiff(t *testing.T) {
	t.Run("diff of modified file", func(t *testing.T) {
		skipIfNoJJ(t)
		tmpDir := t.TempDir()
		initJJRepoWithFile(t, tmpDir)

		// Modify the file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
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
		skipIfNoJJ(t)
		tmpDir := t.TempDir()
		initJJRepoWithFile(t, tmpDir)

		// Modify the file
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		provider, err := NewJujutsuProvider(tmpDir)
		if err != nil {
			t.Fatalf("NewJujutsuProvider() error = %v", err)
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

func TestJujutsuProviderGetInteractiveCommand(t *testing.T) {
	skipIfNoJJ(t)
	tmpDir := t.TempDir()

	// Initialize a jujutsu repository
	initJJRepo(t, tmpDir)

	provider, err := NewJujutsuProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewJujutsuProvider() error = %v", err)
	}

	command := provider.GetInteractiveCommand()

	// Should return one of the known interactive tools
	validCommands := []string{"lazyjj", "jj log"}
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

func TestJujutsuProviderGetLogCommand(t *testing.T) {
	skipIfNoJJ(t)
	tmpDir := t.TempDir()

	// Initialize a jujutsu repository
	initJJRepo(t, tmpDir)

	provider, err := NewJujutsuProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewJujutsuProvider() error = %v", err)
	}

	command := provider.GetLogCommand()

	if command != "jj log" {
		t.Errorf("GetLogCommand() = %q, want %q", command, "jj log")
	}
}

func TestJujutsuProviderDescribe(t *testing.T) {
	skipIfNoJJ(t)
	tmpDir := t.TempDir()
	initJJRepo(t, tmpDir)

	provider, err := NewJujutsuProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewJujutsuProvider() error = %v", err)
	}

	// Set a description
	if err := provider.Describe("test description"); err != nil {
		t.Fatalf("Describe() error = %v", err)
	}

	// Verify description
	desc, _ := provider.runJJ("log", "-r", "@", "--no-graph", "-T", `description.first_line()`)
	if !strings.Contains(desc, "test description") {
		t.Errorf("Description = %q, want to contain %q", desc, "test description")
	}
}

func TestJujutsuProviderNew(t *testing.T) {
	skipIfNoJJ(t)
	tmpDir := t.TempDir()
	initJJRepo(t, tmpDir)

	provider, err := NewJujutsuProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewJujutsuProvider() error = %v", err)
	}

	// Get change ID before
	changeIDBefore, _ := provider.runJJ("log", "-r", "@", "--no-graph", "-T", `change_id.short()`)

	// Create new change
	if err := provider.New(); err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Get change ID after
	changeIDAfter, _ := provider.runJJ("log", "-r", "@", "--no-graph", "-T", `change_id.short()`)

	if changeIDBefore == changeIDAfter {
		t.Error("Change ID should be different after New()")
	}
}

func TestJujutsuProviderSquash(t *testing.T) {
	skipIfNoJJ(t)
	tmpDir := t.TempDir()
	initJJRepoWithFile(t, tmpDir)

	provider, err := NewJujutsuProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewJujutsuProvider() error = %v", err)
	}

	// Modify file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Create new change
	if err := provider.New(); err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Add another modification
	if err := os.WriteFile(testFile, []byte("more changes"), 0644); err != nil {
		t.Fatalf("Failed to modify file again: %v", err)
	}

	// Squash should work (squashes current into parent)
	if err := provider.Squash(); err != nil {
		t.Fatalf("Squash() error = %v", err)
	}
}

func TestJujutsuProviderAbandon(t *testing.T) {
	skipIfNoJJ(t)
	tmpDir := t.TempDir()
	initJJRepo(t, tmpDir)

	provider, err := NewJujutsuProvider(tmpDir)
	if err != nil {
		t.Fatalf("NewJujutsuProvider() error = %v", err)
	}

	// Create a new change first
	if err := provider.New(); err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Abandon should work
	if err := provider.Abandon(); err != nil {
		t.Fatalf("Abandon() error = %v", err)
	}
}

// Helper functions

func initJJRepo(t *testing.T, dir string) {
	t.Helper()

	// Initialize jj repo
	cmd := exec.Command("jj", "git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init jj repo: %v", err)
	}

	// Configure jj user (if not set globally)
	cmd = exec.Command("jj", "config", "set", "--repo", "user.name", "Test User")
	cmd.Dir = dir
	_ = cmd.Run() // Ignore error if already set

	cmd = exec.Command("jj", "config", "set", "--repo", "user.email", "test@test.com")
	cmd.Dir = dir
	_ = cmd.Run() // Ignore error if already set
}

func initJJRepoWithFile(t *testing.T, dir string) {
	t.Helper()

	// Initialize jj repo
	initJJRepo(t, dir)

	// Create and track a test file
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Track the file
	cmd := exec.Command("jj", "file", "track", "test.txt")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to track file: %v", err)
	}

	// Create a new change to establish a "clean" baseline with the file committed
	cmd = exec.Command("jj", "describe", "-m", "initial commit")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to describe change: %v", err)
	}

	cmd = exec.Command("jj", "new")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create new change: %v", err)
	}
}
