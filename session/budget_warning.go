package session

// EvaluateBudgetThreshold is the pure, advisory-only comparison behind the
// per-item soft budget warning (ADR-003) — never blocks, retries, or errors
// the call whose cost triggered it. itemID/stage aren't consulted by the
// comparison; they're carried so every call site's own "[BudgetWarning]"
// log line can't drift from the value actually checked. Deliberately kept
// separate from CapacityMonitor.checkThresholds, which has enforcement
// (pause/stop) consequences.
func EvaluateBudgetThreshold(itemID, stage string, thresholdUSD *float64, cumulativeSpentUSD float64) (warn bool) { //nolint:revive // itemID/stage kept for call-site symmetry with the log line, see doc comment
	if thresholdUSD == nil {
		return false
	}
	return cumulativeSpentUSD >= *thresholdUSD
}
