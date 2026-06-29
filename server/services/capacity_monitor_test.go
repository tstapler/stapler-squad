package services

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

type mockInstancePoller struct {
	instances []*session.Instance
}

func (m *mockInstancePoller) GetInstances() []*session.Instance {
	return m.instances
}

type mockSessionSwitcher struct {
	mu     sync.Mutex
	called map[string]string // sessionID -> targetProgram
}

func (m *mockSessionSwitcher) UpdateSessionProgram(ctx context.Context, sessionID string, newProgram string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called[sessionID] = newProgram
	return nil
}

func (m *mockSessionSwitcher) GetTarget(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called[sessionID]
}

type mockLimitsClient struct {
	limits        ProviderLimits
	contextWindow int
	err           error
}

func (m *mockLimitsClient) Provider() string {
	return m.limits.Provider
}

func (m *mockLimitsClient) QueryLimits(ctx context.Context) (ProviderLimits, error) {
	return m.limits, m.err
}

func (m *mockLimitsClient) UpdateFromResponseHeaders(headers http.Header, current ProviderLimits) ProviderLimits {
	return m.limits
}

func (m *mockLimitsClient) ModelContextWindow(model string) int {
	return m.contextWindow
}

func TestCapacityMonitor_PollAndEvaluate(t *testing.T) {
	eventBus := events.NewEventBus(10)
	poller := &mockInstancePoller{
		instances: []*session.Instance{
			{
				Title:   "test-session",
				UUID:    "session-uuid-1",
				Program: "claude",
				Status:  session.Active,
			},
		},
	}
	// set mock conversation uuid
	poller.instances[0].SetClaudeConversationUUID("session-uuid-1")

	fakeStore := &fakeTokenStore{
		results: []*tokens.ParseResult{
			{
				SessionUUID: "session-uuid-1",
				TotalInput:  10000,
				TotalOutput: 2000,
				TurnTimeline: []tokens.TurnStats{
					{
						Input: 9000,
					},
				},
			},
		},
	}

	switcher := &mockSessionSwitcher{
		called: make(map[string]string),
	}

	cfg := config.CapacityConfig{
		TransitionMode:         config.TransitionModeManual,
		ContextWindowWarnPct:   0.80,
		ContextWindowAutoPct:   0.90,
		PollIntervalSeconds:    60,
		RateLimitWarnRemaining: 5,
		ProviderPriority: []config.ProviderPriority{
			{CLI: "claude", Model: "claude-3-5-sonnet"},
			{CLI: "agy", Model: "gemini-2.0-pro"},
		},
	}

	monitor := NewCapacityMonitor(cfg, eventBus, poller, fakeStore, switcher)

	anthropicClient := &mockLimitsClient{
		contextWindow: 10000, // 9000 used -> 90%
		limits: ProviderLimits{
			Provider:          "anthropic",
			Available:         true,
			RequestsRemaining: 100,
		},
	}
	monitor.RegisterClient("anthropic", anthropicClient)

	// Subscribe to event bus to assert warning notification is published
	ch, subID := eventBus.Subscribe(context.Background())
	defer eventBus.Unsubscribe(subID)

	// Trigger manual poll/evaluate
	monitor.poll(context.Background())

	// Assert limits are cached globally
	limitsMap := monitor.GetCurrentLimits()
	assert.True(t, limitsMap["anthropic"].Available)

	// Assert session limits are calculated
	sessLimits, found := monitor.GetSessionLimits("test-session")
	require.True(t, found)
	assert.Equal(t, 9000, sessLimits.ContextTokensUsed)
	assert.Equal(t, 10000, sessLimits.ContextTokensMax)
	assert.Equal(t, 10000, sessLimits.SessionInputTokens)
	assert.Equal(t, 2000, sessLimits.SessionOutputTokens)
	assert.Greater(t, sessLimits.EstimatedCostUSD, 0.0)

	// Check if notification event was published
	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Contains(t, ev.NotificationMessage, "Capacity Warning for test-session")
		assert.Equal(t, "capacity_alert", ev.NotificationMetadata["type"])
		assert.Equal(t, "agy", ev.NotificationMetadata["suggest_to"])
	case <-time.After(1 * time.Second):
		t.Fatal("expected notification event on eventBus, but none received")
	}

	// Switcher shouldn't be called because TransitionMode is Manual
	assert.Empty(t, switcher.GetTarget("test-session"))
}

func TestCapacityMonitor_AutoTransition(t *testing.T) {
	eventBus := events.NewEventBus(10)
	poller := &mockInstancePoller{
		instances: []*session.Instance{
			{
				Title:   "test-session-auto",
				UUID:    "session-uuid-auto",
				Program: "claude",
				Status:  session.Active,
			},
		},
	}
	poller.instances[0].SetClaudeConversationUUID("session-uuid-auto")

	fakeStore := &fakeTokenStore{
		results: []*tokens.ParseResult{
			{
				SessionUUID: "session-uuid-auto",
				TotalInput:  10000,
				TotalOutput: 2000,
				TurnTimeline: []tokens.TurnStats{
					{
						Input: 9500, // 95% used (> 90%)
					},
				},
			},
		},
	}

	switcher := &mockSessionSwitcher{
		called: make(map[string]string),
	}

	cfg := config.CapacityConfig{
		TransitionMode:         config.TransitionModeAuto,
		ContextWindowWarnPct:   0.80,
		ContextWindowAutoPct:   0.90,
		PollIntervalSeconds:    60,
		RateLimitWarnRemaining: 5,
		ProviderPriority: []config.ProviderPriority{
			{CLI: "claude", Model: "claude-3-5-sonnet"},
			{CLI: "agy", Model: "gemini-2.0-pro"},
		},
	}

	monitor := NewCapacityMonitor(cfg, eventBus, poller, fakeStore, switcher)

	anthropicClient := &mockLimitsClient{
		contextWindow: 10000,
		limits: ProviderLimits{
			Provider:          "anthropic",
			Available:         true,
			RequestsRemaining: 100,
		},
	}
	monitor.RegisterClient("anthropic", anthropicClient)

	monitor.poll(context.Background())

	// Wait for background auto-transition goroutine to complete
	require.Eventually(t, func() bool {
		return switcher.GetTarget("test-session-auto") == "agy"
	}, 2*time.Second, 10*time.Millisecond, "expected switcher to have target 'agy' for test-session-auto")
}

func TestCapacityMonitor_RateLimitWarning(t *testing.T) {
	eventBus := events.NewEventBus(10)
	poller := &mockInstancePoller{
		instances: []*session.Instance{
			{
				Title:   "test-session-rl",
				UUID:    "session-uuid-rl",
				Program: "claude",
				Status:  session.Active,
			},
		},
	}
	poller.instances[0].SetClaudeConversationUUID("session-uuid-rl")

	fakeStore := &fakeTokenStore{
		results: []*tokens.ParseResult{
			{
				SessionUUID: "session-uuid-rl",
			},
		},
	}

	switcher := &mockSessionSwitcher{
		called: make(map[string]string),
	}

	cfg := config.CapacityConfig{
		TransitionMode:         config.TransitionModeManual,
		ContextWindowWarnPct:   0.80,
		ContextWindowAutoPct:   0.90,
		PollIntervalSeconds:    60,
		RateLimitWarnRemaining: 5,
	}

	monitor := NewCapacityMonitor(cfg, eventBus, poller, fakeStore, switcher)

	anthropicClient := &mockLimitsClient{
		contextWindow: 100000,
		limits: ProviderLimits{
			Provider:          "anthropic",
			Available:         true,
			RequestsLimit:     100,
			RequestsRemaining: 3, // <= 5 (warning threshold)
		},
	}
	monitor.RegisterClient("anthropic", anthropicClient)

	ch, subID := eventBus.Subscribe(context.Background())
	defer eventBus.Unsubscribe(subID)

	monitor.poll(context.Background())

	// Check if notification event was published
	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Contains(t, ev.NotificationMessage, "rate_limit_exhausted")
	case <-time.After(1 * time.Second):
		t.Fatal("expected notification event for rate limit warning, but none received")
	}
}
