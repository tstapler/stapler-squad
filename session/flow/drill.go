package flow

import "fmt"

// DrillKey names the item set behind a displayed number so the UI and MCP can
// open "which items?" without recomputing. Keys are defined with the metric
// that registers them.
type DrillKey string

// Drill keys registered by the dwell/funnel/rework/lead-time/PR-fix metrics.
const (
	DrillReworkReviewFail  DrillKey = "rework.review_fail"
	DrillReworkBounce      DrillKey = "rework.bounce"
	DrillReworkPRFix       DrillKey = "rework.pr_fix"
	DrillThrashEpisode     DrillKey = "thrash.episode"
	DrillFPY               DrillKey = "fpy.first_pass"
	DrillFPYThrashAffected DrillKey = "fpy.thrash_affected"
	DrillLeadTime          DrillKey = "tiles.lead_time"
	DrillPRFixDwell        DrillKey = "pr.fix_dwell"
)

// DrillFunnelStage is the key for the items that entered one funnel stage.
func DrillFunnelStage(stage string) DrillKey {
	return DrillKey(fmt.Sprintf("funnel.stage.%s", stage))
}

// DrillSets maps each registered DrillKey to its items, sorted and de-duplicated.
type DrillSets map[DrillKey][]ItemID

// add appends item under key unless it is already the last entry. Callers add
// in ascending item order, which keeps every set sorted and unique.
func (d DrillSets) add(key DrillKey, item ItemID) {
	if list := d[key]; len(list) > 0 && list[len(list)-1] == item {
		return
	}
	d[key] = append(d[key], item)
}
