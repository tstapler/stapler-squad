package session

// instance_tagging_worktree_test.go covers Story 3.3.2: ReclassifyTagsAfterCreate's
// immediate-tag-on-Start() guarantee for brand-new sessions, across all three session types
// that route through setupFirstTimeWorktree, plus the no-deadlock structural guard.

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session/git"
)

func newBugfixTaggingEngine() *classifier.TaggingEngine {
	engine := classifier.NewTaggingEngine()
	engine.ReplaceRules([]classifier.TaggingRule{bugfixSeedRule()})
	return engine
}

func TestReclassifyTagsAfterCreate_should_ApplyTagImmediately_When_NewWorktreeSessionMatchesSeedRule(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	repoDir := t.TempDir()
	require.NoError(t, git.InitializeProjectDirectory(repoDir))

	title := fmt.Sprintf("test-tag-new-worktree-%d", time.Now().UnixNano())
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             repoDir,
		Program:          "sleep 300",
		SessionType:      SessionTypeNewWorktree,
		Branch:           "bugfix/pr-poller",
		TmuxPrefix:       fmt.Sprintf("test_tagworktree_%d_", time.Now().UnixNano()),
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() { _ = cleanup() }()

	inst.SetTaggingEngine(newBugfixTaggingEngine())

	require.NoError(t, inst.Start(true))

	tags := inst.GetTags()
	assert.Contains(t, tags, "Bugfix", "expected the seeded Bugfix rule to apply immediately after Start() returns for a new-worktree session")
	assert.Equal(t, "seed-bugfix", inst.RuleTagProvenance["Bugfix"])
}

func TestReclassifyTagsAfterCreate_should_ApplyTagImmediately_When_NewProjectOrExistingWorktreeSessionCreated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	t.Run("NewProject", func(t *testing.T) {
		base := t.TempDir()
		projectDir := base + "/brand-new-project"

		title := fmt.Sprintf("test-tag-new-project-%d", time.Now().UnixNano())
		inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
			Title:            title,
			Path:             projectDir,
			Program:          "sleep 300",
			SessionType:      SessionTypeNewProject,
			Branch:           "bugfix/new-project",
			TmuxPrefix:       fmt.Sprintf("test_tagnewproj_%d_", time.Now().UnixNano()),
			TmuxServerSocket: coldRestoreSocket(t),
		})
		require.NoError(t, err)
		defer func() { _ = cleanup() }()

		inst.SetTaggingEngine(newBugfixTaggingEngine())
		require.NoError(t, inst.Start(true))

		assert.Contains(t, inst.GetTags(), "Bugfix")
	})

	t.Run("ExistingWorktree", func(t *testing.T) {
		mainRepoDir := t.TempDir()
		require.NoError(t, git.InitializeProjectDirectory(mainRepoDir))

		worktree, _, err := git.NewGitWorktreeWithBranch(mainRepoDir, "existing-worktree-fixture", "bugfix/existing-worktree")
		require.NoError(t, err)
		require.NoError(t, worktree.Setup(), "must physically create the worktree via `git worktree add` before it can be reused as ExistingWorktree")

		title := fmt.Sprintf("test-tag-existing-worktree-%d", time.Now().UnixNano())
		inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
			Title:            title,
			Path:             mainRepoDir,
			Program:          "sleep 300",
			SessionType:      SessionTypeExistingWorktree,
			ExistingWorktree: worktree.GetWorktreePath(),
			TmuxPrefix:       fmt.Sprintf("test_tagexistwt_%d_", time.Now().UnixNano()),
			TmuxServerSocket: coldRestoreSocket(t),
		})
		require.NoError(t, err)
		defer func() { _ = cleanup() }()

		inst.SetTaggingEngine(newBugfixTaggingEngine())
		require.NoError(t, inst.Start(true))

		assert.Contains(t, inst.GetTags(), "Bugfix")
	})
}

// TestReclassifyTagsAfterCreate_should_NotDeadlock_When_CalledAfterSetupFirstTimeWorktreeReturns
// guards against a future refactor calling ReclassifyTagsAfterCreate from inside an
// already-i.mu-held context (which would deadlock): calling it directly, synchronously, right
// after setupFirstTimeWorktree() returns — exactly the call sequence Start() uses — must
// return promptly.
func TestReclassifyTagsAfterCreate_should_NotDeadlock_When_CalledAfterSetupFirstTimeWorktreeReturns(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	projectDir := base + "/deadlock-guard-project"

	inst, err := NewInstance(InstanceOptions{
		Title:       "deadlock-guard",
		Path:        projectDir,
		Program:     "claude",
		SessionType: SessionTypeNewProject,
		Branch:      "bugfix/deadlock-guard",
	})
	require.NoError(t, err)
	inst.SetTaggingEngine(newBugfixTaggingEngine())

	require.NoError(t, inst.setupFirstTimeWorktree())

	done := make(chan struct{})
	go func() {
		inst.ReclassifyTagsAfterCreate()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ReclassifyTagsAfterCreate did not return promptly after setupFirstTimeWorktree() -- possible deadlock")
	}

	assert.Contains(t, inst.GetTags(), "Bugfix")
}
