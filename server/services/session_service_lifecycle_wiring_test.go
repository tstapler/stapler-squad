package services

import (
	"context"
	"net/http"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/credkit"
	"github.com/tstapler/stapler-squad/server/services/julesrpc"
)

type probeLimitsClient struct{ queried chan struct{} }

func (probeLimitsClient) Provider() string { return "probe" }

func (c probeLimitsClient) QueryLimits(context.Context) (credkit.ProviderLimits, error) {
	select {
	case c.queried <- struct{}{}:
	default:
	}
	return credkit.ProviderLimits{}, nil
}

func (probeLimitsClient) UpdateFromResponseHeaders(_ http.Header, cur credkit.ProviderLimits) credkit.ProviderLimits {
	return cur
}

func (probeLimitsClient) ModelContextWindow(string) int { return 0 }

// Start polls every registered client at once, so the probe observes it without a ticker.
func TestSessionService_should_StartCapacityMonitor_When_LifecycleContextSet(t *testing.T) {
	svc := newProviderWiringService(t)
	probe := probeLimitsClient{queried: make(chan struct{}, 1)}
	svc.capacityMonitor.RegisterClient("probe", probe)

	svc.SetLifecycleContext(t.Context())

	select {
	case <-probe.queried:
	case <-time.After(10 * time.Second):
		t.Fatal("capacity monitor was never started")
	}
}

func TestSessionService_should_KeepJulesUsageCounter_When_ConfigDependenciesSetAfterIt(t *testing.T) {
	svc := newProviderWiringService(t)
	usage := julesrpc.NewJulesUsageCounter()
	usage.IncSessionDispatched()

	svc.SetJulesUsageCounter(usage)
	svc.SetJulesConfigDependencies(wiredJulesKeys{}, nil, nil)

	resp, err := svc.GetJulesConfig(t.Context(), connect.NewRequest(&sessionv1.GetJulesConfigRequest{}))
	require.NoError(t, err)
	assert.EqualValues(t, 1, resp.Msg.Config.Usage.SessionDispatched)
}
