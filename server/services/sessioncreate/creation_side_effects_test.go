package sessioncreate

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/driver"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/summaries"
)

func TestCleanupPartialCreation_ShouldKillTheLeftoverTmuxSession_When_NeverStarted(t *testing.T) {
	t.Parallel()
	inst, closed := newFixture(t).leftoverInstance(t, "cleanup-kill")

	require.NoError(t, CleanupPartialCreation(inst, false))

	assert.Equal(t, int32(1), closed.Load())
}

func TestCancelSessionCreation_ShouldCleanUpThePartialCreation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	inst, closed := f.leftoverInstance(t, "cancel-cleanup")
	f.deps.FindLive = func(string) *instance.Instance { return inst }

	_, err := f.handler().CancelSessionCreation(context.Background(),
		connect.NewRequest(&sessionv1.CancelSessionCreationRequest{Id: inst.Title}))

	require.NoError(t, err)
	assert.Equal(t, int32(1), closed.Load())
}

func TestRetrySessionCreation_ShouldCancelTheOldAttemptAndCleanUp_BeforeDispatching(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	inst, closed := f.leftoverInstance(t, "retry-cleanup")
	instance.SetStatusForTest(t, inst, domain.Failed, nil)
	var cancelled, dispatched bool
	inst.SetCreationCancelFunc(func() { cancelled = true })
	f.deps.FindLive = func(string) *instance.Instance { return inst }
	f.deps.Track = func(func()) {
		dispatched = true
		assert.True(t, cancelled, "the old attempt is cancelled before the new one starts")
		assert.Equal(t, int32(1), closed.Load(), "cleanup runs before the new attempt starts")
	}

	_, err := f.handler().RetrySessionCreation(context.Background(),
		connect.NewRequest(&sessionv1.RetrySessionCreationRequest{Id: inst.Title}))

	require.NoError(t, err)
	assert.True(t, dispatched)
	assert.True(t, cancelled)
	assert.Equal(t, int32(1), closed.Load())
}

// failingStore refuses to save, after noting whether the session was running.
type failingStore struct {
	session.InstanceStore
	added   *instance.Instance
	running bool
}

func (s *failingStore) AddInstance(inst *instance.Instance) error {
	s.added, s.running = inst, inst.TmuxSessionExists()
	return errors.New("save refused")
}

func TestSpawners_ShouldDestroyTheStartedSession_When_SavingFails(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repo)
	for name, spawn := range map[string]func(*Handler, domain.SessionSpawnOptions) error{
		"worktree": func(h *Handler, o domain.SessionSpawnOptions) error {
			_, err := h.CreateWorktreeSession(context.Background(), repo, repo, o)
			return err
		},
		"directory": func(h *Handler, o domain.SessionSpawnOptions) error {
			_, err := h.CreateDirectorySession(context.Background(), t.TempDir(), o)
			return err
		},
	} {
		f := newFixture(t)
		store := &failingStore{InstanceStore: f.storage}
		f.deps.Store = store

		err := spawn(f.handler(), domain.SessionSpawnOptions{Title: "save-fails-" + name, ProgramOverride: f.agent})

		require.ErrorContains(t, err, "save refused", name)
		require.NotNil(t, store.added, name)
		t.Cleanup(func() { _ = store.added.Destroy() })
		require.True(t, store.running, "%s: the session must have been started for this test to mean anything", name)
		assert.False(t, store.added.TmuxSessionExists(), "%s: the tmux session must not outlive the failed save", name)
	}
}

func TestCreateDirectorySession_ShouldStartTheSessionDriver(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	inst, err := f.handler().CreateDirectorySession(context.Background(), t.TempDir(),
		domain.SessionSpawnOptions{Title: "spawn-driver", ProgramOverride: f.agent})

	require.NoError(t, err)
	t.Cleanup(func() { _ = inst.Destroy() })
	assert.True(t, driver.SessionDriverRunningForTest(inst))
}

// runPipeline runs the whole resolution pipeline of a new session to its end.
func runPipeline(t *testing.T, f *fixture, title string) *instance.Instance {
	t.Helper()
	inst := f.storedInstance(t, title)
	t.Cleanup(func() { _ = inst.Destroy() })
	f.handler().RunResolutionPipeline(context.Background(), PipelineParams{
		Instance: inst, Epoch: inst.CreationEpoch(), Title: inst.Title, RootDir: inst.GetEffectiveRootDir(),
	})
	return inst
}

func TestResolutionPipeline_ShouldNotStartTheSession_When_ItsContextHasEnded(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.deps.ResolutionTimeout = func() time.Duration { return time.Nanosecond }
	wired := false
	f.deps.WireCallbacks = func(*instance.Instance) { wired = true }

	inst := runPipeline(t, f, "pipeline-expired")

	assert.False(t, wired, "nothing past the ctx.Err guard may run")
	assert.False(t, inst.Started())
	status, reason := inst.StatusAndFailureReason()
	assert.Equal(t, domain.Failed, status)
	assert.Equal(t, "StartupError", reason)
}

func TestResolutionPipeline_ShouldStartTheSessionDriver(t *testing.T) {
	t.Parallel()
	inst := runPipeline(t, newFixture(t), "pipeline-driver")

	require.True(t, inst.Started())
	assert.True(t, driver.SessionDriverRunningForTest(inst))
}

func TestResolutionPipeline_ShouldWireTheBacklogAndSummaryListeners_When_Installed(t *testing.T) {
	t.Parallel()
	listeners := func(install func(*fixture)) int {
		f := newFixture(t)
		install(f)
		return runPipeline(t, f, "pipeline-listeners").LifecycleListenerCount()
	}
	none := listeners(func(*fixture) {})

	withBacklog := listeners(func(f *fixture) {
		l := backlog.NewBacklogLifecycleListener(f.storage)
		f.deps.BacklogListener = func() *backlog.BacklogLifecycleListener { return l }
	})
	withSummary := listeners(func(f *fixture) {
		g := summaries.NewSessionSummaryGenerator(f.storage.GetEntClient(), nil, nil, nil, nil)
		f.deps.SummaryGenerator = func() *summaries.SessionSummaryGenerator { return g }
	})

	assert.Equal(t, none+1, withBacklog, "backlog listener")
	assert.Equal(t, none+1, withSummary, "summary listener")
}
