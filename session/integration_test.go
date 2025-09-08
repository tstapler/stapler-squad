package session

import (
	"claude-squad/session/tmux"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
}

// testSessionRestoredInCorrectWorktree tests the core bug scenario:
// Session is killed and restored, should start in worktree not main repo
func testSessionRestoredInCorrectWorktree(t *testing.T) {
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	// Create instance with real dependencies
	instance, err := NewInstance(InstanceOptions{
		Title:   "test-recovery-session",
		Path:    tempRepo,
		Program: "pwd", // Use pwd to show working directory
	})
	require.NoError(t, err)

	// Start the instance (creates worktree and tmux session)
	err = instance.Start(true)
	require.NoError(t, err)
	require.True(t, instance.Started())

	// Get the worktree path for verification
	gitWorktree, err := instance.GetGitWorktree()
	require.NoError(t, err)
	expectedWorktreePath := gitWorktree.GetWorktreePath()

	// Capture initial session content to verify it starts in worktree
	initialContent, err := instance.Preview()
	require.NoError(t, err)
	require.Contains(t, initialContent, expectedWorktreePath,
		"Session should initially start in worktree directory")

	// Kill the tmux session to simulate the bug scenario by using tmux kill-session
	// We'll create a new tmux session to test with
	testSessionName := "claudesquad_" + instance.Title

	// Force kill the tmux session
	killCmd := exec.Command("tmux", "kill-session", "-t", testSessionName)
	_ = killCmd.Run() // Ignore error if session doesn't exist

	// Create a new tmux session to work with
	tmuxSession := tmux.NewTmuxSession(instance.Title, "pwd")
	instance.SetTmuxSession(tmuxSession)

	// Verify session doesn't exist initially (killed scenario)
	require.False(t, tmuxSession.DoesSessionExist(),
		"Tmux session should not exist after kill")

	// Now restore the session - this is where the bug would manifest
	// The fix ensures it uses RestoreWithWorkDir(worktreePath) instead of Restore()
	err = tmuxSession.RestoreWithWorkDir(expectedWorktreePath)
	require.NoError(t, err)

	// Give tmux time to start
	time.Sleep(200 * time.Millisecond)

	// Verify session was restored in correct directory
	restoredContent, err := tmuxSession.CapturePaneContent()
	require.NoError(t, err)
	require.Contains(t, restoredContent, expectedWorktreePath,
		"Restored session should start in worktree directory, not main repo")

	// Clean up
	_ = instance.Kill()
}

// testMultipleSessionsRestoreIndependently tests that multiple sessions
// can be restored independently in their correct worktrees
func testMultipleSessionsRestoreIndependently(t *testing.T) {
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	// Create two instances
	instance1, err := NewInstance(InstanceOptions{
		Title:   "test-session-1",
		Path:    tempRepo,
		Program: "pwd",
	})
	require.NoError(t, err)

	instance2, err := NewInstance(InstanceOptions{
		Title:   "test-session-2",
		Path:    tempRepo,
		Program: "pwd",
	})
	require.NoError(t, err)

	// Start both instances
	err = instance1.Start(true)
	require.NoError(t, err)

	err = instance2.Start(true)
	require.NoError(t, err)

	// Get worktree paths
	worktree1, _ := instance1.GetGitWorktree()
	worktree2, _ := instance2.GetGitWorktree()
	path1 := worktree1.GetWorktreePath()
	path2 := worktree2.GetWorktreePath()

	// Ensure they have different worktree paths
	require.NotEqual(t, path1, path2, "Sessions should have different worktree paths")

	// Kill both sessions
	sessionName1 := "claudesquad_" + instance1.Title
	sessionName2 := "claudesquad_" + instance2.Title

	killCmd1 := exec.Command("tmux", "kill-session", "-t", sessionName1)
	_ = killCmd1.Run()

	killCmd2 := exec.Command("tmux", "kill-session", "-t", sessionName2)
	_ = killCmd2.Run()

	// Create new tmux sessions to test restoration
	tmux1 := tmux.NewTmuxSession(instance1.Title, "pwd")
	tmux2 := tmux.NewTmuxSession(instance2.Title, "pwd")
	instance1.SetTmuxSession(tmux1)
	instance2.SetTmuxSession(tmux2)

	// Restore session 1
	err = tmux1.RestoreWithWorkDir(path1)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Verify session 1 restored in correct path
	content1, err := tmux1.CapturePaneContent()
	require.NoError(t, err)
	require.Contains(t, content1, path1, "Session 1 should restore in its worktree")

	// Restore session 2
	err = tmux2.RestoreWithWorkDir(path2)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Verify session 2 restored in correct path
	content2, err := tmux2.CapturePaneContent()
	require.NoError(t, err)
	require.Contains(t, content2, path2, "Session 2 should restore in its worktree")

	// Clean up
	_ = instance1.Kill()
	_ = instance2.Kill()
}

// testSessionRecoveryWithExistingChanges tests recovery when worktree has uncommitted changes
func testSessionRecoveryWithExistingChanges(t *testing.T) {
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	instance, err := NewInstance(InstanceOptions{
		Title:   "test-changes-session",
		Path:    tempRepo,
		Program: "pwd",
	})
	require.NoError(t, err)

	err = instance.Start(true)
	require.NoError(t, err)

	// Get worktree and create some changes
	gitWorktree, _ := instance.GetGitWorktree()
	worktreePath := gitWorktree.GetWorktreePath()

	// Create a file with changes
	testFile := filepath.Join(worktreePath, "test-change.txt")
	err = os.WriteFile(testFile, []byte("test changes"), 0644)
	require.NoError(t, err)

	// Kill and restore session
	sessionName := "claudesquad_" + instance.Title
	killCmd := exec.Command("tmux", "kill-session", "-t", sessionName)
	_ = killCmd.Run()

	// Create new tmux session for testing restoration
	tmuxSession := tmux.NewTmuxSession(instance.Title, "pwd")
	instance.SetTmuxSession(tmuxSession)

	err = tmuxSession.RestoreWithWorkDir(worktreePath)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Verify session restored in worktree with changes
	content, err := tmuxSession.CapturePaneContent()
	require.NoError(t, err)
	require.Contains(t, content, worktreePath,
		"Session should restore in worktree even with uncommitted changes")

	// Verify the test file exists in the correct location
	_, err = os.Stat(testFile)
	require.NoError(t, err, "Test file should exist in worktree")

	// Clean up
	_ = instance.Kill()
}

// testFallbackBehaviorWhenWorktreePathMissing tests backward compatibility
func testFallbackBehaviorWhenWorktreePathMissing(t *testing.T) {
	tempRepo := setupTestRepository(t)
	defer os.RemoveAll(tempRepo)

	// Create a tmux session without specifying worktree path
	session := tmux.NewTmuxSession("test-fallback-session", "pwd")

	// Test fallback behavior - should use current directory
	originalDir, _ := os.Getwd()

	// Change to temp directory
	err := os.Chdir(tempRepo)
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	// Use RestoreWithWorkDir with empty path (should fallback to current dir)
	err = session.RestoreWithWorkDir("")
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	content, err := session.CapturePaneContent()
	require.NoError(t, err)
	require.Contains(t, content, tempRepo,
		"Empty worktree path should fallback to current directory")

	// Clean up
	_ = session.Close()
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

		sessionName := "claudesquad_" + sessionTitle
		killCmd := exec.Command("tmux", "kill-session", "-t", sessionName)
		_ = killCmd.Run()

		// Create new tmux session for testing
		tmuxSession := tmux.NewTmuxSession(sessionTitle, "echo 'benchmark test'")
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
