package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/config/clihelp"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/claudeconfig"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// These tests pin SessionService's wiring of the sessiondefaults, claudeconfig and
// workspacedb leaves.

func newConfigLeavesWiringService(t *testing.T) *SessionService {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(svctest.CreateTestStorage(t), bus)
	t.Cleanup(func() { svc.Shutdown() })
	return svc
}

// The nil check matters: three of these are empty structs whose methods work on a nil receiver.
func TestNewSessionService_should_ConstructTheConfigurationLeaves(t *testing.T) {
	svc := newConfigLeavesWiringService(t)

	assert.NotNil(t, svc.configSvc)
	assert.NotNil(t, svc.databaseSvc)
	assert.NotNil(t, svc.defaultsSvc)
	assert.NotNil(t, svc.launcherPresetsSvc)
}

func TestSetConfigService_should_ReplaceTheClaudeConfigService(t *testing.T) {
	svc := newConfigLeavesWiringService(t)
	replacement := claudeconfig.NewConfigService()
	svc.configSvc = nil

	svc.SetConfigService(replacement)

	assert.Same(t, replacement, svc.configSvc)
}

func TestSessionServiceDefaultsSetters_should_ReachTheDefaultsService(t *testing.T) {
	svc := newConfigLeavesWiringService(t)
	shared := &config.Config{}
	var mu sync.RWMutex
	updated := make(chan struct{}, 1)
	svc.SetSharedBacklogConfig(shared, &mu)
	svc.SetOnGlobalDefaultsUpdated(func() { updated <- struct{}{} })

	_, err := svc.UpdateGlobalDefaults(context.Background(), connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
		MaxConcurrentBacklogWorkItems: 7,
	}))
	require.NoError(t, err)

	select {
	case <-updated:
	case <-time.After(10 * time.Second):
		t.Fatal("the defaults-updated callback was not invoked")
	}
	mu.RLock()
	defer mu.RUnlock()
	assert.Equal(t, 7, shared.MaxConcurrentBacklogWorkItems)
}

func TestStartProgramProbeLoginPath_should_StartTheDefaultsServiceProbersDerivation(t *testing.T) {
	svc := newConfigLeavesWiringService(t)
	derived := make(chan struct{})
	var once sync.Once
	svc.defaultsSvc.SetProber(clihelp.NewProber(clihelp.WithShell(func() string {
		once.Do(func() { close(derived) })
		return "" // no shell: nothing is spawned
	})))

	svc.StartProgramProbeLoginPath()

	select {
	case <-derived:
	case <-time.After(10 * time.Second):
		t.Fatal("login-shell PATH derivation was not started")
	}
}
