package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// initTestRepoForReviewQueue creates and commits a minimal git repository for the
// uncommitted-changes detection tests below.
//
// Uses go-git directly rather than shelling out — see
// the `prefer-go-git-over-subshells` skill.
func initTestRepoForReviewQueue(t *testing.T, repoPath string) {
	t.Helper()
	repo, err := gogit.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}
	if _, err := wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}
}

// TestReviewQueue_UncommittedChangesDetection verifies that uncommitted changes are detected
func TestReviewQueue_UncommittedChangesDetection(t *testing.T) {
	t.Parallel()
	// Create temporary git repository for testing
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "test-repo")

	// Initialize git repository
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize git repo, configure user, and create initial commit
	initTestRepoForReviewQueue(t, repoPath)

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
	// t.Cleanup, not defer: subtests below are t.Parallel(), so a plain
	// defer here would tear down the worktree before they actually execute.
	t.Cleanup(func() { worktree.Cleanup() })

	// Create test instance with worktree
	now := time.Now()
	instance := &Instance{
		Title:       "test-uncommitted-changes",
		Path:        repoPath,
		Branch:      branchName,
		Status:      Running,
		gitManager:  GitWorktreeManager{worktree: worktree},
		CreatedAt:   now,
		UpdatedAt:   now,
		ReviewState: ReviewState{LastMeaningfulOutput: now},
	}
	instance.started.Store(true)

	// Create review queue infrastructure
	queue := NewReviewQueue()
	statusManager := NewInstanceStatusManager()
	poller := NewReviewQueuePoller(queue, statusManager, nil)
	poller.AddInstance(instance)

	// Tests 1-3 below are NOT t.Parallel(): they form a single required
	// narrative sequence (clean -> dirty -> committed) sharing one queue,
	// poller, instance, and git worktree. t.Parallel() here previously let
	// "uncommitted_changes_detected" write modified.txt into the shared
	// worktree before "clean_worktree_not_added" had run its
	// checkSession/queue assertion, intermittently failing the "clean"
	// check — the subtests were never actually independent, so removing
	// t.Parallel() (rather than trying to isolate the shared worktree per
	// subtest) is the correct, scoped fix.

	// Test 1: Clean worktree (no uncommitted changes) should not be added to queue
	t.Run("clean_worktree_not_added", func(t *testing.T) {
		poller.checkSession(instance, nil)
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
		// Invalidate the dirty cache so the next checkSession re-runs git status.
		// In production this happens naturally after IsDirtyCacheTTL (30s) or IsDirtyCleanCacheTTL (5min); in tests
		// we invalidate explicitly since we know we just changed the filesystem.
		worktree.InvalidateDirtyCache()

		// Check session - should detect uncommitted changes
		poller.checkSession(instance, nil)

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
		poller.checkSession(instance, nil)

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
	t.Parallel()
	tests := []struct {
		name             string
		existingReason   AttentionReason
		existingPriority Priority
		shouldOverride   bool
		expectedReason   AttentionReason
		expectedPriority Priority
	}{
		{
			name:             "error_overrides_uncommitted",
			existingReason:   ReasonErrorState,
			existingPriority: PriorityUrgent,
			shouldOverride:   false,
			expectedReason:   ReasonErrorState,
			expectedPriority: PriorityUrgent,
		},
		{
			name:             "approval_overrides_uncommitted",
			existingReason:   ReasonApprovalPending,
			existingPriority: PriorityHigh,
			shouldOverride:   false,
			expectedReason:   ReasonApprovalPending,
			expectedPriority: PriorityHigh,
		},
		{
			name:             "input_overrides_uncommitted",
			existingReason:   ReasonInputRequired,
			existingPriority: PriorityMedium,
			shouldOverride:   false,
			expectedReason:   ReasonInputRequired,
			expectedPriority: PriorityMedium,
		},
		{
			name:             "uncommitted_can_override_idle",
			existingReason:   ReasonIdleTimeout,
			existingPriority: PriorityLow,
			shouldOverride:   true,
			expectedReason:   ReasonUncommittedChanges,
			expectedPriority: PriorityLow,
		},
		{
			name:             "uncommitted_can_override_complete",
			existingReason:   ReasonTaskComplete,
			existingPriority: PriorityLow,
			shouldOverride:   true,
			expectedReason:   ReasonUncommittedChanges,
			expectedPriority: PriorityLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
	reason := ReasonUncommittedChanges
	expected := "Uncommitted Changes"
	if reason.String() != expected {
		t.Errorf("Expected ReasonUncommittedChanges.String() to return %q, got %q",
			expected, reason.String())
	}
}

// TestReviewQueue_UncommittedChanges_NoWorktree verifies behavior without worktree
func TestReviewQueue_UncommittedChanges_NoWorktree(t *testing.T) {
	t.Parallel()
	// Create test instance WITHOUT worktree (directory session)
	now := time.Now()
	instance := &Instance{
		Title:       "test-no-worktree",
		Path:        "/tmp/test-path",
		Branch:      "",
		Status:      Running,
		gitManager:  GitWorktreeManager{worktree: nil}, // No worktree
		CreatedAt:   now,
		UpdatedAt:   now,
		ReviewState: ReviewState{LastMeaningfulOutput: now},
	}
	instance.started.Store(true)

	// Create review queue infrastructure
	queue := NewReviewQueue()
	statusManager := NewInstanceStatusManager()
	poller := NewReviewQueuePoller(queue, statusManager, nil)
	poller.AddInstance(instance)

	// Check session - should not crash and not add to queue for uncommitted changes
	poller.checkSession(instance, nil)

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
	t.Parallel()
	// Create temporary git repository
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "integration-repo")

	// Initialize git repository
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	initTestRepoForReviewQueue(t, repoPath)

	// Create worktree
	worktree, branchName, err := git.NewGitWorktree(repoPath, "integration-session")
	if err != nil {
		t.Fatalf("Failed to create git worktree: %v", err)
	}
	worktreePath := worktree.GetWorktreePath()

	if err := worktree.Setup(); err != nil {
		t.Fatalf("Failed to setup worktree: %v", err)
	}
	// t.Cleanup, not defer: subtests below are t.Parallel(), so a plain
	// defer here would tear down the worktree before they actually execute.
	t.Cleanup(func() { worktree.Cleanup() })

	// Create instance
	now := time.Now()
	instance := &Instance{
		Title:       "integration-test",
		Path:        repoPath,
		Branch:      branchName,
		Status:      Running,
		gitManager:  GitWorktreeManager{worktree: worktree},
		CreatedAt:   now,
		UpdatedAt:   now,
		ReviewState: ReviewState{LastMeaningfulOutput: now},
	}
	instance.started.Store(true)

	// Create review queue with poller
	queue := NewReviewQueue()
	statusManager := NewInstanceStatusManager()
	pollerCfg := DefaultReviewQueuePollerConfig()
	pollerCfg.PollInterval = 100 * time.Millisecond
	pollerCfg.SlowPollInterval = 100 * time.Millisecond
	poller := NewReviewQueuePollerWithConfig(queue, statusManager, nil, pollerCfg)
	poller.AddInstance(instance)

	// Start polling in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poller.Start(ctx)
	defer poller.Stop()

	// Wait for poller to start running
	startCfg := wait.FastWaitConfig()
	startCfg.Description = "poller running"
	if err := wait.WaitForCondition(func() bool {
		return poller.IsRunning()
	}, startCfg); err != nil {
		t.Fatalf("poller failed to start: %v", err)
	}

	// Verify initially clean (not in queue)
	if queue.Has(instance.Title) {
		t.Error("Expected clean worktree to not be in queue initially")
	}

	// Create uncommitted changes
	modifiedFile := filepath.Join(worktreePath, "modified.txt")
	if err := os.WriteFile(modifiedFile, []byte("uncommitted"), 0644); err != nil {
		t.Fatalf("Failed to create modified file: %v", err)
	}
	// Invalidate cache so the running poller picks up the change on its next tick
	// rather than waiting for IsDirtyCacheTTL (30s) or IsDirtyCleanCacheTTL (5min) to elapse.
	worktree.InvalidateDirtyCache()

	// Wait for poller to detect changes
	detectCfg := wait.DefaultWaitConfig()
	detectCfg.Description = "uncommitted changes detected"
	if err := wait.WaitForCondition(func() bool {
		return queue.Has(instance.Title)
	}, detectCfg); err != nil {
		t.Fatalf("poller failed to detect uncommitted changes: %v", err)
	}

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

	// Wait for poller to detect committed state (queue entry removed or reason changed)
	committedCfg := wait.DefaultWaitConfig()
	committedCfg.Description = "committed state detected"
	if err := wait.WaitForCondition(func() bool {
		if !queue.Has(instance.Title) {
			return true
		}
		updatedItem, _ := queue.Get(instance.Title)
		return updatedItem.Reason != ReasonUncommittedChanges
	}, committedCfg); err != nil {
		t.Fatalf("poller failed to detect committed state: %v", err)
	}

	// Verify removed from queue (or reason changed)
	if queue.Has(instance.Title) {
		updatedItem, _ := queue.Get(instance.Title)
		if updatedItem.Reason == ReasonUncommittedChanges {
			t.Error("Expected committed changes to remove UncommittedChanges reason from queue")
		}
	}
}
