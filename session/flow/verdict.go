package flow

import (
	"slices"
	"strings"
	"time"
)

// SessionID identifies an agent session (string UUID).
type SessionID string

// Outcome is a review verdict outcome, overall or per criterion.
type Outcome string

// Outcome values as stored in review_verdicts.
const (
	OutcomePass         Outcome = "PASS"
	OutcomePartial      Outcome = "PARTIAL"
	OutcomeFail         Outcome = "FAIL"
	OutcomeUnverifiable Outcome = "UNVERIFIABLE"
)

// CriterionResult is one acceptance criterion's outcome inside a verdict.
type CriterionResult struct {
	Index   int
	Outcome Outcome
}

// Verdict is one review_verdicts row. DiffHash is empty when unknown; the
// unusable diff_truncated column is deliberately not carried (ADR-005).
type Verdict struct {
	ItemID       ItemID
	SessionID    SessionID
	CreatedAt    time.Time
	Outcome      Outcome
	Summary      string
	PerCriterion []CriterionResult
	DiffHash     string
	OverrideBy   string
}

// VerdictClass separates real reviewer judgments from system noise. Closed enum.
type VerdictClass int

// VerdictClass values (ADR-005).
const (
	ClassJudgment VerdictClass = iota
	ClassNoCommits
	ClassSelfCheckFailed
	ClassReviewerDegraded
	ClassAutoVerified
)

var classNames = [...]string{
	ClassJudgment:         "judgment",
	ClassNoCommits:        "no_commits",
	ClassSelfCheckFailed:  "self_check_failed",
	ClassReviewerDegraded: "reviewer_degraded",
	ClassAutoVerified:     "auto_verified",
}

// AllVerdictClasses lists every VerdictClass.
func AllVerdictClasses() []VerdictClass {
	return []VerdictClass{ClassJudgment, ClassNoCommits, ClassSelfCheckFailed, ClassReviewerDegraded, ClassAutoVerified}
}

// String returns the wire name, or "unknown" for an out-of-range value.
func (c VerdictClass) String() string {
	if c < 0 || int(c) >= len(classNames) {
		return "unknown"
	}
	return classNames[c]
}

// verdictRule maps a summary prefix to a class. Matching is case-sensitive on
// the trimmed summary; the table is closed, so a new reviewer shape needs a row.
type verdictRule struct {
	prefix string
	class  VerdictClass
}

var verdictRules = []verdictRule{
	{"Review blocked: no committed changes", ClassNoCommits},
	{"Review UNVERIFIABLE: codebase-read capability self-check failed", ClassSelfCheckFailed},
	{"Review blocked: no diff could be computed", ClassReviewerDegraded},
	{"Degraded to UNVERIFIABLE", ClassReviewerDegraded},
	{"Review UNVERIFIABLE: no acceptance criteria", ClassReviewerDegraded},
	{"Auto-verified by reconcileBouncingItems", ClassAutoVerified},
}

// systemSummaryPrefixes mark summaries written by the system rather than a
// reviewer; one that matches no verdictRules row is table drift.
var systemSummaryPrefixes = []string{"Review blocked:", "Review UNVERIFIABLE:", "Degraded to", "Auto-verified"}

// ClassifyVerdict returns the verdict's class. recognized is false when the
// summary looks system-written yet matches no table row; it is then reported as
// Judgment (the conservative choice) and counted in VerdictSummary.Unrecognized.
func ClassifyVerdict(v Verdict) (class VerdictClass, recognized bool) {
	s := strings.TrimSpace(v.Summary)
	for _, r := range verdictRules {
		if strings.HasPrefix(s, r.prefix) {
			return r.class, true
		}
	}
	for _, p := range systemSummaryPrefixes {
		if strings.HasPrefix(s, p) {
			return ClassJudgment, false
		}
	}
	return ClassJudgment, true
}

// VerdictSummary is the raw and adjusted view of a verdict set. Adjusted rates
// use Judgment verdicts only.
type VerdictSummary struct {
	Total            int
	ByOutcome        map[Outcome]int
	ByClass          map[VerdictClass]int
	NonPass          int
	RawFail          int
	RawFailRate      float64
	AdjustedTotal    int
	AdjustedFail     int
	AdjustedFailRate float64
	Unrecognized     int
	Overridden       int
	OverrideRate     float64

	nonPassByClass map[VerdictClass]int
}

// NonJudgmentNonPass counts non-PASS verdicts that say nothing about the code:
// no-commits and self-check failures.
func (s VerdictSummary) NonJudgmentNonPass() int {
	return s.nonPassByClass[ClassNoCommits] + s.nonPassByClass[ClassSelfCheckFailed]
}

// ReviewerAndSystemNonPass is NonJudgmentNonPass plus reviewer-degraded verdicts.
func (s VerdictSummary) ReviewerAndSystemNonPass() int {
	return s.NonJudgmentNonPass() + s.nonPassByClass[ClassReviewerDegraded]
}

// SummarizeVerdicts tallies verdicts by outcome and class. Empty input yields
// zero rates, never NaN.
func SummarizeVerdicts(vs []Verdict) VerdictSummary {
	s := VerdictSummary{
		Total:          len(vs),
		ByOutcome:      make(map[Outcome]int),
		ByClass:        make(map[VerdictClass]int),
		nonPassByClass: make(map[VerdictClass]int),
	}
	for _, v := range vs {
		s.ByOutcome[v.Outcome]++
		class, ok := ClassifyVerdict(v)
		s.ByClass[class]++
		if !ok {
			s.Unrecognized++
		}
		if v.Outcome != OutcomePass {
			s.NonPass++
			s.nonPassByClass[class]++
		}
		if v.OverrideBy != "" {
			s.Overridden++
		}
		if class == ClassJudgment {
			s.AdjustedTotal++
			if v.Outcome == OutcomeFail {
				s.AdjustedFail++
			}
		}
	}
	s.RawFail = s.ByOutcome[OutcomeFail]
	s.RawFailRate = ratio(s.RawFail, s.Total)
	s.AdjustedFailRate = ratio(s.AdjustedFail, s.AdjustedTotal)
	s.OverrideRate = ratio(s.Overridden, s.Total)
	return s
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// SessionVerdictRow is one row of the item x sessions x verdicts join; Verdict
// is nil for a session that was never reviewed.
type SessionVerdictRow struct {
	ItemID    ItemID
	SessionID SessionID
	Verdict   *Verdict
}

// JoinVerdictsBySession groups verdicts per item, keeping at most one verdict
// per session. review_verdicts is unique per session, so this is a join-safety
// guard against row inflation, not a dedup of real data. Each item's verdicts
// are ordered by CreatedAt.
func JoinVerdictsBySession(rows []SessionVerdictRow) map[ItemID][]Verdict {
	seen := make(map[SessionID]bool)
	out := make(map[ItemID][]Verdict)
	for _, r := range rows {
		if r.Verdict == nil || seen[r.SessionID] {
			continue
		}
		seen[r.SessionID] = true
		out[r.ItemID] = append(out[r.ItemID], *r.Verdict)
	}
	for _, list := range out {
		slices.SortStableFunc(list, func(a, b Verdict) int { return a.CreatedAt.Compare(b.CreatedAt) })
	}
	return out
}
