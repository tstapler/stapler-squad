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
	for i := len(r.TurnTimeline) - 1; i >= 0; i-- {
		turn := r.TurnTimeline[i]
		if !turn.Timestamp.After(cutoff) {
			// Timeline is in transcript order; older turns can't re-enter the window.
			break
		}
		if turn.Timestamp.After(now) {
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
