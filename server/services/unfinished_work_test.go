package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// setupUWSFixture creates an UnfinishedWorkService wired with real SQLite storage,
// a Scanner, and a StateStore backed by a temp directory.
func setupUWSFixture(t *testing.T) (svc *UnfinishedWorkService, cleanup func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "uws-test-*")
	require.NoError(t, err)

	stateStore, err := unfinished.NewStateStore(tmpDir + "/unfinished-state.json")
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	scanner := unfinished.NewScanner(bus, stateStore)

	storage := createTestStorage(t)

	svc = NewUnfinishedWorkService(scanner, stateStore, bus, storage)

	cleanup = func() {
		bus.Close()
		os.RemoveAll(tmpDir)
	}
	return
}

// --------------------------------------------------------------------------
// GetUnfinishedWorkConfig
// --------------------------------------------------------------------------

// TestGetUnfinishedWorkConfig_ReturnsConfig verifies that GetUnfinishedWorkConfig
// returns a valid config without error on a fresh service.
func TestGetUnfinishedWorkConfig_ReturnsConfig(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	resp, err := svc.GetUnfinishedWorkConfig(
		context.Background(),
		connect.NewRequest(&sessionv1.GetUnfinishedWorkConfigRequest{}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Config, "config must not be nil")
}

// --------------------------------------------------------------------------
// UpdateUnfinishedWorkConfig
// --------------------------------------------------------------------------

// TestUpdateUnfinishedWorkConfig_NilConfigReturnsError verifies that passing a
// nil config returns CodeInvalidArgument.
func TestUpdateUnfinishedWorkConfig_NilConfigReturnsError(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.UpdateUnfinishedWorkConfig(
		context.Background(),
		connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
			Config: nil,
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpdateUnfinishedWorkConfig_ValidConfig verifies that a well-formed config
// update succeeds and the returned config reflects the submitted values.
func TestUpdateUnfinishedWorkConfig_ValidConfig(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	cfg := &sessionv1.UnfinishedWorkConfig{
		AutoSpiderSessions: false,
		WatchDirs:          []string{},
		PinnedRepos:        []string{},
	}

	resp, err := svc.UpdateUnfinishedWorkConfig(
		context.Background(),
		connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
			Config: cfg,
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Config)
	require.Equal(t, false, resp.Msg.Config.AutoSpiderSessions)
}

// --------------------------------------------------------------------------
// GetWorktreeAISummary
// --------------------------------------------------------------------------

// TestGetWorktreeAISummary_UnknownWorktree verifies that requesting an AI
// summary for a worktree that has not been scanned returns CodeNotFound.
func TestGetWorktreeAISummary_UnknownWorktree(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.GetWorktreeAISummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetWorktreeAISummaryRequest{
			RepoPath: "/nonexistent/repo",
			Branch:   "main",
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// QuickCommitPush
// --------------------------------------------------------------------------

// TestQuickCommitPush_EmptyCommitMessage verifies that an empty commit message
// returns CodeInvalidArgument.
func TestQuickCommitPush_EmptyCommitMessage(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.QuickCommitPush(
		context.Background(),
		connect.NewRequest(&sessionv1.QuickCommitPushRequest{
			RepoPath:      "/some/repo",
			Branch:        "main",
			CommitMessage: "",
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestQuickCommitPush_UnknownWorktree verifies that a valid commit message but
// an untracked worktree returns CodeNotFound.
func TestQuickCommitPush_UnknownWorktree(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.QuickCommitPush(
		context.Background(),
		connect.NewRequest(&sessionv1.QuickCommitPushRequest{
			RepoPath:      "/nonexistent/repo",
			Branch:        "main",
			CommitMessage: "wip: test commit",
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestQuickCommitPush_SkipsIgnoredTrackedFiles verifies the staging guard
// QuickCommitPush now applies before committing: a scaffolding file already
// tracked in the branch's history (git.ScaffoldingExcludePatterns) must never
// be restaged/recommitted, even by a plain `git add .`.
//
// This exercises the exact call QuickCommitPush makes
// (git.NewGitWorktreeFromStorage(...).StageAllExceptScaffolding()) directly
// against a real repo, rather than driving the full RPC: QuickCommitPush looks
// the worktree up via s.scanner.GetResultByKey, and Scanner's result cache is
// populated by its unexported background scan pipeline (real git-worktree
// discovery, workers, fsnotify) with no exported test seam reachable from this
// package. Standing that up would mean duplicating a large slice of
// session/unfinished's own test infra for a guard that's already covered here
// at the level QuickCommitPush actually delegates to — a deliberate scope
// decision rather than an oversight.
func TestQuickCommitPush_SkipsIgnoredTrackedFiles(t *testing.T) {
	repoDir := t.TempDir()

	runGit := func(args ...string) {
		t.Helper()
		cmd := safeexec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test"), 0o644))
	runGit("add", ".")
	runGit("commit", "-m", "initial commit")
	runGit("branch", "-M", "main")

	// Simulate a stale branch that already committed the scaffolding file.
	contextPath := filepath.Join(repoDir, ".backlog-context.md")
	require.NoError(t, os.WriteFile(contextPath, []byte("stale context"), 0o644))
	runGit("add", ".backlog-context.md")
	runGit("commit", "-m", "stale: commit scaffolding file")

	// A real change alongside it, as a live session would produce.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Updated"), 0o644))

	wt := git.NewGitWorktreeFromStorage(repoDir, repoDir, "", "main", "")
	require.NoError(t, wt.StageAllExceptScaffolding())

	staged, err := wt.HasStagedChanges()
	require.NoError(t, err)
	assert.True(t, staged, "the real README.md change (and the resulting untrack) must still be staged")

	// The scaffolding file must no longer be tracked in the index — its only
	// legitimate appearance in the staged diff at this point is as a removal
	// (git-rm-cached semantics), never as re-added content.
	lsFilesCmd := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "ls-files")
	lsOut, err := lsFilesCmd.CombinedOutput()
	require.NoError(t, err)
	assert.NotContains(t, string(lsOut), ".backlog-context.md", "scaffolding file must be untracked, not recommitted")

	statusCmd := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "diff", "--cached", "--name-only")
	out, err := statusCmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "README.md")
}

// --------------------------------------------------------------------------
// UndismissWorktree
// --------------------------------------------------------------------------

// TestUndismissWorktree_NoOpOnUnknown verifies that undismissing a worktree
// that was never dismissed returns no error (it is a no-op).
func TestUndismissWorktree_NoOpOnUnknown(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.UndismissWorktree(
		context.Background(),
		connect.NewRequest(&sessionv1.UndismissWorktreeRequest{
			RepoPath: "/some/repo",
			Branch:   "main",
		}),
	)

	// Undismiss of an unknown entry is expected to succeed (no-op).
	require.NoError(t, err)
}
