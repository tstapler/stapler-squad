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
	"github.com/tstapler/stapler-squad/testutil/wait"
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
	t.Parallel()
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

	monitor := NewCapacityMonitor(CapacityMonitorParams{
		Config:          cfg,
		EventBus:        eventBus,
		Poller:          poller,
		TokenStore:      fakeStore,
		SessionSwitcher: switcher,
	})

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
	t.Parallel()
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

	monitor := NewCapacityMonitor(CapacityMonitorParams{
		Config:          cfg,
		EventBus:        eventBus,
		Poller:          poller,
		TokenStore:      fakeStore,
		SessionSwitcher: switcher,
	})

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
	wait.RequireEventually(t, func() bool {
		return switcher.GetTarget("test-session-auto") == "agy"
	}, 2*time.Second, 10*time.Millisecond, "expected switcher to have target 'agy' for test-session-auto")
}

func TestCapacityMonitor_RateLimitWarning(t *testing.T) {
	t.Parallel()
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

	monitor := NewCapacityMonitor(CapacityMonitorParams{
		Config:          cfg,
		EventBus:        eventBus,
		Poller:          poller,
		TokenStore:      fakeStore,
		SessionSwitcher: switcher,
	})

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

func TestIsIdleWaitTurn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		turn tokens.TurnStats
		want bool
	}{
		{"no tool calls counts as idle", tokens.TurnStats{ToolNames: nil}, true},
		{"only Monitor counts as idle", tokens.TurnStats{ToolNames: []string{"Monitor"}}, true},
		{"only ListAgents counts as idle", tokens.TurnStats{ToolNames: []string{"ListAgents"}}, true},
		{"Monitor plus ListAgents counts as idle", tokens.TurnStats{ToolNames: []string{"Monitor", "ListAgents"}}, true},
		{"Read is real work", tokens.TurnStats{ToolNames: []string{"Read"}}, false},
		{"Bash is real work", tokens.TurnStats{ToolNames: []string{"Bash"}}, false},
		{"Monitor plus Read is real work", tokens.TurnStats{ToolNames: []string{"Monitor", "Read"}}, false},
		{"unrecognized tool is real work, not silently idle", tokens.TurnStats{ToolNames: []string{"SomeNewTool"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isIdleWaitTurn(tt.turn))
		})
	}
}

func TestConsecutiveIdleWaitTurns(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC)
	turn := func(offsetSec int, tools ...string) tokens.TurnStats {
		return tokens.TurnStats{Timestamp: base.Add(time.Duration(offsetSec) * time.Second), ToolNames: tools}
	}

	t.Run("counts trailing idle turns, stops at real work", func(t *testing.T) {
		t.Parallel()
		timeline := []tokens.TurnStats{
			turn(0, "Read"),
			turn(1),
			turn(2, "Monitor"),
			turn(3),
		}
		assert.Equal(t, 3, consecutiveIdleWaitTurns(timeline, time.Time{}))
	})

	t.Run("all-idle timeline counts everything", func(t *testing.T) {
		t.Parallel()
		timeline := []tokens.TurnStats{turn(0), turn(1, "ListAgents"), turn(2)}
		assert.Equal(t, 3, consecutiveIdleWaitTurns(timeline, time.Time{}))
	})

	t.Run("no trailing idle turns counts zero", func(t *testing.T) {
		t.Parallel()
		timeline := []tokens.TurnStats{turn(0), turn(1, "Write")}
		assert.Equal(t, 0, consecutiveIdleWaitTurns(timeline, time.Time{}))
	})

	t.Run("since cutoff excludes turns at or before it", func(t *testing.T) {
		t.Parallel()
		timeline := []tokens.TurnStats{
			turn(0), // idle, but at-or-before cutoff -> excluded
			turn(10),
			turn(20),
		}
		since := base.Add(5 * time.Second)
		assert.Equal(t, 2, consecutiveIdleWaitTurns(timeline, since))
	})

	t.Run("since cutoff prevents merging a pre-compact run with a post-compact one", func(t *testing.T) {
		t.Parallel()
		// Idle turns both before and after "since" with no real-work turn in
		// between: without the cutoff these would merge into one run.
		timeline := []tokens.TurnStats{turn(0), turn(1), turn(10), turn(11)}
		since := base.Add(5 * time.Second)
		assert.Equal(t, 2, consecutiveIdleWaitTurns(timeline, since))
	})
}

// fakeCompactor records every SessionCompactor invocation for assertions,
// standing in for session.SubmitContentWithEnter (which needs a real
// tmux-backed *session.Instance) so idle-wait-loop tests can run against the
// same struct-literal *session.Instance the other CapacityMonitor tests use.
type fakeCompactor struct {
	mu    sync.Mutex
	calls []string // session titles compacted, in call order
	err   error
}

func (f *fakeCompactor) compact(_ context.Context, inst *session.Instance, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, inst.Snapshot().Title)
	return f.err
}

func (f *fakeCompactor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func idleTimeline(count int, startAt time.Time) []tokens.TurnStats {
	timeline := make([]tokens.TurnStats, count)
	for i := range timeline {
		timeline[i] = tokens.TurnStats{Timestamp: startAt.Add(time.Duration(i) * time.Second)}
	}
	return timeline
}

func newIdleWaitTestMonitor(t *testing.T, ceiling int, parseRes *tokens.ParseResult, compactor *fakeCompactor) (*CapacityMonitor, *events.EventBus, *mockInstancePoller) {
	t.Helper()
	eventBus := events.NewEventBus(10)
	poller := &mockInstancePoller{
		instances: []*session.Instance{
			{Title: "idle-wait-session", UUID: "sess-uuid-1", Program: "claude", Status: session.Active},
		},
	}
	poller.instances[0].SetClaudeConversationUUID("conv-uuid-1")
	parseRes.SessionUUID = "conv-uuid-1"

	monitor := NewCapacityMonitor(CapacityMonitorParams{
		Config: config.CapacityConfig{
			TransitionMode:      config.TransitionModeManual,
			PollIntervalSeconds: 60,
			IdleWaitTurnCeiling: ceiling,
		},
		EventBus:   eventBus,
		Poller:     poller,
		TokenStore: &fakeTokenStore{results: []*tokens.ParseResult{parseRes}},
		Compactor:  compactor.compact,
	})
	monitor.RegisterClient("anthropic", &mockLimitsClient{
		contextWindow: 1_000_000,
		limits:        ProviderLimits{Provider: "anthropic", Available: true, RequestsRemaining: 100},
	})
	return monitor, eventBus, poller
}

func TestCapacityMonitor_IdleWaitLoop_FirstCrossingCompacts(t *testing.T) {
	t.Parallel()
	compactor := &fakeCompactor{}
	base := time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC)
	parseRes := &tokens.ParseResult{TurnTimeline: idleTimeline(15, base)}

	monitor, eventBus, _ := newIdleWaitTestMonitor(t, 15, parseRes, compactor)
	ch, subID := eventBus.Subscribe(context.Background())
	defer eventBus.Unsubscribe(subID)

	monitor.poll(context.Background())

	require.Equal(t, 1, compactor.callCount(), "expected /compact to be sent once on first ceiling crossing")

	select {
	case ev := <-ch:
		assert.Equal(t, "idle_wait_loop_compact", ev.NotificationMetadata["type"])
	case <-time.After(1 * time.Second):
		t.Fatal("expected idle_wait_loop_compact notification, but none received")
	}
}

func TestCapacityMonitor_IdleWaitLoop_BelowCeilingDoesNotCompact(t *testing.T) {
	t.Parallel()
	compactor := &fakeCompactor{}
	base := time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC)
	parseRes := &tokens.ParseResult{TurnTimeline: idleTimeline(14, base)} // one short of the ceiling

	monitor, _, _ := newIdleWaitTestMonitor(t, 15, parseRes, compactor)
	monitor.poll(context.Background())

	assert.Equal(t, 0, compactor.callCount())
}

func TestCapacityMonitor_IdleWaitLoop_RecurrenceAfterCompactEscalates(t *testing.T) {
	t.Parallel()
	compactor := &fakeCompactor{}
	base := time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC)
	parseRes := &tokens.ParseResult{TurnTimeline: idleTimeline(15, base)}

	monitor, eventBus, _ := newIdleWaitTestMonitor(t, 15, parseRes, compactor)

	// First poll: crosses the ceiling, compacts.
	monitor.poll(context.Background())
	require.Equal(t, 1, compactor.callCount())

	// Simulate the intervention timestamp landing between the pre- and
	// post-compact turns, then 15 more idle turns after it — the loop
	// continuing right through the compact instead of breaking.
	monitor.mu.Lock()
	tracker := monitor.idleWaitState["sess-uuid-1"]
	tracker.lastInterventionAt = base.Add(15*time.Second + 500*time.Millisecond)
	monitor.idleWaitState["sess-uuid-1"] = tracker
	monitor.mu.Unlock()

	postCompact := idleTimeline(15, base.Add(16*time.Second))
	parseRes.TurnTimeline = append(parseRes.TurnTimeline, postCompact...)

	ch, subID := eventBus.Subscribe(context.Background())
	defer eventBus.Unsubscribe(subID)

	monitor.poll(context.Background())

	// Still only the one /compact — recurrence escalates, it doesn't compact again.
	assert.Equal(t, 1, compactor.callCount())

	select {
	case ev := <-ch:
		assert.Equal(t, "idle_wait_loop_escalation", ev.NotificationMetadata["type"])
	case <-time.After(1 * time.Second):
		t.Fatal("expected idle_wait_loop_escalation notification, but none received")
	}

	// The loop persists: further polls must not re-notify or re-compact.
	monitor.poll(context.Background())
	assert.Equal(t, 1, compactor.callCount())
	select {
	case ev := <-ch:
		t.Fatalf("unexpected repeat notification: %v", ev.NotificationMetadata["type"])
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCapacityMonitor_IdleWaitLoop_RealWorkAfterCompactClearsTracker(t *testing.T) {
	t.Parallel()
	compactor := &fakeCompactor{}
	base := time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC)
	parseRes := &tokens.ParseResult{TurnTimeline: idleTimeline(15, base)}

	monitor, _, _ := newIdleWaitTestMonitor(t, 15, parseRes, compactor)
	monitor.poll(context.Background())
	require.Equal(t, 1, compactor.callCount())

	monitor.mu.Lock()
	tracker := monitor.idleWaitState["sess-uuid-1"]
	tracker.lastInterventionAt = base.Add(15*time.Second + 500*time.Millisecond)
	monitor.idleWaitState["sess-uuid-1"] = tracker
	monitor.mu.Unlock()

	// A real (non-idle) turn after the compact: the loop broke on its own.
	parseRes.TurnTimeline = append(parseRes.TurnTimeline, tokens.TurnStats{
		Timestamp: base.Add(16 * time.Second),
		ToolNames: []string{"Edit"},
	})

	monitor.poll(context.Background())

	monitor.mu.RLock()
	_, stillTracked := monitor.idleWaitState["sess-uuid-1"]
	monitor.mu.RUnlock()
	assert.False(t, stillTracked, "tracker should clear once real work happens after the compact")
	assert.Equal(t, 1, compactor.callCount(), "should not have compacted again")
}
