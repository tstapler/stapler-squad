package tokens

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSpendRateUSDPerHour(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 22, 18, 0, 0, 0, time.UTC)
	pt := DefaultPricingTable()
	// 1M sonnet cache-read tokens cost $0.30 (+ negligible input/output).
	turn := func(ago time.Duration, model string) TurnStats {
		return TurnStats{Timestamp: now.Add(-ago), Model: model, CacheRead: 1_000_000}
	}

	t.Run("sums only in-window turns and scales to an hour", func(t *testing.T) {
		t.Parallel()
		r := &ParseResult{TurnTimeline: []TurnStats{
			turn(2*time.Hour, "claude-sonnet-4-5"),    // outside
			turn(31*time.Minute, "claude-sonnet-4-5"), // outside
			turn(20*time.Minute, "claude-sonnet-4-5"),
			turn(1*time.Minute, "claude-sonnet-4-5"),
		}}
		rate, ok := SpendRateUSDPerHour(r, pt, now, 30*time.Minute)
		assert.True(t, ok)
		assert.InDelta(t, 0.60/0.5, rate, 1e-9)
	})

	t.Run("turn exactly at the window edge is excluded", func(t *testing.T) {
		t.Parallel()
		r := &ParseResult{TurnTimeline: []TurnStats{turn(30*time.Minute, "claude-sonnet-4-5")}}
		rate, ok := SpendRateUSDPerHour(r, pt, now, 30*time.Minute)
		assert.True(t, ok)
		assert.Zero(t, rate)
	})

	t.Run("idle session is a real zero", func(t *testing.T) {
		t.Parallel()
		rate, ok := SpendRateUSDPerHour(&ParseResult{}, pt, now, 30*time.Minute)
		assert.True(t, ok)
		assert.Zero(t, rate)
	})

	t.Run("unpriced in-window model yields no rate", func(t *testing.T) {
		t.Parallel()
		r := &ParseResult{TurnTimeline: []TurnStats{turn(time.Minute, "gpt-5.6-terra")}}
		_, ok := SpendRateUSDPerHour(r, pt, now, 30*time.Minute)
		assert.False(t, ok)
	})

	t.Run("unpriced turn outside the window is ignored", func(t *testing.T) {
		t.Parallel()
		r := &ParseResult{TurnTimeline: []TurnStats{turn(3*time.Hour, "gpt-5.6-terra"), turn(time.Minute, "claude-sonnet-4-5")}}
		_, ok := SpendRateUSDPerHour(r, pt, now, 30*time.Minute)
		assert.True(t, ok)
	})

	t.Run("zero-stamped and out-of-order turns do not hide in-window spend", func(t *testing.T) {
		t.Parallel()
		r := &ParseResult{TurnTimeline: []TurnStats{
			turn(time.Minute, "claude-sonnet-4-5"),
			turn(3*time.Hour, "claude-sonnet-4-5"),
			{Model: "claude-sonnet-4-5", CacheRead: 1_000_000}, // zero Timestamp
			turn(-time.Minute, "claude-sonnet-4-5"),            // after now
		}}
		rate, ok := SpendRateUSDPerHour(r, pt, now, 30*time.Minute)
		assert.True(t, ok)
		assert.InDelta(t, 0.30/0.5, rate, 1e-9)
	})

	t.Run("nil inputs and non-positive window", func(t *testing.T) {
		t.Parallel()
		_, ok := SpendRateUSDPerHour(nil, pt, now, time.Minute)
		assert.False(t, ok)
		_, ok = SpendRateUSDPerHour(&ParseResult{}, nil, now, time.Minute)
		assert.False(t, ok)
		_, ok = SpendRateUSDPerHour(&ParseResult{}, pt, now, 0)
		assert.False(t, ok)
	})
}
