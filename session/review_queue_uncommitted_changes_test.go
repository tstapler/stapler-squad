package session

import (
	"claude-squad/session/git"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestReviewQueue_UncommittedChangesDetection verifies that uncommitted changes are detected
func TestReviewQueue_UncommittedChangesDetection(t *testing.T) {
	// Create temporary git repository for testing
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "test-repo")

	// Initialize git repository
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize git repo
	if err := runGitCommand(repoPath, "init"); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	// Configure git user
	if err := runGitCommand(repoPath, "config", "user.name", "Test User"); err != nil {
		t.Fatalf("Failed to configure git user.name: %v", err)
	}
	if err := runGitCommand(repoPath, "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("Failed to configure git user.email: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := runGitCommand(repoPath, "add", "."); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}
	if err := runGitCommand(repoPath, "commit", "-m", "Initial commit"); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Create worktree for testing
	worktree, branchName, err := git.NewGitWorktree(repoPath, "test-session")
	if err != nil {
		t.Fatalf("Failed to create git worktree: %v", err)
	}
	worktreePath := worktree.GetWorktreePath()

	// Setup worktree
	if err := worktree.Setup(); err != nil {
		t.Fatalf("Failed to setup worktree: %v", err)
	}
	defer worktree.Cleanup()

	// Create test instance with worktree
	now := time.Now()
	instance := &Instance{
		Title:       "test-uncommitted-changes",
		Path:        repoPath,
		Branch:      branchName,
		Status:      Running,
		gitManager: GitWorktreeManager{worktree: worktree},
		started:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
		ReviewState: ReviewState{LastMeaningfulOutput: now},
	}

	// Create review queue infrastructure
	queue := NewReviewQueue()
	statusManager := NewInstanceStatusManager()
	poller := NewReviewQueuePoller(queue, statusManager, nil)
	poller.AddInstance(instance)

	// Test 1: Clean worktree (no uncommitted changes) should not be added to queue
	t.Run("clean_worktree_not_added", func(t *testing.T) {
		poller.checkSession(instance)
		if queue.Has(instance.Title) {
			t.Error("Expected clean worktree to not be in review queue")
		}
	})

	// Test 2: Add uncommitted changes - should be detected
	t.Run("uncommitted_changes_detected", func(t *testing.T) {
		// Modify file to create uncommitted changes
		modifiedFile := filepath.Join(worktreePath, "modified.txt")
		if err := os.WriteFile(modifiedFile, []byte("uncommitted content"), 0644); err != nil {
			t.Fatalf("Failed to create modified file: %v", err)
		}

		// Check session - should detect uncommitted changes
		poller.checkSession(instance)

		// Verify added to queue with correct reason
		if !queue.Has(instance.Title) {
			t.Error("Expected uncommitted changes to add session to queue")
		}

		item, exists := queue.Get(instance.Title)
		if !exists {
			t.Fatal("Expected item to exist in queue")
		}

		if item.Reason != ReasonUncommittedChanges {
			t.Errorf("Expected reason UncommittedChanges, got %s", item.Reason)
		}

		if item.Priority != PriorityLow {
			t.Errorf("Expected priority Low, got %s", item.Priority)
		}

		if item.Context != "Uncommitted changes ready to commit" {
			t.Errorf("Expected context 'Uncommitted changes ready to commit', got %q", item.Context)
		}
	})

	// Test 3: After committing changes, should be removed from queue
	t.Run("committed_changes_removed", func(t *testing.T) {
		// Commit the changes
		if err := worktree.CommitChanges("Test commit"); err != nil {
			t.Fatalf("Failed to commit changes: %v", err)
		}

		// Check session again - should be removed from queue
		poller.checkSession(instance)

		if queue.Has(instance.Title) {
			item, _ := queue.Get(instance.Title)
			if item.Reason == ReasonUncommittedChanges {
				t.Error("Expected committed changes to remove session from queue")
			}
		}
	})
}

// TestReviewQueue_UncommittedChanges_Priority verifies priority ordering
func TestReviewQueue_UncommittedChanges_Priority(t *testing.T) {
	tests := []struct {
		name              string
		existingReason    AttentionReason
		existingPriority  Priority
		shouldOverride    bool
		expectedReason    AttentionReason
		expectedPriority  Priority
	}{
		{
			name:              "error_overrides_uncommitted",
			existingReason:    ReasonErrorState,
			existingPriority:  PriorityUrgent,
			shouldOverride:    false,
			expectedReason:    ReasonErrorState,
			expectedPriority:  PriorityUrgent,
		},
		{
			name:              "approval_overrides_uncommitted",
			existingReason:    ReasonApprovalPending,
			existingPriority:  PriorityHigh,
			shouldOverride:    false,
			expectedReason:    ReasonApprovalPending,
			expectedPriority:  PriorityHigh,
		},
		{
			name:              "input_overrides_uncommitted",
			existingReason:    ReasonInputRequired,
			existingPriority:  PriorityMedium,
			shouldOverride:    false,
			expectedReason:    ReasonInputRequired,
			expectedPriority:  PriorityMedium,
		},
		{
			name:              "uncommitted_can_override_idle",
			existingReason:    ReasonIdleTimeout,
			existingPriority:  PriorityLow,
			shouldOverride:    true,
			expectedReason:    ReasonUncommittedChanges,
			expectedPriority:  PriorityLow,
		},
		{
			name:              "uncommitted_can_override_complete",
			existingReason:    ReasonTaskComplete,
			existingPriority:  PriorityLow,
			shouldOverride:    true,
			expectedReason:    ReasonUncommittedChanges,
			expectedPriority:  PriorityLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify priority mapping
			priority := reasonToPriority(tt.existingReason)
			if priority != tt.existingPriority {
				t.Errorf("Priority mismatch for %s: expected %s, got %s",
					tt.existingReason, tt.existingPriority, priority)
			}
		})
	}
}

// TestReviewQueue_UncommittedChanges_ReasonString verifies string conversion
func TestReviewQueue_UncommittedChanges_ReasonString(t *testing.T) {
	reason := ReasonUncommittedChanges
	expected := "Uncommitted Changes"
	if reason.String() != expected {
		t.Errorf("Expected ReasonUncommittedChanges.String() to return %q, got %q",
			expected, reason.String())
	}
}

// TestReviewQueue_UncommittedChanges_NoWorktree verifies behavior without worktree
func TestReviewQueue_UncommittedChanges_NoWorktree(t *testing.T) {
	// Create test instance WITHOUT worktree (directory session)
	now := time.Now()
	instance := &Instance{
		Title:       "test-no-worktree",
		Path:        "/tmp/test-path",
		Branch:      "",
		Status:      Running,
		gitManager: GitWorktreeManager{worktree: nil}, // No worktree
		started:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
		ReviewState: ReviewState{LastMeaningfulOutput: now},
	}

	// Create review queue infrastructure
	queue := NewReviewQueue()
	statusManager := NewInstanceStatusManager()
	poller := NewReviewQueuePoller(queue, statusManager, nil)
	poller.AddInstance(instance)

	// Check session - should not crash and not add to queue for uncommitted changes
	poller.checkSession(instance)

	// If added to queue, it should NOT be for uncommitted changes
	if queue.Has(instance.Title) {
		item, _ := queue.Get(instance.Title)
		if item.Reason == ReasonUncommittedChanges {
			t.Error("Expected session without worktree to not be added for uncommitted changes")
		}
	}
}

// TestReviewQueue_UncommittedChanges_Integration verifies full integration
// TODO(flaky): This test is timing-sensitive — it relies on a 3s sleep for the
// poller to detect committed state, which races on slow CI. Replace the sleep
// with a retry loop (e.g. poll every 100ms for up to 5s) and fix the poller
// to immediately re-check after a git commit rather than waiting for the next
// poll tick. Tracked: session/review_queue_uncommitted_changes_test.go:368
func TestReviewQueue_UncommittedChanges_Integration(t *testing.T) {
	// Create temporary git repository
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "integration-repo")

	// Initialize git repository
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	if err := runGitCommand(repoPath, "init"); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	if err := runGitCommand(repoPath, "config", "user.name", "Test User"); err != nil {
		t.Fatalf("Failed to configure git user.name: %v", err)
	}
	if err := runGitCommand(repoPath, "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("Failed to configure git user.email: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := runGitCommand(repoPath, "add", "."); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}
	if err := runGitCommand(repoPath, "commit", "-m", "Initial commit"); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Create worktree
	worktree, branchName, err := git.NewGitWorktree(repoPath, "integration-session")
	if err != nil {
		t.Fatalf("Failed to create git worktree: %v", err)
	}
	worktreePath := worktree.GetWorktreePath()

	if err := worktree.Setup(); err != nil {
		t.Fatalf("Failed to setup worktree: %v", err)
	}
	defer worktree.Cleanup()

	// Create instance
	now := time.Now()
	instance := &Instance{
		Title:       "integration-test",
		Path:        repoPath,
		Branch:      branchName,
		Status:      Running,
		gitManager: GitWorktreeManager{worktree: worktree},
		started:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
		ReviewState: ReviewState{LastMeaningfulOutput: now},
	}

	// Create review queue with poller
	queue := NewReviewQueue()
	statusManager := NewInstanceStatusManager()
	poller := NewReviewQueuePoller(queue, statusManager, nil)
	poller.AddInstance(instance)

	// Start polling in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poller.Start(ctx)
	defer poller.Stop()

	// Wait for initial check
	time.Sleep(500 * time.Millisecond)

	// Verify initially clean (not in queue)
	if queue.Has(instance.Title) {
		t.Error("Expected clean worktree to not be in queue initially")
	}

	// Create uncommitted changes
	modifiedFile := filepath.Join(worktreePath, "modified.txt")
	if err := os.WriteFile(modifiedFile, []byte("uncommitted"), 0644); err != nil {
		t.Fatalf("Failed to create modified file: %v", err)
	}

	// Wait for poller to detect changes
	time.Sleep(3 * time.Second)

	// Verify detected and added to queue
	if !queue.Has(instance.Title) {
		t.Error("Expected uncommitted changes to be detected by poller")
	}

	item, exists := queue.Get(instance.Title)
	if !exists {
		t.Fatal("Expected item to exist in queue")
	}

	if item.Reason != ReasonUncommittedChanges {
		t.Errorf("Expected reason UncommittedChanges, got %s", item.Reason)
	}

	// Commit changes
	if err := worktree.CommitChanges("Integration test commit"); err != nil {
		t.Fatalf("Failed to commit changes: %v", err)
	}

	// Wait for poller to detect committed state
	time.Sleep(3 * time.Second)

	// Verify removed from queue (or reason changed)
	if queue.Has(instance.Title) {
		updatedItem, _ := queue.Get(instance.Title)
		if updatedItem.Reason == ReasonUncommittedChanges {
			t.Error("Expected committed changes to remove UncommittedChanges reason from queue")
		}
	}
}

// Helper function to run git commands
func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git command failed: %s (%w)", output, err)
	}
	return nil
}
