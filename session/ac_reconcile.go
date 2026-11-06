package session

import (
	"fmt"
	"strings"
)

func acTextKey(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// reconcileAcWaivers enforces the waiver invariant for any whole-list rewrite of an item's
// criteria (UI edit, headless re-triage, MCP triage result). A waiver belongs to a criterion's
// text, not its position: each existing waived criterion is carried forward onto the incoming
// criterion with the same normalized text, wherever it now sits. An incoming waived status that
// matches no existing waiver is refused (ErrAcWaiverViaUpdate). Existing waivers whose text no
// longer appears are returned in cleared so the caller can audit their removal.
func reconcileAcWaivers(existing, incoming []AcCriterion) (merged, cleared []AcCriterion, err error) {
	waived := map[string][]AcCriterion{}
	for _, c := range existing {
		if c.Status == AcStatusWaived {
			k := acTextKey(c.Text)
			waived[k] = append(waived[k], c)
		}
	}
	merged = make([]AcCriterion, len(incoming))
	copy(merged, incoming)
	for i, c := range merged {
		k := acTextKey(c.Text)
		if q := waived[k]; len(q) > 0 {
			w := q[0]
			waived[k] = q[1:]
			merged[i].Status, merged[i].WaiverReason, merged[i].WaivedBy, merged[i].WaivedAt = w.Status, w.WaiverReason, w.WaivedBy, w.WaivedAt
			continue
		}
		if c.Status == AcStatusWaived {
			return nil, nil, fmt.Errorf("criterion %d: %w", c.Index, ErrAcWaiverViaUpdate)
		}
		merged[i].WaiverReason, merged[i].WaivedBy, merged[i].WaivedAt = "", "", ""
	}
	for _, c := range existing {
		if c.Status != AcStatusWaived {
			continue
		}
		k := acTextKey(c.Text)
		for _, left := range waived[k] {
			if left.Index == c.Index && left.Text == c.Text {
				cleared = append(cleared, c)
				break
			}
		}
	}
	return merged, cleared, nil
}

// CheckNoNewAcWaivers is the early, user-facing form of the repository's waiver invariant: it
// reports ErrAcWaiverViaUpdate when incoming introduces a waived criterion existing does not
// already hold. The repository re-checks authoritatively inside its transaction.
func CheckNoNewAcWaivers(existing, incoming AcCriteriaJSON) error {
	in, err := ParseAcCriteria(incoming)
	if err != nil {
		return err
	}
	prior, _ := ParseAcCriteria(existing)
	_, _, err = reconcileAcWaivers(prior, in)
	return err
}
