package server

import (
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/julesrpc"
	"github.com/tstapler/stapler-squad/session"
)

type staleJulesUsage struct{}

func (staleJulesUsage) Snapshot() julesrpc.JulesUsageSnapshot {
	return julesrpc.JulesUsageSnapshot{SessionDispatched: 7}
}

// buildJules must install its own live counter, observed here as replacing a sentinel.
func TestBuildJules_should_InstallItsUsageCounter_When_JulesIsDisabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	envtest.NewIsolatedStateDir(t)
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	svc := services.NewSessionService(storage, bus)
	t.Cleanup(svc.Shutdown)
	svc.SetJulesUsageCounter(staleJulesUsage{})

	buildJules(&ServiceDeps{CoreDeps: &CoreDeps{Storage: storage, SessionService: svc}}, &config.Config{}, nil)

	// Drop the OS-keychain key source so GetJulesConfig never reads the real keychain.
	svc.SetJulesConfigDependencies(nil, nil, nil)
	resp, err := svc.GetJulesConfig(t.Context(), connect.NewRequest(&sessionv1.GetJulesConfigRequest{}))
	require.NoError(t, err)
	assert.Zero(t, resp.Msg.Config.Usage.SessionDispatched)
}
