package session

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"
)

// TestInstance_Start_should_ProceedToSpawn_When_WorktreeResolvesCorrectly is
// Epic 4.4's non-regression companion to the structural-guard failure test
// above: a normal SessionTypeNewWorktree session, with no forced failure,
// must still resolve a genuinely distinct worktree and reach Active.
func TestInstance_Start_should_ProceedToSpawn_When_WorktreeResolvesCorrectly(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	fixturetest.CheckTmuxAvailable(t)

	repoDir := t.TempDir()
	require.NoError(t, git.InitializeProjectDirectory(repoDir))

	title := fmt.Sprintf("guard-structural-ok-%d", time.Now().UnixNano())
	inst, cleanup, err := instance.NewInstanceWithCleanup(instance.InstanceOptions{
		Title:            title,
		Path:             repoDir,
		Program:          "sleep 300",
		SessionType:      domain.SessionTypeNewWorktree,
		Branch:           "feature-guard-ok",
		TmuxPrefix:       fmt.Sprintf("test_newworktreeguardok_%d_", time.Now().UnixNano()),
		TmuxServerSocket: fixturetest.ColdRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() { _ = cleanup() }()

	require.NoError(t, inst.Start(true))

	assert.Equal(t, domain.Active, inst.Status)
	assert.True(t, inst.PMForTest().IsAlive())
	assert.NotEqual(t, repoDir, inst.Workspace().WorktreeDir, "a real, isolated worktree must have been created")
}

// TestInstance_Start_should_ProceedToSpawn_When_NoCollisionDetected is Epic
// 4.4's non-regression companion: no guard wired (the normal production
// wiring for a session with no colliding sibling) must not block a normal
// SessionTypeExistingWorktree session from reaching Active.
func TestInstance_Start_should_ProceedToSpawn_When_NoCollisionDetected(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	fixturetest.CheckTmuxAvailable(t)

	repoDir := t.TempDir()
	require.NoError(t, git.InitializeProjectDirectory(repoDir))

	title := fmt.Sprintf("guard-collision-ok-%d", time.Now().UnixNano())
	inst, cleanup, err := instance.NewInstanceWithCleanup(instance.InstanceOptions{
		Title:            title,
		Path:             repoDir,
		Program:          "sleep 300",
		SessionType:      domain.SessionTypeExistingWorktree,
		ExistingWorktree: repoDir,
		TmuxPrefix:       fmt.Sprintf("test_noguardcollision_%d_", time.Now().UnixNano()),
		TmuxServerSocket: fixturetest.ColdRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() { _ = cleanup() }()

	require.NoError(t, inst.Start(true))

	assert.Equal(t, domain.Active, inst.Status)
	assert.True(t, inst.PMForTest().IsAlive())
}
