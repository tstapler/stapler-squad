package server

import (
	"context"
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
