package session

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogprogressnote"
	"github.com/tstapler/stapler-squad/session/ent/reviewverdict"
	"github.com/tstapler/stapler-squad/session/flow"
)

// FlowDoneClaim records what the agent last reported for one criterion before
// one verdict (the latest progress note's status before VerdictAt). It is
// informational and never gates deadlock candidacy (ADR-005).
type FlowDoneClaim struct {
	ItemID         flow.ItemID
	VerdictAt      time.Time
	CriterionIndex int
	AgentStatus    string
	// VerdictOutcome is the criterion outcome in that verdict.
	VerdictOutcome flow.Outcome
}

// Verdicts returns the verdicts created inside w, oldest first. The verdict is
// attributed to its item through item_sessions (one verdict per session, so
// sessions without a verdict contribute nothing). Two bulk queries via eager
// loading, no per-row lookups.
func (r *EntFlowReader) Verdicts(ctx context.Context, w flow.FlowWindow) ([]flow.Verdict, error) {
	rows, err := r.client.ReviewVerdict.Query().
		Where(
			reviewverdict.CreatedAtGTE(w.Start().UTC().Add(-maxOffsetSkew)),
			reviewverdict.CreatedAtLT(w.End().UTC().Add(maxOffsetSkew)),
		).
		WithItemSession(func(q *ent.ItemSessionQuery) { q.WithBacklogItem() }).
		All(ctx)
	if err != nil {
		return nil, flowReadErr("verdicts", err)
	}
	out := make([]flow.Verdict, 0, len(rows))
	for _, row := range rows {
		if !w.Contains(row.CreatedAt) {
			continue
		}
		if v, ok := toFlowVerdict(row); ok {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func toFlowVerdict(row *ent.ReviewVerdict) (flow.Verdict, bool) {
	sess := row.Edges.ItemSession
	if sess == nil || sess.Edges.BacklogItem == nil {
		return flow.Verdict{}, false
	}
	v := flow.Verdict{
		ItemID:       flow.ItemID(sess.Edges.BacklogItem.ID.String()),
		SessionID:    flow.SessionID(sess.SessionUUID),
		CreatedAt:    row.CreatedAt.UTC(),
		Outcome:      flow.Outcome(row.OverallOutcome),
		Summary:      row.Summary,
		DiffHash:     row.DiffHash,
		OverrideBy:   row.OverrideBy,
		PerCriterion: parsePerCriterion(row.PerCriterion),
	}
	return v, true
}

// parsePerCriterion decodes the stored JSON; a malformed value yields nil
// rather than failing the whole read.
func parsePerCriterion(raw string) []flow.CriterionResult {
	if raw == "" {
		return nil
	}
	var cvs []domain.CriterionVerdict
	if err := json.Unmarshal([]byte(raw), &cvs); err != nil {
		return nil
	}
	out := make([]flow.CriterionResult, len(cvs))
	for i, cv := range cvs {
		out[i] = flow.CriterionResult{Index: cv.CriterionIndex, Outcome: flow.Outcome(cv.Outcome)}
	}
	return out
}

// AgentDoneClaims reports per item whether the agent last claimed done, before
// a non-PASS verdict, on a criterion that verdict did not pass. Informational
// only (ADR-005).
func (r *EntFlowReader) AgentDoneClaims(ctx context.Context, w flow.FlowWindow) (map[flow.ItemID]bool, error) {
	details, err := r.AgentDoneClaimDetails(ctx, w)
	if err != nil {
		return nil, err
	}
	out := map[flow.ItemID]bool{}
	for _, d := range details {
		if d.AgentStatus == "done" && d.VerdictOutcome != flow.OutcomePass {
			out[d.ItemID] = true
		}
	}
	return out, nil
}

// AgentDoneClaimDetails returns, for every criterion in each verdict inside w, the
// status of the agent's latest progress note before that verdict. Criteria
// with no earlier note are omitted.
func (r *EntFlowReader) AgentDoneClaimDetails(ctx context.Context, w flow.FlowWindow) ([]FlowDoneClaim, error) {
	verdicts, err := r.Verdicts(ctx, w)
	if err != nil {
		return nil, err
	}
	if len(verdicts) == 0 {
		return nil, nil
	}
	ids := make([]flow.ItemID, 0, len(verdicts))
	for _, v := range verdicts {
		ids = append(ids, v.ItemID)
	}
	notes, err := r.client.BacklogProgressNote.Query().
		Where(
			backlogprogressnote.ItemIDIn(parseFlowItemIDs(ids)...),
			backlogprogressnote.CreatedAtLT(w.End().UTC().Add(maxOffsetSkew)),
		).All(ctx)
	if err != nil {
		return nil, flowReadErr("agent_done_claims", err)
	}
	return doneClaims(verdicts, notes), nil
}

func doneClaims(verdicts []flow.Verdict, notes []*ent.BacklogProgressNote) []FlowDoneClaim {
	type key struct {
		item uuid.UUID
		crit int
	}
	byKey := map[key][]*ent.BacklogProgressNote{}
	for _, n := range notes {
		k := key{n.ItemID, n.CriterionIndex}
		byKey[k] = append(byKey[k], n)
	}
	var out []FlowDoneClaim
	for _, v := range verdicts {
		item, err := uuid.Parse(string(v.ItemID))
		if err != nil {
			continue
		}
		for _, c := range v.PerCriterion {
			if status, ok := latestNoteStatusBefore(byKey[key{item, c.Index}], v.CreatedAt); ok {
				out = append(out, FlowDoneClaim{ItemID: v.ItemID, VerdictAt: v.CreatedAt, CriterionIndex: c.Index, AgentStatus: status, VerdictOutcome: c.Outcome})
			}
		}
	}
	return out
}

func latestNoteStatusBefore(notes []*ent.BacklogProgressNote, before time.Time) (string, bool) {
	var best *ent.BacklogProgressNote
	for _, n := range notes {
		if n.CreatedAt.Before(before) && (best == nil || n.CreatedAt.After(best.CreatedAt)) {
			best = n
		}
	}
	if best == nil {
		return "", false
	}
	return best.Status, true
}
