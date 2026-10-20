package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestWakeDispatcherEnabled_should_FollowFlagMatrix_When_GlobalAndScopesPermuted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		global map[string]bool
		scopes map[string]map[string]bool
		want   bool
	}{
		{"nothing set", nil, nil, false},
		{"global on", map[string]bool{wakeTriggersFlag: true}, nil, true},
		{"global off", map[string]bool{wakeTriggersFlag: false}, nil, false},
		{"scoped true under global off", map[string]bool{wakeTriggersFlag: false},
			map[string]map[string]bool{wakeTriggersFlag: {"trigger:at": true}}, true},
		{"scoped false only", nil, map[string]map[string]bool{wakeTriggersFlag: {"trigger:at": false}}, false},
		{"unrelated scope true", nil, map[string]map[string]bool{wakeTriggersFlag: {"tool:request_help": true}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{FeatureFlags: tt.global, FeatureFlagScopes: tt.scopes}
			assert.Equal(t, tt.want, wakeDispatcherEnabled(cfg))
		})
	}
}

type wakeTestSteerer struct{ wrote chan string }

func (wakeTestSteerer) IsReadyForSteer(string) bool { return true }
func (s wakeTestSteerer) WriteWake(_ context.Context, _ string, text string) (wakes.WriteOutcome, error) {
	s.wrote <- text
	return wakes.WriteDelivered, nil
}

type wakeTestOwners struct{}

func (wakeTestOwners) Owner(context.Context, string) (wakes.OwnerFacts, error) {
	return wakes.OwnerFacts{Exists: true, Live: true}, nil
}

type wakeTestAdmission struct{}

func (wakeTestAdmission) Admit(context.Context) (bool, error) { return false, nil }

// With no workflow repo or scheduler constructed anywhere, a due trigger is
// still claimed and delivered through the dispatcher.
func TestWakeDispatcher_should_DeliverDueTrigger_When_NoWorkflowSchedulerExists(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	wrote := make(chan string, 1)
	d := newWakeDispatcher(wakeWiring{
		Store: storage, Steerer: wakeTestSteerer{wrote: wrote}, Owners: wakeTestOwners{},
		Admission: wakeTestAdmission{}, Leader: wakes.NoLeader{},
	}, storage.SetWakeChangeHook)

	due := time.Now().Add(-time.Minute)
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, "due")
	require.NoError(t, err)
	created, _, err := repo.CreateWakeTrigger(context.Background(), session.CreateWakeTriggerInput{
		OwnerSessionUUID: "owner", Spec: domain.AtSpec{At: due}, Action: action, NextFireAt: &due,
	})
	require.NoError(t, err)

	require.NoError(t, d.Start(context.Background()))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	select {
	case text := <-wrote:
		assert.Contains(t, text, created.ID)
		assert.Contains(t, text, "<wake")
	case <-time.After(10 * time.Second):
		t.Fatal("due trigger was not delivered")
	}
}

func TestWakeLimitSettings_should_MapEveryConfigField(t *testing.T) {
	t.Parallel()
	got := wakeLimitSettings(config.WakeLimitsConfig{PendingPerOwner: 7, ReviveLockSeconds: 9})
	assert.Equal(t, 7, got.PendingPerOwner)
	assert.Equal(t, 9, got.ReviveLockSeconds)
}

type lifecycleProbe struct {
	on        atomic.Bool
	starts    atomic.Int32
	stops     atomic.Int32
	reconcile atomic.Int32
	startErr  error
}

func (p *lifecycleProbe) lifecycle() *wakeLifecycle {
	return &wakeLifecycle{
		enabled: p.on.Load,
		start: func(context.Context) (func(), func(context.Context), error) {
			if p.startErr != nil {
				return nil, nil, p.startErr
			}
			p.starts.Add(1)
			return func() { p.stops.Add(1) }, func(context.Context) { p.reconcile.Add(1) }, nil
		},
	}
}

func TestWakeLifecycle_should_StartAndStopDispatcher_When_FlagFlipsAtRuntime(t *testing.T) {
	t.Parallel()
	p := &lifecycleProbe{}
	lc := p.lifecycle()
	ctx := context.Background()

	lc.sync(ctx)
	assert.EqualValues(t, 0, p.starts.Load(), "flag off at boot: nothing runs")

	p.on.Store(true)
	lc.sync(ctx)
	lc.sync(ctx)
	assert.EqualValues(t, 1, p.starts.Load(), "a flip on starts it once")

	p.on.Store(false)
	lc.sync(ctx)
	lc.sync(ctx)
	assert.EqualValues(t, 1, p.stops.Load(), "a flip off stops it once")

	p.on.Store(true)
	lc.sync(ctx)
	assert.EqualValues(t, 2, p.starts.Load(), "and it can start again")
}

func TestWakeLifecycle_should_ReconcileOnlyWhileRunning_And_PickUpFlagOnTick(t *testing.T) {
	t.Parallel()
	p := &lifecycleProbe{}
	lc := p.lifecycle()
	ctx := context.Background()

	lc.Reconcile(ctx)
	assert.EqualValues(t, 0, p.reconcile.Load())

	p.on.Store(true)
	lc.Reconcile(ctx)
	assert.EqualValues(t, 1, p.starts.Load(), "the periodic tick applies a flip the listener missed")
	assert.EqualValues(t, 1, p.reconcile.Load())

	lc.shutdown()
	assert.EqualValues(t, 1, p.stops.Load())
	lc.shutdown()
	assert.EqualValues(t, 1, p.stops.Load(), "shutdown is idempotent")
}

func TestWakeLifecycle_should_RetryStart_When_NotLeaderThenBecomesLeader(t *testing.T) {
	t.Parallel()
	p := &lifecycleProbe{startErr: wakes.ErrNotLeader}
	p.on.Store(true)
	lc := p.lifecycle()
	ctx := context.Background()

	lc.sync(ctx)
	assert.EqualValues(t, 0, p.starts.Load())

	p.startErr = nil
	lc.sync(ctx)
	assert.EqualValues(t, 1, p.starts.Load())
}
