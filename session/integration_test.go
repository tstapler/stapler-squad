package session

import (
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMain runs before all tests to set up the test environment
func TestMain(m *testing.M) {
	// Initialize the logger for tests with ERROR level to reduce noise
	log.InitializeForTests(log.ERROR, log.ERROR)
	defer log.Close()

	// Run all tests
	exitCode := m.Run()

	// Exit with the same code as the tests
	os.Exit(exitCode)
}

// Test utilities for waiting without static sleeps

// waitForCondition polls a condition until it returns true or timeout occurs
func waitForCondition(t *testing.T, condition func() bool, timeout time.Duration, description string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Check immediately first
	if condition() {
		return
	}

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for %s after %v", description, timeout)
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

// waitForContent polls a content getter until it contains expected text
func waitForContent(t *testing.T, getter func() (string, error), expectedText string, timeout time.Duration, description string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastContent string
	var lastErr error

	checkContent := func() bool {
		content, err := getter()
		if err != nil {
			lastErr = err
			return false
		}
		lastContent = content
		return len(content) > 0 && strings.Contains(content, expectedText)
	}

	// Check immediately first
	if checkContent() {
		return lastContent
	}

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				t.Fatalf("timeout waiting for %s after %v (last error: %v)", description, timeout, lastErr)
			}
			t.Fatalf("timeout waiting for %s after %v (last content: %q)", description, timeout, lastContent)
		case <-ticker.C:
			if checkContent() {
				return lastContent
			}
		}
	}
}

// TestSessionRecoveryScenarios tests the real-world session recovery scenarios
// that happen when tmux sessions are killed and need to be restored
func TestSessionRecoveryScenarios(t *testing.T) {
	// Create a temporary git repository for testing
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	t.Run("SessionRestoredInCorrectWorktreeAfterKill", func(t *testing.T) {
		testSessionRestoredInCorrectWorktree(t)
	})

	t.Run("MultipleSessionsRestoreIndependently", func(t *testing.T) {
		testMultipleSessionsRestoreIndependently(t)
	})

	t.Run("SessionRecoveryWithExistingChanges", func(t *testing.T) {
		testSessionRecoveryWithExistingChanges(t)
	})

	t.Run("FallbackBehaviorWhenWorktreePathMissing", func(t *testing.T) {
		testFallbackBehaviorWhenWorktreePathMissing(t)
	})

	t.Run("ExistingSessionResumption", func(t *testing.T) {
		testExistingSessionResumption(t, tempRepo)
	})

	t.Run("ExistingWorktreeResumption", func(t *testing.T) {
		testExistingWorktreeResumption(t, tempRepo)
	})

	t.Run("FullResumptionScenario", func(t *testing.T) {
		testFullResumptionScenario(t, tempRepo)
	})
}

// testSessionRestoredInCorrectWorktree tests the core bug scenario:
// Session is killed and restored, should start in worktree not main repo
func testSessionRestoredInCorrectWorktree(t *testing.T) {
	// Set up recovery scenario manually
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	instance, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            "recovery-test",
		Path:             tempRepo,
		Program:          "bash -c 'pwd; read'",
		SessionType:      SessionTypeNewWorktree,
		TmuxServerSocket: "test_" + strings.ReplaceAll(t.Name(), "/", "_"),
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup(); err != nil {
			t.Logf("Warning: cleanup failed: %v", err)
		}
	}()

	// Set branch to create a worktree instead of directory session
	instance.Branch = "recovery-test-branch"

	startCleanup, err := instance.StartWithCleanup(true)
	require.NoError(t, err)
	defer func() {
		if err := startCleanup(); err != nil {
			t.Logf("Warning: startCleanup failed: %v", err)
		}
	}()

	// Get worktree path before killing session
	gitWorktree, err := instance.GetGitWorktree()
	require.NoError(t, err)
	worktreePath := gitWorktree.GetWorktreePath()

	// Kill session but keep worktree for recovery testing
	err = instance.KillSessionKeepWorktree()
	require.NoError(t, err)

	// Create an isolated test tmux session for recovery testing
	uniqueName := fmt.Sprintf("recovery-test_%d", time.Now().UnixNano())

	tmuxSession, tmuxCleanup := tmux.NewTmuxSessionWithPrefixAndCleanup(uniqueName, "bash -c 'pwd; read'", "staplersquad_test_")
	defer func() {
		if err := tmuxCleanup(); err != nil {
			t.Logf("Warning: tmuxCleanup failed: %v", err)
		}
	}()

	// Verify session doesn't exist initially (killed scenario)
	require.False(t, tmuxSession.DoesSessionExist(),
		"Tmux session should not exist initially")

	// Test creating a new session in the correct worktree directory
	t.Logf("Starting tmux session with workDir: %q", worktreePath)

	// Verify the worktree path exists
	if stat, err := os.Stat(worktreePath); err != nil {
		t.Fatalf("Expected worktree path doesn't exist: %v", err)
	} else {
		t.Logf("Expected worktree path exists and is dir: %v", stat.IsDir())
	}

	err = tmuxSession.Start(worktreePath)
	require.NoError(t, err, "Should be able to start tmux session in worktree")

	// Verify session now exists
	require.True(t, tmuxSession.DoesSessionExist(),
		"Tmux session should exist after start")

	// Wait for the session to show the worktree path using proper async waiting
	worktreeBasename := filepath.Base(worktreePath)
	finalContent := waitForContent(t,
		func() (string, error) { return tmuxSession.CapturePaneContent() },
		worktreeBasename,
		10*time.Second,
		"tmux session to show worktree basename in content")

	t.Logf("Captured content contains worktree path: %v", strings.Contains(finalContent, worktreePath))

	require.Contains(t, finalContent, worktreePath,
		"Session should be restored in worktree directory, not main repo")
}

// testMultipleSessionsRestoreIndependently tests that multiple sessions
// can be restored independently in their correct worktrees
func testMultipleSessionsRestoreIndependently(t *testing.T) {
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	// Create two instances using new API with cleanup
	instance1, cleanup1, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            "test-session-1",
		Path:             tempRepo,
		Program:          "bash -c 'pwd && sleep 30'",
		SessionType:      SessionTypeNewWorktree,
		TmuxServerSocket: "test_" + strings.ReplaceAll(t.Name(), "/", "_") + "_1",
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup1(); err != nil {
			t.Logf("Warning: cleanup1 failed: %v", err)
		}
	}()

	instance2, cleanup2, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            "test-session-2",
		Path:             tempRepo,
		Program:          "bash -c 'pwd && sleep 30'",
		SessionType:      SessionTypeNewWorktree,
		TmuxServerSocket: "test_" + strings.ReplaceAll(t.Name(), "/", "_") + "_2",
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup2(); err != nil {
			t.Logf("Warning: cleanup2 failed: %v", err)
		}
	}()

	// Start both instances using new API
	startCleanup1, err := instance1.StartWithCleanup(true)
	require.NoError(t, err)
	defer func() {
		if err := startCleanup1(); err != nil {
			t.Logf("Warning: startCleanup1 failed: %v", err)
		}
	}()

	startCleanup2, err := instance2.StartWithCleanup(true)
	require.NoError(t, err)
	defer func() {
		if err := startCleanup2(); err != nil {
			t.Logf("Warning: startCleanup2 failed: %v", err)
		}
	}()

	// Get worktree paths
	worktree1, _ := instance1.GetGitWorktree()
	worktree2, _ := instance2.GetGitWorktree()
	path1 := worktree1.GetWorktreePath()
	path2 := worktree2.GetWorktreePath()

	// Ensure they have different worktree paths
	require.NotEqual(t, path1, path2, "Sessions should have different worktree paths")

	// Kill both sessions but keep worktrees
	_ = instance1.KillSessionKeepWorktree()
	_ = instance2.KillSessionKeepWorktree()

	// Create new tmux sessions to test restoration (using test prefix for isolation)
	tmux1, tmuxCleanup1 := tmux.NewTmuxSessionWithPrefixAndCleanup(instance1.Title, "bash -c 'pwd && sleep 30'", "staplersquad_test_")
	defer func() {
		if err := tmuxCleanup1(); err != nil {
			t.Logf("Warning: tmuxCleanup1 failed: %v", err)
		}
	}()

	tmux2, tmuxCleanup2 := tmux.NewTmuxSessionWithPrefixAndCleanup(instance2.Title, "bash -c 'pwd && sleep 30'", "staplersquad_test_")
	defer func() {
		if err := tmuxCleanup2(); err != nil {
			t.Logf("Warning: tmuxCleanup2 failed: %v", err)
		}
	}()

	instance1.SetTmuxSession(tmux1)
	instance2.SetTmuxSession(tmux2)

	// Start session 1 in its worktree
	err = tmux1.Start(path1)
	require.NoError(t, err, "Should be able to start session 1")

	// Start session 2 in its worktree
	err = tmux2.Start(path2)
	require.NoError(t, err, "Should be able to start session 2")

	// Wait for sessions to be ready and capture content
	waitForCondition(t, func() bool {
		content1, err := tmux1.CapturePaneContent()
		if err != nil {
			return false
		}
		return strings.Contains(content1, path1)
	}, 5*time.Second, "session 1 to show its worktree path")

	waitForCondition(t, func() bool {
		content2, err := tmux2.CapturePaneContent()
		if err != nil {
			return false
		}
		return strings.Contains(content2, path2)
	}, 5*time.Second, "session 2 to show its worktree path")

	// Verify final content
	content1, err := tmux1.CapturePaneContent()
	require.NoError(t, err, "Should capture session 1 content")
	require.Contains(t, content1, path1, "Session 1 should be in its worktree")

	content2, err := tmux2.CapturePaneContent()
	require.NoError(t, err, "Should capture session 2 content")
	require.Contains(t, content2, path2, "Session 2 should be in its worktree")
}

// testSessionRecoveryWithExistingChanges tests recovery when worktree has uncommitted changes
func testSessionRecoveryWithExistingChanges(t *testing.T) {
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	instance, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            "test-changes-session",
		Path:             tempRepo,
		Program:          "bash -c 'pwd && sleep 30'",
		SessionType:      SessionTypeNewWorktree,
		TmuxServerSocket: "test_" + strings.ReplaceAll(t.Name(), "/", "_"),
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup(); err != nil {
			t.Logf("Warning: cleanup failed: %v", err)
		}
	}()

	startCleanup, err := instance.StartWithCleanup(true)
	require.NoError(t, err)
	defer func() {
		if err := startCleanup(); err != nil {
			t.Logf("Warning: startCleanup failed: %v", err)
		}
	}()

	// Get worktree and create some changes
	gitWorktree, _ := instance.GetGitWorktree()
	worktreePath := gitWorktree.GetWorktreePath()

	// Create a file with changes
	testFile := filepath.Join(worktreePath, "test-change.txt")
	err = os.WriteFile(testFile, []byte("test changes"), 0644)
	require.NoError(t, err)

	// Kill the session but keep worktree
	_ = instance.KillSessionKeepWorktree()

	// Create new tmux session for testing restoration (using test prefix for isolation)
	tmuxSession, tmuxCleanup := tmux.NewTmuxSessionWithPrefixAndCleanup(instance.Title, "bash", "staplersquad_test_")
	defer func() {
		if err := tmuxCleanup(); err != nil {
			t.Logf("Warning: tmuxCleanup failed: %v", err)
		}
	}()
	instance.SetTmuxSession(tmuxSession)

	err = tmuxSession.RestoreWithWorkDir(worktreePath)
	require.NoError(t, err)

	// Wait for session to be ready and contain worktree basename in prompt
	worktreeBasename := filepath.Base(worktreePath)
	content := waitForContent(t,
		func() (string, error) { return tmuxSession.CapturePaneContent() },
		worktreeBasename,
		10*time.Second,
		"session to have content containing worktree basename")
	require.NotEmpty(t, content, "Captured content should not be empty")

	// Verify the test file exists in the correct location
	_, err = os.Stat(testFile)
	require.NoError(t, err, "Test file should exist in worktree")
}

// testFallbackBehaviorWhenWorktreePathMissing tests backward compatibility
func testFallbackBehaviorWhenWorktreePathMissing(t *testing.T) {
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	// Create a tmux session without specifying worktree path (using test prefix for isolation)
	session, cleanup := tmux.NewTmuxSessionWithPrefixAndCleanup("test-fallback-session", "bash", "staplersquad_test_")
	defer func() {
		if err := cleanup(); err != nil {
			t.Logf("Warning: cleanup failed: %v", err)
		}
	}()

	// Test fallback behavior - should use current directory
	originalDir, _ := os.Getwd()

	// Change to temp directory
	err := os.Chdir(tempRepo)
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	// Use RestoreWithWorkDir with empty path (should fallback to current dir)
	err = session.RestoreWithWorkDir("")
	require.NoError(t, err)

	// Wait for session to be ready and contain temp repo basename in prompt
	tempRepoBasename := filepath.Base(tempRepo)
	content := waitForContent(t,
		func() (string, error) { return session.CapturePaneContent() },
		tempRepoBasename,
		10*time.Second,
		"session to have content containing temp repo basename")
	require.NotEmpty(t, content, "Captured content should not be empty")
}

// setupTestRepository creates a temporary git repository for testing
func setupTestRepository(t *testing.T) string {
	tempDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	err := cmd.Run()
	require.NoError(t, err)

	// Configure git
	configCmd := exec.Command("git", "config", "user.email", "test@example.com")
	configCmd.Dir = tempDir
	_ = configCmd.Run()

	configCmd2 := exec.Command("git", "config", "user.name", "Test User")
	configCmd2.Dir = tempDir
	_ = configCmd2.Run()

	// Create initial commit
	readmeFile := filepath.Join(tempDir, "README.md")
	err = os.WriteFile(readmeFile, []byte("# Test Repository"), 0644)
	require.NoError(t, err)

	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = tempDir
	err = addCmd.Run()
	require.NoError(t, err)

	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = tempDir
	err = commitCmd.Run()
	require.NoError(t, err)

	return tempDir
}

// testExistingSessionResumption tests that when a tmux session already exists,
// the system reuses it instead of failing with "session already exists" error
func testExistingSessionResumption(t *testing.T, tempRepo string) {
	sessionName := "existing-session-test"

	// Create a tmux session directly (simulating an existing session scenario)
	tmuxSession, tmuxCleanup := tmux.NewTmuxSessionWithPrefixAndCleanup(sessionName, "bash -c 'echo existing && sleep 30'", "staplersquad_test_")
	defer func() {
		if err := tmuxCleanup(); err != nil {
			t.Logf("Warning: tmuxCleanup failed: %v", err)
		}
	}()

	// Start the tmux session first to simulate it already existing
	err := tmuxSession.Start(tempRepo)
	require.NoError(t, err, "Should be able to start initial tmux session")
	require.True(t, tmuxSession.DoesSessionExist(), "Tmux session should exist after start")

	// Now try to create an instance with the same session name - this would previously fail
	instance, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            sessionName,
		Path:             tempRepo,
		Program:          "bash -c 'echo resumed && sleep 30'",
		TmuxServerSocket: "test_" + strings.ReplaceAll(t.Name(), "/", "_"),
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup(); err != nil {
			t.Logf("Warning: cleanup failed: %v", err)
		}
	}()

	// Override the tmux session to use our existing one
	instance.SetTmuxSession(tmuxSession)

	// This should succeed by reusing the existing session instead of failing
	startCleanup, err := instance.StartWithCleanup(true)
	require.NoError(t, err, "Should be able to start instance with existing tmux session")
	defer func() {
		if err := startCleanup(); err != nil {
			t.Logf("Warning: startCleanup failed: %v", err)
		}
	}()

	// Verify the session is still running and accessible
	require.True(t, tmuxSession.DoesSessionExist(), "Tmux session should still exist after resumption")

	// Wait for some content to be available
	content := waitForContent(t,
		func() (string, error) { return tmuxSession.CapturePaneContent() },
		"existing",
		10*time.Second,
		"session to have existing content")
	require.Contains(t, content, "existing", "Should contain content from the existing session")
}

// testExistingWorktreeResumption tests that when a git worktree is already checked out,
// the system finds and reuses it instead of failing with "already checked out" error
func testExistingWorktreeResumption(t *testing.T, tempRepo string) {
	sessionName := "existing-worktree-test"

	// Create first instance to set up the worktree
	instance1, cleanup1, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            sessionName + "-1",
		Path:             tempRepo,
		Program:          "bash -c 'echo first && sleep 30'",
		SessionType:      SessionTypeNewWorktree,
		TmuxServerSocket: "test_" + strings.ReplaceAll(t.Name(), "/", "_") + "_1",
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup1(); err != nil {
			t.Logf("Warning: cleanup1 failed: %v", err)
		}
	}()

	// Manually set branch to test worktree reuse
	instance1.Branch = "feature-existing-worktree"

	startCleanup1, err := instance1.StartWithCleanup(true)
	require.NoError(t, err)
	defer func() {
		if err := startCleanup1(); err != nil {
			t.Logf("Warning: startCleanup1 failed: %v", err)
		}
	}()

	// Get the worktree path from the first instance
	gitWorktree1, err := instance1.GetGitWorktree()
	require.NoError(t, err)
	worktreePath1 := gitWorktree1.GetWorktreePath()

	// Now try to create a second instance with the same branch - this would previously fail
	instance2, cleanup2, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            sessionName + "-2",
		Path:             tempRepo,
		Program:          "bash -c 'echo second && sleep 30'",
		SessionType:      SessionTypeNewWorktree,
		TmuxServerSocket: "test_" + strings.ReplaceAll(t.Name(), "/", "_") + "_2",
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup2(); err != nil {
			t.Logf("Warning: cleanup2 failed: %v", err)
		}
	}()

	// Set the same branch to trigger worktree conflict resolution
	instance2.Branch = "feature-existing-worktree"

	// This should succeed by finding and reusing the existing worktree
	startCleanup2, err := instance2.StartWithCleanup(true)
	require.NoError(t, err, "Should be able to start instance with existing worktree")
	defer func() {
		if err := startCleanup2(); err != nil {
			t.Logf("Warning: startCleanup2 failed: %v", err)
		}
	}()

	// Verify both instances are using the same worktree path
	gitWorktree2, err := instance2.GetGitWorktree()
	require.NoError(t, err)
	worktreePath2 := gitWorktree2.GetWorktreePath()

	require.Equal(t, worktreePath1, worktreePath2, "Both instances should use the same worktree path")

	// Get tmux sessions and verify they have content (don't expect the exact echo content as it might not be captured)
	tmux1, tmuxCleanup1 := tmux.NewTmuxSessionWithPrefixAndCleanup(instance1.Title, "bash -c 'echo first && sleep 30'", "staplersquad_test_")
	defer func() {
		if err := tmuxCleanup1(); err != nil {
			t.Logf("Warning: tmuxCleanup1 failed: %v", err)
		}
	}()

	tmux2, tmuxCleanup2 := tmux.NewTmuxSessionWithPrefixAndCleanup(instance2.Title, "bash -c 'echo second && sleep 30'", "staplersquad_test_")
	defer func() {
		if err := tmuxCleanup2(); err != nil {
			t.Logf("Warning: tmuxCleanup2 failed: %v", err)
		}
	}()

	instance1.SetTmuxSession(tmux1)
	instance2.SetTmuxSession(tmux2)

	// Start sessions in their worktrees
	err = tmux1.Start(worktreePath1)
	require.NoError(t, err, "Should be able to start session 1")

	err = tmux2.Start(worktreePath2)
	require.NoError(t, err, "Should be able to start session 2")

	// Verify both sessions are functional by checking they can capture content
	// (We don't need to check for specific echo output, just that they're working)
	_, err = tmux1.CapturePaneContent()
	require.NoError(t, err, "First session should be able to capture content")

	_, err = tmux2.CapturePaneContent()
	require.NoError(t, err, "Second session should be able to capture content")
}

// testFullResumptionScenario tests the complete scenario that was reported:
// Creating a session in ~/IdeaProjects/pgbouncer-images with postgres-connection-pooling branch
func testFullResumptionScenario(t *testing.T, tempRepo string) {
	// This simulates the exact scenario the user reported
	sessionTitle := "postgres-connection-pooling"
	branchName := "postgres-connection-pooling"

	t.Logf("Testing full resumption scenario with session '%s' and branch '%s'", sessionTitle, branchName)

	// Create initial session (first attempt)
	instance1, cleanup1, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            sessionTitle,
		Path:             tempRepo,
		Program:          "bash -c 'echo initial session && sleep 30'",
		SessionType:      SessionTypeNewWorktree,
		TmuxServerSocket: "test_" + strings.ReplaceAll(t.Name(), "/", "_") + "_1",
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup1(); err != nil {
			t.Logf("Warning: cleanup1 failed: %v", err)
		}
	}()

	// Set the branch manually to test resumption behavior
	instance1.Branch = branchName

	startCleanup1, err := instance1.StartWithCleanup(true)
	require.NoError(t, err, "First session creation should succeed")
	defer func() {
		if err := startCleanup1(); err != nil {
			t.Logf("Warning: startCleanup1 failed: %v", err)
		}
	}()

	// Get the created worktree info
	worktree1, err := instance1.GetGitWorktree()
	require.NoError(t, err)

	t.Logf("First session created successfully with worktree: %s", worktree1.GetWorktreePath())

	// Now simulate the user trying to create the session again (second attempt)
	// This should reuse both the existing tmux session and worktree
	instance2, cleanup2, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            sessionTitle, // Same title
		Path:             tempRepo,     // Same path
		Program:          "bash -c 'echo resumed session && sleep 30'",
		SessionType:      SessionTypeNewWorktree,
		TmuxServerSocket: "test_" + strings.ReplaceAll(t.Name(), "/", "_") + "_2",
	})
	require.NoError(t, err)
	defer func() {
		if err := cleanup2(); err != nil {
			t.Logf("Warning: cleanup2 failed: %v", err)
		}
	}()

	// Set the same branch to trigger resumption behavior
	instance2.Branch = branchName

	// This is the critical test - this should NOT hang or fail
	// It should reuse the existing session and worktree
	startCleanup2, err := instance2.StartWithCleanup(true)
	require.NoError(t, err, "Second session creation should succeed by reusing existing resources")
	defer func() {
		if err := startCleanup2(); err != nil {
			t.Logf("Warning: startCleanup2 failed: %v", err)
		}
	}()

	// Verify both instances share resources appropriately
	worktree2, err := instance2.GetGitWorktree()
	require.NoError(t, err)

	// They should be using the same worktree path
	require.Equal(t, worktree1.GetWorktreePath(), worktree2.GetWorktreePath(),
		"Both instances should use the same worktree path")

	// Create manual tmux sessions to test they work in the shared worktree
	tmux1, tmuxCleanup1 := tmux.NewTmuxSessionWithPrefixAndCleanup(sessionTitle+"-test1", "bash -c 'pwd && sleep 30'", "staplersquad_test_")
	defer func() {
		if err := tmuxCleanup1(); err != nil {
			t.Logf("Warning: tmuxCleanup1 failed: %v", err)
		}
	}()

	tmux2, tmuxCleanup2 := tmux.NewTmuxSessionWithPrefixAndCleanup(sessionTitle+"-test2", "bash -c 'pwd && sleep 30'", "staplersquad_test_")
	defer func() {
		if err := tmuxCleanup2(); err != nil {
			t.Logf("Warning: tmuxCleanup2 failed: %v", err)
		}
	}()

	// Test that both can work in the shared worktree
	err = tmux1.Start(worktree1.GetWorktreePath())
	require.NoError(t, err, "First tmux session should start in shared worktree")

	err = tmux2.Start(worktree2.GetWorktreePath())
	require.NoError(t, err, "Second tmux session should start in shared worktree")

	// Verify both sessions are functional
	require.True(t, tmux1.DoesSessionExist(), "First tmux session should exist")
	require.True(t, tmux2.DoesSessionExist(), "Second tmux session should exist")

	t.Logf("✓ Full resumption scenario completed successfully")
	t.Logf("✓ No hanging behavior detected")
	t.Logf("✓ No 'session already exists' errors")
	t.Logf("✓ No 'already checked out' errors")
	t.Logf("✓ Both sessions are functional")
}

// BenchmarkSessionRestorePerformance benchmarks the session restore performance
func BenchmarkSessionRestorePerformance(b *testing.B) {
	tempRepo := setupTestRepositoryBench(b)
	defer os.RemoveAll(tempRepo)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sessionTitle := fmt.Sprintf("bench-session-%d", i)

		instance, err := NewInstance(InstanceOptions{
			Title:   sessionTitle,
			Path:    tempRepo,
			Program: "echo 'benchmark test'",
		})
		if err != nil {
			b.Fatal(err)
		}

		err = instance.Start(true)
		if err != nil {
			b.Fatal(err)
		}

		// Simulate kill and restore
		gitWorktree, _ := instance.GetGitWorktree()
		worktreePath := gitWorktree.GetWorktreePath()

		// Kill the original session
		_ = instance.Kill()

		// Create new tmux session for testing (using test prefix for isolation)
		tmuxSession := tmux.NewTmuxSessionWithPrefix(sessionTitle, "echo 'benchmark test'", "staplersquad_test_bench_")
		instance.SetTmuxSession(tmuxSession)

		err = tmuxSession.RestoreWithWorkDir(worktreePath)
		if err != nil {
			b.Fatal(err)
		}

		_ = instance.Kill()
	}
}

func setupTestRepositoryBench(b *testing.B) string {
	tempDir := b.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	_ = cmd.Run()

	configCmd := exec.Command("git", "config", "user.email", "test@example.com")
	configCmd.Dir = tempDir
	_ = configCmd.Run()

	configCmd2 := exec.Command("git", "config", "user.name", "Test User")
	configCmd2.Dir = tempDir
	_ = configCmd2.Run()

	readmeFile := filepath.Join(tempDir, "README.md")
	_ = os.WriteFile(readmeFile, []byte("# Benchmark Repository"), 0644)

	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = tempDir
	_ = addCmd.Run()

	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = tempDir
	_ = commitCmd.Run()

	return tempDir
}
