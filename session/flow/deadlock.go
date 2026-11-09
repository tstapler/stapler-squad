package flow

import (
	"slices"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// EstimatedDeadlockHeuristic tags every deadlock output as a heuristic (ADR-005).
const EstimatedDeadlockHeuristic = "deadlock_candidates:heuristic"

// WeakFilterNote is shown with every candidate list: the second signal removes few candidates.
const WeakFilterNote = "the no-progress signal removes few candidates; treat as a ranked review-fail streak list, not a verdict on the reviewer"

// DefaultDeadlockPageSize is the default length of a candidate page.
const DefaultDeadlockPageSize = 10

// capExhaustedMinRun is the shortest judgment run that qualifies an item whose
// bounce or rework cap is exhausted; the cap is itself the no-progress signal.
const capExhaustedMinRun = 2

// Resolution is how an item's review history ended.
type Resolution string

// Resolution values (ADR-005).
const (
	ResolutionOpen          Resolution = "open"
	ResolutionPassed        Resolution = "passed"
	ResolutionAutoVerified  Resolution = "auto_verified"
	ResolutionHumanOverride Resolution = "human_override"
	ResolutionArchived      Resolution = "archived"
	// ResolutionAdvanced: the item's current status is past review (pr_pending
	// or done) even though its last verdict is not a PASS. Status is read now,
	// not at window end, so a past window can show an item as advanced that was
	// still stalled then.
	ResolutionAdvanced Resolution = "advanced"
)

// ItemReviewHistory is the deadlock input for one item. AgentClaimedDone is
// informational only; CapExhausted comes from bounce_cap_exhausted or rework_cap
// stuck rows.
type ItemReviewHistory struct {
	ItemID           ItemID
	Status           domain.BacklogStatus
	Verdicts         []Verdict
	AgentClaimedDone bool
	CapExhausted     bool
}

// DeadlockCandidate is an item whose reviews grind without converging.
type DeadlockCandidate struct {
	ItemID               ItemID
	Resolution           Resolution
	Sequence             string // one letter per verdict, e.g. FRUUURURRURRRP
	RunLength            int
	RepeatedCriterion    int
	HasRepeatedCriterion bool
	DiffUnchangedPairs   int
	AgentClaimedDone     bool
	CapExhausted         bool
	InFlight             bool
	LastVerdictAt        time.Time
}

// ReviewerStreak flags 3+ consecutive real UNVERIFIABLE verdicts: a reviewer
// problem, not an agent deadlock.
type ReviewerStreak struct {
	ItemID        ItemID
	Length        int
	LastVerdictAt time.Time
}

// DeadlockReport is the full ranked result; use Page for the default view.
type DeadlockReport struct {
	Candidates      []DeadlockCandidate
	ReviewerStreaks []ReviewerStreak
	StalledCount    int
	Estimated       []string
}

// DeadlockPage is one page of ranked candidates with the totals and labels
// every consumer must show.
type DeadlockPage struct {
	Candidates []DeadlockCandidate
	TotalCount int
	HasMore    bool
	WeakFilter string
	Estimated  []string
}

// Page returns the top limit candidates; limit <= 0 means DefaultDeadlockPageSize.
func (r DeadlockReport) Page(limit int) DeadlockPage {
	if limit <= 0 {
		limit = DefaultDeadlockPageSize
	}
	n := min(limit, len(r.Candidates))
	return DeadlockPage{
		Candidates: r.Candidates[:n],
		TotalCount: len(r.Candidates),
		HasMore:    n < len(r.Candidates),
		WeakFilter: WeakFilterNote,
		Estimated:  r.Estimated,
	}
}

// DetectDeadlocks applies the ADR-005 rule to every item: a run of at least
// cfg.DeadlockMinRun consecutive judgment non-PASS verdicts that includes an
// agent-side FAIL or PARTIAL, plus a no-progress signal inside the run (the same
// failing criterion in cfg.DeadlockRepeatCriterion consecutive verdicts, or an
// unchanged non-empty diff hash between consecutive verdicts). Non-judgment
// verdicts and synthetic PASSes neither extend nor break a run; a real PASS
// ends it. Candidates are ranked by RunLength, ties by latest verdict.
func DetectDeadlocks(items []ItemReviewHistory, cfg Config, clock Clock) DeadlockReport {
	now := clock.Now()
	report := DeadlockReport{Estimated: []string{EstimatedDeadlockHeuristic}}
	for _, h := range items {
		vs := slices.Clone(h.Verdicts)
		slices.SortStableFunc(vs, func(a, b Verdict) int { return a.CreatedAt.Compare(b.CreatedAt) })
		if len(vs) == 0 {
			continue
		}
		if streak := longestUnverifiableStreak(vs); streak >= cfg.DeadlockMinRun {
			report.ReviewerStreaks = append(report.ReviewerStreaks, ReviewerStreak{ItemID: h.ItemID, Length: streak, LastVerdictAt: vs[len(vs)-1].CreatedAt})
		}
		c, ok := bestRun(h, vs, cfg)
		if !ok {
			continue
		}
		c.Resolution = resolve(h.Status, vs[len(vs)-1])
		c.InFlight = c.Resolution == ResolutionOpen && now.Sub(c.LastVerdictAt) < cfg.StalledAfter
		if c.Resolution == ResolutionOpen && !c.InFlight {
			report.StalledCount++
		}
		report.Candidates = append(report.Candidates, c)
	}
	slices.SortStableFunc(report.Candidates, func(a, b DeadlockCandidate) int {
		if a.RunLength != b.RunLength {
			return b.RunLength - a.RunLength
		}
		if d := b.LastVerdictAt.Compare(a.LastVerdictAt); d != 0 {
			return d
		}
		return strings.Compare(string(a.ItemID), string(b.ItemID))
	})
	slices.SortStableFunc(report.ReviewerStreaks, func(a, b ReviewerStreak) int {
		if a.Length != b.Length {
			return b.Length - a.Length
		}
		return strings.Compare(string(a.ItemID), string(b.ItemID))
	})
	return report
}

// isJudgment reports whether v is a real reviewer judgment.
func isJudgment(v Verdict) bool {
	class, _ := ClassifyVerdict(v)
	return class == ClassJudgment
}

// longestUnverifiableStreak is the longest run of consecutive real UNVERIFIABLE
// judgments; non-judgments are skipped and any other judgment outcome breaks it.
func longestUnverifiableStreak(vs []Verdict) int {
	best, cur := 0, 0
	for _, v := range vs {
		if !isJudgment(v) {
			continue
		}
		if v.Outcome == OutcomeUnverifiable {
			cur++
			best = max(best, cur)
			continue
		}
		cur = 0
	}
	return best
}

// bestRun picks the qualifying run with the greatest length (the latest wins a
// tie) and describes it as a candidate without Resolution and InFlight.
func bestRun(h ItemReviewHistory, vs []Verdict, cfg Config) (DeadlockCandidate, bool) {
	var best DeadlockCandidate
	found := false
	consider := func(run []Verdict) {
		c, ok := evaluateRun(run, h.CapExhausted, cfg)
		if ok && (!found || c.RunLength >= best.RunLength) {
			best, found = c, true
		}
	}
	run := make([]Verdict, 0, len(vs))
	for _, v := range vs {
		switch {
		case !isJudgment(v), isSyntheticPass(v):
			continue
		case v.Outcome == OutcomePass:
			consider(run)
			run = run[:0]
		default:
			run = append(run, v)
		}
	}
	consider(run)
	if !found {
		return DeadlockCandidate{}, false
	}
	best.ItemID = h.ItemID
	best.Sequence = sequenceLetters(vs)
	best.AgentClaimedDone = h.AgentClaimedDone
	best.CapExhausted = h.CapExhausted
	best.LastVerdictAt = vs[len(vs)-1].CreatedAt
	return best, true
}

func isSyntheticPass(v Verdict) bool {
	class, _ := ClassifyVerdict(v)
	return class == ClassAutoVerified
}

func evaluateRun(run []Verdict, capExhausted bool, cfg Config) (DeadlockCandidate, bool) {
	agentSide := slices.ContainsFunc(run, func(v Verdict) bool {
		return v.Outcome == OutcomeFail || v.Outcome == OutcomePartial
	})
	if !agentSide || len(run) < min(cfg.DeadlockMinRun, capMinRun(capExhausted, cfg)) {
		return DeadlockCandidate{}, false
	}
	c := DeadlockCandidate{RunLength: len(run), DiffUnchangedPairs: unchangedDiffPairs(run)}
	c.RepeatedCriterion, c.HasRepeatedCriterion = repeatedCriterion(run, cfg.DeadlockRepeatCriterion)

	longEnough := len(run) >= cfg.DeadlockMinRun
	signal := c.HasRepeatedCriterion || c.DiffUnchangedPairs > 0
	if (longEnough && signal) || capExhausted {
		return c, true
	}
	return DeadlockCandidate{}, false
}

// capMinRun is the effective minimum run length given cap exhaustion.
func capMinRun(capExhausted bool, cfg Config) int {
	if capExhausted {
		return capExhaustedMinRun
	}
	return cfg.DeadlockMinRun
}

func unchangedDiffPairs(run []Verdict) int {
	n := 0
	for i := 1; i < len(run); i++ {
		if run[i].DiffHash != "" && run[i].DiffHash == run[i-1].DiffHash {
			n++
		}
	}
	return n
}

// failsCriterion treats any non-PASS criterion outcome as failing, matching the
// live-data simulation behind ADR-005 (UNVERIFIABLE criteria count).
func failsCriterion(v Verdict, index int) bool {
	return slices.ContainsFunc(v.PerCriterion, func(c CriterionResult) bool {
		return c.Index == index && c.Outcome != OutcomePass
	})
}

// repeatedCriterion finds the criterion failing in the longest streak of
// consecutive run verdicts, reporting it only when the streak reaches need.
// Ties go to the lowest criterion index.
func repeatedCriterion(run []Verdict, need int) (index int, ok bool) {
	streaks := map[int]int{}
	bestLen := 0
	for _, v := range run {
		next := map[int]int{}
		for _, c := range v.PerCriterion {
			if failsCriterion(v, c.Index) {
				next[c.Index] = streaks[c.Index] + 1
			}
		}
		streaks = next
		for idx, n := range streaks {
			if n > bestLen || (n == bestLen && idx < index) {
				bestLen, index = n, idx
			}
		}
	}
	return index, bestLen >= need && bestLen > 0
}

func resolve(status domain.BacklogStatus, last Verdict) Resolution {
	if last.Outcome == OutcomePass {
		switch {
		case last.OverrideBy != "":
			return ResolutionHumanOverride
		case isSyntheticPass(last):
			return ResolutionAutoVerified
		default:
			return ResolutionPassed
		}
	}
	switch status {
	case domain.BacklogStatusArchived:
		return ResolutionArchived
	case domain.BacklogStatusPRPending, domain.BacklogStatusDone:
		return ResolutionAdvanced
	}
	return ResolutionOpen
}

var outcomeLetters = map[Outcome]string{
	OutcomeFail:         "F",
	OutcomePartial:      "R",
	OutcomeUnverifiable: "U",
	OutcomePass:         "P",
}

func sequenceLetters(vs []Verdict) string {
	var sb strings.Builder
	for _, v := range vs {
		if l, ok := outcomeLetters[v.Outcome]; ok {
			sb.WriteString(l)
		} else {
			sb.WriteString("?")
		}
	}
	return sb.String()
}
