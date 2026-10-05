package services

import (
	"context"
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

type fakeTerminator struct {
	mu      sync.Mutex
	reasons []string
}

func (f *fakeTerminator) terminate(_ context.Context, _ *session.Instance, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reasons = append(f.reasons, reason)
	return nil
}

func (f *fakeTerminator) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.reasons...)
}

// cacheReadTimeline builds turns like the #882 incident: almost all cost is
// cache reads, with negligible fresh input/output.
func cacheReadTimeline(count int, cacheRead int64, tools []string, start time.Time, step time.Duration) []tokens.TurnStats {
	out := make([]tokens.TurnStats, count)
	for i := range out {
		out[i] = tokens.TurnStats{
			Timestamp: start.Add(time.Duration(i) * step),
			Model:     "claude-sonnet-4-5",
			Input:     1,
			Output:    10,
			CacheRead: cacheRead,
			ToolNames: tools,
		}
	}
	return out
}

func parseResultFrom(timeline []tokens.TurnStats) *tokens.ParseResult {
	r := &tokens.ParseResult{TurnTimeline: timeline, PrimaryModel: "claude-sonnet-4-5"}
	for _, turn := range timeline {
		r.TotalInput += turn.Input
		r.TotalOutput += turn.Output
		r.CacheRead += turn.CacheRead
		r.CacheCreation += turn.CacheCreation
	}
	return r
}

func newGuardrailMonitor(t *testing.T, role string, cfg config.CapacityConfig, parseRes *tokens.ParseResult) (*CapacityMonitor, *events.EventBus, *fakeCompactor, *fakeTerminator) {
	t.Helper()
	compactor := &fakeCompactor{}
	terminator := &fakeTerminator{}
	monitor, bus, _ := newIdleWaitTestMonitor(t, cfg.IdleWaitTurnCeiling, parseRes, compactor)
	cfg.TransitionMode = config.TransitionModeManual
	cfg.PollIntervalSeconds = 60
	monitor.config = cfg.CapacityConfigOrDefault()
	monitor.roleResolver = func(context.Context, string) string { return role }
	monitor.terminator = terminator.terminate
	return monitor, bus, compactor, terminator
}

func drainTypes(ch <-chan *events.Event) []string {
	var types []string
	for {
		select {
		case ev := <-ch:
			types = append(types, ev.NotificationMetadata["type"])
		case <-time.After(50 * time.Millisecond):
			return types
		}
	}
}

func TestCapacityMonitor_CacheReadCostTripsBudget(t *testing.T) {
	t.Parallel()
	// 100 turns x 5M cache-read tokens = 500M tokens ≈ $150 at sonnet's $0.30/Mtok.
	tl := cacheReadTimeline(100, 5_000_000, []string{"Edit"}, time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC), time.Minute)
	tests := []struct {
		name     string
		role     string
		cfg      config.CapacityConfig
		wantStop bool
	}{
		{"triage default budget stops", session.SessionRoleTriage, config.CapacityConfig{}, true},
		{"work role without budget is not stopped", session.SessionRoleWork, config.CapacityConfig{}, false},
		{"work role over explicit budget is warned, not stopped", session.SessionRoleWork, config.CapacityConfig{RoleCostBudgetUSD: map[string]float64{"work": 50}}, false},
		{"triage budget disabled by explicit 0", session.SessionRoleTriage, config.CapacityConfig{RoleCostBudgetUSD: map[string]float64{"triage": 0}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			monitor, _, _, terminator := newGuardrailMonitor(t, tt.role, tt.cfg, parseResultFrom(tl))
			monitor.poll(context.Background())
			monitor.poll(context.Background())

			limits, ok := monitor.GetSessionLimits("idle-wait-session")
			require.True(t, ok)
			assert.Greater(t, limits.EstimatedCostUSD, 100.0, "cache reads must be priced")
			if tt.wantStop {
				assert.Equal(t, []string{"cost_budget_exceeded"}, terminator.calls(), "one-shot stop")
			} else {
				assert.Empty(t, terminator.calls())
			}
		})
	}
}

func TestCapacityConfig_RoleDefaults(t *testing.T) {
	t.Parallel()
	cfg := config.CapacityConfig{CostBudgetUSD: 40}.CapacityConfigOrDefault()
	assert.Less(t, cfg.CostBudgetFor("triage"), cfg.CostBudgetFor("work"))
	assert.Equal(t, 40.0, cfg.CostBudgetFor("work"))
	assert.Less(t, cfg.IdleWaitCeilingFor("triage"), cfg.IdleWaitCeilingFor("work"))
	assert.Equal(t, 30, cfg.IdleWaitMaxMinutes)

	override := config.CapacityConfig{RoleCostBudgetUSD: map[string]float64{"triage": 3}}.CapacityConfigOrDefault()
	assert.Equal(t, 3.0, override.CostBudgetFor("triage"))
}

func TestCapacityMonitor_IdleWaitWallClockTrigger(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		turns       int
		step        time.Duration
		wantCompact int
	}{
		{"few turns spanning over 30 minutes compacts", 5, 10 * time.Minute, 1},
		{"few turns within 30 minutes does not", 5, 5 * time.Minute, 0},
		{"lone turn is never a loop", 2, 3 * time.Hour, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parseRes := parseResultFrom(cacheReadTimeline(tt.turns, 1000, nil, base, tt.step))
			monitor, _, compactor, _ := newGuardrailMonitor(t, session.SessionRoleWork, config.CapacityConfig{IdleWaitTurnCeiling: 15}, parseRes)
			monitor.poll(context.Background())
			assert.Equal(t, tt.wantCompact, compactor.callCount())
		})
	}
}

// Incident shape (#882): ListAgents / Monitor / text-only wake turns, seconds
// apart, never any real work.
func incidentWakeTurns(count int, start time.Time) []tokens.TurnStats {
	shapes := [][]string{{"ListAgents"}, {"Monitor"}, nil}
	out := cacheReadTimeline(count, 500_000, nil, start, 4*time.Second)
	for i := range out {
		out[i].ToolNames = shapes[i%len(shapes)]
	}
	return out
}

func TestCapacityMonitor_IncidentReplay_CompactThenTerminalOnce(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC)
	// Cost stays below the triage budget so this exercises idle-wait only.
	parseRes := parseResultFrom(incidentWakeTurns(10, base))
	monitor, bus, compactor, terminator := newGuardrailMonitor(t, session.SessionRoleTriage,
		config.CapacityConfig{RoleCostBudgetUSD: map[string]float64{"triage": 0}}, parseRes)
	ch, subID := bus.Subscribe(context.Background())
	defer bus.Unsubscribe(subID)

	// Ceiling for triage is 10: first poll compacts, no stop yet.
	monitor.poll(context.Background())
	assert.Equal(t, 1, compactor.callCount())
	assert.Empty(t, terminator.calls())

	// Loop continues through the compact.
	monitor.mu.Lock()
	tracker := monitor.idleWaitState["sess-uuid-1"]
	tracker.lastInterventionAt = base.Add(41 * time.Second)
	monitor.idleWaitState["sess-uuid-1"] = tracker
	monitor.mu.Unlock()
	parseRes.TurnTimeline = append(parseRes.TurnTimeline, incidentWakeTurns(10, base.Add(time.Minute))...)

	for i := 0; i < 3; i++ {
		monitor.poll(context.Background())
	}
	assert.Equal(t, 1, compactor.callCount(), "never compacts twice")
	assert.Equal(t, []string{"idle_wait_loop_escalation"}, terminator.calls(), "stopped exactly once")

	types := drainTypes(ch)
	count := 0
	for _, ty := range types {
		if ty == "idle_wait_loop_escalation" {
			count++
		}
	}
	assert.Equal(t, 1, count, "one escalation notification, got %v", types)
}

func TestCapacityMonitor_IdleWaitEscalation_NonBacklogSessionNotStopped(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC)
	parseRes := parseResultFrom(incidentWakeTurns(15, base))
	monitor, _, _, terminator := newGuardrailMonitor(t, "", config.CapacityConfig{IdleWaitTurnCeiling: 15}, parseRes)
	monitor.poll(context.Background())
	monitor.mu.Lock()
	monitor.idleWaitState["sess-uuid-1"] = idleWaitTracker{lastInterventionAt: base.Add(61 * time.Second)}
	monitor.mu.Unlock()
	parseRes.TurnTimeline = append(parseRes.TurnTimeline, incidentWakeTurns(15, base.Add(2*time.Minute))...)
	monitor.poll(context.Background())
	assert.Empty(t, terminator.calls())
}

func TestCapacityMonitor_SessionTokenCeilingNotifiesOnce(t *testing.T) {
	t.Parallel()
	// 3M cache-read tokens > ceiling; cost > $20 requires more.
	tl := cacheReadTimeline(100, 1_000_000, []string{"Edit"}, time.Date(2026, 9, 22, 17, 0, 0, 0, time.UTC), time.Minute)
	monitor, bus, _, _ := newGuardrailMonitor(t, session.SessionRoleWork, config.CapacityConfig{}, parseResultFrom(tl))
	ch, subID := bus.Subscribe(context.Background())
	defer bus.Unsubscribe(subID)

	for i := 0; i < 3; i++ {
		monitor.poll(context.Background())
	}
	var n int
	for _, ty := range drainTypes(ch) {
		if ty == "session_token_ceiling" {
			n++
		}
	}
	assert.Equal(t, 1, n)
}
