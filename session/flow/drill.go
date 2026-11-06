package flow

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tstapler/stapler-squad/session/domain"
)

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

// DrillKeys lists the drill keys a client can pass without discovering them
// first: the static keys, one review.class.<class> per verdict class, one
// stuck.<reason> per known stuck reason and one funnel.stage.<status> per
// built-in status. It is sorted and stable, for schema enums and tests.
// Keys outside it that match DrillKeyPatterns (custom funnel stages, future
// stuck reasons) are still accepted by Drilldown.
func DrillKeys() []DrillKey {
	keys := slices.Clone(staticDrillKeys)
	for _, c := range AllVerdictClasses() {
		keys = append(keys, DrillVerdictClass(c))
	}
	for _, r := range domain.AllStuckReasons {
		keys = append(keys, DrillStuckReason(string(r)))
	}
	for _, st := range builtInFunnelStatuses {
		keys = append(keys, DrillFunnelStage(string(st)))
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// builtInFunnelStatuses are the statuses present in every workflow; custom
// stages add funnel.stage.<name> keys at run time.
var builtInFunnelStatuses = []domain.BacklogStatus{
	domain.BacklogStatusIdea, domain.BacklogStatusRefining, domain.BacklogStatusReady, domain.BacklogStatusQueued,
	domain.BacklogStatusInProgress, domain.BacklogStatusReview, domain.BacklogStatusPRPending, domain.BacklogStatusDone,
}

// DrillKeyPatterns documents the dynamic key families, one human-readable
// pattern each.
func DrillKeyPatterns() []string {
	return []string{"funnel.stage.<status>", "review.class.<class>", "stuck.<reason>"}
}

// IsKnownDrillKey reports whether key is in DrillKeys or matches a dynamic
// family prefix with a non-empty suffix.
func IsKnownDrillKey(key DrillKey) bool {
	if _, found := slices.BinarySearch(DrillKeys(), key); found {
		return true
	}
	for _, p := range []string{"funnel.stage.", "review.class.", "stuck."} {
		if rest, ok := strings.CutPrefix(string(key), p); ok && rest != "" {
			return true
		}
	}
	return false
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
