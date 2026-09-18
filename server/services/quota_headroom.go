package services

import (
	"time"

	"github.com/tstapler/stapler-squad/session/tokens"
)

// HeadroomEstimate is the soft/proactive quota signal's output: an estimate of
// remaining session-quota headroom over the trailing 5h window, derived from
// observed token usage against an operator-supplied assumed budget.
type HeadroomEstimate struct {
	WindowStart   time.Time
	WindowEnd     time.Time
	TokensUsed    int64
	AssumedBudget int64
	PctRemaining  float64
	// Valid is false when AssumedBudget<=0 (uncalibrated) or the token store is
	// still loading — both cases mean this estimate must never trigger a pause.
	Valid bool
}

const headroomWindow = 5 * time.Hour

// computeHeadroom turns raw per-session token-usage data into a headroom
// percentage over the trailing 5h window. Pure function: no locking, no I/O.
func computeHeadroom(results []*tokens.ParseResult, assumedBudget int64, isLoading bool, now time.Time) HeadroomEstimate {
	windowStart := now.Add(-headroomWindow)
	if isLoading {
		// A cold or partially-populated TokenStore cache (boot, or mid-restart
		// background walk) must never read as artificially healthy.
		return HeadroomEstimate{
			WindowStart:   windowStart,
			WindowEnd:     now,
			AssumedBudget: assumedBudget,
			PctRemaining:  100.0,
			Valid:         false,
		}
	}

	var used int64
	for _, result := range results {
		if result == nil {
			continue
		}
		for _, turn := range result.TurnTimeline {
			if turn.Timestamp.Before(windowStart) || turn.Timestamp.After(now) {
				continue
			}
			used += turn.Input + turn.Output + turn.CacheCreation + turn.CacheRead
		}
	}

	if assumedBudget <= 0 {
		return HeadroomEstimate{
			WindowStart:   windowStart,
			WindowEnd:     now,
			TokensUsed:    used,
			AssumedBudget: assumedBudget,
			PctRemaining:  100.0,
			Valid:         false,
		}
	}

	pctRemaining := 100 * (1 - float64(used)/float64(assumedBudget))
	if pctRemaining < 0 {
		pctRemaining = 0
	} else if pctRemaining > 100 {
		pctRemaining = 100
	}

	return HeadroomEstimate{
		WindowStart:   windowStart,
		WindowEnd:     now,
		TokensUsed:    used,
		AssumedBudget: assumedBudget,
		PctRemaining:  pctRemaining,
		Valid:         true,
	}
}
