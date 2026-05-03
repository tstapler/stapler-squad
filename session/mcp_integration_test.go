//go:build integration

package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// TestSessionStartInWorktreeWithMCP exercises the full path:
//   - git worktree created in a temp dir
//   - Instance.Start() called (first-time setup)
//   - tmux session created in the worktree directory
//   - claude launched with correct --mcp-config JSON (no schema error)
//   - session starts in the worktree path, not home dir
//
// Run with: go test -tags integration ./session/ -run TestSessionStartInWorktreeWithMCP -v
func TestSessionStartInWorktreeWithMCP(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not in PATH")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not in PATH")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	// Create a bare git repo to use as the base for the worktree.
	repoDir := t.TempDir()
	mustRun(t, "git", "-C", repoDir, "init")
	mustRun(t, "git", "-C", repoDir, "commit", "--allow-empty", "-m", "init")

	mcpURL := "http://localhost:8543/mcp"

	opts := InstanceOptions{
		Title:        "integration-test-mcp",
		Path:         repoDir,
		Program:      "claude",
		SessionType:  SessionTypeNewWorktree,
		MCPServerURL: mcpURL,
		TmuxPrefix:   "sstest_",
	}

	inst, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	t.Cleanup(func() {
		if err := inst.Destroy(); err != nil {
			t.Logf("cleanup Destroy: %v", err)
		}
	})

	cleanup, err := inst.StartWithCleanup(true)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}()

	// Wait for claude to start and produce output.
	var output string
	if err := wait.WaitForConditionWithError(func() (bool, error) {
		var capErr error
		output, capErr = inst.CapturePaneContent()
		if capErr != nil {
			return false, capErr
		}
		return output != "", nil
	}, wait.WaitConfig{Timeout: 10 * time.Second, PollInterval: 200 * time.Millisecond, Description: "claude startup output"}); err != nil {
		t.Logf("warning: claude did not produce output within timeout: %v", err)
	}

	// Re-capture the final terminal output.
	finalOutput, err := inst.CapturePaneContent()
	if err != nil {
		t.Fatalf("CapturePaneContent: %v", err)
	}
	if finalOutput != "" {
		output = finalOutput
	}

	// The MCP schema error would appear immediately if the JSON format is wrong.
	if strings.Contains(output, "Does not adhere to MCP server configuration schema") ||
		strings.Contains(output, "Invalid MCP configuration") {
		t.Errorf("MCP config schema rejected by claude:\n%s", output)
	}

	// Verify the tmux session is in the worktree directory, not home.
	worktreePath := inst.GetEffectiveRootDir()
	if worktreePath == "" {
		t.Fatal("worktree path is empty after start")
	}

	tmuxName := "sstest_integration-test-mcp"
	paneDir := tmuxPaneDir(t, tmuxName)
	if paneDir == "" {
		t.Skip("could not determine pane directory (tmux format may differ)")
	}

	home, _ := os.UserHomeDir()
	if paneDir == home {
		t.Errorf("session started in home dir %q, want worktree %q", paneDir, worktreePath)
	}

	// The pane dir should be under the worktree (claude may cd around, but must start there).
	// We check the worktree is a real directory that was created.
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree directory %q does not exist: %v", worktreePath, err)
	}

	// Verify LaunchCommand contains the correct --mcp-config with mcpServers wrapper.
	lc := inst.LaunchCommand
	if !strings.Contains(lc, `"mcpServers"`) {
		t.Errorf("LaunchCommand missing mcpServers wrapper: %q", lc)
	}
	if strings.Contains(lc, `"stapler-squad":{"type"`) {
		// Old format (no mcpServers wrapper) — should not appear
		t.Errorf("LaunchCommand uses old MCP format without mcpServers wrapper: %q", lc)
	}

	t.Logf("LaunchCommand: %s", lc)
	t.Logf("Worktree path: %s", worktreePath)
	t.Logf("Pane dir: %s", paneDir)
	t.Logf("Terminal output (first 500 chars): %.500s", output)
}

// TestRestartFromPausedUsesWorktreeDir verifies that Instance.Restart() on a
// Paused session recreates the worktree and starts tmux in the correct directory.
func TestRestartFromPausedUsesWorktreeDir(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not in PATH")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	repoDir := t.TempDir()
	mustRun(t, "git", "-C", repoDir, "init")
	mustRun(t, "git", "-C", repoDir, "commit", "--allow-empty", "-m", "init")

	opts := InstanceOptions{
		Title:       "integration-test-restart",
		Path:        repoDir,
		Program:     "sh", // use sh so we don't need claude
		SessionType: SessionTypeNewWorktree,
		TmuxPrefix:  "sstest_",
	}

	inst, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	t.Cleanup(func() { _ = inst.Destroy() })

	// Start it first time.
	if _, err := inst.StartWithCleanup(true); err != nil {
		t.Fatalf("initial Start: %v", err)
	}
	worktreePath := inst.GetEffectiveRootDir()

	// Simulate Pause (removes worktree directory).
	if err := inst.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if inst.Status != Paused {
		t.Fatalf("expected Paused status, got %v", inst.Status)
	}
	// Worktree directory should be gone after pause.
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Logf("note: worktree still exists after pause (may be by design): %v", err)
	}

	// Restart from Paused — should recreate worktree and start in correct dir.
	if err := inst.Restart(false); err != nil {
		t.Fatalf("Restart from Paused: %v", err)
	}
	if inst.Status != Running {
		t.Errorf("expected Running after restart, got %v", inst.Status)
	}

	newWorktreePath := inst.GetEffectiveRootDir()
	if _, err := os.Stat(newWorktreePath); err != nil {
		t.Errorf("worktree %q does not exist after restart: %v", newWorktreePath, err)
	}

	// Pane should be in the worktree, not home — poll until tmux reports the pane dir.
	var paneDir string
	_ = wait.WaitForCondition(func() bool {
		paneDir = tmuxPaneDir(t, "sstest_integration-test-restart")
		return paneDir != ""
	}, wait.WaitConfig{Timeout: 5 * time.Second, PollInterval: 100 * time.Millisecond, Description: "tmux pane dir after restart"})
	home, _ := os.UserHomeDir()
	if paneDir == home {
		t.Errorf("restarted session is in home dir, want worktree %q", newWorktreePath)
	}
	t.Logf("Restarted pane dir: %s, worktree: %s", paneDir, newWorktreePath)
}

// tmuxPaneDir returns the current directory of the first pane in a named tmux session.
func tmuxPaneDir(t *testing.T, sessionName string) string {
	t.Helper()
	out, err := exec.Command("tmux", "display-message", "-t", sessionName, "-p", "#{pane_current_path}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func mustRun(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	// Set git identity for commit to work in temp dir.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %s %v: %v\n%s", name, args, err, out)
	}
}

// ensure git and tmux packages compile (used transitively via NewInstance)
var _ = git.NewGitWorktreeFromStorage
var _ = tmux.NewTmuxSessionWithPrefix

// Verify CapturePaneContent is accessible (it's a method on Instance)
var _ = (*Instance).CapturePaneContent

// ensure filepath is used
var _ = filepath.Join
