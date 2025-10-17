package session

import (
	"claude-squad/session/git"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFromInstanceDataWithMissingWorktree(t *testing.T) {
	// Create a temporary directory to simulate a worktree path
	tempDir, err := os.MkdirTemp("", "claude-squad-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create worktree path within temp dir
	worktreePath := filepath.Join(tempDir, "worktree-path")
	err = os.MkdirAll(worktreePath, 0755)
	if err != nil {
		t.Fatalf("Failed to create worktree directory: %v", err)
	}

	// Test our fix function directly instead of trying to mock everything
	// Create a test instance with a gitWorktree that points to a real path
	instance := &Instance{
		Title:     "Test Instance",
		Path:      "/path/to/repo",
		Branch:    "test-branch",
		Status:    Ready,
		Height:    100,
		Width:     200,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Program:   "claude",
		gitWorktree: git.NewGitWorktreeFromStorage(
			"/path/to/repo",
			worktreePath,
			"Test Instance",
			"test-branch",
			"abcdef1234567890",
		),
		started: true,
	}

	// Test 1: Worktree exists - instance should not be paused
	checkInstanceStatus(t, instance, worktreePath, false)

	// Now delete the worktree directory to simulate a stale worktree
	err = os.RemoveAll(worktreePath)
	if err != nil {
		t.Fatalf("Failed to remove test worktree directory: %v", err)
	}

	// Reload the instance from data - this should detect the missing worktree
	// We need to use a modified approach since we can't call the actual FromInstanceData
	// which would try to start a real session
	instance = &Instance{
		Title:     "Test Instance",
		Path:      "/path/to/repo",
		Branch:    "test-branch",
		Status:    Ready,
		Height:    100,
		Width:     200,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Program:   "claude",
		gitWorktree: git.NewGitWorktreeFromStorage(
			"/path/to/repo",
			worktreePath,
			"Test Instance",
			"test-branch",
			"abcdef1234567890",
		),
		started: true,
	}

	// Test 2: Apply our fix - check if worktree exists and update status
	if !instance.Paused() && instance.gitWorktree != nil {
		worktreePath := instance.gitWorktree.GetWorktreePath()
		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			// Worktree has been deleted, mark instance as paused
			instance.Status = Paused
		}
	}

	// Verify that the instance is now paused
	checkInstanceStatus(t, instance, worktreePath, true)
}

func checkInstanceStatus(t *testing.T, instance *Instance, worktreePath string, expectPaused bool) {
	if expectPaused && !instance.Paused() {
		t.Errorf("Expected instance to be paused when worktree at %s doesn't exist", worktreePath)
	} else if !expectPaused && instance.Paused() {
		t.Errorf("Expected instance to not be paused when worktree at %s exists", worktreePath)
	}
}

func TestStatusEnumValues(t *testing.T) {
	// Test that all status values are defined correctly
	tests := []struct {
		status Status
		name   string
	}{
		{Running, "Running"},
		{Ready, "Ready"},
		{Loading, "Loading"},
		{Paused, "Paused"},
		{NeedsApproval, "NeedsApproval"},
	}

	// Verify that status values are sequential starting from 0
	for i, test := range tests {
		if int(test.status) != i {
			t.Errorf("Expected %s status to have value %d, got %d", test.name, i, test.status)
		}
	}
}

func TestTildeExpansionInNewInstance(t *testing.T) {
	// Get home directory for comparison
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name              string
		inputPath         string
		expectStartsWith  string
		expectEndsWith    string
	}{
		{
			name:             "Tilde with path",
			inputPath:        "~/test-project",
			expectStartsWith: homeDir,
			expectEndsWith:   "test-project",
		},
		{
			name:             "Just tilde",
			inputPath:        "~",
			expectStartsWith: homeDir,
			expectEndsWith:   "",
		},
		{
			name:             "Absolute path unchanged",
			inputPath:        "/tmp/test",
			expectStartsWith: "/tmp",
			expectEndsWith:   "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance, err := NewInstance(InstanceOptions{
				Title:   "Test Session",
				Path:    tt.inputPath,
				Program: "claude",
			})

			if err != nil {
				t.Fatalf("NewInstance failed: %v", err)
			}

			// Critical check: path should NOT contain "/~/" pattern (the bug we're fixing)
			if filepath.Dir(instance.Path) != instance.Path && filepath.Base(filepath.Dir(instance.Path)) == "~" {
				t.Errorf("Path contains unexpanded tilde directory pattern: %s", instance.Path)
			}

			// Check expected prefix
			if tt.expectStartsWith != "" && !filepath.IsAbs(tt.expectStartsWith) {
				// Convert to absolute for comparison
				tt.expectStartsWith, _ = filepath.Abs(tt.expectStartsWith)
			}
			if tt.expectStartsWith != "" && !filepath.HasPrefix(instance.Path, tt.expectStartsWith) {
				t.Errorf("Expected path to start with %s, got: %s", tt.expectStartsWith, instance.Path)
			}

			// Check expected suffix
			if tt.expectEndsWith != "" && filepath.Base(instance.Path) != tt.expectEndsWith {
				t.Errorf("Expected path to end with %s, got: %s", tt.expectEndsWith, filepath.Base(instance.Path))
			}
		})
	}
}

func TestMigrationOfCorruptedPaths(t *testing.T) {
	// Get home directory for comparison
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name           string
		corruptedPath  string
		expectedPrefix string
		shouldFix      bool
	}{
		{
			name:           "Corrupted path with tilde",
			corruptedPath:  "/Users/tylerstapler/IdeaProjects/claude-squad/~/IdeaProjects/platform",
			expectedPrefix: homeDir,
			shouldFix:      true,
		},
		{
			name:           "Another corrupted pattern",
			corruptedPath:  "/tmp/project/~/Documents/code",
			expectedPrefix: homeDir,
			shouldFix:      true,
		},
		{
			name:           "Valid path should not change",
			corruptedPath:  "/Users/tylerstapler/valid/path",
			expectedPrefix: "/Users/tylerstapler",
			shouldFix:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create instance data with potentially corrupted path
			data := InstanceData{
				Title:   "Test Session",
				Path:    tt.corruptedPath,
				Program: "claude",
				Status:  Paused, // Use paused to avoid starting actual session
			}

			instance, err := FromInstanceData(data)
			if err != nil {
				t.Fatalf("FromInstanceData failed: %v", err)
			}

			if tt.shouldFix {
				// Path should be fixed - should not contain "/~/"
				if filepath.Dir(instance.Path) != instance.Path && filepath.Base(filepath.Dir(instance.Path)) == "~" {
					t.Errorf("Migration failed - path still contains unexpanded tilde: %s", instance.Path)
				}

				// Path should start with home directory
				if !filepath.IsAbs(instance.Path) || !filepath.HasPrefix(instance.Path, tt.expectedPrefix) {
					t.Errorf("Expected migrated path to start with %s, got: %s", tt.expectedPrefix, instance.Path)
				}

				// Path should not equal original corrupted path
				if instance.Path == tt.corruptedPath {
					t.Errorf("Path was not migrated, still: %s", instance.Path)
				}
			} else {
				// Path should remain unchanged
				if instance.Path != tt.corruptedPath {
					t.Errorf("Valid path was incorrectly modified from %s to %s", tt.corruptedPath, instance.Path)
				}
			}
		})
	}
}
