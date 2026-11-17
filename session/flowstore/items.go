package flowstore

import (
	"context"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/flow"
)

var (
	_ flow.VerdictSource = (*Reader)(nil)
	_ flow.StuckSource   = (*Reader)(nil)
	_ flow.ItemSource    = (*Reader)(nil)
)

// Items returns raw status and title for each listed item that exists. Titles
// are returned unmodified: sanitization for agent consumption happens once, in
// the MCP adapter, never in the reader.
func (r *Reader) Items(ctx context.Context, ids []flow.ItemID) ([]flow.ItemMeta, error) {
	uids := parseFlowItemIDs(ids)
	if len(uids) == 0 {
		return nil, nil
	}
	rows, err := r.client.BacklogItem.Query().
		Where(backlogitem.IDIn(uids...)).
		Select(backlogitem.FieldID, backlogitem.FieldTitle, backlogitem.FieldStatus).
		All(ctx)
	if err != nil {
		return nil, flowReadErr("items", err)
	}
	out := make([]flow.ItemMeta, len(rows))
	for i, row := range rows {
		out[i] = flow.ItemMeta{ID: flow.ItemID(row.ID.String()), Title: row.Title, Status: domain.BacklogStatus(row.Status)}
	}
	return out, nil
}

// OpenItemIDs returns every item whose status is not terminal (done, archived).
func (r *Reader) OpenItemIDs(ctx context.Context) ([]flow.ItemID, error) {
	uids, err := r.client.BacklogItem.Query().
		Where(backlogitem.StatusNotIn(string(domain.BacklogStatusDone), string(domain.BacklogStatusArchived))).
		IDs(ctx)
	if err != nil {
		return nil, flowReadErr("open_items", err)
	}
	out := make([]flow.ItemID, len(uids))
	for i, id := range uids {
		out[i] = flow.ItemID(id.String())
	}
	return out, nil
}

// Stages returns the pipeline's statuses in pipeline order (see PipelineStages).
func (r *Reader) Stages(context.Context) ([]domain.BacklogStatus, error) {
	return PipelineStages(r.engine), nil
}

// pipelineBuiltInStages mirrors BuiltInStageOrder so this file needs nothing from the session core;
// TestReader_should_ReturnBuiltInStagesInPipelineOrder_When_DefaultEngine pins them together.
var pipelineBuiltInStages = []domain.BacklogStatus{
	domain.BacklogStatusIdea,
	domain.BacklogStatusRefining,
	domain.BacklogStatusReady,
	domain.BacklogStatusQueued,
	domain.BacklogStatusInProgress,
	domain.BacklogStatusReview,
	domain.BacklogStatusPRPending,
	domain.BacklogStatusDone,
	domain.BacklogStatusArchived,
}

// PipelineStages derives the stage order from a WorkflowEngine's transition
// graph, since no engine exposes one. Built-in stages keep their canonical
// order; a custom stage is placed right after the earliest built-in stage that
// can transition into it (ties by slug), or just before the terminal archived
// stage when none does. Built-ins absent from the graph are omitted.
func PipelineStages(engine domain.WorkflowEngine) []domain.BacklogStatus {
	known, preds := walkStageGraph(engine)
	var custom []domain.BacklogStatus
	for s := range known {
		if !isBuiltInStage(s) {
			custom = append(custom, s)
		}
	}
	sortStatuses(custom)

	after := map[domain.BacklogStatus][]domain.BacklogStatus{}
	for _, c := range custom {
		anchor := domain.BacklogStatusArchived
		if p, ok := earliestBuiltIn(preds[c]); ok {
			anchor = p
		}
		after[anchor] = append(after[anchor], c)
	}
	var out []domain.BacklogStatus
	for _, b := range pipelineBuiltInStages {
		if b == domain.BacklogStatusArchived {
			out = append(out, after[b]...)
		}
		if known[b] {
			out = append(out, b)
		}
		if b != domain.BacklogStatusArchived {
			out = append(out, after[b]...)
		}
	}
	return out
}

// walkStageGraph explores the graph from every built-in stage and returns the
// stages it found plus each stage's predecessors.
func walkStageGraph(engine domain.WorkflowEngine) (known map[domain.BacklogStatus]bool, preds map[domain.BacklogStatus][]domain.BacklogStatus) {
	known = map[domain.BacklogStatus]bool{}
	preds = map[domain.BacklogStatus][]domain.BacklogStatus{}
	queue := append([]domain.BacklogStatus(nil), pipelineBuiltInStages...)
	seen := map[domain.BacklogStatus]bool{}
	for len(queue) > 0 {
		from := queue[0]
		queue = queue[1:]
		if seen[from] {
			continue
		}
		seen[from] = true
		targets := engine.AllowedTransitions(from, nil)
		if len(targets) > 0 {
			known[from] = true
		}
		for _, to := range targets {
			known[to] = true
			preds[to] = append(preds[to], from)
			queue = append(queue, to)
		}
	}
	return known, preds
}

func isBuiltInStage(s domain.BacklogStatus) bool {
	for _, b := range pipelineBuiltInStages {
		if b == s {
			return true
		}
	}
	return false
}

func earliestBuiltIn(candidates []domain.BacklogStatus) (domain.BacklogStatus, bool) {
	for _, b := range pipelineBuiltInStages {
		for _, c := range candidates {
			if c == b {
				return b, true
			}
		}
	}
	return "", false
}

func sortStatuses(ss []domain.BacklogStatus) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
