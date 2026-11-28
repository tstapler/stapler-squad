package services

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/julesrpc"
)

type wiredJulesKeys struct{ julesrpc.JulesKeyManager }

func (wiredJulesKeys) APIKey(context.Context) (jules.JulesAPIKey, error) { return "k", nil }

type wiredJulesPoller struct{}

func (wiredJulesPoller) AuthReconnectRequired() bool { return true }

func newProviderWiringService(t *testing.T) *SessionService {
	t.Helper()
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	svc := NewSessionService(svctest.CreateTestStorage(t), bus)
	t.Cleanup(func() { svc.Shutdown() })
	return svc
}

func TestSessionService_should_GiveCapacityMonitorItsSessions_When_Constructed(t *testing.T) {
	svc := newProviderWiringService(t)

	poller, switcher := svc.capacityMonitor.SessionPortsForTest()

	assert.Same(t, svc, poller)
	assert.Same(t, svc, switcher)
}

func TestSessionService_should_ServeJulesConfigFromWiredDependencies_When_SettersRun(t *testing.T) {
	svc := newProviderWiringService(t)
	get := func() *sessionv1.JulesConfigProto {
		resp, err := svc.GetJulesConfig(t.Context(), connect.NewRequest(&sessionv1.GetJulesConfigRequest{}))
		require.NoError(t, err)
		return resp.Msg.Config
	}
	assert.False(t, get().HasApiKey, "constructed with no dependencies")

	svc.SetJulesConfigDependencies(wiredJulesKeys{}, nil, wiredJulesPoller{})
	usage := julesrpc.NewJulesUsageCounter()
	usage.IncSessionDispatched()
	svc.SetJulesUsageCounter(usage)

	got := get()
	assert.True(t, got.HasApiKey)
	assert.True(t, got.AuthReconnectRequired)
	assert.EqualValues(t, 1, got.Usage.SessionDispatched)
}
