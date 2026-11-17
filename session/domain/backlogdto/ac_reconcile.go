package backlogdto

import (
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/session/domain"
)

func acTextKey(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// ReconcileAcWaivers enforces the waiver invariant for any whole-list rewrite of an item's
// criteria (UI edit, headless re-triage, MCP triage result). A waiver belongs to a criterion's
// text, not its position (and a waiver can never be removed by an update: it ends only when the
// criterion's text changes or the criterion is removed, which is audited and reported). Criteria
// with empty text never carry a waiver. Each existing waived criterion is carried forward onto the incoming
// criterion with the same normalized text, wherever it now sits. An incoming waived status that
// matches no existing waiver is refused (ErrAcWaiverViaUpdate). Existing waivers whose text no
// longer appears are returned in cleared so the caller can audit their removal.
func ReconcileAcWaivers(existing, incoming []domain.AcCriterion) (merged, cleared []domain.AcCriterion, err error) {
	waived := map[string][]domain.AcCriterion{}
	for _, c := range existing {
		if k := acTextKey(c.Text); c.Status == domain.AcStatusWaived && k != "" {
			waived[k] = append(waived[k], c)
		}
	}
	merged = make([]domain.AcCriterion, len(incoming))
	copy(merged, incoming)
	for i, c := range merged {
		k := acTextKey(c.Text)
		if q := waived[k]; k != "" && len(q) > 0 {
			w := q[0]
			waived[k] = q[1:]
			merged[i].Status, merged[i].WaiverReason, merged[i].WaivedBy, merged[i].WaivedAt = w.Status, w.WaiverReason, w.WaivedBy, w.WaivedAt
			continue
		}
		if c.Status == domain.AcStatusWaived {
			return nil, nil, fmt.Errorf("criterion %d: %w", c.Index, ErrAcWaiverViaUpdate)
		}
		merged[i].WaiverReason, merged[i].WaivedBy, merged[i].WaivedAt = "", "", ""
	}
	for _, c := range existing {
		if c.Status != domain.AcStatusWaived {
			continue
		}
		k := acTextKey(c.Text)
		if k == "" {
			cleared = append(cleared, c)
			continue
		}
		for _, left := range waived[k] {
			if left.Index == c.Index && left.Text == c.Text {
				cleared = append(cleared, c)
				break
			}
		}
	}
	return merged, cleared, nil
}
