package services

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/session/tokens"
)

// fakeResultsWithUsage builds a single fake ParseResult with one in-window
// (1h ago) turn totaling exactly usedTokens across Input+Output+CacheCreation+CacheRead.
func fakeResultsWithUsage(usedTokens int64) []*tokens.ParseResult {
	return []*tokens.ParseResult{
		{
			TurnTimeline: []tokens.TurnStats{
				{Timestamp: time.Now().Add(-time.Hour), Input: usedTokens},
			},
		},
	}
}

// countingFeatureController is a call-count-tracking test double for
// FeatureController, distinct from fakeFeatureController (feature_flags_test.go)
// which only tracks bool flags, not counts — several QuotaGate regression
// tests need to assert exact call counts across multiple Reconcile ticks.
type countingFeatureController struct {
	enabled      bool
	enableCalls  int
	disableCalls int
}

func (c *countingFeatureController) Enable(_ context.Context) error {
	c.enableCalls++
	c.enabled = true
	return nil
}

func (c *countingFeatureController) Disable() error {
	c.disableCalls++
	c.enabled = false
	return nil
}

func (c *countingFeatureController) IsEnabled() bool { return c.enabled }
