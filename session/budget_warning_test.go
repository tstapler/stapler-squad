package session

import "testing"

func TestEvaluateBudgetThreshold_should_ReturnWarnTrue_When_CumulativeSpendMeetsOrExceedsThreshold(t *testing.T) {
	threshold := 5.00

	cases := []struct {
		name               string
		cumulativeSpentUSD float64
		wantWarn           bool
	}{
		{"just under threshold", 4.99, false},
		{"exactly at threshold", 5.00, true},
		{"just over threshold", 5.05, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateBudgetThreshold("bl_abc123", "triage", &threshold, tc.cumulativeSpentUSD)
			if got != tc.wantWarn {
				t.Errorf("EvaluateBudgetThreshold(spent=%v) = %v, want %v", tc.cumulativeSpentUSD, got, tc.wantWarn)
			}
		})
	}
}

func TestEvaluateBudgetThreshold_should_ReturnWarnFalse_When_ThresholdIsNil(t *testing.T) {
	got := EvaluateBudgetThreshold("bl_abc123", "work", nil, 1_000_000.00)
	if got {
		t.Errorf("EvaluateBudgetThreshold with nil threshold = true, want false regardless of spend")
	}
}
