package prs

// Canonical reason names. The manual nudge, PR-fix auto-steer and PR watch key
// their duplicate window on them, so the same fix is not sent twice.
const (
	ReasonNameMergeConflict     = "MERGE_CONFLICT"
	ReasonNameFailingChecks     = "FAILING_CHECKS"
	ReasonNameUnresolvedThreads = "UNRESOLVED_THREADS"
	// ReasonNameChangesRequested has no manual-nudge counterpart, so a
	// changes-requested-only steer never suppresses a thread nudge.
	ReasonNameChangesRequested = "CHANGES_REQUESTED"
)

// DropSubsumedChangesRequested removes CHANGES_REQUESTED when UNRESOLVED_THREADS
// is also present: the manual nudge has no changes-requested reason and its
// thread prompt carries the same actionable content, so keeping the name would
// key the same fix differently on the two paths and send it twice.
func DropSubsumedChangesRequested(names []string) []string {
	hasThreads := false
	for _, n := range names {
		if n == ReasonNameUnresolvedThreads {
			hasThreads = true
		}
	}
	if !hasThreads {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != ReasonNameChangesRequested {
			out = append(out, n)
		}
	}
	return out
}
