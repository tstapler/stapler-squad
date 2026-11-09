package session

import (
	"context"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/flow"
)

var (
	_ flow.VerdictSource = (*EntFlowReader)(nil)
	_ flow.StuckSource   = (*EntFlowReader)(nil)
	_ flow.ItemSource    = (*EntFlowReader)(nil)
)

// Items returns raw status and title for each listed item that exists. Titles
// are returned unmodified: sanitization for agent consumption happens once, in
// the MCP adapter, never in the reader.
func (r *EntFlowReader) Items(ctx context.Context, ids []flow.ItemID) ([]flow.ItemMeta, error) {
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
func (r *EntFlowReader) OpenItemIDs(ctx context.Context) ([]flow.ItemID, error) {
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
func (r *EntFlowReader) Stages(context.Context) ([]domain.BacklogStatus, error) {
	engine := r.engine
	if engine == nil {
		engine = NewDefaultWorkflowEngine()
	}
	return PipelineStages(engine), nil
}

// PipelineStages derives the stage order from a WorkflowEngine's transition
// graph, since no engine exposes one. Built-in stages keep their canonical
// order; a custom stage is placed right after the earliest built-in stage that
// can transition into it (ties by slug), or just before the terminal archived
// stage when none does. Built-ins absent from the graph are omitted.
func PipelineStages(engine WorkflowEngine) []BacklogStatus {
	known, preds := walkStageGraph(engine)
	var custom []BacklogStatus
	for s := range known {
		if !isBuiltInStage(s) {
			custom = append(custom, s)
		}
	}
	sortStatuses(custom)

	after := map[BacklogStatus][]BacklogStatus{}
	for _, c := range custom {
		anchor := BacklogStatusArchived
		if p, ok := earliestBuiltIn(preds[c]); ok {
			anchor = p
		}
		after[anchor] = append(after[anchor], c)
	}
	var out []BacklogStatus
	for _, b := range builtInStageOrder {
		if b == BacklogStatusArchived {
			out = append(out, after[b]...)
		}
		if known[b] {
			out = append(out, b)
		}
		if b != BacklogStatusArchived {
			out = append(out, after[b]...)
		}
	}
	return out
}

// walkStageGraph explores the graph from every built-in stage and returns the
// stages it found plus each stage's predecessors.
func walkStageGraph(engine WorkflowEngine) (known map[BacklogStatus]bool, preds map[BacklogStatus][]BacklogStatus) {
	known = map[BacklogStatus]bool{}
	preds = map[BacklogStatus][]BacklogStatus{}
	queue := append([]BacklogStatus(nil), builtInStageOrder...)
	seen := map[BacklogStatus]bool{}
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

func isBuiltInStage(s BacklogStatus) bool {
	for _, b := range builtInStageOrder {
		if b == s {
			return true
		}
	}
	return false
}

func earliestBuiltIn(candidates []BacklogStatus) (BacklogStatus, bool) {
	for _, b := range builtInStageOrder {
		for _, c := range candidates {
			if c == b {
				return b, true
			}
		}
	}
	return "", false
}

func sortStatuses(ss []BacklogStatus) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
