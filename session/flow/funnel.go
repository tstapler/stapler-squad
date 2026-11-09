package flow

import (
	"slices"

	"github.com/tstapler/stapler-squad/session/domain"
)

// FunnelStage is one stage of the pipeline funnel.
type FunnelStage struct {
	Status domain.BacklogStatus
	// Entered is the number of distinct items that entered the stage.
	Entered int
	// Advanced is the number of distinct items that left the stage forward
	// (to any status that is not an earlier stage).
	Advanced int
	// BackEdges counts real transitions from this stage back to an earlier stage.
	BackEdges int
	// BackEdgeTargets splits BackEdges by the earlier stage returned to, in
	// stage order; Items is the distinct items behind each.
	BackEdgeTargets []BackEdgeTarget
	Dwell           Percentiles
}

// BackEdgeTarget is the back-edge transitions from a stage to one earlier stage.
type BackEdgeTarget struct {
	To    domain.BacklogStatus
	Count int
	Items int
}

// FunnelResult is the funnel over a supplied StageOrder.
type FunnelResult struct {
	Stages []FunnelStage
	// Other counts real transitions into a status outside StageOrder (excluding archived).
	Other int
	// Archived counts distinct items that entered archived (throughput only).
	Archived int
	Drill    DrillSets
}

// Funnel counts, per stage of order, items entered and advanced plus rework
// back-edges, over real spans only (thrash is excluded by construction). A
// status outside order lands in the Other bucket instead of being dropped;
// archived is excluded from stages and counted as throughput. The stage list
// is data, never hard-coded here.
func Funnel(spans []Span, order []domain.BacklogStatus, dwell DwellResult, cfg Config) FunnelResult {
	stages := make([]domain.BacklogStatus, 0, len(order))
	for _, st := range order {
		if st != domain.BacklogStatusArchived {
			stages = append(stages, st)
		}
	}
	index := make(map[domain.BacklogStatus]int, len(stages))
	for i, st := range stages {
		index[st] = i
	}
	acc := newFunnelAcc(len(stages))
	for _, s := range spans {
		if rs, ok := s.(RealSpan); ok {
			acc.add(rs, index)
		}
	}
	res := FunnelResult{Other: acc.other, Archived: len(acc.archived), Drill: DrillSets{}}
	for i, st := range stages {
		res.Stages = append(res.Stages, FunnelStage{
			Status:    st,
			Entered:   len(acc.entered[i]),
			Advanced:  len(acc.advanced[i]),
			BackEdges: acc.backEdges[i],
			Dwell:     dwell.Stages[st].Percentiles(cfg.LowSampleN),
		})
		for _, item := range sortedItems(acc.entered[i]) {
			res.Drill.add(DrillFunnelStage(string(st)), item)
		}
		res.Drill[DrillFunnelAdvanced(string(st))] = nil // present even when empty
		for _, item := range sortedItems(acc.advanced[i]) {
			res.Drill.add(DrillFunnelAdvanced(string(st)), item)
		}
		res.Stages[i].BackEdgeTargets = acc.targets(i, stages, res.Drill)
	}
	return res
}

type funnelAcc struct {
	entered, advanced []map[ItemID]struct{}
	backEdges         []int
	// back[from][to] is the transitions and distinct items per stage pair.
	back     []map[int]*backEdgeAcc
	archived map[ItemID]struct{}
	other    int
}

func newFunnelAcc(n int) *funnelAcc {
	a := &funnelAcc{
		entered: make([]map[ItemID]struct{}, n), advanced: make([]map[ItemID]struct{}, n),
		backEdges: make([]int, n), archived: map[ItemID]struct{}{}, back: make([]map[int]*backEdgeAcc, n),
	}
	for i := range a.entered {
		a.entered[i], a.advanced[i] = map[ItemID]struct{}{}, map[ItemID]struct{}{}
		a.back[i] = map[int]*backEdgeAcc{}
	}
	return a
}

type backEdgeAcc struct {
	count int
	items map[ItemID]struct{}
}

// targets lists stage i's back-edge targets in stage order and records each
// target's items in drill.
func (a *funnelAcc) targets(i int, stages []domain.BacklogStatus, drill DrillSets) []BackEdgeTarget {
	var out []BackEdgeTarget
	for to := range i {
		e := a.back[i][to]
		if e == nil {
			continue
		}
		out = append(out, BackEdgeTarget{To: stages[to], Count: e.count, Items: len(e.items)})
		for _, item := range sortedItems(e.items) {
			drill.add(DrillFunnelBackEdge(string(stages[i]), string(stages[to])), item)
		}
	}
	return out
}

func (a *funnelAcc) add(rs RealSpan, index map[domain.BacklogStatus]int) {
	item := rs.Item()
	toIdx, toKnown := index[rs.To()]
	switch {
	case toKnown:
		a.entered[toIdx][item] = struct{}{}
	case rs.To() == domain.BacklogStatusArchived:
		a.archived[item] = struct{}{}
	default:
		a.other++
	}
	fromIdx, fromKnown := index[rs.From()]
	if !fromKnown {
		return
	}
	if toKnown && toIdx < fromIdx {
		a.backEdges[fromIdx]++
		e := a.back[fromIdx][toIdx]
		if e == nil {
			e = &backEdgeAcc{items: map[ItemID]struct{}{}}
			a.back[fromIdx][toIdx] = e
		}
		e.count++
		e.items[item] = struct{}{}
		return
	}
	a.advanced[fromIdx][item] = struct{}{}
}

func sortedItems(set map[ItemID]struct{}) []ItemID {
	out := make([]ItemID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
