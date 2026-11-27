package services

// Differential golden for CapacityMonitor (limits, events, switch, compact and
// terminate calls), recorded from origin/main before the move.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/credkit"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tokens"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

var updateCapacityGolden = flag.Bool("update-capacity-golden", false, "rewrite testdata/capacity.golden")

type goldenRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *goldenRecorder) add(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

func (r *goldenRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.lines)
}

func (r *goldenRecorder) switched() bool {
	return slices.ContainsFunc(r.snapshot(), func(s string) bool { return strings.HasPrefix(s, "switch ") })
}

func (r *goldenRecorder) UpdateSessionProgram(_ context.Context, id, program string) error {
	r.add("switch session=%q program=%q", id, program)
	return nil
}

func goldenLimits(l credkit.ProviderLimits) string {
	l.FetchedAt = time.Time{}
	return fmt.Sprintf("%+v", l)
}

func goldenDrain(out *strings.Builder, ch <-chan *events.Event) {
	for {
		select {
		case ev := <-ch:
			keys := make([]string, 0, len(ev.NotificationMetadata))
			for k, v := range ev.NotificationMetadata {
				keys = append(keys, k+"="+v)
			}
			slices.Sort(keys)
			id := ev.NotificationID
			if i := strings.LastIndex(id, "-"); i >= 0 {
				id = id[:i] + "-<ts>"
			}
			fmt.Fprintf(out, "event %v session=%q ctx=%q id=%q type=%d prio=%d title=%q msg=%q meta=%v\n",
				ev.Type, ev.SessionID, ev.Context, id, ev.NotificationType, ev.NotificationPriority,
				ev.NotificationTitle, ev.NotificationMessage, keys)
		default:
			return
		}
	}
}

func TestCapacityMonitorGolden(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	programs := []string{"claude", "agy", "antigravity", "Gemini", "opencode", "openai-codex", "aider"}
	type scenario struct {
		name    string
		cfg     config.CapacityConfig
		role    string
		pollErr error
		turns   int
		failEnd bool
		auto    bool
	}
	base := config.CapacityConfig{TransitionMode: config.TransitionModeManual, PollIntervalSeconds: 60, ContextWindowWarnPct: 2}
	with := func(f func(*config.CapacityConfig)) config.CapacityConfig {
		c := base
		f(&c)
		return c
	}
	scenarios := []scenario{
		{name: "quiet", cfg: base},
		{name: "poll-error", cfg: base, pollErr: errors.New("boom")},
		{name: "context", cfg: with(func(c *config.CapacityConfig) { c.ContextWindowWarnPct = 0.5 })},
		{name: "context-auto", auto: true, cfg: with(func(c *config.CapacityConfig) {
			c.ContextWindowWarnPct = 0.5
			c.TransitionMode = config.TransitionModeAuto
			c.ProviderPriority = []config.ProviderPriority{{CLI: "claude", Model: "m1"}, {CLI: "aider", Model: "m2"}}
		})},
		{name: "rate", cfg: with(func(c *config.CapacityConfig) { c.RateLimitWarnRemaining = 95 })},
		{name: "idle", cfg: with(func(c *config.CapacityConfig) { c.IdleWaitTurnCeiling = 3 }), role: "work", turns: 5},
		{name: "terminate-fails", cfg: with(func(c *config.CapacityConfig) { c.CostBudgetUSD = 0.000001 }), role: instance.SessionRoleTriage, failEnd: true},
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, sc := range scenarios {
		for _, program := range programs {
			bus := events.NewEventBus(64)
			ctx, cancel := context.WithCancel(context.Background())
			ch, _ := bus.Subscribe(ctx)
			rec := &goldenRecorder{}
			inst := &instance.Instance{Title: "golden", UUID: "uuid-golden", Program: program, Status: domain.Active}
			inst.SetClaudeConversationUUID("conv-golden")
			turns := []tokens.TurnStats{{Input: 150_000, Timestamp: start}}
			for i := 0; i < sc.turns; i++ {
				turns = append(turns, tokens.TurnStats{Input: 150_000, Timestamp: start.Add(time.Duration(i+1) * time.Second), ToolNames: []string{"Monitor"}})
			}
			m := NewCapacityMonitor(CapacityMonitorParams{
				Config:          sc.cfg,
				EventBus:        bus,
				Poller:          &svctest.MockInstancePoller{Instances: []*instance.Instance{nil, inst}},
				TokenStore:      &svctest.FakeTokenStore{Results: []*tokens.ParseResult{{SessionUUID: "conv-golden", TotalInput: 200_000, TotalOutput: 40_000, TurnTimeline: turns}}},
				SessionSwitcher: rec,
				Compactor: func(_ context.Context, i *instance.Instance, content string) error {
					rec.add("compact session=%q content=%q", i.Snapshot().Title, content)
					return nil
				},
				RoleResolver: func(_ context.Context, uuid string) string {
					rec.add("role uuid=%q", uuid)
					return sc.role
				},
				Terminator: func(_ context.Context, i *instance.Instance, reason string) error {
					rec.add("terminate session=%q reason=%q", i.Snapshot().Title, reason)
					if sc.failEnd {
						return errors.New("still running")
					}
					return nil
				},
			})
			m.now = func() time.Time { return turns[len(turns)-1].Timestamp.Add(time.Minute) }
			for _, name := range []string{"anthropic", "google", "openai"} {
				m.RegisterClient(name, &mockLimitsClient{contextWindow: 200_000, err: sc.pollErr, limits: credkit.ProviderLimits{
					Provider: name, Available: true, RequestsLimit: 100, RequestsRemaining: 90,
				}})
			}
			fmt.Fprintf(&out, "=== %s %s\n", sc.name, program)
			for pass := 0; pass < 2; pass++ {
				m.poll(ctx)
				// Only the anthropic provider has token-store usage, so only it trips the
				// context threshold; the switch runs on its own goroutine.
				if sc.auto && pass == 0 && (program == "claude" || program == "aider") {
					wait.RequireEventually(t, rec.switched, 5*time.Second, time.Millisecond)
				}
				goldenDrain(&out, ch)
			}
			for _, l := range rec.snapshot() {
				out.WriteString(l + "\n")
			}
			current := m.GetCurrentLimits()
			for _, name := range []string{"anthropic", "google", "openai"} {
				l, ok := current[name]
				fmt.Fprintf(&out, "current %s %t %s\n", name, ok, goldenLimits(l))
			}
			l, ok := m.GetSessionLimits("golden")
			fmt.Fprintf(&out, "session %t %s\n", ok, goldenLimits(l))
			m.UpdateFromResponseHeaders("anthropic", http.Header{})
			m.UpdateFromResponseHeaders("unknown", http.Header{})
			fmt.Fprintf(&out, "after-headers %s\n", goldenLimits(m.GetCurrentLimits()["anthropic"]))
			cancel()
		}
	}
	const path = "testdata/capacity.golden"
	if *updateCapacityGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o750))
		require.NoError(t, os.WriteFile(path, []byte(out.String()), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), out.String())
}
