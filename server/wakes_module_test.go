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

// With no workflow repo or scheduler constructed anywhere, a due trigger is still claimed.
func TestWakesModule_should_ClaimDueTrigger_When_NoWorkflowSchedulerExists(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	claimed := make(chan string, 1)
	mod := newWakesModule(storage, wakes.NoLeader{}, func(_ context.Context, row *session.WakeTriggerData) { claimed <- row.ID })

	due := time.Now().Add(-time.Minute)
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, "due")
	require.NoError(t, err)
	created, _, err := repo.CreateWakeTrigger(context.Background(), session.CreateWakeTriggerInput{
		OwnerSessionUUID: "owner", Spec: domain.AtSpec{At: due}, Action: action, NextFireAt: &due,
	})
	require.NoError(t, err)

	require.NoError(t, mod.timer.Start(context.Background()))
	t.Cleanup(func() { _ = mod.timer.Stop(context.Background()) })

	select {
	case id := <-claimed:
		assert.Equal(t, created.ID, id)
	case <-time.After(10 * time.Second):
		t.Fatal("due trigger was not claimed")
	}
}
