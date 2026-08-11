package services

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/tokens"
)

func TestComputeHeadroom_should_ExcludeOutOfWindowTurns_When_SummingUsage(t *testing.T) {
	now := time.Now()
	results := []*tokens.ParseResult{
		{
			TurnTimeline: []tokens.TurnStats{
				{Timestamp: now.Add(-6 * time.Hour), Input: 1000},
				{Timestamp: now.Add(-1 * time.Hour), Input: 500},
			},
		},
	}

	estimate := computeHeadroom(results, 1000, false, now)

	if estimate.TokensUsed != 500 {
		t.Errorf("TokensUsed = %d, want 500 (out-of-window turn must be excluded)", estimate.TokensUsed)
	}
	if estimate.PctRemaining != 50.0 {
		t.Errorf("PctRemaining = %v, want 50.0", estimate.PctRemaining)
	}
	if !estimate.Valid {
		t.Error("Valid = false, want true (budget is calibrated)")
	}
}

func TestComputeHeadroom_should_ReturnInvalidEstimate_When_AssumedBudgetIsZeroOrStoreIsLoading(t *testing.T) {
	now := time.Now()
	results := []*tokens.ParseResult{
		{
			TurnTimeline: []tokens.TurnStats{
				{Timestamp: now.Add(-1 * time.Hour), Input: 500},
			},
		},
	}

	t.Run("uncalibrated budget", func(t *testing.T) {
		estimate := computeHeadroom(results, 0, false, now)
		if estimate.Valid {
			t.Error("Valid = true, want false (AssumedWindowTokenBudget<=0)")
		}
		if estimate.PctRemaining != 100.0 {
			t.Errorf("PctRemaining = %v, want 100.0", estimate.PctRemaining)
		}
	})

	t.Run("loading store with calibrated budget", func(t *testing.T) {
		estimate := computeHeadroom(results, 1000, true, now)
		if estimate.Valid {
			t.Error("Valid = true, want false (isLoading must force invalid regardless of budget)")
		}
		if estimate.PctRemaining != 100.0 {
			t.Errorf("PctRemaining = %v, want 100.0", estimate.PctRemaining)
		}
	})
}

func TestComputeHeadroom_should_ClampPctRemainingToZero_When_UsageExceedsBudget(t *testing.T) {
	now := time.Now()
	results := []*tokens.ParseResult{
		{
			TurnTimeline: []tokens.TurnStats{
				{Timestamp: now.Add(-1 * time.Hour), Input: 2000},
			},
		},
	}

	estimate := computeHeadroom(results, 1000, false, now)

	if estimate.PctRemaining != 0 {
		t.Errorf("PctRemaining = %v, want 0 (clamped, not negative)", estimate.PctRemaining)
	}
}

func TestComputeHeadroom_should_ReturnZeroUsage_When_ResultsEmpty(t *testing.T) {
	estimate := computeHeadroom(nil, 1000, false, time.Now())

	if estimate.TokensUsed != 0 {
		t.Errorf("TokensUsed = %d, want 0", estimate.TokensUsed)
	}
	if estimate.PctRemaining != 100.0 {
		t.Errorf("PctRemaining = %v, want 100.0", estimate.PctRemaining)
	}
	if !estimate.Valid {
		t.Error("Valid = false, want true (budget is calibrated, just no usage yet)")
	}
}
