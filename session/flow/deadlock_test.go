package flow

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

var deadlockNow = verdictEpoch.AddDate(0, 0, 30)

// dvSpec describes one test verdict: its outcome, the criterion it fails (-1
// for none) and its diff hash.
type dvSpec struct {
	outcome Outcome
	crit    int
	hash    string
}

func fail(crit int) dvSpec { return dvSpec{OutcomeFail, crit, ""} }

func mkHistory(item ItemID, specs ...dvSpec) ItemReviewHistory {
	h := ItemReviewHistory{ItemID: item, Status: domain.BacklogStatusReview}
	for i, sp := range specs {
		v := Verdict{
			ItemID:    item,
			SessionID: SessionID(fmt.Sprintf("%s-s%d", item, i)),
			CreatedAt: verdictEpoch.Add(time.Duration(i) * time.Hour),
			Outcome:   sp.outcome,
			Summary:   sumJudgment,
			DiffHash:  sp.hash,
		}
		for c := range 4 {
			o := OutcomePass
			if c == sp.crit {
				o = sp.outcome
			}
			v.PerCriterion = append(v.PerCriterion, CriterionResult{Index: c, Outcome: o})
		}
		h.Verdicts = append(h.Verdicts, v)
	}
	return h
}

func detect(items ...ItemReviewHistory) DeadlockReport {
	return DetectDeadlocks(items, DefaultConfig(), FixedClock{T: deadlockNow})
}

func TestDeadlock_should_FlagCandidate_When_ThreeJudgmentFailsShareCriterion(t *testing.T) {
	t.Parallel()
	h := mkHistory("a", fail(3), dvSpec{OutcomePartial, 3, ""}, fail(3))

	r := detect(h)
	require.Len(t, r.Candidates, 1)
	c := r.Candidates[0]
	assert.Equal(t, 3, c.RunLength)
	assert.True(t, c.HasRepeatedCriterion)
	assert.Equal(t, 3, c.RepeatedCriterion)
	assert.Equal(t, "FRF", c.Sequence)
	assert.Zero(t, c.DiffUnchangedPairs)
}

func TestDeadlock_should_NotFlagCandidate_When_RunHasNoSecondSignal(t *testing.T) {
	t.Parallel()
	t.Run("three different criteria and distinct hashes", func(t *testing.T) {
		t.Parallel()
		r := detect(mkHistory("a", dvSpec{OutcomeFail, 0, "h1"}, dvSpec{OutcomeFail, 1, "h2"}, dvSpec{OutcomeFail, 2, "h3"}))
		assert.Empty(t, r.Candidates)
	})
	t.Run("equal but empty hashes are not a signal", func(t *testing.T) {
		t.Parallel()
		r := detect(mkHistory("a", fail(0), fail(1), fail(2)))
		assert.Empty(t, r.Candidates)
	})
	t.Run("run shorter than the minimum", func(t *testing.T) {
		t.Parallel()
		r := detect(mkHistory("a", fail(3), fail(3)))
		assert.Empty(t, r.Candidates)
	})
	t.Run("a real PASS breaks the run", func(t *testing.T) {
		t.Parallel()
		r := detect(mkHistory("a", fail(3), fail(3), dvSpec{OutcomePass, -1, ""}, fail(3)))
		assert.Empty(t, r.Candidates)
	})
}

func TestDeadlock_should_FlagCandidateWithDiffSignal_When_ConsecutiveHashesMatch(t *testing.T) {
	t.Parallel()
	r := detect(mkHistory("a", dvSpec{OutcomeFail, 0, "same"}, dvSpec{OutcomeFail, 1, "same"}, dvSpec{OutcomeFail, 2, "other"}))
	require.Len(t, r.Candidates, 1)
	assert.Equal(t, 1, r.Candidates[0].DiffUnchangedPairs)
	assert.False(t, r.Candidates[0].HasRepeatedCriterion)
}

func TestDeadlock_should_SkipNonJudgmentsAndCarryAgentClaimInfoOnly_When_NoCommitsVerdictInRun(t *testing.T) {
	t.Parallel()
	h := mkHistory("a", fail(3), fail(3), fail(3), fail(3))
	h.Verdicts[1].Summary = sumNoCommits // F, F(no committed changes), F, F
	h.AgentClaimedDone = true

	r := detect(h)
	require.Len(t, r.Candidates, 1)
	assert.Equal(t, 3, r.Candidates[0].RunLength, "the no-commits verdict neither extends nor breaks the run")
	assert.Equal(t, "FFFF", r.Candidates[0].Sequence, "the sequence still shows every verdict")
	assert.True(t, r.Candidates[0].AgentClaimedDone)

	h.AgentClaimedDone = false
	r2 := detect(h)
	require.Len(t, r2.Candidates, 1, "removing the agent claim does not change candidacy")
	assert.False(t, r2.Candidates[0].AgentClaimedDone)
}

func TestDeadlock_should_SetResolutionAndInFlight_When_HumanOverrideAutoVerifiedOrFreshVerdict(t *testing.T) {
	t.Parallel()
	finish := func(h ItemReviewHistory, mutate func(*Verdict)) ItemReviewHistory {
		pass := mkHistory(h.ItemID, dvSpec{OutcomePass, -1, ""}).Verdicts[0]
		pass.CreatedAt = h.Verdicts[len(h.Verdicts)-1].CreatedAt.Add(time.Hour)
		pass.SessionID = "final"
		mutate(&pass)
		h.Verdicts = append(h.Verdicts, pass)
		return h
	}
	base := func(id ItemID) ItemReviewHistory { return mkHistory(id, fail(3), fail(3), fail(3)) }

	override := finish(base("override"), func(v *Verdict) { v.OverrideBy = "user" })
	auto := finish(base("auto"), func(v *Verdict) { v.Summary = sumAuto })
	passed := finish(base("passed"), func(*Verdict) {})
	archived := base("archived")
	archived.Status = domain.BacklogStatusArchived
	open := base("open")
	fresh := base("fresh")
	fresh.Verdicts[2].CreatedAt = deadlockNow.Add(-10 * time.Minute)

	r := detect(override, auto, passed, archived, open, fresh)
	got := map[ItemID]DeadlockCandidate{}
	for _, c := range r.Candidates {
		got[c.ItemID] = c
	}
	require.Len(t, got, 6)
	assert.Equal(t, ResolutionHumanOverride, got["override"].Resolution)
	assert.Equal(t, ResolutionAutoVerified, got["auto"].Resolution)
	assert.Equal(t, ResolutionPassed, got["passed"].Resolution)
	assert.Equal(t, ResolutionArchived, got["archived"].Resolution)
	assert.Equal(t, ResolutionOpen, got["open"].Resolution)
	assert.Equal(t, "FFFP", got["passed"].Sequence)

	assert.True(t, got["fresh"].InFlight, "verdict 10 minutes old")
	assert.False(t, got["open"].InFlight)
	assert.False(t, got["override"].InFlight, "resolved items are never in flight")
	assert.Equal(t, 1, r.StalledCount, "only the open, not-in-flight item is stalled")
	assert.Contains(t, r.Estimated, EstimatedDeadlockHeuristic)
}

func TestDeadlock_should_FlagReviewerStreakNotCandidate_When_ThreeRealUnverifiable(t *testing.T) {
	t.Parallel()
	u := dvSpec{OutcomeUnverifiable, -1, ""}
	r := detect(mkHistory("rev", u, u, u))

	assert.Empty(t, r.Candidates, "a reviewer-side streak is not an agent deadlock")
	require.Len(t, r.ReviewerStreaks, 1)
	assert.Equal(t, ItemID("rev"), r.ReviewerStreaks[0].ItemID)
	assert.Equal(t, 3, r.ReviewerStreaks[0].Length)

	assert.Empty(t, detect(mkHistory("rev", u, u, fail(1), u)).ReviewerStreaks, "a FAIL breaks the streak")
}

func TestDeadlock_should_QualifyWithShorterRun_When_BounceCapExhausted(t *testing.T) {
	t.Parallel()
	h := mkHistory("capped", fail(0), fail(1))
	assert.Empty(t, detect(h).Candidates)

	h.CapExhausted = true
	r := detect(h)
	require.Len(t, r.Candidates, 1)
	assert.Equal(t, 2, r.Candidates[0].RunLength)
	assert.True(t, r.Candidates[0].CapExhausted)
}

func TestDeadlock_should_RankByRunLengthAndCarryWeakFilterNote_When_DefaultPage(t *testing.T) {
	t.Parallel()
	items := make([]ItemReviewHistory, 0, 41)
	for i := range 41 {
		specs := []dvSpec{fail(3), fail(3), fail(3)}
		if i == 7 {
			specs = append(specs, fail(3), fail(3))
		}
		h := mkHistory(ItemID(fmt.Sprintf("item-%02d", i)), specs...)
		h.Verdicts[len(h.Verdicts)-1].CreatedAt = verdictEpoch.Add(time.Duration(i+10) * time.Hour)
		items = append(items, h)
	}

	r := detect(items...)
	require.Len(t, r.Candidates, 41)
	assert.Equal(t, ItemID("item-07"), r.Candidates[0].ItemID, "longest run first")
	assert.Equal(t, ItemID("item-40"), r.Candidates[1].ItemID, "ties: latest verdict first")

	p := r.Page(0)
	assert.Len(t, p.Candidates, DefaultDeadlockPageSize)
	assert.Equal(t, 41, p.TotalCount)
	assert.True(t, p.HasMore)
	assert.Equal(t, WeakFilterNote, p.WeakFilter)
	assert.Contains(t, p.Estimated, EstimatedDeadlockHeuristic)
	assert.Len(t, r.Page(100).Candidates, 41)
	assert.False(t, r.Page(100).HasMore)
}

func TestDeadlock_should_ResolveFixtureItems_When_LiveShapesLoaded(t *testing.T) {
	t.Parallel()
	byItem := map[string][]Verdict{}
	for _, fv := range loadFixture[fixtureVerdicts](t, "verdicts.json").Verdicts {
		ts, err := time.Parse(fixtureTimeLayout, fv.CreatedAt)
		require.NoError(t, err)
		v := Verdict{
			ItemID: ItemID(fv.ItemID), SessionID: SessionID(fv.SessionID), CreatedAt: ts.UTC(),
			Outcome: Outcome(fv.OverallOutcome), Summary: fv.Summary, DiffHash: fv.DiffHash, OverrideBy: fv.OverrideBy,
		}
		if fv.ItemID == "0791f5b4" || fv.ItemID == "5eaaa473" {
			v.DiffHash = "stuck-diff" // the fixture is shape-only: supply the no-progress signal
		}
		for _, c := range fv.PerCriterion {
			v.PerCriterion = append(v.PerCriterion, CriterionResult{Index: c.CriterionIndex, Outcome: Outcome(c.Outcome)})
		}
		byItem[fv.ItemID] = append(byItem[fv.ItemID], v)
	}
	last := byItem["0791f5b4"]
	last[len(last)-1].Summary = sumAuto // live 0791f5b4 ends in a synthetic auto-verified PASS
	items := make([]ItemReviewHistory, 0, len(byItem))
	for id, vs := range byItem {
		items = append(items, ItemReviewHistory{ItemID: ItemID(id), Status: domain.BacklogStatusReview, Verdicts: vs, AgentClaimedDone: true})
	}

	r := detect(items...)
	got := map[ItemID]DeadlockCandidate{}
	for _, c := range r.Candidates {
		got[c.ItemID] = c
	}
	require.Contains(t, got, ItemID("0791f5b4"))
	assert.Equal(t, ResolutionAutoVerified, got["0791f5b4"].Resolution)
	assert.Equal(t, "FRUUURURRURRRP", got["0791f5b4"].Sequence)
	require.Contains(t, got, ItemID("5eaaa473"))
	assert.Equal(t, ResolutionHumanOverride, got["5eaaa473"].Resolution)
	require.Contains(t, got, ItemID("rep-crit"))
	assert.Equal(t, 3, got["rep-crit"].RepeatedCriterion)
	assert.NotContains(t, got, ItemID("same-diff"), "a shared hash alone does not make a run of 2 a candidate")
	assert.NotContains(t, got, ItemID("nocommit-a"))
}

func TestDeadlock_should_ResolveAndNotStall_When_StatusMovedPastReview(t *testing.T) {
	t.Parallel()
	mk := func(id ItemID, st domain.BacklogStatus) ItemReviewHistory {
		h := mkHistory(id, fail(3), fail(3), fail(3))
		h.Status = st
		return h
	}
	r := detect(mk("done", domain.BacklogStatusDone), mk("prp", domain.BacklogStatusPRPending), mk("stuck", domain.BacklogStatusReview))

	got := map[ItemID]DeadlockCandidate{}
	for _, c := range r.Candidates {
		got[c.ItemID] = c
	}
	require.Len(t, got, 3)
	assert.Equal(t, ResolutionAdvanced, got["done"].Resolution)
	assert.Equal(t, ResolutionAdvanced, got["prp"].Resolution)
	assert.Equal(t, ResolutionOpen, got["stuck"].Resolution)
	assert.Equal(t, 1, r.StalledCount, "only the item still in review is stalled")
}
