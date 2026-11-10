package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/check"
)

// Trend is one run's coverage counts and their change against the base, kept
// as the docs-coverage-trend.json artifact so bypass drift (growing `exempt` or
// the grandfathered baseline) stays visible over time (Task 4.1.4b).
type Trend struct {
	Date               string  `json:"date"` // YYYY-MM-DD, orders the series
	InScope            int     `json:"inScope"`
	Covered            int     `json:"covered"`
	Exempt             int     `json:"exempt"`
	Grandfathered      int     `json:"grandfathered"`
	Gap                int     `json:"gap"`
	ExemptDelta        int     `json:"exemptDelta"`        // vs base; 0 when no base
	GrandfatheredDelta int     `json:"grandfatheredDelta"` // vs base; 0 when no base
	ExemptRatio        float64 `json:"exemptRatio"`
}

// NewTrend builds the run's trend row. base is nil when the base is unresolved.
func NewTrend(date string, cur check.Counts, base *check.Counts) Trend {
	t := Trend{
		Date: date, InScope: cur.InScope, Covered: cur.Covered, Exempt: cur.Exempt,
		Grandfathered: cur.Grandfathered, Gap: cur.Gap,
	}
	if cur.InScope > 0 {
		t.ExemptRatio = float64(cur.Exempt) / float64(cur.InScope)
	}
	if base != nil {
		t.ExemptDelta = cur.Exempt - base.Exempt
		t.GrandfatheredDelta = cur.Grandfathered - base.Grandfathered
	}
	return t
}

// WriteJSON writes the trend as the artifact body.
func (t Trend) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}

// Line is the one summary line per run.
func (t Trend) Line() string {
	return fmt.Sprintf("Trend: covered %d | exempt %d (%+d) | grandfathered %d (%+d) | gap %d",
		t.Covered, t.Exempt, t.ExemptDelta, t.Grandfathered, t.GrandfatheredDelta, t.Gap)
}

const (
	trendWindowWeeks      = 4 // "grew in 3 of the last 4 weeks"
	trendGrowthWeeks      = 3
	trendCeilingAlertFrac = 0.8 // ratio past 80% of the ceiling
)

// Analyze reads a series of weekly trend rows (any order, at most the last 8
// are meaningful) and returns the reasons to comment on the flag tracking
// issue: exempt grew in 3 of the last 4 weeks, or its ratio passed 80% of the
// ceiling. A flat or shrinking series returns nothing.
func Analyze(series []Trend, exemptRatioCeiling float64) []string {
	s := append([]Trend(nil), series...)
	sort.SliceStable(s, func(i, j int) bool { return s[i].Date < s[j].Date })
	if len(s) == 0 {
		return nil
	}
	var reasons []string
	// Four weekly steps need five points; with fewer, use the steps available.
	from := len(s) - trendWindowWeeks - 1
	if from < 0 {
		from = 0
	}
	grew := 0
	for i := from + 1; i < len(s); i++ {
		if s[i].Exempt > s[i-1].Exempt {
			grew++
		}
	}
	if grew >= trendGrowthWeeks {
		reasons = append(reasons, fmt.Sprintf("exempt grew in %d of the last %d weeks (now %d)", grew, len(s)-from-1, s[len(s)-1].Exempt))
	}
	last := s[len(s)-1]
	if exemptRatioCeiling > 0 && last.ExemptRatio > trendCeilingAlertFrac*exemptRatioCeiling {
		reasons = append(reasons, fmt.Sprintf("exempt ratio %.1f%% passed 80%% of the %.1f%% ceiling", last.ExemptRatio*100, exemptRatioCeiling*100))
	}
	return reasons
}
