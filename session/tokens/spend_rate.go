package tokens

import "time"

// SpendRateUSDPerHour is the estimated spend rate over the trailing window
// ending at now: the priced cost of every turn stamped in (now-window, now],
// scaled to dollars per hour. ok is false when window <= 0 or any in-window
// turn is unpriced, so a partial total is never reported as a real rate.
// An idle session yields (0, true).
func SpendRateUSDPerHour(r *ParseResult, pt *PricingTable, now time.Time, window time.Duration) (rate float64, ok bool) {
	if r == nil || pt == nil || window <= 0 {
		return 0, false
	}
	cutoff := now.Add(-window)
	var total float64
	// Full scan, not an early break: transcripts can carry out-of-order or
	// unstamped turns, and undercounting would fail quiet.
	for _, turn := range r.TurnTimeline {
		if !turn.Timestamp.After(cutoff) || turn.Timestamp.After(now) {
			continue
		}
		cost, priced := pt.EstimateTurnCost(turn)
		if !priced {
			return 0, false
		}
		total += cost
	}
	return total / window.Hours(), true
}
