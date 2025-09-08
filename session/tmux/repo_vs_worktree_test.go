package tmux

import (
	"claude-squad/executor"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRepoVsWorktreeDirectoryHandling specifically tests that sessions restore
// in the correct directories for both regular repos and worktrees
func TestRepoVsWorktreeDirectoryHandling(t *testing.T) {
	t.Run("RegularRepoSessionRestoration", func(t *testing.T) {
		testRegularRepoSessionRestoration(t)
	})

	t.Run("WorktreeSessionRestoration", func(t *testing.T) {
		testWorktreeSessionRestorationDetailed(t)
	})

	t.Run("MixedScenarioValidation", func(t *testing.T) {
		testMixedScenarioValidation(t)
	})
}

// testRegularRepoSessionRestoration verifies that non-worktree sessions
// (regular repository sessions) restore in the correct repository directory
func testRegularRepoSessionRestoration(t *testing.T) {
	tempDir := t.TempDir()

	// Simulate a regular repository directory structure
	repoDir := filepath.Join(tempDir, "my-main-project")
	err := os.MkdirAll(repoDir, 0755)
	require.NoError(t, err)

	// Create some typical repo files
	readmeFile := filepath.Join(repoDir, "README.md")
	err = os.WriteFile(readmeFile, []byte("# Main Project"), 0644)
	require.NoError(t, err)

	ptyFactory := NewMockPtyFactory(t)
	cmdExec := createMockExecutorForMissingSession()

	// Create session for main repo (not a worktree)
	session := newTmuxSession("main-project-session", "pwd", ptyFactory, cmdExec, TmuxPrefix)

	// Test: When restoring a regular repo session, it should start in the repo directory
	_ = session.RestoreWithWorkDir(repoDir)

	// Verify the session would be created with the correct working directory
	var newSessionCmd string
	for _, cmd := range ptyFactory.cmds {
		cmdStr := executor.ToString(cmd)
		if strings.Contains(cmdStr, "new-session") {
			newSessionCmd = cmdStr
			break
		}
	}

	require.NotEmpty(t, newSessionCmd, "Should have attempted to create a new session")
	require.Contains(t, newSessionCmd, repoDir,
		"Regular repo session should start in repo directory: %s", repoDir)

	// Verify it contains the expected tmux command structure
	expectedParts := []string{
		"tmux",
		"new-session",
		"-d",
		"-s",
		"claudesquad_main-project-session",
		"-c",
		repoDir,
		"pwd",
	}

	for _, part := range expectedParts {
		require.Contains(t, newSessionCmd, part,
			"Command should contain: %s", part)
	}

	t.Logf("✓ Regular repo session command: %s", newSessionCmd)
}

// testWorktreeSessionRestorationDetailed verifies that worktree sessions
// restore in the correct worktree directory (not main repo)
func testWorktreeSessionRestorationDetailed(t *testing.T) {
	tempDir := t.TempDir()

	// Simulate main repo and worktree directory structure
	mainRepoDir := filepath.Join(tempDir, "main-repo")
	worktreeDir := filepath.Join(tempDir, "feature-branch-worktree")

	err := os.MkdirAll(mainRepoDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(worktreeDir, 0755)
	require.NoError(t, err)

	// Create files to distinguish the directories
	mainReadme := filepath.Join(mainRepoDir, "README.md")
	err = os.WriteFile(mainReadme, []byte("# Main Repo"), 0644)
	require.NoError(t, err)

	featureFile := filepath.Join(worktreeDir, "feature.go")
	err = os.WriteFile(featureFile, []byte("// Feature implementation"), 0644)
	require.NoError(t, err)

	ptyFactory := NewMockPtyFactory(t)
	cmdExec := createMockExecutorForMissingSession()

	// Create session for worktree (feature branch)
	session := newTmuxSession("feature-branch-session", "pwd", ptyFactory, cmdExec, TmuxPrefix)

	// Test: When restoring a worktree session, it should start in the worktree directory
	_ = session.RestoreWithWorkDir(worktreeDir)

	// Verify the session would be created with the correct working directory
	var newSessionCmd string
	for _, cmd := range ptyFactory.cmds {
		cmdStr := executor.ToString(cmd)
		if strings.Contains(cmdStr, "new-session") {
			newSessionCmd = cmdStr
			break
		}
	}

	require.NotEmpty(t, newSessionCmd, "Should have attempted to create a new session")
	require.Contains(t, newSessionCmd, worktreeDir,
		"Worktree session should start in worktree directory: %s", worktreeDir)
	require.NotContains(t, newSessionCmd, mainRepoDir,
		"Worktree session should NOT start in main repo directory: %s", mainRepoDir)

	t.Logf("✓ Worktree session command: %s", newSessionCmd)
}

// testMixedScenarioValidation tests both scenarios in the same test to ensure
// they work correctly and independently
func testMixedScenarioValidation(t *testing.T) {
	tempDir := t.TempDir()

	// Set up directories
	mainRepoDir := filepath.Join(tempDir, "project-main")
	worktreeDir := filepath.Join(tempDir, "project-feature-x")

	err := os.MkdirAll(mainRepoDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(worktreeDir, 0755)
	require.NoError(t, err)

	// Test both scenarios
	scenarios := []struct {
		name             string
		sessionName      string
		targetDir        string
		shouldNotContain string
	}{
		{
			name:             "MainRepoSession",
			sessionName:      "main-development",
			targetDir:        mainRepoDir,
			shouldNotContain: worktreeDir,
		},
		{
			name:             "FeatureWorktreeSession",
			sessionName:      "feature-x-development",
			targetDir:        worktreeDir,
			shouldNotContain: mainRepoDir,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			ptyFactory := NewMockPtyFactory(t)
			cmdExec := createMockExecutorForMissingSession()

			session := newTmuxSession(scenario.sessionName, "pwd", ptyFactory, cmdExec, TmuxPrefix)

			// Restore in the target directory
			_ = session.RestoreWithWorkDir(scenario.targetDir)

			// Find and verify the command
			var newSessionCmd string
			for _, cmd := range ptyFactory.cmds {
				cmdStr := executor.ToString(cmd)
				if strings.Contains(cmdStr, "new-session") {
					newSessionCmd = cmdStr
					break
				}
			}

			require.NotEmpty(t, newSessionCmd, "Should create new session command")
			require.Contains(t, newSessionCmd, scenario.targetDir,
				"Session should use target directory: %s", scenario.targetDir)
			require.NotContains(t, newSessionCmd, scenario.shouldNotContain,
				"Session should NOT use incorrect directory: %s", scenario.shouldNotContain)

			t.Logf("✓ %s: %s", scenario.name, newSessionCmd)
		})
	}
}

// TestSessionTypeClassification tests that we can identify and handle
// different types of session restoration scenarios
func TestSessionTypeClassification(t *testing.T) {
	tempDir := t.TempDir()

	scenarios := []struct {
		name        string
		description string
		sessionName string
		targetDir   string
		dirSetup    func() string
	}{
		{
			name:        "MainRepository",
			description: "Session for main branch/repository work",
			sessionName: "main-branch-work",
			dirSetup: func() string {
				dir := filepath.Join(tempDir, "main-project")
				os.MkdirAll(dir, 0755)
				// Simulate .git directory
				os.MkdirAll(filepath.Join(dir, ".git"), 0755)
				return dir
			},
		},
		{
			name:        "FeatureWorktree",
			description: "Session for feature branch worktree",
			sessionName: "feature-auth-system",
			dirSetup: func() string {
				dir := filepath.Join(tempDir, "auth-feature-worktree")
				os.MkdirAll(dir, 0755)
				// Simulate .git file (pointing to main repo)
				os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../main-project/.git/worktrees/auth"), 0644)
				return dir
			},
		},
		{
			name:        "BugfixWorktree",
			description: "Session for bugfix branch worktree",
			sessionName: "bugfix-login-error",
			dirSetup: func() string {
				dir := filepath.Join(tempDir, "bugfix-worktree")
				os.MkdirAll(dir, 0755)
				// Different structure to test flexibility
				return dir
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			targetDir := scenario.dirSetup()

			ptyFactory := NewMockPtyFactory(t)
			cmdExec := createMockExecutorForMissingSession()

			session := newTmuxSession(scenario.sessionName, "pwd", ptyFactory, cmdExec, TmuxPrefix)

			// Test restoration
			_ = session.RestoreWithWorkDir(targetDir)

			// Verify correct directory usage
			var newSessionCmd string
			for _, cmd := range ptyFactory.cmds {
				cmdStr := executor.ToString(cmd)
				if strings.Contains(cmdStr, "new-session") {
					newSessionCmd = cmdStr
					break
				}
			}

			require.NotEmpty(t, newSessionCmd, "Should create session command")
			require.Contains(t, newSessionCmd, targetDir,
				"%s should use correct directory: %s", scenario.description, targetDir)

			t.Logf("✓ %s (%s): %s", scenario.name, scenario.description, newSessionCmd)
		})
	}
}

// TestDirectoryPathEdgeCases tests various edge cases in directory path handling
func TestDirectoryPathEdgeCases(t *testing.T) {
	t.Run("AbsoluteVsRelativePaths", func(t *testing.T) {
		tempDir := t.TempDir()
		absoluteDir := filepath.Join(tempDir, "absolute-path-test")
		err := os.MkdirAll(absoluteDir, 0755)
		require.NoError(t, err)

		// Test absolute path
		ptyFactory1 := NewMockPtyFactory(t)
		session1 := newTmuxSession("absolute-test", "pwd", ptyFactory1, createMockExecutorForMissingSession(), TmuxPrefix)
		_ = session1.RestoreWithWorkDir(absoluteDir)

		// Find command
		var cmd1 string
		for _, cmd := range ptyFactory1.cmds {
			cmdStr := executor.ToString(cmd)
			if strings.Contains(cmdStr, "new-session") {
				cmd1 = cmdStr
				break
			}
		}

		require.Contains(t, cmd1, absoluteDir, "Should handle absolute paths")
		t.Logf("Absolute path: %s", cmd1)

		// Test relative path
		originalDir, _ := os.Getwd()
		defer os.Chdir(originalDir)
		os.Chdir(tempDir)

		relativeDir := "./relative-test"
		os.MkdirAll(relativeDir, 0755)

		ptyFactory2 := NewMockPtyFactory(t)
		session2 := newTmuxSession("relative-test", "pwd", ptyFactory2, createMockExecutorForMissingSession(), TmuxPrefix)
		_ = session2.RestoreWithWorkDir(relativeDir)

		var cmd2 string
		for _, cmd := range ptyFactory2.cmds {
			cmdStr := executor.ToString(cmd)
			if strings.Contains(cmdStr, "new-session") {
				cmd2 = cmdStr
				break
			}
		}

		require.Contains(t, cmd2, relativeDir, "Should handle relative paths")
		t.Logf("Relative path: %s", cmd2)
	})

	t.Run("PathsWithSpaces", func(t *testing.T) {
		tempDir := t.TempDir()
		spaceDir := filepath.Join(tempDir, "directory with spaces")
		err := os.MkdirAll(spaceDir, 0755)
		require.NoError(t, err)

		ptyFactory := NewMockPtyFactory(t)
		session := newTmuxSession("space-test", "pwd", ptyFactory, createMockExecutorForMissingSession(), TmuxPrefix)
		_ = session.RestoreWithWorkDir(spaceDir)

		var newSessionCmd string
		for _, cmd := range ptyFactory.cmds {
			cmdStr := executor.ToString(cmd)
			if strings.Contains(cmdStr, "new-session") {
				newSessionCmd = cmdStr
				break
			}
		}

		require.Contains(t, newSessionCmd, spaceDir, "Should handle paths with spaces")
		t.Logf("Path with spaces: %s", newSessionCmd)
	})
}
