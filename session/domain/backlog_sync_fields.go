package domain

import (
	"encoding/json"
	"fmt"
)

// determineBackwardSyncTarget implements ADR-002's policy: a closed GitHub
// issue maps to BacklogStatusArchived for pre-work statuses only. It never
// targets "done" (would require porting HasUnshippedCode/OverallOutcome
// computation cross-package and risks conflating "closed" with "shipped").
// Returns ok=false when there is no valid target under this policy (item is
// already done/archived, or mid-flight in_progress/review/pr_pending).
func DetermineBackwardSyncTarget(current BacklogStatus) (target BacklogStatus, ok bool) {
	switch current {
	case BacklogStatusIdea, BacklogStatusRefining, BacklogStatusReady, BacklogStatusQueued:
		return BacklogStatusArchived, true
	default:
		// A custom stage (or in_progress/review/pr_pending/done/archived)
		// falls here: no auto-archive target is known for it, so none is
		// computed — correct fail-safe per the "BacklogStatus becomes the
		// open stage-slug type" decision (Epic 2.1, Story 2.1.3e). Whether a
		// custom pre-work stage should also backward-sync to archived is an
		// explicit opt-in for a future epic, not something this default
		// should guess at.
		return "", false
	}
}

// ParseUserModifiedFields deserializes UserModifiedFields JSON (e.g. ["title","description"]).
func ParseUserModifiedFields(raw string) []string {
	if raw == "" {
		return nil
	}
	var fields []string
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil
	}
	return fields
}

// ContainsModifiedField returns true if name is in the fields slice.
func ContainsModifiedField(fields []string, name string) bool {
	for _, f := range fields {
		if f == name {
			return true
		}
	}
	return false
}

// MergeUserModifiedFields adds newFields to the existing JSON-encoded set of
// user-modified field names, deduplicating, and returns the re-serialized JSON.
func MergeUserModifiedFields(raw string, newFields ...string) (string, error) {
	existing := ParseUserModifiedFields(raw)

	seen := make(map[string]bool, len(existing)+len(newFields))
	merged := make([]string, 0, len(existing)+len(newFields))
	for _, f := range existing {
		if !seen[f] {
			seen[f] = true
			merged = append(merged, f)
		}
	}
	for _, f := range newFields {
		if !seen[f] {
			seen[f] = true
			merged = append(merged, f)
		}
	}

	out, err := json.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("failed to marshal user modified fields: %w", err)
	}
	return string(out), nil
}
