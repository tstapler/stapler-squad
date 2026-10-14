package tokens

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRollingSpendRate(t *testing.T) {
	t.Parallel()
	pt := DefaultPricingTable()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	// Sonnet 4.5 cache reads priced via the real table; derive the per-turn cost rather than hardcoding a price.
	turn := func(ago time.Duration, model string) TurnStats {
		return TurnStats{Timestamp: now.Add(-ago), Model: model, Input: 1_000_000}
	}
	unit, priced := pt.EstimateTurnCost(turn(0, "claude-sonnet-4-5"))
	assert.True(t, priced)
	assert.Greater(t, unit, 0.0)

	tests := []struct {
		name   string
		r      *ParseResult
		pt     *PricingTable
		window time.Duration
		want   float64
		wantOK bool
	}{
		{"nil result", nil, pt, time.Hour, 0, false},
		{"nil pricing", &ParseResult{}, nil, time.Hour, 0, false},
		{"zero window", &ParseResult{TurnTimeline: []TurnStats{turn(time.Minute, "claude-sonnet-4-5")}}, pt, 0, 0, false},
		{"no turns", &ParseResult{}, pt, time.Hour, 0, false},
		{"only unpriced model", &ParseResult{TurnTimeline: []TurnStats{turn(time.Minute, "mystery-model")}}, pt, time.Hour, 0, false},
		{"all turns outside window", &ParseResult{TurnTimeline: []TurnStats{turn(2*time.Hour, "claude-sonnet-4-5")}}, pt, time.Hour, 0, false},
		{"turn exactly at window start is excluded", &ParseResult{TurnTimeline: []TurnStats{turn(time.Hour, "claude-sonnet-4-5")}}, pt, time.Hour, 0, false},
		{"future turn ignored", &ParseResult{TurnTimeline: []TurnStats{turn(-time.Minute, "claude-sonnet-4-5")}}, pt, time.Hour, 0, false},
		{"zero timestamp ignored", &ParseResult{TurnTimeline: []TurnStats{{Model: "claude-sonnet-4-5", Input: 1_000_000}}}, pt, time.Hour, 0, false},
		{
			"two in-window turns, one stale, one unpriced",
			&ParseResult{TurnTimeline: []TurnStats{
				turn(3*time.Hour, "claude-sonnet-4-5"),
				turn(30*time.Minute, "claude-sonnet-4-5"),
				turn(10*time.Minute, "claude-sonnet-4-5"),
				turn(5*time.Minute, "mystery-model"),
			}},
			pt, time.Hour, 2 * unit, true,
		},
		{
			"half-hour window doubles the hourly scale",
			&ParseResult{TurnTimeline: []TurnStats{turn(10*time.Minute, "claude-sonnet-4-5")}},
			pt, 30 * time.Minute, 2 * unit, true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := RollingSpendRate(tc.r, tc.pt, now, tc.window)
			assert.Equal(t, tc.wantOK, ok)
			assert.InDelta(t, tc.want, got, 1e-9)
		})
	}
}
