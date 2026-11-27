package services

// Differential golden for SessionService.GetProviderLimits and SetTokenStoreReader,
// the hub's reads and write of CapacityMonitor, recorded from origin/main before
// the monitor moved.

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/capacity"
	"github.com/tstapler/stapler-squad/server/services/credkit"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
	"github.com/tstapler/stapler-squad/session/tokens"
)

var updateProviderLimitsGolden = flag.Bool("update-provider-limits-golden", false, "rewrite testdata/provider_limits.golden")

type providerLimitsStub struct{ name string }

func (c providerLimitsStub) Provider() string { return c.name }

func (c providerLimitsStub) QueryLimits(context.Context) (credkit.ProviderLimits, error) {
	return c.limits(), nil
}

func (c providerLimitsStub) UpdateFromResponseHeaders(http.Header, credkit.ProviderLimits) credkit.ProviderLimits {
	return c.limits()
}

func (c providerLimitsStub) ModelContextWindow(model string) int { return 1000 + len(model) }

func (c providerLimitsStub) limits() credkit.ProviderLimits {
	return credkit.ProviderLimits{
		Provider: c.name, Available: true, RequestsLimit: 10, RequestsRemaining: 9,
		TokensLimit: 500, TokensRemaining: 400, LastErrorCode: "e-" + c.name,
	}
}

type recordingTokenStore struct {
	tokens.TokenStoreReader
	asked []string
}

func (s *recordingTokenStore) GetByUUID(uuid string) *tokens.ParseResult {
	s.asked = append(s.asked, uuid)
	return &tokens.ParseResult{SessionUUID: uuid, TotalInput: 70, TotalOutput: 7}
}

func TestProviderLimitsGolden(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	programs := []string{"claude", "agy", "antigravity", "gemini", "opencode", "openai", "aider"}
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	for _, p := range programs {
		inst := &instance.Instance{Title: p, UUID: "uuid-" + p, Program: p, Status: domain.Active}
		inst.SetClaudeConversationUUID("conv-" + p)
		poller.AddInstance(inst)
	}
	newMonitor := func(clients ...string) *capacity.CapacityMonitor {
		m := capacity.NewCapacityMonitor(capacity.CapacityMonitorParams{
			Config: config.CapacityConfig{PollIntervalSeconds: 3600, ContextWindowWarnPct: 2}, EventBus: events.NewEventBus(8),
		})
		for _, name := range clients {
			m.RegisterClient(name, providerLimitsStub{name})
			m.UpdateFromResponseHeaders(name, http.Header{})
		}
		return m
	}
	call := func(svc *SessionService, id string) {
		resp, err := svc.GetProviderLimits(context.Background(), connect.NewRequest(&sessionv1.GetProviderLimitsRequest{SessionId: id}))
		if err != nil {
			fmt.Fprintf(&out, "%q err %v\n", id, err)
			return
		}
		b, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(resp.Msg)
		require.NoError(t, err)
		fmt.Fprintf(&out, "%q %s\n", id, strings.ReplaceAll(string(b), " ", ""))
	}
	for _, tc := range []struct {
		name    string
		monitor *capacity.CapacityMonitor
	}{
		{"no-monitor", nil},
		{"no-clients", newMonitor()},
		{"all-clients", newMonitor("anthropic", "google", "openai")},
		{"google-only", newMonitor("google")},
	} {
		svc := &SessionService{ReviewQueuePoller: poller, capacityMonitor: tc.monitor}
		fmt.Fprintf(&out, "=== %s\n", tc.name)
		call(svc, "")
		call(svc, "missing")
		for _, p := range programs {
			call(svc, p)
		}
	}

	// A nil monitor makes SetTokenStoreReader a no-op; a real one is read by the
	// next poll, whose session limits GetProviderLimits then prefers.
	(&SessionService{}).SetTokenStoreReader(&recordingTokenStore{})
	m := capacity.NewCapacityMonitor(capacity.CapacityMonitorParams{
		Config: config.CapacityConfig{PollIntervalSeconds: 3600, ContextWindowWarnPct: 2}, EventBus: events.NewEventBus(8),
		Poller: singleInstancePoller{poller.FindInstance("claude")},
	})
	m.RegisterClient("anthropic", providerLimitsStub{"anthropic"})
	svc := &SessionService{ReviewQueuePoller: poller, capacityMonitor: m}
	store := &recordingTokenStore{}
	svc.SetTokenStoreReader(store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.Start(ctx) // polls once, then returns on the cancelled context
	fmt.Fprintf(&out, "=== token-store asked=%v\n", store.asked)
	call(svc, "claude")

	const path = "testdata/provider_limits.golden"
	if *updateProviderLimitsGolden {
		require.NoError(t, os.WriteFile(path, []byte(out.String()), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), out.String())
}

type singleInstancePoller struct{ inst *instance.Instance }

func (p singleInstancePoller) GetInstances() []*instance.Instance {
	return []*instance.Instance{p.inst}
}
