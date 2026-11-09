package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/flow"
)

func seedFlowSession(t *testing.T, c *ent.Client, item uuid.UUID) *ent.ItemSession {
	t.Helper()
	s, err := c.ItemSession.Create().SetBacklogItemID(item).SetSessionUUID(uuid.NewString()).
		SetSessionRole("work").Save(context.Background())
	require.NoError(t, err)
	return s
}

type flowVerdictSeed struct {
	Outcome, PerCriterion, DiffHash string
	At                              time.Time
}

func seedFlowVerdict(t *testing.T, c *ent.Client, sess *ent.ItemSession, v flowVerdictSeed) {
	t.Helper()
	_, err := c.ReviewVerdict.Create().SetItemSession(sess).SetOverallOutcome(v.Outcome).
		SetPerCriterion(v.PerCriterion).SetSummary("s").SetDiffHash(v.DiffHash).SetCreatedAt(v.At).
		Save(context.Background())
	require.NoError(t, err)
}

func flowWindow(t *testing.T, start, end time.Time) flow.FlowWindow {
	t.Helper()
	w, err := flow.NewFlowWindowBounds(start, end)
	require.NoError(t, err)
	return w
}

func TestEntFlowReader_should_JoinVerdictsThroughItemSessions_When_SessionHasNoVerdict(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	item := seedFlowItem(t, c, "x")
	base := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		s := seedFlowSession(t, c, item)
		seedFlowVerdict(t, c, s, flowVerdictSeed{"FAIL", `[{"criterion_index":3,"outcome":"FAIL","evidence":"e"},{"criterion_index":0,"outcome":"PASS","evidence":""}]`,
			"hash", base.Add(time.Duration(i) * time.Hour)})
	}
	seedFlowSession(t, c, item) // verdictless, no ended_at

	got, err := NewEntFlowReader(c).Verdicts(context.Background(), flowWindow(t, base.Add(-time.Hour), base.Add(24*time.Hour)))
	require.NoError(t, err)
	require.Len(t, got, 3)
	for i, v := range got {
		require.Equal(t, flow.ItemID(item.String()), v.ItemID)
		require.Equal(t, flow.OutcomeFail, v.Outcome)
		require.Equal(t, "hash", v.DiffHash)
		require.Equal(t, base.Add(time.Duration(i)*time.Hour), v.CreatedAt, "ordered by created_at")
		require.Equal(t, []flow.CriterionResult{{Index: 3, Outcome: flow.OutcomeFail}, {Index: 0, Outcome: flow.OutcomePass}}, v.PerCriterion)
	}
}

func TestEntFlowReader_should_ExcludeVerdictsOutsideWindow(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	item := seedFlowItem(t, c, "x")
	base := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	seedFlowVerdict(t, c, seedFlowSession(t, c, item), flowVerdictSeed{"PASS", "", "", base.Add(-48 * time.Hour)})
	seedFlowVerdict(t, c, seedFlowSession(t, c, item), flowVerdictSeed{"PASS", "", "", base})

	got, err := NewEntFlowReader(c).Verdicts(context.Background(), flowWindow(t, base.Add(-time.Hour), base.Add(time.Hour)))
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestEntFlowReader_should_ReturnDoneClaim_When_LastNoteBeforeVerdictIsDone(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	item := seedFlowItem(t, c, "x")
	base := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	note := func(status string, at time.Time) {
		_, err := c.BacklogProgressNote.Create().SetItemID(item).SetCriterionIndex(3).SetStatus(status).SetCreatedAt(at).Save(context.Background())
		require.NoError(t, err)
	}
	note("in_progress", base)
	note("done", base.Add(time.Hour))
	seedFlowVerdict(t, c, seedFlowSession(t, c, item), flowVerdictSeed{"FAIL", `[{"criterion_index":3,"outcome":"FAIL","evidence":""}]`, "", base.Add(2 * time.Hour)})
	note("in_progress", base.Add(3*time.Hour)) // after the verdict: ignored

	r := NewEntFlowReader(c)
	w := flowWindow(t, base, base.Add(24*time.Hour))
	got, err := r.AgentDoneClaimDetails(context.Background(), w)
	require.NoError(t, err)
	require.Equal(t, []FlowDoneClaim{{ItemID: flow.ItemID(item.String()), VerdictAt: base.Add(2 * time.Hour), CriterionIndex: 3, AgentStatus: "done", VerdictOutcome: flow.OutcomeFail}}, got)

	claims, err := r.AgentDoneClaims(context.Background(), w)
	require.NoError(t, err)
	require.Equal(t, map[flow.ItemID]bool{flow.ItemID(item.String()): true}, claims)
}
