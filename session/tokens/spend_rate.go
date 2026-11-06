package tokens

import "time"

// RollingSpendRate returns the session's priced spend over the trailing
// window ending at now, scaled to USD per hour. A partially filled window is
// not extrapolated: a session younger than window is divided by the full
// window, so a brief burst cannot masquerade as a sustained rate.
//
// ok is false when no rate can be stated (nil inputs, non-positive window, or
// no priced turn inside the window); callers must treat that as "no alert",
// never as $0. Unpriced and timestampless turns are skipped, which can only
// understate the rate.
func RollingSpendRate(r *ParseResult, pt *PricingTable, now time.Time, window time.Duration) (usdPerHour float64, ok bool) {
	if r == nil || pt == nil || window <= 0 {
		return 0, false
	}
	start := now.Add(-window)
	var total float64
	for _, turn := range r.TurnTimeline {
		if turn.Timestamp.IsZero() || !turn.Timestamp.After(start) || turn.Timestamp.After(now) {
			continue
		}
		if cost, priced := pt.EstimateTurnCost(turn); priced {
			total += cost
			ok = true
		}
	}
	if !ok {
		return 0, false
	}
	return total / window.Hours(), true
}
