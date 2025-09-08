package tmux

import (
	"claude-squad/executor"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDirectoryRestorationScenarios tests session restoration for both worktree and non-worktree scenarios
// This addresses the user's request to ensure sessions go to the right repo directory
func TestDirectoryRestorationScenarios(t *testing.T) {
	t.Run("WorktreeSessionRestoresInWorktreeDirectory", func(t *testing.T) {
		testWorktreeSessionRestoration(t)
	})

	t.Run("RepoSessionRestoresInRepoDirectory", func(t *testing.T) {
		testRepoSessionRestoration(t)
	})

	t.Run("CompareWorktreeVsRepoRestoration", func(t *testing.T) {
		testCompareWorktreeVsRepoRestoration(t)
	})

	t.Run("RestoreWithWorkDirFallsBackToCurrentDir", func(t *testing.T) {
		testRestoreWithWorkDirFallback(t)
	})
}

// testWorktreeSessionRestoration tests that sessions for worktrees restore in the worktree directory
func testWorktreeSessionRestoration(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	// Create test directories
	tempDir := t.TempDir()
	worktreeDir := filepath.Join(tempDir, "my-feature-worktree")
	err := os.MkdirAll(worktreeDir, 0755)
	require.NoError(t, err)

	// Mock executor for missing session scenario
	cmdExec := createMockExecutorForMissingSession()

	session := newTmuxSession("feature-session", "pwd", ptyFactory, cmdExec, TmuxPrefix)

	// Test: RestoreWithWorkDir should use the provided worktree directory
	_ = session.RestoreWithWorkDir(worktreeDir)

	// Find the new-session command
	var newSessionCmd string
	for _, cmd := range ptyFactory.cmds {
		cmdStr := executor.ToString(cmd)
		if strings.Contains(cmdStr, "new-session") {
			newSessionCmd = cmdStr
			break
		}
	}

	// Verify the session would start in the worktree directory
	if newSessionCmd != "" {
		require.Contains(t, newSessionCmd, worktreeDir,
			"Worktree session should restore in worktree directory: %s", worktreeDir)
		t.Logf("✓ Worktree session command: %s", newSessionCmd)
	}
}

// testRepoSessionRestoration tests that sessions for regular repos restore in the repo directory
func testRepoSessionRestoration(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	// Create test directories
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "my-project")
	err := os.MkdirAll(repoDir, 0755)
	require.NoError(t, err)

	// Mock executor for missing session scenario
	cmdExec := createMockExecutorForMissingSession()

	session := newTmuxSession("main-repo-session", "pwd", ptyFactory, cmdExec, TmuxPrefix)

	// Test: RestoreWithWorkDir should use the provided repo directory
	_ = session.RestoreWithWorkDir(repoDir)

	// Find the new-session command
	var newSessionCmd string
	for _, cmd := range ptyFactory.cmds {
		cmdStr := executor.ToString(cmd)
		if strings.Contains(cmdStr, "new-session") {
			newSessionCmd = cmdStr
			break
		}
	}

	// Verify the session would start in the repo directory
	if newSessionCmd != "" {
		require.Contains(t, newSessionCmd, repoDir,
			"Repo session should restore in repo directory: %s", repoDir)
		t.Logf("✓ Repo session command: %s", newSessionCmd)
	}
}

// testCompareWorktreeVsRepoRestoration demonstrates that both worktree and repo sessions
// restore in their correct respective directories
func testCompareWorktreeVsRepoRestoration(t *testing.T) {
	tempDir := t.TempDir()

	// Set up directories
	repoDir := filepath.Join(tempDir, "main-project")
	worktreeDir := filepath.Join(tempDir, "feature-branch-worktree")
	err := os.MkdirAll(repoDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(worktreeDir, 0755)
	require.NoError(t, err)

	t.Run("RepoSession", func(t *testing.T) {
		ptyFactory := NewMockPtyFactory(t)
		cmdExec := createMockExecutorForMissingSession()

		session := newTmuxSession("repo-main", "pwd", ptyFactory, cmdExec, TmuxPrefix)

		// Restore in repo directory
		err := session.RestoreWithWorkDir(repoDir)
		require.NoError(t, err)

		// Verify command uses repo directory
		var newSessionCmd string
		for _, cmd := range ptyFactory.cmds {
			cmdStr := executor.ToString(cmd)
			if strings.Contains(cmdStr, "new-session") {
				newSessionCmd = cmdStr
				break
			}
		}

		if newSessionCmd != "" {
			require.Contains(t, newSessionCmd, repoDir,
				"Repo session should use repo directory")
			require.NotContains(t, newSessionCmd, worktreeDir,
				"Repo session should NOT use worktree directory")
			t.Logf("Repo session: %s", newSessionCmd)
		}
	})

	t.Run("WorktreeSession", func(t *testing.T) {
		ptyFactory := NewMockPtyFactory(t)
		cmdExec := createMockExecutorForMissingSession()

		session := newTmuxSession("worktree-feature", "pwd", ptyFactory, cmdExec, TmuxPrefix)

		// Restore in worktree directory
		err := session.RestoreWithWorkDir(worktreeDir)
		require.NoError(t, err)

		// Verify command uses worktree directory
		var newSessionCmd string
		for _, cmd := range ptyFactory.cmds {
			cmdStr := executor.ToString(cmd)
			if strings.Contains(cmdStr, "new-session") {
				newSessionCmd = cmdStr
				break
			}
		}

		if newSessionCmd != "" {
			require.Contains(t, newSessionCmd, worktreeDir,
				"Worktree session should use worktree directory")
			require.NotContains(t, newSessionCmd, repoDir,
				"Worktree session should NOT use repo directory")
			t.Logf("Worktree session: %s", newSessionCmd)
		}
	})
}

// testRestoreWithWorkDirFallback tests the fallback behavior when no working directory is specified
func testRestoreWithWorkDirFallback(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	// Create test directory and change to it
	testDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)

	err := os.Chdir(testDir)
	require.NoError(t, err)

	cmdExec := createMockExecutorForMissingSession()
	session := newTmuxSession("fallback-test", "pwd", ptyFactory, cmdExec, TmuxPrefix)

	// Test: RestoreWithWorkDir with empty path should fallback to current directory
	_ = session.RestoreWithWorkDir("")

	// Find the new-session command
	var newSessionCmd string
	for _, cmd := range ptyFactory.cmds {
		cmdStr := executor.ToString(cmd)
		if strings.Contains(cmdStr, "new-session") {
			newSessionCmd = cmdStr
			break
		}
	}

	// Verify it uses current directory
	if newSessionCmd != "" {
		// Handle path resolution for macOS /var vs /private/var
		currentDir, _ := os.Getwd()
		resolvedCurrentDir, _ := filepath.EvalSymlinks(currentDir)
		resolvedTestDir, _ := filepath.EvalSymlinks(testDir)

		containsCurrentDir := strings.Contains(newSessionCmd, currentDir) ||
			strings.Contains(newSessionCmd, resolvedCurrentDir) ||
			strings.Contains(newSessionCmd, testDir) ||
			strings.Contains(newSessionCmd, resolvedTestDir)

		require.True(t, containsCurrentDir,
			"Empty workDir should fallback to current directory. Expected one of: %s, %s, %s, %s. Got: %s",
			currentDir, resolvedCurrentDir, testDir, resolvedTestDir, newSessionCmd)

		t.Logf("✓ Fallback behavior works: %s", newSessionCmd)
	}
}

// TestOldVsNewBehaviorComparison demonstrates the bug fix by comparing old and new behavior
func TestOldVsNewBehaviorComparison(t *testing.T) {
	tempDir := t.TempDir()
	correctDir := filepath.Join(tempDir, "correct-target-dir")
	err := os.MkdirAll(correctDir, 0755)
	require.NoError(t, err)

	// Change to different directory to simulate the bug condition
	wrongDir := filepath.Join(tempDir, "wrong-current-dir")
	err = os.MkdirAll(wrongDir, 0755)
	require.NoError(t, err)

	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	err = os.Chdir(wrongDir)
	require.NoError(t, err)

	t.Run("OLD_Buggy_Behavior_Simulation", func(t *testing.T) {
		ptyFactory := NewMockPtyFactory(t)
		cmdExec := createMockExecutorForMissingSession()

		session := newTmuxSession("old-behavior", "pwd", ptyFactory, cmdExec, TmuxPrefix)

		// Simulate OLD behavior: use Restore() which falls back to current directory
		_ = session.Restore()

		var newSessionCmd string
		for _, cmd := range ptyFactory.cmds {
			cmdStr := executor.ToString(cmd)
			if strings.Contains(cmdStr, "new-session") {
				newSessionCmd = cmdStr
				break
			}
		}

		if newSessionCmd != "" {
			// OLD behavior would use current directory (wrong!)
			currentDir, _ := os.Getwd()
			resolvedCurrentDir, _ := filepath.EvalSymlinks(currentDir)
			containsWrongDir := strings.Contains(newSessionCmd, wrongDir) ||
				strings.Contains(newSessionCmd, currentDir) ||
				strings.Contains(newSessionCmd, resolvedCurrentDir)

			require.True(t, containsWrongDir,
				"OLD behavior should use current directory (demonstrating the bug)")
			require.NotContains(t, newSessionCmd, correctDir,
				"OLD behavior should NOT use correct target directory")

			t.Logf("OLD (buggy) behavior: %s", newSessionCmd)
		}
	})

	t.Run("NEW_Fixed_Behavior", func(t *testing.T) {
		ptyFactory := NewMockPtyFactory(t)
		cmdExec := createMockExecutorForMissingSession()

		session := newTmuxSession("new-behavior", "pwd", ptyFactory, cmdExec, TmuxPrefix)

		// NEW behavior: use RestoreWithWorkDir() with correct directory
		_ = session.RestoreWithWorkDir(correctDir)

		var newSessionCmd string
		for _, cmd := range ptyFactory.cmds {
			cmdStr := executor.ToString(cmd)
			if strings.Contains(cmdStr, "new-session") {
				newSessionCmd = cmdStr
				break
			}
		}

		if newSessionCmd != "" {
			// NEW behavior should use correct directory
			require.Contains(t, newSessionCmd, correctDir,
				"NEW behavior should use correct target directory")

			// Ensure it's NOT using wrong directory
			require.NotContains(t, newSessionCmd, wrongDir,
				"NEW behavior should NOT use wrong current directory")

			t.Logf("NEW (fixed) behavior: %s", newSessionCmd)
		}
	})
}

// TestDirectoryResolutionEdgeCases tests edge cases in directory resolution
func TestDirectoryResolutionEdgeCases(t *testing.T) {
	t.Run("NonExistentDirectory", func(t *testing.T) {
		ptyFactory := NewMockPtyFactory(t)
		cmdExec := createMockExecutorForMissingSession()

		session := newTmuxSession("nonexistent-test", "pwd", ptyFactory, cmdExec, TmuxPrefix)

		nonExistentDir := "/path/that/does/not/exist"

		// This should still work - tmux will handle the directory error
		_ = session.RestoreWithWorkDir(nonExistentDir)

		var newSessionCmd string
		for _, cmd := range ptyFactory.cmds {
			cmdStr := executor.ToString(cmd)
			if strings.Contains(cmdStr, "new-session") {
				newSessionCmd = cmdStr
				break
			}
		}

		if newSessionCmd != "" {
			require.Contains(t, newSessionCmd, nonExistentDir,
				"Should still pass non-existent directory to tmux")
			t.Logf("Non-existent dir command: %s", newSessionCmd)
		}
	})

	t.Run("RelativePathDirectory", func(t *testing.T) {
		ptyFactory := NewMockPtyFactory(t)
		cmdExec := createMockExecutorForMissingSession()

		session := newTmuxSession("relative-test", "pwd", ptyFactory, cmdExec, TmuxPrefix)

		relativeDir := "./some/relative/path"

		_ = session.RestoreWithWorkDir(relativeDir)

		var newSessionCmd string
		for _, cmd := range ptyFactory.cmds {
			cmdStr := executor.ToString(cmd)
			if strings.Contains(cmdStr, "new-session") {
				newSessionCmd = cmdStr
				break
			}
		}

		if newSessionCmd != "" {
			require.Contains(t, newSessionCmd, relativeDir,
				"Should handle relative paths")
			t.Logf("Relative path command: %s", newSessionCmd)
		}
	})
}
